package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// localParse understands simple lines without any AI, e.g.
//
//	"gym 7am", "dentist tomorrow 4:30pm", "lunch with Ali friday 1pm at Avenues",
//	"team sync next tue 10:00 for 30m", "Ali's birthday 12 dec", "call mum tonight 9".
//
// It returns ok=false when it finds no date or time at all (so it never invents an event).
func localParse(text string, now time.Time) (Draft, bool) {
	s := " " + strings.TrimSpace(text) + " "
	lower := strings.ToLower(s)
	var (
		day      time.Time
		haveDay  bool
		hour     = -1
		minute   int
		dur      = time.Hour
		location string
		tonight  bool
	)
	cut := func(loc []int) {
		lower = lower[:loc[0]] + " " + lower[loc[1]:]
		s = s[:loc[0]] + " " + s[loc[1]:]
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// duration: "for 30m", "for 2 hours", "for 1.5h"
	if m := reDur.FindStringSubmatchIndex(lower); m != nil {
		n, _ := strconv.ParseFloat(lower[m[2]:m[3]], 64)
		unit := lower[m[4]:m[5]]
		if strings.HasPrefix(unit, "m") {
			dur = time.Duration(n * float64(time.Minute))
		} else {
			dur = time.Duration(n * float64(time.Hour))
		}
		cut(m[:2])
	}
	noon := false
	if m := reNoon.FindStringIndex(lower); m != nil {
		noon = true
		cut(m)
	}
	// location: "at <place>" / "@ <place>" when what follows isn't a time
	if m := reLoc.FindStringSubmatchIndex(lower); m != nil {
		location = strings.TrimSpace(s[m[2]:m[3]])
		cut(m[:2])
	}
	// day words
	switch {
	case reMatch(reTonight, &lower, &s, cut):
		day, haveDay, tonight = today, true, true
	case reMatch(reToday, &lower, &s, cut):
		day, haveDay = today, true
	case reMatch(reTomorrow, &lower, &s, cut):
		day, haveDay = today.AddDate(0, 0, 1), true
	}
	if !haveDay {
		if m := reWeekday.FindStringSubmatchIndex(lower); m != nil {
			next := m[2] >= 0
			wd := weekdays[lower[m[4]:m[4]+3]]
			delta := (int(wd) - int(today.Weekday()) + 7) % 7
			if delta == 0 || next {
				if delta == 0 {
					delta = 7
				}
			}
			day, haveDay = today.AddDate(0, 0, delta), true
			cut(m[:2])
		}
	}
	if !haveDay {
		if m := reDate1.FindStringSubmatchIndex(lower); m != nil { // 12 dec
			d, _ := strconv.Atoi(lower[m[2]:m[3]])
			day, haveDay = dateFrom(today, months[lower[m[4]:m[4]+3]], d), true
			cut(m[:2])
		} else if m := reDate2.FindStringSubmatchIndex(lower); m != nil { // dec 12
			d, _ := strconv.Atoi(lower[m[6]:m[7]])
			day, haveDay = dateFrom(today, months[lower[m[2]:m[2]+3]], d), true
			cut(m[:2])
		}
	}
	// time: 7am, 7:30pm, 19:30, noon, "at 9"
	if m := reTime.FindStringSubmatchIndex(lower); m != nil {
		h, _ := strconv.Atoi(lower[m[2]:m[3]])
		if m[4] >= 0 {
			minute, _ = strconv.Atoi(lower[m[4]:m[5]])
		}
		ap := ""
		if m[6] >= 0 {
			ap = lower[m[6]:m[7]]
		}
		if ap == "pm" && h < 12 {
			h += 12
		} else if ap == "am" && h == 12 {
			h = 0
		} else if ap == "" && m[4] < 0 && h >= 1 && h <= 7 {
			h += 12 // "dinner 7" means 7 pm
		}
		if tonight && h < 12 {
			h += 12
		}
		if h <= 23 && minute <= 59 {
			hour = h
			cut(m[:2])
		}
	} else if noon {
		hour, minute = 12, 0
	}

	if tonight && hour < 0 {
		hour = 20
	}
	if !haveDay && hour < 0 {
		return Draft{}, false
	}
	title := strings.Join(strings.Fields(reFiller.ReplaceAllString(" "+s+" ", " ")), " ")
	title = strings.Trim(title, " ,.-")
	if title == "" {
		title = "New event"
	}
	title = strings.ToUpper(title[:1]) + title[1:]
	d := Draft{Title: title, Location: location}
	if hour < 0 { // date only → all-day
		d.AllDay = true
		d.Start, d.End = day, day.AddDate(0, 0, 1)
		return d, true
	}
	if !haveDay { // time only → next occurrence
		day = today
		if time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location()).Before(now) {
			day = day.AddDate(0, 0, 1)
		}
	}
	d.Start = time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
	d.End = d.Start.Add(dur)
	return d, true
}

func reMatch(re *regexp.Regexp, lower, s *string, cut func([]int)) bool {
	if m := re.FindStringIndex(*lower); m != nil {
		cut(m)
		return true
	}
	return false
}

func dateFrom(today time.Time, mon time.Month, d int) time.Time {
	t := time.Date(today.Year(), mon, d, 0, 0, 0, 0, today.Location())
	if t.Before(today) {
		t = t.AddDate(1, 0, 0)
	}
	return t
}

var (
	reDur      = regexp.MustCompile(`\bfor (\d+(?:\.\d+)?) ?(m|min|mins|minutes|h|hr|hrs|hour|hours)\b`)
	reLoc      = regexp.MustCompile(`\s(?:at|@)\s+([^\d\s][^@]*?)\s*(?:$|\s(?:on|tomorrow|tmrw|today|tonight|next|this|for|noon|(?:sun|mon|tue|wed|thu|fri|sat)[a-z]*|\d))`)
	reTonight  = regexp.MustCompile(`\btonight\b`)
	reToday    = regexp.MustCompile(`\btoday\b`)
	reTomorrow = regexp.MustCompile(`\b(tomorrow|tmrw|tmr)\b`)
	reWeekday  = regexp.MustCompile(`\b(?:(next) |this |on )?((sun|mon|tue|wed|thu|fri|sat)[a-z]*)\b`)
	reDate1    = regexp.MustCompile(`\b(?:on )?(\d{1,2})(?:st|nd|rd|th)? ((jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*)\b`)
	reDate2    = regexp.MustCompile(`\b(?:on )?((jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*) (\d{1,2})(?:st|nd|rd|th)?\b`)
	reTime     = regexp.MustCompile(`\b(?:at )?(\d{1,2})(?::(\d{2}))? ?(am|pm)?\b`)
	reNoon     = regexp.MustCompile(`\b(?:at )?noon\b`)
	reFiller   = regexp.MustCompile(`(?i)\s(at|on|@)\s*$`)
	weekdays   = map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	months     = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
)
