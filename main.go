package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

type App struct {
	wa           *application.App
	tray         *application.SystemTray
	win          *application.WebviewWindow
	cfg          *Config
	cal          *Cal
	mu           sync.Mutex
	events       []Event
	err          error
	kick         chan struct{}
	update       UpdateInfo // latest release check (guarded by mu)
	notif        *notifications.NotificationService
	notifOK      bool
	alerted      map[string]bool // event+start already notified
	badged       bool            // tray shows the "starting soon" badge
	iconKey      string          // current tray image (soon|style), guarded by trayMu
	trayMu       sync.Mutex
	searchEvs    []Event // cached two-year range for search (guarded by mu)
	searchAt     time.Time
	settings     *application.WebviewWindow
	quick        *application.WebviewWindow // quick-add: just the AI box, from a shortcut anywhere
	quickOn      string                     // currently registered quick-add shortcut
	snoozed      map[string]time.Time       // event key → when to alert again (Snooze 10 min)
	week         *application.WebviewWindow
	travel       map[string]int // "id|unix" → driving minutes (guarded by mu)
	travelAt     map[string]time.Time
	labelEmoji   map[string]string // title → emoji, sent by the panel
	leaveAlerted map[string]bool
	hotkeyOn     string       // currently registered shortcut
	justUpdated  *UpdatedInfo // set on the first launch after an update, until the panel has shown it (guarded by mu)
	mainCheck    KeyCheck     // what the AI service said about the main key when it was saved (guarded by mu)
	backupCheck  KeyCheck
	trayHidden   bool   // macOS is hiding the menu bar item (full menu bar / notch), guarded by mu
	updating     string // while an update installs: the menu bar text ("⬇︎ Updating 40%"), guarded by trayMu
	extras       extras // weather + countdowns, refreshed with events
}

type extras struct {
	Weather    []DayWeather
	Countdowns []Countdown
	Todos      []Todo
}

