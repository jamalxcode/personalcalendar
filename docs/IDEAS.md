# Smart Cal — ideas

Things worth doing that haven't been done yet. Pick by number, like the critiques. When one ships, move it to [DESIGN.md](../DESIGN.md) with the date and what was decided.

## From the critique of 12 Nov 2026 (not picked yet)

1. **Sign and notarize with an Apple Developer ID** ($99/year). No "Open Anyway" on install, stable permissions, dependable notifications, and updates whose origin can be proven — the single biggest step from "personal project" to "proper Mac app".
2. **Prove the menu bar fixes on real hardware** (v2.2.7, v2.3.9) — and if the item still hides, a small floating "today" chip as a fallback.
3. **"Copy diagnostics" in About** — version, permissions, whether the menu bar item is visible, the last AI result, recent errors — so a problem report is one paste, not guesswork.
4. **Run the browser-preview UI checks in CI before every release**, like the Go tests. They already exist (a Playwright script with a mocked backend); they just aren't in the repo or the workflow. Needed before splitting `app.js` further.
10. **Hijri dates and Islamic occasions** next to the Gregorian date (and optional prayer times as faint marks in the day) — very Kuwait, and something Fantastical doesn't do well.
13. **Birthdays from Contacts**, and **event templates** ("Gym" → 1 hour, a set calendar, 30-minute alert).
14. **A second time zone** in the day view when you're travelling or have a flight coming up.

## Done from the UI critique (v2.4.0, 21 Nov 2026)

All 17: shorter top, Week/Day in the right column, scroll-to-day, + to add, tips, Advanced models · agenda keyboard, compact rows, AI history, Services menu, no refresh button, ⌥-click quick add · time-of-day tint, small wins, weekly recap vs last week, local occasions with Ramadan times, countdown rings. (Idea 10 above — Hijri dates and occasions — is partly done: occasions and the Hijri date on hover.)

## Ideas that came up along the way

- **Drag to move in the agenda** (the wide layout's right column) — right-click covers it today; dragging an event onto another day in the month would be natural.
- **Snooze choices** (5 / 10 / 30 minutes) instead of a single 10-minute snooze, and **Running late by Messages** as well as email.
- **Leave-now from where you are** (current location) rather than a fixed starting address.
- **A morning brief you can open** — clicking the notification could show a short "today" card with the drive times and to-dos.
- **Grow the AI test set from real misses** — every sentence the AI gets wrong becomes a line in `testdata/ai_eval.json`; run it with a key before AI changes.
- **Apple Intelligence on the Mac** again, once Apple's on-device model is officially released and stable — the v2.1 design (a small Swift helper) is in the design log and lessons.
- **A Smart Cal widget** for the desktop / Notification Center (WidgetKit, Swift) showing today and Up next.
- **Split `frontend/app.js`** into views, agenda, AI box, settings and menus — after idea 4, so the split can be checked automatically.
- **List Smart Cal on Flattery** (flattery.sala.company) as the free Fantastical-style menu bar calendar — and add Fantastical to Flattery's product searches.

## Decided against (so they don't come back by accident)

- **Availability sharing / booking pages** — the calendar stays private.
- **Storing keys in the Keychain** — ad-hoc-signed apps get password prompts; a 0600 file in Application Support instead.
- **Unreleased Apple features in the shipped app** — Siri and Apple Intelligence were removed in v2.2 for that reason.
- **Always previewing typed events** — a preview you can forget is how a meeting went missing; add straight away with Undo instead.
