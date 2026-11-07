# Lessons learned

- **ChatGPT ≠ OpenAI API.** A ChatGPT subscription (and ChatGPT's own Google Calendar connector) can't be called from your own app. Apps need an API key from platform.openai.com.
- **Google OAuth needs your own client.** Desktop-app OAuth client + loopback redirect (`http://127.0.0.1:<random port>/`) + PKCE is the modern flow; the old "copy/paste code" (OOB) flow is gone.
- **Testing-mode tokens die after 7 days.** Publish the consent screen (no verification needed for personal use) to keep the refresh token alive.
- **Menu-bar-only app** = `LSUIElement` = true in Info.plist (no Dock icon, no app menu).
- **Menu bar text in Fyne:** Fyne only exposes a tray *icon* + menu, but it uses `fyne.io/systray` underneath, so calling `systray.SetTitle(...)` directly puts text next to the icon. `systray.SetTemplateIcon` makes the icon follow light/dark menu bar.
- **Pin Fyne to 2.5.x.** Fyne 2.6+ requires UI updates from goroutines to be wrapped in `fyne.Do`; this code updates widgets from background goroutines, which 2.5 allows.
- **Structured outputs** (`response_format: json_schema, strict: true`) make the LLM return exactly the fields we need — no regex parsing. Always give the model the current date, weekday and UTC offset, or "next Tuesday" goes wrong.
- **List upcoming events with `timeMin = now`** — Google filters by *end* time, so in-progress meetings and today's all-day events are still returned.
- **Native file picker from Go without cgo glue:** `osascript -e 'POSIX path of (choose file …)'`.
- **Builds happen on the Mac.** Fyne needs cgo + Apple frameworks; cross-compiling from Linux isn't worth it.
