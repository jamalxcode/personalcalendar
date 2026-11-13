package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	HasAI            bool           `json:"hasAI"`      // an OpenRouter/OpenAI key is set
	TrayHidden       bool           `json:"trayHidden"` // macOS is hiding the menu bar item (full menu bar / notch)
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
	Todos            []Todo         `json:"todos"`
	RemindersAccess  string         `json:"remindersAccess"`
	ShowTodos        bool           `json:"showTodos"`
	Travel           map[string]int `json:"travel"` // "id|unixStart" → driving minutes
	TextSize         int            `json:"textSize"`
	Layout           string         `json:"layout"` // wide | compact
	Lang             string         `json:"lang"`
	ReduceMotion     bool           `json:"reduceMotion"`
	Contrast         bool           `json:"contrast"`
	AskLogin         bool           `json:"askLogin"`
	Now              time.Time      `json:"now"`
}

func (a *App) state() State {
	evs, err := a.snapshot()
	cals := a.cal.CachedCalendars()
	st := State{
		Access:           a.cal.Access(),
		HasKey:           strings.TrimSpace(a.cfg.OpenAIKey) != "",
		HasAI:            hasAI(a.cfg),
		Use24h:           a.cfg.is24h(),
		ConfirmBeforeAdd: a.cfg.ConfirmBeforeAdd,
		DaysAhead:        a.cfg.DaysAhead,
		Events:           evs,
		Calendars:        cals,
		DefaultCalendar:  a.cfg.DefaultCalendar,
		Hotkey:           a.cfg.Hotkey,
		Accent:           a.cfg.accent(),
		ShowHolidayRows:  a.cfg.ShowHolidayRows,
		AIProvider:       aiProvider(a.cfg.OpenAIKey),
		WeekStart:        a.cfg.WeekStart,
		WeekendDays:      a.cfg.WeekendDays,
		Playful:          !a.cfg.NoPlayful,
		Confetti:         !a.cfg.NoPlayful && !a.cfg.NoConfetti,
		Sounds:           !a.cfg.NoPlayful && a.cfg.Sounds,
		Name:             a.cfg.Name,
		WeatherCity:      a.cfg.WeatherCity,
		RemindersAccess:  a.cal.RemindersAccess(),
		Lang:             a.cfg.lang(),
		ReduceMotion:     sysCache.ReduceMotion,
		Contrast:         sysCache.Contrast,
		AskLogin:         !a.cfg.LoginAsked,
		ShowTodos:        !a.cfg.NoTodos,
		TextSize:         a.cfg.TextSize,
		Layout:           a.cfg.Layout,
		Now:              time.Now(),
		GoogleAccounts:   []string{},
	}
	st.Connected = st.Access == "full"
	a.mu.Lock()
	st.Weather, st.Countdowns, st.Todos = a.extras.Weather, a.extras.Countdowns, a.extras.Todos
	st.Travel = map[string]int{}
	for k, v := range a.travel {
		st.Travel[k] = v
	}
	a.mu.Unlock()
	if st.Todos == nil {
		st.Todos = []Todo{}
	}
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
	a.mu.Lock()
	st.TrayHidden = a.trayHidden
	a.mu.Unlock()
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
		t0 := time.Now()
		ok := s.a.cal.RequestAccess()
		if !ok && time.Since(t0) < 2*time.Second {
			// refused instantly without a prompt: macOS is holding an old answer for a previous build.
			// Clear it for this app only and ask again.
			resetCalendarPermission()
			ok = s.a.cal.RequestAccess()
		}
		s.a.showPanel() // the system prompt takes focus away from the popover
		if ok {
			return s.a.doRefresh(), nil
		}
	default:
		// refused earlier (often for an older build): clear that answer and ask again
		resetCalendarPermission()
		if s.a.cal.Access() == "notDetermined" && s.a.cal.RequestAccess() {
			s.a.showPanel()
			return s.a.doRefresh(), nil
		}
	}
	openURL("x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars")
	return s.a.state(), fmt.Errorf("macOS didn't show its prompt. In the System Settings window that just opened, turn on “Smart Cal” under Calendars, then come back")
}

