import { Call, Events } from "/wails/runtime.js";

// ---------- Go bridge ----------
const SVC = "main.CalService.";
const go = (method, ...args) => Call.ByName(SVC + method, ...args);
const errMsg = (e) => (e && (e.message || e.toString())) || "Something went wrong";

// ---------- state ----------
let S = { events: [], calendars: [], use24h: false, connected: false, hasKey: false, hasCreds: false };
let drafts = [];
let busy = false;
let viewMonth = startOfMonth(new Date());
let selectedDay = null;
let settingsCache = null;

const $ = (id) => document.getElementById(id);
const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
};

// ---------- dates ----------
function startOfMonth(d) { return new Date(d.getFullYear(), d.getMonth(), 1); }
function startOfDay(d) { return new Date(d.getFullYear(), d.getMonth(), d.getDate()); }
function addDays(d, n) { const x = new Date(d); x.setDate(x.getDate() + n); return x; }
function sameDay(a, b) { return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate(); }
function dayKey(d) { return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`; }

function fmtTime(d) {
  if (S.use24h) return d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
  const h = d.getHours() % 12 || 12, m = d.getMinutes(), ap = d.getHours() < 12 ? "AM" : "PM";
  return m === 0 ? `${h} ${ap}` : `${h}:${String(m).padStart(2, "0")} ${ap}`;
}
function dur(ms) {
  const m = Math.max(0, Math.round(ms / 60000));
  if (m < 1) return "now";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60), r = m % 60;
  return r ? `${h}h ${r}m` : `${h}h`;
}
function dayLabel(d, now = new Date()) {
  if (sameDay(d, now) || d < now) return "Today";
  if (sameDay(d, addDays(now, 1))) return "Tomorrow";
  return d.toLocaleDateString("en-GB", { weekday: "long" });
}
function dayHead(d) {
  const now = new Date();
  const lbl = dayLabel(d, now);
  const date = d.toLocaleDateString("en-GB", { day: "numeric", month: "short" });
  if (lbl === "Today" || lbl === "Tomorrow") return `${lbl} · ${d.toLocaleDateString("en-GB", { weekday: "short" })} ${date}`;
  if (d - now < 6 * 864e5) return `${lbl} · ${date}`;
  return d.toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "short" });
}
function whenText(start, end, allDay) {
  const s = new Date(start), e = new Date(end);
  const day = dayHead(startOfDay(s));
  if (allDay) {
    const n = Math.round((startOfDay(e) - startOfDay(s)) / 864e5);
    return n > 1 ? `${day} · all day · ${n} days` : `${day} · all day`;
  }
  return `${day} · ${fmtTime(s)} – ${fmtTime(e)}`;
}
const evs = () => S.events.map((e) => ({ ...e, s: new Date(e.start), e: new Date(e.end) }));

// ---------- toast & confirm ----------
let toastTimer;
function toast(msg, isErr = false) {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast" + (isErr ? " err" : "");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hidden"), isErr ? 5000 : 2400);
}
function confirmBox(html) {
  return new Promise((res) => {
    $("confirmText").innerHTML = html;
    $("confirm").classList.remove("hidden");
    const done = (v) => { $("confirm").classList.add("hidden"); $("confirmYes").onclick = $("confirmNo").onclick = null; res(v); };
    $("confirmYes").onclick = () => done(true);
    $("confirmNo").onclick = () => done(false);
    $("confirmYes").focus();
  });
}
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

// ---------- mini calendar ----------
// ---------- month grid ----------
const monthCache = new Map(); // "y-m" → events (with s/e dates) for months other than the loaded range
const mkey = (d) => `${d.getFullYear()}-${d.getMonth()}`;
const withDates = (list) => list.map((e) => ({ ...e, s: new Date(e.start), e: new Date(e.end) }));

async function loadMonth(m, force = false) {
  const k = mkey(m);
  if (monthCache.has(k) && !force) return;
  const from = addDays(m, -7), to = addDays(new Date(m.getFullYear(), m.getMonth() + 1, 1), 7);
  try {
    monthCache.set(k, withDates(await go("EventsBetween", from.toISOString(), to.toISOString())));
    if (mkey(viewMonth) === k) renderMiniCal();
  } catch (_) { /* keep dots from the loaded range */ }
}

function monthEvents() {
  const cached = monthCache.get(mkey(viewMonth));
  return cached || evs();
}

function renderMiniCal() {
  const now = new Date();
  $("monthName").textContent = viewMonth.toLocaleDateString("en-GB", { month: "long" });
  $("yearName").textContent = viewMonth.getFullYear();
  $("todayBtn").style.visibility = sameDay(viewMonth, startOfMonth(now)) && !selectedDay ? "hidden" : "visible";

  const dow = $("dow");
  if (!dow.childElementCount) {
    ["S", "M", "T", "W", "T", "F", "S"].forEach((d, i) => dow.append(el("span", i === 5 || i === 6 ? "wknd" : "", d)));
  }

  const dots = new Map(), counts = new Map(), hols = new Map();
  for (const e of monthEvents()) {
    let d = startOfDay(e.s);
    const last = e.allDay ? addDays(startOfDay(e.e), -1) : startOfDay(e.e);
    for (let i = 0; d <= last && i < 31; i++, d = addDays(d, 1)) {
      const k = dayKey(d);
      if (e.holiday && !S.showHolidayRows) { hols.set(k, [...(hols.get(k) || []), e.title]); continue; }
      counts.set(k, (counts.get(k) || 0) + 1);
      if (!dots.has(k)) dots.set(k, []);
      const arr = dots.get(k);
      if (arr.length < 3 && !arr.includes(e.color)) arr.push(e.color);
    }
  }

  const grid = $("grid");
  grid.innerHTML = "";
  const first = viewMonth;
  const start = addDays(first, -first.getDay());
  const weeks = Math.ceil((first.getDay() + new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()) / 7);
  for (let i = 0; i < weeks * 7; i++) {
    const d = addDays(start, i);
    const k = dayKey(d);
    const cell = el("div", "day");
    if (d.getMonth() !== first.getMonth()) cell.classList.add("out");
    if (d.getDay() === 5 || d.getDay() === 6) cell.classList.add("wknd");
    if (sameDay(d, now)) cell.classList.add("today");
    if (selectedDay && sameDay(d, selectedDay)) cell.classList.add("sel");
    cell.append(el("span", "num", d.getDate()));
    const n = counts.get(k) || 0;
    const tips = [];
    if (n) {
      cell.classList.add("has");
      if (n >= 3) cell.classList.add("busy");
      tips.push(`${n} event${n > 1 ? "s" : ""}`);
    }
    if (hols.has(k)) { cell.classList.add("holiday"); tips.push(...hols.get(k)); }
    if (tips.length) cell.title = tips.join("\n");
    const ds = dots.get(k);
    if (ds) {
      const w = el("span", "dots");
      ds.forEach((c) => { const x = el("i"); x.style.background = c || "var(--accent)"; w.append(x); });
      cell.append(w);
    }
    cell.onclick = () => openDay(d);
    cell.ondblclick = () => {
      const q = $("quick");
      q.value = `${q.value ? q.value + " " : ""}on ${d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" })} `;
      q.focus();
    };
    grid.append(cell);
  }
}

