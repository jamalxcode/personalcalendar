package main

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	wa     *application.App
	tray   *application.SystemTray
	win    *application.WebviewWindow
	cfg    *Config
	cal    *Cal
	mu     sync.Mutex
	events []Event
	err    error
	kick   chan struct{}
}

func main() {
	// build.sh uses this to produce the .icns for the bundle.
	if len(os.Args) == 3 && os.Args[1] == "--write-icon" {
		if err := os.WriteFile(os.Args[2], appIconPNG(), 0o644); err != nil {
			log.Fatal(err)
		}
		return
	}

	a := &App{cfg: loadConfig(), cal: &Cal{}, kick: make(chan struct{}, 1)}
	a.cal.Init()
	storeChanged = a.RequestRefresh // macOS tells us when any calendar changes

	assets, _ := fs.Sub(frontendFS, "frontend")
	a.wa = application.New(application.Options{
		Name:        "Personal Calendar",
		Description: "Menu bar calendar with AI quick add",
		Services:    []application.Service{application.NewService(&CalService{a: a})},
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
		Height:          620,
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
	a.tray.SetTemplateIcon(trayIconPNG())
	a.tray.SetTooltip("Personal Calendar  (" + hotkey + ")")
	a.tray.SetMenu(a.buildMenu())
	a.tray.AttachWindow(a.win).WindowOffset(6)

	if err := a.wa.GlobalShortcut.Register(hotkey, func() { a.tray.ToggleWindow() }); err != nil {
		log.Printf("hotkey %s not registered: %v", hotkey, err)
	}

	go a.loop()
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
	m.Add("Quit Personal Calendar").OnClick(func(*application.Context) { a.wa.Quit() })
	return m
}

func (a *App) loop() {
	time.Sleep(time.Second)
	if a.cal.Access() == "notDetermined" {
		a.cal.RequestAccess() // first launch: macOS asks "allow access to Calendars?"
	}
	a.doRefresh()
	if !a.cal.Connected() || a.cfg.OpenAIKey == "" {
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
	type calLine struct{ Name, Account, Source string; Google, Writable, Primary bool }
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
	label := now.Format("Mon 2 Jan")
	if e, ok := nextEvent(evs, now); ok {
		t := truncate(e.Title, 22)
		switch {
		case !e.Start.After(now):
			label = fmt.Sprintf("● %s · %s left", t, shortDur(e.End.Sub(now)))
		case e.Start.Sub(now) <= time.Hour:
			label = fmt.Sprintf("%s in %s", t, shortDur(e.Start.Sub(now)))
		case sameDay(e.Start, now):
			label = fmt.Sprintf("%s · %s", a.fmtTime(e.Start), t)
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
