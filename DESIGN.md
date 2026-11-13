# Smart Cal — design log

How the app got its shape: each request from the owner (Jamal, "Mr. boss"), what was decided, and why.
Newest first. Screenshots are in [docs/design](docs/design). Release notes per version are in [CHANGELOG.md](CHANGELOG.md);
technical lessons are in [LESSONS.md](LESSONS.md).

## Standing principles

- **Private calendar.** No availability sharing or booking pages — "calendar remains private".
- **No Keychain prompts.** Keys and settings live in `~/Library/Application Support/PersonalCalendar/config.json` (mode 0600). The key is entered once.
- **Kuwait defaults.** Times shown in Kuwait time; week starts Saturday; weekend Friday + Saturday (region presets and custom days available).
- **Iterative, critique-driven.** Each round should improve meaningfully on the last; critiques are turned into numbered lists the owner picks from.
- **Menu bar space is precious.** Every character in the menu bar has to earn its place.
- **Nothing moves under your cursor.** Controls that you click repeatedly (view switcher, arrows) stay in fixed positions.
- **Behave like a Mac app.** System font at 13 pt, arrow cursor, light/dark and the Mac's accent colour, Reduce Motion / Increase Contrast, VoiceOver labels. Standard menus (About · Settings ⌘, · Quit ⌘Q; File · Edit · View · Window · Help), Mac keyboard symbols (⌃⌥⇧⌘), Settings that save instantly and close with the red button or ⌘W, the destructive button on the left and the main action on the right. Monochrome symbols in working UI; emoji only where Playful mode asks for them.
- **The AI never edits silently.** Changes and deletes to existing events show a before/after preview and apply only on a click; they touch only what the user mentioned, keep notes and time zone, and date maths is done in code. New events are added straight away with Undo (since v2.3), with a preview when something clashes, for several events, or offline.
- **Ship small, straight to `main`.** No feature branches ("update directly and we can revert back if needed"). Each change bumps `VERSION` so it reaches the Update button the same day; docs-only changes don't. Every UI change is checked in a browser preview with a mocked Go backend (light, dark, compact and wide) before it ships, since there's no Mac in the loop — and anything that only a Mac can show (menu bar, notifications, windows) is called out as untested.
- **Only officially released platform features.** Beta or unreleased Apple features (like v2.1's Apple Intelligence and Siri) stay out of the shipped app.

---

## v2.3.6 — Don't reschedule the past (13 Nov 2026)

**Reported:** "the entry was 10:00 drive geely and i got a rescheduling, for a past event. unless its a future similar event, rescheduling a past event is not right." (Also: Settings still showed Gemini for the OpenAI backup key — the screenshot was of the version before v2.3.5, which shows "OpenAI key saved" and "→ Uses gpt-4o-mini via OpenAI" and corrects the stored model.)
**Decided:** the AI prompt says a time + title is a new event and "update" needs a change request; in code, `meantAsChange` turns an AI "update" into an "add" (same title, the requested time, moved forward to the next day if that time has passed) unless the words ask for a change (English or Arabic) and — for an event that has already ended — refer back to it (yesterday, this morning, last…). Deletes are unchanged. The reported sentence is in the tests and the AI test set.

## v2.3.5 — No guessing which AI is used (13 Nov 2026)

**Asked:** "i gave smart cal an openai key, and it had gemini on there, will it know it was an openai key?" → it did (any non-OpenRouter `sk-` key is OpenAI, and an OpenRouter-style model with an OpenAI key falls back to gpt-4o-mini) but Settings kept showing Gemini → "yes do it, eliminate all confusion" → "but how would you know the key was from openai? or from anywhere else?"
**Decided:**
- One function (`effectiveModel`) decides the service and the exact model name for requests *and* for Settings, so they can't disagree. Saved models are corrected to ones the key's service has (`fitModel`), on save and when the settings load.
- Settings shows per key: "OpenAI key saved" / "OpenRouter key saved", "→ Uses gpt-4o-mini via OpenAI", and model suggestions for that service only.
- Prefixes identify the company (sk-or- OpenRouter; sk-proj-/sk-svcacct-/sk- OpenAI; sk-ant- Anthropic; AIza Google; gsk_ Groq; xai- xAI) — but a prefix is only a convention, so every saved key is **checked with its service** (OpenRouter `/key` + credits, OpenAI `/models`) and the answer shown under it; "accepted but no credits" counts as a problem (red). Keys for services Smart Cal can't call are refused with an explanation, rather than being sent to the wrong company.

## v2.3.4 — An easy Quit (13 Nov 2026)

**Asked:** "smart cal doesn't show in apple menu → force quit and there is not easily accessible quit in the drop down, fix it".
**Found:** Force Quit lists only apps with a Dock icon (regular activation policy); a menu-bar-only (accessory) app can't appear there — a macOS rule, not something Smart Cal can change. Quit existed only in the menu bar item's right-click menu and in Settings → About.
**Decided:** a Quit button (power symbol, tooltip "Quit Smart Cal (⌘Q)", red on hover) at the end of the panel's bottom line, no confirmation (like ⌘Q in any Mac app); ⌘Q handled in the panel too. The existing "Also show in the Dock" setting now explains that it also puts Smart Cal in Force Quit and gives it a Dock menu with Quit.

## v2.3.3 — Updates in plain sight; the gear at the bottom (13 Nov 2026)

**Asked:** "update is confusing and hidden, have a button showing update is available right under today's date, have the gear / setting button on the last line, not next to the calendar, for the current version number on the last line, if there is an update, let it indicate it also and make it clickable link to initiate an update".
**Found:** the footer's "Update to vX" pill only opened Settings → About, where a second button did the update — two steps, the first one misleading.
**Decided:** an accent-coloured banner under the date line ("Smart Cal vX is available · Update now") that installs directly; the bottom line reads "v2.3.2 · Update to v2.3.3" in the accent colour and installs on click (otherwise the version opens About); the gear moves from the calendar header to the bottom line, right of refresh. In the narrow layout the shortcut hint gives way to the update link so the line never wraps.

## v2.3.2 — Say why the AI failed; a backup AI (13 Nov 2026)

**Asked:** "ai key for openrouter is there, why is it not working? i need more reasons and clues from smart cal? do i need to pay? is it not accessible? why?" — then, while a *Test AI* button was being built: "i want to know the reason from smart cal, i dont want to do a test button, as soon as it fails, i need to know a reason, also add a second backup ai option so if one fails, the other ai tries to do it".
**Decided:**
- AI errors are structured (`aiError`: provider, model, HTTP status, the provider's message, network/timeout) and explained by `explainAI` — a reason and a fix for 401, 402, 403, 404, 408, 429, 400 (structured outputs / too long), 5xx, network and timeouts. On 401/402/403/429 from OpenRouter, Smart Cal also asks OpenRouter about the account (`/credits`, `/key`) and adds what it says ("$0.00 of $5.00 credits left · free tier").
- The reason appears **immediately**, under the AI box, until dismissed — with the offline-read event's preview, or alone when nothing could be read. No test button. Settings → AI shows "Last AI request failed (time): reason".
- **Backup AI:** a second key + model; tried at once when the main AI fails (after its one retry); its answer comes with a note saying the main AI failed and why; if both fail, both reasons. The default backup model is from another company (google/gemini-2.5-flash) so one provider's outage or empty account doesn't stop both; with an OpenAI key, OpenRouter-style model names fall back to OpenAI models.

## v2.3.1 — Pasted notices with full dates (13 Nov 2026)

**Reported:** pasting "Cambridge IGCSE October/November 2026 Results / Date: Thursday, 14 January 2027 / Time: 9:00 AM Kuwait / …" gave a preview for *Thursday 19 Nov, 14:00* with the whole text as the title — "the proper date is supposed to be 14 jan 2027, fix it".
**Found:** the AI request had failed, so the offline parser answered: it took "Thursday" as next Thursday and "14" as 2 PM, didn't know "14 January 2027", and the "AI unavailable" message was a two-second toast.
**Decided:** the offline parser reads explicit dates first (day-month-year, month-day-year, d/m/y as written in Kuwait, ISO) and respects years; a weekday beside a date is ignored; pasted multi-line text uses "Date:"/"Time:" lines, the first line as title, the full text as notes, and only clear times (colon or am/pm). The preview carries an orange note when the offline parser was used, with the reason. The AI call retries once on network errors, 429 and 5xx. The notice is a test case for both the offline parser and the AI test set.

## v2.3.0 — From the critique: calmer, quicker, more helpful (12 Nov 2026)

**Asked:** "critique smart cal, how can it be made more useful". The critique, in short: it does a lot well; what holds it back is trust and clutter, not features. The list:
1. Sign and notarize with an Apple Developer ID · 2. confirm the menu bar fix on a real Mac · 3. a "Copy diagnostics" button · 4. automated UI tests before each release · 5. declutter the top (today said four times) · 6. a "now" line in the agenda · 7. add straight away with Undo instead of a preview trap · 8. an AI test set from real sentences · 9. quick add from anywhere · 10. Hijri dates, Islamic occasions, optional prayer times · 11. alerts you can act on · 12. a morning brief · 13. birthdays from Contacts and event templates · 14. a second time zone when travelling · 15. split `app.js` · 16. a beta channel. Top three recommended: 1, 4, 5.
→ "do 5 6 7 8 9 11 12 15 16".
**Decided:**
- **5 Declutter:** the brand line is removed (credit stays in About); the greeting is one line (full text on hover/click); Up next is a highlighted row in the agenda/list with a pill ("Up next · in 35m · leave by 8:25") instead of a card.
- **6 Now line:** today's group gets a red "Now · 10:42" line between past (dimmed) and remaining events, refreshed every 15 s.
- **7 Add straight away:** "Preview before adding" is off by default (and switched off once for existing installs); Undo covers mistakes; the preview still appears for clashes, several events, or offline results. Fixed the bug that made this path drop the event (addDrafts refused to run while the AI call still marked the box busy).
- **8 AI quality:** prompt rule + code guard for trailing minutes ("… 60"); `testdata/ai_eval.json` (33 English/Arabic sentences with expected results) and `TestAIEval`, run with `SMARTCAL_EVAL_KEY` — add every miss as a case.
- **9 Quick add:** a second global shortcut (⌃⌥N; ⌃⌥Space was avoided because it switches keyboard languages) opens a small floating window with just the AI box; it sizes itself to its content and hides after adding.
- **11 Actionable alerts:** Join / Snooze 10 min / Running late (mailto: the other attendees) — the categories depend on whether there's a link and other people.
- **12 Morning brief:** once a day at the chosen time (default 8:00), skipped on empty days or if the Mac woke more than 4 h late.
- **15 Code split (partial):** date and flight helpers moved to `dates.js` and `flights.js` (they were copied in `app.js` and `week.js`), alongside `menu.js`. The rest of `app.js` shares a lot of state; splitting it further safely needs automated UI tests first (critique item 4).
- **Reported while this was being built:** "why is this black box permanently showing? shouldn't it be temporary then go away?" — the "To-do added · Undo" message. Its hide timer referred to a variable from another function and failed silently; fixed, and it now waits while the pointer is on it so Undo stays reachable.
- **16 Beta channel:** a `VERSION` like `2.4.0-beta.1` is published as a GitHub pre-release; "Get beta versions" makes the updater consider pre-releases. Versions compare with pre-releases before their release (2.4.0-beta.2 < 2.4.0).

**Backlog (from the critique, not picked yet):** 1 Developer ID signing and notarization · 2 verify the menu bar fix on hardware (add a floating fallback if it still hides) · 3 "Copy diagnostics" in About · 4 run the browser preview tests in CI before every release (needed before splitting `app.js` further) · 10 Hijri dates / Islamic occasions / prayer times · 13 birthdays from Contacts, event templates · 14 a second time zone in the day view.

**Asked:** "update all design decisions and convo and readme for this repo". **Done:** this log now records every request of the 10–12 Nov sessions, the critique and its backlog; README rewritten for the v2.3 panel (wide layout, agenda, Up next highlight, quick add, alerts, brief, betas, how changes ship) with new screenshots ([panel](docs/design/v2.3-panel-wide.png), [quick add](docs/design/v2.3-quick-add.png)).

## v2.2.10 — An agenda that runs on (12 Nov 2026)

**Asked:** "remove the bottom today, and have the today next to the calendar, scroll and extend to the next day if space permits and the day after and beyond".
**Decided:** in the wide layout the right column is a continuous agenda instead of a one-day timeline: it starts on today (or the clicked day; ‹ › and Today move it), shows each day's events under its own sticky heading — the first day always, later days when they have something — and loads two more weeks whenever you near the end (up to a year). It spans the panel's full height; the month, Up next and to-dos share the left column. The upcoming list under the to-dos is dropped in this layout (it duplicated the agenda). Rescheduling is by right-click there; the compact layout keeps the day timeline with drag-to-move.
**Fixed along the way:** the new wrapper's class `cal-row` was already used by the calendar rows in Settings; renamed to `wide-row`/`wide-left`.

## v2.2.9 — Month and day side by side (12 Nov 2026)

**Asked:** "lets have a month with a day view next to it, and todo below". **Decided:** a wide layout (760 pt, shrinks to fit the screen) as the default: the AI box across the top; the month and the Up next card on the left; the selected day as an hour timeline on the right (today on open, any day you click, ‹ › and Today), with all-day items and waiting events on top, drag-to-move and double-click-to-add as in the day view; to-dos and the upcoming list full width below. Clicking a day no longer replaces the list in this layout. The one-column panel stays as Settings → Look → Layout → Compact.

## v2.2.8 — Headings as big as the date (12 Nov 2026)

**Asked:** "Make the To-do and Today title bigger, as big as the title today's date". **Decided:** every list heading (To-do, each day in the upcoming list, a single day, the week) uses the top date's style — 22 pt bold, day name in full weight, the date part in the muted colour at a lighter weight ("Today · Thu 12 Nov"), "Today" in the accent colour. Counts and weather stay small; long headings shrink to an ellipsis rather than pushing the count off the row.

## v2.2.7 — Never vanish from a full menu bar (12 Nov 2026)

**Reported:** after updating to v2.2.6, "i cant see it in menu bar, i get a calendar window when i start it, not menubar, no dock icon, even force quit doesnt show smart cal". (Menu-bar-only apps never appear in Force Quit; the week window reopens because it was open at quit.)
**Found:** Wails creates the status item without an `autosaveName`, so macOS puts it at the far left on every launch — the first position hidden behind the notch when the menu bar is full. An update relaunch made it disappear.
**Decided:** set `autosaveName` (macOS then remembers a ⌘-dragged position and visibility); detect when the item isn't on screen (window not visible / outside the areas beside the notch); in that case open the panel centred at launch with a banner explaining how to make room, and make every "show the panel" path (hotkey, menu items, reopening the app, launching it again) fall back to the centred panel.

## v2.2.6 — Update progress, due dates in words, week window menu (11 Nov 2026)

**Asked:** "add it to the week window too". **Decided:** the same menu, now in a shared `menu.js` (pop-up menu + date picker) used by both windows.

**Asked:** "when you do an update via the gear button and accept, everything pauses and the user doesn't know what's going on". **Decided:** a progress card in every open Smart Cal window — icon, title, the step in words with the download percentage, a bar (download = 0–80%, then Check, Install, Restart), and the four steps ticked off — plus "⬇︎ Updating 40%" in the menu bar, because the panel may be closed. Failures say what went wrong and that nothing was changed; if the restart takes over 20 s, it says to open Smart Cal from Applications. A second Update click while one runs is refused.

**Asked:** "i moved the todo for nbk to next week, but there is no indication when it is due, add a line saying when it is due relatively like after 4 days, or after 4 hours, or a date if its way off". **Decided:** the date plus the distance — "Today 21:00 · in 4 hours", "Tomorrow 09:00 · in 10 hours", "Wed 18 Nov, 09:00 · in 7 days", "Overdue by 13 hours" — and just the date from two weeks out. A weekday alone ("Wednesday") is never used for a date more than a few days away.

## v2.2.5 — Right-click to reschedule events (11 Nov 2026)

**Asked:** "add the same right-click reschedule to events".
**Decided:** the to-do menu's choices for every event row in the panel (one listener on the list, so the upcoming, day, week and timeline rows all get it): Today · Tomorrow · In 2 days · In 3 days · Next week (the day it's already on is left out), Pick a date…, then Open · Edit… · Duplicate · Delete. Moves keep the time of day and length (whole calendar days, so DST can't shift the clock), carry over notes and time zone, ask "this one or all future" for repeats, check clashes and offer Undo — the same path as drag-to-move. Read-only and holiday events get only Open/Duplicate. The big week window keeps drag-to-move for now.

## v2.2.4 — Right-click to reschedule to-dos (11 Nov 2026)

**Asked:** "i got these todos and i want to right click and reschedule after a few days, give me that option".
**Decided:** a Mac context menu on each to-do (opens at the pointer, nothing preselected): Today · Tomorrow · In 2 days · In 3 days · Next week, each with the date it lands on in grey, then Pick a date… (an inline date field), Remove date, Mark as done, Delete. Days count from today — rescheduling is about "not now", so an overdue to-do moved "in 3 days" lands 3 days from today, not 3 days after its old date. A to-do with a time keeps the time. Undo restores the old date. It saves only title and date, so the reminder's notes and list stay as they were.

## v2.2.3 — Notes, alerts and the waiting event (11 Nov 2026)

**Reported:** a screenshot of an event card with "240 h before" and notes reading `<p>Ali&#x27;s last day for his Dubai/UAE visa…</p>` — "fix this formatting error".
**Decided:** Google Calendar keeps descriptions as HTML and EventKit passes it through, so `notesText` (Go, `notes.go`) turns it into plain text where events are read — one place, so the card, the editor, search and the AI all get clean text. Plain notes pass through untouched. Alerts are written in the largest whole unit (weeks, days, hours).

**Reported:** "i have a meeting for saturday, but its not showing below the calendar". The screenshot showed the meeting still in the AI preview (not added), and "on Sun 15 Nov on Sun 15 Nov" in the box from double-clicking days.
**Decided:** a waiting event must be visible where you look for it: it shows in its day's list with an Add button and a dashed ring on its day; double-clicking a day while an event is waiting moves it there (the obvious intent) instead of typing into the box; the preview is labelled "Not added yet".

## v2.2.2 — …and the year (11 Nov 2026)

**Asked:** "add the year too". **Decided:** "Wednesday 11 November 2026", the year in the muted colour at a lighter weight. The longest dates ("Wednesday 30 September 2026") only just fit at 22 pt, so the line steps its size down (to 15 pt at most) when it would overflow — e.g. with Settings → text size above 100% — instead of being cut off with "…".

## v2.2.1 — Today's date first (11 Nov 2026)

**Asked:** "when opening smart cal from the menu bar have today's date prominently shown as the first line in large letters, because sometimes you just need to know what day it is".

**Decided:** the panel's first line is the date at 22 pt bold — weekday in the accent colour, then day and month ("Wednesday 11 November"); the year is in the tooltip, since the month header already shows it. It gets the full width so long dates never get cut off; the brand ("Smart Cal · from go.sala.company") moved to a small muted line underneath. It refreshes when the panel opens and at midnight, and uses the app's language.

## v2.2 — Siri & Apple Intelligence removed, Mac conventions, AI edit fixes (10 Nov 2026)

Screenshots: [panel](docs/design/v2.2-panel.png) · [event card](docs/design/v2.2-event-card.png) · [Settings](docs/design/v2.2-settings-window.png).

**Context:** the session began with "look for menu bar calendar app in the repos" (Smart Cal is the menu bar calendar missing from the Flattery catalog), then "review it and tell me what we can improve". The review found it solid — well documented, sensible choices, no frontend security holes (text is escaped, `OpenURL` only opens https/ical) — with the real problems in AI editing and release safety (listed under "do all the fixes" below).

**Asked:** "remove the siri ai, it hasn't been officially released yet".
**Decided:** remove both halves of the v2.1 work — the Siri / Shortcuts actions and the Apple Intelligence engine — together with the Swift helper (`assistant/`) and `apple.go`. The AI box is OpenRouter / OpenAI only again (no engine setting), with the offline parser as the fallback; the greeting and event emojis need the key too. The v2.1 notes below are kept as history.

**Asked:** "look at the app, is it following mac gui and standards, if not fix it where needed".
**Found right already:** system font at 13 pt, arrow cursor, no text selection on controls, light/dark, accent colour, Reduce Motion, VoiceOver labels, Apple-ordered menu bar item menu, standard About panel, the week window.
**Fixed:** our own app menu (Smart Cal · File · Edit · View · Window · Help) instead of Wails' default, whose Help item loaded wails.io inside our window; event card with monochrome symbols, Delete left / Edit right / the rest in a ⋯ pop-up menu so the row never wraps; Settings window titled after the selected pane, no "Done"; ⌃⌥C written with Mac symbols instead of "ctrlaltc"; sentence-case section headings; the gear sized like the other toolbar icons.
**Kept (then):** the brand line at the top of the panel and the coloured *Up next* card — part of the look, not a platform convention. Both went in v2.3 when the owner chose to declutter.

**Asked:** "do all the fixes" (from a code review).
**Fixed:**
- "push it back by a week" moved events a week *earlier* — any "back" in the sentence flipped the direction, even in a title ("back massage"), and also skipped the into-the-past check. Now push back / postpone / delay always mean later; earlier needs "earlier", "sooner", "move it up" or "bring it forward".
- AI edits erased the notes (the event list carries none, and saving writes every field) and reset the time zone to local. Go now fetches the event first and carries both over, so every path gets it. Undo restores a changed repeat.
- The "only change what you mention" guards understood English only; Arabic keywords added.
- All-day moves across a DST change keep whole days.
- http:// links in notes open.
- Releases publish a SHA-256 next to the zip; the updater checks it and `codesign --verify` before swapping; releases run vet + tests first; CI unpacks the zip and checks it like the updater.
- Tests for date shifting, edit guards, applying changes and version comparison (`ai_test.go`).
**Not done:** splitting `frontend/app.js` — a refactor that needs the app running on a Mac to verify.

**Asked:** "why does my update function say im up to date?" → the changes were on a branch; a release is built only when `VERSION` changes on `main`. **Decided:** merged and published v2.2.0. From now on changes go **straight to `main`** ("dont do any new branches, update directly and we can revert back if needed") — and anything that bumps `VERSION` ships to everyone's Update button.

**Asked:** "update docs, specs and readme". **Done:** README, this log and LESSONS brought up to date for v2.2, with screenshots (there's no separate spec — this log and its standing principles serve as one).

## v2.1.1 — AI edits that only change what you said (9 Nov 2026)

**Reported:** "i asked ai to move this item to 1 month from now, and it moved it to the past which was october… i meant 30 days from today which should be december 9". The preview also showed a new title, all-day for 31 days, a "Work" location and a monthly repeat.

**Decided:** relative moves are computed by Smart Cal, not the model ("N days/weeks/months from now", "later", "earlier", "push it by…"), keeping the event's time and length; edits keep every field the user didn't mention; and an edit that would land in the past without the user saying so is refused with a hint. The reported case is a unit test.

## v2.1 — Apple Intelligence & Siri (8 Nov 2026) — removed in v2.2

**Asked:** "can we use Siri AI through App Intents for the text box in the calendar menu?" → explained the two directions (Foundation Models = Apple's model inside our box; App Intents = Siri calling into Smart Cal) → "yes both".

**Decided:**
- **AI engine setting:** Auto (default) · Apple Intelligence · OpenRouter. Auto uses the on-device model for short everyday requests and OpenRouter for screenshots and long pasted text (the on-device model is small, with a small context window, and doesn't read images here). Failures fall back to OpenRouter when a key exists; with neither, the offline parser still handles simple lines.
- **Siri actions:** Add to Smart Cal · What's Next · Ask Smart Cal, with natural phrases; Siri asks a follow-up for the text. They go through the same AI, calendars and default alerts as the app. **Deleting by voice is refused** because it can't be undone.
- **Architecture:** a small Swift helper app, *Smart Cal Assistant*, inside `Smart Cal.app/Contents/Helpers`. It hosts the App Intents and runs Foundation Models on request (`--fm`, JSON in/out). The Go app stays the main program; the helper reaches it through a private Unix socket in the settings folder. Apple's model receives the same JSON Schema as OpenRouter, converted to a dynamic generation schema, so every AI feature works with either engine.

## v2.0 — Following Apple's Human Interface Guidelines (7 Nov 2026)

**Asked:** "apple has some gui rules like about, and other things, what are they if we were to apply them to the calendar app?" → a 17-point list → "do it all".

**Decided (all 17):**
1. Standard **About Smart Cal** panel (icon, version, credits, © go.sala.company).
2. Tray menu in Apple's order: About · Check for Updates… · Settings… (⌘,) · app items (Open, Week View ⌘⇧W, New Event ⌘N, Refresh ⌘R, Calendar app, Google web) · Smart Cal Help · Quit (⌘Q).
3. **Settings apply instantly** — the Save button was retired (it had been requested in v1.2; Apple's convention won once the owner chose "do it all"). A small "✓ Saved" tick confirms.
4. **Settings in their own window** with toolbar icon tabs (Calendar, AI, Look, Menu bar, Alerts, About).
5. **Monochrome (template) menu bar icon** option; colour remains the default.
6. **Show in the Dock** option, since the menu bar item can hide behind the notch; ⌃⌥C always works.
7. **Ask before adding a login item** (first run shows "Start Smart Cal when you log in?"). Existing installs keep their choice.
8. Shortcuts: ⌘N, ⌘Z / ⌘⇧Z, ⌘1–⌘4, ?, plus the existing ⌘F ⌘T ⌘R ⌘, Esc.
9. **System accent colour** and **system clock format** options (the Saturday week start stays a Smart Cal setting).
10. Accessibility: VoiceOver labels from tooltips, Reduce Motion, Increase Contrast, visible focus rings.
11–12. Undo over confirmations and just-in-time permissions were already in place.
13. Native polish: focus rings, toolbar tabs, system materials.
14. Week and Settings windows remember size and position.
15. App icon on Apple's rounded-square grid (mascot on a soft tile).
16. **Arabic** interface with right-to-left layout, Arabic dates (Latin digits) and an Arabic AI greeting.
17. **Help & shortcuts** sheet.

![Settings window](docs/design/v2-settings-window.png)
![Help and Arabic](docs/design/v2-help-arabic.png)
![Icon on a tile](docs/design/v2-icon-tile.png)

## v1.9 — Menu bar choices (7 Nov 2026)

**Asked:** "the next event is useful but i need to drastically abbreviate… 10 suggestions to allow more to show on the menu bar from other apps". Then: show examples of emoji + countdown; "4 is good"; "the actual app icon is wasting space, lets remove it, unless there is no forthcoming event"; "give the user a choice of style in the settings… show them as user choice examples… how they will appear in the settings"; "update it with menu bar choices".

**Decided:**
- Default label became **Compact**: emoji + countdown + one word — `👥 2h Meet`, `🦷 25m Dentist`, `👥 now Meet`, later days `✈️ Su 8a KU101` (about a third of the old width).
- The **app icon is hidden** whenever there is text; it comes back only when nothing is coming up (so there is always something to click). The "meeting in 5 minutes" orange badge moved from the icon into the text: `🟠 👥 4m Meet`.
- A new **Menu bar** settings tab with a live preview (tomorrow / in 2 h / in 25 min / started) and 10 styles, each shown as a mini menu bar:
  Compact · Minimal (`👥 2h`) · Countdown + title · Time + title · Full (original) · Today only · Last hour only · Date · Date + compact · Icon only.
- **Fine-tune** switches apply on top: title length (none / 1 word / short / full), how far ahead (1 hour / today / any day), countdown or clock, show emoji, hide during a meeting, orange-dot timing (off / 5 / 10 / 15 min).
- Old settings migrate: "Next event" → Full, "Within the hour" → Last hour only.

Mock-up that was approved: ![menu bar settings mock-up](docs/design/menubar-settings-mockup.png)
As built: ![menu bar settings](docs/design/menubar-settings.png)

Options considered and not chosen as default: initials (`MF 10a`, too cryptic), count of events left (`3 left`, doesn't say what's next), title-only (`Meet fri…`, loses the time).

## v1.8.4 — Flight tracking

**Asked:** "for flights reference, have the flight number open a flightradar link to allow us to see the flight especially when it is flying".

**Decided:** detect flight numbers (KU 101, EK857, "flight QR 1077") in the title, location and notes; a ✈︎ button opens `flightradar24.com/data/flights/<code>`. From 1 h before departure to 1 h after landing it reads **Track live** and pulses. Airline codes must be capitals unless the event mentions a flight/airport — otherwise "meet at 9" would match ("AT" is an airline). Flights get a ✈️ emoji.

![flight tracking](docs/design/flight-tracking.png)

## v1.8.3 — Cleaner icon

**Asked:** "i like this icon better (the greeting mascot), and put the ai magic wand on it, change everything to this cleaner icon".

**Decided:** the flat smiling-calendar mascot with a royal-blue magic wand and gold sparkles is the app icon, menu bar icon (a bolder simplified version for 22 pt) and panel brand mark. The greeting mascot itself stays wand-less so its face can change mood.

![icon sheet](docs/design/icon-sheet.png)

## v1.8.5 / v1.8.6 — Fixed view switcher, "you are here" highlights

**Asked:** "keep the year month week day keys position fixed, the jumping around confuses the view, or have them left aligned and static and the text representation to the right of it". Then: "highlight the current month in year, current week in month view, current day in week view, and current hour in day view".

**Decided:** the four view buttons sit at the far left and never move; the title follows them; arrows, search and gear are fixed on the right (the "back to today" dot reserves its space even when hidden). Highlights: current month tile (year), tinted band across the current week (month), tinted outlined today line (week), current-hour line (day).

![views](docs/design/views.png)

## v1.8.2 — Week as lines, day as hours; calendar permission

**Asked:** "expand the week view as 1 day of the week per line / expand the day view as hourly view per line". Also: "clicked on allow… nothing happened… dont even know why i need to allow".

**Decided:** Week view = 7 lines (day, weather, every event; click a day to open it). Day view = 24 hour lines with "now" highlighted; click an empty hour to add there.
The Allow button now explains why access is needed (Smart Cal reads the Mac's calendars, which is where Google syncs), shows "Waiting for macOS…", clears a stale macOS answer and asks again, and falls back to opening the right Settings page. Builds are signed with a fixed designated requirement so the permission survives updates.

![week and day lines](docs/design/week-day-lines.png)

## v1.8.0 / v1.8.1 — AI box first; Year · Month · Week · Day

**Asked:** "move the ai text box right below the calendar (title), then the good evening and calendar follows. also right after November 2026, have an icon for year view, month view, weekly view, day view, with the selected view with a darker border". Then "expand the drop down if needed".

**Decided:** order is brand → AI box → greeting → calendar → countdowns → list. Segmented view switcher with a dark outline on the selected view. The drop-down grows to the screen height (up to 880 pt).

## v1.8.0 — "More practical and usable" critique

**Asked:** do 1 (repeating events, with AI help), 2 (clash warnings + free-slot finder), 3 (paste text and screenshots), 4 (undo everywhere), 5 (default alerts + leave-now travel time), 6 (Apple Reminders to-dos), 8 (pop-out week window), 10 (small fixes: search notes and the past year, configurable shortcut, settings tabs, time zones, text size). **Not** 7 (availability sharing — calendar stays private).

## v1.6 / v1.7 — "More fun" critique; rename

**Asked:** do everything except Kuwait-specific #4; week starts Saturday, weekend Fri + Sat adjustable by country; rename to **Smart Cal from go.sala.company**; a gear that looks like a gear.

**Decided:** mascot with moods, AI greeting, event emojis, weather, countdowns, weekly recap, timeline drag, confetti/sounds, AI voice — all behind one Playful switch.

## v1.0 – v1.5 — Foundations

- Menu bar calendar in Go; Fyne was replaced by **Wails v3** for a native-feeling UI.
- Google via the Mac's own calendar sync (EventKit) — "a simple 1 or 2 step process" instead of OAuth JSON files.
- **OpenRouter** (or OpenAI) for the AI box, which adds, answers, edits and deletes with before/after previews; an offline parser handles simple lines.
- Self-update from GitHub releases (repo made public so downloads work without a token).
- Settings with Save and Return; event days highlighted (dots were too small); royal blue default with a highlight-colour choice; the AI box with a big border, a magic-wand icon and two lines; a bigger gear.
