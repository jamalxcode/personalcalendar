package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAIEval sends the sentences in testdata/ai_eval.json to the real AI and checks what comes
// back. It's skipped unless SMARTCAL_EVAL_KEY holds an OpenRouter or OpenAI key:
//
//	SMARTCAL_EVAL_KEY=sk-or-… go test -run TestAIEval -v
//
// (SMARTCAL_EVAL_MODEL picks the model; default gpt-4o-mini.) Add a line for every miss you see.
func TestAIEval(t *testing.T) {
	key := os.Getenv("SMARTCAL_EVAL_KEY")
	if key == "" {
		t.Skip("set SMARTCAL_EVAL_KEY to run the AI test set")
	}
	raw, err := os.ReadFile("testdata/ai_eval.json")
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		Cases []struct {
			Text        string   `json:"text"`
			Kind        string   `json:"kind"`
			Day         string   `json:"day"`
			Time        string   `json:"time"`
			Minutes     int      `json:"minutes"`
			AllDay      bool     `json:"all_day"`
			Repeats     bool     `json:"repeats"`
			TitleHas    []string `json:"title_has"`
			TitleNot    []string `json:"title_not"`
			LocationHas string   `json:"location_has"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	model := os.Getenv("SMARTCAL_EVAL_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	cfg := &Config{OpenAIKey: key, OpenAIModel: model, WeekStart: 6, WeekendDays: []int{5, 6}, Voice: "friendly"}
	cals := []CalendarInfo{{ID: "c1", Name: "Personal", Writable: true, Primary: true}}
	today := time.Now()
	sod := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	pass := 0
	for _, c := range set.Cases {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		res, err := askAI(ctx, cfg, c.Text, "", cals, nil)
		cancel()
		var miss []string
		if err != nil {
			miss = append(miss, "error: "+err.Error())
		} else if res.Kind != c.Kind {
			miss = append(miss, "kind "+res.Kind+", want "+c.Kind)
		} else if c.Kind == "add" {
			if len(res.Drafts) == 0 {
				miss = append(miss, "no event")
			} else {
				d := res.Drafts[0]
				if c.Day != "" && !onDay(d.Start, c.Day, sod) {
					miss = append(miss, "day "+d.Start.Format("Mon 2 Jan")+", want "+c.Day)
				}
				if c.Time != "" && d.Start.Format("15:04") != c.Time {
					miss = append(miss, "time "+d.Start.Format("15:04")+", want "+c.Time)
				}
				if c.Minutes > 0 && int(d.End.Sub(d.Start).Minutes()) != c.Minutes {
					miss = append(miss, "length "+d.End.Sub(d.Start).String())
				}
				if c.AllDay != d.AllDay && (c.AllDay || c.Time != "") {
					miss = append(miss, "all-day mismatch")
				}
				if c.Repeats && !d.Repeat.Active() {
					miss = append(miss, "should repeat")
				}
				for _, w := range c.TitleHas {
					if !strings.Contains(strings.ToLower(d.Title), strings.ToLower(w)) {
						miss = append(miss, "title "+d.Title+" lacks "+w)
					}
				}
				for _, w := range c.TitleNot {
					if strings.Contains(d.Title, w) {
						miss = append(miss, "title "+d.Title+" has "+w)
					}
				}
				if c.LocationHas != "" && !strings.Contains(strings.ToLower(d.Location), strings.ToLower(c.LocationHas)) {
					miss = append(miss, "location "+d.Location)
				}
			}
		} else if c.Kind == "todo" {
			for _, w := range c.TitleHas {
				if len(res.Todos) == 0 || !strings.Contains(strings.ToLower(res.Todos[0].Title), strings.ToLower(w)) {
					miss = append(miss, "to-do lacks "+w)
				}
			}
		}
		if len(miss) == 0 {
			pass++
			t.Logf("ok    %s", c.Text)
		} else {
			t.Errorf("MISS  %s → %s", c.Text, strings.Join(miss, "; "))
		}
	}
	t.Logf("%d of %d right", pass, len(set.Cases))
}

// onDay: "+N" days from today, or a weekday ("sat") within the coming week (today counts).
func onDay(t time.Time, want string, sod time.Time) bool {
	if strings.HasPrefix(want, "+") {
		n := 0
		for _, r := range want[1:] {
			n = n*10 + int(r-'0')
		}
		d := sod.AddDate(0, 0, n)
		return t.Year() == d.Year() && t.YearDay() == d.YearDay()
	}
	days := map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	wd, ok := days[want]
	return ok && t.Weekday() == wd && !t.Before(sod) && t.Before(sod.AddDate(0, 0, 8))
}

func TestFixTrailingMinutes(t *testing.T) {
	kw := time.FixedZone("AST", 3*3600)
	s := time.Date(2026, 11, 14, 10, 0, 0, 0, kw)
	cases := []struct {
		title, want string
		mins        int
	}{
		{"Meet miteb to catch up 60", "Meet miteb to catch up", 60},
		{"Gym 90", "Gym", 90},
		{"Haircut 30 min", "Haircut", 30},
		{"Flight KU 101", "Flight KU 101", 60}, // not a usual length: left alone
		{"Room 2", "Room 2", 60},
		{"Meeting", "Meeting", 60},
	}
	for _, c := range cases {
		d := Draft{Title: c.title, Start: s, End: s.Add(time.Hour)}
		fixTrailingMinutes(&d)
		if d.Title != c.want || int(d.End.Sub(d.Start).Minutes()) != c.mins {
			t.Errorf("%q → %q %v, want %q %dm", c.title, d.Title, d.End.Sub(d.Start), c.want, c.mins)
		}
	}
}