func main() {
	// build.sh uses this to produce the .icns for the bundle.
	if len(os.Args) == 3 && os.Args[1] == "--write-icon" {
		if err := os.WriteFile(os.Args[2], appIconPNG(), 0o644); err != nil {
			log.Fatal(err)
		}
		return
	}

	_, statErr := os.Stat(configPath())
	hadConfig := statErr == nil // an existing install (not the first launch ever)
	a := &App{cfg: loadConfig(), cal: &Cal{}, kick: make(chan struct{}, 1), alerted: map[string]bool{},
		travel: map[string]int{}, travelAt: map[string]time.Time{}, leaveAlerted: map[string]bool{}, snoozed: map[string]time.Time{}}
	a.noteUpdate(hadConfig)
	a.notif = notifications.New()
	a.cal.Init()
	storeChanged = a.RequestRefresh // macOS tells us when any calendar changes

	assets, _ := fs.Sub(frontendFS, "frontend")
	svc := &CalService{a: a}
	a.wa = application.New(application.Options{
		Name:        "Smart Cal",
		Description: "Menu bar calendar with AI quick add",
		Services:    []application.Service{application.NewService(svc), application.NewService(a.notif)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory, // no Dock icon
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "company.sala.personalcalendar",
			// opening Smart Cal again (Applications, Spotlight, Launchpad) while it runs: show the panel
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { a.showPanel() },
		},
	})

	a.win = a.wa.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "popover",
		Title:           "Smart Cal",
		Width:           380,
		Height:          700,
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
	a.tray.SetTooltip("Smart Cal")
	a.tray.SetMenu(a.buildMenu())
	a.wa.Menu.Set(a.buildAppMenu()) // the menu bar while Smart Cal is active (shows with "Show in Dock")
	a.tray.AttachWindow(a.win).WindowOffset(6)

	if err := a.setHotkey(a.cfg.Hotkey); err != nil {
		log.Printf("hotkey %s not registered: %v", a.cfg.Hotkey, err)
	}

	// the big week window (opened from the panel, the tray menu or ⌘⇧W)
	a.week = a.wa.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "week",
		Title:     "Smart Cal — Week",
		Width:     1120,
		Height:    780,
		MinWidth:  760,
		MinHeight: 520,
		Hidden:    true,
		URL:       "/week.html",
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBar{AppearsTransparent: true, FullSizeContent: true, HideTitle: true},
			InvisibleTitleBarHeight: 46,
		},
	})
	a.week.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		a.week.Hide()
		e.Cancel()
	})

	// Quick add: a small floating AI box (like Spotlight) from a shortcut anywhere
	a.quick = a.wa.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "quick",
		Title:           "Quick Add",
		Width:           600,
		Height:          340,
		Frameless:       true,
		Hidden:          true,
		DisableResize:   true,
		AlwaysOnTop:     true,
		HideOnFocusLost: true,
		BackgroundType:  application.BackgroundTypeTranslucent,
		URL:             "/?quick=1",
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBar{AppearsTransparent: true, Hide: true},
			InvisibleTitleBarHeight: 0,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
	})
	a.quick.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		a.quick.Hide()
		e.Cancel()
	})
	if err := a.setQuickHotkey(a.cfg.QuickHotkey); err != nil {
		log.Printf("quick-add shortcut %s not registered: %v", a.cfg.QuickHotkey, err)
	}

	// Settings: its own window, like every Mac app (⌘, from anywhere in Smart Cal)
	a.settings = a.wa.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "settings",
		Title:     "Settings",
		Width:     620,
		Height:    720,
		MinWidth:  520,
		MinHeight: 480,
		Hidden:    true,
		URL:       "/?settings=1",
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{AppearsTransparent: true, FullSizeContent: true},
		},
	})
	a.settings.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		a.settings.Hide()
		e.Cancel()
	})
	// window frames: restore once the native windows exist (before Run they're no-ops), then track changes
	// clicking Smart Cal in Finder/Launchpad while it runs: show the panel
	a.wa.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { a.showPanel() })
	a.wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.rememberFrame(a.week, &a.cfg.WeekBounds)
		a.rememberFrame(a.settings, &a.cfg.SetBounds)
	})

	// clicking the Dock icon (when shown) opens the panel
	a.wa.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		a.showPanel()
	})
	// size the drop-down to the screen once the app (and its screen list) is up
	a.wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.applyTextSize()
	})

	a.notif.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		info := r.Response.UserInfo
		meet, _ := info["meet"].(string)
		key, _ := info["key"].(string)
		late, _ := info["late"].(string)
		switch r.Response.ActionIdentifier {
		case "join":
			if meet != "" {
				openURL(meet)
				return
			}
		case "snooze":
			if key != "" {
				a.mu.Lock()
				a.snoozed[key] = time.Now().Add(10 * time.Minute)
				a.mu.Unlock()
				return
			}
		case "late":
			if strings.HasPrefix(late, "mailto:") {
				openURL(late)
				return
			}
		}
		a.showPanel()
	})

	go a.loop()
	go a.updateLoop()
	go a.alertLoop()
	go a.travelLoop()
	if err := a.wa.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *App) buildMenu() *application.Menu {
	// Apple's order: About · updates · settings, then the app's own items, then Help and Quit.
	m := a.wa.NewMenu()
	m.Add("About Smart Cal").OnClick(func(*application.Context) { a.showAboutPanel() })
	m.Add("Check for Updates…").OnClick(func(*application.Context) { a.showSettings("about-check") })
	m.AddSeparator()
	m.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.showSettings("") })
	m.AddSeparator()
	m.Add("Open Smart Cal").OnClick(func(*application.Context) { a.showPanel() })
	m.Add("Open Week View").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) { a.showWeek() })
	m.Add("New Event…").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) {
		a.showPanel()
		a.wa.Event.Emit("ui:new")
	})
	m.Add("Refresh").SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { a.RequestRefresh() })
	m.AddSeparator()
	m.Add("Open Calendar App").OnClick(func(*application.Context) { openURL("ical://") })
	m.Add("Open Google Calendar (web)").OnClick(func(*application.Context) { openURL("https://calendar.google.com") })
	m.AddSeparator()
	m.Add("Smart Cal Help").OnClick(func(*application.Context) {
		a.showPanel()
		a.wa.Event.Emit("ui:help")
	})
	m.AddSeparator()
	m.Add("Quit Smart Cal").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.wa.Quit() })
	return m
}

// buildAppMenu is Smart Cal's own menu bar. Without it Wails installs a generic default whose
// About isn't ours and whose Help item loads wails.io inside our window. The Edit menu also makes
// ⌘X/⌘C/⌘V/⌘A work in text fields. Shortcuts the panel already handles in JS (⌘N, ⌘R, ⌘1–4…)
// are shown here too; the page handles them first, so they never fire twice.
// showPanel opens the panel under the menu bar item — or, when macOS is hiding the item
// (full menu bar, behind the notch), in the middle of the screen so it can still be reached.
func (a *App) showPanel() {
	go func() {
		if a.checkTray() {
			a.tray.ShowWindow()
			return
		}
		application.InvokeSync(func() {
			a.win.Center()
			a.win.Show().Focus()
		})
	}()
}

func (a *App) togglePanel() {
	if a.win.IsVisible() {
		a.win.Hide()
		return
	}
	a.showPanel()
}

// checkTray re-reads whether the menu bar item is visible, tells the panel when that changes,
// and returns true if it is (or can't be told).
func (a *App) checkTray() bool {
	hidden := trayVisible() == 0
	a.mu.Lock()
	changed := hidden != a.trayHidden
	a.trayHidden = hidden
	a.mu.Unlock()
	if changed {
		a.wa.Event.Emit("state", a.state())
	}
	return !hidden
}

