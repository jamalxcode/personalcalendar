# Smart Cal

*from [go.sala.company](https://go.sala.company)*

A Fantastical-style **macOS menu bar calendar**, written in Go with **Wails v3** (native WebKit UI), that works with your **Google Calendar** (through macOS's own calendar sync — no Google Cloud setup) and uses **OpenAI or OpenRouter** to understand plain-English event entry.

- **Menu bar** shows the next event compactly — `👥 2h Meet`, `🦷 25m Dentist`, `✈️ Su 8a KU101` — with no icon while there is text. Ten styles plus fine-tune options in Settings → Menu bar.
- **Panel** (click the icon or press **⌃⌥C**, configurable): the AI box on top, then Year / Month / Week / Day views with highlighted event days (any month, weekends shaded, holiday days underlined), the AI box, an *Up next* card with countdown + **Join**, and your upcoming events by day. Holidays show as small tags.
- **Click a day** for that day's agenda; **click an event** for its card — time, place (opens Maps), notes, attendees, Join — with **Edit / Duplicate / Delete / Open in Calendar**.
- **AI box** (OpenRouter): add (`dinner with Sara Thursday 8pm at Mais Alghanim`), ask (`what's on Thursday?`, `am I free tomorrow afternoon?`) or change (`move lunch to 3pm`, `cancel Friday's dentist`) — changes show a before/after preview first. Simple lines work offline without a key.
- **Search** (⌘F or `/`) across the past month and the next year.
- **Meeting alerts** with a Join button, **starts at login**, **highlight colour** of your choice, self-**updates** from GitHub.
- **Playful mode** (one switch): a mascot whose mood follows your day, an AI greeting, emojis on events, weather, countdowns (pin events; birthdays automatic), a weekly recap, confetti, optional sounds, and a choice of AI voice.
- **Day timeline**: drag events to move them, drag the bottom edge to resize, Undo, double-click to add.
- **Repeating events, clash warnings and free-slot finding**, by typing or in the editor; editing a repeat asks "this one or all future".
- **Paste or drop** text and screenshots (flight bookings, invites) to create events; **Undo** for every change.
- **To-dos** from Apple Reminders, default alerts, and **Leave now** alerts from Apple Maps drive time.
- **Big week window** (⌘⇧W): drag across days, resize, double-click to add.
- **A proper Mac app:** Settings window that saves instantly (⌘,), standard About panel, Apple-ordered menu, system accent colour and clock, Reduce Motion / Increase Contrast, VoiceOver labels, monochrome icon or Dock icon options, Arabic with right-to-left layout.
- **Week settings**: week starts Saturday, weekend Fri+Sat by default — presets for the Gulf, Saudi, UAE, Egypt, Europe and US, or pick your own days.
- Keys: `⌃⌥C` toggle · `⌘N` new · `⌘Z`/`⌘⇧Z` undo/redo · `⌘1–4` views · `?` help · `↑↓` move · `↩` open · `⌫` delete · `← →` previous/next day · `⌘F` search · `⌘R` refresh · `⌘T` today · `⌘,` settings · `Esc` back/close.

Design decisions and the conversations behind them: [DESIGN.md](DESIGN.md).

## Build (on your Mac)

Needs Go 1.21+ (`brew install go`; it auto-fetches the Go 1.26 toolchain Wails v3 needs) and Xcode command-line tools (`xcode-select --install`). No Node/npm needed — the UI is plain HTML/CSS/JS.

```bash
git clone https://github.com/jamalxcode/personalcalendar.git
cd personalcalendar
./build.sh --run        # builds "Smart Cal.app" onto your Desktop and opens it
```

Drag it into /Applications and add it to **System Settings → General → Login Items** to start at login.

## Setup (2 clicks)

1. **Allow** — on first launch macOS asks for calendar access. Click Allow.
2. **Google account** — if your Google account isn't on the Mac yet, click **Add Google…** in the app. It opens System Settings → Internet Accounts; sign in with Google and keep *Calendars* ticked. Your calendars show up in the app by themselves.

Then paste an **AI key** in Settings for quick add: an OpenAI key (`sk-…`, from <https://platform.openai.com/api-keys>) or an OpenRouter key (`sk-or-…`, from <https://openrouter.ai/keys>) — the app detects which. The model name is editable (default `gpt-4o-mini`; on OpenRouter it becomes `openai/gpt-4o-mini`).

Works with every calendar macOS syncs — Google, iCloud, Exchange — and you choose which ones to show and which one new events go to.

## Updates

The app checks GitHub for a newer release at launch and every 6 hours. When one exists an **Update to vX** pill appears in the panel footer (also Settings → Updates, or right-click the menu bar icon → Check for Updates…). One click downloads it, swaps the app in place and relaunches.

**Publishing a new version:** bump `VERSION`, add a `## vX.Y.Z` section to `CHANGELOG.md` (those lines become the release notes shown in the app), push to `main`. The *Release* workflow builds the app on a macOS runner and attaches `PersonalCalendar-X.Y.Z-macos-arm64.zip` to a new GitHub release.

> macOS may ask for calendar access again after an update — the build isn't signed with an Apple Developer ID, so each version looks new to the privacy system.

## Where things are stored

`~/Library/Application Support/PersonalCalendar/config.json` (folder name kept from the original name) (settings + AI key, file mode `0600`; deliberately not the Keychain, so there are no password prompts). Calendar data stays in macOS; only the text you type into quick add is sent to OpenAI/OpenRouter.

## Code map

| File | What |
|---|---|
| `main.go` | Wails app, menu bar item (label + icon + right-click menu), popover window attached to the tray, global hotkey, refresh loop |
| `service.go` | `CalService` — the Go methods the UI calls (state, setup steps, parse, add, delete, settings) |
| `frontend/` | the popover: `index.html`, `style.css`, `app.js` (talks to Go via `/wails/runtime.js`) |
| `eventkit_darwin.m` | Objective-C bridge to Apple EventKit: permission, calendars, events, add, delete, change notifications |
| `calendar_darwin.go` | Go wrapper over the bridge (+ Meet/Zoom/Teams link detection) |
| `ai.go` | OpenRouter/OpenAI structured output: one call decides add / answer / update / delete |
| `fun.go` | weather (Open-Meteo), greeting, emoji cache, countdowns |
| `parse_local.go` (+ test) | offline parser for simple lines when there's no key or no network |
| `config.go` | settings file |
| `icons.go` | menu bar template icon + app icon, drawn in code |
| `updater.go` | checks GitHub releases, downloads the zip, swaps the bundle, relaunches |
| `.github/workflows/release.yml` | builds + publishes a release when `VERSION` changes |
| `.github/workflows/ci.yml` | compiles + tests every push to a non-main branch on a macOS runner |
| `build.sh` / `build.command` | compile (`-tags production,private_mac_apis`), bundle as a menu-bar-only `.app`, ad-hoc sign, copy to Desktop |