// AddGoogleAccount is setup step 2: opens System Settings → Internet Accounts.
// Once the account is added, macOS notifies us and the calendars appear on their own.
func (s *CalService) AddGoogleAccount() {
	openURL("x-apple.systempreferences:com.apple.Internet-Accounts-Settings.extension")
	s.a.win.Hide()
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

// ---- settings ----

type Settings struct {
	OpenAIKeyHint    string     `json:"openaiKeyHint"` // "sk-…abcd" for display
	OpenAIKey        string     `json:"openaiKey"`     // write-only: empty = keep current
	BackupKeyHint    string     `json:"backupKeyHint"` // "sk-or…abcd" for display
	BackupKey        string     `json:"backupKey"`     // write-only: empty = keep, "-" = remove
	BackupModel      string     `json:"backupModel"`
	OpenAIModel      string     `json:"openaiModel"`
	Use24h           bool       `json:"use24h"`
	ConfirmBeforeAdd bool       `json:"confirmBeforeAdd"`
	DaysAhead        int        `json:"daysAhead"`
	HiddenCalendars  []string   `json:"hiddenCalendars"`
	DefaultCalendar  string     `json:"defaultCalendar"`
	Accent           string     `json:"accent"`
	MenuBarStyle     string     `json:"menuBarStyle"`
	BarTitle         string     `json:"barTitle"`
	BarAhead         string     `json:"barAhead"`
	BarEmoji         bool       `json:"barEmoji"`
	BarTime          string     `json:"barTime"`
	BarHideInMeeting bool       `json:"barHideInMeeting"`
	BarDot           string     `json:"barDot"`
	AlertMode        string     `json:"alertMode"`
	LaunchAtLogin    bool       `json:"launchAtLogin"`
	ShowHolidayRows  bool       `json:"showHolidayRows"`
	Region           string     `json:"region"`
	WeekStart        int        `json:"weekStart"`
	WeekendDays      []int      `json:"weekendDays"`
	Playful          bool       `json:"playful"`
	Confetti         bool       `json:"confetti"`
	Sounds           bool       `json:"sounds"`
	Voice            string     `json:"voice"`
	Name             string     `json:"name"`
	Weather          bool       `json:"weather"`
	WeatherCity      string     `json:"weatherCity"`
	DefaultAlarm     string     `json:"defaultAlarm"`
	AllDayAlarm      string     `json:"allDayAlarm"`
	TravelFrom       string     `json:"travelFrom"`
	ShowTodos        bool       `json:"showTodos"`
	TodoList         string     `json:"todoList"`
	TodoLists        []TodoList `json:"todoLists"`
	Hotkey           string     `json:"hotkey"`
	TextSize         int        `json:"textSize"`
	Layout           string     `json:"layout"`
	QuickHotkey      string     `json:"quickHotkey"`
	BriefTime        string     `json:"briefTime"`
	BetaUpdates      bool       `json:"betaUpdates"`
	ClockMode        string     `json:"clockMode"`
	TrayIcon         string     `json:"trayIcon"`
	ShowInDock       bool       `json:"showInDock"`
	Language         string     `json:"language"`
	SystemAccent     string     `json:"systemAccent"` // read-only: the Mac's accent colour
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
	bhint := ""
	if k := strings.TrimSpace(c.BackupKey); len(k) > 8 {
		bhint = k[:5] + "…" + k[len(k)-4:]
	}
	return Settings{OpenAIKeyHint: hint, BackupKeyHint: bhint, BackupModel: c.BackupModel, OpenAIModel: c.OpenAIModel, Use24h: c.Use24h,
		ConfirmBeforeAdd: c.ConfirmBeforeAdd, DaysAhead: c.DaysAhead,
		HiddenCalendars: hidden, DefaultCalendar: c.DefaultCalendar, Accent: c.Accent,
		MenuBarStyle: c.MenuBarStyle, BarTitle: c.BarTitle, BarAhead: c.BarAhead, BarEmoji: !c.BarNoEmoji,
		BarTime: c.BarTime, BarHideInMeeting: c.BarHideInMeeting, BarDot: c.BarDot, AlertMode: c.AlertMode, LaunchAtLogin: s.loginEnabled(),
		ShowHolidayRows: c.ShowHolidayRows, Region: c.Region, WeekStart: c.WeekStart, WeekendDays: c.WeekendDays,
		Playful: !c.NoPlayful, Confetti: !c.NoConfetti, Sounds: c.Sounds, Voice: c.Voice, Name: c.Name,
		Weather: !c.NoWeather, WeatherCity: c.WeatherCity, DefaultAlarm: c.DefaultAlarm, AllDayAlarm: c.AllDayAlarm,
		TravelFrom: c.TravelFrom, ShowTodos: !c.NoTodos, TodoList: c.TodoList, TodoLists: s.a.cal.ReminderLists(),
		Hotkey: c.Hotkey, TextSize: c.TextSize, Layout: c.Layout, QuickHotkey: c.QuickHotkey, BriefTime: c.BriefTime, BetaUpdates: c.BetaUpdates, ClockMode: c.ClockMode, TrayIcon: c.TrayIcon,
		ShowInDock: c.ShowInDock, Language: c.Language, SystemAccent: sysCache.Accent}
}

func (s *CalService) SaveSettings(in Settings) (State, error) {
	c := s.a.cfg
	if k := strings.TrimSpace(in.OpenAIKey); k != "" {
		c.OpenAIKey = k
	}
	if m := strings.TrimSpace(in.OpenAIModel); m != "" {
		c.OpenAIModel = m
	}
	switch k := strings.TrimSpace(in.BackupKey); {
	case k == "-":
		c.BackupKey = ""
	case k != "":
		c.BackupKey = k
	}
	if m := strings.TrimSpace(in.BackupModel); m != "" {
		c.BackupModel = m
	}
	c.ConfirmBeforeAdd = in.ConfirmBeforeAdd
	if in.DaysAhead > 0 {
		c.DaysAhead = in.DaysAhead
	}
	c.HiddenCalendars = in.HiddenCalendars
	c.DefaultCalendar = in.DefaultCalendar
	if validHex(in.Accent) {
		c.Accent = strings.ToUpper(in.Accent)
	} else if in.Accent == "system" {
		c.Accent = "system"
	}
	if m := oneOf(in.ClockMode, "", "system", "12", "24"); m != "" {
		c.ClockMode = m
		c.Use24h = c.is24h()
	}
	if t := oneOf(in.TrayIcon, "", "color", "mono"); t != "" && t != c.TrayIcon {
		c.TrayIcon = t
		s.a.iconKey = "" // re-set the icon on the next label update
	}
	if in.ShowInDock != c.ShowInDock {
		c.ShowInDock = in.ShowInDock
		setDockVisible(c.ShowInDock)
	}
	if l := oneOf(in.Language, "", "auto", "en", "ar"); l != "" {
		c.Language = l
	}
	if validBarStyle(in.MenuBarStyle) {
		c.MenuBarStyle = in.MenuBarStyle
	}
	c.BarTitle = oneOf(in.BarTitle, c.BarTitle, "none", "word", "short", "full")
	c.BarAhead = oneOf(in.BarAhead, c.BarAhead, "hour", "today", "any")
	c.BarTime = oneOf(in.BarTime, c.BarTime, "countdown", "clock")
	c.BarDot = oneOf(in.BarDot, c.BarDot, "off", "5", "10", "15")
	c.BarNoEmoji = !in.BarEmoji
	c.BarHideInMeeting = in.BarHideInMeeting
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
	switch in.DefaultAlarm {
	case "none", "0", "5", "10", "15", "30", "60":
		c.DefaultAlarm = in.DefaultAlarm
	}
	switch in.AllDayAlarm {
	case "none", "dayBefore9", "sameDay9":
		c.AllDayAlarm = in.AllDayAlarm
	}
	if strings.TrimSpace(in.TravelFrom) != c.TravelFrom {
		c.TravelFrom = strings.TrimSpace(in.TravelFrom)
		s.a.clearTravel()
	}
	c.NoTodos, c.TodoList = !in.ShowTodos, in.TodoList
	if in.QuickHotkey != "" && in.QuickHotkey != c.QuickHotkey {
		if err := s.a.setQuickHotkey(in.QuickHotkey); err != nil {
			return s.a.state(), err
		}
	}
	if in.BriefTime == "off" {
		c.BriefTime = "off"
	} else if _, err := time.Parse("15:04", in.BriefTime); err == nil {
		c.BriefTime = in.BriefTime
	}
	if in.BetaUpdates != c.BetaUpdates {
		c.BetaUpdates = in.BetaUpdates
		go s.a.checkForUpdate() // switching channels: look again straight away
	}
	if l := oneOf(in.Layout, "", "wide", "compact"); l != "" && l != c.Layout {
		c.Layout = l
		s.a.applyTextSize() // resizes the panel and tells it to re-lay itself out
	}
	switch in.TextSize {
	case 100, 115, 130:
		if in.TextSize != c.TextSize {
			c.TextSize = in.TextSize
			s.a.applyTextSize()
		}
	}
	if in.Hotkey != "" && in.Hotkey != c.Hotkey {
		if err := s.a.setHotkey(in.Hotkey); err != nil {
			return s.a.state(), err
		}
	}
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
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "ical://") {
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
func (s *CalService) InstallUpdate() (err error) {
	if !s.a.beginUpdate() {
		return fmt.Errorf("an update is already being installed")
	}
	// every step goes to all windows (progress card) and to the menu bar ("⬇︎ Updating 40%")
	progress := func(st UpdateStep) {
		s.a.wa.Event.Emit("update:progress", st)
		label := "⬇︎ Updating…"
		if st.Stage == "downloading" && st.Pct >= 0 {
			label = fmt.Sprintf("⬇︎ Updating %d%%", st.Pct)
		}
		s.a.setUpdating(label)
	}
	defer func() {
		if err != nil { // give the menu bar back and tell the windows why
			s.a.setUpdating("")
			s.a.wa.Event.Emit("update:progress", UpdateStep{Stage: "error", Text: err.Error(), Pct: -1})
		}
	}()
	progress(UpdateStep{Stage: "checking", Text: "Checking for the newest version…", Pct: -1})
	info, err := s.a.checkForUpdate()
	if err != nil {
		return err
	}
	if !info.Available {
		return fmt.Errorf("you're already on the latest version (v%s)", version)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if err := installUpdate(ctx, info, progress); err != nil {
		return err
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		go func() { time.Sleep(3 * time.Second); os.Exit(0) }() // never let a stuck quit block the update
		s.a.wa.Quit()                                           // the swap script waits for us to exit, then relaunches the new version
	}()
	return nil
}

func (s *CalService) Hide() { s.a.win.Hide() }

// SetQuickHeight fits the quick-add window to what it shows (Spotlight-style).
func (s *CalService) SetQuickHeight(h int) {
	if h < 90 {
		h = 90
	}
	if h > 560 {
		h = 560
	}
	s.a.quick.SetSize(600, h)
}

// AIStatus: how the last AI request went (Settings → AI).
func (s *CalService) AIStatus() AIStatus { return lastAIStatus() }

// HideQuick closes the quick-add box.
func (s *CalService) HideQuick() { s.a.quick.Hide() }

// OpenPanel closes the quick-add box and opens the full panel (⌘↩ in quick add).
func (s *CalService) OpenPanel() {
	s.a.quick.Hide()
	s.a.showPanel()
}

func (s *CalService) Quit() { s.a.wa.Quit() }

// ======================= events: ask / add / edit / delete =======================

// Ask handles one input typed in the AI box. Without a key (or if the AI can't be reached)
// simple lines are still understood offline.
func (s *CalService) Ask(text string) (AskResult, error) { return s.AskWith(text, "") }

// AskWith is Ask with an optional pasted/dropped image (base64 JPEG).
func (s *CalService) AskWith(text, imageB64 string) (AskResult, error) {
	text = strings.TrimSpace(text)
	if text == "" && imageB64 == "" {
		return AskResult{}, fmt.Errorf("type something first")
	}
	cals := s.a.cal.CachedCalendars()
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -7)
	known, _ := s.a.cal.Range(s.a.cfg, from, from.AddDate(0, 0, 67))
	if len(known) > 400 {
		known = known[:400]
	}
	// room for the main AI (up to 70 s), one retry, and the backup AI
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Second)
	defer cancel()
	ctx, info := withCallInfo(ctx)
	res, err := askAI(ctx, s.a.cfg, text, imageB64, cals, known)
	if err != nil {
		if imageB64 != "" && err == errNoKey {
			return AskResult{}, fmt.Errorf("reading screenshots needs your OpenRouter key (Settings → AI)")
		}
		// say why straight away — never just "AI unavailable"
		why := "No AI key yet — add your OpenRouter key in Settings → AI for smarter results."
		if err != errNoKey {
			actx, acancel := context.WithTimeout(context.Background(), 10*time.Second)
			why = failureText(actx, err, s.a.cfg)
			acancel()
		}
		if d, ok := localParse(text, now); ok && imageB64 == "" {
			fixTrailingMinutes(&d)
			res = AskResult{Kind: "add", Drafts: []Draft{d}, Offline: true}
			res.Note = "The AI didn't answer, so Smart Cal read this on its own — check the date and title. Why: " + why
		} else {
			return AskResult{}, fmt.Errorf("The AI didn't answer. %s", why)
		}
	} else if len(info.notes) > 0 {
		res.Note = strings.Join(info.notes, " ")
	}
	def := s.defaultCal(cals)
	for i := range res.Drafts {
		if res.Drafts[i].CalendarID == "" {
			res.Drafts[i].CalendarID = def
		}
	}
	if res.slotReq != nil {
		res.Slots = s.findSlots(*res.slotReq, def)
		if len(res.Slots) == 0 {
			res.Answer = "I couldn't find a free " + shortDur(time.Duration(max(15, res.slotReq.Minutes))*time.Minute) + " in that range — try a wider range or different hours."
		}
	}
	// clashes for each new event
	res.Clashes = make([][]Event, len(res.Drafts))
	for i, d := range res.Drafts {
		res.Clashes[i] = s.clashes(d, "")
	}
	if res.Draft != nil && res.Target != nil {
		// saving an edit writes every field, so carry over what the AI didn't change:
		// the notes (the event list has none) and the time zone the event was made in
		if det, err := s.a.cal.Detail(res.Target.ID, res.Target.Start); err == nil {
			if res.Draft.Notes == "" {
				res.Draft.Notes = det.Notes
			}
			if res.Draft.TimeZone == "" && !res.Draft.AllDay {
				res.Draft.TimeZone = det.TimeZone
			}
		}
		res.Clashes = [][]Event{s.clashes(*res.Draft, res.Target.ID)}
	}
	for _, l := range []*[]Draft{&res.Drafts, &res.Slots} {
		if *l == nil {
			*l = []Draft{}
		}
	}
	if res.Refs == nil {
		res.Refs = []Event{}
	}
	if res.Todos == nil {
		res.Todos = []TodoDraft{}
	}
	return res, nil
}

// FromClipboard runs whatever is on the clipboard (text or a screenshot) through the AI.
func (s *CalService) FromClipboard() (AskResult, error) {
	text, img := readClipboard()
	if strings.TrimSpace(text) == "" && img == "" {
		return AskResult{}, fmt.Errorf("the clipboard is empty — copy a message, email or screenshot first")
	}
	if img != "" {
		text = "" // a copied screenshot wins over any stray text
	}
	return s.AskWith(text, img)
}

func (s *CalService) defaultCal(cals []CalendarInfo) string {
	if s.a.cfg.DefaultCalendar != "" {
		return s.a.cfg.DefaultCalendar
	}
	for _, c := range cals {
		if c.Primary {
			return c.ID
		}
	}
	return ""
}

func (s *CalService) clashes(d Draft, skipID string) []Event {
	if d.AllDay {
		return []Event{}
	}
	evs, err := s.a.cal.Range(s.a.cfg, d.Start.Add(-12*time.Hour), d.End.Add(12*time.Hour))
	if err != nil {
		return []Event{}
	}
	return clashesWith(evs, d.Start, d.End, skipID)
}

// CheckClashes is used by the editor and drag-to-reschedule to warn about overlaps.
func (s *CalService) CheckClashes(d Draft, skipID string) []Event { return s.clashes(d, skipID) }

func (s *CalService) findSlots(r aiSlot, calID string) []Draft {
	now := time.Now()
	from, err1 := parseDay(r.From)
	to, err2 := parseDay(r.To)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if err1 != nil || from.Before(today) {
		from = today
	}
	if err2 != nil || to.Before(from) {
		to = from.AddDate(0, 0, 6)
	}
	if to.Sub(from) > 45*24*time.Hour {
		to = from.AddDate(0, 0, 45)
	}
	mins := r.Minutes
	if mins < 15 {
		mins = 60
	}
	earliest, latest := r.Earliest, r.Latest
	if earliest <= 0 || earliest > 22 {
		earliest = 8
	}
	if latest <= earliest || latest > 24 {
		latest = 20
	}
	evs, err := s.a.cal.Range(s.a.cfg, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil
	}
	wk := map[int]bool{}
	for _, d := range s.a.cfg.WeekendDays {
		wk[d] = true
	}
	title := strings.TrimSpace(r.Title)
	if title == "" {
		title = "Busy"
	}
	var out []Draft
	for _, b := range freeSlots(evs, from, to, time.Duration(mins)*time.Minute, earliest, latest, wk, r.IncludeWeekend, now, 5) {
		out = append(out, Draft{Title: title, Start: b.s, End: b.e, CalendarID: calID})
	}
	return out
}

// AddResult carries fresh state plus references to the new events (for Undo).
type AddResult struct {
	State   State      `json:"state"`
	Created []EventRef `json:"created"`
}

// Add saves drafts (with the default alert unless one was chosen) and returns refs for Undo.
func (s *CalService) Add(drafts []Draft) (AddResult, error) {
	var created []EventRef
	for _, d := range drafts {
		if strings.TrimSpace(d.Title) == "" {
			return AddResult{State: s.a.state()}, fmt.Errorf("event needs a title")
		}
		if d.Alarm == "" && d.ReminderMinutes == 0 {
			if d.AllDay {
				d.Alarm = s.a.cfg.AllDayAlarm
			} else {
				d.Alarm = s.a.cfg.DefaultAlarm
			}
		}
		ref, err := s.a.cal.Insert(d.CalendarID, d)
		if err != nil {
			return AddResult{State: s.a.doRefresh(), Created: created}, err
		}
		created = append(created, ref)
	}
	if created == nil {
		created = []EventRef{}
	}
	return AddResult{State: s.a.doRefresh(), Created: created}, nil
}

// UpdateEvent saves an edited event; allFuture applies it to this and later occurrences.
func (s *CalService) UpdateEvent(id string, origStart time.Time, d Draft, allFuture bool) (State, error) {
	if strings.TrimSpace(d.Title) == "" {
		return s.a.state(), fmt.Errorf("event needs a title")
	}
	if !d.End.After(d.Start) {
		return s.a.state(), fmt.Errorf("the end must be after the start")
	}
	if err := s.a.cal.Update(id, origStart, d, allFuture); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// Delete removes one occurrence (or this and all future ones of a repeating event).
func (s *CalService) Delete(eventID string, start time.Time, allFuture bool) (State, error) {
	if err := s.a.cal.Delete(eventID, start, allFuture); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// Search looks through titles, places, calendars and notes, a year back and a year ahead.
func (s *CalService) Search(q string) ([]Event, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []Event{}
	if len(q) < 2 {
		return out, nil
	}
	now := time.Now()
	// one two-year fetch serves every keystroke for a minute (cleared on any refresh)
	s.a.mu.Lock()
	evs, fresh := s.a.searchEvs, time.Since(s.a.searchAt) < time.Minute
	s.a.mu.Unlock()
	if !fresh {
		var err error
		evs, err = s.a.cal.Range(s.a.cfg, now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0))
		if err != nil {
			return out, err
		}
		s.a.mu.Lock()
		s.a.searchEvs, s.a.searchAt = evs, now
		s.a.mu.Unlock()
	}
	// newest first for the past, soonest first for the future
	var past, future []Event
	for _, e := range evs {
		hay := strings.ToLower(e.Title + "\n" + e.Location + "\n" + e.Calendar + "\n" + e.Notes)
		if strings.Contains(hay, q) {
			if e.End.Before(now) {
				past = append(past, e)
			} else {
				future = append(future, e)
			}
		}
	}
	for i := len(past) - 1; i >= 0 && len(out) < 30; i-- {
		out = append([]Event{past[i]}, out...)
	}
	for _, e := range future {
		if len(out) >= 80 {
			break
		}
		out = append(out, e)
	}
	return out, nil
}

// ======================= to-dos (Apple Reminders) =======================

func (s *CalService) AllowReminders() (State, error) {
	switch s.a.cal.RemindersAccess() {
	case "full":
	default:
		if s.a.cal.RemindersAccess() != "notDetermined" {
			_ = exec.Command("/usr/bin/tccutil", "reset", "Reminders", "company.sala.personalcalendar").Run()
		}
		ok := s.a.cal.RequestReminders()
		s.a.showPanel()
		if !ok {
			openURL("x-apple.systempreferences:com.apple.preference.security?Privacy_Reminders")
			return s.a.state(), fmt.Errorf("macOS didn't show its prompt. Turn on “Smart Cal” under Reminders in the window that just opened")
		}
	}
	return s.a.doRefresh(), nil
}

// AddTodos creates to-dos; returns their IDs for Undo.
func (s *CalService) AddTodos(todos []TodoDraft) ([]string, error) {
	ids := []string{}
	for _, t := range todos {
		m := map[string]any{"title": t.Title, "notes": t.Notes, "listId": s.a.cfg.TodoList, "due": 0, "hasTime": t.HasTime}
		if t.Due != nil {
			m["due"] = t.Due.Unix()
		}
		td, err := s.a.cal.SaveReminder(m)
		if err != nil {
			return ids, err
		}
		ids = append(ids, td.ID)
	}
	s.a.RequestRefresh()
	return ids, nil
}

func (s *CalService) CompleteTodo(id string, done bool) (State, error) {
	if _, err := s.a.cal.SaveReminder(map[string]any{"id": id, "done": done}); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

func (s *CalService) UpdateTodo(id, title string, due *time.Time, hasTime bool) (State, error) {
	m := map[string]any{"id": id, "title": title, "due": 0, "hasTime": hasTime}
	if due != nil {
		m["due"] = due.Unix()
	}
	if _, err := s.a.cal.SaveReminder(m); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

func (s *CalService) DeleteTodo(id string) (State, error) {
	if err := s.a.cal.DeleteReminder(id); err != nil {
		return s.a.state(), err
	}
	return s.a.doRefresh(), nil
}

// ======================= windows =======================

// OpenWeek shows the big week window.
func (s *CalService) OpenWeek() {
	s.a.win.Hide()
	s.a.showWeek()
}

// resetCalendarPermission clears macOS's stored calendar answer for this app only,
// so the next request shows the system prompt again.
func resetCalendarPermission() {
	_ = exec.Command("/usr/bin/tccutil", "reset", "Calendar", "company.sala.personalcalendar").Run()
}

// SetLabelEmojis receives the emoji the panel shows for each event title (rules + AI picks),
// so the menu bar label can use the same one.
func (s *CalService) SetLabelEmojis(m map[string]string) {
	s.a.mu.Lock()
	s.a.labelEmoji = m
	s.a.mu.Unlock()
	s.a.updateLabel()
}

// ======================= Mac conventions =======================

// OpenSettings shows the Settings window (tab: "", "ai", "about", "about-check", …).
func (s *CalService) OpenSettings(tab string) { s.a.showSettings(tab) }

// CloseSettings hides the Settings window.
func (s *CalService) CloseSettings() { s.a.settings.Hide() }

// ShowAbout opens the standard macOS About panel.
func (s *CalService) ShowAbout() { s.a.showAboutPanel() }

// AnswerLogin records the first-run "Start Smart Cal when you log in?" answer.
func (s *CalService) AnswerLogin(yes bool) (State, error) {
	c := s.a.cfg
	c.LoginAsked = true
	var err error
	if yes {
		err = s.a.wa.Autostart.Enable()
	} else if s.loginEnabled() {
		err = s.a.wa.Autostart.Disable()
	}
	c.LaunchAtLogin = yes && err == nil
	_ = c.Save()
	if err != nil {
		return s.a.state(), fmt.Errorf("couldn't change Start at login: %v", err)
	}
	return s.a.state(), nil
}
