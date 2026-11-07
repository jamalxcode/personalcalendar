# Personal Calendar

A Fantastical-style **macOS menu bar calendar**, written in Go with **Wails v3** (native WebKit UI), that works with your **Google Calendar** (through macOS's own calendar sync — no Google Cloud setup) and uses **OpenAI or OpenRouter** to understand plain-English event entry.

- Menu bar shows what's next: `Standup in 12m`, `● Lunch with Ali · 25m left`, or today's date when the day is clear.
- Click it (or press **⌃⌥C** anywhere) and a frosted popover drops down under the icon: mini month with event dots, quick add, an "Up next" card with live countdown + **Join** for Google Meet, and your upcoming events grouped by day. Click away and it hides. Right-click the icon for Refresh / Settings / Quit.
- Click a day in the mini month to jump to it; double-click a day to start an event on that date.
- **Quick add**: type `Dinner with Sara Thursday 8pm at Mais Alghanim`, hit Return, check the preview card (title is editable, calendar is pickable), hit Return again. Done.
  - Handles durations (`for 30m`), all-day (`Ali's birthday Friday`), multi-day (`Dubai trip 12–15 Dec`), locations, reminders (`remind me 1h before`), several events at once, and picks a calendar if you name one (`… on Work`).
  - Knows the Kuwait weekend is Fri–Sat.
- Hover an event for Join / delete (with confirmation); click it to open in Calendar.
- Settings: choose which calendars to show, which one new events go to, 12/24h, days ahead, preview on/off.
- Shortcuts: `⌃⌥C` toggle popover (global) · `⌘R` refresh · `⌘T` today · `⌘,` settings · `Esc` cancel preview / hide.

## Build (on your Mac)

Needs Go 1.21+ (`brew install go`; it auto-fetches the Go 1.26 toolchain Wails v3 needs) and Xcode command-line tools (`xcode-select --install`). No Node/npm needed — the UI is plain HTML/CSS/JS.

```bash
git clone https://github.com/jamalxcode/personalcalendar.git
cd personalcalendar
./build.sh --run        # builds "Personal Calendar.app" onto your Desktop and opens it
```

Drag it into /Applications and add it to **System Settings → General → Login Items** to start at login.

## Setup (2 clicks)

1. **Allow** — on first launch macOS asks for calendar access. Click Allow.
2. **Google account** — if your Google account isn't on the Mac yet, click **Add Google…** in the app. It opens System Settings → Internet Accounts; sign in with Google and keep *Calendars* ticked. Your calendars show up in the app by themselves.

Then paste an **AI key** in Settings for quick add: an OpenAI key (`sk-…`, from <https://platform.openai.com/api-keys>) or an OpenRouter key (`sk-or-…`, from <https://openrouter.ai/keys>) — the app detects which. The model name is editable (default `gpt-4o-mini`; on OpenRouter it becomes `openai/gpt-4o-mini`).

Works with every calendar macOS syncs — Google, iCloud, Exchange — and you choose which ones to show and which one new events go to.

## Where things are stored

`~/Library/Application Support/PersonalCalendar/config.json` (settings + AI key, file mode `0600`; deliberately not the Keychain, so there are no password prompts). Calendar data stays in macOS; only the text you type into quick add is sent to OpenAI/OpenRouter.

## Code map

| File | What |
|---|---|
| `main.go` | Wails app, menu bar item (label + icon + right-click menu), popover window attached to the tray, global hotkey, refresh loop |
| `service.go` | `CalService` — the Go methods the UI calls (state, setup steps, parse, add, delete, settings) |
| `frontend/` | the popover: `index.html`, `style.css`, `app.js` (talks to Go via `/wails/runtime.js`) |
| `eventkit_darwin.m` | Objective-C bridge to Apple EventKit: permission, calendars, events, add, delete, change notifications |
| `calendar_darwin.go` | Go wrapper over the bridge (+ Meet/Zoom/Teams link detection) |
| `ai.go` | OpenAI / OpenRouter structured-output parsing of free text → events |
| `config.go` | settings file |
| `icons.go` | menu bar template icon + app icon, drawn in code |
| `build.sh` / `build.command` | compile (`-tags production,private_mac_apis`), bundle as a menu-bar-only `.app`, ad-hoc sign, copy to Desktop |
