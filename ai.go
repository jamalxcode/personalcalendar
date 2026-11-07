package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Draft is an event the AI parsed from free text, waiting to be added (or an edited version of one).
type Draft struct {
	Title           string    `json:"title"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	AllDay          bool      `json:"allDay"`
	Location        string    `json:"location"`
	Notes           string    `json:"notes"`
	CalendarID      string    `json:"calendarId"`
	ReminderMinutes int       `json:"reminderMinutes"` // explicit "remind me N min before"
	Alarm           string    `json:"alarm"`           // "" default/keep | none | minutes | dayBefore9 | sameDay9
	Repeat          Repeat    `json:"repeat"`
	SetRepeat       bool      `json:"setRepeat"` // update: apply Repeat (otherwise recurrence is left alone)
	TimeZone        string    `json:"timeZone"`  // IANA, "" = local
	RepeatText      string    `json:"repeatText"`
}

// TodoDraft is a to-do the AI wants to create.
type TodoDraft struct {
	Title   string     `json:"title"`
	Notes   string     `json:"notes"`
	Due     *time.Time `json:"due"`
	HasTime bool       `json:"hasTime"`
}

// AskResult is what one input typed (or pasted) in the AI box turns into.
type AskResult struct {
	Kind      string      `json:"kind"`   // add | answer | update | delete | slots | todo
	Answer    string      `json:"answer"` // for answer (and a one-line summary for the others)
	Drafts    []Draft     `json:"drafts"` // add
	Clashes   [][]Event   `json:"clashes"`
	Target    *Event      `json:"target"` // update / delete
	Draft     *Draft      `json:"draft"`  // update: the event after the change
	AllFuture bool        `json:"allFuture"`
	Refs      []Event     `json:"refs"`  // answer: events the answer is about
	Slots     []Draft     `json:"slots"` // slots: free times that can be booked
	Todos     []TodoDraft `json:"todos"`
	Offline   bool        `json:"offline"` // parsed without AI
	slotReq   *aiSlot
}

type aiRepeat struct {
	Freq     string `json:"freq"`
	Interval int    `json:"interval"`
	Weekdays []int  `json:"weekdays"`
	Until    string `json:"until"`
	Count    int    `json:"count"`
}

type aiEvent struct {
	Title           string   `json:"title"`
	Start           string   `json:"start"`
	End             string   `json:"end"`
	AllDay          bool     `json:"all_day"`
	Location        string   `json:"location"`
	Notes           string   `json:"notes"`
	Calendar        string   `json:"calendar"`
	ReminderMinutes int      `json:"reminder_minutes"`
	Repeat          aiRepeat `json:"repeat"`
	TimeZone        string   `json:"time_zone"`
}

type aiSlot struct {
	Title          string `json:"title"`
	Minutes        int    `json:"duration_minutes"`
	From           string `json:"from_date"`
	To             string `json:"to_date"`
	Earliest       int    `json:"earliest_hour"`
	Latest         int    `json:"latest_hour"`
	IncludeWeekend bool   `json:"include_weekend"`
}

type aiTodo struct {
	Title   string `json:"title"`
	Due     string `json:"due"`
	HasTime bool   `json:"has_time"`
	Notes   string `json:"notes"`
}

type aiReply struct {
	Kind    string    `json:"kind"`
	Answer  string    `json:"answer"`
	Events  []aiEvent `json:"events"`
	Target  int       `json:"target"`
	Refs    []int     `json:"refs"`
	Changes aiEvent   `json:"changes"`
	Scope   string    `json:"scope"`
	Slot    aiSlot    `json:"slot"`
	Todos   []aiTodo  `json:"todos"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func obj(props map[string]any) map[string]any {
	req := make([]string, 0, len(props))
	for k := range props {
		req = append(req, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

var repeatSchema = obj(map[string]any{
	"freq": map[string]any{"type": "string", "enum": []string{"none", "daily", "weekly", "monthly", "yearly", "keep"},
		"description": "none = one-off. For changes: keep = leave the repetition as it is"},
	"interval": integer("Every N days/weeks/… (1 = every)"),
	"weekdays": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Weekly only: 0=Sun … 6=Sat; empty = same weekday as the start"},
	"until":    str("YYYY-MM-DD last day, or empty"),
	"count":    integer("Number of occurrences, or 0"),
})

func eventSchema() map[string]any {
	return obj(map[string]any{
		"title":            str("Short, clean event title (no date/time words). For changes: new title or empty to keep"),
		"start":            str("RFC3339 with the user's local offset, e.g. 2026-11-08T13:00:00+03:00. All-day: YYYY-MM-DD. For changes: empty to keep"),
		"end":              str("RFC3339 local. All-day: YYYY-MM-DD of the day AFTER the last day. For changes: empty to keep"),
		"all_day":          map[string]any{"type": "boolean"},
		"location":         str("Place, or empty string"),
		"notes":            str("Extra details (booking refs, links), or empty string"),
		"calendar":         str("Exact name of one of the user's calendars if named or clearly implied, else empty"),
		"reminder_minutes": integer("Minutes before start for a reminder if asked, else 0"),
		"repeat":           repeatSchema,
		"time_zone":        str("IANA zone if the user named another city/zone for the time (e.g. Asia/Dubai for '3pm Dubai time'), else empty"),
	})
}

var askSchema = obj(map[string]any{
	"kind": map[string]any{"type": "string", "enum": []string{"add", "answer", "update", "delete", "slots", "todo"},
		"description": "add = create events; answer = question about the schedule; update/delete = change/remove one existing event; slots = find free time; todo = a task/reminder without a set meeting time"},
	"answer":  str("For answer: short, direct reply. Otherwise: a one-line summary of what will happen"),
	"events":  map[string]any{"type": "array", "items": eventSchema(), "description": "add: the new events. Otherwise empty"},
	"target":  integer("update/delete: the [n] number of the existing event, else -1"),
	"refs":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "answer: [n] numbers of events mentioned (max 8)"},
	"changes": eventSchema(),
	"scope": map[string]any{"type": "string", "enum": []string{"this", "future"},
		"description": "update/delete of a repeating event: 'future' if the user means all/every/from now on, else 'this'"},
	"slot": obj(map[string]any{
		"title":            str("What the time is for, e.g. Gym"),
		"duration_minutes": integer("Length needed"),
		"from_date":        str("YYYY-MM-DD first day to search"),
		"to_date":          str("YYYY-MM-DD last day to search"),
		"earliest_hour":    integer("Earliest start hour 0-23 (default 8)"),
		"latest_hour":      integer("Latest end hour 1-24 (default 20)"),
		"include_weekend":  map[string]any{"type": "boolean"},
	}),
	"todos": map[string]any{"type": "array", "items": obj(map[string]any{
		"title":    str("The task, short"),
		"due":      str("YYYY-MM-DD, or RFC3339 local if a time was given, or empty"),
		"has_time": map[string]any{"type": "boolean"},
		"notes":    str("Details or empty"),
	}), "description": "todo: the tasks. Otherwise empty"},
})

// askAI routes one input (typed, pasted text, or a screenshot): add events, answer a question,
// change/delete an existing event, find free time, or create to-dos.
// known is the list of nearby events the model may refer to by index.
func askAI(ctx context.Context, cfg *Config, text, imageB64 string, cals []CalendarInfo, known []Event) (AskResult, error) {
	if strings.TrimSpace(cfg.OpenAIKey) == "" {
		return AskResult{}, errNoKey
	}
	now := time.Now()
	var names []string
	for _, c := range cals {
		if c.Writable {
			names = append(names, c.Name)
		}
	}
	timeFmt := "3:04 PM"
	if cfg.Use24h {
		timeFmt = "15:04"
	}
	var list strings.Builder
	for i, e := range known {
		when := e.Start.Format("Mon 2 Jan")
		if e.AllDay {
			when += " (all day)"
		} else {
			when += " " + e.Start.Format(timeFmt) + "–" + e.End.Format(timeFmt)
		}
		fmt.Fprintf(&list, "[%d] %s | %s | %s", i, when, e.Title, e.Calendar)
		if e.Location != "" {
			fmt.Fprintf(&list, " | at %s", e.Location)
		}
		if e.Recurring {
			list.WriteString(" | repeats")
		}
		if e.Holiday {
			list.WriteString(" | holiday")
		}
		list.WriteString("\n")
	}
	sys := fmt.Sprintf(`You are the assistant inside a Fantastical-style calendar app.
Now is %s (%s), timezone %s (UTC%s). Interpret relative dates ("tomorrow", "next Tue", "this weekend") from now.

Decide what the user wants:
- "add": new event(s). Default duration 1 hour; a time with no date = next occurrence; a weekday = the next upcoming one; no time = all-day; "at/@ <place>" = location; several events → several items.
  Repeats: "every Monday", "daily", "every 2 weeks", "on the 1st of every month", "every weekday until June", "for 10 weeks" → fill repeat (weekly + weekdays for specific days). Otherwise repeat.freq = "none".
  Time zones: "3pm Dubai time" → set time_zone (e.g. Asia/Dubai) and give start/end converted to the user's local offset.
- If the input is PASTED content (an email, chat message, ticket, invitation, school letter) or a SCREENSHOT/photo: extract every event in it with exact dates, times and places. Flights: one event per flight, title like "✈ KU 101 KWI→DXB", start/end at departure/arrival converted to local offset, time_zone = departure airport's zone, location = departure airport, booking reference and seat in notes. Ignore marketing and boilerplate. If it contains tasks rather than events, use "todo".
- "answer": a question about their schedule. Answer briefly from the event list below; list items with times; say plainly when something isn't there. Put the [n] of mentioned events in refs.
- "update": change ONE existing event ("move dentist to 5pm", "make gym 7am every week"). target = its [n]; in changes fill only what changes (empty strings / repeat.freq "keep" for the rest). Keep the duration when moving unless told otherwise; give start and end. scope = "future" for "every/all/from now on" on repeating events.
- "delete": remove ONE existing event; scope as above.
- "slots": "find an hour for the gym this week", "when can I fit a 2h meeting tomorrow?" → fill slot (dates, duration, hours; include_weekend only if asked or the request is about the weekend). Do NOT invent the free times yourself — the app computes them.
- "todo": tasks/reminders without a meeting: "remind me to call the bank tomorrow", "buy milk", "renew passport by Friday". Fill todos (due date if given; has_time only if a time was said).
If ambiguous between several events, use "answer" to ask which one.

The user's weekend is %s; their week starts on %s. Always use the offset %s in times. Writable calendars: %s.
%s

The user's events (past week to next two months):
%s`,
		now.Format("Monday 2 January 2006 15:04"), now.Format("2006-01-02T15:04:05Z07:00"),
		now.Location().String(), now.Format("-07:00"), cfg.WeekendText(), weekdayNames[cfg.WeekStart], now.Format("-07:00"),
		strings.Join(names, ", "), voiceLine(cfg), list.String())

	user := text
	if strings.TrimSpace(user) == "" && imageB64 != "" {
		user = "(screenshot attached — extract the events or tasks in it)"
	} else if len(user) > 280 {
		user = "PASTED CONTENT:\n" + user
	}
	content, err := callAIWithImage(ctx, cfg, sys, user, imageB64, "calendar_action", askSchema)
	if err != nil {
		return AskResult{}, err
	}
	var r aiReply
	if err := json.Unmarshal(content, &r); err != nil {
		return AskResult{}, fmt.Errorf("couldn't read the AI reply: %v", err)
	}
	res := AskResult{Kind: r.Kind, Answer: r.Answer, AllFuture: r.Scope == "future"}
	pick := func(i int) *Event {
		if i >= 0 && i < len(known) {
			e := known[i]
			return &e
		}
		return nil
	}
	switch r.Kind {
	case "add":
		ds, err := aiToDrafts(r.Events, cals)
		if err != nil {
			return res, err
		}
		if len(ds) == 0 {
			return res, fmt.Errorf("couldn't find an event in that — try e.g. “Lunch with Ali tomorrow 1pm”")
		}
		res.Drafts = ds
	case "answer":
		for _, i := range r.Refs {
			if e := pick(i); e != nil {
				res.Refs = append(res.Refs, *e)
			}
		}
	case "update", "delete":
		t := pick(r.Target)
		if t == nil {
			return AskResult{Kind: "answer", Answer: "I couldn't tell which event you mean — try including its name or day."}, nil
		}
		res.Target = t
		if !t.Recurring {
			res.AllFuture = false
		}
		if r.Kind == "update" {
			d, err := applyChanges(*t, r.Changes, cals)
			if err != nil {
				return res, err
			}
			res.Draft = &d
		}
	case "slots":
		sl := r.Slot
		res.slotReq = &sl
	case "todo":
		for _, t := range r.Todos {
			if strings.TrimSpace(t.Title) == "" {
				continue
			}
			td := TodoDraft{Title: strings.TrimSpace(t.Title), Notes: t.Notes, HasTime: t.HasTime}
			if t.Due != "" {
				if tt, err := time.Parse(time.RFC3339, t.Due); err == nil {
					l := tt.Local()
					td.Due = &l
				} else if dd, err := parseDay(t.Due); err == nil {
					td.Due, td.HasTime = &dd, false
				}
			}
			res.Todos = append(res.Todos, td)
		}
		if len(res.Todos) == 0 {
			return res, fmt.Errorf("couldn't find a task in that")
		}
	default:
		return res, fmt.Errorf("the AI gave an unexpected answer — try rephrasing")
	}
	return res, nil
}

func (r aiRepeat) toRepeat() Repeat {
	out := Repeat{Freq: r.Freq, Interval: r.Interval, Weekdays: r.Weekdays, Until: r.Until, Count: r.Count}
	if out.Weekdays == nil {
		out.Weekdays = []int{}
	}
	if out.Freq == "" {
		out.Freq = "none"
	}
	return out
}

// applyChanges merges the AI's partial changes into an existing event.
func applyChanges(t Event, c aiEvent, cals []CalendarInfo) (Draft, error) {
	d := Draft{Title: t.Title, Start: t.Start, End: t.End, AllDay: t.AllDay, Location: t.Location, CalendarID: t.CalendarID}
	if strings.TrimSpace(c.Title) != "" {
		d.Title = strings.TrimSpace(c.Title)
	}
	if c.Location != "" {
		d.Location = c.Location
	}
	if c.Repeat.Freq != "" && c.Repeat.Freq != "keep" {
		d.Repeat, d.SetRepeat = c.Repeat.toRepeat(), true
		d.RepeatText = d.Repeat.Text()
	}
	if c.TimeZone != "" {
		d.TimeZone = c.TimeZone
	}
	if c.Notes != "" {
		d.Notes = c.Notes
	}
	for _, cal := range cals {
		if c.Calendar != "" && strings.EqualFold(cal.Name, c.Calendar) && cal.Writable {
			d.CalendarID = cal.ID
		}
	}
	dur := t.End.Sub(t.Start)
	if c.AllDay && !t.AllDay {
		d.AllDay = true
	}
	if c.Start != "" {
		if d.AllDay {
			s, err := parseDay(c.Start)
			if err != nil {
				return d, fmt.Errorf("bad date %q", c.Start)
			}
			d.Start, d.End = s, s.AddDate(0, 0, 1)
		} else {
			s, err := time.Parse(time.RFC3339, c.Start)
			if err != nil {
				return d, fmt.Errorf("bad time %q", c.Start)
			}
			d.Start = s.Local()
			d.End = d.Start.Add(dur)
		}
	}
	if c.End != "" {
		if d.AllDay {
			if e, err := parseDay(c.End); err == nil && e.After(d.Start) {
				d.End = e
			}
		} else if e, err := time.Parse(time.RFC3339, c.End); err == nil && e.After(d.Start) {
			d.End = e.Local()
		}
	}
	return d, nil
}

func aiToDrafts(evs []aiEvent, cals []CalendarInfo) ([]Draft, error) {
	var out []Draft
	for _, e := range evs {
		d := Draft{Title: e.Title, AllDay: e.AllDay, Location: e.Location, Notes: e.Notes, ReminderMinutes: e.ReminderMinutes,
			Repeat: e.Repeat.toRepeat(), TimeZone: e.TimeZone}
		d.RepeatText = d.Repeat.Text()
		for _, c := range cals {
			if e.Calendar != "" && strings.EqualFold(c.Name, e.Calendar) && c.Writable {
				d.CalendarID = c.ID
			}
		}
		if d.AllDay {
			s, err := parseDay(e.Start)
			if err != nil {
				return nil, fmt.Errorf("bad date %q", e.Start)
			}
			en, err := parseDay(e.End)
			if err != nil || !en.After(s) {
				en = s.AddDate(0, 0, 1)
			}
			d.Start, d.End = s, en
		} else {
			s, err := time.Parse(time.RFC3339, e.Start)
			if err != nil {
				return nil, fmt.Errorf("bad time %q", e.Start)
			}
			en, err := time.Parse(time.RFC3339, e.End)
			if err != nil || !en.After(s) {
				en = s.Add(time.Hour)
			}
			d.Start, d.End = s.Local(), en.Local()
		}
		out = append(out, d)
	}
	return out, nil
}

// voiceLine sets the tone of answers (Settings → AI personality).
func voiceLine(cfg *Config) string {
	name := strings.TrimSpace(cfg.Name)
	who := ""
	if name != "" {
		who = " The user's name is " + name + "."
	}
	switch cfg.Voice {
	case "formal":
		return "Tone for answers: concise, polite and professional, no emoji." + who
	case "fun":
		return "Tone for answers: playful and witty, a light joke or emoji is welcome, but always accurate and short." + who
	default:
		return "Tone for answers: warm and friendly, plain words, at most one emoji." + who
	}
}

var errNoKey = fmt.Errorf("add your OpenRouter key in Settings to use AI")

// callAI sends one structured-output chat request to OpenRouter (sk-or-… keys) or OpenAI.
func callAI(ctx context.Context, cfg *Config, sys, user, schemaName string, schema map[string]any) ([]byte, error) {
	return callAIWithImage(ctx, cfg, sys, user, "", schemaName, schema)
}

// callAIWithImage is callAI with an optional base64 JPEG (screenshots/photos; needs a vision model).
func callAIWithImage(ctx context.Context, cfg *Config, sys, user, imageB64, schemaName string, schema map[string]any) ([]byte, error) {
	key := strings.TrimSpace(cfg.OpenAIKey)
	base, model, provider := "https://api.openai.com/v1", cfg.OpenAIModel, "OpenAI"
	if strings.HasPrefix(key, "sk-or-") {
		base, provider = "https://openrouter.ai/api/v1", "OpenRouter"
		if !strings.Contains(model, "/") {
			model = "openai/" + model
		}
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": userContent(user, imageB64)},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schemaName, "strict": true, "schema": schema},
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "Smart Cal")
	req.Header.Set("HTTP-Referer", "https://go.sala.company")
	resp, err := (&http.Client{Timeout: 70 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s unreachable: %w", provider, err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(rb, &e)
		if e.Error.Message == "" {
			e.Error.Message = string(rb)
		}
		return nil, fmt.Errorf("%s (%d): %s", provider, resp.StatusCode, e.Error.Message)
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &cr); err != nil || len(cr.Choices) == 0 {
		return nil, fmt.Errorf("unexpected %s response", provider)
	}
	if r := cr.Choices[0].Message.Refusal; r != "" {
		return nil, fmt.Errorf("%s", r)
	}
	c := strings.TrimSpace(cr.Choices[0].Message.Content)
	c = strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(c, "```json"), "```"), "```")
	return []byte(c), nil
}

func userContent(text, imageB64 string) any {
	if imageB64 == "" {
		return text
	}
	return []map[string]any{
		{"type": "text", "text": text},
		{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64," + imageB64}},
	}
}

func parseDay(s string) (time.Time, error) {
	if len(s) >= 10 {
		s = s[:10]
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}
