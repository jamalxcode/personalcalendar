package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Repeat describes a recurrence. Freq "" or "none" means a one-off event.
type Repeat struct {
	Freq     string `json:"freq"`     // none | daily | weekly | monthly | yearly
	Interval int    `json:"interval"` // every N (default 1)
	Weekdays []int  `json:"weekdays"` // weekly: 0=Sun … 6=Sat
	Until    string `json:"until"`    // YYYY-MM-DD last day (inclusive), "" = no end
	Count    int    `json:"count"`    // number of occurrences, 0 = no limit
}

func (r Repeat) Active() bool { return r.Freq != "" && r.Freq != "none" && r.Freq != "keep" }

var dayShort = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// Text is a human description: "Every Mon & Wed", "Every 2 weeks", "Monthly until 31 Dec".
func (r Repeat) Text() string {
	if !r.Active() {
		return ""
	}
	n := r.Interval
	if n < 1 {
		n = 1
	}
	unit := map[string]string{"daily": "day", "weekly": "week", "monthly": "month", "yearly": "year"}[r.Freq]
	var s string
	switch {
	case r.Freq == "weekly" && len(r.Weekdays) > 0 && n == 1:
		var d []string
		for _, w := range r.Weekdays {
			if w >= 0 && w <= 6 {
				d = append(d, dayShort[w])
			}
		}
		if len(d) == 5 && !contains(r.Weekdays, 5) && !contains(r.Weekdays, 6) {
			s = "Every weekday (Sun–Thu)"
		} else {
			s = "Every " + joinAnd(d)
		}
	case n == 1:
		s = map[string]string{"daily": "Every day", "weekly": "Every week", "monthly": "Every month", "yearly": "Every year"}[r.Freq]
	default:
		s = fmt.Sprintf("Every %d %ss", n, unit)
	}
	if r.Until != "" {
		if t, err := time.Parse("2006-01-02", r.Until); err == nil {
			s += " until " + t.Format("2 Jan 2006")
		}
	} else if r.Count > 0 {
		s += fmt.Sprintf(", %d times", r.Count)
	}
	return s
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	default:
		return strings.Join(xs[:len(xs)-1], ", ") + " & " + xs[len(xs)-1]
	}
}

// rawRepeat is the native (EventKit bridge) shape.
type rawRepeat struct {
	Freq     string  `json:"freq"`
	Interval int     `json:"interval"`
	Weekdays []int   `json:"weekdays"`
	Until    float64 `json:"until"`
	Count    int     `json:"count"`
}

func (r rawRepeat) toRepeat() Repeat {
	out := Repeat{Freq: r.Freq, Interval: r.Interval, Weekdays: r.Weekdays, Count: r.Count}
	if out.Weekdays == nil {
		out.Weekdays = []int{}
	}
	if r.Until > 0 {
		out.Until = time.Unix(int64(r.Until), 0).Format("2006-01-02")
	}
	if out.Freq == "" {
		out.Freq = "none"
	}
	return out
}

func (r Repeat) toRaw() rawRepeat {
	out := rawRepeat{Freq: r.Freq, Interval: r.Interval, Weekdays: r.Weekdays, Count: r.Count}
	if out.Interval < 1 {
		out.Interval = 1
	}
	if out.Weekdays == nil {
		out.Weekdays = []int{}
	}
	if t, err := time.ParseInLocation("2006-01-02", r.Until, time.Local); err == nil {
		out.Until = float64(t.Add(24*time.Hour - time.Second).Unix()) // end of that day
	}
	return out
}

// EventRef points at one occurrence of an event (used for Undo).
type EventRef struct {
	ID    string    `json:"id"`
	Start time.Time `json:"start"`
}

// alarmOffset turns a Draft.Alarm choice into seconds relative to the start.
// "" = keep/none, "none", "0","5","15",… minutes before, "dayBefore9", "sameDay9" (all-day).
func alarmOffset(choice string) (secs float64, has, set bool) {
	switch choice {
	case "":
		return 0, false, false
	case "none":
		return 0, false, true
	case "dayBefore9":
		return -15 * 3600, true, true // 09:00 the day before an all-day event
	case "sameDay9":
		return 9 * 3600, true, true
	}
	if m, err := strconv.Atoi(choice); err == nil && m >= 0 {
		return -float64(m) * 60, true, true
	}
	return 0, false, false
}

