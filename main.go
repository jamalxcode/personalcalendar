package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed all:frontend
var frontendFS embed.FS

// diagDir is set by build.sh (-X main.diagDir=...) to <source>/.diag so the developer
// can read what the app sees (calendar access, accounts found) without screen access.
var diagDir string

var buildStamp = "dev"

// Global hotkey that toggles the popover from anywhere.
const hotkey = "Ctrl+Alt+C"

type App struct {
	wa      *application.App
	tray    *application.SystemTray
	win     *application.WebviewWindow
	cfg     *Config
	cal     *Cal
	mu      sync.Mutex
	events  []Event
	err     error
	kick    chan struct{}
	update  UpdateInfo // latest release check (guarded by mu)
	notif   *notifications.NotificationService
	notifOK bool
	alerted map[string]bool // event+start already notified
}

func main() {
	// build.sh uses this to produce the .icns for the bundle.
	if len(os.Args) == 3 && os.Args[1] == "--write-icon" {
		if err := os.WriteFile(os.Args[2], appIconPNG(), 0o644); err != nil {
			log.Fatal(err)
		}
		return
	}

	a := &App{cfg: loadConfig(), cal: &Cal{}, kick: make(chan struct{}, 1), alerted: map[string]bool{}}
	a.notif = notifications.New()
	a.cal.Init()
	storeChanged = a.RequestRefresh // macOS tells us when any calendar changes

	assets, _ := fs.Sub(frontendFS, "frontend")
	a.wa = application.New(application.Options{
		Name:        "Personal Calendar",
		Description: "Menu bar calendar with AI quick add",
		Services:    []application.Service{application.NewService(&CalService{a: a}), application.NewService(a.notif)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory, // no Dock icon
		},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "company.sala.personalcalendar"},
	})

	a.win = a.wa.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "popover",
		Title:           "Personal Calendar",
		Width:           380,
		Height:          650,
		Frameless:       true,
		Hidden:          true,
		DisableResize:   true,
		AlwaysOnTop:     true,
		HideOnFocusLost: true,
		BackgroundType:  application.BackgroundTypeTranslucent,
		URL:             "/",
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBar{AppearsTransparent: true, Hide: true},
			InvisibleTitleBarHeight: 0,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
	})
	a.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		a.win.Hide()
		e.Cancel()
	})
	a.win.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		a.wa.Event.Emit("popover:shown")
	})

	a.tray = a.wa.SystemTray.New()
	a.tray.SetIcon(trayIconPNG()) // full-colour icon (not a template, so macOS doesn't flatten it)
	a.tray.SetTooltip("Personal Calendar  (" + hotkey + ")")
	a.tray.SetMenu(a.buildMenu())
	a.tray.AttachWindow(a.win).WindowOffset(6)

	if err := a.wa.GlobalShortcut.Register(hotkey, func() { a.tray.ToggleWindow() }); err != nil {
		log.Printf("hotkey %s not registered: %v", hotkey, err)
	}

	a.notif.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		meet, _ := r.Response.UserInfo["meet"].(string)
		if r.Response.ActionIdentifier == "join" && meet != "" {
			openURL(meet)
			return
		}
		a.tray.ShowWindow()
	})

	go a.loop()
	go a.updateLoop()
	go a.alertLoop()
	if err := a.wa.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *App) buildMenu() *application.Menu {
	m := a.wa.NewMenu()
	m.Add("Open Personal Calendar").OnClick(func(*application.Context) { a.tray.ShowWindow() })
	m.Add("Refresh").OnClick(func(*application.Context) { a.RequestRefresh() })
	m.Add("Settings…").OnClick(func(*application.Context) {
		a.tray.ShowWindow()
		a.wa.Event.Emit("ui:settings")
	})
	m.Add("Open Calendar App").OnClick(func(*application.Context) { openURL("ical://") })
	m.Add("Open Google Calendar (web)").OnClick(func(*application.Context) { openURL("https://calendar.google.com") })
	m.AddSeparator()
	m.Add("Check for Updates…").OnClick(func(*application.Context) {
		a.tray.ShowWindow()
		a.wa.Event.Emit("ui:updates")
	})
	m.Add("Quit Personal Calendar").OnClick(func(*application.Context) { a.wa.Quit() })
	return m
}

func (a *App) loop() {
	time.Sleep(time.Second)
	if !a.cfg.LoginConfigured { // first run: start at login by default
		if err := a.wa.Autostart.Enable(); err == nil {
			a.cfg.LaunchAtLogin = true
		}
		a.cfg.LoginConfigured = true
		_ = a.cfg.Save()
	}
	if a.cal.Access() == "notDetermined" {
		a.cal.RequestAccess() // first launch: macOS asks "allow access to Calendars?"
	}
	a.doRefresh()
	if !a.cal.Connected() { // a missing AI key only shows a banner; the calendar view comes first
		a.tray.ShowWindow()
		a.wa.Event.Emit("ui:settings")
	}
	fetch := time.NewTicker(2 * time.Minute)
	tick := time.NewTicker(15 * time.Second)
	for {
		select {
		case <-fetch.C:
			a.doRefresh()
		case <-a.kick:
			a.doRefresh()
		case <-tick.C:
			a.updateLabel()
		}
	}
}

