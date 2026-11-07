package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------- weather (Open-Meteo, no key needed) ----------------

type DayWeather struct {
	Date string  `json:"date"` // YYYY-MM-DD
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
	Code int     `json:"code"` // WMO weather code
}

type weatherCache struct {
	mu      sync.Mutex
	days    []DayWeather
	fetched time.Time
	key     string
}

var wx weatherCache

func (a *App) weather() []DayWeather {
	if a.cfg.NoWeather || a.cfg.NoPlayful {
		return []DayWeather{}
	}
	key := fmt.Sprintf("%.3f,%.3f", a.cfg.WeatherLat, a.cfg.WeatherLon)
	wx.mu.Lock()
	defer wx.mu.Unlock()
	if wx.key == key && time.Since(wx.fetched) < time.Hour && len(wx.days) > 0 {
		return wx.days
	}
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&daily=weather_code,temperature_2m_max,temperature_2m_min&timezone=auto&forecast_days=10",
		a.cfg.WeatherLat, a.cfg.WeatherLon)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return wx.days // keep the last good forecast
	}
	defer resp.Body.Close()
	var r struct {
		Daily struct {
			Time []string  `json:"time"`
			Code []int     `json:"weather_code"`
			Max  []float64 `json:"temperature_2m_max"`
			Min  []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil || len(r.Daily.Time) == 0 {
		return wx.days
	}
	var out []DayWeather
	for i, d := range r.Daily.Time {
		if i < len(r.Daily.Code) && i < len(r.Daily.Max) && i < len(r.Daily.Min) {
			out = append(out, DayWeather{Date: d, Code: r.Daily.Code[i], Max: r.Daily.Max[i], Min: r.Daily.Min[i]})
		}
	}
	wx.days, wx.fetched, wx.key = out, time.Now(), key
	return out
}

// geocode turns a city name into coordinates (Open-Meteo geocoding).
func geocode(city string) (name string, lat, lon float64, err error) {
	u := "https://geocoding-api.open-meteo.com/v1/search?count=1&language=en&name=" + url.QueryEscape(city)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	var r struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Results) == 0 {
		return "", 0, 0, fmt.Errorf("couldn't find a place called %q", city)
	}
	g := r.Results[0]
	n := g.Name
	if g.Country != "" {
		n += ", " + g.Country
	}
	return n, g.Latitude, g.Longitude, nil
}

// ---------------- countdowns ----------------

type Countdown struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	Birthday bool      `json:"birthday"`
	Pinned   bool      `json:"pinned"`
}

