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

// Draft is an event the AI parsed from free text, waiting to be added.
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

var parseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"events": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":            map[string]any{"type": "string", "description": "Short, clean event title (no date/time words)"},
					"start":            map[string]any{"type": "string", "description": "RFC3339 with offset, e.g. 2026-11-08T13:00:00+03:00. For all-day: YYYY-MM-DD"},
					"end":              map[string]any{"type": "string", "description": "RFC3339 with offset. For all-day: YYYY-MM-DD of the day AFTER the last day (exclusive)"},
					"all_day":          map[string]any{"type": "boolean"},
					"location":         map[string]any{"type": "string", "description": "Place, or empty string"},
					"notes":            map[string]any{"type": "string", "description": "Extra details, or empty string"},
					"calendar":         map[string]any{"type": "string", "description": "Exact name of one of the user's calendars if they named or clearly implied one, else empty string"},
					"reminder_minutes": map[string]any{"type": "integer", "description": "Minutes before start for a reminder if the user asked for one, else 0"},
				},
				"required":             []string{"title", "start", "end", "all_day", "location", "notes", "calendar", "reminder_minutes"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"events"},
	"additionalProperties": false,
}

func parseEvents(ctx context.Context, cfg *Config, text string, cals []CalendarInfo) ([]Draft, error) {
	if strings.TrimSpace(cfg.OpenAIKey) == "" {
		return nil, fmt.Errorf("add your OpenAI API key in Settings first")
	}
	now := time.Now()
	var names []string
	for _, c := range cals {
		if c.Writable {
			names = append(names, c.Name)
		}
	}
	sys := fmt.Sprintf(`You turn a short natural-language note into calendar events, like Fantastical.
Now is %s (%s), timezone %s (UTC%s). Interpret relative dates ("tomorrow", "next Tue", "this weekend") from now.
Rules:
- Default duration 1 hour if no end/duration given. "for 30m"/"2 hours" sets duration.
- A time with no date means the next occurrence of that time (today if still ahead, else tomorrow).
- A weekday with no "next" means the next upcoming one (not today unless the time is still ahead today).
- No time at all (e.g. "Ali's birthday Friday") → all-day event.
- "at <place>" or "@ <place>" → location. Keep the title short and natural (e.g. "Lunch with Ali").
- Kuwait weekend is Friday–Saturday; "weekend" means Fri–Sat.
- If several separate events are described, return each.
- User's calendars: %s.
Always use the offset %s in times.`,
		now.Format("Monday 2 January 2006 15:04"), now.Format("2006-01-02T15:04:05Z07:00"),
		now.Location().String(), now.Format("-07:00"), strings.Join(names, ", "), now.Format("-07:00"))

	body := map[string]any{
		"model": cfg.OpenAIModel,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": text},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "calendar_events", "strict": true, "schema": parseSchema},
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.OpenAIKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("OpenAI (%d): %s", resp.StatusCode, e.Error.Message)
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
		return nil, fmt.Errorf("unexpected OpenAI response")
	}
	if r := cr.Choices[0].Message.Refusal; r != "" {
		return nil, fmt.Errorf("%s", r)
	}
	var parsed struct {
		Events []aiEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(cr.Choices[0].Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("couldn't read the parsed event: %v", err)
	}
	if len(parsed.Events) == 0 {
		return nil, fmt.Errorf("couldn't find an event in that — try e.g. “Lunch with Ali tomorrow 1pm”")
	}
	var out []Draft
	for _, e := range parsed.Events {
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

func parseDay(s string) (time.Time, error) {
	if len(s) >= 10 {
		s = s[:10]
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}
