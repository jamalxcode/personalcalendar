# Changelog

Release notes shown in the app's Update panel come from the matching section here.

## v2.3.1

- Fixed: pasting a notice like *"Cambridge IGCSE Results · Date: Thursday, 14 January 2027 · Time: 9:00 AM"* made an event for next Thursday at 14:00 with the whole text as its title. When the AI can't be reached, the offline reader now:
  - understands full dates — "14 January 2027", "January 14, 2027", "14/01/2027", "2027-01-14" — and a date beats a weekday name
  - reads "Date:" and "Time:" lines in pasted text, uses the first line as the title and keeps the whole text (with its links) in the notes, and doesn't mistake numbers like "14" for times
- When the AI is unavailable, the preview now says so (and why) instead of a message that disappears in two seconds — and the AI is tried once more on a dropped connection or a busy server before giving up

## v2.3.0

- **Typed events are added straight away**, with Undo — no more waiting in a preview you can forget. (The preview still appears when something clashes, for several events at once, or offline; turn it back on in Settings → Look → *Preview before adding*.) This also fixes a bug where, with the preview off, a typed event could silently disappear
- **Quick add from any app:** press **⌃⌥N** for a small AI box in the middle of the screen — type, press ↩, done. Esc closes it, ⌘↩ opens the full panel. Change or turn off the shortcut in Settings → Alerts
- **Alerts you can act on:** **Join** (online meetings), **Snooze 10 min**, and **Running late…**, which drafts an email to the other people in the meeting
- **Morning brief** at 8:00 (or the time you pick in Settings → Alerts): "3 events · first 9:00 Dentist · leave by 8:35 · 1 to-do overdue". Nothing is sent on an empty day
- **Less clutter at the top:** the "Smart Cal from go.sala.company" line is gone, the greeting is one line (click it to read it all), and **Up next** is now a highlight on the next event in the agenda — with its countdown and "leave by" — instead of a separate card
- **A "now" line** in today's events: what's over is dimmed above it, what's left is below
- **Smarter AI entry:** a trailing number like "catch up with Miteb **60**" is the length in minutes, not part of the title. A set of real sentences (English and Arabic) can now be run against the AI to catch mistakes
- **Beta versions:** Settings → About → *Get beta versions* to try new things before everyone else
- Fixed: the "To-do added · Undo" message (and other messages with Undo) never went away

## v2.2.10

- **The days run down the right side:** in the wide layout the right column is now an agenda that starts with today and carries on into tomorrow, the day after and beyond — each day with its own heading that stays in view while you scroll, more days loading as you go (up to a year). Click a day in the month to start there, ‹ › to move a day, **Today** to come back. The list that repeated today's events at the bottom is gone; the to-dos now sit under the month and the agenda gets the panel's full height
- Fixed: in v2.2.9 the calendar list in Settings → Calendar lost its layout (a style name clash)

## v2.2.9

- **Month and day side by side:** the panel is wider, with the month (and Up next) on the left and the selected day's timeline on the right — today when it opens, or any day you click; ‹ › step through days and **Today** comes back. To-dos and your upcoming events are below. You can drag events in the day timeline as before. Prefer the narrow panel? Settings → Look → Layout → **Compact — one column**
- Fixed: the red "now" line in a day timeline was drawn at the top when it was earlier than the first hour shown

## v2.2.8

- **Bigger headings in the list:** "To-do", "Today · Thu 12 Nov", "Tomorrow · Fri 13 Nov" and the other day headings — plus a single day's and the week's heading — are now the same size as today's date at the top. The day name is bold ("Today" in your highlight colour) and the date after it lighter; the event count and weather stay small on the right

## v2.2.7

- Fixed: when the menu bar is full (on MacBooks with a notch especially), macOS hides the menu bar item that was added last — and Smart Cal was always added last, at the far left, on every launch and after every update, so it could vanish completely. Now:
  - macOS remembers where Smart Cal sits: ⌘-drag it to a visible spot once and it stays there through restarts and updates
  - if macOS is hiding it anyway, Smart Cal opens its panel at launch and explains how to get the item back
  - opening Smart Cal again (Applications, Spotlight, Launchpad) shows the panel, and ⌃⌥C opens it in the middle of the screen when the menu bar item can't be seen

## v2.2.6

