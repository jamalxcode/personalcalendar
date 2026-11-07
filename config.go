package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config is persisted to ~/Library/Application Support/PersonalCalendar/config.json.
type Config struct {
	OpenAIKey        string   `json:"openai_key"`
	OpenAIModel      string   `json:"openai_model"`
	Use24h           bool     `json:"use_24h"`
	ConfirmBeforeAdd bool     `json:"confirm_before_add"`
	DaysAhead        int      `json:"days_ahead"`
	HiddenCalendars  []string `json:"hidden_calendars"` // calendar IDs not shown
	DefaultCalendar  string   `json:"default_calendar"` // calendar ID for new events ("" = primary)
	Accent           string   `json:"accent"`           // highlight colour, #RRGGBB
	MenuBarStyle     string   `json:"menubar_style"`    // next | soon | date | icon
	AlertMode        string   `json:"alert_mode"`       // minutes before a meeting to notify: off | 0 | 1 | 5 | 10
	LaunchAtLogin    bool     `json:"launch_at_login"`
	LoginConfigured  bool     `json:"login_configured"`  // first-run default applied
	ShowHolidayRows  bool     `json:"show_holiday_rows"` // false = holidays appear as small day tags
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
	c := &Config{OpenAIModel: "gpt-4o-mini", ConfirmBeforeAdd: true, DaysAhead: 14}
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, c)
	}
	if c.OpenAIModel == "" {
		c.OpenAIModel = "gpt-4o-mini"
	}
	switch c.MenuBarStyle {
	case "next", "soon", "date", "icon":
	default:
		c.MenuBarStyle = "next"
	}
	switch c.AlertMode {
	case "off", "0", "1", "5", "10":
	default:
		c.AlertMode = "1"
	}
	if !validHex(c.Accent) {
		c.Accent = "#4169E1" // royal blue
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
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(), b, 0o600)
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