// ---------- list modes: upcoming | day | search ----------
let listMode = "upcoming";
let dayView = null;      // { d, events }
let searchResults = [];
let searchQ = "";

async function openDay(d) {
  if (sameDay(d, new Date()) && listMode === "day" && dayView && sameDay(dayView.d, d)) { closeDay(); return; }
  selectedDay = d;
  if (d.getMonth() !== viewMonth.getMonth() || d.getFullYear() !== viewMonth.getFullYear()) viewMonth = startOfMonth(d);
  renderMiniCal();
  listMode = "day";
  dayView = { d, events: null };
  renderList();
  try {
    const list = withDates(await go("EventsBetween", startOfDay(d).toISOString(), addDays(startOfDay(d), 1).toISOString()));
    if (dayView && sameDay(dayView.d, d)) { dayView.events = list; renderList(); }
  } catch (err) { toast(errMsg(err), true); }
}

function closeDay() {
  listMode = "upcoming";
  dayView = null;
  selectedDay = null;
  renderMiniCal();
  renderList();
  $("scroll").scrollTo({ top: 0 });
}

// ---------- up next ----------
function renderUpNext() {
  const now = new Date();
  const box = $("upnext");
  box.innerHTML = "";
  if (listMode !== "upcoming") return;
  const next = evs().find((e) => !e.allDay && !e.holiday && e.e > now);
  if (!next) return;
  const inProgress = next.s <= now;
  let lbl = inProgress ? `Now · ${dur(next.e - now)} left` : `Up next · in ${dur(next.s - now)}`;
  if (!inProgress && next.s - now > 24 * 36e5) lbl = `Up next · ${dayLabel(next.s)}`;
  const card = el("div", "upnext");
  const grow = el("div", "grow");
  grow.append(el("div", "lbl", lbl), el("div", "ttl", next.title));
  let sub = `${fmtTime(next.s)} – ${fmtTime(next.e)}`;
  if (next.location) sub += `  ·  ${next.location}`;
  grow.append(el("div", "sub", sub));
  card.append(grow);
  if (next.meetLink) {
    const j = el("button", "join", "Join");
    j.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", next.meetLink); };
    card.append(j);
  }
  card.onclick = () => openEvent(next);
  box.append(card);
}

