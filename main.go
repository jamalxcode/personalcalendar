package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"
)

const appID = "company.sala.personalcalendar"

type App struct {
	fa   fyne.App
	desk desktop.App
	cfg  *Config
	cal  *GCal
	ui   *MainUI

	mu      sync.Mutex
	events  []Event
	lastErr error
	lastOK  time.Time
	refresh chan struct{}
}

func main() {
	// Used by build.sh to produce the .icns for the app bundle.
	if len(os.Args) == 3 && os.Args[1] == "--write-icon" {
		if err := os.WriteFile(os.Args[2], appIcon().Content(), 0o644); err != nil {
			panic(err)
		}
		return
	}
	a := &App{fa: app.NewWithID(appID), cfg: loadConfig(), cal: &GCal{}, refresh: make(chan struct{}, 1)}
	a.fa.Settings().SetTheme(&calTheme{})
	desk, ok := a.fa.(desktop.App)
	if !ok {
		panic("desktop driver required")
	}
	a.desk = desk
	icon := fyne.NewStaticResource("tray.png", trayIconPNG())
	a.fa.SetIcon(appIcon())

	_ = a.cal.Init()
	a.ui = newMainUI(a)
	a.rebuildTray()
	desk.SetSystemTrayIcon(icon)

	a.fa.Lifecycle().SetOnStarted(func() {
		systray.SetTemplateIcon(trayIconPNG(), trayIconPNG())
		a.updateTitle()
		if !a.cal.Connected() || a.cfg.OpenAIKey == "" {
			a.ui.Show()
			a.ui.openSettings()
		}
	})

	go a.loop()
	a.fa.Run()
}

func (a *App) loop() {
	a.doRefresh()
	fetch := time.NewTicker(2 * time.Minute)
	tick := time.NewTicker(20 * time.Second)
	for {
		select {
		case <-fetch.C:
			a.doRefresh()
		case <-a.refresh:
			a.doRefresh()
		case <-tick.C:
			a.updateTitle()
			a.ui.tickNext()
		}
	}
}

func (a *App) RequestRefresh() {
	select {
	case a.refresh <- struct{}{}:
	default:
	}
}

func (a *App) doRefresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := a.cal.Upcoming(ctx, a.cfg, a.cfg.DaysAhead)
	a.mu.Lock()
	if err == nil {
		a.events = evs
		a.lastOK = time.Now()
	}
	a.lastErr = err
	a.mu.Unlock()
	a.ui.render()
	a.rebuildTray()
	a.updateTitle()
}

func (a *App) Events() ([]Event, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Event(nil), a.events...), a.lastErr
}

// nextEvent returns the in-progress or next timed event (all-day events are skipped for the menu bar).
func nextEvent(evs []Event, now time.Time) (Event, bool) {
	for _, e := range evs {
		if e.AllDay || !e.End.After(now) {
			continue
		}
		return e, true
	}
	return Event{}, false
}

func (a *App) updateTitle() {
	evs, _ := a.Events()
	now := time.Now()
	title := now.Format("Mon 2 Jan")
	if e, ok := nextEvent(evs, now); ok {
		t := truncate(e.Title, 20)
		switch {
		case !e.Start.After(now):
			title = fmt.Sprintf("● %s · %s left", t, shortDur(e.End.Sub(now)))
		case e.Start.Sub(now) <= 60*time.Minute:
			title = fmt.Sprintf("%s in %s", t, shortDur(e.Start.Sub(now)))
		case sameDay(e.Start, now):
			title = fmt.Sprintf("%s %s", a.fmtTime(e.Start), t)
		}
	}
	systray.SetTitle(" " + title)
	systray.SetTooltip("Personal Calendar")
}

func (a *App) rebuildTray() {
	evs, err := a.Events()
	now := time.Now()
	var items []*fyne.MenuItem
	items = append(items, fyne.NewMenuItem("Open Personal Calendar…", func() { a.ui.Show() }))
	items = append(items, fyne.NewMenuItemSeparator())
	if err != nil && len(evs) == 0 {
		it := fyne.NewMenuItem("⚠︎ "+truncate(err.Error(), 50), func() { a.ui.Show() })
		items = append(items, it)
	}
	shown := 0
	lastDay := ""
	for _, e := range evs {
		if shown >= 10 {
			break
		}
		day := dayLabel(e.Start, now)
		if day != lastDay {
			h := fyne.NewMenuItem(strings.ToUpper(day), nil)
			h.Disabled = true
			items = append(items, h)
			lastDay = day
		}
		when := "All day"
		if !e.AllDay {
			when = a.fmtTime(e.Start)
		}
		link := e.Link
		items = append(items, fyne.NewMenuItem(fmt.Sprintf("   %s   %s", when, truncate(e.Title, 40)), func() { openURL(link) }))
		shown++
	}
	if shown == 0 && err == nil {
		none := fyne.NewMenuItem("Nothing coming up", nil)
		none.Disabled = true
		items = append(items, none)
	}
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Refresh", a.RequestRefresh),
		fyne.NewMenuItem("Open Google Calendar", func() { openURL("https://calendar.google.com") }),
		fyne.NewMenuItem("Settings…", func() { a.ui.Show(); a.ui.openSettings() }),
	)
	a.desk.SetSystemTrayMenu(fyne.NewMenu("Personal Calendar", items...))
	systray.SetTemplateIcon(trayIconPNG(), trayIconPNG())
}

func (a *App) fmtTime(t time.Time) string {
	if a.cfg.Use24h {
		return t.Format("15:04")
	}
	if t.Minute() == 0 {
		return t.Format("3 PM")
	}
	return t.Format("3:04 PM")
}

func openURL(u string) {
	if u != "" {
		_ = exec.Command("open", u).Start()
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func shortDur(d time.Duration) string {
	m := int(d.Round(time.Minute).Minutes())
	if m < 1 {
		return "now"
	}
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h, mm := m/60, m%60
	if mm == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, mm)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func dayLabel(t, now time.Time) string {
	switch {
	case sameDay(t, now) || t.Before(now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, 1)):
		return "Tomorrow"
	case t.Sub(now) < 6*24*time.Hour:
		return t.Format("Monday")
	default:
		return t.Format("Monday 2 Jan")
	}
}
