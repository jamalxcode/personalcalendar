# Changelog

Release notes shown in the app's Update panel come from the matching section here.

## v1.8.7

- **Compact menu bar** (now the default): emoji + countdown + one word — "👥 2h Meet", "🦷 25m Dentist", "👥 now Meet", and for later days "✈️ Su 8a KU101". Takes about a third of the space, leaving room for your other menu bar apps. The old styles are still in Settings → Look

## v1.8.6

- "You are here" highlights in every view: the current month in Year view, the current week (a tinted band) in Month view, today in Week view and the current hour in Day view

## v1.8.5

- The Year · Month · Week · Day buttons now sit on the left and never move; the title follows them, and the arrows, search and gear stay put too

## v1.8.4

- **Flight tracking:** events with a flight number (KU 101, EK857, "flight QR 1077"…) get a ✈︎ button that opens the flight on Flightradar24. From an hour before take-off until an hour after landing it says **Track live** and the button pulses, so you can see where the plane is
- Works on the Up next card, in the event list, in the event details and in the big week window; flights get a ✈️ emoji

## v1.8.3

- New, cleaner icon everywhere (app, menu bar, panel): the smiling calendar mascot holding the AI magic wand

## v1.8.2

- Fixed: "Allow" for calendar access could do nothing after an update. Smart Cal now clears macOS's stale answer and asks again, shows "Waiting for macOS…", and opens the right Settings page if the prompt still doesn't appear
- Permissions now survive future updates (the app keeps a stable signature), so macOS shouldn't ask again after each update
- The banner explains why access is needed
- **Week view** is now one line per day (all 7 days, with weather and every event); click a day to open it
- **Day view** is now one line per hour, with "now" highlighted; click an empty hour to add an event there

## v1.8.1

- The drop-down panel is taller (fits the AI box, calendar and list), up to your screen's height
- Updating is more reliable: if the old copy doesn't quit in time it's stopped, so the new version always opens
- Release notes in the Update panel show bold text properly

## v1.8.0

- **New layout:** the AI box sits right at the top, then the greeting, then the calendar
- **Year · Month · Week · Day views** — switch with the icons next to the title (the selected one has a dark outline); ‹ › step by year, month, week or day, and the blue dot takes you back to today
- **Repeating events:** "gym every Mon & Wed 7am", "rent on the 1st every month" — or pick Repeat in the editor (custom days, until a date). Editing or deleting a repeating event asks: this one, or this and all future
- **Clash warnings** on previews, the editor and drag-and-drop, plus a free-slot finder ("find an hour for the gym this week") with one-click booking
- **Paste anything:** paste or drop text, emails or screenshots (flight bookings, invites) into the AI box, or press the clipboard button — the AI pulls the events out
- **Undo everywhere:** adds, edits, moves, deletes and to-dos
- **Default alerts** for new events (and all-day events), and **Leave now** notifications using live drive time from Apple Maps
- **To-dos** from Apple Reminders: see, tick off and add — or just type "remind me to call the bank tomorrow"
- **Big week view** window (⌘⇧W or from the menu bar): drag to move and resize across days, double-click to add
- Time zones shown when an event is in another zone; search now covers notes and the past year; configurable keyboard shortcut; settings split into tabs; text size setting

## v1.7.0

- Renamed to **Smart Cal** — from go.sala.company. The app renames itself on disk ("Smart Cal.app"); settings, your AI key and calendar permission carry over.
- A proper settings gear (crisp 8-tooth cog that turns when you hover it)
- Greeting bubble looks clean in dark mode

## v1.6.0

- Meet the mascot: a smiling calendar whose face follows your day (happy, busy, focused, sleepy, party) — click it for a tip
- One-line greeting at the top (AI-written with your OpenRouter key, built-in otherwise); evenings preview tomorrow
- Emojis on events, picked automatically (🦷 dentist, 🚗 test drive, 🍽️ lunch…; the AI fills in the rest, cached)
- Weather on the next days (Open-Meteo, your city in Settings)
- Countdowns: pin any future event; birthdays in the next month appear automatically
- Weekly recap card at the start of the weekend (or "This week" in the footer)
- Day view is now a timeline: drag to move, drag the bottom edge to resize, Undo, double-click empty space to add
- Confetti when you add events, a "Day clear!" moment, month page-flip, optional soft sounds
- AI voice: friendly, fun or formal
- Week starts on Saturday; weekend Friday + Saturday by default, with region presets and custom weekend days
- The menu bar icon gets an orange badge in the 5 minutes before a meeting
- Playful mode switch turns all of the fun off in one go

## v1.5.0

- Event card: click any event for time, place (opens Maps), notes, attendees and a Join button — with Edit, Duplicate, Delete and Open in Calendar
- Any day: click a day in the month (any month) to see its agenda; ← → step through days
- The AI box now answers questions ("what's on Thursday?", "am I free tomorrow afternoon?") and makes changes ("move lunch to 3pm", "cancel Friday's dentist") with a before/after preview
- Works offline / without a key for simple lines ("gym 7am", "dentist tomorrow 4:30pm")
- Search events across the past month and the next year (⌘F or /)
- Meeting alerts: a notification before each meeting with a Join button (1/5/10 min, at start, or off)
- Starts at login (toggle in Settings)
- Menu bar shows the next event (e.g. "Mon 09:00 · Follow up") — or pick date / icon only
- Holidays appear as small tags on the day instead of full rows; duplicate events from overlapping calendars are merged
- Weekends (Fri–Sat) shaded in the month view; arrow keys move through events, Return opens, ⌫ deletes

## v1.4.0

- New app and menu bar icon: the smiling calendar with the magic wand

## v1.3.3

- AI quick add box: 2 lines, bold accent border and a magic-wand AI icon (Return adds, Shift+Return for a new line)
- Bigger, easier-to-spot settings gear

## v1.3.2

- After pasting your AI key you now see a green "Key saved ✓" card (provider + masked key) with a Change button — no more blank box, no Save needed

## v1.3.1

- Fixed: Settings buttons overlapping text when scrolled to the bottom
- Cleaner release notes in the Update panel

## v1.3.0

- Settings: AI quick add (OpenRouter) moves to the top and is highlighted until a key is saved
- Paste your OpenRouter key and it saves instantly — stored in a local file, never the Keychain, never asked again
- Calendar connection collapses to one "✓ Connected" line once set up
- Save / Return bar no longer overlaps the settings content

## v1.2.0

- New default highlight colour: royal blue
- Settings → Highlight colour: pick from 8 presets or any custom colour (live preview)

## v1.1.0

- Days with events are highlighted in the month view (busier days darker)
- Settings: separate Save and Return to calendar buttons
- AI quick add now runs through OpenRouter (paste your OpenRouter key)
- "Calendar — brought to you by go.sala.company" title line
- Opens on the calendar view; hint when Google sync on the Mac needs a re-login

## v1.0.0

- Menu bar calendar with a frosted dropdown panel (⌃⌥C from anywhere)
- Works with Google, iCloud and Exchange calendars already on the Mac — 2-click setup
- AI quick add with preview (OpenAI or OpenRouter key)
- Up next card with countdown and Join button for Meet/Zoom/Teams
- Update button: installs new versions straight from GitHub
