package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config is persisted to ~/Library/Application Support/PersonalCalendar/config.json.
type Config struct {
	OpenAIKey        string   `json:"openai_key"`
	OpenAIModel      string   `json:"openai_model"`
	BackupKey        string   `json:"backup_key"`   // second AI, tried when the main one fails ("" = none)
	BackupModel      string   `json:"backup_model"` // its model
	Use24h           bool     `json:"use_24h"`
	ConfirmBeforeAdd bool     `json:"confirm_before_add"`
	DaysAhead        int      `json:"days_ahead"`
	HiddenCalendars  []string `json:"hidden_calendars"` // calendar IDs not shown
	DefaultCalendar  string   `json:"default_calendar"` // calendar ID for new events ("" = primary)
	Accent           string   `json:"accent"`           // highlight colour, #RRGGBB
	MenuBarStyle     string   `json:"menubar_style"`    // compact | minimal | countdown | time | full | today | lasthour | date | datecompact | icon
	BarTitle         string   `json:"bar_title"`        // none | word | short | full
	BarAhead         string   `json:"bar_ahead"`        // hour | today | any
	BarNoEmoji       bool     `json:"bar_no_emoji"`
	BarTime          string   `json:"bar_time"` // countdown | clock
	BarHideInMeeting bool     `json:"bar_hide_in_meeting"`
	BarDot           string   `json:"bar_dot"`    // minutes before a meeting for the orange dot: off | 5 | 10 | 15
	BarSet           bool     `json:"bar_set"`    // v1.9 menu bar options migrated
	AlertMode        string   `json:"alert_mode"` // minutes before a meeting to notify: off | 0 | 1 | 5 | 10
	LaunchAtLogin    bool     `json:"launch_at_login"`
	LoginConfigured  bool     `json:"login_configured"`  // first-run default applied
	ShowHolidayRows  bool     `json:"show_holiday_rows"` // false = holidays appear as small day tags

	// Week & region
	Region      string `json:"region"`       // preset that filled week_start/weekend (kw, sa, ae, eu, us, custom)
	WeekStart   int    `json:"week_start"`   // 0=Sun … 6=Sat
	WeekendDays []int  `json:"weekend_days"` // e.g. [5,6] = Fri+Sat
	WeekSet     bool   `json:"week_set"`     // false → apply Kuwait defaults

	// Playful
	NoPlayful   bool    `json:"no_playful"` // master switch (inverted so the default is on)
	NoConfetti  bool    `json:"no_confetti"`
	Sounds      bool    `json:"sounds"`
	Voice       string  `json:"voice"` // friendly | formal | fun
	Name        string  `json:"name"`  // used in greetings
	NoWeather   bool    `json:"no_weather"`
	WeatherCity string  `json:"weather_city"`
	WeatherLat  float64 `json:"weather_lat"`
	WeatherLon  float64 `json:"weather_lon"`
	Pinned      []Pin   `json:"pinned"` // countdowns

	// Practical
	DefaultAlarm string `json:"default_alarm"` // new timed events: none | 0 | 5 | 10 | 15 | 30 | 60
	AllDayAlarm  string `json:"all_day_alarm"` // new all-day events: none | dayBefore9 | sameDay9
	AlarmsSet    bool   `json:"alarms_set"`    // defaults applied once
	EmojiBarSet  bool   `json:"emoji_bar_set"` // compact emoji menu bar applied once
	TravelFrom   string `json:"travel_from"`   // address for "leave now" driving times ("" = off)
	NoTodos      bool   `json:"no_todos"`      // hide the to-do section
	TodoList     string `json:"todo_list"`     // Reminders list for new to-dos ("" = default)
	Hotkey       string `json:"hotkey"`        // global shortcut ("off" = none)
	TextSize     int    `json:"text_size"`     // 100 | 115 | 130 (%)
	Layout       string `json:"layout"`        // wide (month + day side by side) | compact (one column)

	// Mac conventions (v2.0)
	ClockMode  string `json:"clock_mode"`   // system | 12 | 24
	TrayIcon   string `json:"tray_icon"`    // color | mono
	ShowInDock bool   `json:"show_in_dock"` // also show a Dock icon
	LoginAsked bool   `json:"login_asked"`  // asked "start at login?" (or decided before v2.0)
	Language   string `json:"language"`     // auto | en | ar
	WeekBounds *Rect  `json:"week_bounds,omitempty"`
	SetBounds  *Rect  `json:"settings_bounds,omitempty"`
	MacSet     bool   `json:"mac_set"` // v2.0 defaults applied

	// v2.3
	QuickHotkey string `json:"quick_hotkey"` // global shortcut for the quick-add window ("off" = none)
	BriefTime   string `json:"brief_time"`   // morning brief notification "HH:MM" or "off"
	BriefLast   string `json:"brief_last"`   // date of the last brief (YYYY-MM-DD), so it's sent once a day
	BetaUpdates bool   `json:"beta_updates"` // also offer pre-release (beta) versions
	V23Set      bool   `json:"v23_set"`      // v2.3 defaults applied once
	LastVersion string `json:"last_version"` // the version that last ran here — a different one means an update just happened
}