// ---------- list ----------
function renderList() {
  const list = $("list");
  list.innerHTML = "";
  kbIndex = -1;
  renderUpNext();
  const now = new Date();

  if (!S.connected) { list.append(emptyState("📅", "Allow calendar access to see what's coming up.")); return; }

  if (listMode === "search") {
    const head = el("div", "mode-head");
    head.append(el("span", "", searchQ.length < 2 ? "Search" : `${searchResults.length} result${searchResults.length === 1 ? "" : "s"} for “${searchQ}”`));
    list.append(head);
    if (searchQ.length >= 2 && !searchResults.length) list.append(emptyState("🔍", "Nothing found in the past month or the next year."));
    renderGroups(list, searchResults, { now, showPast: true, spread: false });
    return;
  }

  if (listMode === "day" && dayView) {
    const d = dayView.d;
    const head = el("div", "mode-head");
    const ttl = el("span", "", d.toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long" }));
    const back = el("button", "pill-btn", "Upcoming ›");
    back.onclick = closeDay;
    head.append(ttl, back);
    list.append(head);
    if (!dayView.events) { list.append(emptyState("⏳", "Loading…")); return; }
    const evsDay = dayView.events;
    if (!evsDay.filter((e) => S.showHolidayRows || !e.holiday).length) {
      const hol = evsDay.filter((e) => e.holiday);
      if (hol.length) list.append(holidayTags(hol));
      const em = emptyState("☀️", "Nothing planned.");
      const add = el("button", "btn", "Add something this day");
      add.onclick = () => { const q = $("quick"); q.value = `on ${d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" })} `; q.focus(); };
      em.append(add);
      list.append(em);
      return;
    }
    renderGroups(list, evsDay, { now, showPast: true, spread: false, dayOnly: d });
    return;
  }

  const all = evs();
  if (!all.length) {
    list.append(emptyState("🌴", S.error ? "Couldn't load events." : `Nothing in the next ${S.daysAhead} days.`));
    list.append(syncHint());
    return;
  }
  renderGroups(list, all, { now, showPast: false, spread: true });
  if (all.filter((e) => !e.holiday).length < 3) list.append(syncHint());
}

// Groups events by day; holidays become small tags on the day header (unless shown as rows).
function renderGroups(list, events, { now, showPast, spread, dayOnly }) {
  const groups = new Map();
  const push = (d, e) => { const k = dayKey(d); if (!groups.has(k)) groups.set(k, { d, items: [], hols: [] }); (e.holiday && !S.showHolidayRows ? groups.get(k).hols : groups.get(k).items).push(e); };
  for (const e of events) {
    if (dayOnly) { push(dayOnly, e); continue; }
    if (e.allDay && spread) {
      let d = startOfDay(e.s) < startOfDay(now) ? startOfDay(now) : startOfDay(e.s);
      for (let i = 0; d < startOfDay(e.e) && i < 14; i++, d = addDays(d, 1)) push(d, e);
    } else {
      push(startOfDay(!showPast && e.s < now ? now : e.s), e);
    }
  }
  const keys = [...groups.values()].sort((a, b) => a.d - b.d);
  for (const g of keys) {
    if (!g.items.length && !g.hols.length) continue;
    g.items.sort((a, b) => (b.allDay - a.allDay) || (a.s - b.s));
    const head = el("div", "day-head" + (sameDay(g.d, now) ? " is-today" : ""));
    head.dataset.day = dayKey(g.d);
    const timed = g.items.filter((e) => !e.allDay).length;
    head.append(el("span", "", dayHead(g.d)), el("span", "count", timed ? `${timed} event${timed > 1 ? "s" : ""}` : ""));
    list.append(head);
    if (g.hols.length) list.append(holidayTags(g.hols));
    for (const e of g.items) list.append(eventRow(e, now));
  }
}

function holidayTags(hols) {
  const w = el("div", "hol-tags");
  [...new Set(hols.map((h) => h.title))].forEach((t) => w.append(el("span", "hol-tag", "🎉 " + t)));
  return w;
}

// macOS doesn't tell apps when an account's login has expired, so give a way to check.
function syncHint() {
  const d = el("div", "sync-hint muted small");
  d.innerHTML = `Events missing? macOS may need you to sign in to Google again — <a href="#">open Calendar</a> and look for “Sign in”.`;
  d.querySelector("a").onclick = (ev) => { ev.preventDefault(); go("OpenURL", "ical://"); };
  return d;
}

function emptyState(icon, text) {
  const d = el("div", "empty");
  d.append(el("div", "big", icon), el("div", "", text));
  return d;
}

function eventRow(e, now) {
  const row = el("div", "ev");
  row.tabIndex = -1;
  if (e.allDay) row.classList.add("allday");
  if (!e.allDay && e.s <= now && e.e > now) row.classList.add("now");
  if (e.e < now) row.classList.add("past");
  const bar = el("div", "bar");
  bar.style.background = e.color || "var(--accent)";
  const when = el("div", "when");
  if (e.allDay) when.textContent = "all-day";
  else when.append(el("div", "", fmtTime(e.s)), el("div", "end", fmtTime(e.e)));
  const body = el("div", "body");
  body.append(el("div", "title", e.title));
  const meta = [e.location, e.meetLink && !e.location ? "Online meeting" : "", S.calendars.length > 1 ? e.calendar : ""].filter(Boolean).join(" · ");
  if (meta) body.append(el("div", "meta", meta));
  const tools = el("div", "tools");
  if (e.meetLink) {
    const j = el("button", "", "▶︎");
    j.title = "Join";
    j.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", e.meetLink); };
    tools.append(j);
  }
  if (e.writable !== false) {
    const del = el("button", "del", "🗑");
    del.title = "Delete";
    del.onclick = (ev) => { ev.stopPropagation(); deleteEvent(e); };
    tools.append(del);
  }
  row.append(bar, when, body, tools);
  row.onclick = () => openEvent(e);
  row._ev = e;
  return row;
}

async function deleteEvent(e) {
  const ok = await confirmBox(`<b>Delete “${esc(e.title)}”?</b><span class="muted">${esc(whenText(e.start, e.end, e.allDay))}<br>This removes it from your calendar${e.calendar ? " (" + esc(e.calendar) + ")" : ""}.</span>`);
  if (!ok) return false;
  try {
    applyState(await go("Delete", e.id, e.start));
    afterChange();
    toast("Deleted");
    return true;
  } catch (err) { toast(errMsg(err), true); return false; }
}

// keep day view / search / month dots fresh after any change
function afterChange() {
  monthCache.clear();
  loadMonth(viewMonth);
  if (listMode === "day" && dayView) openDay(dayView.d);
  if (listMode === "search") runSearch(searchQ);
}

// ---------- event card (details / edit / duplicate) ----------
let sheetEvent = null;

function closeSheet() {
  $("sheet").classList.add("hidden");
  sheetEvent = null;
}

async function openEvent(e) {
  sheetEvent = e;
  const sh = $("sheet");
  sh.classList.remove("hidden");
  sh.innerHTML = "";
  sh.append(sheetHeader(e.title, e.color));
  sh.append(emptyState("⏳", "Loading…"));
  try {
    const d = await go("GetEvent", e.id, e.start);
    if (sheetEvent !== e) return;
    renderDetail(d);
  } catch (err) {
    // fall back to what the list already knows
    renderDetail({ ...e, notes: "", attendees: [], organizer: "", recurring: false, url: "" });
  }
}

function sheetHeader(title, color, backLabel = "‹ Back", onBack = closeSheet) {
  const h = el("div", "sheet-head");
  const back = el("button", "pill-btn", backLabel);
  back.onclick = onBack;
  const sw = el("span", "sheet-dot");
  sw.style.background = color || "var(--accent)";
  h.append(back, sw);
  return h;
}

const linkify = (t) => esc(t).replace(/(https?:\/\/[^\s<]+)/g, '<a href="#" data-open="$1">$1</a>').replace(/\n/g, "<br>");

function renderDetail(d) {
  const sh = $("sheet");
  sh.innerHTML = "";
  const s = new Date(d.start), en = new Date(d.end);
  sh.append(sheetHeader(d.title, d.color));
  const body = el("div", "sheet-body");
  const t = el("div", "sheet-title", d.title);
  body.append(t);
  const when = el("div", "sheet-when");
  when.textContent = whenText(d.start, d.end, d.allDay) + (d.recurring ? "  ·  repeats" : "");
  body.append(when);
  if (!d.allDay) body.append(el("div", "muted small", dur(en - s)));

  const rows = el("div", "sheet-rows");
  const addRow = (icon, html) => { const r = el("div", "sheet-row"); r.innerHTML = `<span class="ico">${icon}</span><div>${html}</div>`; rows.append(r); return r; };
  addRow("📅", `<span class="cal-chip" style="--c:${esc(d.color)}">${esc(d.calendar)}</span>`);
  if (d.location) addRow("📍", `<a href="#" data-open="https://maps.apple.com/?q=${encodeURIComponent(d.location)}">${esc(d.location)}</a>`);
  if (d.meetLink) {
    const r = addRow("🎥", `<button class="btn primary small-btn" data-open="${esc(d.meetLink)}">Join meeting</button>`);
    r.classList.add("join-row");
  }
  if (d.organizer) addRow("👤", `Organised by ${esc(d.organizer)}`);
  if (d.attendees && d.attendees.length) {
    const icon = { accepted: "✅", declined: "❌", maybe: "❔", pending: "⏳", unknown: "·" };
    addRow("👥", `<div class="people">${d.attendees.map((p) => `<div>${icon[p.status] || "·"} ${esc(p.name || p.email)}${p.me ? " <span class='muted'>(you)</span>" : ""}</div>`).join("")}</div>`);
  }
  if (d.notes) addRow("📝", `<div class="notes">${linkify(d.notes)}</div>`);
  if (d.url && d.url !== d.meetLink) addRow("🔗", `<a href="#" data-open="${esc(d.url)}">${esc(d.url)}</a>`);
  body.append(rows);
  sh.append(body);

  const actions = el("div", "sheet-actions");
  const mk = (label, cls, fn) => { const b = el("button", "btn " + cls, label); b.onclick = fn; actions.append(b); return b; };
  if (d.writable) mk("Edit", "primary", () => renderEdit(d, "edit"));
  mk("Duplicate", "", () => renderEdit(d, "duplicate"));
  mk("Open in Calendar", "", () => go("OpenURL", d.link));
  if (d.writable) mk("Delete", "danger-ghost", async () => { if (await deleteEvent(d)) closeSheet(); });
  sh.append(actions);
  sh.querySelectorAll("[data-open]").forEach((a) => {
    a.onclick = (ev) => { ev.preventDefault(); go("OpenURL", a.dataset.open); };
  });
}

const pad = (n) => String(n).padStart(2, "0");
const dateVal = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const timeVal = (d) => `${pad(d.getHours())}:${pad(d.getMinutes())}`;
const fromInputs = (date, time) => { const [y, m, dd] = date.split("-").map(Number); const [h, mi] = (time || "00:00").split(":").map(Number); return new Date(y, m - 1, dd, h, mi); };

// mode: edit | duplicate | new
function renderEdit(d, mode) {
  const sh = $("sheet");
  sh.classList.remove("hidden");
  sh.innerHTML = "";
  const s = new Date(d.start), en = new Date(d.end);
  const lastDay = d.allDay ? addDays(en, -1) : en;
  sh.append(sheetHeader(mode === "edit" ? "Edit" : mode === "duplicate" ? "Duplicate" : "New", d.color, "‹ Cancel",
    () => (mode === "edit" ? renderDetail(d) : closeSheet())));
  const f = el("div", "sheet-body edit-form");
  const writable = S.calendars.filter((c) => c.writable);
  f.innerHTML = `
    <div class="sheet-kicker">${mode === "edit" ? "Edit event" : mode === "duplicate" ? "Duplicate event" : "New event"}</div>
    <input id="fTitle" class="f-title" placeholder="Title">
    <label class="check"><input type="checkbox" id="fAllDay"> All-day</label>
    <div class="f-grid">
      <span>Starts</span><div class="f-pair"><input type="date" id="fSDate"><input type="time" id="fSTime"></div>
      <span>Ends</span><div class="f-pair"><input type="date" id="fEDate"><input type="time" id="fETime"></div>
      <span>Location</span><input id="fLoc" placeholder="Add location">
      <span>Calendar</span><select id="fCal"></select>
    </div>
    <textarea id="fNotes" rows="3" placeholder="Notes"></textarea>`;
  sh.append(f);
  $("fTitle").value = mode === "duplicate" ? d.title : d.title || "";
  $("fAllDay").checked = !!d.allDay;
  $("fSDate").value = dateVal(s); $("fSTime").value = timeVal(s);
  $("fEDate").value = dateVal(lastDay); $("fETime").value = timeVal(en);
  $("fLoc").value = d.location || "";
  $("fNotes").value = d.notes || "";
  writable.forEach((c) => { const o = el("option", "", c.name); o.value = c.id; if (c.id === d.calendarId) o.selected = true; $("fCal").append(o); });
  const syncAllDay = () => { const a = $("fAllDay").checked; $("fSTime").classList.toggle("hidden", a); $("fETime").classList.toggle("hidden", a); };
  $("fAllDay").onchange = syncAllDay; syncAllDay();
  // moving the start keeps the duration
  let lastStart = fromInputs($("fSDate").value, $("fSTime").value);
  const shift = () => {
    const ns = fromInputs($("fSDate").value, $("fSTime").value);
    const ne = new Date(fromInputs($("fEDate").value, $("fETime").value).getTime() + (ns - lastStart));
    $("fEDate").value = dateVal(ne); $("fETime").value = timeVal(ne);
    lastStart = ns;
  };
  $("fSDate").onchange = shift; $("fSTime").onchange = shift;

  const actions = el("div", "sheet-actions");
  const save = el("button", "btn primary", mode === "edit" ? "Save changes" : "Add event");
  save.onclick = async () => {
    const allDay = $("fAllDay").checked;
    let start = fromInputs($("fSDate").value, allDay ? "00:00" : $("fSTime").value);
    let end = allDay ? addDays(fromInputs($("fEDate").value, "00:00"), 1) : fromInputs($("fEDate").value, $("fETime").value);
    if (end <= start) { toast("The end must be after the start", true); return; }
    const draft = { title: $("fTitle").value.trim(), start: start.toISOString(), end: end.toISOString(), allDay,
      location: $("fLoc").value.trim(), notes: $("fNotes").value, calendarId: $("fCal").value, reminderMinutes: 0 };
    if (!draft.title) { toast("Give it a title", true); return; }
    save.disabled = true;
    try {
      if (mode === "edit") applyState(await go("UpdateEvent", d.id, d.start, draft));
      else applyState(await go("Add", [draft]));
      afterChange();
      toast(mode === "edit" ? "Saved" : "Added");
      closeSheet();
    } catch (err) { toast(errMsg(err), true); save.disabled = false; }
  };
  const cancel = el("button", "btn", "Cancel");
  cancel.onclick = () => (mode === "edit" ? renderDetail(d) : closeSheet());
  actions.append(cancel, save);
  sh.append(actions);
  setTimeout(() => $("fTitle").focus(), 30);
}

// ---------- search ----------
let searchTimer;
function openSearch() {
  $("searchBar").classList.remove("hidden");
  $("searchInput").focus();
  $("searchInput").select();
}
function closeSearch() {
  $("searchBar").classList.add("hidden");
  $("searchInput").value = "";
  searchQ = ""; searchResults = [];
  if (listMode === "search") { listMode = "upcoming"; renderList(); }
}
async function runSearch(q) {
  searchQ = q.trim();
  listMode = searchQ ? "search" : "upcoming";
  if (searchQ.length < 2) { searchResults = []; renderList(); return; }
  try {
    const r = withDates(await go("Search", searchQ));
    if (q.trim() !== searchQ) return;
    searchResults = r;
  } catch (err) { toast(errMsg(err), true); }
  renderList();
}

// ---------- keyboard navigation ----------
let kbIndex = -1;
function kbMove(delta) {
  const rows = [...document.querySelectorAll("#list .ev")];
  if (!rows.length) return;
  kbIndex = Math.max(0, Math.min(rows.length - 1, kbIndex + delta));
  rows.forEach((r, i) => r.classList.toggle("kb", i === kbIndex));
  rows[kbIndex].scrollIntoView({ block: "nearest" });
}
function kbOpen() {
  const rows = [...document.querySelectorAll("#list .ev")];
  if (kbIndex >= 0 && rows[kbIndex]) openEvent(rows[kbIndex]._ev);
}

// ---------- banner (setup hints) ----------
function renderBanner() {
  const b = $("setupBanner");
  if (!S.connected) {
    b.innerHTML = `<b>One click to start:</b> let Personal Calendar read your calendars. <button class="btn primary" id="bannerAllow">Allow</button>`;
    b.classList.remove("hidden");
    $("bannerAllow").onclick = allowAccess;
    return;
  }
  const parts = [];
  if (!S.hasGoogle && !S.calendars.length) parts.push(`No calendars on this Mac yet <button class="btn" id="bannerGoogle">Add Google…</button>`);
  if (!S.hasKey) parts.push(`Add your OpenRouter key for smarter AI (simple lines work without it) — <a href="#" id="bannerGo">Settings</a>`);
  if (S.error) parts.push(esc(S.error));
  if (!parts.length) { b.classList.add("hidden"); return; }
  b.innerHTML = parts.join("<br>");
  b.classList.remove("hidden");
  if ($("bannerGo")) $("bannerGo").onclick = (ev) => { ev.preventDefault(); openSettings().then(focusAI); };
  if ($("bannerGoogle")) $("bannerGoogle").onclick = () => go("AddGoogleAccount");
}

function renderFooter() {
  $("updated").textContent = S.now ? `Updated ${fmtTime(new Date(S.now))}` : "";
  $("hotkeyHint").textContent = S.hotkey ? S.hotkey.replace("Ctrl", "⌃").replace("Alt", "⌥").replace(/\+/g, "") + " to open" : "";
}

function applyState(st) {
  if (!st) return;
  S = st;
  if (!pendingAccent) applyAccent(S.accent);
  renderMiniCal();
  renderList();
  renderBanner();
  renderFooter();
  if (drafts.length) renderDrafts();
  if (!$("settings").classList.contains("hidden")) renderGoogleBox();
  if (mkey(viewMonth) !== mkey(new Date())) loadMonth(viewMonth, true);
}

// ---------- AI box ----------
function setBusy(b) {
  busy = b;
  $("spark").classList.toggle("busy", b);
  $("quick").disabled = b;
}

let aiResult = null;

async function submitQuick() {
  const text = $("quick").value.trim();
  if (busy) return;
  if (!text) { if (drafts.length) addDrafts(); else if (aiResult) applyAI(); return; }
  setBusy(true);
  try {
    const r = await go("Ask", text);
    $("quick").value = "";
    if (r.kind === "add") {
      aiResult = null;
      drafts = r.drafts;
      if (r.offline && r.answer) toast(r.answer);
      if (!S.confirmBeforeAdd && !r.offline) { await addDrafts(); return; }
      renderDrafts();
    } else {
      drafts = [];
      aiResult = r;
      renderAI();
    }
  } catch (err) {
    toast(errMsg(err), true);
  } finally {
    setBusy(false);
    $("quick").focus();
  }
}

function renderAI() {
  const box = $("drafts");
  box.innerHTML = "";
  const r = aiResult;
  if (!r) return;
  const card = el("div", "ai-card " + r.kind);
  const head = el("div", "ai-head");
  head.append(el("span", "ai-badge", r.kind === "answer" ? "✦ Answer" : r.kind === "update" ? "✦ Change" : "✦ Delete"));
  const x = el("button", "icon-btn small", "✕");
  x.title = "Dismiss";
  x.onclick = clearDrafts;
  head.append(x);
  card.append(head);
  if (r.answer) card.append(el("div", "ai-answer", r.answer));
  if (r.kind === "answer" && r.refs.length) {
    const refs = el("div", "ai-refs");
    withDates(r.refs).forEach((e) => refs.append(eventRow(e, new Date())));
    card.append(refs);
  }
  if (r.kind === "update" && r.target && r.draft) {
    const t = r.target, d = r.draft;
    const cmp = el("div", "ai-compare");
    cmp.innerHTML = `<div class="was"><span class="muted small">Now</span><b>${esc(t.title)}</b><div>${esc(whenText(t.start, t.end, t.allDay))}</div>${t.location ? `<div class="muted">📍 ${esc(t.location)}</div>` : ""}</div>
      <div class="arrow">→</div>
      <div class="will"><span class="muted small">After</span><b>${esc(d.title)}</b><div>${esc(whenText(d.start, d.end, d.allDay))}</div>${d.location ? `<div class="muted">📍 ${esc(d.location)}</div>` : ""}</div>`;
    card.append(cmp);
  }
  if (r.kind === "delete" && r.target) {
    card.append(eventRow(withDates([r.target])[0], new Date()));
  }
  if (r.kind !== "answer") {
    const actions = el("div", "draft-actions");
    const cancel = el("button", "btn", "Cancel");
    cancel.onclick = clearDrafts;
    const ok = el("button", "btn " + (r.kind === "delete" ? "danger" : "primary"), r.kind === "delete" ? "Delete ↩" : "Apply ↩");
    ok.onclick = applyAI;
    actions.append(cancel, ok);
    card.append(actions);
  }
  box.append(card);
}

async function applyAI() {
  const r = aiResult;
  if (!r || busy || r.kind === "answer") return;
  setBusy(true);
  try {
    if (r.kind === "update") applyState(await go("UpdateEvent", r.target.id, r.target.start, r.draft));
    if (r.kind === "delete") applyState(await go("Delete", r.target.id, r.target.start));
    toast(r.kind === "update" ? `Updated “${r.draft.title}”` : `Deleted “${r.target.title}”`);
    aiResult = null;
    renderAI();
    afterChange();
  } catch (err) { toast(errMsg(err), true); }
  finally { setBusy(false); $("quick").focus(); }
}

function renderDrafts() {
  const box = $("drafts");
  box.innerHTML = "";
  if (!drafts.length) return;
  const writable = S.calendars.filter((c) => c.writable);
  drafts.forEach((d, i) => {
    const card = el("div", "draft");
    const t = el("div", "draft-title", d.title);
    t.contentEditable = "plaintext-only";
    t.title = "Click to edit";
    t.oninput = () => { drafts[i].title = t.textContent.trim(); };
    t.onkeydown = (ev) => { if (ev.key === "Enter") { ev.preventDefault(); addDrafts(); } };
    card.append(t, el("div", "draft-when", whenText(d.start, d.end, d.allDay)));
    const meta = [];
    if (d.location) meta.push("📍 " + d.location);
    if (d.reminderMinutes > 0) meta.push(`🔔 ${dur(d.reminderMinutes * 60000)} before`);
    if (d.notes) meta.push("📝 " + d.notes);
    if (meta.length) card.append(el("div", "draft-meta", meta.join("   ")));
    if (writable.length > 1) {
      const row = el("div", "draft-cal");
      const sw = el("span");
      sw.style.cssText = "width:8px;height:8px;border-radius:50%;display:inline-block";
      const sel = el("select");
      writable.forEach((c) => {
        const o = el("option", "", c.name);
        o.value = c.id;
        if (c.id === d.calendarId) o.selected = true;
        sel.append(o);
      });
      const paint = () => { sw.style.background = (writable.find((c) => c.id === sel.value) || {}).color || "var(--accent)"; };
      sel.onchange = () => { drafts[i].calendarId = sel.value; paint(); };
      paint();
      row.append(sw, sel);
      card.append(row);
    }
    box.append(card);
  });
  const actions = el("div", "draft-actions");
  const cancel = el("button", "btn", "Cancel");
  cancel.onclick = clearDrafts;
  const add = el("button", "btn primary", drafts.length > 1 ? `Add ${drafts.length} events ↩` : "Add ↩");
  add.id = "addBtn";
  add.onclick = addDrafts;
  actions.append(cancel, add);
  box.append(actions);
}

function clearDrafts() { drafts = []; aiResult = null; renderDrafts(); $("drafts").innerHTML = ""; $("quick").focus(); }

async function addDrafts() {
  if (busy || !drafts.length) return;
  setBusy(true);
  const n = drafts.length, first = drafts[0];
  try {
    const st = await go("Add", drafts);
    drafts = [];
    renderDrafts();
    applyState(st);
    afterChange();
    toast(n === 1 ? `Added “${first.title}”` : `Added ${n} events`);
    flashDay(new Date(first.start));
  } catch (err) {
    toast(errMsg(err), true);
  } finally {
    setBusy(false);
    $("quick").focus();
  }
}

function flashDay(d) {
  const head = document.querySelector(`[data-day="${dayKey(startOfDay(d))}"]`);
  if (!head) return;
  $("scroll").scrollTo({ top: head.offsetTop - $("scroll").offsetTop, behavior: "smooth" });
  let n = head.nextElementSibling;
  while (n && n.classList.contains("ev")) { n.classList.add("flash"); n = n.nextElementSibling; }
}

// ---------- setup ----------
async function allowAccess() {
  try {
    applyState(await go("AllowAccess"));
    if (S.connected) toast(S.hasGoogle ? "Connected — your calendars are in" : "Allowed — now add your Google account");
  } catch (err) { toast(errMsg(err), true); }
  if (!$("settings").classList.contains("hidden")) renderGoogleBox();
}

// ---------- highlight colour ----------
const ACCENTS = [
  ["Royal blue", "#4169E1"], ["Red", "#E5484D"], ["Orange", "#F0802E"], ["Green", "#2FA36B"],
  ["Teal", "#12A4A4"], ["Purple", "#8B5CF6"], ["Pink", "#E0559A"], ["Graphite", "#5B6270"],
];
let pendingAccent = null;

function hexToRgb(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || "");
  if (!m) return null;
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}
const mix = (rgb, to, t) => rgb.map((c, i) => Math.round(c + (to[i] - c) * t));
const toHex = (rgb) => "#" + rgb.map((c) => c.toString(16).padStart(2, "0")).join("");