// countdowns = pinned events still ahead + birthdays in the next 30 days.
func (a *App) countdowns() []Countdown {
	now := time.Now()
	out := []Countdown{}
	keep := a.cfg.Pinned[:0:0]
	for _, p := range a.cfg.Pinned {
		t, err := time.Parse(time.RFC3339, p.Start)
		if err != nil || t.Before(now.Add(-24*time.Hour)) {
			continue // expired pins drop off
		}
		keep = append(keep, p)
		out = append(out, Countdown{ID: p.ID, Title: p.Title, Start: t, Pinned: true})
	}
	if len(keep) != len(a.cfg.Pinned) {
		a.cfg.Pinned = keep
		_ = a.cfg.Save()
	}
	if !a.cfg.NoPlayful && a.cal.Connected() {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		if evs, err := a.cal.Range(a.cfg, day, day.AddDate(0, 0, 31)); err == nil {
			for _, e := range evs {
				if isBirthday(e, a.cal.CachedCalendars()) {
					out = append(out, Countdown{ID: e.ID, Title: e.Title, Start: e.Start, Birthday: true})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func isBirthday(e Event, cals []CalendarInfo) bool {
	t := strings.ToLower(e.Title)
	if strings.Contains(t, "birthday") || strings.Contains(t, "bday") || strings.Contains(t, "b-day") {
		return true
	}
	for _, c := range cals {
		if c.ID == e.CalendarID && c.Source == "birthdays" {
			return true
		}
	}
	return false
}

// ---------------- greeting ----------------

type greetCache struct {
	mu   sync.Mutex
	key  string
	text string
}

var greet greetCache

// greeting returns a one-line morning/afternoon/evening summary. It uses the AI when a key is
// set (cached per part-of-day + schedule, so it costs at most a few calls a day) and a
// built-in template otherwise.
func (a *App) greeting(ctx context.Context) string {
	now := time.Now()
	evs, _ := a.snapshot()
	part := partOfDay(now)
	focus := now
	if part == "evening" || part == "night" {
		focus = now.AddDate(0, 0, 1) // in the evening, preview tomorrow
	}
	var dayEvents []Event
	for _, e := range evs {
		if e.Holiday || !sameDay(e.Start, focus) && !(e.AllDay && !e.Start.After(focus) && e.End.After(focus)) {
			continue
		}
		if sameDay(focus, now) && !e.AllDay && !e.End.After(now) {
			continue
		}
		dayEvents = append(dayEvents, e)
	}
	var sig strings.Builder
	for _, e := range dayEvents {
		sig.WriteString(e.ID + e.Start.Format("1504") + "|")
	}
	key := now.Format("2006-01-02") + part + sig.String() + a.cfg.Voice + a.cfg.Name + a.cfg.lang()
	greet.mu.Lock()
	if greet.key == key && greet.text != "" {
		t := greet.text
		greet.mu.Unlock()
		return t
	}
	greet.mu.Unlock()

	text := a.localGreeting(now, part, focus, dayEvents)
	if strings.TrimSpace(a.cfg.OpenAIKey) != "" {
		if t, err := a.aiGreeting(ctx, now, part, focus, dayEvents); err == nil && t != "" {
			text = t
		}
	}
	greet.mu.Lock()
	greet.key, greet.text = key, text
	greet.mu.Unlock()
	return text
}

func partOfDay(t time.Time) string {
	switch h := t.Hour(); {
	case h < 5:
		return "night"
	case h < 12:
		return "morning"
	case h < 17:
		return "afternoon"
	case h < 22:
		return "evening"
	default:
		return "night"
	}
}

func (a *App) isWeekend(t time.Time) bool {
	for _, d := range a.cfg.WeekendDays {
		if int(t.Weekday()) == d {
			return true
		}
	}
	return false
}

func (a *App) localGreeting(now time.Time, part string, focus time.Time, evs []Event) string {
	hello := map[string]string{"morning": "Good morning", "afternoon": "Good afternoon", "evening": "Good evening", "night": "Hello, night owl"}[part]
	if n := strings.TrimSpace(a.cfg.Name); n != "" {
		hello += " " + n
	}
	icon := map[string]string{"morning": "☀️", "afternoon": "🌤️", "evening": "🌙", "night": "🦉"}[part]
	when := "today"
	if !sameDay(focus, now) {
		when = "tomorrow"
	}
	var timed []Event
	for _, e := range evs {
		if !e.AllDay {
			timed = append(timed, e)
		}
	}
	var line string
	switch {
	case len(timed) == 0 && a.isWeekend(focus):
		line = "Nothing planned " + when + " — enjoy the weekend."
	case len(timed) == 0:
		line = "Nothing on the calendar " + when + " — it's all yours."
	case len(timed) == 1:
		line = fmt.Sprintf("Just one thing %s: %s at %s.", when, timed[0].Title, a.fmtTime(timed[0].Start))
	default:
		line = fmt.Sprintf("%d things %s, first at %s.", len(timed), when, a.fmtTime(timed[0].Start))
		last := timed[len(timed)-1]
		if sameDay(focus, now) && last.End.Hour() < 17 {
			line += fmt.Sprintf(" Free after %s.", a.fmtTime(last.End))
		}
	}
	if d := a.daysToWeekend(now); d > 0 && d <= 2 && part != "night" {
		line += fmt.Sprintf(" Weekend in %d day%s.", d, map[bool]string{true: "", false: "s"}[d == 1])
	}
	return fmt.Sprintf("%s %s. %s", hello, icon, line)
}

func (a *App) daysToWeekend(now time.Time) int {
	if a.isWeekend(now) || len(a.cfg.WeekendDays) == 0 {
		return 0
	}
	for i := 1; i <= 7; i++ {
		if a.isWeekend(now.AddDate(0, 0, i)) {
			return i
		}
	}
	return 0
}

var greetSchema = map[string]any{
	"type":                 "object",
	"properties":           map[string]any{"text": str("The greeting, max 24 words, one line")},
	"required":             []string{"text"},
	"additionalProperties": false,
}

func (a *App) aiGreeting(ctx context.Context, now time.Time, part string, focus time.Time, evs []Event) (string, error) {
	var b strings.Builder
	for _, e := range evs {
		if e.AllDay {
			fmt.Fprintf(&b, "- all day: %s\n", e.Title)
		} else {
			fmt.Fprintf(&b, "- %s–%s %s\n", a.fmtTime(e.Start), a.fmtTime(e.End), e.Title)
		}
	}
	if b.Len() == 0 {
		b.WriteString("(nothing)\n")
	}
	weather := ""
	for _, w := range a.weather() {
		if w.Date == focus.Format("2006-01-02") {
			weather = fmt.Sprintf("Weather %s: high %.0f°C, low %.0f°C, WMO code %d.", map[bool]string{true: "today", false: "tomorrow"}[sameDay(focus, now)], w.Max, w.Min, w.Code)
		}
	}
	sys := fmt.Sprintf(`Write the one-line greeting shown at the top of a calendar app.
It is %s, %s (the %s). Summarise %s's schedule below: how many things, the first time, a free stretch if useful.
Mention the weather only if notable (very hot, rain, sandstorm-dusty). The weekend is %s; mention it if it's close.
%s
Max 24 words, one line, start with a greeting that fits the time of day.%s`,
		now.Format("Monday 2 January 15:04"), now.Location().String(), part,
		map[bool]string{true: "today", false: "tomorrow"}[sameDay(focus, now)], a.cfg.WeekendText(), voiceLine(a.cfg),
		map[bool]string{true: "\nWrite it in Arabic (Gulf-friendly Modern Standard Arabic).", false: ""}[a.cfg.lang() == "ar"])
	content, err := callAI(ctx, a.cfg, sys, b.String()+weather, "greeting", greetSchema)
	if err != nil {
		return "", err
	}
	var r struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Text), nil
}

// ---------------- emojis for event titles ----------------
// The frontend matches common words itself; titles it can't match are sent here once
// and the AI's pick is cached in emoji.json so each title costs at most one lookup ever.

var emojiMu sync.Mutex

func emojiPath() string { return filepath.Join(appDir(), "emoji.json") }

func loadEmojiCache() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(emojiPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

var emojiSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"items": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": str("The title exactly as given"),
				"emoji": str("One emoji that best fits this calendar event"),
			},
			"required": []string{"title", "emoji"}, "additionalProperties": false,
		}},
	},
	"required":             []string{"items"},
	"additionalProperties": false,
}

