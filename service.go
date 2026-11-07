package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CalService is bound to the frontend: JS calls these via
// Call.ByName("github.com/jamalxcode/personalcalendar.CalService.<Method>", ...).
type CalService struct{ a *App }

// State is everything the popover needs to render.
type State struct {
	Connected        bool           `json:"connected"`
	HasKey           bool           `json:"hasKey"`
	HasCreds         bool           `json:"hasCreds"`
	Use24h           bool           `json:"use24h"`
	ConfirmBeforeAdd bool           `json:"confirmBeforeAdd"`
	DaysAhead        int            `json:"daysAhead"`
	Events           []Event        `json:"events"`
	Calendars        []CalendarInfo `json:"calendars"`
	DefaultCalendar  string         `json:"defaultCalendar"`
	Error            string         `json:"error"`
	Hotkey           string         `json:"hotkey"`
	Now              time.Time      `json:"now"`
}

func (a *App) state() State {
	evs, err := a.snapshot()
	_, credErr := os.Stat(credentialsPath())
	st := State{
		Connected:        a.cal.Connected(),
		HasKey:           strings.TrimSpace(a.cfg.OpenAIKey) != "",
		HasCreds:         credErr == nil,
		Use24h:           a.cfg.Use24h,
		ConfirmBeforeAdd: a.cfg.ConfirmBeforeAdd,
		DaysAhead:        a.cfg.DaysAhead,
		Events:           evs,
		Calendars:        a.cal.CachedCalendars(),
		DefaultCalendar:  a.cfg.DefaultCalendar,
		Hotkey:           hotkey,
		Now:              time.Now(),
	}
	if st.Events == nil {
		st.Events = []Event{}
	}
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

func (s *CalService) GetState() State { return s.a.state() }

func (s *CalService) Refresh() State { return s.a.doRefresh() }

// Parse turns free text into draft events (not yet saved).
func (s *CalService) Parse(text string) ([]Draft, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cals := s.a.cal.CachedCalendars()
	drafts, err := parseEvents(ctx, s.a.cfg, text, cals)
	if err != nil {
		return nil, err
	}
	for i := range drafts {
		if drafts[i].CalendarID == "" {
			drafts[i].CalendarID = s.a.cfg.DefaultCalendar
		}
		if drafts[i].CalendarID == "" {
			for _, c := range cals {
				if c.Primary {
					drafts[i].CalendarID = c.ID
				}
			}
		}
	}
	return drafts, nil
}

// Add saves drafts to Google Calendar and returns fresh state.
func (s *CalService) Add(drafts []Draft) (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, d := range drafts {
		if strings.TrimSpace(d.Title) == "" {
			return s.a.state(), fmt.Errorf("event needs a title")
		}
		if _, err := s.a.cal.Insert(ctx, d.CalendarID, d); err != nil {
			return s.a.state(), err
		}
	}
	return s.a.doRefresh(), nil
}

func (s *CalService) Delete(calendarID, eventID string) (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.a.cal.Delete(ctx, calendarID, eventID); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// ---- settings ----

type Settings struct {
	OpenAIKeyHint    string   `json:"openaiKeyHint"` // "sk-…abcd" for display
	OpenAIKey        string   `json:"openaiKey"`     // write-only: empty = keep current
	OpenAIModel      string   `json:"openaiModel"`
	Use24h           bool     `json:"use24h"`
	ConfirmBeforeAdd bool     `json:"confirmBeforeAdd"`
	DaysAhead        int      `json:"daysAhead"`
	HiddenCalendars  []string `json:"hiddenCalendars"`
	DefaultCalendar  string   `json:"defaultCalendar"`
}

func (s *CalService) GetSettings() Settings {
	c := s.a.cfg
	hint := ""
	if k := strings.TrimSpace(c.OpenAIKey); len(k) > 8 {
		hint = k[:3] + "…" + k[len(k)-4:]
	}
	hidden := c.HiddenCalendars
	if hidden == nil {
		hidden = []string{}
	}
	return Settings{OpenAIKeyHint: hint, OpenAIModel: c.OpenAIModel, Use24h: c.Use24h,
		ConfirmBeforeAdd: c.ConfirmBeforeAdd, DaysAhead: c.DaysAhead,
		HiddenCalendars: hidden, DefaultCalendar: c.DefaultCalendar}
}

func (s *CalService) SaveSettings(in Settings) (State, error) {
	c := s.a.cfg
	if k := strings.TrimSpace(in.OpenAIKey); k != "" {
		c.OpenAIKey = k
	}
	if m := strings.TrimSpace(in.OpenAIModel); m != "" {
		c.OpenAIModel = m
	}
	c.Use24h = in.Use24h
	c.ConfirmBeforeAdd = in.ConfirmBeforeAdd
	if in.DaysAhead > 0 {
		c.DaysAhead = in.DaysAhead
	}
	c.HiddenCalendars = in.HiddenCalendars
	c.DefaultCalendar = in.DefaultCalendar
	if err := c.Save(); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// ImportCredentials opens the native file picker and copies the Google OAuth client JSON in.
func (s *CalService) ImportCredentials() (bool, error) {
	out, err := exec.Command("osascript", "-e",
		`POSIX path of (choose file with prompt "Choose the Google OAuth credentials.json" of type {"public.json"})`).Output()
	s.a.tray.ShowWindow() // the picker steals focus, which hides the popover
	if err != nil {
		return false, nil // cancelled
	}
	path := strings.TrimSpace(string(out))
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(b), "client_id") {
		return false, fmt.Errorf("that file isn't a Google OAuth client (no client_id) — download the Desktop app client JSON")
	}
	if !strings.Contains(string(b), `"installed"`) {
		return false, fmt.Errorf("that's a Web client — create an OAuth client of type “Desktop app” instead")
	}
	return true, os.WriteFile(credentialsPath(), b, 0o600)
}

func (s *CalService) ConnectGoogle() (State, error) {
	if err := s.a.cal.Login(context.Background()); err != nil {
		return s.a.state(), err
	}
	s.a.tray.ShowWindow()
	return s.a.doRefresh(), nil
}

func (s *CalService) DisconnectGoogle() State {
	s.a.cal.Disconnect()
	return s.a.doRefresh()
}

func (s *CalService) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") {
		openURL(u)
		s.a.win.Hide()
	}
}

func (s *CalService) Hide() { s.a.win.Hide() }

func (s *CalService) Quit() { s.a.wa.Quit() }