function applyAccent(hex) {
  const rgb = hexToRgb(hex) || hexToRgb("#4169E1");
  const r = document.documentElement.style;
  r.setProperty("--accent", toHex(rgb));
  r.setProperty("--accent-rgb", rgb.join(", "));
  r.setProperty("--accent-2", toHex(mix(rgb, [255, 255, 255], 0.28)));
  r.setProperty("--accent-ink", toHex(mix(rgb, [0, 0, 0], 0.25)));
}

function renderSwatches() {
  const box = $("swatches");
  box.innerHTML = "";
  const cur = (pendingAccent || S.accent || "#4169E1").toUpperCase();
  let named = false;
  for (const [name, hex] of ACCENTS) {
    const b = el("button", "swatch");
    b.style.background = hex;
    b.style.color = hex;
    b.title = name;
    if (hex === cur) { b.classList.add("on"); named = true; $("accentName").textContent = name; }
    b.onclick = () => { pendingAccent = hex; applyAccent(hex); renderSwatches(); };
    box.append(b);
  }
  const pick = el("input", "swatch-custom");
  pick.type = "color";
  pick.value = cur.toLowerCase();
  pick.title = "Custom colour";
  pick.oninput = () => { pendingAccent = pick.value.toUpperCase(); applyAccent(pendingAccent); $("accentName").textContent = "Custom " + pendingAccent; };
  box.append(pick);
  if (!named) $("accentName").textContent = "Custom " + cur;
}

