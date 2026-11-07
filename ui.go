package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type MainUI struct {
	a        *App
	w        fyne.Window
	dayTitle *canvas.Text
	dateSub  *canvas.Text
	input    *widget.Entry
	status   *canvas.Text
	preview  *fyne.Container
	hero     *fyne.Container
	list     *fyne.Container
	scroll   *container.Scroll
	drafts   []Draft
	busy     bool
	settings fyne.Window
}

func newMainUI(a *App) *MainUI {
	u := &MainUI{a: a}
	w := a.fa.NewWindow("Personal Calendar")
	u.w = w
	w.SetCloseIntercept(func() { w.Hide() })
	w.SetFixedSize(true)

	u.dayTitle = canvas.NewText("", theme.ForegroundColor())
	u.dayTitle.TextSize = 22
	u.dayTitle.TextStyle.Bold = true
	u.dateSub = canvas.NewText("", accent)
	u.dateSub.TextSize = 13
	u.dateSub.TextStyle.Bold = true

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { u.setStatus("Refreshing…"); a.RequestRefresh() })
	refreshBtn.Importance = widget.LowImportance
	gearBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), u.openSettings)
	gearBtn.Importance = widget.LowImportance
	header := container.NewBorder(nil, nil, nil, container.NewHBox(refreshBtn, gearBtn),
		container.NewVBox(u.dateSub, u.dayTitle))

	u.input = widget.NewEntry()
	u.input.SetPlaceHolder("Add event…  e.g. Lunch with Ali tomorrow 1pm at Avenues")
	u.input.OnSubmitted = func(s string) { u.submit(s) }

	u.status = canvas.NewText("", mutedText())
	u.status.TextSize = 11

	u.preview = container.NewVBox()
	u.hero = container.NewVBox()
	u.list = container.NewVBox()
	u.scroll = container.NewVScroll(u.list)

	top := container.NewVBox(header, u.input, u.status, u.preview, u.hero)
	w.SetContent(container.NewPadded(container.NewBorder(top, nil, nil, nil, u.scroll)))
	w.Resize(fyne.NewSize(400, 640))

	// ⌘N focuses quick-add, Esc hides window, ⌘R refreshes, ⌘, opens settings
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { w.Canvas().Focus(u.input) })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { a.RequestRefresh() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { u.openSettings() })
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyEscape {
			if len(u.drafts) > 0 {
				u.clearDrafts()
			} else {
				w.Hide()
			}
		}
	})
	u.render()
	return u
}

func (u *MainUI) Show() {
	u.render()
	u.w.Show()
	u.w.RequestFocus()
	u.w.Canvas().Focus(u.input)
}

func (u *MainUI) setStatus(s string) {
	u.status.Text = s
	u.status.Color = mutedText()
	u.status.Refresh()
}

func (u *MainUI) setError(err error) {
	u.status.Text = "⚠︎ " + err.Error()
	u.status.Color = accent
	u.status.Refresh()
}

// ---------- quick add ----------

func (u *MainUI) submit(text string) {
	text = strings.TrimSpace(text)
	if u.busy {
		return
	}
	// Enter on an empty box with a pending preview = confirm.
	if text == "" {
		if len(u.drafts) > 0 {
			u.addDrafts()
		}
		return
	}
	u.busy = true
	u.setStatus("✨ Understanding…")
	go func() {
		defer func() { u.busy = false }()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()
		cals := u.a.cal.CachedCalendars()
		drafts, err := parseEvents(ctx, u.a.cfg, text, cals)
		if err != nil {
			u.setError(err)
			return
		}
		for i := range drafts {
			if drafts[i].CalendarID == "" {
				drafts[i].CalendarID = u.a.cfg.DefaultCalendar
			}
		}
		u.drafts = drafts
		if !u.a.cfg.ConfirmBeforeAdd {
			u.input.SetText("")
			u.busy = false
			u.addDrafts()
			return
		}
		u.input.SetText("")
		u.setStatus("Press Return to add · Esc to cancel")
		u.renderPreview()
	}()
}

