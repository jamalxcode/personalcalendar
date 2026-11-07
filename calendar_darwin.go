package main

/*
#cgo CFLAGS: -fobjc-arc -Wno-unused-command-line-argument
#cgo LDFLAGS: -framework EventKit -framework AppKit -framework Foundation -framework MapKit -framework CoreLocation
#include <stdlib.h>
void  ek_init(void);
int   ek_status(void);
int   ek_request(void);
void  ek_refresh(void);
char *ek_calendars(void);
char *ek_events(double from, double to, const char *hidden);
char *ek_add_json(const char *json);
char *ek_delete(const char *eventId, double start, int span);
char *ek_detail(const char *eventId, double start);
char *ek_update_json(const char *eventId, double origStart, const char *json, int span);
int   ek_rem_status(void);
int   ek_rem_request(void);
char *ek_rem_lists(void);
char *ek_reminders(void);
char *ek_rem_save(const char *json);
char *ek_rem_delete(const char *rid);
char *clip_read(void);
void tray_show_image(int show);
void tray_set_image(const void *data, int len, int tmpl);
void show_about(const char *version, const char *credits);
void set_dock_visible(int show);
void app_activate(void);
char *sys_prefs(void);
double ek_eta(const char *from, double lat, double lon, const char *place);
char *ek_event_place(const char *eventId, double start);
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
	TimeZone   string    `json:"timeZone"` // IANA zone the event was created in ("" = floating/all-day)
	Recurring  bool      `json:"recurring"`
	Notes      string    `json:"-"` // kept for search only
}

// EventDetail is the full record for the event card.
type EventDetail struct {
	Event
	Notes     string     `json:"notes"`
	URL       string     `json:"url"`
	Organizer string     `json:"organizer"`
	Attendees []Attendee `json:"attendees"`
	Repeat    Repeat     `json:"repeat"`
	HasAlarm  bool       `json:"hasAlarm"`
	AlarmSecs float64    `json:"alarmSecs"`
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
	TimeZone   string  `json:"timeZone"`
	Recurring  bool    `json:"recurring"`
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
		Title: r.Title, Location: r.Location, AllDay: r.AllDay, TimeZone: r.TimeZone,
		Recurring: r.Recurring, Notes: r.Notes,
		Start: time.Unix(int64(r.Start), 0), End: time.Unix(int64(r.End), 0)}
	if r.AllDay {
		ev.TimeZone = ""
	}
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
		Writable  bool       `json:"writable"`
		Organizer string     `json:"organizer"`
		Attendees []Attendee `json:"attendees"`
		Repeat    rawRepeat  `json:"repeat"`
		HasAlarm  bool       `json:"hasAlarm"`
		AlarmSecs float64    `json:"alarmSecs"`
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
	return EventDetail{Event: ev, Notes: d.Notes, URL: d.URL, Organizer: d.Organizer, Attendees: d.Attendees,
		Repeat: d.Repeat.toRepeat(), HasAlarm: d.HasAlarm, AlarmSecs: d.AlarmSecs}, nil
}

// Insert creates an event and returns a reference to it (for Undo).
func (c *Cal) Insert(calID string, d Draft) (EventRef, error) {
	if !c.Connected() {
		return EventRef{}, errNoAccess
	}
	d.CalendarID = calID
	j := C.CString(draftJSON(d, true))
	defer C.free(unsafe.Pointer(j))
	c.mu.Lock()
	raw := takeString(C.ek_add_json(j))
	c.mu.Unlock()
	var r struct {
		ID    string  `json:"id"`
		Start float64 `json:"start"`
		Error string  `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return EventRef{}, err
	}
	if r.Error != "" {
		return EventRef{}, errors.New(r.Error)
	}
	return EventRef{ID: r.ID, Start: time.Unix(int64(r.Start), 0)}, nil
}

// Update edits an event; allFuture applies it to this and later occurrences of a repeating event.
func (c *Cal) Update(eventID string, origStart time.Time, d Draft, allFuture bool) error {
	if !c.Connected() {
		return errNoAccess
	}
	id, j := C.CString(eventID), C.CString(draftJSON(d, false))
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(j))
	span := 0
	if allFuture {
		span = 1
	}
	c.mu.Lock()
	msg := takeString(C.ek_update_json(id, C.double(origStart.Unix()), j, C.int(span)))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (c *Cal) Delete(eventID string, start time.Time, allFuture bool) error {
	id := C.CString(eventID)
	defer C.free(unsafe.Pointer(id))
	span := 0
	if allFuture {
		span = 1
	}
	c.mu.Lock()
	msg := takeString(C.ek_delete(id, C.double(start.Unix()), C.int(span)))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

// ---- reminders ----

func (c *Cal) RemindersAccess() string {
	switch C.ek_rem_status() {
	case 0:
		return "notDetermined"
	case 3:
		return "full"
	case 4:
		return "writeOnly"
	default:
		return "denied"
	}
}

func (c *Cal) RequestReminders() bool { return C.ek_rem_request() == 1 }

func (c *Cal) Reminders() ([]Todo, error) {
	if c.RemindersAccess() != "full" {
		return []Todo{}, nil
	}
	c.mu.Lock()
	raw := takeString(C.ek_reminders())
	c.mu.Unlock()
	var out []rawTodo
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	todos := make([]Todo, 0, len(out))
	for _, r := range out {
		todos = append(todos, r.toTodo())
	}
	return todos, nil
}

func (c *Cal) ReminderLists() []TodoList {
	if c.RemindersAccess() != "full" {
		return []TodoList{}
	}
	c.mu.Lock()
	raw := takeString(C.ek_rem_lists())
	c.mu.Unlock()
	var out []TodoList
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []TodoList{}
	}
	return out
}

// SaveReminder adds (empty ID) or updates a to-do. Only the fields set in m are changed.
func (c *Cal) SaveReminder(m map[string]any) (Todo, error) {
	b, _ := json.Marshal(m)
	j := C.CString(string(b))
	defer C.free(unsafe.Pointer(j))
	c.mu.Lock()
	raw := takeString(C.ek_rem_save(j))
	c.mu.Unlock()
	var r struct {
		rawTodo
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Todo{}, err
	}
	if r.Error != "" {
		return Todo{}, errors.New(r.Error)
	}
	return r.rawTodo.toTodo(), nil
}

func (c *Cal) DeleteReminder(id string) error {
	j := C.CString(id)
	defer C.free(unsafe.Pointer(j))
	c.mu.Lock()
	msg := takeString(C.ek_rem_delete(j))
	c.mu.Unlock()
	if msg != "" {
		return errors.New(msg)
	}
	return nil
}

// ---- clipboard + travel time ----

// readClipboard returns the clipboard's text and/or image (base64 JPEG, max 1600px).
func readClipboard() (text, imageB64 string) {
	raw := takeString(C.clip_read())
	var r struct{ Text, Image string }
	_ = json.Unmarshal([]byte(raw), &r)
	return r.Text, r.Image
}

// driveTime returns the driving time from `from` to the event's place (Apple Maps), or -1.
func (c *Cal) driveTime(from, eventID string, start time.Time) time.Duration {
	id := C.CString(eventID)
	c.mu.Lock() // the store lookup is quick and must be serialised…
	raw := takeString(C.ek_event_place(id, C.double(start.Unix())))
	c.mu.Unlock()
	C.free(unsafe.Pointer(id))
	var p struct {
		Lat, Lon float64
		Loc      string
	}
	if json.Unmarshal([]byte(raw), &p) != nil || (p.Lat == 0 && p.Lon == 0 && p.Loc == "") {
		return -1
	}
	f, loc := C.CString(from), C.CString(p.Loc)
	defer C.free(unsafe.Pointer(f))
	defer C.free(unsafe.Pointer(loc))
	secs := float64(C.ek_eta(f, C.double(p.Lat), C.double(p.Lon), loc)) // …the network part runs outside the lock
	if secs < 0 {
		return -1
	}
	return time.Duration(secs * float64(time.Second))
}

func (c *Cal) RefreshSources() {
	c.mu.Lock()
	C.ek_refresh()
	c.mu.Unlock()
}

// trayShowImage hides or shows the app icon in the menu bar item (the text stays).
func trayShowImage(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.tray_show_image(C.int(v))
}

func showAbout(version, credits string) {
	v, c := C.CString(version), C.CString(credits)
	C.show_about(v, c)
	C.free(unsafe.Pointer(v))
	C.free(unsafe.Pointer(c))
}

func setDockVisible(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.set_dock_visible(C.int(v))
}

func appActivate() { C.app_activate() }

// SysPrefs is the Mac's own look and locale settings.
type SysPrefs struct {
	Accent       string `json:"accent"`
	H24          bool   `json:"h24"`
	FirstWeekday int    `json:"firstWeekday"`
	ReduceMotion bool   `json:"reduceMotion"`
	Contrast     bool   `json:"contrast"`
	Lang         string `json:"lang"`
}

func sysPrefs() SysPrefs {
	var p SysPrefs
	_ = json.Unmarshal([]byte(takeString(C.sys_prefs())), &p)
	return p
}

// traySetImage replaces the menu bar icon (template = monochrome that follows the menu bar's colour).
func traySetImage(png []byte, template bool) {
	if len(png) == 0 {
		return
	}
	t := 0
	if template {
		t = 1
	}
	C.tray_set_image(unsafe.Pointer(&png[0]), C.int(len(png)), C.int(t))
}