// Pin is an event pinned as a countdown.
type Pin struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Start string `json:"start"` // RFC3339
}

var cfgMu sync.Mutex

func appDir() string {
	base, err := os.UserConfigDir() // ~/Library/Application Support on macOS
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, "PersonalCalendar")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func configPath() string      { return filepath.Join(appDir(), "config.json") }
func credentialsPath() string { return filepath.Join(appDir(), "credentials.json") }
func tokenPath() string       { return filepath.Join(appDir(), "token.json") }

func loadConfig() *Config {
	c := &Config{OpenAIModel: "gpt-4o-mini", DaysAhead: 14}
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, c)
	}
	if c.BackupModel == "" {
		c.BackupModel = "google/gemini-2.5-flash" // a different company's model, so one outage doesn't take both down
	}
	if c.OpenAIModel == "" {
		c.OpenAIModel = "gpt-4o-mini"
	}
	// each model must be one its key's service has (an OpenAI key can't use google/gemini-…)
	c.OpenAIModel = fitModel(c.OpenAIKey, c.OpenAIModel)
	c.BackupModel = fitModel(c.BackupKey, c.BackupModel)
	if !c.EmojiBarSet { // v1.8.7: the compact "👥 2h Meet" label becomes the default
		if c.MenuBarStyle == "" || c.MenuBarStyle == "next" {
			c.MenuBarStyle = "emoji"
		}
		c.EmojiBarSet = true
	}
	if !c.BarSet { // v1.9: old style names → new ones, with matching fine-tune values
		switch c.MenuBarStyle {
		case "next":
			c.MenuBarStyle = "full"
		case "soon":
			c.MenuBarStyle = "lasthour"
		case "date", "icon":
		default:
			c.MenuBarStyle = "compact"
		}
		c.BarTitle, c.BarAhead, c.BarTime, c.BarDot = "word", "any", "countdown", "5"
		if c.MenuBarStyle == "lasthour" {
			c.BarAhead = "hour"
		}
		c.BarSet = true
	}
	if !validBarStyle(c.MenuBarStyle) {
		c.MenuBarStyle = "compact"
	}
	c.BarTitle = oneOf(c.BarTitle, "word", "none", "word", "short", "full")
	c.BarAhead = oneOf(c.BarAhead, "any", "hour", "today", "any")
	c.BarTime = oneOf(c.BarTime, "countdown", "countdown", "clock")
	c.BarDot = oneOf(c.BarDot, "5", "off", "5", "10", "15")
	switch c.AlertMode {
	case "off", "0", "1", "5", "10":
	default:
		c.AlertMode = "1"
	}
	if !validHex(c.Accent) && c.Accent != "system" {
		c.Accent = "#4169E1" // royal blue
	}
	if !c.MacSet {
		if c.Use24h {
			c.ClockMode = "24" // keep an explicit choice
		} else {
			c.ClockMode = "system"
		}
		c.LoginAsked = c.LoginConfigured // existing installs already decided
		c.MacSet = true
	}
	c.ClockMode = oneOf(c.ClockMode, "system", "system", "12", "24")
	c.TrayIcon = oneOf(c.TrayIcon, "color", "color", "mono")
	c.Language = oneOf(c.Language, "auto", "auto", "en", "ar")
	if !c.WeekSet {
		c.Region, c.WeekStart, c.WeekendDays = "kw", 6, []int{5, 6} // Saturday start, Fri+Sat weekend
	}
	if c.WeekStart < 0 || c.WeekStart > 6 {
		c.WeekStart = 6
	}
	if c.WeekendDays == nil {
		c.WeekendDays = []int{}
	}
	switch c.Voice {
	case "friendly", "formal", "fun":
	default:
		c.Voice = "friendly"
	}
	if c.WeatherCity == "" {
		c.WeatherCity, c.WeatherLat, c.WeatherLon = "Kuwait City", 29.3759, 47.9774
	}
	if !c.AlarmsSet {
		c.DefaultAlarm, c.AllDayAlarm, c.AlarmsSet = "15", "dayBefore9", true
	}
	if c.Hotkey == "" {
		c.Hotkey = "Ctrl+Alt+C"
	}
	if !c.V23Set {
		// v2.3: typed events are added straight away (with Undo) instead of waiting in a preview,
		// a quick-add shortcut and a morning brief
		c.ConfirmBeforeAdd = false
		if c.QuickHotkey == "" {
			c.QuickHotkey = "Ctrl+Alt+N" // ⌃⌥Space often switches keyboard languages, so N for "new"
		}
		if c.BriefTime == "" {
			c.BriefTime = "08:00"
		}
		c.V23Set = true
	}
	if c.QuickHotkey == "" {
		c.QuickHotkey = "off"
	}
	if c.BriefTime != "off" {
		if _, err := time.Parse("15:04", c.BriefTime); err != nil {
			c.BriefTime = "08:00"
		}
	}
	c.Layout = oneOf(c.Layout, "wide", "wide", "compact")
	switch c.TextSize {
	case 100, 115, 130:
	default:
		c.TextSize = 100
	}
	if c.Pinned == nil {
		c.Pinned = []Pin{}
	}
	if c.DaysAhead <= 0 {
		c.DaysAhead = 14
	}
	// The key lives only in this file (0600) — deliberately not the Keychain, so there are no password prompts.
	if c.OpenAIKey == "" {
		c.OpenAIKey = os.Getenv("OPENROUTER_API_KEY")
	}
	if c.OpenAIKey == "" {
		c.OpenAIKey = os.Getenv("OPENAI_API_KEY")
	}
	return c
}

