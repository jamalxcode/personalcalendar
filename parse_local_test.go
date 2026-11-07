package main

import (
	"testing"
	"time"
)

func TestLocalParse(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	now := time.Date(2026, 11, 7, 16, 0, 0, 0, kw) // Saturday
	cases := []struct {
		in, title, start, loc string
		allDay                bool
		dur                   time.Duration
	}{
		{"gym 7am", "Gym", "2026-11-08 07:00", "", false, time.Hour},
		{"dentist tomorrow 4:30pm", "Dentist", "2026-11-08 16:30", "", false, time.Hour},
		{"lunch with Ali friday 1pm at Avenues", "Lunch with Ali", "2026-11-13 13:00", "Avenues", false, time.Hour},
		{"lunch at Avenues tomorrow 1pm", "Lunch", "2026-11-08 13:00", "Avenues", false, time.Hour},
		{"team sync next tue 10:00 for 30m", "Team sync", "2026-11-10 10:00", "", false, 30 * time.Minute},
		{"Ali's birthday 12 dec", "Ali's birthday", "2026-12-12 00:00", "", true, 24 * time.Hour},
		{"call mum tonight 9", "Call mum", "2026-11-07 21:00", "", false, time.Hour},
		{"dinner 7", "Dinner", "2026-11-07 19:00", "", false, time.Hour},
		{"standup at noon", "Standup", "2026-11-08 12:00", "", false, time.Hour},
		{"review nov 20 at 3pm for 2 hours", "Review", "2026-11-20 15:00", "", false, 2 * time.Hour},
		{"pay rent saturday", "Pay rent", "2026-11-14 00:00", "", true, 24 * time.Hour},
	}
	for _, c := range cases {
		d, ok := localParse(c.in, now)
		if !ok {
			t.Errorf("%q: not parsed", c.in)
			continue
		}
		got := d.Start.Format("2006-01-02 15:04")
		if d.Title != c.title || got != c.start || d.Location != c.loc || d.AllDay != c.allDay || d.End.Sub(d.Start) != c.dur {
			t.Errorf("%q → title=%q start=%s loc=%q allDay=%v dur=%v", c.in, d.Title, got, d.Location, d.AllDay, d.End.Sub(d.Start))
		}
	}
	if _, ok := localParse("buy milk", now); ok {
		t.Errorf("'buy milk' should not parse (no date/time)")
	}
}

func TestNewerVersion(t *testing.T) {
	if !newerVersion("1.0.10", "1.0.9") || newerVersion("1.0.0", "1.0.0") || !newerVersion("1.5.0", "1.4.9") {
		t.Fatal("version compare wrong")
	}
}
