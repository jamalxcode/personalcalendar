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
	Accent           string         `json:"accent"`
	ShowHolidayRows  bool           `json:"showHolidayRows"`
	AIProvider       string         `json:"aiProvider"` // OpenRouter | OpenAI | ""
	WeekStart        int            `json:"weekStart"`
	WeekendDays      []int          `json:"weekendDays"`
	Playful          bool           `json:"playful"`
	Confetti         bool           `json:"confetti"`
	Sounds           bool           `json:"sounds"`
	Name             string         `json:"name"`
	Weather          []DayWeather   `json:"weather"`
	WeatherCity      string         `json:"weatherCity"`
	Countdowns       []Countdown    `json:"countdowns"`
	Pinned           []string       `json:"pinned"` // pinned event IDs
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
		Accent:           a.cfg.Accent,
		ShowHolidayRows:  a.cfg.ShowHolidayRows,
		AIProvider:       aiProvider(a.cfg.OpenAIKey),
		WeekStart:        a.cfg.WeekStart,
		WeekendDays:      a.cfg.WeekendDays,
		Playful:          !a.cfg.NoPlayful,
		Confetti:         !a.cfg.NoPlayful && !a.cfg.NoConfetti,
		Sounds:           !a.cfg.NoPlayful && a.cfg.Sounds,
		Name:             a.cfg.Name,
		WeatherCity:      a.cfg.WeatherCity,
		Now:              time.Now(),
		GoogleAccounts:   []string{},
	}
	st.Connected = st.Access == "full"
	a.mu.Lock()
	st.Weather, st.Countdowns = a.extras.Weather, a.extras.Countdowns
	a.mu.Unlock()
	if st.Weather == nil {
		st.Weather = []DayWeather{}
	}
	if st.Countdowns == nil {
		st.Countdowns = []Countdown{}
	}
	st.Pinned = []string{}
	for _, p := range a.cfg.Pinned {
		st.Pinned = append(st.Pinned, p.ID)
	}
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
		return s.a.state(), fmt.Errorf("turn on “Smart Cal” under Calendars in the window that just opened, then come back")
	}
}

// AddGoogleAccount is setup step 2: opens System Settings → Internet Accounts.
// Once the account is added, macOS notifies us and the calendars appear on their own.
func (s *CalService) AddGoogleAccount() {
	openURL("x-apple.systempreferences:com.apple.Internet-Accounts-Settings.extension")
	s.a.win.Hide()
}

// Ask handles one line typed in the AI box: add events, answer a question, or edit/delete an event.
// Without a key (or if the AI can't be reached) simple lines are still understood offline.
func (s *CalService) Ask(text string) (AskResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return AskResult{}, fmt.Errorf("type something first")
	}
	cals := s.a.cal.CachedCalendars()
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -7)
	known, _ := s.a.cal.Range(s.a.cfg, from, from.AddDate(0, 0, 67))
	if len(known) > 400 {
		known = known[:400]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	res, err := askAI(ctx, s.a.cfg, text, cals, known)
	if err != nil {
		// offline fallback for simple "thing + day/time" lines
		if d, ok := localParse(text, now); ok {
			res = AskResult{Kind: "add", Drafts: []Draft{d}, Offline: true}
			if err == errNoKey {
				res.Answer = "Understood without AI — add your OpenRouter key in Settings for smarter results."
			} else {
				res.Answer = "AI unavailable (" + err.Error() + ") — understood it offline."
			}
		} else {
			return AskResult{}, err
		}
	}
	def := s.a.cfg.DefaultCalendar
	if def == "" {
		for _, c := range cals {
			if c.Primary {
				def = c.ID
			}
		}
	}
	for i := range res.Drafts {
		if res.Drafts[i].CalendarID == "" {
			res.Drafts[i].CalendarID = def
		}
	}
	if res.Drafts == nil {
		res.Drafts = []Draft{}
	}
	if res.Refs == nil {
		res.Refs = []Event{}
	}
	return res, nil
}

// EventsBetween powers the any-day view and the month dots.
func (s *CalService) EventsBetween(from, to time.Time) ([]Event, error) {
	evs, err := s.a.cal.Range(s.a.cfg, from, to)
	if evs == nil {
		evs = []Event{}
	}
	return evs, err
}

// GetEvent loads full details (notes, attendees…) for the event card.
func (s *CalService) GetEvent(id string, start time.Time) (EventDetail, error) {
	return s.a.cal.Detail(id, start)
}

