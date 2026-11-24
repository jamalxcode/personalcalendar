# Smart Cal — business rules

What Smart Cal does, as rules. If the code and this page disagree, one of them is wrong — fix both. The *why* behind each rule is in [DESIGN.md](../DESIGN.md); how things look is in [INTERFACE-RULES.md](INTERFACE-RULES.md).

## 1. Your calendar stays yours

- **Calendars come from macOS** (EventKit): Google, iCloud, Exchange — whatever the Mac syncs. There's no Google sign-in in the app and no Google Cloud project.
- **Nothing is shared.** No availability pages, no booking links — "calendar remains private".
- **What leaves the Mac:** only when you use the AI — the text you typed and a list of nearby events (past week to two months ahead, at most 400) go to your AI service (OpenRouter or OpenAI). Weather asks Open-Meteo for your city. Updates ask GitHub. That's all.
- **Keys and settings** live in `~/Library/Application Support/PersonalCalendar/config.json`, mode 0600, written atomically. Never the Keychain (no password prompts).
- **You choose which calendars show** and which one new events go to. Read-only calendars named "Holiday" become small day tags; the same holiday arriving twice is shown once.

## 2. Adding events with the AI box

- **Added straight away, with Undo.** A preview appears instead when: *Preview before adding* is on, the event clashes with another, there are several events, or the AI wasn't used (offline read).
- **Defaults:** 1 hour long; a time with no date = the next time that time comes round; a weekday = the next one; no time = all-day; "at / @ place" = location.
- **A trailing number is minutes:** "catch up with Miteb 60" → 60 minutes, title without "60" (15–240 in usual steps).
- **Pasted text** (emails, notices, bookings): one event per real event; flights become "✈ KU 101 KWI→DXB" with departure and arrival times, departure airport as the place, booking reference and seat in the notes. Screenshots need an AI key.
- **Time zones:** "3pm Dubai time" is converted to your time and the zone is kept on the event.
- **Deadlines** ("residency expires on 26 Nov 2029", "set the date for it") are all-day events on that date. **"Remind me N minutes / hours / days / weeks before"** sets the event's alert; days or weeks before an all-day event alert at 9:00 that day.
- **The AI's answer is never wasted:** dated items it lists as to-dos when you asked for an event become events, and the other way round.
- **Repeats** ("every Monday", "every weekday until June", "for 10 weeks") become real repeat rules.
- **Without AI** (no key, or the AI failed) a built-in reader handles simple lines and pasted notices:
  - explicit dates win over weekday names: "Thursday, 14 January 2027" is 14 Jan 2027;
  - dates as "14 January 2027", "January 14, 2027", "14/01/2027" (day first) or "2027-01-14"; a year is respected, a date without one is the next time it comes round;
  - in pasted text: "Date:" / "Time:" lines are used, the first line is the title, the whole text goes into the notes, and only clear times (with a colon or am/pm) count;
  - "remind me 30 days before" becomes the alert; instructions to the app ("set the date for it", "put it in my calendar") are left out of the title;
  - it never invents an event: no date or time, no event.

## 3. Changing events