// UpdatedInfo: Smart Cal was just updated (shown once in the panel: "✓ Updated to vX").
type UpdatedInfo struct {
	From string `json:"from"` // "" when the previous version didn't record itself
	To   string `json:"to"`
}

// noteUpdate compares this version with the one that last ran here. Older versions don't record
// themselves, so an existing install with no record also counts as "just updated" (once).
func (a *App) noteUpdate(hadConfig bool) {
	if version == "0.0.0" { // a development build
		return
	}
	prev := a.cfg.LastVersion
	if prev == version {
		return
	}
	if (prev != "" && newerVersion(version, prev)) || (prev == "" && hadConfig) {
		a.justUpdated = &UpdatedInfo{From: prev, To: version}
	}
	a.cfg.LastVersion = version
	_ = a.cfg.Save()
}

func (a *App) justUpdatedInfo() *UpdatedInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.justUpdated
}

func (a *App) buildAppMenu() *application.Menu {
	m := a.wa.NewMenu()

	app := m.AddSubmenu("Smart Cal")
	app.Add("About Smart Cal").OnClick(func(*application.Context) { a.showAboutPanel() })
	app.Add("Check for Updates…").OnClick(func(*application.Context) { a.showSettings("about-check") })
	app.AddSeparator()
	app.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.showSettings("") })
	app.AddSeparator()
	app.AddRole(application.ServicesMenu)
	app.AddSeparator()
	app.AddRole(application.Hide)
	app.AddRole(application.HideOthers)
	app.AddRole(application.UnHide)
	app.AddSeparator()
	app.Add("Quit Smart Cal").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.wa.Quit() })

	file := m.AddSubmenu("File")
	file.Add("New Event…").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) {
		a.showPanel()
		a.wa.Event.Emit("ui:new")
	})
	file.Add("Open Week View").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) { a.showWeek() })
	file.AddSeparator()
	file.AddRole(application.CloseWindow)

	m.AddRole(application.EditMenu)

	view := m.AddSubmenu("View")
	for i, v := range []string{"Year", "Month", "Week", "Day"} {
		key := strings.ToLower(v)
		view.Add(v).SetAccelerator(fmt.Sprintf("CmdOrCtrl+%d", i+1)).OnClick(func(*application.Context) {
			a.showPanel()
			a.wa.Event.Emit("ui:view", key)
		})
	}
	view.AddSeparator()
	view.Add("Go to Today").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) {
		a.showPanel()
		a.wa.Event.Emit("ui:today")
	})
	view.Add("Refresh").SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { a.RequestRefresh() })

	win := m.AddSubmenu("Window")
	win.AddRole(application.Minimise)
	win.AddRole(application.Zoom)
	win.AddSeparator()
	win.AddRole(application.BringAllToFront)

	help := m.AddSubmenu("Help")
	help.Add("Smart Cal Help").OnClick(func(*application.Context) {
		a.showPanel()
		a.wa.Event.Emit("ui:help")
	})
	help.Add("What's New").OnClick(func(*application.Context) {
		openURL("https://github.com/" + releaseRepo + "/blob/main/CHANGELOG.md")
	})
	return m
}

func (a *App) loop() {
	time.Sleep(time.Second)
	if renamed := renameBundle(); renamed && a.cfg.LaunchAtLogin {
		_ = a.wa.Autostart.Disable()
		_ = a.wa.Autostart.Enable() // re-register the login item at the new path
	}
	if !a.cfg.LoginConfigured { // first run: don't add a login item silently — the panel asks
		a.cfg.LoginConfigured = true
		_ = a.cfg.Save()
	}
	if a.cfg.ShowInDock {
		setDockVisible(true)
	}
	if a.cal.Access() == "notDetermined" {
		a.cal.RequestAccess() // first launch: macOS asks "allow access to Calendars?"
	}
	a.doRefresh()
	trayAutosave() // remember where the menu bar item sits from now on
	time.Sleep(time.Second)
	if !a.cal.Connected() { // first run: show the panel with its one-click Allow banner
		a.showPanel()
	} else if a.justUpdatedInfo() != nil {
		// just updated: drop the panel down so it's clear the new version is running and all is well
		a.showPanel()
	} else if !a.checkTray() {
		// macOS is hiding the menu bar item (full menu bar / behind the notch): open the panel so
		// Smart Cal doesn't seem to vanish; it explains how to get the item back
		a.showPanel()
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
			a.checkTray()
		}
	}
}