// ---------- updates ----------
let U = null;
function renderUpdate(info, statusMsg) {
  if (info) U = info;
  const cur = (U && U.current) || "";
  $("versionLbl").textContent = cur ? `v${cur}` : "";
  $("updVersion").textContent = cur ? `Personal Calendar v${cur}` : "Personal Calendar";
  const avail = U && U.available;
  $("updatePill").classList.toggle("hidden", !avail);
  if (avail) $("updatePill").textContent = `Update to v${U.latest}`;
  $("installUpdBtn").classList.toggle("hidden", !avail);
  if (avail) $("installUpdBtn").textContent = `Update to v${U.latest} now`;
  $("updNotes").classList.toggle("hidden", !(avail && U.notes));
  if (avail) {
    const ul = $("updNotes");
    ul.innerHTML = "";
    (U.notes || "").split("\n").map((l) => l.replace(/^\s*[-*]\s*/, "").trim()).filter(Boolean)
      .forEach((l) => ul.append(el("li", "", l)));
  }
  let st = statusMsg;
  if (!st && U && U.checked && !U.checked.startsWith("0001")) {
    st = avail ? `v${U.latest} is available.` : `You're up to date · checked ${fmtTime(new Date(U.checked))}`;
  }
  if (st) $("updStatus").textContent = st;
}

