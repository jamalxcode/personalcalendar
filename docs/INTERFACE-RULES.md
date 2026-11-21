# Smart Cal — interface rules

How Smart Cal looks and behaves. New UI follows these; when a rule changes, change it here too. What the app *does* is in [BUSINESS-RULES.md](BUSINESS-RULES.md).

## 1. Be a Mac app

- **System font**, 13 pt body; numbers in tables and times use tabular figures.
- **Arrow cursor** on controls (not the web's pointing hand); text can't be selected on controls, only in content (titles, notes, AI answers).
- **Light and dark**, your highlight colour (or the Mac's), **Reduce Motion** (animations stop) and **Increase Contrast** (stronger lines and text).
- **VoiceOver labels** on every icon-only button; visible keyboard focus.
- **Arabic** flips the layout right-to-left.
- **Monochrome symbols** in working UI (the event card, menus). Emoji only where Playful mode asks for them (event emojis, the mascot, the greeting).
- **Mac keyboard symbols** in text: ⌃ ⌥ ⇧ ⌘ in that order ("⌃⌥C"), never "ctrl+alt+c".
- **Standard menus.** The menu bar item's right-click menu in Apple's order (About · Check for Updates · Settings · … · Help · Quit). The app menu bar (when shown in the Dock): Smart Cal · File · Edit · View · Window · Help, with the usual shortcuts.
- **Standard About panel**, not a custom window.

## 2. Windows

| Window | Rules |
|---|---|
| **Panel** | Drops down under the menu bar item (centred when macOS hides the item). Wide layout 760 pt, compact 380 pt; shrinks to fit the screen; text size scales it. Closes when you click elsewhere or press Esc — so whenever Smart Cal opens it on its own, it activates itself first. |
| **Settings** | Its own window, toolbar tabs, titled after the selected pane (Calendar, AI, Look…). Saves as you change things — no Save or Done button; closes with the red button or ⌘W. Secret fields save only on Enter or paste, never half-typed. |
| **Week** | A normal resizable window (⌘⇧W); remembers its size and place. |
| **Quick add** | A floating box in the middle of the screen (⌃⌥N), only the AI box; grows and shrinks with its content; closes after adding, on Esc, or on a click elsewhere; ⌘↩ opens the panel. |

## 3. The panel, top to bottom

1. **Today's date**, 22 pt bold, full width: weekday in the highlight colour, day and month, the year muted, today's weather small at the end — "**Friday** 13 November 2026 ☀️ 24°". A soft tint behind it follows the time of day (dawn, day, golden hour, evening, night; none with Increase Contrast). The Hijri date and the greeting are in its tooltip. Shrinks (to 15 pt at most) rather than being cut off. On occasions a line under it: "🇰🇼 Happy National Day!", "🌙 Ramadan Kareem · day 12 · Iftar 17:47 · Suhoor ends 4:54".
2. **Update banner** when a newer version exists ("Smart Cal vX is available · Update now"), or the green **"✓ Updated to vX — all good"** once after an update.
3. **The AI box** — one line when empty, growing up to five lines with what you type — with the "working" line and, when the AI couldn't do it, the **reason**, both under the box.
4. **The preview** of what the AI understood (when there is one), with "Not added yet · Cancel · Add".
5. **A one-line greeting** — Playful mode, 5:00–12:00 only (click to read it all).
6. **Wide layout:** left — the month (or Year), countdowns and the to-dos; right — the agenda (Month/Year), the week as lines (Week) or the day's timeline (Day). **Compact:** the month or a week strip, to-dos and the upcoming list in one column.
7. **A tip** (one per opening, each shown once) just above the bottom line.
8. **The bottom line:** shortcut hint · This week · version (or "· Update to vX", clickable) · ⚙ settings · ⏻ quit. No refresh button (it refreshes itself; ⌘R). Never wraps: in the narrow layout the shortcut hint gives way.

## 4. Lists and the agenda

- **Headings are as big as the date** (22 pt; 19 pt in narrow side columns): day name bold, the date part muted — "**Today** · Thu 12 Nov" — "Today" in the highlight colour. They stay pinned while you scroll through their day. Counts and weather stay small, on the right.
- **The agenda** starts today (or the clicked day) and runs on, loading two more weeks as you near the end (up to a year). Empty days are skipped, except the first.
- **Up next** is a highlight on the next event's row with a pill: "Up next · in 26m · leave by 8:25", or "Now · 20m left".
- **The now line** (red, "Now · 10:42") sits between today's finished events (dimmed) and what's left.
- **Events waiting to be added** show in their day with a dashed outline and an Add button, and their day gets a dashed ring in the month.
- **Holidays** are small tags under the day heading, not rows (unless you choose rows).
- **+** appears on a day heading (and a month day) on hover. Clicking a day already in the agenda scrolls to it and flashes its heading.
- **Compact rows** (setting): one line per event — time and title; place, end time and calendar hidden.
- **Countdowns** are chips with a ring that fills over the last 30 days.
- **Empty days** get a friendly line, not just "Nothing planned".

## 5. Buttons, menus and cards

- **Destructive on the far left, the main action on the far right**, the rest in a **⋯** menu — so a row of buttons never wraps.
- **Pop-up menus look and behave like Mac menus:** right-click opens at the pointer; nothing is highlighted when opened with the mouse, the first item when opened from the keyboard; ↑↓ move, Esc closes. Hints (dates) are grey on the right; destructive items are red, last, after a separator.
- **Confirmations** only for things Undo can't fix; Cancel on the left, the verb on the right ("Delete", not "Yes").
- **Labels are verbs and plain words:** "Update now", "Pin as Countdown", "Pick a date…" (an ellipsis when more input follows).

## 6. Telling you what's happening

- **Working:** while the AI thinks — the busy cursor over the whole window, a moving bar in the AI box, and a line: "Thinking…", "Still working…" (6 s), "Waiting for the AI…" (20 s).
- **Updating:** a progress card (download %, Download · Check · Install · Restart) in every open window, and "⬇︎ Updating 40%" in the menu bar.
- **Done:** a short message at the bottom — 2.4 s, errors 5 s; with **Undo**, 6 s, and it waits while the pointer is on it.
- **Anything you need to act on or understand stays until you close it** — the AI's failure reason, the "Updated" banner, the "menu bar is full" explanation. Never only a passing message.
- **Reasons, not codes:** "OpenRouter says your credits have run out (402) — add credits at openrouter.ai/credits", not "AI unavailable".

## 7. Words, dates and times

- **Relative and absolute together:** "Wed 18 Nov, 09:00 · in 6 days", "Today 21:00 · in 4 hours", "Overdue by 13 hours"; just the date from two weeks out. A weekday alone only for the next few days.
- **Times** follow the Mac's 12/24-hour setting (or your choice); durations are short ("1h", "25m").
- **Plain English, short sentences**, sentence case for headings. Arabic translations come from the dictionary in `i18n.js`.

## 8. Colour

- **The highlight colour** (yours, or the Mac's) for today, the selection, primary buttons, the update banner and links.
- **Red** (#d93036) for destructive actions, errors and the now line; **green** (#2fa36b) for success; **orange** for warnings (the AI read something without AI, a meeting is about to start).
- **Contrast** holds in light and dark; tinted backgrounds are low-alpha versions of the accent.

## 9. Keyboard

- **Global shortcuts only for opening Smart Cal:** ⌃⌥C (panel) and ⌃⌥N (quick add), both changeable. Never steal common app shortcuts (⌘W, ⌘N, ⌘⇧W…) globally, and avoid ⌃⌥Space (it switches keyboard languages).
- **In the panel:** ⌘N new · ⌘Z / ⌘⇧Z undo/redo · ⌘1–4 views · ⌘F or / search · ⌘R refresh · ⌘T today · ⌘, settings · ⌘W close · ⌘Q quit · ? help · ↑↓ ↩ ⌫ in the agenda or list, then T tomorrow · E edit · Space open · ← → days · ↑ in the AI box = what you typed before.
- **The mouse:** ⌥-click the menu bar item = quick add; right-click any event or to-do = reschedule menu.
- **Esc steps back** one level at a time: menu → confirm → event card → settings → preview → typed text → search → day → close the panel.

## 10. Checking the interface

- Every UI change is looked at in a browser preview with a mocked backend before it ships: **light and dark, wide and compact**, and the states it touches (empty, busy, error, update available).
- What only a Mac can show (the menu bar, notifications, real windows, the busy cursor) is called out as untested until it's seen on one.