// alertLoop posts a notification shortly before each timed meeting (with Join when there's a link).
func (a *App) alertLoop() {
	time.Sleep(3 * time.Second)
	if ok, err := a.notif.RequestNotificationAuthorization(); err == nil && ok {
		a.notifOK = true
		// alerts you can act on: Join (online meetings), Snooze 10 min, Running late (emails the others)
		join := notifications.NotificationAction{ID: "join", Title: "Join"}
		snooze := notifications.NotificationAction{ID: "snooze", Title: "Snooze 10 min"}
		late := notifications.NotificationAction{ID: "late", Title: "Running late…"}
		for id, acts := range map[string][]notifications.NotificationAction{
			"ev-join-late": {join, snooze, late}, "ev-join": {join, snooze}, "ev-late": {snooze, late}, "ev": {snooze},
		} {
			_ = a.notif.RegisterNotificationCategory(notifications.NotificationCategory{ID: id, Actions: acts})
		}
	}
	t := time.NewTicker(15 * time.Second)
	for range t.C {
		a.checkAlerts()
		a.checkSnoozed()
		a.checkBrief()
	}
}

// sendEventAlert posts the alert for one event with the actions that fit it.
// It returns false when notifications aren't allowed.
func (a *App) sendEventAlert(e Event, key, body string) bool {
	if !a.notifOK {
		return false
	}
	late := ""
	if det, err := a.cal.Detail(e.ID, e.Start); err == nil {
		late = lateMailto(det)
	}
	cat := "ev"
	switch {
	case e.MeetLink != "" && late != "":
		cat = "ev-join-late"
	case e.MeetLink != "":
		cat = "ev-join"
	case late != "":
		cat = "ev-late"
	}
	opts := notifications.NotificationOptions{
		ID: "meet-" + key + "-" + strconv.FormatInt(time.Now().Unix(), 10), Title: e.Title, Body: body, CategoryID: cat,
		Data: map[string]interface{}{"meet": e.MeetLink, "eventId": e.ID, "key": key, "late": late},
	}
	return a.notif.SendNotificationWithActions(opts) == nil
}