func (u *MainUI) clearDrafts() {
	u.drafts = nil
	u.preview.Objects = nil
	u.preview.Refresh()
	u.setStatus("")
}

func (u *MainUI) addDrafts() {
	if u.busy || len(u.drafts) == 0 {
		return
	}
	u.busy = true
	drafts := u.drafts
	u.setStatus("Adding to Google Calendar…")
	go func() {
		defer func() { u.busy = false }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, d := range drafts {
			if _, err := u.a.cal.Insert(ctx, d.CalendarID, d); err != nil {
				u.setError(err)
				return
			}
		}
		n := len(drafts)
		u.clearDrafts()
		if n == 1 {
			u.setStatus("✓ Added “" + truncate(drafts[0].Title, 40) + "”")
		} else {
			u.setStatus(fmt.Sprintf("✓ Added %d events", n))
		}
		u.a.fa.SendNotification(fyne.NewNotification("Event added", u.describe(drafts[0])))
		u.a.RequestRefresh()
	}()
}

func (u *MainUI) describe(d Draft) string {
	s := d.Title + " — " + u.whenText(d.Start, d.End, d.AllDay)
	if d.Location != "" {
		s += " · " + d.Location
	}
	return s
}

func (u *MainUI) whenText(s, e time.Time, allDay bool) string {
	now := time.Now()
	day := dayLabel(s, now)
	if day == "Today" || day == "Tomorrow" || s.Sub(now) < 6*24*time.Hour {
		day += ", " + s.Format("2 Jan")
	}
	if allDay {
		days := int(e.Sub(s).Hours()/24 + 0.5)
		if days > 1 {
			return fmt.Sprintf("%s · all day (%d days)", day, days)
		}
		return day + " · all day"
	}
	return fmt.Sprintf("%s · %s – %s", day, u.a.fmtTime(s), u.a.fmtTime(e))
}

func (u *MainUI) renderPreview() {
	var objs []fyne.CanvasObject
	cals := u.a.cal.CachedCalendars()
	var names []string
	for _, c := range cals {
		if c.Writable {
			names = append(names, c.Name)
		}
	}
	for i := range u.drafts {
		i := i
		d := u.drafts[i]
		title := canvas.NewText(d.Title, theme.ForegroundColor())
		title.TextStyle.Bold = true
		title.TextSize = 15
		when := canvas.NewText(u.whenText(d.Start, d.End, d.AllDay), accent)
		when.TextSize = 12
		lines := []fyne.CanvasObject{title, when}
		if d.Location != "" {
			loc := canvas.NewText("📍 "+d.Location, mutedText())
			loc.TextSize = 12
			lines = append(lines, loc)
		}
		if d.ReminderMinutes > 0 {
			r := canvas.NewText(fmt.Sprintf("🔔 %s before", shortDur(time.Duration(d.ReminderMinutes)*time.Minute)), mutedText())
			r.TextSize = 12
			lines = append(lines, r)
		}
		if len(names) > 1 {
			sel := widget.NewSelect(names, func(name string) {
				for _, c := range cals {
					if c.Name == name {
						u.drafts[i].CalendarID = c.ID
					}
				}
			})
			cur := ""
			for _, c := range cals {
				if (d.CalendarID == "" && c.Primary) || c.ID == d.CalendarID {
					cur = c.Name
				}
			}
			sel.SetSelected(cur)
			lines = append(lines, sel)
		}
		bg := canvas.NewRectangle(heroBG())
		bg.CornerRadius = 10
		objs = append(objs, container.NewStack(bg, container.NewPadded(container.NewVBox(lines...))))
	}
	add := widget.NewButtonWithIcon("Add", theme.ConfirmIcon(), u.addDrafts)
	add.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", u.clearDrafts)
	objs = append(objs, container.NewGridWithColumns(2, cancel, add))
	u.preview.Objects = objs
	u.preview.Refresh()
}

// ---------- event list ----------

