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
//
// Pasted text (several lines — an email, a booking, an exam notice) is read differently: the
// title is the first line, the date and time come from "Date:" / "Time:" lines when there are
// any, bare numbers aren't taken as hours, and the whole text goes into the notes.
func localParse(text string, now time.Time) (Draft, bool) {
	if strings.Contains(strings.TrimSpace(text), "\n") {
		return localParsePasted(text, now)
	}
	return localParseLine(text, now, false)
}

func localParsePasted(text string, now time.Time) (Draft, bool) {
	var title string
	var when []string
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if m := reLabel.FindStringSubmatch(ln); m != nil {
			when = append(when, m[2])
			continue
		}
		if title == "" && !reURL.MatchString(ln) {
			title = ln
		}
	}
	src := strings.Join(when, " ")
	if src == "" { // no labelled lines: look through the whole text
		src = strings.Join(strings.Fields(reURL.ReplaceAllString(text, " ")), " ")
	}
	d, ok := localParseLine(src, now, true)
	if !ok {
		return Draft{}, false
	}
	if title != "" {
		d.Title = strings.TrimSpace(strings.TrimRight(title, ":"))
	}
	d.Notes = strings.TrimSpace(text)
	return d, true
}

func localParseLine(text string, now time.Time, pasted bool) (Draft, bool) {
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

	// repeats: "every monday", "every mon and wed", "daily", "every day", "weekly", "monthly", "every weekday"
	var rep Repeat
	if m := reEvery.FindStringIndex(lower); m != nil {
		phrase := lower[m[0]:m[1]]
		switch {
		case strings.Contains(phrase, "weekday"):
			rep = Repeat{Freq: "weekly", Interval: 1, Weekdays: []int{0, 1, 2, 3, 4}}
		case strings.Contains(phrase, "daily") || strings.Contains(phrase, "every day"):
			rep = Repeat{Freq: "daily", Interval: 1}
		case strings.Contains(phrase, "monthly") || strings.Contains(phrase, "every month"):
			rep = Repeat{Freq: "monthly", Interval: 1}
		case strings.Contains(phrase, "yearly") || strings.Contains(phrase, "every year"):
			rep = Repeat{Freq: "yearly", Interval: 1}
		default:
			rep = Repeat{Freq: "weekly", Interval: 1}
			for _, w := range reWeekdayWord.FindAllString(phrase, -1) {
				rep.Weekdays = append(rep.Weekdays, int(weekdays[w[:3]]))
			}
		}
		cut(m)
	}
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
	// an explicit date: "14 January 2027", "January 14, 2027", "14/01/2027", "2027-01-14", "12 dec"
	if m := reDateISO.FindStringSubmatchIndex(lower); m != nil {
		y, _ := strconv.Atoi(lower[m[2]:m[3]])
		mo, _ := strconv.Atoi(lower[m[4]:m[5]])
		dd, _ := strconv.Atoi(lower[m[6]:m[7]])
		if mo >= 1 && mo <= 12 && dd >= 1 && dd <= 31 {
			day, haveDay = time.Date(y, time.Month(mo), dd, 0, 0, 0, 0, now.Location()), true
			cut(m[:2])
		}
	} else if m := reDateNum.FindStringSubmatchIndex(lower); m != nil { // day/month/year, as written in Kuwait and the UK
		dd, _ := strconv.Atoi(lower[m[2]:m[3]])
		mo, _ := strconv.Atoi(lower[m[4]:m[5]])
		y, _ := strconv.Atoi(lower[m[6]:m[7]])
		if y < 100 {
			y += 2000
		}
		if mo >= 1 && mo <= 12 && dd >= 1 && dd <= 31 {
			day, haveDay = time.Date(y, time.Month(mo), dd, 0, 0, 0, 0, now.Location()), true
			cut(m[:2])
		}
	} else if m := reDate1.FindStringSubmatchIndex(lower); m != nil { // 14 january (2027)
		dd, _ := strconv.Atoi(lower[m[2]:m[3]])
		day, haveDay = dateWithYear(today, months[lower[m[4]:m[4]+3]], dd, lower, m[8], m[9]), true
		cut(m[:2])
	} else if m := reDate2.FindStringSubmatchIndex(lower); m != nil { // january 14(, 2027)
		dd, _ := strconv.Atoi(lower[m[6]:m[7]])
		day, haveDay = dateWithYear(today, months[lower[m[2]:m[2]+3]], dd, lower, m[8], m[9]), true
		cut(m[:2])
	}
	if haveDay { // a weekday next to the date ("Thursday, 14 January") is just decoration
		if m := reWeekday.FindStringIndex(lower); m != nil {
			cut(m)
		}
	}
	// day words
	switch {
	case haveDay:
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
	// time: 7am, 7:30pm, 19:30, noon, "at 9" — in pasted text only clear times (with a colon or am/pm)
	var m []int
	for _, x := range reTime.FindAllStringSubmatchIndex(lower, -1) {
		if !pasted || x[4] >= 0 || x[6] >= 0 {
			m = x
			break
		}
	}
	if m != nil {
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
	if !haveDay && hour < 0 && !rep.Active() {
		return Draft{}, false
	}
	title := strings.Join(strings.Fields(reFiller.ReplaceAllString(" "+s+" ", " ")), " ")
	title = strings.Trim(title, " ,.-")
	if title == "" {
		title = "New event"
	}
	title = strings.ToUpper(title[:1]) + title[1:]
	d := Draft{Title: title, Location: location, Repeat: rep}
	if rep.Active() {
		d.RepeatText = rep.Text()
		if !haveDay && len(rep.Weekdays) > 0 { // "gym every mon and wed 7am" → first upcoming of those days
			best := 8
			for _, w := range rep.Weekdays {
				dd := (w - int(today.Weekday()) + 7) % 7
				if dd < best {
					best = dd
				}
			}
			day, haveDay = today.AddDate(0, 0, best), true
		}
	}
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

// dateWithYear: the given year when the text has one (lower[ys:ye]), else the next time that date comes round.
func dateWithYear(today time.Time, mon time.Month, d int, lower string, ys, ye int) time.Time {
	if ys >= 0 {
		if y, err := strconv.Atoi(lower[ys:ye]); err == nil {
			return time.Date(y, mon, d, 0, 0, 0, 0, today.Location())
		}
	}
	return dateFrom(today, mon, d)
}

func dateFrom(today time.Time, mon time.Month, d int) time.Time {
	t := time.Date(today.Year(), mon, d, 0, 0, 0, 0, today.Location())
	if t.Before(today) {
		t = t.AddDate(1, 0, 0)
	}
	return t
}

var (
	reEvery       = regexp.MustCompile(`\b(?:daily|weekly|monthly|yearly|every (?:week ?day|day|week|month|year|(?:(?:sun|mon|tue|wed|thu|fri|sat)[a-z]*(?:,? (?:and |& )?)?)+))\b`)
	reWeekdayWord = regexp.MustCompile(`(sun|mon|tue|wed|thu|fri|sat)[a-z]*`)
	reDur         = regexp.MustCompile(`\bfor (\d+(?:\.\d+)?) ?(m|min|mins|minutes|h|hr|hrs|hour|hours)\b`)
	reLoc         = regexp.MustCompile(`\s(?:at|@)\s+([^\d\s][^@]*?)\s*(?:$|\s(?:on|tomorrow|tmrw|today|tonight|next|this|for|noon|(?:sun|mon|tue|wed|thu|fri|sat)[a-z]*|\d))`)
	reTonight     = regexp.MustCompile(`\btonight\b`)
	reToday       = regexp.MustCompile(`\btoday\b`)
	reTomorrow    = regexp.MustCompile(`\b(tomorrow|tmrw|tmr)\b`)
	reWeekday     = regexp.MustCompile(`\b(?:(next) |this |on )?((sun|mon|tue|wed|thu|fri|sat)[a-z]*)\b`)
	reDate1       = regexp.MustCompile(`\b(?:on )?(\d{1,2})(?:st|nd|rd|th)?,? ((jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*)\.?(?:,? (\d{4}))?\b`)
	reDate2       = regexp.MustCompile(`\b(?:on )?((jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*)\.? (\d{1,2})(?:st|nd|rd|th)?(?:,? (\d{4}))?\b`)
	reDateISO     = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	reDateNum     = regexp.MustCompile(`\b(\d{1,2})[/.](\d{1,2})[/.](\d{4}|\d{2})\b`)
	reTime        = regexp.MustCompile(`\b(?:at )?(\d{1,2})(?::(\d{2}))? ?(am|pm)?\b`)
	reLabel       = regexp.MustCompile(`(?i)^(date|time|when|starts?|on)\s*:\s*(.+)$`)
	reURL         = regexp.MustCompile(`https?://\S+`)
	reNoon        = regexp.MustCompile(`\b(?:at )?noon\b`)
	reFiller      = regexp.MustCompile(`(?i)\s(at|on|@)\s*$`)
	weekdays      = map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	months        = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
)