func (a *App) emojisFor(titles []string) map[string]string {
	emojiMu.Lock()
	defer emojiMu.Unlock()
	cache := loadEmojiCache()
	out := map[string]string{}
	var missing []string
	for _, t := range titles {
		k := strings.ToLower(strings.TrimSpace(t))
		if k == "" {
			continue
		}
		if e, ok := cache[k]; ok {
			out[t] = e
		} else if len(missing) < 40 {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 || strings.TrimSpace(a.cfg.OpenAIKey) == "" {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	content, err := callAI(ctx, a.cfg, "Pick one fitting emoji for each calendar event title. Return every title exactly as given.",
		strings.Join(missing, "\n"), "event_emojis", emojiSchema)
	if err != nil {
		return out
	}
	var r struct {
		Items []struct{ Title, Emoji string } `json:"items"`
	}
	if json.Unmarshal(content, &r) != nil {
		return out
	}
	for _, it := range r.Items {
		k := strings.ToLower(strings.TrimSpace(it.Title))
		if k != "" && it.Emoji != "" && len([]rune(it.Emoji)) <= 4 {
			cache[k] = it.Emoji
			out[it.Title] = it.Emoji
		}
	}
	for _, t := range missing { // remember misses too, so they aren't asked again
		if _, ok := cache[strings.ToLower(strings.TrimSpace(t))]; !ok {
			cache[strings.ToLower(strings.TrimSpace(t))] = ""
		}
	}
	if b, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(emojiPath(), b, 0o600)
	}
	return out
}