- **Only when you ask.** The AI may change or delete an existing event only when your words ask for it ("move", "reschedule", "change", "push", "postpone", "cancel", "rename"… or the Arabic equivalents). Otherwise "10:00 drive geely" is a **new** event, even if a similar one exists.
- **Not the past.** An event that has already ended is changed only if you point back at it ("this morning's…", "yesterday's…", "last…"). Otherwise it becomes a new event — moved to tomorrow if the time has passed today.
- **Always a before/after preview** for AI changes and deletes; nothing happens until you click.
- **Only what you mentioned changes.** Title, place, calendar, repeat, all-day, length, notes and time zone stay unless your words mention them (English and Arabic keywords).
- **Relative moves are computed, not guessed:** "1 month from now", "2 weeks later", "push it back by a week", "postpone by 3 days" — push back / postpone / delay mean later; earlier only with "earlier", "sooner", "move it up", "bring it forward". The time of day and length are kept; all-day events move by whole days.
- **No silent moves into the past:** an edit that would land before today is refused with a hint, unless you said so.
- **Notes and time zone survive** every edit (they're fetched before saving, because saving writes every field).
- **Repeating events** ask: this one, or this and all future ones. Undo puts the old repeat rule back.

## 4. Rescheduling with a right-click

- **Events** (panel and week window) and **to-dos**: Today · Tomorrow · In 2 days · In 3 days · Next week, each showing its date, or Pick a date….
- **Days count from today**, not from the old date ("not now, in 3 days").
- **After…** moves later by an amount — minutes, hours, days or weeks (quick choices 15 min to 1 week) — counted from the item's current time; if that's still in the past (overdue), from now. Days and weeks keep the time of day; minutes and hours give a date-only to-do a time; all-day events offer days and weeks only.
- **Events keep their time of day and length**, moved by whole calendar days (daylight saving can't shift the clock); notes and time zone are kept; repeats ask this/all future; overlaps are warned about; Undo.
- **To-dos keep their time** if they have one; only the date is saved, so notes and list stay; Undo; Remove date is available.
- **Read-only and holiday events** offer only Open and Duplicate.

## 4b. Adding faster

- **+ on a day** (agenda heading or month day) starts an event on that day; if an event is waiting to be added, it moves there instead.
- **Add to Smart Cal** (Services menu in any app) sends the selected text to the AI box, exactly as if it were typed.
- **⌥-click** the menu bar item opens quick add; **↑** in the AI box recalls the last 30 entries (kept on this Mac only).

## 5. To-dos

- **From Apple Reminders**, shown under the month (wide layout) or above the list.
- **Due line:** "Today 21:00 · in 4 hours", "Tomorrow 09:00 · in 10 hours", "Wed 18 Nov, 09:00 · in 6 days", "Overdue by 2 days"; just the date from two weeks away. A weekday alone is never used for a date more than a few days off.
- **Added from the AI box** when your words are a task, not a meeting ("remind me to…", "buy milk").
- **Clearing every to-do due today** earns a cheer and counts toward a streak of "all done" days (kept on this Mac).

## 6. Alerts and the morning brief

- **Meeting alerts** 1 minute before by default (Settings → Alerts: 0, 1, 5, 10 minutes, or off). Each alert offers **Join** (online meetings), **Snooze 10 min**, and **Running late…** (drafts an email to the other attendees). If notifications aren't allowed, the panel drops down instead.
- **Default alerts for new events:** 15 minutes before for timed events, 9:00 the day before for all-day ones.
- **Leave-now alerts:** for events with a place, Apple Maps drive time from your starting address; places that can't be found are remembered so they aren't looked up again.
- **Morning brief:** once a day at 8:00 by default — events, the first one, when to leave, to-dos due or overdue. Nothing on an empty day; skipped if the Mac wakes more than 4 hours after the time.

## 7. The AI services

- **Which service a key belongs to** comes from its prefix — `sk-or-` OpenRouter; `sk-proj-` / `sk-svcacct-` / `sk-` OpenAI — and is then **confirmed with the service** when the key is saved (OpenRouter: the key and its credits; OpenAI: the model list).
- **Keys Smart Cal can't use** (Anthropic `sk-ant-`, Google `AIza…`, Groq `gsk_`, xAI `xai-`) are not saved; the reason is shown. An OpenRouter key reaches those models instead.
- **Models must fit the key:** an OpenAI key uses OpenAI models only — anything else becomes gpt-4o-mini, and Settings shows exactly what's sent ("→ Uses gpt-4o-mini via OpenAI").
- **One retry** on a dropped connection, a timeout, 429 or 5xx. Each request may take up to 70 seconds.
- **Backup AI:** if the main AI fails, the backup answers; you're told the main one failed and why. If both fail, both reasons. The default backup model is from another company.
- **"Didn't answer" means it:** no key, no connection, a timeout or an error from the service. When the AI replied but its answer couldn't be used, the message says that instead.
- **Failures are explained at once**, under the AI box, until dismissed — 401 bad key, 402 no credits, 403 refused, 404 no such model, 429 too many requests, 5xx their side, network, timeout — with what OpenRouter says about the account for credit, limit and key problems.

## 8. The menu bar

- **The label shows the next event**, in one of ten styles (default "👥 2h Meet"); the app icon is hidden while there's text. An orange dot appears 5 minutes before a meeting (adjustable).
- **macOS remembers where the item sits** (it has an autosave name): ⌘-drag it once and it stays.
- **If macOS hides it** (full menu bar, notch): after watching for about 20 seconds at launch, a notification says Smart Cal is running and how to open it (⌃⌥C). The panel opens centred when the item can't be seen.
- **Quit** is always reachable: ⏻ on the panel's bottom line, ⌘Q, or the item's right-click menu. Force Quit lists Smart Cal only with *Also show in the Dock* on — macOS lists only apps with a Dock icon.

## 9. Updates and releases

- **Checked** at launch and every 6 hours; stable releases only, or betas too with *Get beta versions*.
- **When there's a newer version:** a banner under today's date ("Smart Cal vX is available · Update now") and "· Update to vX" on the bottom line — both install directly.
- **Installing:** download (with progress), check the SHA-256 published with the release and the app's signature, swap the app, restart. The menu bar shows "⬇︎ Updating 40%". Failures say what went wrong and that nothing changed.
- **After an update** the new version opens the panel with "✓ Updated to vX — all good" (once), with What's new.
- **A release** is made when `VERSION` changes on `main`: tests and vet run first; `X.Y.Z-beta.N` is published as a pre-release. Pushes that don't change `VERSION` publish nothing.

## 9b. Recap and occasions

- **Weekly recap** on the first weekend day and the last workday evening (once a week, dismissible; any time from "This week"): events, hours scheduled, free hours 8–18 on workdays, the busiest day, the longest free stretch, and the change from last week.
- **Occasions** under the date: National Day (25 Feb), Liberation Day (26 Feb), Ramadan (with today's Iftar and when Suhoor ends), Eid al-Fitr (1–3 Shawwal), the Day of Arafah, Eid al-Adha (10–13 Dhu al-Hijjah), the Islamic New Year — Hijri dates from the Umm al-Qura calendar. Prayer times are calculated on the Mac for the weather location (Fajr at 18°, Maghrib at sunset) and can differ from the official timetable by a minute or two.

## 10. Week, region and language

- **Week starts Saturday, weekend Friday + Saturday** by default (Kuwait), with presets for the Gulf, Saudi, UAE, Egypt, Europe and the US, or your own days. Never hard-coded: the grid, the recap, the greeting and the AI all use the setting.
- **English or Arabic** (right-to-left), following the Mac by default. Arabic works in the AI box, including the "only change what you mention" rules.

## 11. Notes and links

- **Google Calendar's HTML notes are shown as plain text** — paragraphs, line breaks, lists, and links (with their address when the text differs). Editing such an event in Smart Cal saves the notes as plain text.
- **Links** in notes open in the browser (https and http); Smart Cal opens nothing else from event text.

## 12. How Smart Cal changes

- **Straight to `main`**, no feature branches; revert if needed.
- **Each change that matters to users ships the same day** with a `VERSION` bump and a CHANGELOG entry (the release notes shown in the app). Docs-only changes don't release.
- **Every request and decision** is written up in DESIGN.md; every hard-won lesson in LESSONS.md; these rules and the interface rules are kept in step with the code.