async function checkUpdate() {
  $("checkUpdBtn").disabled = true;
  renderUpdate(null, "Checking GitHub…");
  try { renderUpdate(await go("CheckForUpdate")); }
  catch (err) { renderUpdate(null, errMsg(err)); }
  finally { $("checkUpdBtn").disabled = false; }
}

async function installUpdate() {
  $("installUpdBtn").disabled = true;
  $("updatePill").disabled = true;
  renderUpdate(null, "Starting update…");
  toast("Updating — the app will restart by itself");
  try { await go("InstallUpdate"); }
  catch (err) {
    toast(errMsg(err), true);
    renderUpdate(null, errMsg(err));
    $("installUpdBtn").disabled = false;
    $("updatePill").disabled = false;
  }
}

// ---------- settings ----------
async function openSettings() {
  $("main").classList.add("hidden");
  $("settings").classList.remove("hidden");
  settingsCache = await go("GetSettings");
  $("sKey").value = "";
  $("sModel").value = settingsCache.openaiModel;
  $("s24").checked = settingsCache.use24h;
  $("sConfirm").checked = settingsCache.confirmBeforeAdd;
  $("sDays").value = String(settingsCache.daysAhead);
  $("sMenuBar").value = settingsCache.menuBarStyle || "next";
  $("sAlert").value = settingsCache.alertMode || "1";
  $("sLogin").checked = !!settingsCache.launchAtLogin;
  $("sHolRows").checked = !!settingsCache.showHolidayRows;
  pendingAccent = null;
  showSetupDetails = false;
  editingKey = false;
  renderSwatches();
  renderGoogleBox();
  document.querySelector(".settings-body").scrollTop = 0;
}

