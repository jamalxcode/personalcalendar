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
	week         *application.WebviewWindow
	travel       map[string]int // "id|unix" → driving minutes (guarded by mu)
	travelAt     map[string]time.Time
	labelEmoji   map[string]string // title → emoji, sent by the panel
	leaveAlerted map[string]bool
	hotkeyOn     string // currently registered shortcut
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

	a := &App{cfg: loadConfig(), cal: &Cal{}, kick: make(chan struct{}, 1), alerted: map[string]bool{},
		travel: map[string]int{}, travelAt: map[string]time.Time{}, leaveAlerted: map[string]bool{}}
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
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "company.sala.personalcalendar"},
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
	a.wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.rememberFrame(a.week, &a.cfg.WeekBounds)
		a.rememberFrame(a.settings, &a.cfg.SetBounds)
	})

	// clicking the Dock icon (when shown) opens the panel
	a.wa.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		a.tray.ShowWindow()
	})
	// size the drop-down to the screen once the app (and its screen list) is up
	a.wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.applyTextSize()
	})

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
	m.Add("Open Smart Cal").OnClick(func(*application.Context) { a.tray.ShowWindow() })
	m.Add("Open Week View").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) { a.showWeek() })
	m.Add("New Event…").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) {
		a.tray.ShowWindow()
		a.wa.Event.Emit("ui:new")
	})
	m.Add("Refresh").SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { a.RequestRefresh() })
	m.AddSeparator()
	m.Add("Open Calendar App").OnClick(func(*application.Context) { openURL("ical://") })
	m.Add("Open Google Calendar (web)").OnClick(func(*application.Context) { openURL("https://calendar.google.com") })
	m.AddSeparator()
	m.Add("Smart Cal Help").OnClick(func(*application.Context) {
		a.tray.ShowWindow()
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
		a.tray.ShowWindow()
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
			a.tray.ShowWindow()
			a.wa.Event.Emit("ui:view", key)
		})
	}
	view.AddSeparator()
	view.Add("Go to Today").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) {
		a.tray.ShowWindow()
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
		a.tray.ShowWindow()
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
	if !a.cal.Connected() { // first run: show the panel with its one-click Allow banner
		a.tray.ShowWindow()
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

// setHotkey swaps the global shortcut ("off" = none). On failure the old one is kept.
func (a *App) setHotkey(accel string) error {
	old := a.hotkeyOn
	if old != "" {
		_ = a.wa.GlobalShortcut.Unregister(old)
		a.hotkeyOn = ""
	}
	if accel != "off" {
		if err := a.wa.GlobalShortcut.Register(accel, func() { a.tray.ToggleWindow() }); err != nil {
			if old != "" && a.wa.GlobalShortcut.Register(old, func() { a.tray.ToggleWindow() }) == nil {
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
	w, h := int(380*z), int(880*z)
	if a.wa.Screen != nil {
		if sc := a.wa.Screen.GetPrimary(); sc != nil && sc.WorkArea.Height > 0 {
			if max := sc.WorkArea.Height - 20; h > max {
				h = max
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
				a.tray.ShowWindow()
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
