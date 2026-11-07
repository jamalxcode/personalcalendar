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
	ReminderMinutes int       `json:"reminderMinutes"`
}

// AskResult is what one line typed in the AI box turns into.
type AskResult struct {
	Kind    string  `json:"kind"`    // add | answer | update | delete
	Answer  string  `json:"answer"`  // for answer (and a one-line summary for the others)
	Drafts  []Draft `json:"drafts"`  // add
	Target  *Event  `json:"target"`  // update / delete
	Draft   *Draft  `json:"draft"`   // update: the event after the change
	Refs    []Event `json:"refs"`    // answer: events the answer is about
	Offline bool    `json:"offline"` // parsed without AI
}

type aiEvent struct {
	Title           string `json:"title"`
	Start           string `json:"start"`
	End             string `json:"end"`
	AllDay          bool   `json:"all_day"`
	Location        string `json:"location"`
	Notes           string `json:"notes"`
	Calendar        string `json:"calendar"`
	ReminderMinutes int    `json:"reminder_minutes"`
}

type aiReply struct {
	Kind    string    `json:"kind"`
	Answer  string    `json:"answer"`
	Events  []aiEvent `json:"events"`
	Target  int       `json:"target"`
	Refs    []int     `json:"refs"`
	Changes aiEvent   `json:"changes"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var eventProps = map[string]any{
	"title":            str("Short, clean event title (no date/time words). For changes: new title or empty to keep"),
	"start":            str("RFC3339 with offset, e.g. 2026-11-08T13:00:00+03:00. All-day: YYYY-MM-DD. For changes: empty to keep"),
	"end":              str("RFC3339 with offset. All-day: YYYY-MM-DD of the day AFTER the last day. For changes: empty to keep"),
	"all_day":          map[string]any{"type": "boolean"},
	"location":         str("Place, or empty string"),
	"notes":            str("Extra details, or empty string"),
	"calendar":         str("Exact name of one of the user's calendars if named or clearly implied, else empty"),
	"reminder_minutes": map[string]any{"type": "integer", "description": "Minutes before start for a reminder if asked, else 0"},
}
var eventReq = []string{"title", "start", "end", "all_day", "location", "notes", "calendar", "reminder_minutes"}

func eventSchema() map[string]any {
	return map[string]any{"type": "object", "properties": eventProps, "required": eventReq, "additionalProperties": false}
}

var askSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"add", "answer", "update", "delete"},
			"description": "add = create new events; answer = a question about the schedule; update = change one existing event; delete = remove one existing event"},
		"answer":  str("For answer: a short, friendly, direct answer (times in the user's format). For add/update/delete: a one-line summary of what will happen"),
		"events":  map[string]any{"type": "array", "items": eventSchema(), "description": "For add: the new events. Otherwise empty"},
		"target":  map[string]any{"type": "integer", "description": "For update/delete: the [n] number of the existing event. Otherwise -1"},
		"refs":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "For answer: [n] numbers of the events the answer mentions (max 8). Otherwise empty"},
		"changes": eventSchema(),
	},
	"required":             []string{"kind", "answer", "events", "target", "refs", "changes"},
	"additionalProperties": false,
}

// askAI routes one line of text: add events, answer a question, or change/delete an existing event.
// known is the list of nearby events the model may refer to by index.
func askAI(ctx context.Context, cfg *Config, text string, cals []CalendarInfo, known []Event) (AskResult, error) {
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
		if e.Holiday {
			list.WriteString(" | holiday")
		}
		list.WriteString("\n")
	}
	sys := fmt.Sprintf(`You are the assistant inside a Fantastical-style calendar app.
Now is %s (%s), timezone %s (UTC%s). Interpret relative dates ("tomorrow", "next Tue", "this weekend") from now.

Decide what the user wants:
- "add": they describe new event(s) ("lunch with Ali tomorrow 1pm"). Default duration 1 hour; a time with no date = next occurrence; a weekday = the next upcoming one; no time at all = all-day; "at/@ <place>" = location; several events → several items.
- "answer": a question about their schedule ("what's on Thursday?", "am I free tomorrow afternoon?", "when is my next dentist?"). Answer briefly and concretely from the event list below; list items with times; say plainly when something isn't there. Put the [n] of mentioned events in refs.
- "update": change ONE existing event ("move dentist to 5pm", "rename lunch to team lunch", "make Friday's meeting 2 hours"). Set target to its [n]; in changes fill only what changes (others empty strings, all_day false unless switching to all-day, reminder 0). When moving, keep the original duration unless told otherwise and give both start and end.
- "delete": remove ONE existing event ("cancel lunch on Friday"). Set target.
If the request is ambiguous between several events, use "answer" to ask which one.

The user's weekend is %s; their week starts on %s ("weekend" = those days). Always use the offset %s in times. Writable calendars: %s.
%s

The user's events (past week to next two months):
%s`,
		now.Format("Monday 2 January 2006 15:04"), now.Format("2006-01-02T15:04:05Z07:00"),
		now.Location().String(), now.Format("-07:00"), cfg.WeekendText(), weekdayNames[cfg.WeekStart], now.Format("-07:00"),
		strings.Join(names, ", "), voiceLine(cfg), list.String())

	content, err := callAI(ctx, cfg, sys, text, "calendar_action", askSchema)
	if err != nil {
		return AskResult{}, err
	}
	var r aiReply
	if err := json.Unmarshal(content, &r); err != nil {
		return AskResult{}, fmt.Errorf("couldn't read the AI reply: %v", err)
	}
	res := AskResult{Kind: r.Kind, Answer: r.Answer}
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
		if r.Kind == "update" {
			d, err := applyChanges(*t, r.Changes, cals)
			if err != nil {
				return res, err
			}
			res.Draft = &d
		}
	default:
		return res, fmt.Errorf("the AI gave an unexpected answer — try rephrasing")
	}
	return res, nil
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
		d := Draft{Title: e.Title, AllDay: e.AllDay, Location: e.Location, Notes: e.Notes, ReminderMinutes: e.ReminderMinutes}
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
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": user},
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
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
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

func parseDay(s string) (time.Time, error) {
	if len(s) >= 10 {
		s = s[:10]
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}