let showSetupDetails = false;
let editingKey = false;
function arrangeSettings() {
  const body = document.querySelector(".settings-body");
  const setupDone = S.connected && S.hasGoogle;
  const compact = setupDone && !showSetupDetails;
  $("connectedLine").classList.toggle("hidden", !setupDone);
  $("connectGroup").classList.toggle("hidden", compact);
  $("connectedText").textContent = setupDone
    ? `Connected to ${S.googleAccounts.join(", ")} · ${S.calendars.length} calendars` : "";
  // AI quick add goes right under the status line once the calendar is set up
  const anchor = setupDone ? $("connectedLine") : $("connectGroup");
  if (anchor.nextElementSibling !== $("aiGroup")) anchor.after($("aiGroup"));
  // emphasise it while there's no key
  $("aiGroup").classList.toggle("attention", !S.hasKey);
  const ks = $("keyState");
  ks.textContent = S.hasKey ? "✓ key saved" : "key needed";
  ks.className = "key-state " + (S.hasKey ? "ok" : "missing");
  $("aiIntro").classList.toggle("hidden", S.hasKey);
  const hint = (settingsCache && settingsCache.openaiKeyHint) || "";
  const showSaved = S.hasKey && !editingKey;
  $("keySaved").classList.toggle("hidden", !showSaved);
  $("keyField").classList.toggle("hidden", showSaved);
  $("keyProvider").textContent = (hint.startsWith("sk-or") ? "OpenRouter" : "OpenAI") + " key saved";
  $("keyMasked").innerHTML = `<code>${esc(hint || "key")}</code> · stored on this Mac`;
  $("sKey").placeholder = S.hasKey ? "Paste a new key to replace the saved one" : "Paste your OpenRouter key  sk-or-…";
}

function focusAI() {
  const g = $("aiGroup");
  document.querySelector(".settings-body").scrollTo({ top: g.offsetTop - 70, behavior: "smooth" });
  g.classList.remove("flash"); void g.offsetWidth; g.classList.add("flash");
  setTimeout(() => $("sKey").focus(), 250);
}

function renderGoogleBox() {
  arrangeSettings();
  $("step1").classList.toggle("done", S.connected);
  $("step1Hint").innerHTML = S.connected ? "Allowed ✓"
    : S.access === "notDetermined" ? "macOS will ask once — click <b>Allow</b>."
    : "It was turned off — click Allow to open the switch in System Settings.";
  $("step2").classList.toggle("done", S.hasGoogle);
  $("step2").classList.toggle("waiting", !S.connected);
  const accounts = [...new Set(S.calendars.map((c) => c.account).filter(Boolean))];
  $("step2Hint").innerHTML = !S.connected
    ? "Do step 1 first — the app can't see any calendars until access is allowed."
    : S.hasGoogle ? esc(S.googleAccounts.join(", ")) + " ✓"
    : accounts.length ? `Found ${esc(accounts.join(", "))} but no Google calendars. Already added Google in System Settings? Click <a href="#" id="recheck">Check again</a>.`
    : "Sign in with Google once and keep <b>Calendars</b> ticked. It appears here by itself.";
  if ($("recheck")) $("recheck").onclick = (ev) => { ev.preventDefault(); refresh().then(() => toast(S.hasGoogle ? "Google calendars found" : "Still no Google calendars — open the Calendar app once so macOS syncs")); };

  const list = $("calList");
  list.innerHTML = "";
  $("calGroup").classList.toggle("hidden", !S.connected || !S.calendars.length);
  const hidden = new Set((settingsCache && settingsCache.hiddenCalendars) || []);
  const def = (settingsCache && settingsCache.defaultCalendar) || (S.calendars.find((c) => c.primary) || {}).id;
  for (const c of S.calendars) {
    const row = el("label", "cal-row");
    const cb = el("input");
    cb.type = "checkbox"; cb.checked = !hidden.has(c.id); cb.dataset.cal = c.id;
    const sw = el("span", "sw"); sw.style.background = c.color;
    const nm = el("span", "nm", c.name);
    if (c.account && c.account !== c.name) nm.append(el("span", "muted small", "  " + c.account));
    row.append(cb, sw, nm);
    if (c.writable) {
      const rb = el("input");
      rb.type = "radio"; rb.name = "defcal"; rb.value = c.id; rb.checked = c.id === def; rb.title = "New events go here";
      row.append(rb);
    }
    list.append(row);
  }
}

function closeSettings() {
  if (pendingAccent) { pendingAccent = null; applyAccent(S.accent); }
  $("settings").classList.add("hidden");
  $("main").classList.remove("hidden");
  $("quick").focus();
}

async function saveSettings() {
  const hidden = [...document.querySelectorAll("#calList input[type=checkbox]")].filter((c) => !c.checked).map((c) => c.dataset.cal);
  const def = (document.querySelector("#calList input[type=radio]:checked") || {}).value || "";
  const payload = {
    openaiKey: $("sKey").value.trim(),
    openaiModel: $("sModel").value.trim(),
    use24h: $("s24").checked,
    confirmBeforeAdd: $("sConfirm").checked,
    daysAhead: parseInt($("sDays").value, 10),
    hiddenCalendars: hidden,
    defaultCalendar: def,
    accent: pendingAccent || S.accent || "#4169E1",
    menuBarStyle: $("sMenuBar").value,
    alertMode: $("sAlert").value,
    launchAtLogin: $("sLogin").checked,
    showHolidayRows: $("sHolRows").checked,
  };
  try {
    pendingAccent = null;
    applyState(await go("SaveSettings", payload));
    settingsCache = await go("GetSettings");
    const keyJustSaved = !!payload.openaiKey;
    $("sKey").value = "";
    if (keyJustSaved) editingKey = false;
    arrangeSettings();
    if (keyJustSaved) { const k = $("keySaved"); k.classList.remove("just"); void k.offsetWidth; k.classList.add("just"); }
    toast(keyJustSaved ? "Key saved on this Mac ✓ — quick add is ready" : "Saved ✓");
  } catch (err) { toast(errMsg(err), true); }
}