// alertLoop posts a notification shortly before each timed meeting (with Join when there's a link).
func (a *App) alertLoop() {
	time.Sleep(3 * time.Second)
	if ok, err := a.notif.RequestNotificationAuthorization(); err == nil && ok {
		a.notifOK = true
		_ = a.notif.RegisterNotificationCategory(notifications.NotificationCategory{
			ID:      "meeting",
			Actions: []notifications.NotificationAction{{ID: "join", Title: "Join"}},
		})
	}
	t := time.NewTicker(15 * time.Second)
	for range t.C {
		a.checkAlerts()
	}
}

func (a *App) checkAlerts() {
	mode := a.cfg.AlertMode
	if mode == "off" {
		return
	}
	lead, _ := strconv.Atoi(mode)
	evs, _ := a.snapshot()
	now := time.Now()
	for _, e := range evs {
		if e.AllDay || e.Holiday {
			continue
		}
		fire := e.Start.Add(-time.Duration(lead) * time.Minute)
		if now.Before(fire) || now.After(e.Start.Add(2*time.Minute)) {
			continue
		}
		key := e.ID + e.Start.Format(time.RFC3339)
		if a.alerted[key] {
			continue
		}
		a.alerted[key] = true
		when := "Starting now"
		if d := e.Start.Sub(now); d > 30*time.Second {
			when = "In " + shortDur(d)
		}
		body := when + " · " + a.fmtTime(e.Start) + "–" + a.fmtTime(e.End)
		if e.Location != "" {
			body += " · " + e.Location
		}
		sent := false
		if a.notifOK {
			opts := notifications.NotificationOptions{
				ID: "meet-" + key, Title: e.Title, Body: body,
				Data: map[string]interface{}{"meet": e.MeetLink, "eventId": e.ID},
			}
			var err error
			if e.MeetLink != "" {
				opts.CategoryID = "meeting"
				err = a.notif.SendNotificationWithActions(opts)
			} else {
				err = a.notif.SendNotification(opts)
			}
			sent = err == nil
		}
		if !sent { // notifications not allowed: drop the panel down instead
			a.tray.ShowWindow()
		}
		a.wa.Event.Emit("alert", map[string]any{"title": e.Title, "body": body, "meet": e.MeetLink,
			"id": e.ID, "start": e.Start})
	}
}

// updateLoop checks GitHub for a newer release shortly after launch and every 6 hours.
func (a *App) updateLoop() {
	time.Sleep(10 * time.Second)
	for {
		a.checkForUpdate()
		time.Sleep(6 * time.Hour)
	}
}

func (a *App) checkForUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	info, err := checkLatest(ctx)
	if err == nil {
		a.mu.Lock()
		a.update = info
		a.mu.Unlock()
		a.wa.Event.Emit("update", info)
	}
	return info, err
}

func (a *App) RequestRefresh() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

func (a *App) doRefresh() State {
	evs, err := a.cal.Upcoming(a.cfg, a.cfg.DaysAhead)
	a.mu.Lock()
	if err == nil {
		a.events = evs
	}
	a.err = err
	a.mu.Unlock()
	a.updateLabel()
	st := a.state()
	a.wa.Event.Emit("state", st)
	writeDiag(st)
	return st
}

func writeDiag(st State) {
	if diagDir == "" {
		return
	}
	_ = os.MkdirAll(diagDir, 0o755)
	type calLine struct {
		Name, Account, Source     string
		Google, Writable, Primary bool
	}
	var cals []calLine
	for _, c := range st.Calendars {
		cals = append(cals, calLine{c.Name, c.Account, c.Source, c.Google, c.Writable, c.Primary})
	}
	d := map[string]any{
		"time": time.Now().Format(time.RFC3339), "build": buildStamp, "access": st.Access, "hasGoogle": st.HasGoogle,
		"googleAccounts": st.GoogleAccounts, "events": len(st.Events), "error": st.Error,
		"hasKey": st.HasKey, "calendars": cals,
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	_ = os.WriteFile(filepath.Join(diagDir, "status.json"), b, 0o644)
}

func (a *App) snapshot() ([]Event, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Event(nil), a.events...), a.err
}

// nextEvent returns the in-progress or next timed event (all-day events don't go in the menu bar).
func nextEvent(evs []Event, now time.Time) (Event, bool) {
	for _, e := range evs {
		if !e.AllDay && e.End.After(now) {
			return e, true
		}
	}
	return Event{}, false
}

func (a *App) updateLabel() {
	evs, _ := a.snapshot()
	now := time.Now()
	date := now.Format("Mon 2 Jan")
	label := date
	style := a.cfg.MenuBarStyle
	if style == "icon" {
		a.tray.SetLabel("")
		return
	}
	if e, ok := nextEvent(evs, now); ok && style != "date" {
		t := truncate(e.Title, 22)
		switch {
		case !e.Start.After(now):
			label = fmt.Sprintf("● %s · %s left", t, shortDur(e.End.Sub(now)))
		case e.Start.Sub(now) <= time.Hour:
			label = fmt.Sprintf("%s in %s", t, shortDur(e.Start.Sub(now)))
		case sameDay(e.Start, now):
			label = fmt.Sprintf("%s · %s", a.fmtTime(e.Start), t)
		case style == "next" && e.Start.Sub(now) < 7*24*time.Hour:
			day := e.Start.Format("Mon")
			if sameDay(e.Start, now.AddDate(0, 0, 1)) {
				day = "Tomorrow"
			}
			label = fmt.Sprintf("%s %s · %s", day, a.fmtTime(e.Start), t)
		}
	}
	a.tray.SetLabel(label)
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
