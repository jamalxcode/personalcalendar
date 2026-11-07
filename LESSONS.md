# Lessons learned

- **ChatGPT ≠ OpenAI API.** A ChatGPT subscription (and ChatGPT's own Google Calendar connector) can't be called from your own app. Apps need an API key from platform.openai.com.
- **Google OAuth needs your own client.** Desktop-app OAuth client + loopback redirect (`http://127.0.0.1:<random port>/`) + PKCE is the modern flow; the old "copy/paste code" (OOB) flow is gone.
- **Testing-mode tokens die after 7 days.** Publish the consent screen (no verification needed for personal use) to keep the refresh token alive.
- **Menu-bar-only app** = `LSUIElement` = true in Info.plist (no Dock icon, no app menu).
- **Fyne → Wails v3.** v1 used Fyne: it can't anchor a window under the menu bar icon and its self-drawn widgets never look like macOS. Wails v3 gives `SystemTray.AttachWindow(win)` (a real dropdown popover that hides on focus loss), `SetLabel` for menu bar text, `SetTemplateIcon` for light/dark, and a WebKit UI styled with plain CSS.
- **No npm needed with Wails v3.** Serve plain files with `application.AssetFileServerFS(embedFS)` and load `<script type="module" src="/wails/runtime.js">`; call Go with `Call.ByName("main.CalService.Method", ...)` (package `main` binds as `main.`). Go → JS pushes use `app.Event.Emit("state", data)` / `Events.On("state", ev => ev.data)`.
- **Frosted popover needs `-tags private_mac_apis`.** Without it the webview stays opaque above the translucent backdrop. Fine for a personal app (not App Store).
- **Wails v3 needs Go 1.26.** An older Go downloads the right toolchain automatically (GOTOOLCHAIN=auto).
- **No `window.confirm` in the webview** — build a small in-page confirm instead.
- **The macOS file picker steals focus** and hides a HideOnFocusLost popover; re-show it afterwards.
- **Structured outputs** (`response_format: json_schema, strict: true`) make the LLM return exactly the fields we need — no regex parsing. Always give the model the current date, weekday and UTC offset, or "next Tuesday" goes wrong.
- **List upcoming events with `timeMin = now`** — Google filters by *end* time, so in-progress meetings and today's all-day events are still returned.
- **Native file picker from Go without cgo glue:** `osascript -e 'POSIX path of (choose file …)'`.
- **Builds happen on the Mac.** Fyne needs cgo + Apple frameworks; cross-compiling from Linux isn't worth it.