func (u *MainUI) render() {
	now := time.Now()
	u.dayTitle.Text = now.Format("Monday")
	u.dateSub.Text = strings.ToUpper(now.Format("2 January 2006"))
	u.dayTitle.Color = theme.ForegroundColor()
	u.dayTitle.Refresh()
	u.dateSub.Refresh()

	evs, err := u.a.Events()
	if err != nil {
		u.setError(err)
	} else if strings.HasPrefix(u.status.Text, "⚠︎") || u.status.Text == "Refreshing…" {
		u.setStatus("")
	}
	u.renderHero(evs, now)

	var objs []fyne.CanvasObject
	lastDay := ""
	for _, e := range evs {
		day := dayLabel(e.Start, now)
		if day != lastDay {
			h := canvas.NewText(strings.ToUpper(day), mutedText())
			h.TextSize = 11
			h.TextStyle.Bold = true
			if lastDay != "" {
				objs = append(objs, layout.NewSpacer())
			}
			objs = append(objs, container.NewPadded(h))
			lastDay = day
		}
		objs = append(objs, u.eventRow(e, now))
	}
	if len(evs) == 0 && err == nil {
		t := canvas.NewText("Nothing on the calendar for the next "+fmt.Sprint(u.a.cfg.DaysAhead)+" days 🎉", mutedText())
		t.Alignment = fyne.TextAlignCenter
		objs = append(objs, layout.NewSpacer(), t)
	}
	u.list.Objects = objs
	u.list.Refresh()
}

func (u *MainUI) tickNext() {
	evs, _ := u.a.Events()
	u.renderHero(evs, time.Now())
}

func (u *MainUI) renderHero(evs []Event, now time.Time) {
	e, ok := nextEvent(evs, now)
	if !ok {
		u.hero.Objects = nil
		u.hero.Refresh()
		return
	}
	label := "UP NEXT · in " + shortDur(e.Start.Sub(now))
	if !e.Start.After(now) {
		label = "NOW · " + shortDur(e.End.Sub(now)) + " left"
	} else if e.Start.Sub(now) > 24*time.Hour {
		label = "UP NEXT · " + dayLabel(e.Start, now)
	}
	l := canvas.NewText(label, accent)
	l.TextSize = 11
	l.TextStyle.Bold = true
	t := canvas.NewText(truncate(e.Title, 38), theme.ForegroundColor())
	t.TextSize = 17
	t.TextStyle.Bold = true
	sub := u.a.fmtTime(e.Start) + " – " + u.a.fmtTime(e.End)
	if e.Location != "" {
		sub += "  ·  " + truncate(e.Location, 30)
	}
	s := canvas.NewText(sub, mutedText())
	s.TextSize = 12
	var right fyne.CanvasObject
	if e.MeetLink != "" {
		link := e.MeetLink
		j := widget.NewButtonWithIcon("Join", theme.MediaVideoIcon(), func() { openURL(link) })
		j.Importance = widget.HighImportance
		right = container.NewCenter(j)
	}
	bg := canvas.NewRectangle(heroBG())
	bg.CornerRadius = 12
	link := e.Link
	body := newTapArea(container.NewVBox(l, t, s), func() { openURL(link) })
	u.hero.Objects = []fyne.CanvasObject{container.NewStack(bg, container.NewPadded(container.NewBorder(nil, nil, nil, right, body)))}
	u.hero.Refresh()
}

func (u *MainUI) eventRow(e Event, now time.Time) fyne.CanvasObject {
	dot := canvas.NewCircle(hexColor(e.Color))
	dotBox := container.NewGridWrap(fyne.NewSize(10, 10), dot)

	when := "all-day"
	if !e.AllDay {
		when = u.a.fmtTime(e.Start)
	}
	tm := canvas.NewText(when, mutedText())
	tm.TextSize = 12
	timeCol := container.NewGridWrap(fyne.NewSize(62, 18), tm)

	title := canvas.NewText(truncate(e.Title, 34), theme.ForegroundColor())
	title.TextSize = 13
	if !e.AllDay && !e.Start.After(now) {
		title.TextStyle.Bold = true
	}
	lines := []fyne.CanvasObject{title}
	sub := ""
	if e.Location != "" {
		sub = truncate(e.Location, 40)
	}
	if !e.AllDay {
		d := e.End.Sub(e.Start)
		if sub != "" {
			sub = shortDur(d) + " · " + sub
		} else {
			sub = shortDur(d) + " · " + e.Calendar
		}
	}
	if sub != "" {
		st := canvas.NewText(sub, mutedText())
		st.TextSize = 11
		lines = append(lines, st)
	}

	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { u.confirmDelete(e) })
	del.Importance = widget.LowImportance

	left := container.NewHBox(container.NewCenter(dotBox), container.NewCenter(timeCol))
	link := e.Link
	body := newTapArea(container.NewBorder(nil, nil, left, nil, container.NewVBox(lines...)), func() { openURL(link) })
	bg := canvas.NewRectangle(cardBG())
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewBorder(nil, nil, nil, container.NewCenter(del), container.NewPadded(body)))
}

