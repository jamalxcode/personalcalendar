package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

var errNotConnected = errors.New("Google Calendar is not connected — open Settings")

// Event is the app's flattened view of a Google Calendar event.
type Event struct {
	ID         string    `json:"id"`
	CalendarID string    `json:"calendarId"`
	Calendar   string    `json:"calendar"`
	Color      string    `json:"color"`
	Title      string    `json:"title"`
	Location   string    `json:"location"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	AllDay     bool      `json:"allDay"`
	Link       string    `json:"link"`
	MeetLink   string    `json:"meetLink"`
}

type CalendarInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Primary  bool   `json:"primary"`
	Writable bool   `json:"writable"`
}

type GCal struct {
	mu   sync.Mutex
	svc  *calendar.Service
	cals []CalendarInfo
}

func oauthConfig() (*oauth2.Config, error) {
	b, err := os.ReadFile(credentialsPath())
	if err != nil {
		return nil, fmt.Errorf("no credentials.json imported yet")
	}
	return google.ConfigFromJSON(b, calendar.CalendarEventsScope, calendar.CalendarReadonlyScope)
}

// savingSource persists refreshed tokens so we never have to log in again.
type savingSource struct {
	src  oauth2.TokenSource
	last string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		if b, e := json.Marshal(t); e == nil {
			_ = os.WriteFile(tokenPath(), b, 0o600)
		}
	}
	return t, nil
}

// Init tries to build a service from saved credentials + token. Returns errNotConnected if not set up.
func (g *GCal) Init() error {
	conf, err := oauthConfig()
	if err != nil {
		return errNotConnected
	}
	b, err := os.ReadFile(tokenPath())
	if err != nil {
		return errNotConnected
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return errNotConnected
	}
	return g.build(conf, &tok)
}

func (g *GCal) build(conf *oauth2.Config, tok *oauth2.Token) error {
	ctx := context.Background()
	ts := &savingSource{src: conf.TokenSource(ctx, tok)}
	svc, err := calendar.NewService(ctx, option.WithTokenSource(oauth2.ReuseTokenSource(tok, ts)))
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.svc = svc
	g.mu.Unlock()
	return nil
}

func (g *GCal) Connected() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.svc != nil
}

func (g *GCal) Disconnect() {
	g.mu.Lock()
	g.svc = nil
	g.cals = nil
	g.mu.Unlock()
	_ = os.Remove(tokenPath())
}

// Login runs the desktop OAuth loopback flow in the user's browser.
func (g *GCal) Login(ctx context.Context) error {
	conf, err := oauthConfig()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	conf.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	sb := make([]byte, 16)
	_, _ = rand.Read(sb)
	state := hex.EncodeToString(sb)
	verifier := oauth2.GenerateVerifier()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "<h2>Sign-in cancelled: %s</h2>", e)
			errCh <- fmt.Errorf("google sign-in: %s", e)
			return
		}
		fmt.Fprint(w, `<html><body style="font-family:-apple-system;text-align:center;padding-top:80px">
<h2>✅ Personal Calendar is connected</h2><p>You can close this tab.</p></body></html>`)
		codeCh <- q.Get("code")
	})}
	go srv.Serve(ln)
	defer srv.Close()

	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	_ = exec.Command("open", url).Start()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for Google sign-in")
	}
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return err
	}
	b, _ := json.Marshal(tok)
	if err := os.WriteFile(tokenPath(), b, 0o600); err != nil {
		return err
	}
	return g.build(conf, tok)
}

func (g *GCal) service() (*calendar.Service, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.svc == nil {
		return nil, errNotConnected
	}
	return g.svc, nil
}

func (g *GCal) Calendars(ctx context.Context) ([]CalendarInfo, error) {
	svc, err := g.service()
	if err != nil {
		return nil, err
	}
	list, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []CalendarInfo
	for _, c := range list.Items {
		name := c.SummaryOverride
		if name == "" {
			name = c.Summary
		}
		out = append(out, CalendarInfo{
			ID: c.Id, Name: name, Color: c.BackgroundColor, Primary: c.Primary,
			Writable: c.AccessRole == "owner" || c.AccessRole == "writer",
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Primary && !out[j].Primary })
	g.mu.Lock()
	g.cals = out
	g.mu.Unlock()
	return out, nil
}

func (g *GCal) CachedCalendars() []CalendarInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]CalendarInfo(nil), g.cals...)
}

// Upcoming returns events from now (including ones in progress) through `days` ahead, across visible calendars.
func (g *GCal) Upcoming(ctx context.Context, cfg *Config, days int) ([]Event, error) {
	svc, err := g.service()
	if err != nil {
		return nil, err
	}
	cals, err := g.Calendars(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	tMax := dayStart.AddDate(0, 0, days)

	var (
		mu     sync.Mutex
		out    []Event
		wg     sync.WaitGroup
		firstE error
	)
	for _, c := range cals {
		if cfg.IsHidden(c.ID) {
			continue
		}
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.Events.List(c.ID).Context(ctx).
				TimeMin(now.Format(time.RFC3339)).TimeMax(tMax.Format(time.RFC3339)).
				SingleEvents(true).OrderBy("startTime").MaxResults(100).Do()
			if err != nil {
				mu.Lock()
				if firstE == nil && c.Primary {
					firstE = err
				}
				mu.Unlock()
				return
			}
			for _, it := range res.Items {
				if it.Status == "cancelled" || declined(it) {
					continue
				}
				ev, ok := toEvent(it, c)
				if !ok {
					continue
				}
				mu.Lock()
				out = append(out, ev)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstE != nil && len(out) == 0 {
		return nil, firstE
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].AllDay && !out[j].AllDay
	})
	return out, nil
}

func declined(it *calendar.Event) bool {
	for _, a := range it.Attendees {
		if a.Self && a.ResponseStatus == "declined" {
			return true
		}
	}
	return false
}

func toEvent(it *calendar.Event, c CalendarInfo) (Event, bool) {
	ev := Event{ID: it.Id, CalendarID: c.ID, Calendar: c.Name, Color: c.Color,
		Title: it.Summary, Location: it.Location, Link: it.HtmlLink, MeetLink: it.HangoutLink}
	if ev.Title == "" {
		ev.Title = "(No title)"
	}
	if it.Start == nil || it.End == nil {
		return ev, false
	}
	if it.Start.Date != "" {
		ev.AllDay = true
		s, err1 := time.ParseInLocation("2006-01-02", it.Start.Date, time.Local)
		e, err2 := time.ParseInLocation("2006-01-02", it.End.Date, time.Local)
		if err1 != nil || err2 != nil {
			return ev, false
		}
		ev.Start, ev.End = s, e
	} else {
		s, err1 := time.Parse(time.RFC3339, it.Start.DateTime)
		e, err2 := time.Parse(time.RFC3339, it.End.DateTime)
		if err1 != nil || err2 != nil {
			return ev, false
		}
		ev.Start, ev.End = s.Local(), e.Local()
	}
	return ev, true
}

// Insert creates an event from a parsed draft. Returns the created event's web link.
func (g *GCal) Insert(ctx context.Context, calID string, d Draft) (string, error) {
	svc, err := g.service()
	if err != nil {
		return "", err
	}
	if calID == "" {
		calID = "primary"
	}
	ev := &calendar.Event{Summary: d.Title, Location: d.Location, Description: d.Notes}
	if d.AllDay {
		end := d.End
		if !end.After(d.Start) {
			end = d.Start.AddDate(0, 0, 1)
		}
		ev.Start = &calendar.EventDateTime{Date: d.Start.Format("2006-01-02")}
		ev.End = &calendar.EventDateTime{Date: end.Format("2006-01-02")}
	} else {
		tz := time.Local.String()
		if tz == "Local" || tz == "" {
			tz = "Asia/Kuwait"
		}
		ev.Start = &calendar.EventDateTime{DateTime: d.Start.Format(time.RFC3339), TimeZone: tz}
		ev.End = &calendar.EventDateTime{DateTime: d.End.Format(time.RFC3339), TimeZone: tz}
	}
	if d.ReminderMinutes > 0 {
		ev.Reminders = &calendar.EventReminders{
			UseDefault:      false,
			Overrides:       []*calendar.EventReminder{{Method: "popup", Minutes: int64(d.ReminderMinutes)}},
			ForceSendFields: []string{"UseDefault"},
		}
	}
	created, err := svc.Events.Insert(calID, ev).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.HtmlLink, nil
}

func (g *GCal) Delete(ctx context.Context, calID, eventID string) error {
	svc, err := g.service()
	if err != nil {
		return err
	}
	return svc.Events.Delete(calID, eventID).Context(ctx).Do()
}
