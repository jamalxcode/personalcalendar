package main

import (
	"testing"
	"time"
)

// The AI edit safeguards: relative moves are done in code, fields the user didn't mention stay put.

func TestShiftAndCheck(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, kw)                                 // Tue 10 Nov
	ev := Event{Title: "Dentist", Start: time.Date(2026, 11, 20, 15, 0, 0, 0, kw)} // Fri 20 Nov 15:00
	ev.End = ev.Start.Add(time.Hour)
	cases := []struct {
		text string
		want string // new start, "2006-01-02 15:04"
	}{
		{"push the dentist back by a week", "2026-11-27 15:00"},
		{"push it a week back", "2026-11-27 15:00"},
		{"push it by a week", "2026-11-27 15:00"},
		{"postpone dentist by 3 days", "2026-11-23 15:00"},
		{"delay it by 1 week", "2026-11-27 15:00"},
		{"move my back massage by 2 days", "2026-11-22 15:00"},
		{"move it by 2 days earlier", "2026-11-18 15:00"},
		{"move it up by 2 days", "2026-11-18 15:00"},
		{"bring it forward by 2 days", "2026-11-18 15:00"},
		{"move it 2 days earlier", "2026-11-18 15:00"},
		{"2 weeks later", "2026-12-04 15:00"},
		// the v2.1.1 report: "1 month from now" = 10 Dec at the event's usual time, not October
		{"move this to 1 month from now", "2026-12-10 15:00"},
		{"30 days from today", "2026-12-10 15:00"},
	}
	for _, c := range cases {
		d := Draft{Title: ev.Title, Start: ev.Start, End: ev.End}
		if err := shiftAndCheck(&d, ev, c.text, now); err != nil {
			t.Errorf("%q: unexpected error %v", c.text, err)
			continue
		}
		if got := d.Start.Format("2006-01-02 15:04"); got != c.want {
			t.Errorf("%q: start %s, want %s", c.text, got, c.want)
		}
		if d.End.Sub(d.Start) != time.Hour {
			t.Errorf("%q: length changed to %v", c.text, d.End.Sub(d.Start))
		}
	}
}

func TestShiftRefusesThePast(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, kw)
	ev := Event{Title: "Dentist", Start: time.Date(2026, 11, 12, 15, 0, 0, 0, kw)}
	ev.End = ev.Start.Add(time.Hour)
	d := Draft{Start: time.Date(2026, 10, 12, 15, 0, 0, 0, kw), End: time.Date(2026, 10, 12, 16, 0, 0, 0, kw)}
	if err := shiftAndCheck(&d, ev, "move it to the 12th", now); err == nil {
		t.Fatal("a silent move into the past should be refused")
	}
	// "push back" used to count as permission to go into the past
	d = Draft{Start: ev.Start, End: ev.End}
	if err := shiftAndCheck(&d, ev, "push it back by a week", now); err != nil || !d.Start.After(ev.Start) {
		t.Fatalf("push back should move later: %v %v", d.Start, err)
	}
}

func TestShiftAllDayKeepsWholeDays(t *testing.T) {
	// a zone with a DST change inside the move: the end must still be midnight
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, ny)
	ev := Event{Title: "Trip", AllDay: true, Start: time.Date(2026, 10, 31, 0, 0, 0, 0, ny)}
	ev.End = time.Date(2026, 11, 2, 0, 0, 0, 0, ny) // Sat–Sun, crosses the 1 Nov change (49 h)
	d := Draft{Start: ev.Start, End: ev.End, AllDay: true}
	if err := shiftAndCheck(&d, ev, "push it by a week", now); err != nil {
		t.Fatal(err)
	}
	if d.End.Hour() != 0 || d.End.Sub(d.Start) < 47*time.Hour {
		t.Errorf("all-day end %v (start %v)", d.End, d.Start)
	}
}

func TestGuardChanges(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	ev := Event{Title: "Gym", Location: "Home", Start: time.Date(2026, 11, 20, 7, 0, 0, 0, kw)}
	ev.End = ev.Start.Add(time.Hour)
	noisy := aiEvent{Title: "Workout", Location: "Work", Calendar: "Work", AllDay: true, Notes: "x", TimeZone: "Asia/Dubai",
		End: "2026-11-20T09:00:00+03:00", Repeat: aiRepeat{Freq: "monthly"}}

	g := guardChanges(ev, noisy, nil, "move gym to 8am")
	if g.Title != "" || g.Location != "" || g.Calendar != "" || g.AllDay || g.Notes != "" || g.TimeZone != "" || g.End != "" || g.Repeat.Freq != "keep" {
		t.Errorf("unmentioned fields should be dropped, got %+v", g)
	}
	g = guardChanges(ev, noisy, nil, "rename it to Workout and make it every week")
	if g.Title != "Workout" || g.Repeat.Freq != "monthly" {
		t.Errorf("mentioned fields should stay, got %+v", g)
	}
	// Arabic: "repeat it every week" and "change the name"
	g = guardChanges(ev, noisy, nil, "كرر النادي كل أسبوع")
	if g.Repeat.Freq != "monthly" {
		t.Errorf("Arabic repeat request was dropped: %+v", g.Repeat)
	}
	g = guardChanges(ev, noisy, nil, "غيّر الاسم إلى Workout")
	if g.Title != "Workout" {
		t.Errorf("Arabic rename was dropped: %q", g.Title)
	}
	g = guardChanges(ev, noisy, nil, "انقل النادي للساعة ٨")
	if g.Title != "" || g.Repeat.Freq != "keep" {
		t.Errorf("Arabic move shouldn't rename or repeat: %+v", g)
	}
}

func TestApplyChangesKeepsLength(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	ev := Event{Title: "Lunch", Start: time.Date(2026, 11, 20, 12, 0, 0, 0, kw), CalendarID: "c1"}
	ev.End = ev.Start.Add(90 * time.Minute)
	d, err := applyChanges(ev, aiEvent{Start: "2026-11-20T15:00:00+03:00", Repeat: aiRepeat{Freq: "keep"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Lunch" || d.CalendarID != "c1" || d.SetRepeat || d.End.Sub(d.Start) != 90*time.Minute {
		t.Errorf("unexpected draft %+v", d)
	}
	if !d.Start.Equal(time.Date(2026, 11, 20, 15, 0, 0, 0, kw)) {
		t.Errorf("start %v", d.Start)
	}
}

func TestNewerVersionCases(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.2.0", "2.1.1", true},
		{"1.2.10", "1.2.9", true},
		{"2.1.1", "2.1.1", false},
		{"2.1", "2.1.0", false},
		{"2.0.9", "2.1.0", false},
		{"v3.0.0", "2.9.9", true},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestCompareVersionsPrerelease(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.3.0", "2.3.0-beta.2", 1},
		{"2.3.0-beta.2", "2.3.0-beta.1", 1},
		{"2.3.0-beta.10", "2.3.0-beta.9", 1},
		{"2.3.0-beta.1", "2.2.10", 1},
		{"2.2.10", "2.3.0-beta.1", -1},
		{"2.3.0-rc.1", "2.3.0-beta.5", 1},
		{"v2.3.0", "2.3.0", 0},
		{"2.2.10", "2.2.9", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