func (u *MainUI) confirmDelete(e Event) {
	dialog.ShowConfirm("Delete event?", fmt.Sprintf("“%s”\n%s\n\nThis removes it from Google Calendar.", e.Title, u.whenText(e.Start, e.End, e.AllDay)),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				if err := u.a.cal.Delete(ctx, e.CalendarID, e.ID); err != nil {
					u.setError(err)
					return
				}
				u.setStatus("🗑 Deleted “" + truncate(e.Title, 40) + "”")
				u.a.RequestRefresh()
			}()
		}, u.w)
}

// ---------- settings ----------

func (u *MainUI) openSettings() {
	if u.settings != nil {
		u.settings.Show()
		u.settings.RequestFocus()
		return
	}
	a := u.a
	w := a.fa.NewWindow("Personal Calendar — Settings")
	u.settings = w
	w.SetOnClosed(func() { u.settings = nil })

	key := widget.NewPasswordEntry()
	key.SetText(a.cfg.OpenAIKey)
	key.SetPlaceHolder("sk-…")
	model := widget.NewEntry()
	model.SetText(a.cfg.OpenAIModel)
	h24 := widget.NewCheck("24-hour clock", nil)
	h24.SetChecked(a.cfg.Use24h)
	confirm := widget.NewCheck("Show preview before adding", nil)
	confirm.SetChecked(a.cfg.ConfirmBeforeAdd)
	days := widget.NewSelect([]string{"7", "14", "30", "60"}, nil)
	days.SetSelected(fmt.Sprint(a.cfg.DaysAhead))
	if days.Selected == "" {
		days.SetSelected("14")
	}

	gStatus := widget.NewLabel("")
	calsBox := container.NewVBox()
	defSel := widget.NewSelect(nil, nil)
	var calChecks map[string]*widget.Check
	var cals []CalendarInfo

	loadCals := func() {
		calsBox.Objects = nil
		calChecks = map[string]*widget.Check{}
		if !a.cal.Connected() {
			gStatus.SetText("Not connected")
			calsBox.Refresh()
			return
		}
		gStatus.SetText("Connected ✓ — loading calendars…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cs, err := a.cal.Calendars(ctx)
			if err != nil {
				gStatus.SetText("Connected, but: " + err.Error())
				return
			}
			cals = cs
			gStatus.SetText("Connected ✓")
			var writable []string
			defName := ""
			for _, c := range cs {
				ck := widget.NewCheck(c.Name, nil)
				ck.SetChecked(!a.cfg.IsHidden(c.ID))
				calChecks[c.ID] = ck
				sw := canvas.NewCircle(hexColor(c.Color))
				calsBox.Add(container.NewHBox(container.NewCenter(container.NewGridWrap(fyne.NewSize(10, 10), sw)), ck))
				if c.Writable {
					writable = append(writable, c.Name)
					if c.ID == a.cfg.DefaultCalendar || (a.cfg.DefaultCalendar == "" && c.Primary) {
						defName = c.Name
					}
				}
			}
			defSel.Options = writable
			defSel.SetSelected(defName)
			calsBox.Refresh()
		}()
	}

	importBtn := widget.NewButtonWithIcon("Import credentials.json…", theme.FolderOpenIcon(), func() {
		path, err := chooseFile()
		if err != nil || path == "" {
			return
		}
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "client_id") {
			dialog.ShowError(fmt.Errorf("that doesn't look like a Google OAuth client file"), w)
			return
		}
		if err := os.WriteFile(credentialsPath(), b, 0o600); err != nil {
			dialog.ShowError(err, w)
			return
		}
		gStatus.SetText("Credentials imported — now click Connect Google")
	})
	connectBtn := widget.NewButtonWithIcon("Connect Google", theme.LoginIcon(), func() {
		gStatus.SetText("Waiting for you in the browser…")
		go func() {
			if err := a.cal.Login(context.Background()); err != nil {
				gStatus.SetText("✗ " + err.Error())
				return
			}
			loadCals()
			a.RequestRefresh()
		}()
	})
	connectBtn.Importance = widget.HighImportance
	disconnectBtn := widget.NewButton("Disconnect", func() {
		a.cal.Disconnect()
		loadCals()
		a.RequestRefresh()
	})
	help := widget.NewHyperlink("How do I get credentials.json?", nil)
	help.OnTapped = func() { openURL("https://github.com/jamalxcode/personalcalendar#google-setup") }
	keyHelp := widget.NewHyperlink("Get an OpenAI API key", nil)
	keyHelp.OnTapped = func() { openURL("https://platform.openai.com/api-keys") }

	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		a.cfg.OpenAIKey = strings.TrimSpace(key.Text)
		a.cfg.OpenAIModel = strings.TrimSpace(model.Text)
		a.cfg.Use24h = h24.Checked
		a.cfg.ConfirmBeforeAdd = confirm.Checked
		fmt.Sscan(days.Selected, &a.cfg.DaysAhead)
		if calChecks != nil && len(cals) > 0 {
			a.cfg.HiddenCalendars = nil
			for id, ck := range calChecks {
				if !ck.Checked {
					a.cfg.HiddenCalendars = append(a.cfg.HiddenCalendars, id)
				}
			}
			for _, c := range cals {
				if c.Name == defSel.Selected {
					a.cfg.DefaultCalendar = c.ID
				}
			}
		}
		if err := a.cfg.Save(); err != nil {
			dialog.ShowError(err, w)
			return
		}
		a.RequestRefresh()
		w.Close()
	})
	save.Importance = widget.HighImportance

	form := container.NewVBox(
		widget.NewRichTextFromMarkdown("### OpenAI"),
		widget.NewForm(widget.NewFormItem("API key", key), widget.NewFormItem("Model", model)),
		keyHelp,
		widget.NewSeparator(),
		widget.NewRichTextFromMarkdown("### Google Calendar"),
		gStatus,
		container.NewGridWithColumns(2, importBtn, connectBtn),
		container.NewHBox(help, layout.NewSpacer(), disconnectBtn),
		widget.NewLabel("Show these calendars:"),
		calsBox,
		widget.NewForm(widget.NewFormItem("New events go to", defSel)),
		widget.NewSeparator(),
		widget.NewRichTextFromMarkdown("### Display"),
		h24, confirm,
		widget.NewForm(widget.NewFormItem("Days ahead", days)),
	)
	w.SetContent(container.NewBorder(nil, container.NewPadded(container.NewHBox(layout.NewSpacer(), save)), nil, nil,
		container.NewVScroll(container.NewPadded(form))))
	w.Resize(fyne.NewSize(460, 640))
	loadCals()
	w.Show()
	w.RequestFocus()
}

// chooseFile uses the native macOS file picker via AppleScript.
func chooseFile() (string, error) {
	out, err := exec.Command("osascript", "-e",
		`POSIX path of (choose file with prompt "Choose the Google OAuth credentials.json" of type {"public.json"})`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ---------- small widgets ----------

type tapArea struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
}

func newTapArea(c fyne.CanvasObject, f func()) *tapArea {
	t := &tapArea{content: c, onTap: f}
	t.ExtendBaseWidget(t)
	return t
}
func (t *tapArea) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(t.content) }
func (t *tapArea) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}