// UpdateEvent saves an edited event (one occurrence for repeating events).
func (s *CalService) UpdateEvent(id string, origStart time.Time, d Draft) (State, error) {
	if strings.TrimSpace(d.Title) == "" {
		return s.a.state(), fmt.Errorf("event needs a title")
	}
	if !d.End.After(d.Start) {
		return s.a.state(), fmt.Errorf("the end must be after the start")
	}
	if err := s.a.cal.Update(id, origStart, d); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// Search looks through the past month and the next year.
func (s *CalService) Search(q string) ([]Event, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []Event{}
	if len(q) < 2 {
		return out, nil
	}
	now := time.Now()
	evs, err := s.a.cal.Range(s.a.cfg, now.AddDate(0, -1, 0), now.AddDate(1, 0, 0))
	if err != nil {
		return out, err
	}
	for _, e := range evs {
		if strings.Contains(strings.ToLower(e.Title), q) || strings.Contains(strings.ToLower(e.Location), q) ||
			strings.Contains(strings.ToLower(e.Calendar), q) {
			out = append(out, e)
			if len(out) >= 60 {
				break
			}
		}
	}
	return out, nil
}

// GetGreeting returns the one-line greeting for the top of the panel.
func (s *CalService) GetGreeting() string {
	if s.a.cfg.NoPlayful {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return s.a.greeting(ctx)
}

// Emojis returns AI-picked emojis for titles the frontend couldn't match itself (cached forever).
func (s *CalService) Emojis(titles []string) map[string]string {
	return s.a.emojisFor(titles)
}

// PinEvent adds a countdown for an event; UnpinEvent removes it.
func (s *CalService) PinEvent(id, title string, start time.Time) State {
	for _, p := range s.a.cfg.Pinned {
		if p.ID == id {
			return s.a.state()
		}
	}
	s.a.cfg.Pinned = append(s.a.cfg.Pinned, Pin{ID: id, Title: title, Start: start.Format(time.RFC3339)})
	_ = s.a.cfg.Save()
	return s.a.doRefresh()
}

func (s *CalService) UnpinEvent(id string) State {
	keep := []Pin{}
	for _, p := range s.a.cfg.Pinned {
		if p.ID != id {
			keep = append(keep, p)
		}
	}
	s.a.cfg.Pinned = keep
	_ = s.a.cfg.Save()
	return s.a.doRefresh()
}

func (s *CalService) loginEnabled() bool {
	st, err := s.a.wa.Autostart.Status()
	return err == nil && st.Enabled
}

func aiProvider(key string) string {
	key = strings.TrimSpace(key)
	switch {
	case key == "":
		return ""
	case strings.HasPrefix(key, "sk-or-"):
		return "OpenRouter"
	default:
		return "OpenAI"
	}
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
	Accent           string   `json:"accent"`
	MenuBarStyle     string   `json:"menuBarStyle"`
	AlertMode        string   `json:"alertMode"`
	LaunchAtLogin    bool     `json:"launchAtLogin"`
	ShowHolidayRows  bool     `json:"showHolidayRows"`
	Region           string   `json:"region"`
	WeekStart        int      `json:"weekStart"`
	WeekendDays      []int    `json:"weekendDays"`
	Playful          bool     `json:"playful"`
	Confetti         bool     `json:"confetti"`
	Sounds           bool     `json:"sounds"`
	Voice            string   `json:"voice"`
	Name             string   `json:"name"`
	Weather          bool     `json:"weather"`
	WeatherCity      string   `json:"weatherCity"`
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
		HiddenCalendars: hidden, DefaultCalendar: c.DefaultCalendar, Accent: c.Accent,
		MenuBarStyle: c.MenuBarStyle, AlertMode: c.AlertMode, LaunchAtLogin: s.loginEnabled(),
		ShowHolidayRows: c.ShowHolidayRows, Region: c.Region, WeekStart: c.WeekStart, WeekendDays: c.WeekendDays,
		Playful: !c.NoPlayful, Confetti: !c.NoConfetti, Sounds: c.Sounds, Voice: c.Voice, Name: c.Name,
		Weather: !c.NoWeather, WeatherCity: c.WeatherCity}
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
	if validHex(in.Accent) {
		c.Accent = strings.ToUpper(in.Accent)
	}
	switch in.MenuBarStyle {
	case "next", "soon", "date", "icon":
		c.MenuBarStyle = in.MenuBarStyle
	}
	switch in.AlertMode {
	case "off", "0", "1", "5", "10":
		c.AlertMode = in.AlertMode
	}
	c.ShowHolidayRows = in.ShowHolidayRows
	if in.LaunchAtLogin != s.loginEnabled() {
		var err error
		if in.LaunchAtLogin {
			err = s.a.wa.Autostart.Enable()
		} else {
			err = s.a.wa.Autostart.Disable()
		}
		if err != nil {
			return s.a.state(), fmt.Errorf("couldn't change Start at login: %v", err)
		}
	}
	c.LaunchAtLogin = in.LaunchAtLogin
	if in.WeekStart >= 0 && in.WeekStart <= 6 {
		c.WeekStart = in.WeekStart
	}
	days := []int{}
	for _, d := range in.WeekendDays {
		if d >= 0 && d <= 6 {
			days = append(days, d)
		}
	}
	c.WeekendDays, c.Region, c.WeekSet = days, in.Region, true
	c.NoPlayful, c.NoConfetti, c.Sounds = !in.Playful, !in.Confetti, in.Sounds
	switch in.Voice {
	case "friendly", "formal", "fun":
		c.Voice = in.Voice
	}
	c.Name = strings.TrimSpace(in.Name)
	c.NoWeather = !in.Weather
	if city := strings.TrimSpace(in.WeatherCity); city != "" && !strings.EqualFold(city, c.WeatherCity) {
		name, lat, lon, err := geocode(city)
		if err != nil {
			return s.a.state(), err
		}
		c.WeatherCity, c.WeatherLat, c.WeatherLon = name, lat, lon
	}
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
