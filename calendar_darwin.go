package main

/*
#cgo CFLAGS: -fobjc-arc -Wno-unused-command-line-argument
#cgo LDFLAGS: -framework EventKit -framework AppKit -framework Foundation
#include <stdlib.h>
void  ek_init(void);
int   ek_status(void);
int   ek_request(void);
void  ek_refresh(void);
char *ek_calendars(void);
char *ek_events(double from, double to, const char *hidden);
char *ek_add(const char *title, const char *location, const char *notes, const char *calendarId,
             double start, double end, int allDay, int alarmMinutes);
char *ek_delete(const char *eventId, double start);
char *ek_detail(const char *eventId, double start);
char *ek_update(const char *eventId, double origStart, const char *title, const char *location,
                const char *notes, const char *calendarId, double start, double end, int allDay);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Event is the app's flattened view of a calendar event.
type Event struct {
	ID         string    `json:"id"`
	CalendarID string    `json:"calendarId"`
	Calendar   string    `json:"calendar"`
	Color      string    `json:"color"`
	Title      string    `json:"title"`
	Location   string    `json:"location"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	AllDay     bool      `json:"allDay"`
	Link       string    `json:"link"`
	MeetLink   string    `json:"meetLink"`
	Holiday    bool      `json:"holiday"` // from a holiday calendar: shown as a day tag, not a row
	Writable   bool      `json:"writable"`
}

// EventDetail is the full record for the event card.
type EventDetail struct {
	Event
	Notes     string     `json:"notes"`
	URL       string     `json:"url"`
	Recurring bool       `json:"recurring"`
	Organizer string     `json:"organizer"`
	Attendees []Attendee `json:"attendees"`
}

type Attendee struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status"`
	Me     bool   `json:"me"`
}

var holidayRe = regexp.MustCompile(`(?i)holiday`)

func isHolidayCal(c CalendarInfo) bool {
	return !c.Writable && (holidayRe.MatchString(c.Name) || holidayRe.MatchString(c.Account))
}

type CalendarInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Primary  bool   `json:"primary"`
	Writable bool   `json:"writable"`
	Account  string `json:"account"`
	Google   bool   `json:"google"`
	Source   string `json:"sourceType"`
}

var errNoAccess = errors.New("calendar access not allowed yet — open Settings")

// storeChanged is called (from any thread) when macOS reports calendar changes.
var storeChanged func()

//export goStoreChanged
func goStoreChanged() {
	if storeChanged != nil {
		storeChanged()
	}
}

// Cal wraps EventKit. EKEventStore isn't thread-safe, so every call is serialised.
type Cal struct {
	mu   sync.Mutex
	cals []CalendarInfo
}

func takeString(p *C.char) string {
	s := C.GoString(p)
	C.free(unsafe.Pointer(p))
	return s
}

func (c *Cal) Init() {
	c.mu.Lock()
	defer c.mu.Unlock()
	C.ek_init()
}

// Access: notDetermined | restricted | denied | full | writeOnly
func (c *Cal) Access() string {
	switch C.ek_status() {
	case 0:
		return "notDetermined"
	case 1:
		return "restricted"
	case 2:
		return "denied"
	case 3:
		return "full"
	default:
		return "writeOnly"
	}
}

func (c *Cal) Connected() bool { return c.Access() == "full" }

// RequestAccess shows the macOS permission prompt (blocks until answered).
func (c *Cal) RequestAccess() bool {
	return C.ek_request() == 1 // not under c.mu: the prompt can take a while
}

func (c *Cal) Calendars() ([]CalendarInfo, error) {
	if !c.Connected() {
		return nil, errNoAccess
	}
	c.mu.Lock()
	raw := takeString(C.ek_calendars())
	c.mu.Unlock()
	var out []CalendarInfo
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Primary != out[j].Primary {
			return out[i].Primary
		}
		if out[i].Google != out[j].Google {
			return out[i].Google
		}
		if out[i].Account != out[j].Account {
			return out[i].Account < out[j].Account
		}
		return out[i].Name < out[j].Name
	})
	c.mu.Lock()
	c.cals = out
	c.mu.Unlock()
	return out, nil
}

func (c *Cal) CachedCalendars() []CalendarInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]CalendarInfo(nil), c.cals...)
}

var meetRe = regexp.MustCompile(`https://(meet\.google\.com/[a-z]{3,4}-[a-z]{4}-[a-z]{3,4}|[a-z0-9.-]*zoom\.us/j/[0-9A-Za-z?=&._-]+|teams\.microsoft\.com/l/meetup-join/[^\s"<>]+)`)

type rawEvent struct {
	ID         string  `json:"id"`
	CalendarID string  `json:"calendarId"`
	Calendar   string  `json:"calendar"`
	Color      string  `json:"color"`
	Title      string  `json:"title"`
	Location   string  `json:"location"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	AllDay     bool    `json:"allDay"`
	URL        string  `json:"url"`
	Notes      string  `json:"notes"`
}

// Upcoming returns events from now (including ones in progress) through `days` ahead.
func (c *Cal) Upcoming(cfg *Config, days int) ([]Event, error) {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	evs, err := c.Range(cfg, dayStart, dayStart.AddDate(0, 0, days))
	if err != nil {
		return nil, err
	}
	out := evs[:0]
	for _, e := range evs {
		if e.End.After(now) { // skip ones that already finished today
			out = append(out, e)
		}
	}
	return out, nil
}

// Range returns events overlapping [from, to) across visible calendars, de-duplicated
// (the same event subscribed through two calendars shows once).
func (c *Cal) Range(cfg *Config, from, to time.Time) ([]Event, error) {
	if !c.Connected() {
		return nil, errNoAccess
	}
	cals, err := c.Calendars()
	if err != nil {
		return nil, err
	}
	byID := map[string]CalendarInfo{}
	for _, cal := range cals {
		byID[cal.ID] = cal
	}
	hidden, _ := json.Marshal(cfg.HiddenCalendars)
	ch := C.CString(string(hidden))
	defer C.free(unsafe.Pointer(ch))

	c.mu.Lock()
	raw := takeString(C.ek_events(C.double(from.Unix()), C.double(to.Unix()), ch))
	c.mu.Unlock()

	var rs []rawEvent
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rs))
	seen := map[string]bool{}
	for _, r := range rs {
		ev := rawToEvent(r)
		cal := byID[r.CalendarID]
		ev.Holiday = isHolidayCal(cal)
		ev.Writable = cal.Writable
		key := strings.ToLower(strings.TrimSpace(ev.Title)) + "|" + ev.Start.Format(time.RFC3339) + "|" + fmt.Sprint(ev.AllDay)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].AllDay && !out[j].AllDay
	})
	return out, nil
}

func rawToEvent(r rawEvent) Event {
	ev := Event{ID: r.ID, CalendarID: r.CalendarID, Calendar: r.Calendar, Color: r.Color,
		Title: r.Title, Location: r.Location, AllDay: r.AllDay,
		Start: time.Unix(int64(r.Start), 0), End: time.Unix(int64(r.End), 0)}
	if ev.Title == "" {
		ev.Title = "(No title)"
	}
	for _, s := range []string{r.URL, r.Location, r.Notes} {
		if m := meetRe.FindString(s); m != "" {
			ev.MeetLink = m
			break
		}
	}
	ev.Link = "ical://ekevent/" + url.PathEscape(r.ID) + "?method=show&options=more"
	return ev
}

// Detail loads the full event (notes, attendees…) for the event card.
func (c *Cal) Detail(eventID string, start time.Time) (EventDetail, error) {
	id := C.CString(eventID)
	defer C.free(unsafe.Pointer(id))
	c.mu.Lock()
	raw := takeString(C.ek_detail(id, C.double(start.Unix())))
	c.mu.Unlock()
	var d struct {
		rawEvent
		Error     string     `json:"error"`
		Recurring bool       `json:"recurring"`
		Writable  bool       `json:"writable"`
		Organizer string     `json:"organizer"`
		Attendees []Attendee `json:"attendees"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return EventDetail{}, err
	}
	if d.Error != "" {
		return EventDetail{}, errors.New(d.Error)
	}
	ev := rawToEvent(d.rawEvent)
	ev.Writable = d.Writable
	if d.Attendees == nil {
		d.Attendees = []Attendee{}
	}
	return EventDetail{Event: ev, Notes: d.Notes, URL: d.URL, Recurring: d.Recurring,
		Organizer: d.Organizer, Attendees: d.Attendees}, nil
}

// Update edits one occurrence of an event.
func (c *Cal) Update(eventID string, origStart time.Time, d Draft) error {
	if !c.Connected() {
		return errNoAccess
	}
	cs := []*C.char{C.CString(eventID), C.CString(d.Title), C.CString(d.Location), C.CString(d.Notes), C.CString(d.CalendarID)}
	defer func() {
		for _, p := range cs {
			C.free(unsafe.Pointer(p))
		}
	}()
	allDay := 0
	if d.AllDay {
		allDay = 1
	}
	c.mu.Lock()
	msg := takeString(C.ek_update(cs[0], C.double(origStart.Unix()), cs[1], cs[2], cs[3], cs[4],
		C.double(d.Start.Unix()), C.double(d.End.Unix()), C.int(allDay)))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (c *Cal) Insert(calID string, d Draft) error {
	if !c.Connected() {
		return errNoAccess
	}
	cs := []*C.char{C.CString(d.Title), C.CString(d.Location), C.CString(d.Notes), C.CString(calID)}
	defer func() {
		for _, p := range cs {
			C.free(unsafe.Pointer(p))
		}
	}()
	allDay := 0
	if d.AllDay {
		allDay = 1
	}
	c.mu.Lock()
	msg := takeString(C.ek_add(cs[0], cs[1], cs[2], cs[3], C.double(d.Start.Unix()), C.double(d.End.Unix()),
		C.int(allDay), C.int(d.ReminderMinutes)))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (c *Cal) Delete(eventID string, start time.Time) error {
	id := C.CString(eventID)
	defer C.free(unsafe.Pointer(id))
	c.mu.Lock()
	msg := takeString(C.ek_delete(id, C.double(start.Unix())))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (c *Cal) RefreshSources() {
	c.mu.Lock()
	C.ek_refresh()
	c.mu.Unlock()
}
