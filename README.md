# Personal Calendar

A Fantastical-style **macOS menu bar calendar**, written in Go with **Wails v3** (native WebKit UI), that talks to **Google Calendar** and uses **OpenAI** to understand plain-English event entry.

- Menu bar shows what's next: `Standup in 12m`, `● Lunch with Ali · 25m left`, or today's date when the day is clear.
- Click it (or press **⌃⌥C** anywhere) and a frosted popover drops down under the icon: mini month with event dots, quick add, an "Up next" card with live countdown + **Join** for Google Meet, and your upcoming events grouped by day. Click away and it hides. Right-click the icon for Refresh / Settings / Quit.
- Click a day in the mini month to jump to it; double-click a day to start an event on that date.
- **Quick add**: type `Dinner with Sara Thursday 8pm at Mais Alghanim`, hit Return, check the preview card (title is editable, calendar is pickable), hit Return again. Done.
  - Handles durations (`for 30m`), all-day (`Ali's birthday Friday`), multi-day (`Dubai trip 12–15 Dec`), locations, reminders (`remind me 1h before`), several events at once, and picks a calendar if you name one (`… on Work`).
  - Knows the Kuwait weekend is Fri–Sat.
- Hover an event for Join / delete (with confirmation); click it to open in Google Calendar.
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

## OpenAI setup

Create an API key at <https://platform.openai.com/api-keys> and paste it in Settings. (A ChatGPT Plus subscription doesn't include API access — the API is pay-as-you-go; each event parse costs a tiny fraction of a cent with `gpt-4o-mini`.) The model name is editable in Settings.

## Google setup

Google requires every app to have its own OAuth client. One-time, ~3 minutes:

1. Go to <https://console.cloud.google.com/> → create a project (e.g. "Personal Calendar").
2. **APIs & Services → Library** → search **Google Calendar API** → **Enable**.
3. **APIs & Services → OAuth consent screen** (Google Auth Platform) → External → app name "Personal Calendar", your email → save. Under **Audience / Test users** add your own Gmail.
4. **Clients → Create client → Application type: Desktop app** → Create → **Download JSON**.
5. In the app: Settings → **Import credentials.json…** → pick the downloaded file → **Connect Google** → approve in the browser.

> Tip: while the consent screen is in *Testing*, Google expires the login after 7 days. To avoid re-connecting weekly, press **Publish app** on the Audience page (you'll see an "unverified app" warning once when connecting — that's you, click *Continue*).

## Where things are stored

`~/Library/Application Support/PersonalCalendar/` — `config.json` (settings + OpenAI key), `credentials.json`, `token.json` (Google refresh token). All files are `0600`. Nothing is sent anywhere except Google and OpenAI.

## Code map

| File | What |
|---|---|
| `main.go` | Wails app, menu bar item (label + icon + right-click menu), popover window attached to the tray, global hotkey, refresh loop |
| `service.go` | `CalService` — the Go methods the UI calls (state, parse, add, delete, settings, Google connect) |
| `frontend/` | the popover: `index.html`, `style.css`, `app.js` (talks to Go via `/wails/runtime.js`) |
| `gcal.go` | Google OAuth (loopback + PKCE), list / insert / delete events |
| `ai.go` | OpenAI structured-output parsing of free text → events |
| `config.go` | settings file |
| `icons.go` | menu bar template icon + app icon, drawn in code |
| `build.sh` / `build.command` | compile (`-tags production,private_mac_apis`), bundle as a menu-bar-only `.app`, ad-hoc sign, copy to Desktop |