// draftJSON serialises a Draft for the native bridge.
func draftJSON(d Draft, isNew bool) string {
	secs, has, set := alarmOffset(d.Alarm)
	if d.ReminderMinutes > 0 { // an explicit "remind me 1h before" wins
		secs, has, set = -float64(d.ReminderMinutes)*60, true, true
	}
	m := map[string]any{
		"title": d.Title, "location": d.Location, "notes": d.Notes, "calendarId": d.CalendarID,
		"start": d.Start.Unix(), "end": d.End.Unix(), "allDay": d.AllDay, "timeZone": d.TimeZone,
		"setAlarm": set, "hasAlarm": has, "alarmSecs": secs,
		"setRepeat": isNew || d.SetRepeat,
	}
	if d.Repeat.Active() {
		m["repeat"] = d.Repeat.toRaw()
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ---- to-dos (Apple Reminders) ----

type Todo struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Notes    string     `json:"notes"`
	List     string     `json:"list"`
	ListID   string     `json:"listId"`
	Color    string     `json:"color"`
	Done     bool       `json:"done"`
	Due      *time.Time `json:"due"` // nil = no date
	HasTime  bool       `json:"hasTime"`
	Priority int        `json:"priority"`
}

type rawTodo struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Notes    string  `json:"notes"`
	List     string  `json:"list"`
	ListID   string  `json:"listId"`
	Color    string  `json:"color"`
	Done     bool    `json:"done"`
	Due      float64 `json:"due"`
	HasTime  bool    `json:"hasTime"`
	Priority int     `json:"priority"`
}

func (r rawTodo) toTodo() Todo {
	t := Todo{ID: r.ID, Title: r.Title, Notes: r.Notes, List: r.List, ListID: r.ListID, Color: r.Color,
		Done: r.Done, HasTime: r.HasTime, Priority: r.Priority}
	if r.Due > 0 {
		d := time.Unix(int64(r.Due), 0)
		t.Due = &d
	}
	return t
}

type TodoList struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Primary bool   `json:"primary"`
}

// ---- free slots ----

type busy struct{ s, e time.Time }

// freeSlots finds up to n gaps of at least dur between dayFrom and dayTo (inclusive days),
// within [earliest, latest) hours each day, skipping weekend days unless allowWeekend.
// Slots start on the half hour; at most 2 per day so suggestions spread across days.
func freeSlots(evs []Event, dayFrom, dayTo time.Time, dur time.Duration, earliest, latest int,
	weekend map[int]bool, allowWeekend bool, now time.Time, n int) []busy {
	var bs []busy
	for _, e := range evs {
		if !e.AllDay && !e.Holiday {
			bs = append(bs, busy{e.Start, e.End})
		}
	}
	var out []busy
	for d := dayFrom; !d.After(dayTo) && len(out) < n; d = d.AddDate(0, 0, 1) {
		if weekend[int(d.Weekday())] && !allowWeekend {
			continue
		}
		perDay := 0
		dayEnd := time.Date(d.Year(), d.Month(), d.Day(), latest, 0, 0, 0, d.Location())
		for t := time.Date(d.Year(), d.Month(), d.Day(), earliest, 0, 0, 0, d.Location()); !t.Add(dur).After(dayEnd); t = t.Add(30 * time.Minute) {
			if t.Before(now.Add(15 * time.Minute)) {
				continue
			}
			end := t.Add(dur)
			clash := false
			for _, b := range bs {
				if b.s.Before(end) && b.e.After(t) {
					clash = true
					break
				}
			}
			if !clash {
				out = append(out, busy{t, end})
				perDay++
				t = end.Add(-30 * time.Minute) // next suggestion after this one
				if perDay >= 2 || len(out) >= n {
					break
				}
			}
		}
	}
	return out
}

// clashesWith returns timed, non-holiday events overlapping [s, e), excluding skipID.
func clashesWith(evs []Event, s, e time.Time, skipID string) []Event {
	out := []Event{}
	for _, x := range evs {
		if x.AllDay || x.Holiday || (skipID != "" && x.ID == skipID) {
			continue
		}
		if x.Start.Before(e) && x.End.After(s) {
			out = append(out, x)
		}
	}
	return out
}
