package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ======================= Apple Intelligence (via the Swift helper) =======================

// helperApp is Smart Cal.app/Contents/Helpers/Smart Cal Assistant.app (empty if missing).
func helperApp() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	app := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Helpers", "Smart Cal Assistant.app")
	if _, err := os.Stat(app); err != nil {
		return ""
	}
	return app
}

func helperBin() string {
	if app := helperApp(); app != "" {
		return filepath.Join(app, "Contents", "MacOS", "Smart Cal Assistant")
	}
	return ""
}

var (
	appleMu     sync.Mutex
	appleStatus string // "available", "not-enabled", "device-not-eligible", "not-ready", "needs-macos-26", "missing"
	appleAt     time.Time
)

// appleAI returns Apple Intelligence's status (cached for 5 minutes; "missing" = no helper in this build).
func appleAI() string {
	appleMu.Lock()
	st, at := appleStatus, appleAt
	appleMu.Unlock()
	if st != "" {
		if time.Since(at) > 5*time.Minute {
			appleMu.Lock()
			appleAt = time.Now() // one background re-check at a time
			appleMu.Unlock()
			go checkApple()
		}
		return st
	}
	return checkApple()
}

func checkApple() string {
	appleMu.Lock()
	defer appleMu.Unlock()
	bin := helperBin()
	if bin == "" {
		appleStatus, appleAt = "missing", time.Now()
		return appleStatus
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--fm-status").Output()
	st := strings.TrimSpace(string(out))
	if err != nil || st == "" {
		st = "unavailable"
	}
	appleStatus, appleAt = st, time.Now()
	return st
}

func appleAvailable() bool { return appleAI() == "available" }

// callApple runs one request on the on-device model. schema is the same JSON Schema sent to OpenRouter.
func callApple(ctx context.Context, sys, user string, schema map[string]any) ([]byte, error) {
	bin := helperBin()
	if bin == "" {
		return nil, fmt.Errorf("Apple Intelligence helper missing")
	}
	in, _ := json.Marshal(map[string]any{"instructions": sys, "prompt": user, "schema": schema})
	cmd := exec.CommandContext(ctx, bin, "--fm")
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("Apple Intelligence: %v", err)
	}
	var r struct {
		JSON  string `json:"json"`
		Error string `json:"error"`
	}
	if json.Unmarshal(out, &r) != nil {
		return nil, fmt.Errorf("Apple Intelligence gave an unexpected answer")
	}
	if r.Error != "" {
		return nil, fmt.Errorf("Apple Intelligence: %s", r.Error)
	}
	return []byte(r.JSON), nil
}

// pickEngine decides who answers: "apple" (on-device), "remote" (OpenRouter/OpenAI) or "" (no AI).
// Auto: Apple Intelligence for everyday short requests (free, private, offline); OpenRouter for
// screenshots and long pasted text when a key is set.
func pickEngine(cfg *Config, user, imageB64 string) string {
	hasKey := strings.TrimSpace(cfg.OpenAIKey) != ""
	switch cfg.AIEngine {
	case "apple":
		if appleAvailable() && imageB64 == "" {
			return "apple"
		}
		if hasKey {
			return "remote"
		}
		return ""
	case "openrouter":
		if hasKey {
			return "remote"
		}
		if imageB64 == "" && appleAvailable() {
			return "apple"
		}
		return ""
	}
	if imageB64 == "" && (!hasKey || len([]rune(user)) <= 400) && appleAvailable() {
		return "apple"
	}
	if hasKey {
		return "remote"
	}
	return ""
}

// registerHelper makes sure macOS knows the helper (so its Siri/Shortcuts actions show up).
func registerHelper() {
	app := helperApp()
	if app == "" {
		return
	}
	ls := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	_ = exec.Command(ls, "-f", app).Run()
}

// ======================= Siri / Shortcuts bridge =======================

// startAssistant listens on a private Unix socket for the helper's Siri/Shortcuts requests.
func (a *App) startAssistant(svc *CalService) {
	path := filepath.Join(appDir(), "assistant.sock")
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return
	}
	_ = os.Chmod(path, 0o600)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go a.serveAssistant(svc, c)
		}
	}()
}

