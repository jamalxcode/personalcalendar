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

---

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