// ---------- wiring ----------
function wire() {
  $("quickForm").onsubmit = (ev) => { ev.preventDefault(); submitQuick(); };
  // 2-line box: Return adds, Shift+Return makes a new line
  $("quick").addEventListener("keydown", (ev) => {
    if (ev.key === "Enter" && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); submitQuick(); }
  });
  const goMonth = (n) => { viewMonth = new Date(viewMonth.getFullYear(), viewMonth.getMonth() + n, 1); renderMiniCal(); loadMonth(viewMonth); };
  $("prevMonth").onclick = () => goMonth(-1);
  $("nextMonth").onclick = () => goMonth(1);
  $("todayBtn").onclick = () => { viewMonth = startOfMonth(new Date()); closeDay(); };
  $("searchBtn").onclick = () => ($("searchBar").classList.contains("hidden") ? openSearch() : closeSearch());
  $("searchInput").addEventListener("input", () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => runSearch($("searchInput").value), 220); });
  $("searchInput").addEventListener("keydown", (ev) => {
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); closeSearch(); $("quick").focus(); }
    if (ev.key === "ArrowDown") { ev.preventDefault(); $("searchInput").blur(); kbMove(1); }
    if (ev.key === "Enter") { ev.preventDefault(); $("searchInput").blur(); kbIndex = -1; kbMove(1); kbOpen(); }
  });
  $("searchClose").onclick = closeSearch;
  $("openSettings").onclick = openSettings;
  $("closeSettings").onclick = closeSettings;
  $("returnBtn").onclick = closeSettings;
  $("changeKey").onclick = () => { editingKey = true; arrangeSettings(); $("sKey").focus(); };
  $("showSetup").onclick = (ev) => { ev.preventDefault(); showSetupDetails = !showSetupDetails; arrangeSettings(); $("showSetup").textContent = showSetupDetails ? "hide" : "details"; };
  $("sKey").addEventListener("keydown", (ev) => { if (ev.key === "Enter") { ev.preventDefault(); saveSettings(); } });
  // paste the key → it's saved immediately; no need to press Save
  $("sKey").addEventListener("input", () => {
    const v = $("sKey").value.trim();
    if (/^sk-[A-Za-z0-9_-]{20,}$/.test(v)) saveSettings();
  });
  $("saveSettings").onclick = saveSettings;
  $("refreshBtn").onclick = refresh;
  $("quitBtn").onclick = () => go("Quit");
  $("allowBtn").onclick = allowAccess;
  $("addGoogleBtn").onclick = () => go("AddGoogleAccount");
  document.querySelectorAll("[data-url]").forEach((a) => {
    a.onclick = (ev) => { ev.preventDefault(); go("OpenURL", a.dataset.url); };
  });

  document.addEventListener("keydown", (ev) => {
    const inSettings = !$("settings").classList.contains("hidden");
    const sheetOpen = !$("sheet").classList.contains("hidden");
    const typing = ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement && document.activeElement.tagName);
    if (ev.key === "Escape") {
      ev.preventDefault();
      if (!$("confirm").classList.contains("hidden")) { $("confirmNo").click(); return; }
      if (sheetOpen) { closeSheet(); return; }
      if (inSettings) { closeSettings(); return; }
      if (drafts.length || aiResult) { clearDrafts(); return; }
      if ($("quick").value) { $("quick").value = ""; return; }
      if (!$("searchBar").classList.contains("hidden")) { closeSearch(); return; }
      if (listMode === "day") { closeDay(); return; }
      go("Hide");
    }
    if (!inSettings && !sheetOpen) {
      // arrows move through the list (from the AI box too, when it's empty)
      const quickEmpty = document.activeElement === $("quick") && !$("quick").value;
      if ((ev.key === "ArrowDown" || ev.key === "ArrowUp") && (!typing || quickEmpty)) {
        ev.preventDefault();
        if (quickEmpty) $("quick").blur();
        kbMove(ev.key === "ArrowDown" ? 1 : -1);
        if (kbIndex === 0 && ev.key === "ArrowUp" && quickEmpty) $("quick").focus();
      }
      if (ev.key === "Enter" && !typing && kbIndex >= 0) { ev.preventDefault(); kbOpen(); }
      if ((ev.key === "Backspace" || ev.key === "Delete") && !typing && kbIndex >= 0) {
        const rows = [...document.querySelectorAll("#list .ev")];
        if (rows[kbIndex] && rows[kbIndex]._ev.writable !== false) { ev.preventDefault(); deleteEvent(rows[kbIndex]._ev); }
      }
      if (!typing && ev.key === "/") { ev.preventDefault(); openSearch(); }
      if (!typing && ev.key === "ArrowLeft" && !ev.metaKey) { ev.preventDefault(); openDay(addDays(selectedDay || new Date(), -1)); }
      if (!typing && ev.key === "ArrowRight" && !ev.metaKey) { ev.preventDefault(); openDay(addDays(selectedDay || new Date(), 1)); }
    }
    if (ev.metaKey && ev.key.toLowerCase() === "f") { ev.preventDefault(); openSearch(); }
    if (ev.metaKey && ev.key === ",") { ev.preventDefault(); openSettings(); }
    if (ev.metaKey && ev.key.toLowerCase() === "r") { ev.preventDefault(); refresh(); }
    if (ev.metaKey && ev.key.toLowerCase() === "n") { ev.preventDefault(); closeSettings(); }
    if (ev.metaKey && ev.key.toLowerCase() === "w") { ev.preventDefault(); go("Hide"); }
    if (ev.metaKey && ev.key.toLowerCase() === "t") { ev.preventDefault(); $("todayBtn").click(); }
  });

  Events.On("state", (ev) => applyState(ev.data));
  Events.On("update", (ev) => renderUpdate(ev.data));
  Events.On("update:progress", (ev) => { renderUpdate(null, ev.data); toast(ev.data); });
  Events.On("ui:updates", () => { openSettings().then(() => { $("updatesGroup").scrollIntoView({ behavior: "smooth" }); checkUpdate(); }); });
  $("checkUpdBtn").onclick = checkUpdate;
  $("installUpdBtn").onclick = installUpdate;
  $("updatePill").onclick = () => { openSettings().then(() => $("updatesGroup").scrollIntoView({ behavior: "smooth" })); };
  Events.On("ui:settings", () => openSettings());
  Events.On("alert", (ev) => {
    const a = ev.data;
    toast(`⏰ ${a.title} — ${a.body}`);
  });
  Events.On("popover:shown", () => {
    viewMonth = startOfMonth(new Date());
    selectedDay = null;
    if (listMode === "day") { listMode = "upcoming"; dayView = null; }
    renderMiniCal();
    renderList();
    if ($("settings").classList.contains("hidden")) setTimeout(() => $("quick").focus(), 30);
    go("Refresh").then(applyState).catch(() => {});
  });
  window.addEventListener("focus", () => { if ($("settings").classList.contains("hidden")) $("quick").focus(); });

  // keep countdowns fresh while open
  setInterval(() => { renderUpNext(); }, 15000);
}

async function refresh() {
  try { applyState(await go("Refresh")); } catch (err) { toast(errMsg(err), true); }
}

wire();
renderMiniCal();
go("GetState").then(applyState).catch((err) => toast(errMsg(err), true));
go("LatestUpdate").then((u) => renderUpdate(u)).catch(() => {});