// lateMailto drafts "running late" to the other attendees ("" when there's nobody to tell).
func lateMailto(d EventDetail) string {
	var to []string
	for _, p := range d.Attendees {
		if !p.Me && strings.Contains(p.Email, "@") {
			to = append(to, p.Email)
		}
	}
	if len(to) == 0 {
		return ""
	}
	q := func(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
	subject := "Running a few minutes late — " + d.Title
	body := "Hi,\n\nI'm running about 10 minutes late for \"" + d.Title + "\". Sorry — I'll be there as soon as I can.\n"
	return "mailto:" + strings.Join(to, ",") + "?subject=" + q(subject) + "&body=" + q(body)
}

// checkSnoozed re-sends alerts whose 10-minute snooze is up.
func (a *App) checkSnoozed() {
	now := time.Now()
	a.mu.Lock()
	due := []string{}
	for k, t := range a.snoozed {
		if !now.Before(t) {
			due = append(due, k)
			delete(a.snoozed, k)
		}
	}
	a.mu.Unlock()
	if len(due) == 0 {
		return
	}
	evs, _ := a.snapshot()
	for _, k := range due {
		for _, e := range evs {
			if e.ID+e.Start.Format(time.RFC3339) != k || now.After(e.End) {
				continue
			}
			when := "Started " + shortDur(now.Sub(e.Start)) + " ago"
			if e.Start.After(now) {
				when = "In " + shortDur(e.Start.Sub(now))
			}
			if !a.sendEventAlert(e, k, "Snoozed · "+when+" · "+a.fmtTime(e.Start)+"–"+a.fmtTime(e.End)) {
				a.showPanel()
			}
		}
	}
}

// checkBrief sends the morning brief once a day at the chosen time: "3 events · first 9:00
// Dentist · leave by 8:35 · 1 to-do overdue". Skipped when the day is empty, or if the Mac
// was asleep until more than four hours after the time.
func (a *App) checkBrief() {
	bt := a.cfg.BriefTime
	if bt == "off" || bt == "" || !a.notifOK || !a.cal.Connected() {
		return
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	if a.cfg.BriefLast == today {
		return
	}
	t, err := time.Parse("15:04", bt)
	if err != nil {
		return
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if now.Before(at) {
		return
	}
	a.cfg.BriefLast = today
	_ = a.cfg.Save()
	if now.After(at.Add(4 * time.Hour)) {
		return
	}
	title, body := a.briefText(now)
	if body == "" {
		return
	}
	_ = a.notif.SendNotification(notifications.NotificationOptions{ID: "brief-" + today, Title: title, Body: body})
}

func (a *App) briefText(now time.Time) (string, string) {
	evs, _ := a.snapshot()
	sod := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	eod := sod.AddDate(0, 0, 1)
	var timed []Event
	allDay := 0
	for _, e := range evs {
		if e.Holiday || !e.Start.Before(eod) || !e.End.After(sod) {
			continue
		}
		if e.AllDay {
			allDay++
		} else {
			timed = append(timed, e)
		}
	}
	var parts []string
	if n := len(timed) + allDay; n > 0 {
		parts = append(parts, fmt.Sprintf("%d event%s", n, map[bool]string{true: "", false: "s"}[n == 1]))
	}
	var first *Event
	for i := range timed {
		if timed[i].Start.After(now) && (first == nil || timed[i].Start.Before(first.Start)) {
			first = &timed[i]
		}
	}
	if first != nil {
		parts = append(parts, "first "+a.fmtTime(first.Start)+" "+truncate(first.Title, 30))
		a.mu.Lock()
		mins := a.travel[first.ID+"|"+strconv.FormatInt(first.Start.Unix(), 10)]
		a.mu.Unlock()
		if mins > 0 {
			parts = append(parts, "leave by "+a.fmtTime(first.Start.Add(-time.Duration(mins+5)*time.Minute)))
		}
	}
	overdue, dueToday := 0, 0
	a.mu.Lock()
	todos := a.extras.Todos
	a.mu.Unlock()
	for _, td := range todos {
		if td.Done || td.Due == nil {
			continue
		}
		if td.Due.Before(sod) || (td.HasTime && td.Due.Before(now)) {
			overdue++
		} else if td.Due.Before(eod) {
			dueToday++
		}
	}
	if dueToday > 0 {
		parts = append(parts, fmt.Sprintf("%d to-do%s today", dueToday, map[bool]string{true: "", false: "s"}[dueToday == 1]))
	}
	if overdue > 0 {
		parts = append(parts, fmt.Sprintf("%d to-do%s overdue", overdue, map[bool]string{true: "", false: "s"}[overdue == 1]))
	}
	if len(parts) == 0 {
		return "", ""
	}
	return "Good morning — " + now.Format("Monday 2 January"), strings.Join(parts, " · ")
}

func (a *App) checkAlerts() {
	mode := a.cfg.AlertMode
	if mode == "off" {
		return
	}
	lead, _ := strconv.Atoi(mode)
	evs, _ := a.snapshot()
	now := time.Now()
	if len(a.alerted) > 500 { // keep the "already alerted" memory small: drop events no longer upcoming
		keep := map[string]bool{}
		for _, e := range evs {
			if k := e.ID + e.Start.Format(time.RFC3339); a.alerted[k] {
				keep[k] = true
			}
		}
		a.alerted = keep
	}
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
		if !a.sendEventAlert(e, key, body) { // notifications not allowed: drop the panel down instead
			a.showPanel()
		}
		a.wa.Event.Emit("alert", map[string]any{"title": e.Title, "body": body, "meet": e.MeetLink,
			"id": e.ID, "start": e.Start})
	}
}

// toggleQuick shows the quick-add box in the middle of the screen (or hides it).
func (a *App) toggleQuick() {
	if a.quick.IsVisible() {
		a.quick.Hide()
		return
	}
	a.quick.Center()
	a.quick.Show().Focus()
	appActivate()
	a.wa.Event.Emit("quick:shown")
}

// setQuickHotkey swaps the quick-add shortcut ("off" = none). On failure the old one is kept.
func (a *App) setQuickHotkey(accel string) error {
	old := a.quickOn
	if old != "" {
		_ = a.wa.GlobalShortcut.Unregister(old)
		a.quickOn = ""
	}
	if accel != "off" {
		if accel == a.hotkeyOn {
			return fmt.Errorf("that shortcut already opens the panel")
		}
		if err := a.wa.GlobalShortcut.Register(accel, func() { a.toggleQuick() }); err != nil {
			if old != "" && a.wa.GlobalShortcut.Register(old, func() { a.toggleQuick() }) == nil {
				a.quickOn = old
			}
			return fmt.Errorf("couldn't use that shortcut (another app may have it): %v", err)
		}
		a.quickOn = accel
	}
	a.cfg.QuickHotkey = accel
	return nil
}

// setHotkey swaps the global shortcut ("off" = none). On failure the old one is kept.
func (a *App) setHotkey(accel string) error {
	old := a.hotkeyOn
	if old != "" {
		_ = a.wa.GlobalShortcut.Unregister(old)
		a.hotkeyOn = ""
	}
	if accel != "off" {
		if err := a.wa.GlobalShortcut.Register(accel, func() { a.togglePanel() }); err != nil {
			if old != "" && a.wa.GlobalShortcut.Register(old, func() { a.togglePanel() }) == nil {
				a.hotkeyOn = old
			}
			return fmt.Errorf("couldn't use that shortcut (another app may have it): %v", err)
		}
		a.hotkeyOn = accel
	}
	a.cfg.Hotkey = accel
	// (⌘⇧W for the week window is handled inside Smart Cal only — registering it globally
	// would steal "Close Window" from Safari, Chrome, Finder…)
	return nil
}

// applyTextSize scales the panel (the page zooms itself; the window grows to match).
// panelSize: the drop-down grows to fit the AI box + calendar + list, up to the screen's height.
func (a *App) panelSize() (int, int) {
	z := float64(a.cfg.TextSize) / 100
	base := 380
	if a.cfg.Layout == "wide" {
		base = 760 // month on the left, the selected day on the right
	}
	w, h := int(float64(base)*z), int(880*z)
	if a.wa.Screen != nil {
		if sc := a.wa.Screen.GetPrimary(); sc != nil && sc.WorkArea.Height > 0 {
			if max := sc.WorkArea.Height - 20; h > max {
				h = max
			}
			if max := sc.WorkArea.Width - 40; w > max && max > 380 {
				w = max
			}
		}
	}
	if h < 560 {
		h = 560
	}
	return w, h
}

func (a *App) applyTextSize() {
	a.win.SetSize(a.panelSize())
	a.wa.Event.Emit("ui:textsize", a.cfg.TextSize)
}

// showSettings opens the Settings window on a tab ("" = last one).
func (a *App) showSettings(tab string) {
	a.win.Hide()
	appActivate()
	a.settings.Show()
	a.settings.Focus()
	a.wa.Event.Emit("settings:open", tab)
}

func (a *App) showAboutPanel() {
	a.win.Hide()
	showAbout(version, "Smart Cal — your calendar in the menu bar, with AI quick add.\nfrom go.sala.company")
}

// rememberFrame restores a window's last size and position and saves it when it changes.
func (a *App) rememberFrame(w *application.WebviewWindow, slot **Rect) {
	if r := *slot; r != nil && r.W > 300 && r.H > 300 && a.onScreen(r) {
		w.SetBounds(application.Rect{X: r.X, Y: r.Y, Width: r.W, Height: r.H})
	}
	var (
		tmu sync.Mutex
		t   *time.Timer
	)
	save := func(*application.WindowEvent) {
		tmu.Lock()
		defer tmu.Unlock()
		if t != nil {
			t.Stop()
		}
		t = time.AfterFunc(800*time.Millisecond, func() {
			b := w.Bounds()
			if b.Width > 300 && b.Height > 300 {
				cfgMu.Lock()
				*slot = &Rect{X: b.X, Y: b.Y, W: b.Width, H: b.Height}
				cfgMu.Unlock()
				_ = a.cfg.Save()
			}
		})
	}
	w.OnWindowEvent(events.Common.WindowDidMove, save)
	w.OnWindowEvent(events.Common.WindowDidResize, save)
}

func (a *App) showWeek() {
	a.week.Show()
	a.week.Focus()
	a.wa.Event.Emit("week:shown")
}

func (a *App) clearTravel() {
	a.mu.Lock()
	a.travel, a.travelAt = map[string]int{}, map[string]time.Time{}
	a.mu.Unlock()
}

// travelLoop asks Apple Maps for the driving time to upcoming events with a place and
// sends a "Leave now" notification when it's time to go.
func (a *App) travelLoop() {
	time.Sleep(20 * time.Second)
	for {
		a.checkTravel()
		time.Sleep(3 * time.Minute)
	}
}

func (a *App) checkTravel() {
	from := strings.TrimSpace(a.cfg.TravelFrom)
	if from == "" || !a.cal.Connected() {
		return
	}
	evs, _ := a.snapshot()
	now := time.Now()
	for _, e := range evs {
		if e.AllDay || e.Holiday || strings.TrimSpace(e.Location) == "" || strings.HasPrefix(e.Location, "http") {
			continue
		}
		if e.Start.Before(now) || e.Start.Sub(now) > 4*time.Hour {
			continue
		}
		key := e.ID + "|" + strconv.FormatInt(e.Start.Unix(), 10)
		a.mu.Lock()
		last, mins := a.travelAt[key], a.travel[key]
		a.mu.Unlock()
		if time.Since(last) > 10*time.Minute {
			d := a.cal.driveTime(from, e.ID, e.Start)
			if d < 0 { // unknown place (e.g. "Zoom"): remember the miss so it isn't geocoded every 3 minutes
				a.mu.Lock()
				a.travelAt[key] = time.Now()
				a.mu.Unlock()
				continue
			}
			mins = int(d.Round(time.Minute).Minutes())
			a.mu.Lock()
			a.travel[key], a.travelAt[key] = mins, time.Now()
			a.mu.Unlock()
			a.wa.Event.Emit("state", a.state())
		}
		if mins <= 0 || a.leaveAlerted[key] { // mins is 0 for a cached miss too
			continue
		}
		leaveAt := e.Start.Add(-time.Duration(mins+5) * time.Minute) // 5 min to get to the car
		if now.After(leaveAt) {
			a.leaveAlerted[key] = true
			body := fmt.Sprintf("~%d min drive to %s · starts %s", mins, e.Location, a.fmtTime(e.Start))
			sent := false
			if a.notifOK {
				sent = a.notif.SendNotification(notifications.NotificationOptions{
					ID: "leave-" + key, Title: "🚗 Leave now for " + e.Title, Body: body,
				}) == nil
			}
			if !sent {
				a.showPanel()
			}
			a.wa.Event.Emit("alert", map[string]any{"title": "🚗 Leave now for " + e.Title, "body": body, "id": e.ID, "start": e.Start})
		}
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
	info, err := checkLatest(ctx, a.cfg.BetaUpdates)
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
	sysCache = sysPrefs()
	evs, err := a.cal.Upcoming(a.cfg, a.cfg.DaysAhead)
	ex := extras{Weather: a.weather(), Countdowns: a.countdowns()}
	if !a.cfg.NoTodos {
		ex.Todos, _ = a.cal.Reminders()
	}
	a.mu.Lock()
	a.searchAt = time.Time{} // events may have changed: next search refetches
	if err == nil {
		a.events = evs
	}
	a.err = err
	a.extras = ex
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
	a.setTray(a.barLabel(evs, now), evs, now)
}

// barLabel builds the menu bar text for the chosen style and fine-tune settings.
// Styles: compact "👥 2h Meet" · minimal "👥 2h" · countdown "2h Meet friends" · time "10:00 Meet friends" ·
// full "Tomorrow 10:00 · Meet friends" · today / lasthour (compact, limited) · date "Sat 7 Nov" ·
// datecompact "Sat 7 · 👥 2h Meet" · icon "".
func (a *App) barLabel(evs []Event, now time.Time) string {
	c := a.cfg
	switch c.MenuBarStyle {
	case "icon":
		return ""
	case "date":
		return now.Format("Mon 2 Jan")
	case "full":
		return a.fullLabel(evs, now)
	}
	part := ""
	if e, ok := nextEvent(evs, now); ok && a.withinAhead(e, now) {
		if !(c.BarHideInMeeting && !e.Start.After(now)) {
			part = a.composeLabel(e, now)
		}
	}
	if c.MenuBarStyle == "datecompact" {
		if part == "" {
			return now.Format("Mon 2 Jan")
		}
		return now.Format("Mon 2") + " · " + part
	}
	return part
}

func (a *App) withinAhead(e Event, now time.Time) bool {
	ahead := a.cfg.BarAhead
	switch a.cfg.MenuBarStyle { // these styles carry their own limit
	case "today":
		ahead = "today"
	case "lasthour":
		ahead = "hour"
	}
	switch ahead {
	case "hour":
		return e.Start.Sub(now) <= time.Hour
	case "today":
		return !e.Start.After(now) || sameDay(e.Start, now)
	}
	return true
}

func (a *App) composeLabel(e Event, now time.Time) string {
	c := a.cfg
	title, timeMode, emoji := c.BarTitle, c.BarTime, !c.BarNoEmoji
	switch c.MenuBarStyle { // the style decides what the parts are
	case "minimal":
		title = "none"
	case "countdown":
		emoji, timeMode = false, "countdown"
		if title == "word" {
			title = "full"
		}
	case "time":
		emoji, timeMode = false, "clock"
		if title == "word" {
			title = "full"
		}
	}
	var parts []string
	if emoji {
		a.mu.Lock()
		em := a.labelEmoji[e.Title]
		a.mu.Unlock()
		if em == "" {
			em = "📅"
		}
		parts = append(parts, em)
	}
	parts = append(parts, a.barWhen(e, now, timeMode))
	switch title {
	case "word":
		if w := shortWord(e.Title); w != "" {
			parts = append(parts, w)
		}
	case "short":
		parts = append(parts, truncate(e.Title, 15))
	case "full":
		parts = append(parts, truncate(e.Title, 30))
	}
	return strings.Join(parts, " ")
}

// barWhen: "now", "25m", "2h½", "5h" (countdown) or "10a"/"10:00" (clock); other days "Su 10a".
func (a *App) barWhen(e Event, now time.Time, mode string) string {
	d := e.Start.Sub(now)
	clock := compactClock(e.Start)
	if a.cfg.is24h() {
		clock = e.Start.Format("15:04")
	}
	switch {
	case d <= 0:
		return "now"
	case !sameDay(e.Start, now) && d >= 12*time.Hour:
		return e.Start.Format("Mon")[:2] + " " + clock
	case mode == "clock":
		return clock
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Round(time.Minute).Minutes()))
	case d < 3*time.Hour && int(d.Minutes())%60 >= 30:
		return fmt.Sprintf("%dh½", int(d.Hours()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// fullLabel is the original long style.
func (a *App) fullLabel(evs []Event, now time.Time) string {
	label := now.Format("Mon 2 Jan")
	e, ok := nextEvent(evs, now)
	if !ok {
		return label
	}
	t := truncate(e.Title, 22)
	switch {
	case !e.Start.After(now):
		if a.cfg.BarHideInMeeting {
			return ""
		}
		return fmt.Sprintf("● %s · %s left", t, shortDur(e.End.Sub(now)))
	case e.Start.Sub(now) <= time.Hour:
		return fmt.Sprintf("%s in %s", t, shortDur(e.Start.Sub(now)))
	case sameDay(e.Start, now):
		return fmt.Sprintf("%s · %s", a.fmtTime(e.Start), t)
	case e.Start.Sub(now) < 7*24*time.Hour:
		day := e.Start.Format("Mon")
		if sameDay(e.Start, now.AddDate(0, 0, 1)) {
			day = "Tomorrow"
		}
		return fmt.Sprintf("%s %s · %s", day, a.fmtTime(e.Start), t)
	}
	return label
}

// setTray shows the label with no icon (saves menu bar space); the icon comes back only when
// there's no text to show. In the 5 minutes before a meeting the label gets an orange dot
// (or, icon-only, the icon gets its orange badge).
func (a *App) setTray(label string, evs []Event, now time.Time) {
	a.trayMu.Lock() // called from the refresh loop and from settings/panel calls
	defer a.trayMu.Unlock()
	if a.updating != "" { // an update is installing: say so instead of the next event
		a.tray.SetLabel(a.updating)
		trayShowImage(false)
		return
	}
	soon := a.meetingSoon(evs, now)
	if label != "" && soon {
		label = "🟠 " + label
	}
	a.tray.SetLabel(label)
	a.badge(soon && label == "")
	trayShowImage(label == "")
}

// beginUpdate marks an update as running (menu bar: "⬇︎ Updating…"); false if one already is.
func (a *App) beginUpdate() bool {
	a.trayMu.Lock()
	defer a.trayMu.Unlock()
	if a.updating != "" {
		return false
	}
	a.updating = "⬇︎ Updating…"
	a.tray.SetLabel(a.updating)
	trayShowImage(false)
	return true
}

// setUpdating shows text in the menu bar while an update installs ("" = back to normal).
func (a *App) setUpdating(text string) {
	a.trayMu.Lock()
	a.updating = text
	if text != "" {
		a.tray.SetLabel(text)
		trayShowImage(false)
	}
	a.trayMu.Unlock()
	if text == "" {
		a.updateLabel()
	}
}

func (a *App) meetingSoon(evs []Event, now time.Time) bool {
	mins, err := strconv.Atoi(a.cfg.BarDot)
	if err != nil || mins <= 0 {
		return false
	}
	if e, ok := nextEvent(evs, now); ok {
		d := e.Start.Sub(now)
		return d > 0 && d <= time.Duration(mins)*time.Minute
	}
	return false
}

// badge the menu bar icon (orange dot) when a meeting is about to start
func (a *App) badge(soon bool) {
	key := fmt.Sprintf("%v|%s", soon, a.cfg.TrayIcon)
	if key == a.iconKey {
		return
	}
	a.iconKey = key
	a.badged = soon
	mono := a.cfg.TrayIcon == "mono"
	img := trayIcon
	switch {
	case mono && soon:
		img = trayIconMonoAlert
	case mono:
		img = trayIconMono
	case soon:
		img = trayIconAlert
	}
	traySetImage(img, mono) // monochrome = template image: black/white like Apple's own icons
}

func (a *App) fmtTime(t time.Time) string {
	if a.cfg.is24h() {
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

// compactClock: 10:00 → "10a", 13:30 → "1:30p".
func compactClock(t time.Time) string {
	h, m := t.Hour(), t.Minute()
	ap := "a"
	if h >= 12 {
		ap = "p"
	}
	if h = h % 12; h == 0 {
		h = 12
	}
	if m == 0 {
		return fmt.Sprintf("%d%s", h, ap)
	}
	return fmt.Sprintf("%d:%02d%s", h, m, ap)
}

// shortWord picks the first meaningful word of a title, at most 8 characters.
func shortWord(title string) string {
	skip := map[string]bool{"the": true, "a": true, "an": true, "my": true, "with": true, "to": true, "at": true, "and": true, "&": true, "-": true, "·": true}
	ws := strings.Fields(title)
	for i, w := range ws {
		w = strings.Trim(w, ".,:;!?()[]\"'")
		if len(w) == 2 && strings.ToUpper(w) == w && i+1 < len(ws) && isDigits(ws[i+1]) && len(ws[i+1]) <= 4 {
			return w + ws[i+1] // flight number: "KU 101" → "KU101"
		}
		if w == "" || skip[strings.ToLower(w)] {
			continue
		}
		r := []rune(w)
		if r[0] > 0x2000 && len(r) <= 2 { // a leading emoji
			continue
		}
		if len(r) > 8 {
			return string(r[:7]) + "…"
		}
		return w
	}
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// onScreen: a saved frame is only restored if it still overlaps a connected display.
func (a *App) onScreen(r *Rect) bool {
	if a.wa.Screen == nil {
		return true
	}
	for _, sc := range a.wa.Screen.GetAll() {
		b := sc.Bounds
		if r.X < b.X+b.Width && r.X+r.W > b.X && r.Y < b.Y+b.Height && r.Y+r.H > b.Y {
			return true
		}
	}
	return false
}
