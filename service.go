package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CalService is bound to the frontend: JS calls these via Call.ByName("main.CalService.<Method>", ...).
type CalService struct{ a *App }

// State is everything the popover needs to render.
type State struct {
	Access           string         `json:"access"` // notDetermined | denied | restricted | full | writeOnly
	Connected        bool           `json:"connected"`
	HasGoogle        bool           `json:"hasGoogle"`
	GoogleAccounts   []string       `json:"googleAccounts"`
	HasKey           bool           `json:"hasKey"`
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
	cals := a.cal.CachedCalendars()
	st := State{
		Access:           a.cal.Access(),
		HasKey:           strings.TrimSpace(a.cfg.OpenAIKey) != "",
		Use24h:           a.cfg.Use24h,
		ConfirmBeforeAdd: a.cfg.ConfirmBeforeAdd,
		DaysAhead:        a.cfg.DaysAhead,
		Events:           evs,
		Calendars:        cals,
		DefaultCalendar:  a.cfg.DefaultCalendar,
		Hotkey:           hotkey,
		Now:              time.Now(),
		GoogleAccounts:   []string{},
	}
	st.Connected = st.Access == "full"
	seen := map[string]bool{}
	for _, c := range cals {
		if c.Google && !seen[c.Account] {
			seen[c.Account] = true
			st.GoogleAccounts = append(st.GoogleAccounts, c.Account)
		}
	}
	st.HasGoogle = len(st.GoogleAccounts) > 0
	if st.Events == nil {
		st.Events = []Event{}
	}
	if st.Calendars == nil {
		st.Calendars = []CalendarInfo{}
	}
	if err != nil && st.Connected {
		st.Error = err.Error()
	}
	return st
}

func (s *CalService) GetState() State { return s.a.state() }

func (s *CalService) Refresh() State {
	s.a.cal.RefreshSources()
	return s.a.doRefresh()
}

// AllowAccess is setup step 1: shows the macOS calendar permission prompt.
// If access was refused earlier, macOS won't prompt again, so we open the Privacy settings instead.
func (s *CalService) AllowAccess() (State, error) {
	switch s.a.cal.Access() {
	case "full":
		return s.a.doRefresh(), nil
	case "notDetermined":
		ok := s.a.cal.RequestAccess()
		s.a.tray.ShowWindow() // the system prompt takes focus away from the popover
		if !ok {
			return s.a.doRefresh(), fmt.Errorf("calendar access wasn't allowed")
		}
		return s.a.doRefresh(), nil
	default:
		openURL("x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars")
		return s.a.state(), fmt.Errorf("turn on “Personal Calendar” under Calendars in the window that just opened, then come back")
	}
}

// AddGoogleAccount is setup step 2: opens System Settings → Internet Accounts.
// Once the account is added, macOS notifies us and the calendars appear on their own.
func (s *CalService) AddGoogleAccount() {
	openURL("x-apple.systempreferences:com.apple.Internet-Accounts-Settings.extension")
	s.a.win.Hide()
}

// Parse turns free text into draft events (not yet saved).
func (s *CalService) Parse(text string) ([]Draft, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cals := s.a.cal.CachedCalendars()
	drafts, err := parseEvents(ctx, s.a.cfg, text, cals)
	if err != nil {
		return nil, err
	}
	def := s.a.cfg.DefaultCalendar
	if def == "" {
		for _, c := range cals {
			if c.Primary {
				def = c.ID
			}
		}
	}
	for i := range drafts {
		if drafts[i].CalendarID == "" {
			drafts[i].CalendarID = def
		}
	}
	return drafts, nil
}

// Add saves drafts to the calendar and returns fresh state.
func (s *CalService) Add(drafts []Draft) (State, error) {
	for _, d := range drafts {
		if strings.TrimSpace(d.Title) == "" {
			return s.a.state(), fmt.Errorf("event needs a title")
		}
		if err := s.a.cal.Insert(d.CalendarID, d); err != nil {
			return s.a.state(), err
		}
	}
	return s.a.doRefresh(), nil
}

func (s *CalService) Delete(eventID string, start time.Time) (State, error) {
	if err := s.a.cal.Delete(eventID, start); err != nil {
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
		hint = k[:5] + "…" + k[len(k)-4:]
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

func (s *CalService) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "ical://") {
		openURL(u)
		s.a.win.Hide()
	}
}

// ---- updates ----

func (s *CalService) Version() string { return version }

// LatestUpdate returns the last check result without hitting the network.
func (s *CalService) LatestUpdate() UpdateInfo {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	u := s.a.update
	u.Current = version
	return u
}

func (s *CalService) CheckForUpdate() (UpdateInfo, error) { return s.a.checkForUpdate() }

// InstallUpdate downloads the newest release, swaps the app and relaunches it.
func (s *CalService) InstallUpdate() error {
	info, err := s.a.checkForUpdate()
	if err != nil {
		return err
	}
	if !info.Available {
		return fmt.Errorf("you're already on the latest version (v%s)", version)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	progress := func(msg string) { s.a.wa.Event.Emit("update:progress", msg) }
	if err := installUpdate(ctx, info, progress); err != nil {
		return err
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		s.a.wa.Quit() // the swap script waits for us to exit, then relaunches the new version
	}()
	return nil
}

func (s *CalService) Hide() { s.a.win.Hide() }

func (s *CalService) Quit() { s.a.wa.Quit() }