- **You can see an update happening:** after you click Update, a progress card shows the download percentage and each step — Download, Check, Install, Restart — in the panel and in Settings, and the menu bar reads "⬇︎ Updating 40%". If something goes wrong it says so, and that nothing was changed. (You'll see it from the *next* update on — this one is still installed by the previous version)
- **To-dos say when they're due:** "Today 21:00 · in 4 hours", "Tomorrow 09:00", "Wed 18 Nov, 09:00 · in 7 days", "Overdue by 2 days", or just the date when it's more than two weeks away. Before, a to-do moved to next Wednesday just said "Wednesday", which looked like today
- **Right-click to reschedule in the week window too** — the same menu as the panel, on timed events and all-day chips
- Fixed: right-clicking (or Control-clicking) an event in the day timeline or the week window also opened it

## v2.2.5

- **Right-click an event to reschedule it** (or Control-click), anywhere in the panel — upcoming list, day, week and timeline: Today, Tomorrow, In 2 days, In 3 days, Next week (each with its date), or **Pick a date…**. It keeps the event's time, length, notes and time zone, asks "this one or all future" for repeating events, warns about overlaps, and has Undo. The same menu also has Open, Edit…, Duplicate and Delete

## v2.2.4

- **Reschedule to-dos with a right-click** (or Control-click): Today, Tomorrow, In 2 days, In 3 days, Next week — each showing the date it lands on — or **Pick a date…**, **Remove date**, **Mark as done** and **Delete**. Days count from today, a to-do with a time keeps its time, and every change has Undo

## v2.2.3

- Fixed: notes from Google Calendar showed raw HTML (`<p>Ali&#x27;s last day…</p>`). They now show as plain text — paragraphs, line breaks, lists and links included — in the event card, the editor, search and the AI
- Fixed: long alerts read "240 h before"; they now say "10 days before" (and "1 week before", "2 hours before"…)
- Fixed: an event typed in the AI box but not yet added (with *Confirm before adding* on) was easy to lose — the day showed "Nothing planned". Now:
  - the waiting event shows in that day's list with an **Add** button, and its day gets a dashed ring in the calendar
  - double-clicking a day (or "Add something this day") moves the waiting event to that day, keeping its time, instead of typing "on Sun 15 Nov" into the box
  - the preview says "Not added yet" next to Cancel / Add

## v2.2.2

- The date at the top now includes the year — "Wednesday 11 November 2026" — in a lighter grey so the day and date still stand out. Long dates or a bigger text size shrink the line slightly instead of cutting it off

## v2.2.1

- **Today's date at the top:** opening Smart Cal shows the date in large letters as the first line — "Wednesday 11 November" — because sometimes you just need to know what day it is. It rolls over at midnight, follows the app's language (Arabic too), and the full date with the year shows when you point at it

## v2.2.0

- Removed Apple Intelligence and Siri / Shortcuts for now — they rely on features Apple hasn't officially released yet. The AI box uses your OpenRouter or OpenAI key (Settings → AI); simple lines still work offline without a key
- The app no longer includes the Smart Cal Assistant helper, so it's smaller and builds without Xcode 26
- Fixed: "push it back by a week" moved the event a week *earlier* (and so did any title with "back" in it, like "back massage"). Push back, postpone and delay now always move later; earlier needs "earlier", "sooner", "move it up" or "bring it forward"
- Fixed: AI edits could erase an event's notes (for example "make gym every Monday") and switch an event made in another time zone to local time. Notes and time zone are now kept unless you ask to change them
- Fixed: Undo after an AI edit that changed the repeat now puts the old repeat back
- Fixed: Arabic edit requests (rename, repeat, all-day, notes, length, time zone) were ignored by the "only change what you mention" check
- Fixed: moving an all-day event across a daylight-saving change no longer leaves it ending at 1 AM
- Plain http:// links in event notes now open
- Updates are checked before they're installed: the download must match the checksum published with the release, and the app's signature must be intact
- **Closer to a standard Mac app:**
  - A proper menu bar when Smart Cal is in the Dock: Smart Cal · File · Edit · View · Window · Help, with About, Settings (⌘,), New Event (⌘N), Year/Month/Week/Day (⌘1–⌘4), Go to Today (⌘T), Hide, Quit. The old generic menu's Help item opened the Wails website inside the app
  - The event card uses plain monochrome symbols instead of emoji, and its buttons follow Mac order: Delete on the left, Edit on the right, the rest (Duplicate, Pin as Countdown, Open in Calendar) in a ⋯ menu, so the row no longer wraps onto two lines
  - The Settings window is titled after the pane you're on (Calendar, AI, Look…) and closes with its red button or ⌘W, without an extra "Done" button
  - The panel footer shows the shortcut as ⌃⌥C instead of "ctrlaltc"
  - Section headings in the list ("Today · Tue 10 Nov", "To-do") are no longer in all caps, and the settings gear matches the other toolbar icons

## v2.1.1

- Fixed: "move this to 1 month from now" could move an event into the past (October) and also rename it, make it all-day and repeating. Relative moves ("1 month from now", "30 days from today", "2 weeks later", "push it by 3 days") are now worked out exactly by Smart Cal, keeping the event's time and length
- AI edits now only change what you mention — the title, place, calendar, repeat, all-day, length and notes stay as they were unless you ask
- An edit that would land in the past is refused with a hint, instead of being applied

## v2.1.0 — Apple Intelligence & Siri

- **Apple Intelligence in the AI box:** on Macs with Apple Intelligence, the AI runs on your Mac — free, private, works offline, no key needed. Settings → AI → AI engine: **Auto** (Apple Intelligence for everyday lines, OpenRouter for screenshots and long pasted text), **Apple Intelligence**, or **OpenRouter**. If Apple's model can't handle something, Auto falls back to OpenRouter when you have a key
- The greeting and event emojis also use Apple Intelligence, so the playful touches work without a key
- **Siri & Shortcuts:** “Add to Smart Cal”, “What's next in Smart Cal”, “Ask Smart Cal” — Siri asks what you'd like and Smart Cal does it (adds events, answers, moves events, adds to-dos, finds free time). The same three actions are in the Shortcuts app and Spotlight. Deleting stays in the app (it can't be undone by voice)
- Settings → AI shows whether Apple Intelligence is ready, and why not if it isn't

## v2.0.2 — second review

- Fixed: ⌘⇧W was registered as a system-wide shortcut, so it stole "Close Window" from Safari, Chrome, Finder and other apps. It now works only inside Smart Cal
- Fixed: updating could put the new app *inside* an existing "Smart Cal.app" if an old copy was still around; the old copy is now moved aside first. The updater also handles unusual folder names safely and cleans up its download
- Leave-now: places that can't be found (e.g. "Zoom") are no longer looked up again every 3 minutes
- The Settings window no longer redraws the hidden calendar on every refresh
- Meeting-alert memory is trimmed so it can't grow forever

## v2.0.1 — review fixes

- Fixed: typing your AI key while another setting saved could store a half-typed key (the key now saves only on Enter or paste)
- Fixed: the Day view jumped back to "now" every couple of minutes while you were scrolling
- Fixed: changes made in another app (or a refresh) now update the open Week/Day lists and the Year view straight away
- Fixed: the week and Settings windows now really reopen where you left them (and only if that spot is still on a screen)
- Settings are written safely (temporary file + rename), so a crash can't wipe your config or key
- Faster search: one fetch serves every keystroke for a minute
- Leave-now: drive-time lookups no longer hold the calendar lock while waiting on the network (no more brief freezes), and places are cached so Apple's geocoding limits aren't hit
- The Settings window no longer repeats the panel's background work (greeting, emoji lookups, refreshes)
- Small thread-safety fix for the menu bar icon

## v2.0.0 — a proper Mac app

- **Settings is now its own window** (⌘, from anywhere) with toolbar tabs, and changes **save as you make them** — no Save button
- **About Smart Cal** opens the standard macOS About panel; the menu-bar right-click menu follows Apple's order (About · Check for Updates · Settings · … · Help · Quit) with the usual shortcuts
- **Follows your Mac:** "System" highlight colour and "Same as the Mac" clock (12/24-hour); Reduce Motion turns off animations and confetti; Increase Contrast strengthens lines and text
- **Monochrome menu bar icon** option (like Apple's icons), and **Show in the Dock** option for crowded menu bars
- **Asks before starting at login** instead of turning it on by itself
- **Shortcuts:** ⌘N new event, ⌘Z undo (⌘⇧Z redo for moves), ⌘1–⌘4 Year/Month/Week/Day, ? for help
- **Help & shortcuts** sheet (menu: Smart Cal Help)
- **Arabic** with right-to-left layout and Arabic dates (Settings → Look → Language), including an Arabic AI greeting
- VoiceOver labels on icon buttons and visible keyboard focus
- The week and Settings windows remember their size and position
- App icon sits on a soft rounded-square tile like other Mac apps

## v1.9.0

- **Menu bar choices:** a new Menu bar tab in Settings with a live preview and 10 styles, each shown as it will look — Compact, Minimal, Countdown + title, Time + title, Full, Today only, Last hour only, Date, Date + compact, Icon only
- **Fine-tune:** title length, how far ahead, countdown or clock, emoji on/off, hide during meetings, and when the orange dot appears
- Your current choice carries over

## v1.8.8

- The menu bar shows just the text ("👥 2h Meet") — the app icon is hidden to save space, and comes back only when there's nothing coming up
- In the 5 minutes before a meeting the text gets an orange dot (🟠 👥 4m Meet)

## v1.8.7

- **Compact menu bar** (now the default): emoji + countdown + one word — "👥 2h Meet", "🦷 25m Dentist", "👥 now Meet", and for later days "✈️ Su 8a KU101". Takes about a third of the space, leaving room for your other menu bar apps. The old styles are still in Settings → Alerts

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