func (a *App) serveAssistant(svc *CalService, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(90 * time.Second))
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	var req struct{ Cmd, Text string }
	_ = json.Unmarshal(line, &req)
	reply, err := a.assistantDo(svc, req.Cmd, strings.TrimSpace(req.Text))
	resp := map[string]string{"reply": reply}
	if err != nil {
		resp["error"] = err.Error()
	}
	b, _ := json.Marshal(resp)
	_, _ = c.Write(append(b, '\n'))
}

// assistantDo carries out a Siri/Shortcuts request and returns what Siri should say.
func (a *App) assistantDo(svc *CalService, cmd, text string) (string, error) {
	if !a.cal.Connected() {
		return "", fmt.Errorf("Smart Cal needs calendar access first — open it and click Allow")
	}
	if cmd == "next" {
		return a.nextSentence(), nil
	}
	if text == "" {
		return "", fmt.Errorf("tell me what to add or ask")
	}
	res, err := svc.AskWith(text, "")
	if err != nil {
		return "", err
	}
	switch res.Kind {
	case "add":
		if len(res.Drafts) == 0 {
			return orDefault(res.Answer, "I couldn't find an event in that."), nil
		}
		if _, err := svc.Add(res.Drafts); err != nil {
			return "", err
		}
		d := res.Drafts[0]
		msg := fmt.Sprintf("Added %s, %s.", d.Title, a.spokenWhen(d.Start, d.AllDay))
		if n := len(res.Drafts) - 1; n > 0 {
			msg = fmt.Sprintf("Added %s and %d more.", d.Title, n)
		}
		if len(res.Clashes) > 0 && len(res.Clashes[0]) > 0 {
			msg += " Heads up: it overlaps " + res.Clashes[0][0].Title + "."
		}
		return msg, nil
	case "update":
		if res.Target == nil || res.Draft == nil {
			return orDefault(res.Answer, "I couldn't tell which event to change."), nil
		}
		if _, err := svc.UpdateEvent(res.Target.ID, res.Target.Start, *res.Draft, res.AllFuture); err != nil {
			return "", err
		}
		return fmt.Sprintf("Done — %s is now %s.", res.Draft.Title, a.spokenWhen(res.Draft.Start, res.Draft.AllDay)), nil
	case "delete":
		// deleting by voice can't be undone, so it stays in the app
		if res.Target != nil {
			return fmt.Sprintf("To delete %s, open Smart Cal — I don't delete events by voice.", res.Target.Title), nil
		}
		return "To delete an event, open Smart Cal.", nil
	case "slots":
		if len(res.Slots) == 0 {
			return orDefault(res.Answer, "I couldn't find a free slot."), nil
		}
		var parts []string
		for i, s := range res.Slots {
			if i == 3 {
				break
			}
			parts = append(parts, a.spokenWhen(s.Start, false))
		}
		return "You're free " + joinAndWords(parts) + ". Open Smart Cal to book one.", nil
	case "todo":
		if len(res.Todos) == 0 {
			return orDefault(res.Answer, "I couldn't find a to-do in that."), nil
		}
		if _, err := svc.AddTodos(res.Todos); err != nil {
			return "", err
		}
		return "Added to your to-dos: " + res.Todos[0].Title + ".", nil
	}
	return orDefault(res.Answer, "Done."), nil
}

func (a *App) nextSentence() string {
	evs, _ := a.snapshot()
	e, ok := nextEvent(evs, time.Now())
	if !ok {
		return "Nothing coming up. Enjoy the free time."
	}
	if !e.Start.After(time.Now()) {
		return fmt.Sprintf("%s is on now, until %s.", e.Title, a.fmtTime(e.End))
	}
	s := fmt.Sprintf("Next: %s, %s", e.Title, a.spokenWhen(e.Start, false))
	if e.Location != "" {
		s += " at " + e.Location
	}
	return s + "."
}

// spokenWhen: "today at 3:00 PM", "tomorrow at 10:00", "on Monday 9 November at 9:00", "on Friday" (all-day).
func (a *App) spokenWhen(t time.Time, allDay bool) string {
	now := time.Now()
	var day string
	switch {
	case sameDay(t, now):
		day = "today"
	case sameDay(t, now.AddDate(0, 0, 1)):
		day = "tomorrow"
	case t.Sub(now) < 6*24*time.Hour:
		day = "on " + t.Format("Monday")
	default:
		day = "on " + t.Format("Monday 2 January")
	}
	if allDay {
		return day
	}
	return day + " at " + a.fmtTime(t)
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func joinAndWords(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " or " + xs[len(xs)-1]
}