func (c *Config) Save() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// write a temp file and rename it, so a crash mid-write can never lose the config (and the AI key)
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

func (c *Config) IsHidden(id string) bool {
	for _, h := range c.HiddenCalendars {
		if h == id {
			return true
		}
	}
	return false
}

func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// WeekendText is "Friday and Saturday" style text for the AI prompt.
func (c *Config) WeekendText() string {
	if len(c.WeekendDays) == 0 {
		return "no fixed weekend"
	}
	var n []string
	for _, d := range c.WeekendDays {
		if d >= 0 && d <= 6 {
			n = append(n, weekdayNames[d])
		}
	}
	return strings.Join(n, " and ")
}

func validBarStyle(s string) bool {
	switch s {
	case "compact", "minimal", "countdown", "time", "full", "today", "lasthour", "date", "datecompact", "icon":
		return true
	}
	return false
}

// oneOf returns v if it is one of allowed, else def.
func oneOf(v, def string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

// Rect is a remembered window frame.
type Rect struct{ X, Y, W, H int }

// sysCache holds the Mac's own settings, refreshed with the events.
var sysCache = SysPrefs{Accent: "#4169E1", Lang: "en"}

func (c *Config) is24h() bool {
	switch c.ClockMode {
	case "12":
		return false
	case "24":
		return true
	}
	return sysCache.H24
}

func (c *Config) accent() string {
	if c.Accent == "system" {
		return sysCache.Accent
	}
	return c.Accent
}

func (c *Config) lang() string {
	if c.Language == "auto" {
		if strings.HasPrefix(sysCache.Lang, "ar") {
			return "ar"
		}
		return "en"
	}
	return c.Language
}
