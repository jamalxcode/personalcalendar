import { Call, Events } from "/wails/runtime.js";
import { popMenu, pickDate } from "./menu.js";

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
  if (S.use24h) return d.toLocaleTimeString(LOC(), { hour: "2-digit", minute: "2-digit" });
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
  if (sameDay(d, now) || d < now) return tr("Today");
  if (sameDay(d, addDays(now, 1))) return tr("Tomorrow");
  return d.toLocaleDateString(LOC(), { weekday: "long" });
}
function dayHead(d) {
  const now = new Date();
  const lbl = dayLabel(d, now);
  const date = d.toLocaleDateString(LOC(), { day: "numeric", month: "short" });
  if (lbl === "Today" || lbl === "Tomorrow") return `${lbl} · ${d.toLocaleDateString(LOC(), { weekday: "short" })} ${date}`;
  if (d - now < 6 * 864e5) return `${lbl} · ${date}`;
  return d.toLocaleDateString(LOC(), { weekday: "long", day: "numeric", month: "short" });
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

// "Ctrl+Alt+CmdOrCtrl+C" → "⌃⌥⌘C" (Apple's modifier order: ⌃ ⌥ ⇧ ⌘)
function macKeys(accel) {
  const parts = String(accel).split("+");
  const key = parts.pop();
  const mods = new Set(parts.map((p) => p.toLowerCase()));
  let out = "";
  if (mods.has("ctrl") || mods.has("control")) out += "⌃";
  if (mods.has("alt") || mods.has("option")) out += "⌥";
  if (mods.has("shift")) out += "⇧";
  if (mods.has("cmd") || mods.has("cmdorctrl") || mods.has("command") || mods.has("super")) out += "⌘";
  return out + (key.length === 1 ? key.toUpperCase() : key);
}

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

function renderMonthGrid() {
  const now = new Date();
  $("monthName").textContent = viewMonth.toLocaleDateString(LOC(), { month: "long" });
  $("yearName").textContent = viewMonth.getFullYear();

  const ws = S.weekStart ?? 6;
  const wk = new Set(S.weekendDays ?? [5, 6]);
  const dow = $("dow");
  dow.innerHTML = "";
  for (let i = 0; i < 7; i++) {
    const wd = (ws + i) % 7;
    dow.append(el("span", wk.has(wd) ? "wknd" : "", LANG === "ar" ? new Date(2026, 0, 4 + wd).toLocaleDateString(LOC(), { weekday: "narrow" }) : "SMTWTFS"[wd]));
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
  const lead = (first.getDay() - ws + 7) % 7;
  const start = addDays(first, -lead);
  const weeks = Math.ceil((lead + new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()) / 7);
  const curWk = weekStartFor(now);
  for (let i = 0; i < weeks * 7; i++) {
    const d = addDays(start, i);
    const k = dayKey(d);
    const cell = el("div", "day");
    if (d.getMonth() !== first.getMonth()) cell.classList.add("out");
    if (wk.has(d.getDay())) cell.classList.add("wknd");
    if (sameDay(d, now)) cell.classList.add("today");
    if (d >= curWk && d < addDays(curWk, 7)) cell.classList.add("curweek", i % 7 === 0 ? "cw-first" : i % 7 === 6 ? "cw-last" : "cw-mid");
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
    if (drafts.some((x) => sameDay(new Date(x.start), d))) { cell.classList.add("pending"); tips.push("Waiting to be added — click Add above"); }
    if (tips.length) cell.title = tips.join("\n");
    const ds = dots.get(k);
    if (ds) {
      const w = el("span", "dots");
      ds.forEach((c) => { const x = el("i"); x.style.background = c || "var(--accent)"; w.append(x); });
      cell.append(w);
    }
    cell.onclick = () => openDay(d);
    cell.ondblclick = () => {
      if (drafts.length) { moveDraftsTo(d); return; } // an event is waiting: move it, don't type into the box
      const q = $("quick");
      q.value = `${q.value ? q.value + " " : ""}on ${d.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" })} `;
      q.focus();
    };
    grid.append(cell);
  }
}


// ---------- flights: "KU 101", "EK857", "flight QR1077" → Flightradar24 ----------
const AIRLINES = new Set(("KU EK QR EY GF WY SV J9 FZ G9 XY F3 MS RJ ME TK PC BA VS LH LX OS SN AF KL IB VY FR U2 W6 AZ " +
  "LO SK AY TP A3 SU PS AA DL UA AC AS B6 WN SQ CX NH JL KE OZ TG MH GA PR CI BR QF NZ VA EI PK AI 6E UK SG " +
  "IX WB UL KQ ET SA AT ZH MU CA CZ HU HX 3U KC HY FJ LY RO JU OU FB BG").split(" "));
const FLIGHT_RE = /\b([A-Z][A-Z0-9]|[0-9][A-Z])\s?-?(\d{1,4})\b/g;
function flightOf(...texts) {
  const txt = texts.filter(Boolean).join("  ");
  const ctx = /flight|✈|airport|depart|arriv|boarding|terminal|gate\b|رحلة/i.test(txt);
  for (const m of txt.matchAll(FLIGHT_RE)) {
    const code = m[1], num = m[2];
    if (/^\d+$/.test(code)) continue;
    if (AIRLINES.has(code) || (ctx && /^[A-Z]{2}$/.test(code))) {
      return { code: code + num, label: `${code} ${num}`, url: `https://www.flightradar24.com/data/flights/${(code + num).toLowerCase()}` };
    }
  }
  return null;
}
// in the air (or about to be): from 1 h before departure to 1 h after the event ends
const flyingSoon = (e) => { const n = Date.now(); return !e.allDay && n >= +e.s - 36e5 && n <= +e.e + 36e5; };

// ---------- calendar views: year | month | week | day ----------
let calView = (() => { try { return localStorage.getItem("calView") || "month"; } catch (_) { return "month"; } })();
let viewYear = new Date().getFullYear();
let weekAnchor = null;           // first day of the week shown in week/day view
let weekView = null;             // { start, events }
const yearCache = new Map();     // year → Map(dayKey → count)

function weekStartFor(d) {
  const ws = S.weekStart ?? 6;
  return addDays(startOfDay(d), -((d.getDay() - ws + 7) % 7));
}

function renderMiniCal() {
  document.querySelectorAll("#views button").forEach((b) => b.classList.toggle("on", b.dataset.view === calView));
  const now = new Date();
  const mc = document.querySelector(".minical");
  mc.classList.remove("year-mode", "strip-mode");
  if (calView === "year") { mc.classList.add("year-mode"); renderYearGrid(); setTodayDot(viewYear !== now.getFullYear()); return; }
  if (calView === "week" || calView === "day") { mc.classList.add("strip-mode"); renderWeekStrip(); return; }
  renderMonthGrid();
  setTodayDot(!sameDay(viewMonth, startOfMonth(now)) || !!selectedDay);
}

function setTodayDot(show) { $("todayBtn").classList.toggle("show", show); }

function renderYearGrid() {
  $("monthName").textContent = String(viewYear);
  $("yearName").textContent = "";
  $("dow").innerHTML = "";
  const grid = $("grid");
  grid.innerHTML = "";
  const counts = yearCache.get(viewYear);
  if (!counts) loadYear(viewYear);
  const now = new Date(), ws = S.weekStart ?? 6, wk = new Set(S.weekendDays ?? [5, 6]);
  for (let m = 0; m < 12; m++) {
    const first = new Date(viewYear, m, 1);
    const box = el("button", "ym" + (m === now.getMonth() && viewYear === now.getFullYear() ? " cur" : ""));
    box.append(el("div", "ym-name", first.toLocaleDateString(LOC(), { month: "short" })));
    const g = el("div", "ym-grid");
    const lead = (first.getDay() - ws + 7) % 7;
    for (let i = 0; i < lead; i++) g.append(el("i", "pad"));
    const days = new Date(viewYear, m + 1, 0).getDate();
    for (let d = 1; d <= days; d++) {
      const dt = new Date(viewYear, m, d);
      const c = el("i", "", "");
      const n = counts ? counts.get(dayKey(dt)) || 0 : 0;
      if (n) c.className = n >= 3 ? "busy" : "has";
      if (wk.has(dt.getDay())) c.classList.add("wk");
      if (sameDay(dt, now)) c.classList.add("today");
      g.append(c);
    }
    box.append(g);
    box.onclick = () => { viewMonth = first; setView("month"); };
    grid.append(box);
  }
}

async function loadYear(y) {
  if (yearCache.has(y)) return;
  yearCache.set(y, new Map());
  try {
    const list = withDates(await go("EventsBetween", new Date(y, 0, 1).toISOString(), new Date(y + 1, 0, 1).toISOString()));
    const m = new Map();
    for (const e of list) {
      if (e.holiday) continue;
      let d = startOfDay(e.s);
      const last = e.allDay ? addDays(startOfDay(e.e), -1) : startOfDay(e.e);
      for (let i = 0; d <= last && i < 31; i++, d = addDays(d, 1)) m.set(dayKey(d), (m.get(dayKey(d)) || 0) + 1);
    }
    yearCache.set(y, m);
    if (calView === "year" && viewYear === y) renderMiniCal();
  } catch (_) { yearCache.delete(y); }
}

function renderWeekStrip() {
  if (!weekAnchor) weekAnchor = weekStartFor(selectedDay || new Date());
  const now = new Date(), wk = new Set(S.weekendDays ?? [5, 6]);
  const end = addDays(weekAnchor, 6);
  if (calView === "day" && selectedDay) {
    $("monthName").textContent = selectedDay.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
    $("yearName").textContent = String(selectedDay.getFullYear());
  } else {
    $("monthName").textContent = weekAnchor.getMonth() === end.getMonth()
      ? `${weekAnchor.getDate()}–${end.getDate()} ${end.toLocaleDateString(LOC(), { month: "short" })}`
      : `${weekAnchor.toLocaleDateString(LOC(), { day: "numeric", month: "short" })} – ${end.toLocaleDateString(LOC(), { day: "numeric", month: "short" })}`;
    $("yearName").textContent = String(end.getFullYear());
  }
  $("dow").innerHTML = "";
  $("grid").innerHTML = "";
  const inToday0 = now >= weekAnchor && now < addDays(weekAnchor, 7);
  setTodayDot(calView === "day" ? !(selectedDay && sameDay(selectedDay, now)) : !inToday0);
  return; // week = one line per day, day = one line per hour (both drawn in the list below)
  const dow = $("dow");
  const counts = new Map();
  const src = (weekView && +weekView.start === +weekAnchor && weekView.events) || monthEvents();
  for (const e of src) {
    if (e.holiday) continue;
    let d = startOfDay(e.s);
    const last = e.allDay ? addDays(startOfDay(e.e), -1) : startOfDay(e.e);
    for (let i = 0; d <= last && i < 14; i++, d = addDays(d, 1)) counts.set(dayKey(d), (counts.get(dayKey(d)) || 0) + 1);
  }
  const grid = $("grid");
  grid.innerHTML = "";
  for (let i = 0; i < 7; i++) {
    const d = addDays(weekAnchor, i);
    const cell = el("div", "day");
    if (wk.has(d.getDay())) cell.classList.add("wknd");
    if (sameDay(d, now)) cell.classList.add("today");
    if (calView === "day" && selectedDay && sameDay(d, selectedDay)) cell.classList.add("sel");
    cell.append(el("span", "num", d.getDate()));
    const n = counts.get(dayKey(d)) || 0;
    if (n) { cell.classList.add("has"); if (n >= 3) cell.classList.add("busy"); cell.title = `${n} event${n > 1 ? "s" : ""}`; }
    const w = weatherFor(d);
    if (w) cell.append(el("span", "strip-wx", `${WX(w.code)}${Math.round(w.max)}°`));
    cell.onclick = () => { selectedDay = d; setView("day"); };
    grid.append(cell);
  }
  const inToday = now >= weekAnchor && now < addDays(weekAnchor, 7);
  setTodayDot(calView === "day" ? !(selectedDay && sameDay(selectedDay, now)) : !inToday);
}

async function loadWeekList(quiet) {
  const start = weekAnchor;
  if (!quiet || !weekView || +weekView.start !== +start) { weekView = { start, events: null }; renderList(); }
  try {
    const list = withDates(await go("EventsBetween", start.toISOString(), addDays(start, 7).toISOString()));
    if (weekView && +weekView.start === +start) { weekView.events = list; renderList(); renderMiniCal(); }
  } catch (err) { toast(errMsg(err), true); }
}

function setView(v) {
  calView = v;
  lastHourScroll = "";
  try { localStorage.setItem("calView", v); } catch (_) {}
  if (v === "day") {
    const d = selectedDay || new Date();
    weekAnchor = weekStartFor(d);
    openDay(d, true);
  } else if (v === "week") {
    weekAnchor = weekAnchor || weekStartFor(selectedDay || new Date());
    selectedDay = null;
    listMode = "week";
    dayView = null;
    loadWeekList();
  } else {
    if (listMode === "day" || listMode === "week") { listMode = "upcoming"; dayView = null; weekView = null; }
    selectedDay = null;
    if (v === "year") loadYear(viewYear);
    renderList();
  }
  renderMiniCal();
}

function stepPeriod(n) {
  if (calView === "year") { viewYear += n; loadYear(viewYear); renderMiniCal(); return; }
  if (calView === "week") { weekAnchor = addDays(weekAnchor, 7 * n); loadWeekList(); renderMiniCal(); return; }
  if (calView === "day") { const d = addDays(selectedDay || new Date(), n); selectedDay = d; weekAnchor = weekStartFor(d); openDay(d, true); return; }
  viewMonth = new Date(viewMonth.getFullYear(), viewMonth.getMonth() + n, 1);
  renderMiniCal(); loadMonth(viewMonth);
  if (playful()) {
    const g = $("grid");
    g.classList.remove("flip-next", "flip-prev"); void g.offsetWidth;
    g.classList.add(n > 0 ? "flip-next" : "flip-prev");
    sound("flip");
  }
}

function goToday() {
  const now = new Date();
  viewMonth = startOfMonth(now); viewYear = now.getFullYear(); weekAnchor = weekStartFor(now);
  if (calView === "day") { selectedDay = now; openDay(now, true); return; }
  if (calView === "week") { loadWeekList(); renderMiniCal(); return; }
  closeDay();
}


// ---------- week view: one line per day ----------
function renderWeekRows(list, start, events) {
  const now = new Date(), wk = new Set(S.weekendDays ?? [5, 6]);
  const box = el("div", "wrows");
  for (let i = 0; i < 7; i++) {
    const d = addDays(start, i), d1 = addDays(d, 1);
    const row = el("div", "wrow");
    if (wk.has(d.getDay())) row.classList.add("wknd");
    if (sameDay(d, now)) row.classList.add("today");
    const lab = el("div", "wrow-day");
    lab.append(el("div", "wrow-dn", d.toLocaleDateString(LOC(), { weekday: "short" })), el("div", "wrow-num", d.getDate()));
    const w = weatherFor(d);
    if (w) lab.append(el("div", "wrow-wx", `${WX(w.code)}${Math.round(w.max)}°`));
    const items = el("div", "wrow-items");
    const day = events.filter((e) => e.s < d1 && e.e > d && (S.showHolidayRows || !e.holiday)).sort((a, b) => (b.allDay - a.allDay) || (a.s - b.s));
    const hols = events.filter((e) => e.holiday && !S.showHolidayRows && e.s < d1 && e.e > d);
    for (const h of hols) items.append(el("span", "wrow-hol", h.title));
    if (!day.length && !hols.length) items.append(el("span", "wrow-free muted", "Free"));
    for (const e of day) {
      const it = el("button", "wrow-ev" + (e.e < now ? " past" : ""));
      it.style.setProperty("--c", e.color || "var(--accent)");
      it.append(el("span", "wrow-t", e.allDay ? "All day" : fmtTime(e.s)), el("span", "wrow-ttl", withEmoji(e)));
      it.onclick = (ev) => { ev.stopPropagation(); openEvent(e); };
      it._ev = e;
      items.append(it);
    }
    row.append(lab, items);
    row.onclick = () => { selectedDay = d; setView("day"); };
    row.title = "Open this day";
    box.append(row);
  }
  list.append(box);
}

// ---------- day view: one line per hour ----------
let lastHourScroll = "";
function renderHourRows(list, d, events) {
  const now = new Date(), day0 = startOfDay(d), day1 = addDays(day0, 1);
  const shown = events.filter((e) => S.showHolidayRows || !e.holiday);
  const hols = events.filter((e) => e.holiday && !S.showHolidayRows);
  if (hols.length) list.append(holidayTags(hols));
  const allDay = shown.filter((e) => e.allDay);
  if (allDay.length) {
    const ad = el("div", "hrow hrow-allday");
    ad.append(el("div", "hrow-h", "All day"));
    const items = el("div", "hrow-items");
    for (const e of allDay) items.append(hourItem(e));
    ad.append(items);
    list.append(ad);
  }
  const timed = shown.filter((e) => !e.allDay);
  const box = el("div", "hrows");
  let scrollTo = null;
  for (let h = 0; h < 24; h++) {
    const hs = new Date(day0.getTime() + h * 36e5), he = new Date(hs.getTime() + 36e5);
    const row = el("div", "hrow");
    if (h < 7 || h >= 22) row.classList.add("night");
    const isNow = sameDay(d, now) && now >= hs && now < he;
    if (isNow) row.classList.add("now");
    if (he <= now && sameDay(d, now)) row.classList.add("past");
    row.append(el("div", "hrow-h", fmtTime(hs)));
    const items = el("div", "hrow-items");
    for (const e of timed) {
      const startsHere = e.s >= hs && e.s < he || (h === 0 && e.s < day0 && e.e > day0);
      const continues = !startsHere && e.s < hs && e.e > hs;
      if (startsHere) items.append(hourItem(e));
      else if (continues) { const c = el("button", "hrow-cont", `↳ ${e.title}`); c.style.setProperty("--c", e.color || "var(--accent)"); c.onclick = (ev) => { ev.stopPropagation(); openEvent(e); }; items.append(c); }
    }
    row.append(items);
    row.onclick = () => {
      const def = S.defaultCalendar || (S.calendars.find((x) => x.primary) || {}).id || "";
      renderEdit({ title: "", start: hs.toISOString(), end: he.toISOString(), allDay: false, location: "", notes: "", calendarId: def, color: "" }, "new");
    };
    row.title = "Click to add an event at " + fmtTime(hs);
    if (isNow) scrollTo = row;
    box.append(row);
  }
  list.append(box);
  // scroll to now (today) or to the first event / 7 am
  const first = timed.length ? Math.min(...timed.map((e) => (e.s < day0 ? 0 : e.s.getHours()))) : 7;
  const target = scrollTo || box.children[Math.min(first, 7)];
  const k = dayKey(d);
  if (lastHourScroll === k) return;
  lastHourScroll = k;
  requestAnimationFrame(() => {
    const sc = $("scroll");
    sc.scrollTop += target.getBoundingClientRect().top - sc.getBoundingClientRect().top - 70;
  });
}

function hourItem(e) {
  const it = el("button", "hrow-ev");
  it._ev = e;
  it.style.setProperty("--c", e.color || "var(--accent)");
  it.append(el("span", "hrow-ttl", withEmoji(e)));
  const sub = e.allDay ? (e.location || "") : `${fmtTime(e.s)} – ${fmtTime(e.e)}${e.location ? " · " + e.location : ""}`;
  if (sub) it.append(el("span", "hrow-sub", sub));
  it.onclick = (ev) => { ev.stopPropagation(); openEvent(e); };
  return it;
}

// ---------- list modes: upcoming | day | search ----------
let listMode = "upcoming";
let dayView = null;      // { d, events }
let searchResults = [];
let searchQ = "";

async function openDay(d, keepView) {
  if (!keepView && sameDay(d, new Date()) && listMode === "day" && dayView && sameDay(dayView.d, d)) { closeDay(); return; }
  selectedDay = d;
  if (calView === "day" || calView === "week") weekAnchor = weekStartFor(d);
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

// reload the open day without resetting its scroll (used when events change)
async function reloadDay() {
  const d = dayView && dayView.d;
  if (!d) return;
  try {
    const list = withDates(await go("EventsBetween", startOfDay(d).toISOString(), addDays(startOfDay(d), 1).toISOString()));
    if (dayView && sameDay(dayView.d, d)) { dayView.events = list; const top = $("scroll").scrollTop; renderList(); $("scroll").scrollTop = top; }
  } catch (_) {}
}

function closeDay() {
  if (calView === "day" || calView === "week") { calView = "month"; try { localStorage.setItem("calView", "month"); } catch (_) {} weekView = null; }
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
  grow.append(el("div", "lbl", lbl), el("div", "ttl", withEmoji(next)));
  let sub = `${fmtTime(next.s)} – ${fmtTime(next.e)}`;
  if (next.location) sub += `  ·  ${next.location}`;
  grow.append(el("div", "sub", sub));
  const tm = (S.travel || {})[`${next.id}|${Math.floor(next.s / 1000)}`];
  if (tm && next.s > now) {
    const leave = new Date(next.s - (tm + 5) * 6e4);
    grow.append(el("div", "sub travel", leave <= now ? `🚗 ~${tm} min drive — leave now!` : `🚗 ~${tm} min drive · leave by ${fmtTime(leave)}`));
  }
  card.append(grow);
  if (next.meetLink) {
    const j = el("button", "join", "Join");
    j.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", next.meetLink); };
    card.append(j);
  }
  const nfl = flightOf(next.title, next.location);
  if (nfl) {
    const f = el("button", "join", flyingSoon(next) ? `✈︎ Live ${nfl.label}` : `✈︎ ${nfl.label}`);
    f.title = "Track on Flightradar24";
    f.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", nfl.url); };
    card.append(f);
  }
  card.onclick = () => openEvent(next);
  box.append(card);
}

// ---------- list ----------
function renderList() {
  const list = $("list");
  list.innerHTML = "";
  if (typeof renderCountdowns === "function") renderCountdowns();
  if (typeof renderTodos === "function") renderTodos();
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

  if (listMode === "week" && weekView) {
    const head = el("div", "mode-head");
    head.append(el("span", "", "This week" === "" ? "" : `Week of ${weekView.start.toLocaleDateString(LOC(), { day: "numeric", month: "short" })}`));
    const big = el("button", "pill-btn", "⤢ Big week view");
    big.title = "Open the week window (⌘⇧W)";
    big.onclick = () => go("OpenWeek");
    head.append(big);
    list.append(head);
    if (!weekView.events) { list.append(emptyState("⏳", "Loading…")); return; }
    renderWeekRows(list, weekView.start, weekView.events);
    return;
  }

  if (listMode === "day" && dayView) {
    const d = dayView.d;
    const head = el("div", "mode-head");
    const ttl = el("span", "", d.toLocaleDateString(LOC(), { weekday: "long", day: "numeric", month: "long" }));
    const back = el("button", "pill-btn", "Upcoming ›");
    back.onclick = closeDay;
    const wxd = weatherChip(d);
    if (wxd) ttl.append(" ", wxd);
    head.append(ttl, back);
    list.append(head);
    if (!dayView.events) { list.append(emptyState("⏳", "Loading…")); return; }
    const evsDay = dayView.events;
    const waiting = drafts.filter((x) => sameDay(new Date(x.start), d));
    if (waiting.length) list.append(pendingBox(waiting));
    if (calView !== "day" && !evsDay.filter((e) => S.showHolidayRows || !e.holiday).length) {
      if (waiting.length) return;
      const hol = evsDay.filter((e) => e.holiday);
      if (hol.length) list.append(holidayTags(hol));
      const em = emptyState("☀️", "Nothing planned.");
      const add = el("button", "btn", "Add something this day");
      add.onclick = () => { if (drafts.length) { moveDraftsTo(d); return; } const q = $("quick"); q.value = `on ${d.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" })} `; q.focus(); };
      em.append(add);
      list.append(em);
      return;
    }
    if (calView === "day") renderHourRows(list, d, evsDay); else renderTimeline(list, d, evsDay);
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
  renderRecap(list);
}

// Groups events by day; holidays become small tags on the day header (unless shown as rows).
function renderGroups(list, events, { now, showPast, spread, dayOnly, from }) {
  const groups = new Map();
  const push = (d, e) => { const k = dayKey(d); if (!groups.has(k)) groups.set(k, { d, items: [], hols: [] }); (e.holiday && !S.showHolidayRows ? groups.get(k).hols : groups.get(k).items).push(e); };
  for (const e of events) {
    if (dayOnly) { push(dayOnly, e); continue; }
    if (e.allDay && spread) {
      const base = from || startOfDay(now);
      let d = startOfDay(e.s) < base ? base : startOfDay(e.s);
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
    const right = el("span", "count", timed ? `${timed} event${timed > 1 ? "s" : ""}` : "");
    const wxc = weatherChip(g.d);
    if (wxc) right.prepend(wxc);
    head.append(el("span", "", dayHead(g.d)), right);
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
  body.append(el("div", "title", withEmoji(e) + (e.recurring ? " 🔁" : "")));
  const meta = [tzNote(e) ? "🌍 " + tzNote(e) : "", e.location, e.meetLink && !e.location ? "Online meeting" : "", S.calendars.length > 1 ? e.calendar : ""].filter(Boolean).join(" · ");
  if (meta) body.append(el("div", "meta", meta));
  const tools = el("div", "tools");
  const fl = flightOf(e.title, e.location);
  if (fl) {
    const f = el("button", "flight-btn" + (flyingSoon(e) ? " live" : ""), "✈︎");
    f.title = `Track ${fl.label} on Flightradar24`;
    f.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", fl.url); };
    tools.append(f);
  }
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
  let full = e;
  try { full = await go("GetEvent", e.id, e.start); } catch (_) {}
  let allFuture = false;
  if (full.recurring || e.recurring) {
    const sc = await scopeBox(`Delete “${e.title}”?`, "Delete");
    if (!sc) return false;
    allFuture = sc === "future";
  }
  try {
    applyState(await go("Delete", e.id, e.start, allFuture));
    afterChange();
    sound("whoosh");
    if (allFuture) toast("Deleted this and all future events");
    else toastAction(`Deleted “${e.title}”`, "Undo", async () => {
      try { const r = await go("Add", [draftFromDetail(full)]); applyState(r.state); afterChange(); toast("Restored"); }
      catch (err) { toast(errMsg(err), true); }
    });
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

// small monochrome symbols for the event card (SF Symbols style, drawn with currentColor)
const SYM = {
  calendar: '<rect x="3.5" y="5" width="17" height="15" rx="3"/><path d="M3.5 10h17M8 3v4M16 3v4"/>',
  repeat: '<path d="M4 11V9a3 3 0 0 1 3-3h12M16 3l3 3-3 3M20 13v2a3 3 0 0 1-3 3H5M8 21l-3-3 3-3"/>',
  bell: '<path d="M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15zM10 20.5a2 2 0 0 0 4 0"/>',
  pin: '<path d="M12 21s-6.5-5.6-6.5-11a6.5 6.5 0 0 1 13 0c0 5.4-6.5 11-6.5 11z"/><circle cx="12" cy="10" r="2.3"/>',
  plane: '<path d="M10.5 13.5 3 11l1.5-1.5 8 .5 4-4.5a2 2 0 0 1 3 3l-4.5 4 .5 8L14 22l-2.5-7.5L8 18v2.5L6.5 22 5 19l-3-1.5L3.5 16H6z"/>',
  video: '<rect x="3" y="6.5" width="12.5" height="11" rx="2.5"/><path d="m15.5 10.5 5-3v9l-5-3z"/>',
  person: '<circle cx="12" cy="8" r="3.8"/><path d="M4.5 20a7.5 7.5 0 0 1 15 0"/>',
  people: '<circle cx="9" cy="8.5" r="3.3"/><path d="M2.5 19.5a6.5 6.5 0 0 1 13 0M15.5 5.5a3.3 3.3 0 0 1 0 6.3M18 13.6a6.5 6.5 0 0 1 3.5 5.9"/>',
  notes: '<rect x="5" y="3.5" width="14" height="17" rx="2.5"/><path d="M8.5 8.5h7M8.5 12h7M8.5 15.5h4"/>',
  link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  globe: '<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.5 2.6 3.6 5.4 3.6 8.5s-1.1 5.9-3.6 8.5c-2.5-2.6-3.6-5.4-3.6-8.5s1.1-5.9 3.6-8.5z"/>',
};
const sym = (name) => `<svg class="sym" viewBox="0 0 24 24" aria-hidden="true">${SYM[name] || ""}</svg>`;


const linkify = (t) => esc(t).replace(/(https?:\/\/[^\s<]+)/g, '<a href="#" data-open="$1">$1</a>').replace(/\n/g, "<br>");

function renderDetail(d) {
  const sh = $("sheet");
  sh.innerHTML = "";
  const s = new Date(d.start), en = new Date(d.end);
  sh.append(sheetHeader(d.title, d.color));
  const body = el("div", "sheet-body");
  body.append(el("div", "sheet-title", withEmoji(d)));
  const when = el("div", "sheet-when");
  when.textContent = whenText(d.start, d.end, d.allDay);
  body.append(when);
  const subs = [];
  if (!d.allDay) subs.push(dur(en - s));
  const tz = tzNote(d);
  if (tz) subs.push(`${tz} local time`);
  if (subs.length) body.append(el("div", "muted small", subs.join("  ·  ")));

  const rows = el("div", "sheet-rows");
  const addRow = (icon, html) => { const r = el("div", "sheet-row"); r.innerHTML = `<span class="ico">${icon}</span><div>${html}</div>`; rows.append(r); return r; };
  addRow(sym("calendar"), `<span class="cal-chip" style="--c:${esc(d.color)}">${esc(d.calendar)}</span>`);
  if (repeats(d.repeat)) addRow(sym("repeat"), esc(repeatText(d.repeat)));
  if (d.hasAlarm) addRow(sym("bell"), esc(alarmText(alarmChoice(d), d.allDay)));
  if (d.location) {
    const key = `${d.id}|${Math.floor(s / 1000)}`;
    const m = (S.travel || {})[key];
    addRow(sym("pin"), `<a href="#" data-open="https://maps.apple.com/?q=${encodeURIComponent(d.location)}">${esc(d.location)}</a>${m ? `<div class="muted small">~${m} min drive · leave by ${fmtTime(new Date(s - (m + 5) * 6e4))}</div>` : ""}`);
  }
  const dfl = flightOf(d.title, d.location, d.notes);
  if (dfl) {
    const live = flyingSoon({ s: new Date(d.start), e: new Date(d.end), allDay: d.allDay });
    addRow(sym("plane"), `<button class="btn primary small-btn" data-open="${esc(dfl.url)}">${live ? "Track live" : "Track"} ${esc(dfl.label)} on Flightradar24</button>${live ? `<div class="muted small">In the air or about to depart — see where it is now</div>` : ""}`);
  }
  if (d.meetLink) addRow(sym("video"), `<button class="btn primary small-btn" data-open="${esc(d.meetLink)}">Join meeting</button>`).classList.add("join-row");
  if (d.organizer) addRow(sym("person"), `Organised by ${esc(d.organizer)}`);
  if (d.attendees && d.attendees.length) {
    const icon = { accepted: ["✓", "Accepted"], declined: ["✕", "Declined"], maybe: ["?", "Maybe"], pending: ["•", "No reply yet"], unknown: ["•", ""] };
    const person = (p) => { const [g, t] = icon[p.status] || icon.unknown; return `<div><span class="rsvp ${esc(p.status)}" title="${t}" aria-label="${t}">${g}</span> ${esc(p.name || p.email)}${p.me ? " <span class='muted'>(you)</span>" : ""}</div>`; };
    addRow(sym("people"), `<div class="people">${d.attendees.map(person).join("")}</div>`);
  }
  if (d.notes) addRow(sym("notes"), `<div class="notes">${linkify(d.notes)}</div>`);
  if (d.url && d.url !== d.meetLink) addRow(sym("link"), `<a href="#" data-open="${esc(d.url)}">${esc(d.url)}</a>`);
  body.append(rows);
  sh.append(body);

  // Mac order: destructive action on the far left, the main action on the far right,
  // everything else in a "⋯" menu so the row never wraps.
  const actions = el("div", "sheet-actions");
  const mk = (label, cls, fn) => { const b = el("button", "btn " + cls, label); b.onclick = fn; actions.append(b); return b; };
  if (d.writable) mk("Delete", "danger-ghost", async () => { if (await deleteEvent(d)) closeSheet(); });
  const more = [];
  if (d.writable) more.push({ label: "Duplicate", fn: () => renderEdit(d, "duplicate") });
  if (s > new Date()) {
    const pinned = (S.pinned || []).includes(d.id);
    more.push({ label: pinned ? "Remove Countdown" : "Pin as Countdown", fn: async () => {
      try {
        applyState(pinned ? await go("UnpinEvent", d.id) : await go("PinEvent", d.id, d.title, d.start));
        toast(pinned ? "Countdown removed" : "Pinned — it's counting down under the AI box");
        renderDetail(d);
      } catch (err) { toast(errMsg(err), true); }
    } });
  }
  more.push({ label: "Open in Calendar", fn: () => go("OpenURL", d.link) });
  const moreBtn = mk("⋯", "more-btn", () => popMenu(moreBtn, more));
  moreBtn.title = "More";
  moreBtn.setAttribute("aria-label", "More actions");
  moreBtn.setAttribute("aria-haspopup", "menu");
  if (d.writable) mk("Edit", "primary", () => renderEdit(d, "edit"));
  else mk("Duplicate", "primary", () => renderEdit(d, "duplicate"));
  sh.append(actions);
  sh.querySelectorAll("[data-open]").forEach((a) => { a.onclick = (ev) => { ev.preventDefault(); go("OpenURL", a.dataset.open); }; });
}

// mode: edit | duplicate | new

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
      <span>Repeat</span><div class="f-rep"><div class="f-pair"><select id="fRepeat"></select><input type="date" id="fUntil" title="Until (optional)"></div><div class="day-chips hidden" id="fDays"></div></div>
      <span>Alert</span><select id="fAlarm"></select>
      <span>Location</span><input id="fLoc" placeholder="Add location">
      <span>Calendar</span><select id="fCal"></select>
    </div>
    <div class="f-clash hidden" id="fClash"></div>
    <textarea id="fNotes" rows="3" placeholder="Notes"></textarea>`;
  sh.append(f);
  $("fTitle").value = d.title || "";
  $("fAllDay").checked = !!d.allDay;
  $("fSDate").value = dateVal(s); $("fSTime").value = timeVal(s);
  $("fEDate").value = dateVal(lastDay); $("fETime").value = timeVal(en);
  $("fLoc").value = d.location || "";
  $("fNotes").value = d.notes || "";
  writable.forEach((c) => { const o = el("option", "", c.name); o.value = c.id; if (c.id === d.calendarId) o.selected = true; $("fCal").append(o); });

  // repeat
  const rep = d.repeat || { freq: "none", interval: 1, weekdays: [], until: "", count: 0 };
  const sd = () => fromInputs($("fSDate").value, "00:00").getDay();
  const fillRepeat = () => {
    const sel = $("fRepeat"); const cur = sel.value; sel.innerHTML = "";
    [["none", "Never"], ["daily", "Every day"], ["weekdays", "Every weekday"], ["weekly", `Every week (${DAY3[sd()]})`],
     ["biweekly", "Every 2 weeks"], ["monthly", "Every month"], ["yearly", "Every year"], ["custom", "Custom days…"]]
      .forEach(([v, l]) => { const o = el("option", "", l); o.value = v; sel.append(o); });
    if (cur) sel.value = cur;
  };
  fillRepeat();
  const wkSet = new Set(S.weekendDays ?? [5, 6]);
  const workdays = [0, 1, 2, 3, 4, 5, 6].filter((x) => !wkSet.has(x));
  let choice = "none";
  if (repeats(rep)) {
    const n = rep.interval || 1, wd = rep.weekdays || [];
    if (rep.freq === "daily" && n === 1) choice = "daily";
    else if (rep.freq === "weekly" && n === 1 && wd.length === workdays.length && workdays.every((x) => wd.includes(x))) choice = "weekdays";
    else if (rep.freq === "weekly" && n === 1 && wd.length <= 1) choice = "weekly";
    else if (rep.freq === "weekly" && n === 2 && wd.length <= 1) choice = "biweekly";
    else if (rep.freq === "monthly" && n === 1) choice = "monthly";
    else if (rep.freq === "yearly" && n === 1) choice = "yearly";
    else choice = "custom";
  }
  $("fRepeat").value = choice;
  $("fUntil").value = rep.until || "";
  const days = $("fDays");
  DAY3.forEach((l, i) => {
    const b = el("button", "day-chip" + ((rep.weekdays || []).includes(i) ? " on" : ""), l[0]);
    b.type = "button"; b.dataset.d = i; b.title = l;
    b.onclick = () => b.classList.toggle("on");
    days.append(b);
  });
  const syncRepeat = () => {
    const v = $("fRepeat").value;
    days.classList.toggle("hidden", v !== "custom");
    $("fUntil").classList.toggle("hidden", v === "none");
  };
  $("fRepeat").onchange = syncRepeat; syncRepeat();
  const readRepeat = () => {
    const v = $("fRepeat").value, until = $("fUntil").value || "";
    const base = { interval: 1, weekdays: [], until, count: 0 };
    switch (v) {
      case "daily": return { ...base, freq: "daily" };
      case "weekdays": return { ...base, freq: "weekly", weekdays: workdays };
      case "weekly": return { ...base, freq: "weekly", weekdays: [sd()] };
      case "biweekly": return { ...base, freq: "weekly", interval: 2, weekdays: [sd()] };
      case "monthly": return { ...base, freq: "monthly" };
      case "yearly": return { ...base, freq: "yearly" };
      case "custom": { const w = [...days.querySelectorAll(".on")].map((b) => +b.dataset.d); return { ...base, freq: "weekly", weekdays: w.length ? w : [sd()] }; }
      default: return { freq: "none", interval: 1, weekdays: [], until: "", count: 0 };
    }
  };
  const origRepeat = JSON.stringify(choice === "none" ? null : { ...rep });

  // alert
  const fillAlarm = () => {
    const allDay = $("fAllDay").checked, sel = $("fAlarm"), cur = sel.value; sel.innerHTML = "";
    const opts = allDay ? [["", "Default"], ["none", "None"], ["dayBefore9", "09:00 the day before"], ["sameDay9", "09:00 on the day"]]
      : [["", "Default"], ["none", "None"], ["0", "At start"], ["5", "5 min before"], ["10", "10 min before"], ["15", "15 min before"], ["30", "30 min before"], ["60", "1 hour before"], ["1440", "1 day before"]];
    opts.forEach(([v, l]) => { const o = el("option", "", l); o.value = v; sel.append(o); });
    if ([...sel.options].some((o) => o.value === cur)) sel.value = cur;
  };
  fillAlarm();
  if (mode === "edit" && d.hasAlarm !== undefined) {
    const c = alarmChoice(d);
    if (![...$("fAlarm").options].some((o) => o.value === c)) { const o = el("option", "", alarmText(c, d.allDay)); o.value = c; $("fAlarm").append(o); }
    $("fAlarm").value = c;
  }

  const syncAllDay = () => { const a = $("fAllDay").checked; $("fSTime").classList.toggle("hidden", a); $("fETime").classList.toggle("hidden", a); fillAlarm(); };
  $("fAllDay").onchange = syncAllDay; syncAllDay();
  let lastStart = fromInputs($("fSDate").value, $("fSTime").value);
  const shift = () => {
    const ns = fromInputs($("fSDate").value, $("fSTime").value);
    const ne = new Date(fromInputs($("fEDate").value, $("fETime").value).getTime() + (ns - lastStart));
    $("fEDate").value = dateVal(ne); $("fETime").value = timeVal(ne);
    lastStart = ns;
    fillRepeat();
    checkClash();
  };
  $("fSDate").onchange = shift; $("fSTime").onchange = shift;
  $("fEDate").onchange = $("fETime").onchange = () => checkClash();

  const readDraft = () => {
    const allDay = $("fAllDay").checked;
    const start = fromInputs($("fSDate").value, allDay ? "00:00" : $("fSTime").value);
    const end = allDay ? addDays(fromInputs($("fEDate").value, "00:00"), 1) : fromInputs($("fEDate").value, $("fETime").value);
    const repeat = readRepeat();
    return { title: $("fTitle").value.trim(), start: start.toISOString(), end: end.toISOString(), allDay,
      location: $("fLoc").value.trim(), notes: $("fNotes").value, calendarId: $("fCal").value, timeZone: d.timeZone || "",
      alarm: $("fAlarm").value, repeat, setRepeat: JSON.stringify(repeats(repeat) ? repeat : null) !== origRepeat, reminderMinutes: 0 };
  };
  let clashTimer;
  async function checkClash() {
    clearTimeout(clashTimer);
    clashTimer = setTimeout(async () => {
      const dr = readDraft();
      if (dr.allDay || new Date(dr.end) <= new Date(dr.start)) { $("fClash").classList.add("hidden"); return; }
      try {
        const cl = await go("CheckClashes", dr, mode === "edit" ? d.id : "");
        $("fClash").classList.toggle("hidden", !cl.length);
        $("fClash").textContent = cl.length ? `⚠️ Overlaps ${cl.map((c) => `“${c.title}” ${fmtTime(new Date(c.start))}`).join(", ")}` : "";
      } catch (_) {}
    }, 250);
  }
  checkClash();

  const actions = el("div", "sheet-actions");
  const save = el("button", "btn primary", mode === "edit" ? "Save changes" : "Add event");
  save.onclick = async () => {
    const draft = readDraft();
    if (new Date(draft.end) <= new Date(draft.start)) { toast("The end must be after the start", true); return; }
    if (!draft.title) { toast("Give it a title", true); return; }
    save.disabled = true;
    try {
      if (mode === "edit") {
        let allFuture = false;
        if (d.recurring || repeats(d.repeat)) {
          if (draft.setRepeat) allFuture = true; // changing the pattern applies from here on
          else {
            const sc = await scopeBox("Save changes", "Save");
            if (!sc) { save.disabled = false; return; }
            allFuture = sc === "future";
          }
        }
        const prev = draftFromDetail(d);
        applyState(await go("UpdateEvent", d.id, d.start, draft, allFuture));
        afterChange();
        toastAction("Saved", "Undo", async () => {
          try { applyState(await go("UpdateEvent", d.id, draft.start, prev, allFuture)); afterChange(); toast("Undone"); } catch (err) { toast(errMsg(err), true); }
        });
      } else {
        const r = await go("Add", [draft]);
        applyState(r.state);
        afterChange();
        celebrate("add");
        toastAction("Added", "Undo", () => undoAdd(r.created, [repeats(draft.repeat)]));
      }
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
    b.innerHTML = `<b>Calendar access needed.</b> Smart Cal shows and adds events using your Mac's calendars (your Google calendar comes through them). macOS asks you to allow this once. <button class="btn primary" id="bannerAllow">Allow</button>`;
    b.classList.remove("hidden");
    $("bannerAllow").onclick = allowAccess;
    return;
  }
  if (S.askLogin && !SETTINGS_WIN) {
    b.innerHTML = `<b>${esc(tr("Start Smart Cal when you log in?"))}</b> <span class="muted">${esc(tr("So your next event is always in the menu bar."))}</span> <button class="btn primary" id="loginYes">${esc(tr("Yes"))}</button> <button class="btn" id="loginNo">${esc(tr("Not now"))}</button>`;
    b.classList.remove("hidden");
    $("loginYes").onclick = async () => { try { applyState(await go("AnswerLogin", true)); toast("Smart Cal will start when you log in"); } catch (err) { toast(errMsg(err), true); } };
    $("loginNo").onclick = async () => { applyState(await go("AnswerLogin", false)); };
    return;
  }
  const parts = [];
  if (!S.hasGoogle && !S.calendars.length) parts.push(`No calendars on this Mac yet <button class="btn" id="bannerGoogle">Add Google…</button>`);
  if (S.trayHidden) parts.push(`macOS is hiding Smart Cal's menu bar item because the menu bar is full. Make room by quitting a menu bar app or holding ⌘ and dragging icons off the bar; then ⌘-drag Smart Cal where you want it — it stays there. Meanwhile ${esc(macKeys(S.hotkey && S.hotkey !== "off" ? S.hotkey : "Ctrl+Alt+C"))} opens Smart Cal, or turn on <b>Also show in the Dock</b> in Settings → Menu bar.`);
  if (!S.hasAI) parts.push(`Add an OpenRouter key for smarter AI (simple lines work without it) — <a href="#" id="bannerGo">Settings</a>`);
  if (S.error) parts.push(esc(S.error));
  if (!parts.length) { b.classList.add("hidden"); return; }
  b.innerHTML = parts.join("<br>");
  b.classList.remove("hidden");
  if ($("bannerGo")) $("bannerGo").onclick = (ev) => { ev.preventDefault(); openSettings("ai"); };
  if ($("bannerGoogle")) $("bannerGoogle").onclick = () => go("AddGoogleAccount");
}

function renderFooter() {
  $("updated").textContent = "";
  $("refreshBtn").title = S.now ? `Refresh (⌘R) · updated ${fmtTime(new Date(S.now))}` : "Refresh (⌘R)";
  $("hotkeyHint").textContent = S.hotkey && S.hotkey !== "off" ? macKeys(S.hotkey) + " to open" : "";
}

let eventsSig = "";
// ---------- today's date at the top of the panel ----------
let todayTimer;
function renderToday() {
  const now = new Date();
  const dow = now.toLocaleDateString(LOC(), { weekday: "long" });
  const date = now.toLocaleDateString(LOC(), { day: "numeric", month: "long" });
  if ($("todayDow").textContent !== dow) $("todayDow").textContent = dow;
  if ($("todayDate").textContent !== date) $("todayDate").textContent = date;
  const year = now.toLocaleDateString(LOC(), { year: "numeric" });
  if ($("todayYear").textContent !== year) $("todayYear").textContent = year;
  // long dates ("Wednesday 30 September 2026") or a bigger text size: shrink a little rather than cut it off
  const line = $("todayLine");
  line.style.fontSize = "";
  for (let px = 22; px > 15 && line.scrollWidth > line.clientWidth; px--) line.style.fontSize = px - 1 + "px";
  // roll over at midnight even if the panel stays open
  clearTimeout(todayTimer);
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 1);
  todayTimer = setTimeout(renderToday, Math.min(next - now, 36e5));
}

function applyState(st) {
  if (!st) return;
  S = st;
  const sig = (S.events || []).map((e) => e.id + e.start + e.end + e.title).join("|");
  if (eventsSig && sig !== eventsSig) {
    // something changed (here or in another app): drop cached ranges and reload what's open
    monthCache.clear(); yearCache.clear();
    if (!SETTINGS_WIN) {
      if (calView === "year") loadYear(viewYear);
      if (listMode === "week" && weekView) loadWeekList(true);
      if (listMode === "day" && dayView) reloadDay();
    }
  }
  eventsSig = sig;
  if ((S.lang || "en") !== LANG) i18nApply(S.lang || "en");
  document.body.classList.toggle("reduce-motion", !!S.reduceMotion);
  document.body.classList.toggle("more-contrast", !!S.contrast);
  if (!pendingAccent) applyAccent(S.accent);
  if (SETTINGS_WIN) { // the Settings window only needs its own sections refreshed
    document.documentElement.style.setProperty("--z", String((S.textSize || 100) / 100));
    if (settingsCache) { renderGoogleBox(); renderRemStatus(); }
    return;
  }
  renderMiniCal();
  renderList();
  renderBanner();
  renderFooter();
  renderMascot();
  renderCountdowns();
  renderTodos();
  checkDayClear();
  document.documentElement.style.setProperty("--z", String((S.textSize || 100) / 100));
  document.body.style.zoom = String((S.textSize || 100) / 100);
  renderToday(); // after the zoom, so the fit check measures the real width
  if (drafts.length) renderDrafts();
  if (!$("settings").classList.contains("hidden")) renderGoogleBox();
  if (mkey(viewMonth) !== mkey(new Date())) loadMonth(viewMonth, true);
}

// =====================================================================
// Playful layer: mascot, greeting, emojis, weather, countdowns, recap,
// confetti/sounds, and the drag-to-reschedule day timeline.
// =====================================================================

const playful = () => S.playful !== false;

// ---------- emojis ----------
const EMOJI_RULES = [
  [/test drive|car\b|garage|service|geely|ford|toyota|lexus|bmw|tesla|oil change/, "🚗"], [/renew|license|licence|passport|visa|civil id|insurance/, "🪪"],
  [/birthday|bday|b-day/, "🎂"], [/anniversar/, "💍"], [/wedding/, "💒"], [/party|celebrat/, "🎉"],
  [/flight|fly|airport|✈/, "✈️"], [/trip|travel|vacation|holiday|getaway/, "🧳"], [/hotel|check.?in/, "🏨"],
  [/dentist|teeth|tooth/, "🦷"], [/doctor|clinic|hospital|checkup|check-up|appointment with dr/, "🩺"],
  [/pharm|medicine|prescription/, "💊"], [/eye|optician/, "👓"], [/vet\b|vet appointment/, "🐾"],
  [/gym|workout|training|crossfit|lift/, "🏋️"], [/run\b|running|jog/, "🏃"], [/swim/, "🏊"], [/yoga|pilates/, "🧘"],
  [/football|soccer|match|game\b/, "⚽"], [/padel|tennis/, "🎾"], [/golf/, "⛳"], [/bike|cycling/, "🚴"],
  [/breakfast/, "🥐"], [/brunch/, "🥞"], [/lunch/, "🍽️"], [/dinner|supper|iftar|suhoor|ghabqa/, "🍲"],
  [/coffee|karak|tea\b|cafe|café/, "☕"], [/drinks|bar\b/, "🍹"], [/bbq|grill/, "🍖"], [/pizza/, "🍕"], [/sushi/, "🍣"],
  [/meeting|sync|standup|stand-up|1:1|one on one|catch.?up|call\b|zoom|teams|meet\b/, "💬"],
  [/interview/, "🤝"], [/review|retro|planning/, "🗂️"], [/present|demo|pitch/, "📊"], [/deadline|due\b/, "⏰"],
  [/class|lesson|lecture|course|teach|tutor/, "📚"], [/exam|test\b|quiz|igcse|sat\b/, "📝"], [/school|pick ?up|drop ?off/, "🎒"],
  [/pay|rent|bill|invoice|bank|transfer|dewa|salary/, "💳"], [/shop|buy|grocery|groceries|mall|avenues/, "🛍️"],
  [/haircut|barber|salon/, "💇"], [/clean|laundry/, "🧺"], [/movie|cinema|film/, "🎬"], [/concert|music|gig/, "🎵"],
  [/mosque|prayer|jumu|friday prayer/, "🕌"], [/family|mom|mum|dad|parents|kids/, "👨‍👩‍👧"], [/date night|date with/, "❤️"],
  [/follow ?up|remind|todo|to-do/, "📌"], [/decide|decision/, "🤔"], [/code|deploy|release|bug/, "💻"], [/write|blog|post/, "✍️"],
  [/beach|sea/, "🏖️"], [/camp|desert|kashta/, "🏕️"], [/garden|plant/, "🪴"], [/dubai|riyadh|london|paris|cairo|istanbul/, "🌍"],
];
const aiEmoji = new Map();     // title → emoji ("" = no good match)
const askedEmoji = new Set();
let emojiTimer;

function emojiFor(e) {
  if (!playful() || !e || !e.title) return "";
  if (e.holiday) return "🎉";
  if (/^✈/.test(e.title)) return "";
  if (flightOf(e.title, e.location)) return "✈️";
  const t = e.title.toLowerCase();
  for (const [re, em] of EMOJI_RULES) if (re.test(t)) return em;
  if (aiEmoji.has(e.title)) return aiEmoji.get(e.title);
  if (!askedEmoji.has(e.title)) { askedEmoji.add(e.title); queueEmojiLookup(); }
  return "";
}
function queueEmojiLookup() {
  if (SETTINGS_WIN) return;
  clearTimeout(emojiTimer);
  emojiTimer = setTimeout(async () => {
    const titles = [...askedEmoji].filter((t) => !aiEmoji.has(t));
    if (!titles.length || !S.hasAI) return;
    try {
      const m = await go("Emojis", titles.slice(0, 40));
      titles.forEach((t) => aiEmoji.set(t, (m && m[t]) || ""));
      renderList();
      renderCountdowns();
    } catch (_) { /* fine without */ }
  }, 600);
}
const withEmoji = (e) => { const em = emojiFor(e); return em ? `${em} ${e.title}` : e.title; };

// ---------- weather ----------
const WX = (c) => c === 0 ? "☀️" : c <= 2 ? "🌤️" : c === 3 ? "☁️" : c <= 48 ? "🌫️" : c <= 57 ? "🌦️" : c <= 67 ? "🌧️" : c <= 77 ? "🌨️" : c <= 82 ? "🌧️" : c <= 86 ? "🌨️" : "⛈️";
function weatherFor(d) {
  if (!playful() || !S.weather) return null;
  const k = dateVal(d);
  return S.weather.find((w) => w.date === k) || null;
}
function weatherChip(d) {
  const w = weatherFor(d);
  if (!w) return null;
  const c = el("span", "wx-chip", `${WX(w.code)} ${Math.round(w.max)}°`);
  c.title = `${S.weatherCity || ""}: high ${Math.round(w.max)}°, low ${Math.round(w.min)}°`;
  if (w.max >= 45) c.classList.add("hot");
  return c;
}

// ---------- mascot ----------
function mascotMood() {
  const now = new Date();
  if (celebratingUntil > Date.now()) return "ecstatic";
  const today = evs().filter((e) => sameDay(e.s, now) || (e.allDay && e.s <= now && e.e > now));
  if (today.some((e) => /birthday|bday/i.test(e.title)) || (S.countdowns || []).some((c) => c.birthday && sameDay(new Date(c.start), now))) return "party";
  const h = now.getHours();
  if (h >= 23 || h < 6) return "sleepy";
  const timed = today.filter((e) => !e.allDay && !e.holiday);
  if (timed.some((e) => e.s <= now && e.e > now)) return "focused";
  if (timed.filter((e) => e.e > now).length >= 5) return "busy";
  return "happy";
}

function mascotSVG(mood) {
  const eyes = {
    happy: `<path d="M22 20q3-4 6 0M36 20q3-4 6 0" class="ln"/>`,
    ecstatic: `<path d="M21 21q3.5-5 7 0M36 21q3.5-5 7 0" class="ln"/>`,
    party: `<path d="M22 20q3-4 6 0M36 20q3-4 6 0" class="ln"/>`,
    busy: `<circle cx="25" cy="19.5" r="1.8" class="dot"/><circle cx="39" cy="19.5" r="1.8" class="dot"/>`,
    focused: `<path d="M22 19.5h6M36 19.5h6" class="ln"/><path d="M21 16.5l7 1.5M43 16.5l-7 1.5" class="ln thin"/>`,
    sleepy: `<path d="M22 20q3 2.5 6 0M36 20q3 2.5 6 0" class="ln"/>`,
  }[mood];
  const mouth = {
    happy: `<path d="M28.5 23.5q3.5 4 7 0z" class="mouth"/>`,
    ecstatic: `<path d="M27 23q5 6.5 10 0z" class="mouth"/>`,
    party: `<path d="M27.5 23.5q4.5 5 9 0z" class="mouth"/>`,
    busy: `<ellipse cx="32" cy="25" rx="1.8" ry="2" class="mouth"/>`,
    focused: `<path d="M29.5 25h5" class="ln"/>`,
    sleepy: `<ellipse cx="32" cy="25" rx="1.4" ry="1" class="mouth"/>`,
  }[mood];
  const extra = {
    busy: `<path d="M50 8q3 4.5 0 7q-3-2.5 0-7z" fill="#5AB8FF"/>`,
    sleepy: `<text x="47" y="11" class="zz">z</text><text x="53" y="6" class="zz small">z</text>`,
    party: `<path d="M24 9l8-9 8 9z" fill="#FFC53D"/><circle cx="32" cy="0.5" r="2" fill="#E5484D"/><circle cx="27" cy="6" r="1" fill="#fff"/><circle cx="35" cy="4" r="1" fill="#fff"/>`,
    ecstatic: `<path d="M53 6l1.2 2.8 2.8 1.2-2.8 1.2L53 14l-1.2-2.8L49 10l2.8-1.2zM9 4l.9 2 2 .9-2 .9L9 10l-.9-2-2-.9 2-.9z" fill="#FFC53D"/>`,
  }[mood] || "";
  return `<svg viewBox="0 -2 64 66" class="mascot-svg ${mood}" aria-label="${mood} calendar">
    <rect x="7" y="11" width="50" height="47" rx="11" class="paper"/>
    <path d="M7 22a11 11 0 0 1 11-11h28a11 11 0 0 1 11 11v8H7z" class="band"/>
    <rect x="17" y="4" width="7" height="14" rx="3.5" class="ring"/><rect x="40" y="4" width="7" height="14" rx="3.5" class="ring"/>
    <g class="face">${eyes}${mouth}<circle cx="20.5" cy="24.5" r="2" class="blush"/><circle cx="43.5" cy="24.5" r="2" class="blush"/></g>
    <g class="cells"><rect x="15" y="35" width="8" height="6" rx="2"/><rect x="28" y="35" width="8" height="6" rx="2" class="hot"/><rect x="41" y="35" width="8" height="6" rx="2"/>
    <rect x="15" y="45" width="8" height="6" rx="2"/><rect x="28" y="45" width="8" height="6" rx="2"/><rect x="41" y="45" width="8" height="6" rx="2"/></g>
    ${extra}</svg>`;
}

const MASCOT_LINES = {
  happy: ["Looking good today!", "I've got your schedule. You've got this. 💪", "Tip: try “what's on Friday?”", "Double-click a day to add something there."],
  busy: ["Busy one! Remember to drink water 💧", "Lots on today — I'll nudge you before each one.", "Tip: drag events in the day view to move them."],
  focused: ["Shh… you're in a meeting 🤫", "Focus mode. I'll keep watch."],
  sleepy: ["Zzz… tomorrow's preview is below.", "Late night? Your calendar can wait. 🌙"],
  party: ["Happy birthday vibes! 🎂", "Party time! 🎉"],
  ecstatic: ["Day clear! 🎉 Nicely done.", "That's everything for today! 🙌"],
};
let celebratingUntil = 0;

function renderMascot() {
  const box = $("greet");
  box.classList.toggle("hidden", !playful());
  if (!playful()) return;
  const mood = mascotMood();
  const m = $("mascot");
  if (m.dataset.mood !== mood) { m.innerHTML = mascotSVG(mood); m.dataset.mood = mood; }
}

function mascotPoke() {
  const m = $("mascot");
  m.classList.remove("boing"); void m.offsetWidth; m.classList.add("boing");
  const lines = MASCOT_LINES[m.dataset.mood] || MASCOT_LINES.happy;
  $("greetText").textContent = lines[Math.floor(Math.random() * lines.length)];
  sound("pop");
  clearTimeout(mascotPoke._t);
  mascotPoke._t = setTimeout(refreshGreeting, 6000);
}

// ---------- greeting ----------
let greetPending = false;
async function refreshGreeting() {
  if (SETTINGS_WIN) return;
  if (!playful() || greetPending) return;
  greetPending = true;
  try {
    const t = await go("GetGreeting");
    if (t) { $("greetText").textContent = t; $("greet").classList.add("ready"); }
  } catch (_) { /* keep the last one */ }
  finally { greetPending = false; }
}

// ---------- countdowns ----------
function daysUntil(d) {
  return Math.round((startOfDay(d) - startOfDay(new Date())) / 864e5);
}
function renderCountdowns() {
  const box = $("countdowns");
  box.innerHTML = "";
  const list = (S.countdowns || []).filter((c) => daysUntil(new Date(c.start)) >= 0);
  if (!list.length || listMode !== "upcoming") return;
  for (const c of list) {
    const n = daysUntil(new Date(c.start));
    const chip = el("button", "cd-chip" + (c.birthday ? " bday" : ""));
    const em = c.birthday ? "🎂" : (emojiFor({ title: c.title }) || "📌");
    chip.innerHTML = `<span class="cd-em">${em}</span><span class="cd-t">${esc(c.title)}</span><span class="cd-n">${n === 0 ? "today!" : n === 1 ? "tomorrow" : n + " days"}</span>`;
    chip.title = whenText(c.start, c.start, true);
    chip.onclick = () => openEvent({ id: c.id, title: c.title, start: c.start, end: c.start, color: "", s: new Date(c.start), e: new Date(c.start) });
    box.append(chip);
  }
}

// ---------- weekly recap ----------
function weekStartOf(d) {
  const ws = S.weekStart ?? 6;
  return addDays(startOfDay(d), -((d.getDay() - ws + 7) % 7));
}
function recapDue() {
  if (!playful()) return false;
  const now = new Date(), wk = new Set(S.weekendDays ?? [5, 6]);
  const firstWeekendDay = wk.has(now.getDay()) && !wk.has(addDays(now, -1).getDay());
  const lastWorkdayEvening = !wk.has(now.getDay()) && wk.has(addDays(now, 1).getDay()) && now.getHours() >= 16;
  let dismissed = "";
  try { dismissed = localStorage.getItem("recapDismissed") || ""; } catch (_) {}
  return (firstWeekendDay || lastWorkdayEvening) && dismissed !== dateVal(weekStartOf(now));
}

async function buildRecap() {
  const ws = weekStartOf(new Date());
  const list = withDates(await go("EventsBetween", ws.toISOString(), addDays(ws, 7).toISOString())).filter((e) => !e.holiday);
  const wk = new Set(S.weekendDays ?? [5, 6]);
  const timed = list.filter((e) => !e.allDay);
  const mins = timed.reduce((a, e) => a + (e.e - e.s) / 6e4, 0);
  const perDay = new Map();
  timed.forEach((e) => { const k = dayKey(e.s); perDay.set(k, { d: startOfDay(e.s), n: ((perDay.get(k) || {}).n || 0) + 1 }); });
  const busiest = [...perDay.values()].sort((a, b) => b.n - a.n)[0];
  // free time on workdays between 08:00 and 18:00
  let free = 0, longest = { m: 0, d: null, from: null };
  for (let i = 0; i < 7; i++) {
    const d = addDays(ws, i);
    if (wk.has(d.getDay())) continue;
    const dayStart = new Date(d); dayStart.setHours(8, 0, 0, 0);
    const dayEnd = new Date(d); dayEnd.setHours(18, 0, 0, 0);
    const busy = timed.filter((e) => e.e > dayStart && e.s < dayEnd).sort((a, b) => a.s - b.s);
    let cur = dayStart;
    for (const e of [...busy, { s: dayEnd, e: dayEnd }]) {
      const gap = (Math.max(cur, Math.min(e.s, dayEnd)) - cur) / 6e4;
      if (gap > 0) { free += gap; if (gap > longest.m) longest = { m: gap, d, from: new Date(cur) }; }
      if (e.e > cur) cur = new Date(Math.min(e.e, dayEnd));
    }
  }
  const n = timed.length;
  const quip = n === 0 ? "A whole week without meetings. Legendary. 🏆"
    : n <= 4 ? "A calm week — plenty of room to think. 🧘"
    : n <= 12 ? "A solid, balanced week. Well played. 👏"
    : n <= 20 ? "That was a busy one — you earned this weekend. 🛋️"
    : "Wow. That's a lot of meetings. Treat yourself. 🍰";
  return { n, hours: mins / 60, busiest, free: free / 60, longest, quip, ws };
}

async function renderRecap(container, force = false) {
  if (!force && !recapDue()) return;
  const card = el("div", "recap");
  card.innerHTML = `<div class="recap-head"><span>✨ Your week</span><button class="icon-btn small" title="Dismiss">✕</button></div><div class="muted small">Crunching numbers…</div>`;
  container.prepend(card);
  card.querySelector("button").onclick = () => {
    try { localStorage.setItem("recapDismissed", dateVal(weekStartOf(new Date()))); } catch (_) {}
    card.remove();
  };
  try {
    const r = await buildRecap();
    const range = `${r.ws.toLocaleDateString(LOC(), { day: "numeric", month: "short" })} – ${addDays(r.ws, 6).toLocaleDateString(LOC(), { day: "numeric", month: "short" })}`;
    card.querySelector(".muted").remove();
    const body = el("div", "recap-body");
    body.innerHTML = `
      <div class="muted small">${range}</div>
      <div class="recap-stats">
        <div><b>${r.n}</b><span>events</span></div>
        <div><b>${r.hours.toFixed(r.hours < 10 ? 1 : 0)}h</b><span>scheduled</span></div>
        <div><b>${Math.round(r.free)}h</b><span>free (8–18)</span></div>
      </div>
      ${r.busiest ? `<div class="recap-line">📈 Busiest: <b>${r.busiest.d.toLocaleDateString(LOC(), { weekday: "long" })}</b> (${r.busiest.n})</div>` : ""}
      ${r.longest.d ? `<div class="recap-line">🌿 Longest free stretch: <b>${dur(r.longest.m * 6e4)}</b> on ${r.longest.d.toLocaleDateString(LOC(), { weekday: "long" })} from ${fmtTime(r.longest.from)}</div>` : ""}
      <div class="recap-quip">${r.quip}</div>`;
    card.append(body);
  } catch (err) { card.remove(); }
}

// ---------- confetti & sounds ----------
function confetti(power = 1) {
  if (!S.confetti || S.reduceMotion) return;
  const c = $("confetti");
  const ctx = c.getContext("2d");
  const W = (c.width = c.offsetWidth * 2), H = (c.height = c.offsetHeight * 2);
  const accent = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#4169e1";
  const colors = [accent, "#FFC53D", "#E5484D", "#2FA36B", "#8B5CF6", "#12A4A4", "#ffffff"];
  const parts = Array.from({ length: Math.round(110 * power) }, () => ({
    x: W / 2 + (Math.random() - 0.5) * W * 0.3, y: H * 0.38,
    vx: (Math.random() - 0.5) * 26, vy: -Math.random() * 30 - 12,
    r: Math.random() * 6 + 4, a: Math.random() * Math.PI, va: (Math.random() - 0.5) * 0.4,
    c: colors[Math.floor(Math.random() * colors.length)], sq: Math.random() > 0.5,
  }));
  const t0 = performance.now();
  (function frame(t) {
    const k = (t - t0) / 1800;
    ctx.clearRect(0, 0, W, H);
    for (const p of parts) {
      p.vy += 1.1; p.vx *= 0.985; p.x += p.vx; p.y += p.vy; p.a += p.va;
      ctx.save(); ctx.globalAlpha = Math.max(0, 1 - k); ctx.translate(p.x, p.y); ctx.rotate(p.a); ctx.fillStyle = p.c;
      if (p.sq) ctx.fillRect(-p.r, -p.r / 2, p.r * 2, p.r); else { ctx.beginPath(); ctx.arc(0, 0, p.r * 0.6, 0, 7); ctx.fill(); }
      ctx.restore();
    }
    if (k < 1) requestAnimationFrame(frame); else ctx.clearRect(0, 0, W, H);
  })(t0);
}

let audioCtx;
function sound(kind) {
  if (!S.sounds) return;
  try {
    audioCtx = audioCtx || new AudioContext();
    const notes = { pop: [[880, 0, 0.07], [1320, 0.05, 0.08]], chime: [[523, 0, 0.18], [659, 0.1, 0.18], [784, 0.2, 0.3]],
      flip: [[300, 0, 0.03]], whoosh: [[520, 0, 0.09], [330, 0.06, 0.12]] }[kind] || [];
    const t = audioCtx.currentTime;
    for (const [f, at, d] of notes) {
      const o = audioCtx.createOscillator(), g = audioCtx.createGain();
      o.type = kind === "flip" ? "triangle" : "sine";
      o.frequency.value = f;
      g.gain.setValueAtTime(0.0001, t + at);
      g.gain.exponentialRampToValueAtTime(0.12, t + at + 0.01);
      g.gain.exponentialRampToValueAtTime(0.0001, t + at + d);
      o.connect(g).connect(audioCtx.destination);
      o.start(t + at); o.stop(t + at + d + 0.02);
    }
  } catch (_) {}
}

function celebrate(kind) {
  if (!playful()) return;
  if (kind === "add") { confetti(0.8); sound("pop"); }
  if (kind === "clear") {
    celebratingUntil = Date.now() + 8000;
    confetti(1.3); sound("chime");
    renderMascot();
    $("greetText").textContent = MASCOT_LINES.ecstatic[0];
    setTimeout(() => { renderMascot(); refreshGreeting(); }, 8200);
  }
}

// "Day clear!" — once per day, when today had timed events and they've all finished.
function checkDayClear() {
  if (!playful()) return;
  const now = new Date();
  const today = evs().filter((e) => !e.allDay && !e.holiday && sameDay(e.s, now));
  const k = "dayClear-" + dateVal(now);
  let done = false;
  try { done = !!localStorage.getItem(k); } catch (_) {}
  if (done) return;
  // evs() only holds events that haven't ended yet, so "had meetings today" is tracked separately
  if (today.length) { try { localStorage.setItem("hadToday-" + dateVal(now), "1"); } catch (_) {} return; }
  let had = false;
  try { had = !!localStorage.getItem("hadToday-" + dateVal(now)); } catch (_) {}
  if (had && now.getHours() >= 9) {
    try { localStorage.setItem(k, "1"); } catch (_) {}
    celebrate("clear");
  }
}

// ---------- toast with an action (Undo) ----------
// ⌘Z undoes the last change that offered "Undo"; ⌘⇧Z puts it back where the change can be replayed.
const undoStack = [], redoStack = [];
async function undoLast() {
  const u = undoStack.pop();
  if (!u) { toast(tr("Nothing to undo")); return; }
  try { await u.fn(); toast(tr("Undone") + ": " + u.msg); if (u.redo) redoStack.push(u); } catch (err) { toast(errMsg(err), true); }
}
async function redoLast() {
  const r = redoStack.pop();
  if (!r) { toast(tr("Nothing to redo")); return; }
  try { await r.redo(); toast(tr("Redone") + ": " + r.msg); } catch (err) { toast(errMsg(err), true); }
}
function toastAction(msg, label, fn, redo) {
  if (label === "Undo") {
    const entry = { msg, fn, redo };
    undoStack.push(entry); if (undoStack.length > 30) undoStack.shift();
    redoStack.length = 0;
    const orig = fn;
    fn = async () => { const i = undoStack.indexOf(entry); if (i >= 0) undoStack.splice(i, 1); await orig(); if (redo) redoStack.push(entry); };
    label = tr("Undo");
  }
  const box = $("toast");
  box.innerHTML = "";
  box.className = "toast";
  box.append(el("span", "", msg));
  const b = el("button", "toast-btn", label);
  b.onclick = () => { box.classList.add("hidden"); fn(); };
  box.append(b);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hidden"), 6000);
}

// ---------- day timeline with drag-to-reschedule ----------
const HOUR = 44; // px per hour
const SNAP = 15; // minutes

function renderTimeline(list, d, events) {
  const allDay = events.filter((e) => e.allDay || e.holiday);
  const timed = events.filter((e) => !e.allDay && !e.holiday).sort((a, b) => a.s - b.s);
  if (allDay.length) {
    const hol = allDay.filter((e) => e.holiday && !S.showHolidayRows);
    if (hol.length) list.append(holidayTags(hol));
    allDay.filter((e) => !(e.holiday && !S.showHolidayRows)).forEach((e) => list.append(eventRow(e, new Date())));
  }
  const day0 = startOfDay(d);
  const hourOf = (t) => (t - day0) / 36e5;
  let h0 = 8, h1 = 20;
  timed.forEach((e) => { h0 = Math.min(h0, Math.floor(Math.max(0, hourOf(e.s)))); h1 = Math.max(h1, Math.ceil(Math.min(24, hourOf(e.e)))); });
  const tl = el("div", "timeline");
  tl.style.height = `${(h1 - h0) * HOUR}px`;
  for (let h = h0; h <= h1; h++) {
    const line = el("div", "tl-hour");
    line.style.top = `${(h - h0) * HOUR}px`;
    const lbl = new Date(day0); lbl.setHours(h);
    line.append(el("span", "", h === 24 ? "" : fmtTime(lbl)));
    tl.append(line);
  }
  if (sameDay(d, new Date())) {
    const nowLine = el("div", "tl-now");
    nowLine.style.top = `${(hourOf(new Date()) - h0) * HOUR}px`;
    tl.append(nowLine);
  }
  // simple overlap layout: columns within clusters
  const cols = [];
  const placed = timed.map((e) => {
    let c = 0;
    while (cols[c] && cols[c] > e.s) c++;
    cols[c] = e.e;
    return { e, c };
  });
  const width = Math.max(1, cols.length);
  for (const { e, c } of placed) tl.append(timelineBlock(e, c, width, h0, day0));
  // double-click empty space: new event at that time
  tl.ondblclick = (ev) => {
    if (ev.target !== tl && !ev.target.classList.contains("tl-hour")) return;
    const y = ev.clientY - tl.getBoundingClientRect().top;
    const mins = Math.round(((y / HOUR) * 60) / 30) * 30;
    const s = new Date(day0.getTime() + (h0 * 60 + mins) * 6e4);
    const def = S.defaultCalendar || (S.calendars.find((x) => x.primary) || {}).id || "";
    renderEdit({ title: "", start: s.toISOString(), end: new Date(s.getTime() + 36e5).toISOString(), allDay: false, location: "", notes: "", calendarId: def, color: "" }, "new");
  };
  list.append(tl);
  list.append(el("div", "tl-hint muted small", "Drag to move · drag the bottom edge to resize · double-click empty space to add"));
}

function timelineBlock(e, col, cols, h0, day0) {
  const b = el("div", "tl-ev");
  b._ev = e;
  const top = ((e.s - day0) / 36e5 - h0) * HOUR;
  const height = Math.max(18, ((e.e - e.s) / 36e5) * HOUR - 2);
  Object.assign(b.style, { top: `${top}px`, height: `${height}px`, left: `calc(52px + (100% - 56px) * ${col / cols})`, width: `calc((100% - 56px) / ${cols} - 3px)`, "--c": e.color || "var(--accent)" });
  const ttl = el("div", "tl-title", withEmoji(e));
  const tm = el("div", "tl-time", `${fmtTime(e.s)} – ${fmtTime(e.e)}`);
  b.append(ttl, tm);
  if (height < 34) b.classList.add("short");
  const editable = e.writable !== false;
  if (editable) { const h = el("div", "tl-resize"); b.append(h); }

  let mode = null, y0 = 0, moved = false, ds = 0, de = 0;
  const snapMin = (px) => Math.round(((px / HOUR) * 60) / SNAP) * SNAP;
  b.onpointerdown = (ev) => {
    if (ev.button !== 0 || ev.ctrlKey) return; // right-click / Control-click opens the menu instead
    mode = editable && ev.target.classList.contains("tl-resize") ? "resize" : editable ? "move" : "click";
    y0 = ev.clientY; moved = false; ds = de = 0;
    b.setPointerCapture(ev.pointerId);
    ev.preventDefault();
  };
  b.onpointermove = (ev) => {
    if (!mode || mode === "click") return;
    const dy = ev.clientY - y0;
    if (Math.abs(dy) > 4) moved = true;
    if (!moved) return;
    const m = snapMin(dy);
    if (mode === "move") { ds = de = m; b.style.transform = `translateY(${(m / 60) * HOUR}px)`; }
    else { de = Math.max(m, SNAP - (e.e - e.s) / 6e4); b.style.height = `${Math.max(18, height + (de / 60) * HOUR)}px`; }
    b.classList.add("dragging");
    const ns = new Date(e.s.getTime() + ds * 6e4), ne = new Date(e.e.getTime() + de * 6e4);
    tm.textContent = `${fmtTime(ns)} – ${fmtTime(ne)}`;
  };
  b.onpointerup = async () => {
    const m0 = mode; mode = null;
    if (!m0) return; // the press wasn't ours (right-click / Control-click)
    b.classList.remove("dragging");
    if (!moved || m0 === "click") { if (!moved) openEvent(e); return; }
    if (!ds && !de) return;
    const ns = new Date(e.s.getTime() + ds * 6e4), ne = new Date(e.e.getTime() + de * 6e4);
    const draft = { title: e.title, start: ns.toISOString(), end: ne.toISOString(), allDay: false, location: e.location || "", notes: "", calendarId: e.calendarId, reminderMinutes: 0 };
    try {
      // keep the notes: fetch details first so the update doesn't wipe them
      try { const full = await go("GetEvent", e.id, e.start); draft.notes = full.notes || ""; draft.timeZone = full.timeZone || ""; } catch (_) {}
      let allFuture = false;
      if (e.recurring) {
        const sc = await scopeBox(`Move “${e.title}”`, "Move");
        if (!sc) { afterChange(); return; }
        allFuture = sc === "future";
      }
      applyState(await go("UpdateEvent", e.id, e.start, draft, allFuture));
      go("CheckClashes", draft, e.id).then((cl) => { if (cl && cl.length) setTimeout(() => toast(`⚠️ Now overlaps “${cl[0].title}”`), 2500); }).catch(() => {});
      sound("pop");
      const undo = { ...draft, start: e.start, end: e.end };
      toastAction(`Moved to ${fmtTime(ns)}–${fmtTime(ne)}`, "Undo", async () => {
        try { applyState(await go("UpdateEvent", e.id, draft.start, undo, allFuture)); afterChange(); toast("Undone"); } catch (err) { toast(errMsg(err), true); }
      }, async () => { applyState(await go("UpdateEvent", e.id, undo.start, draft, allFuture)); afterChange(); });
      afterChange();
    } catch (err) { toast(errMsg(err), true); afterChange(); }
  };
  return b;
}

// =====================================================================
// v1.8 practical layer: repeats, scope choice, undo, clashes, slots,
// paste/drop/clipboard → events, to-dos, travel time, time zones.
// =====================================================================

const DAY3 = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const localTZ = Intl.DateTimeFormat().resolvedOptions().timeZone;
const repeats = (r) => r && r.freq && r.freq !== "none" && r.freq !== "keep";

function repeatText(r) {
  if (!repeats(r)) return "";
  const n = Math.max(1, r.interval || 1);
  let s;
  if (r.freq === "weekly" && (r.weekdays || []).length && n === 1) {
    const d = r.weekdays.map((w) => DAY3[w]);
    s = d.length === 5 && !r.weekdays.includes(5) && !r.weekdays.includes(6) ? "Every weekday (Sun–Thu)"
      : "Every " + (d.length > 1 ? d.slice(0, -1).join(", ") + " & " + d[d.length - 1] : d[0]);
  } else if (n === 1) s = { daily: "Every day", weekly: "Every week", monthly: "Every month", yearly: "Every year" }[r.freq];
  else s = `Every ${n} ${{ daily: "days", weekly: "weeks", monthly: "months", yearly: "years" }[r.freq]}`;
  if (r.until) s += " until " + new Date(r.until + "T00:00").toLocaleDateString(LOC(), { day: "numeric", month: "short", year: "numeric" });
  else if (r.count > 0) s += `, ${r.count} times`;
  return s;
}

// time in the event's own zone when it differs from the Mac's ("14:00 Dubai")
function tzNote(e, when) {
  if (!e.timeZone || e.allDay || e.timeZone === localTZ) return "";
  try {
    const t = new Date(when || e.start);
    const loc = t.toLocaleTimeString(LOC(), { hour: "2-digit", minute: "2-digit", hour12: !S.use24h, timeZone: e.timeZone });
    const local = t.toLocaleTimeString(LOC(), { hour: "2-digit", minute: "2-digit", hour12: !S.use24h });
    if (loc === local) return "";
    return `${loc} ${e.timeZone.split("/").pop().replace(/_/g, " ")}`;
  } catch (_) { return ""; }
}

// Delete / save a repeating event: this one or this and all future?
function scopeBox(title, verb) {
  return new Promise((res) => {
    $("confirmText").innerHTML = `<b>${esc(title)}</b><span class="muted">This is a repeating event.</span>`;
    const yes = $("confirmYes"), no = $("confirmNo");
    const extra = el("button", "btn", `${verb} all future`);
    yes.textContent = `${verb} this one`;
    yes.className = "btn " + (verb === "Delete" ? "danger" : "primary");
    no.parentElement.insertBefore(extra, yes);
    $("confirm").classList.remove("hidden");
    const done = (v) => {
      $("confirm").classList.add("hidden");
      extra.remove();
      yes.textContent = "Delete"; yes.className = "btn danger";
      yes.onclick = no.onclick = null;
      res(v);
    };
    yes.onclick = () => done("this");
    extra.onclick = () => done("future");
    no.onclick = () => done(null);
  });
}

function alarmChoice(d) {
  if (!d.hasAlarm) return "none";
  const s = Math.round(d.alarmSecs || 0);
  if (d.allDay) return s === -54000 ? "dayBefore9" : s === 32400 ? "sameDay9" : "dayBefore9";
  return String(Math.max(0, Math.round(-s / 60)));
}
function alarmText(choice, allDay) {
  if (!choice || choice === "none") return "";
  if (choice === "dayBefore9") return "09:00 the day before";
  if (choice === "sameDay9") return "09:00 on the day";
  const m = +choice;
  if (m === 0) return "At start";
  const unit = (n, one, many) => `${n} ${n === 1 ? one : many} before`;
  if (m % 10080 === 0) return unit(m / 10080, "week", "weeks");
  if (m % 1440 === 0) return unit(m / 1440, "day", "days");
  if (m % 60 === 0) return unit(m / 60, "hour", "hours");
  if (m > 1440) return `${Math.floor(m / 1440)} d ${Math.round((m % 1440) / 60)} h before`;
  if (m > 60) return `${Math.floor(m / 60)} h ${m % 60} min before`;
  return `${m} min before`;
}

function draftFromDetail(d) {
  return { title: d.title, start: d.start, end: d.end, allDay: !!d.allDay, location: d.location || "", notes: d.notes || "",
    calendarId: d.calendarId, timeZone: d.timeZone || "", alarm: d.hasAlarm !== undefined ? alarmChoice(d) : "",
    repeat: { freq: "none", interval: 1, weekdays: [], until: "", count: 0 }, setRepeat: false, reminderMinutes: 0 };
}

async function undoAdd(created, wholeSeries) {
  try {
    let st;
    for (let i = 0; i < created.length; i++) st = await go("Delete", created[i].id, created[i].start, !!(wholeSeries && wholeSeries[i]));
    if (st) applyState(st);
    afterChange();
    toast("Undone");
  } catch (err) { toast(errMsg(err), true); }
}

// ---------- attachments: paste / drop / clipboard ----------
let attachment = null; // base64 JPEG

function shrinkImage(file) {
  return new Promise((res, rej) => {
    const img = new Image();
    img.onload = () => {
      const k = Math.min(1, 1600 / Math.max(img.width, img.height));
      const c = document.createElement("canvas");
      c.width = Math.max(1, Math.round(img.width * k)); c.height = Math.max(1, Math.round(img.height * k));
      const ctx = c.getContext("2d");
      ctx.fillStyle = "#fff"; ctx.fillRect(0, 0, c.width, c.height);
      ctx.drawImage(img, 0, 0, c.width, c.height);
      res(c.toDataURL("image/jpeg", 0.82));
      URL.revokeObjectURL(img.src);
    };
    img.onerror = rej;
    img.src = URL.createObjectURL(file);
  });
}
async function attachImage(file) {
  try {
    const url = await shrinkImage(file);
    attachment = url.split(",")[1];
    $("attachImg").src = url;
    $("attach").classList.remove("hidden");
    $("quick").focus();
    if (!S.hasKey) toast("Reading screenshots needs your OpenRouter key (Settings → AI)", true);
  } catch (_) { toast("Couldn't read that image", true); }
}
function clearAttachment() {
  attachment = null;
  $("attach").classList.add("hidden");
  $("attachImg").removeAttribute("src");
}

// ---------- to-dos ----------
let todoAdding = false;
let todoAll = false;
function todoDue(t) { return t.due ? new Date(t.due) : null; }

// When a to-do is due, in words: "Today 09:00 · in 4 hours", "Wed 18 Nov, 09:00 · in 7 days",
// "Overdue by 2 days"; more than two weeks away it's just the date ("Wed 2 Dec").
function dueText(due, hasTime, now = new Date()) {
  const days = Math.round((startOfDay(due) - startOfDay(now)) / 864e5);
  const date = due.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
  const time = hasTime ? fmtTime(due) : "";
  const n = (v, one, many) => `${v} ${v === 1 ? one : many}`;
  if (hasTime ? due < now : days < 0) {
    const mins = Math.round((now - due) / 6e4);
    const by = !hasTime || mins >= 1440 ? n(Math.max(1, hasTime ? Math.floor(mins / 1440) : -days), "day", "days")
      : mins >= 60 ? n(Math.floor(mins / 60), "hour", "hours") : n(Math.max(1, mins), "minute", "minutes");
    return `Overdue by ${by}`;
  }
  if (days === 0) {
    if (!hasTime) return "Today";
    const mins = Math.round((due - now) / 6e4);
    return `Today ${time} · in ${mins >= 60 ? n(Math.round(mins / 60), "hour", "hours") : n(Math.max(1, mins), "minute", "minutes")}`;
  }
  if (days === 1) {
    const mins = Math.round((due - now) / 6e4);
    return `Tomorrow${time ? " " + time : ""}${hasTime && mins < 1440 ? ` · in ${mins >= 60 ? n(Math.round(mins / 60), "hour", "hours") : n(Math.max(1, mins), "minute", "minutes")}` : ""}`;
  }
  if (days < 14) return `${date}${time ? ", " + time : ""} · in ${days} days`;
  return `${date}${time ? ", " + time : ""}`;
}
function renderTodos() {
  const box = $("todos");
  box.innerHTML = "";
  if (!S.connected || S.showTodos === false || listMode !== "upcoming") return;
  let dismissed = false;
  try { dismissed = !!localStorage.getItem("todoPromptDismissed"); } catch (_) {}
  if (S.remindersAccess !== "full") {
    if (dismissed) return;
    const p = el("div", "todo-prompt");
    p.innerHTML = `<span>✅ Show your to-dos from Apple Reminders here</span>`;
    const allow = el("button", "btn primary small-btn", "Allow");
    allow.onclick = async () => { try { applyState(await go("AllowReminders")); } catch (err) { toast(errMsg(err), true); } };
    const x = el("button", "icon-btn small", "✕");
    x.onclick = () => { try { localStorage.setItem("todoPromptDismissed", "1"); } catch (_) {} renderTodos(); };
    p.append(allow, x);
    box.append(p);
    return;
  }
  const now = new Date(), sod = startOfDay(now);
  const list = (S.todos || []).filter((t) => !t.done).sort((a, b) => {
    const da = todoDue(a), db = todoDue(b);
    if (!da && !db) return 0; if (!da) return 1; if (!db) return -1; return da - db;
  });
  const head = el("div", "todo-head");
  const overdue = list.filter((t) => todoDue(t) && todoDue(t) < sod).length;
  head.innerHTML = `<span>To-do</span><span class="count">${list.length ? list.length + (overdue ? ` · <b class="od">${overdue} overdue</b>` : "") : "all done ✨"}</span>`;
  const add = el("button", "icon-btn small", "+");
  add.title = "Add a to-do";
  add.onclick = () => { todoAdding = true; renderTodos(); };
  head.append(add);
  box.append(head);
  if (todoAdding) {
    const row = el("div", "todo-new");
    const inp = el("input");
    inp.placeholder = "New to-do — press ↩ (tip: “call bank tomorrow” in the AI box sets a date)";
    inp.onkeydown = async (ev) => {
      if (ev.key === "Escape") { todoAdding = false; renderTodos(); }
      if (ev.key === "Enter" && inp.value.trim()) {
        try {
          const ids = await go("AddTodos", [{ title: inp.value.trim(), notes: "", due: null, hasTime: false }]);
          inp.value = "";
          toastAction("To-do added", "Undo", async () => { for (const id of ids) await go("DeleteTodo", id); go("Refresh").then(applyState); });
          go("Refresh").then(applyState);
        } catch (err) { toast(errMsg(err), true); }
      }
    };
    row.append(inp);
    box.append(row);
    setTimeout(() => inp.focus(), 20);
  }
  const shown = todoAll ? list : list.slice(0, 5);
  for (const t of shown) box.append(todoRow(t, sod));
  if (list.length > 5) {
    const more = el("button", "link-btn todo-more", todoAll ? "Show less" : `${list.length - 5} more…`);
    more.onclick = () => { todoAll = !todoAll; renderTodos(); };
    box.append(more);
  }
}
function todoRow(t, sod) {
  const r = el("div", "todo");
  const cb = el("button", "todo-check");
  cb.style.setProperty("--c", t.color || "var(--accent)");
  cb.title = "Mark done";
  cb.onclick = async () => {
    r.classList.add("done");
    sound("pop");
    try {
      applyState(await go("CompleteTodo", t.id, true));
      toastAction(`Done: ${t.title}`, "Undo", async () => { applyState(await go("CompleteTodo", t.id, false)); });
    } catch (err) { r.classList.remove("done"); toast(errMsg(err), true); }
  };
  const body = el("div", "todo-body");
  body.append(el("div", "todo-title", t.title));
  const due = todoDue(t);
  const meta = [];
  if (due) {
    meta.push(dueText(due, t.hasTime));
    if (t.hasTime ? due < new Date() : due < sod) r.classList.add("overdue");
  }
  if ((S.todos || []).some((x) => x.list !== t.list)) meta.push(t.list);
  if (meta.length) body.append(el("div", "todo-meta", meta.join(" · ")));
  const del = el("button", "icon-btn small todo-del", "🗑");
  del.title = "Delete";
  del.onclick = async () => {
    try {
      applyState(await go("DeleteTodo", t.id));
      toastAction("To-do deleted", "Undo", async () => {
        await go("AddTodos", [{ title: t.title, notes: t.notes || "", due: t.due, hasTime: t.hasTime }]);
        go("Refresh").then(applyState);
      });
    } catch (err) { toast(errMsg(err), true); }
  };
  r.append(cb, body, del);
  r.title = "Right-click to reschedule";
  r.oncontextmenu = (ev) => { ev.preventDefault(); todoMenu(t, r, { x: ev.clientX, y: ev.clientY }); };
  return r;
}

// ---------- right-click an event: reschedule it ----------
// Same choices as to-dos. Days count from today, the event keeps its time of day and its length.
function eventMenu(e, row, at) {
  const s = new Date(e.start), today = startOfDay(new Date());
  const short = (d) => d.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
  const items = [];
  if (e.writable !== false && !e.holiday) {
    for (const [label, n] of [["Today", 0], ["Tomorrow", 1], ["In 2 days", 2], ["In 3 days", 3], ["Next week", 7]]) {
      const day = addDays(today, n);
      if (sameDay(day, s)) continue; // already on that day
      items.push({ label, hint: short(day), fn: () => moveEventToDay(e, day) });
    }
    items.push({ label: "Pick a date…", fn: () => pickEventDate(e, at) }, "-");
  }
  items.push({ label: "Open", fn: () => openEvent(e) });
  if (e.writable !== false) items.push({ label: "Edit…", fn: async () => { let d = e; try { d = await go("GetEvent", e.id, e.start); } catch (_) {} $("sheet").classList.remove("hidden"); renderEdit(d, "edit"); } });
  items.push({ label: "Duplicate", fn: async () => { let d = e; try { d = await go("GetEvent", e.id, e.start); } catch (_) {} $("sheet").classList.remove("hidden"); renderEdit(d, "duplicate"); } });
  if (e.writable !== false) items.push("-", { label: "Delete", danger: true, fn: () => deleteEvent(e) });
  popMenu(row, items, at);
}

// move an event to another day, same time and length; keeps notes and time zone; asks for repeats; Undo
async function moveEventToDay(e, day) {
  const s = new Date(e.start), en = new Date(e.end);
  const shift = Math.round((startOfDay(day) - startOfDay(s)) / 864e5);
  if (!shift) return;
  const ns = new Date(s), ne = new Date(en);
  ns.setDate(ns.getDate() + shift); ne.setDate(ne.getDate() + shift); // calendar days, so DST can't shift the time
  const draft = { title: e.title, start: ns.toISOString(), end: ne.toISOString(), allDay: !!e.allDay, location: e.location || "", notes: "", calendarId: e.calendarId, reminderMinutes: 0, timeZone: e.timeZone || "" };
  try {
    try { const full = await go("GetEvent", e.id, e.start); draft.notes = full.notes || ""; draft.timeZone = full.timeZone || draft.timeZone; } catch (_) {}
    let allFuture = false;
    if (e.recurring) {
      const sc = await scopeBox(`Move “${e.title}”`, "Move");
      if (!sc) return;
      allFuture = sc === "future";
    }
    applyState(await go("UpdateEvent", e.id, e.start, draft, allFuture));
    go("CheckClashes", draft, e.id).then((cl) => { if (cl && cl.length) setTimeout(() => toast(`⚠️ Now overlaps “${cl[0].title}”`), 2500); }).catch(() => {});
    sound("pop");
    const undo = { ...draft, start: new Date(e.start).toISOString(), end: new Date(e.end).toISOString() };
    toastAction(`Moved to ${dayLabel(ns)}${e.allDay ? "" : " " + fmtTime(ns)}`, "Undo", async () => {
      try { applyState(await go("UpdateEvent", e.id, draft.start, undo, allFuture)); afterChange(); toast("Undone"); } catch (err) { toast(errMsg(err), true); }
    });
    afterChange();
  } catch (err) { toast(errMsg(err), true); }
}

// "Pick a date…" for an event: a small date box at the pointer; ↩ or Move saves, Esc cancels
function pickEventDate(e, at) {
  pickDate(at, `Move “${e.title}” to`, new Date(e.start), (day) => moveEventToDay(e, day));
}

// Reschedule a to-do from its right-click menu. Days count from today; a to-do with a time keeps it.
function todoMenu(t, row, at) {
  const due = todoDue(t), today = startOfDay(new Date());
  const on = (days) => {
    const d = addDays(today, days);
    if (t.hasTime && due) d.setHours(due.getHours(), due.getMinutes(), 0, 0);
    return d;
  };
  const short = (d) => d.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
  const opt = (label, days) => ({ label, hint: short(on(days)), fn: () => rescheduleTodo(t, on(days), t.hasTime && !!due) });
  const items = [opt("Today", 0), opt("Tomorrow", 1), opt("In 2 days", 2), opt("In 3 days", 3), opt("Next week", 7),
    { label: "Pick a date…", fn: () => pickTodoDate(t, row) }];
  if (due) items.push({ label: "Remove date", fn: () => rescheduleTodo(t, null, false) });
  items.push("-",
    { label: "Mark as done", fn: () => row.querySelector(".todo-check").click() },
    { label: "Delete", danger: true, fn: () => row.querySelector(".todo-del").click() });
  popMenu(row, items, at);
}

async function rescheduleTodo(t, when, hasTime) {
  const before = { due: t.due ? new Date(t.due) : null, hasTime: t.hasTime };
  try {
    applyState(await go("UpdateTodo", t.id, t.title, when ? when.toISOString() : null, hasTime));
    const msg = when ? `“${t.title}” is now due ${dueText(when, hasTime).replace(/^Today/, "today").replace(/^Tomorrow/, "tomorrow")}` : `“${t.title}” has no date now`;
    toastAction(msg, "Undo", async () => {
      try { applyState(await go("UpdateTodo", t.id, t.title, before.due ? before.due.toISOString() : null, before.hasTime)); } catch (err) { toast(errMsg(err), true); }
    });
  } catch (err) { toast(errMsg(err), true); }
}

// "Pick a date…": a date field in place of the to-do's date line; ↩ or picking a day saves, Esc cancels
function pickTodoDate(t, row) {
  const body = row.querySelector(".todo-body");
  const old = body.querySelector(".todo-meta");
  const inp = el("input", "todo-date");
  inp.type = "date";
  const due = todoDue(t) || addDays(startOfDay(new Date()), 1);
  inp.value = `${due.getFullYear()}-${String(due.getMonth() + 1).padStart(2, "0")}-${String(due.getDate()).padStart(2, "0")}`;
  if (old) old.replaceWith(inp); else body.append(inp);
  let done = false;
  const save = () => {
    if (done || !inp.value) return;
    done = true;
    const [y, m, d] = inp.value.split("-").map(Number);
    const when = new Date(y, m - 1, d);
    const cur = todoDue(t);
    if (t.hasTime && cur) when.setHours(cur.getHours(), cur.getMinutes(), 0, 0);
    rescheduleTodo(t, when, t.hasTime && !!cur);
  };
  inp.onkeydown = (ev) => {
    if (ev.key === "Enter") { ev.preventDefault(); save(); }
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); done = true; renderTodos(); }
  };
  inp.onchange = save;
  inp.onblur = () => { if (!done) renderTodos(); };
  inp.focus();
}

// ---------- settings tabs ----------
let lastTab = (() => { try { return localStorage.getItem("settingsTab"); } catch (_) { return null; } })();
function showTab(tab) {
  lastTab = tab;
  try { localStorage.setItem("settingsTab", tab); } catch (_) {}
  document.querySelectorAll("#tabs button").forEach((b) => b.classList.toggle("on", b.dataset.tab === tab));
  document.querySelectorAll(".settings-body [data-tab]").forEach((g) => g.classList.toggle("tab-off", g.dataset.tab !== tab));
  document.querySelector(".settings-body").scrollTop = 0;
  if (SETTINGS_WIN) {
    const b = document.querySelector(`#tabs button[data-tab="${tab}"] span`);
    const name = b ? b.textContent : "Settings";
    document.querySelector("#settings header.top .month-title").textContent = name;
    document.title = name;
  }
}

function slotLabel(s) {
  const a = new Date(s.start), b = new Date(s.end);
  return `${dayLabel(a)} ${a.toLocaleDateString(LOC(), { day: "numeric", month: "short" })} · ${fmtTime(a)}–${fmtTime(b)}`;
}

// ---------- AI box ----------
function setBusy(b) {
  busy = b;
  $("spark").classList.toggle("busy", b);
  $("quick").disabled = b;
}

let aiResult = null;
let draftClashes = [];

async function submitQuick(fromClipboard) {
  const text = $("quick").value.trim();
  if (busy) return;
  if (!text && !attachment && fromClipboard !== true) { if (drafts.length) addDrafts(); else if (aiResult) applyAI(); return; }
  setBusy(true);
  try {
    const r = fromClipboard === true ? await go("FromClipboard") : attachment ? await go("AskWith", text, attachment) : await go("Ask", text);
    $("quick").value = "";
    clearAttachment();
    if (r.kind === "add") {
      aiResult = null;
      drafts = r.drafts;
      draftClashes = r.clashes || [];
      if (r.offline && r.answer) toast(r.answer);
      if (!S.confirmBeforeAdd && !r.offline && !draftClashes.some((c) => c && c.length) && drafts.length === 1) { await addDrafts(); return; }
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
  const card = el("div", "ai-card ai-" + r.kind);
  const head = el("div", "ai-head");
  head.append(el("span", "ai-badge", { answer: "✦ Answer", update: "✦ Change", delete: "✦ Delete", slots: "✦ Free times", todo: "✦ To-do" }[r.kind] || "✦"));
  const x = el("button", "icon-btn small", "✕");
  x.title = "Dismiss";
  x.onclick = clearDrafts;
  head.append(x);
  card.append(head);
  if (r.answer) card.append(el("div", "ai-answer", r.answer));
  const actions = el("div", "draft-actions");
  const cancel = el("button", "btn", r.kind === "answer" || r.kind === "slots" ? "Close" : "Cancel");
  cancel.onclick = clearDrafts;

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
      <div class="will"><span class="muted small">After</span><b>${esc(d.title)}</b><div>${esc(whenText(d.start, d.end, d.allDay))}</div>${d.location ? `<div class="muted">📍 ${esc(d.location)}</div>` : ""}${d.repeatText ? `<div class="muted">🔁 ${esc(d.repeatText)}</div>` : ""}</div>`;
    card.append(cmp);
    if (t.recurring) card.append(el("div", "muted small", r.allFuture ? "Applies to this and all future events" : "Applies to this occurrence only"));
    const cl = (r.clashes || [])[0] || [];
    if (cl.length) card.append(el("div", "clash", `⚠️ Overlaps ${cl.map((c) => `“${c.title}”`).join(", ")}`));
  }
  if (r.kind === "delete" && r.target) {
    card.append(eventRow(withDates([r.target])[0], new Date()));
    if (r.target.recurring) card.append(el("div", "muted small", r.allFuture ? "Deletes this and all future events" : "Deletes this occurrence only"));
  }
  if (r.kind === "slots") {
    const list = el("div", "slots");
    (r.slots || []).forEach((s) => {
      const b = el("button", "slot");
      b.innerHTML = `<span>${esc(slotLabel(s))}</span><b>Book</b>`;
      b.onclick = async () => {
        try {
          const res = await go("Add", [s]);
          applyState(res.state); afterChange(); celebrate("add");
          clearDrafts();
          toastAction(`Booked “${s.title}” ${slotLabel(s)}`, "Undo", () => undoAdd(res.created, [false]));
        } catch (err) { toast(errMsg(err), true); }
      };
      list.append(b);
    });
    card.append(list);
  }
  if (r.kind === "todo") {
    const list = el("div", "todo-drafts");
    r.todos.forEach((t) => {
      const due = t.due ? new Date(t.due) : null;
      list.append(el("div", "todo-draft", `☐ ${t.title}${due ? "  ·  " + dayLabel(due) + " " + due.toLocaleDateString(LOC(), { day: "numeric", month: "short" }) + (t.hasTime ? " " + fmtTime(due) : "") : ""}`));
    });
    card.append(list);
    if (S.remindersAccess !== "full") card.append(el("div", "muted small", "Smart Cal will ask for access to Reminders."));
  }
  if (r.kind !== "answer" && r.kind !== "slots") {
    const ok = el("button", "btn " + (r.kind === "delete" ? "danger" : "primary"),
      r.kind === "delete" ? "Delete ↩" : r.kind === "todo" ? (r.todos.length > 1 ? `Add ${r.todos.length} to-dos ↩` : "Add to-do ↩") : "Apply ↩");
    ok.onclick = applyAI;
    actions.append(cancel, ok);
  } else actions.append(cancel);
  card.append(actions);
  box.append(card);
}

async function applyAI() {
  const r = aiResult;
  if (!r || busy || r.kind === "answer" || r.kind === "slots") return;
  setBusy(true);
  try {
    if (r.kind === "update") {
      let det = r.target;
      try { det = await go("GetEvent", r.target.id, r.target.start); } catch (_) {}
      const prev = draftFromDetail(det);
      r.draft.notes = r.draft.notes || prev.notes;
      // Undo puts the old repeat back too when this edit changed it
      if (r.draft.setRepeat) {
        prev.setRepeat = true;
        if (det.repeat && det.repeat.freq) prev.repeat = det.repeat;
      }
      applyState(await go("UpdateEvent", r.target.id, r.target.start, r.draft, !!r.allFuture));
      toastAction(`Updated “${r.draft.title}”`, "Undo", async () => {
        try { applyState(await go("UpdateEvent", r.target.id, r.draft.start, prev, !!r.allFuture)); afterChange(); toast("Undone"); } catch (err) { toast(errMsg(err), true); }
      });
    }
    if (r.kind === "delete") {
      let full = r.target;
      try { full = await go("GetEvent", r.target.id, r.target.start); } catch (_) {}
      applyState(await go("Delete", r.target.id, r.target.start, !!r.allFuture));
      if (r.allFuture) toast(`Deleted “${r.target.title}” and future ones`);
      else toastAction(`Deleted “${r.target.title}”`, "Undo", async () => {
        try { const a = await go("Add", [draftFromDetail(full)]); applyState(a.state); afterChange(); toast("Restored"); } catch (err) { toast(errMsg(err), true); }
      });
    }
    if (r.kind === "todo") {
      if (S.remindersAccess !== "full") applyState(await go("AllowReminders"));
      const ids = await go("AddTodos", r.todos);
      sound("pop");
      toastAction(ids.length > 1 ? `Added ${ids.length} to-dos` : "To-do added", "Undo", async () => {
        for (const id of ids) await go("DeleteTodo", id);
        go("Refresh").then(applyState);
      });
      go("Refresh").then(applyState);
    }
    aiResult = null;
    renderAI();
    afterChange();
  } catch (err) { toast(errMsg(err), true); }
  finally { setBusy(false); $("quick").focus(); }
}

function renderDrafts() {
  const box = $("drafts");
  box.innerHTML = "";
  if (!SETTINGS_WIN) {
    const sig = drafts.map((x) => x.start).join("|");
    if (sig !== renderDrafts.sig) {
      renderDrafts.sig = sig;
      renderMiniCal();
      if (listMode === "day") renderList();
    }
  }
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
    if (d.repeatText || repeats(d.repeat)) meta.push("🔁 " + (d.repeatText || repeatText(d.repeat)));
    if (d.location) meta.push("📍 " + d.location);
    const tz = tzNote(d);
    if (tz) meta.push("🌍 " + tz);
    if (d.reminderMinutes > 0) meta.push(`🔔 ${dur(d.reminderMinutes * 60000)} before`);
    if (d.notes) meta.push("📝 " + d.notes);
    if (meta.length) card.append(el("div", "draft-meta", meta.join("   ")));
    const cl = draftClashes[i] || [];
    if (cl.length) card.append(el("div", "clash", `⚠️ Overlaps ${cl.map((c) => `“${c.title}” ${fmtTime(new Date(c.start))}`).join(", ")}`));
    if (writable.length > 1) {
      const row = el("div", "draft-cal");
      const sw = el("span");
      sw.style.cssText = "width:8px;height:8px;border-radius:50%;display:inline-block";
      const sel = el("select");
      writable.forEach((c) => { const o = el("option", "", c.name); o.value = c.id; if (c.id === d.calendarId) o.selected = true; sel.append(o); });
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
  actions.append(el("span", "draft-hint", "Not added yet"), cancel, add);
  box.append(actions);
}

// events typed into the AI box but not added yet (Confirm before adding is on)
function pendingBox(list) {
  const box = el("div", "pending-box"), rows = el("div", "pending-list");
  for (const x of list) {
    const row = el("div", "pending-row");
    const when = x.allDay ? "All day" : `${fmtTime(new Date(x.start))} – ${fmtTime(new Date(x.end))}`;
    const txt = el("div", "pending-text");
    txt.append(el("div", "pending-title", x.title), el("div", "muted small", `${when} · not added yet`));
    row.append(txt);
    rows.append(row);
  }
  box.append(rows);
  const add = el("button", "btn primary small-btn", list.length > 1 ? `Add ${list.length}` : "Add");
  add.onclick = addDrafts;
  box.append(add);
  return box;
}

// move the waiting event(s) to another day, keeping their times and length
async function moveDraftsTo(day) {
  const first = new Date(drafts[0].start);
  const shift = Math.round((startOfDay(day) - startOfDay(first)) / 864e5);
  if (!shift) return;
  drafts = drafts.map((x) => {
    const s = new Date(x.start), e = new Date(x.end);
    s.setDate(s.getDate() + shift); e.setDate(e.getDate() + shift); // calendar days, so DST can't shift the time
    return { ...x, start: s.toISOString(), end: e.toISOString() };
  });
  try { draftClashes = await Promise.all(drafts.map((x) => go("CheckClashes", x, ""))); } catch (_) { draftClashes = []; }
  renderDrafts();
  toast(`Moved to ${day.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" })} — press Add to save it`);
}

function clearDrafts() { drafts = []; draftClashes = []; aiResult = null; renderDrafts(); $("drafts").innerHTML = ""; $("quick").focus(); }

async function addDrafts() {
  if (busy || !drafts.length) return;
  setBusy(true);
  const list = drafts, n = list.length, first = list[0];
  try {
    const r = await go("Add", list);
    drafts = [];
    draftClashes = [];
    renderDrafts();
    applyState(r.state);
    afterChange();
    celebrate("add");
    toastAction(n === 1 ? `Added “${first.title}”` : `Added ${n} events`, "Undo", () => undoAdd(r.created, list.map((d) => repeats(d.repeat))));
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
  const btn = $("bannerAllow");
  if (btn) { btn.disabled = true; btn.textContent = "Waiting for macOS…"; }
  toast("Look for the macOS prompt and click Allow");
  try {
    applyState(await go("AllowAccess"));
    if (S.connected) toast(S.hasGoogle ? "Connected — your calendars are in" : "Allowed — now add your Google account");
  } catch (err) { toast(errMsg(err), true); renderBanner(); }
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
  const raw = (pendingAccent || (settingsCache && settingsCache.accent) || S.accent || "#4169E1");
  const cur = raw.toUpperCase();
  let named = false;
  const sys = el("button", "swatch swatch-system" + (raw === "system" ? " on" : ""), "A");
  sys.style.setProperty("--sys", (settingsCache && settingsCache.systemAccent) || "#007AFF");
  sys.title = "System (follows your Mac's accent colour)";
  sys.setAttribute("aria-label", "System accent colour");
  sys.onclick = () => { pendingAccent = "system"; applyAccent((settingsCache && settingsCache.systemAccent) || "#007AFF"); renderSwatches(); };
  box.append(sys);
  if (raw === "system") { named = true; $("accentName").textContent = tr("System") + " (macOS)"; }
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
  if (raw === "system") pick.value = ((settingsCache && settingsCache.systemAccent) || "#007AFF").toLowerCase();
}

// ---------- updates ----------
let U = null;
function renderUpdate(info, statusMsg) {
  if (info) U = info;
  const cur = (U && U.current) || "";
  $("versionLbl").textContent = cur ? `v${cur}` : "";
  $("updVersion").textContent = cur ? `Smart Cal v${cur}` : "Smart Cal";
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
      .forEach((l) => { const li = el("li"); li.innerHTML = esc(l).replace(/\*\*(.+?)\*\*/g, "<b>$1</b>"); ul.append(li); });
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
  showUpdateStep({ stage: "checking", text: "Getting ready…", pct: -1 });
  try { await go("InstallUpdate"); }
  catch (err) {
    showUpdateStep({ stage: "error", text: errMsg(err), pct: -1 });
    $("installUpdBtn").disabled = false;
    $("updatePill").disabled = false;
  }
}

// The update progress card: a bar (download %), the steps, and a clear message if it fails.
const UPD_ORDER = ["checking", "downloading", "verifying", "installing", "restarting"];
let updStuckTimer;
function showUpdateStep(st) {
  if (typeof st === "string") st = { stage: "", text: st, pct: -1 };
  const box = $("updOverlay");
  box.classList.remove("hidden");
  const failed = st.stage === "error";
  box.classList.toggle("failed", failed);
  $("updTitle").textContent = failed ? "The update didn't finish" : st.stage === "restarting" ? "Almost done" : "Updating Smart Cal";
  $("updStep").textContent = st.pct >= 0 && st.stage === "downloading" ? `${st.text} ${st.pct}%` : st.text;
  // the bar: download fills 0–80%, then the quick steps
  const at = { checking: 3, verifying: 84, installing: 92, restarting: 100 };
  const fill = st.stage === "downloading" ? (st.pct >= 0 ? Math.round(st.pct * 0.8) : -1) : at[st.stage] ?? -1;
  $("updBar").classList.toggle("busy", fill < 0 && !failed);
  $("updFill").style.width = fill < 0 ? "" : `${Math.max(3, fill)}%`;
  const cur = UPD_ORDER.indexOf(st.stage);
  $("updSteps").querySelectorAll("li").forEach((li) => {
    const i = UPD_ORDER.indexOf(li.dataset.s);
    li.className = failed ? "" : i < cur ? "done" : i === cur ? "now" : "";
  });
  $("updHint").textContent = failed ? "Nothing was changed — your current version keeps working. You can try again from Settings → About."
    : "Smart Cal will close and reopen by itself — nothing else to do.";
  $("updClose").classList.toggle("hidden", !failed);
  clearTimeout(updStuckTimer);
  if (st.stage === "restarting") {
    // the new version normally opens within seconds; if not, say what to do
    updStuckTimer = setTimeout(() => { $("updHint").textContent = "Taking longer than usual — if Smart Cal doesn't reopen, open it from your Applications folder."; }, 20000);
  }
  if (failed) { renderUpdate(null, st.text); $("updClose").focus(); }
}

// ---------- week settings ----------
const REGIONS = { kw: [6, [5, 6]], sa: [0, [5, 6]], ae: [1, [6, 0]], eg: [6, [5, 6]], eu: [1, [6, 0]], us: [0, [6, 0]] };
function renderWeekendChips(days) {
  const box = $("sWeekend");
  box.innerHTML = "";
  const set = new Set(days);
  ["S", "M", "T", "W", "T", "F", "S"].forEach((l, d) => {
    const b = el("button", "day-chip" + (set.has(d) ? " on" : ""), l);
    b.dataset.d = d;
    b.title = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][d];
    b.onclick = () => { b.classList.toggle("on"); $("sRegion").value = matchRegion(); };
    box.append(b);
  });
}
function matchRegion() {
  const ws = +$("sWeekStart").value;
  const days = [...document.querySelectorAll("#sWeekend .on")].map((b) => +b.dataset.d).sort().join();
  for (const [k, [w, d]] of Object.entries(REGIONS)) if (w === ws && [...d].sort().join() === days) return k;
  return "custom";
}

// ---------- settings ----------
// Settings live in their own window (index.html?settings=1). From the panel, ask Go to open it.
const SETTINGS_WIN = new URLSearchParams(location.search).has("settings");
function openSettings(tab) {
  if (SETTINGS_WIN) return loadSettings(tab);
  return go("OpenSettings", typeof tab === "string" ? tab : "").catch(() => {});
}

let loadingSettings = false;
async function loadSettings(tab) {
  loadingSettings = true;
  $("main").classList.add("hidden");
  $("settings").classList.remove("hidden");
  settingsCache = await go("GetSettings");
  $("sKey").value = "";
  $("sModel").value = settingsCache.openaiModel;
  $("sClock").value = settingsCache.clockMode || "system";
  $("sLang").value = settingsCache.language || "auto";
  $("sDock").checked = !!settingsCache.showInDock;
  $("sConfirm").checked = settingsCache.confirmBeforeAdd;
  $("sDays").value = String(settingsCache.daysAhead);
  loadMenuBarSettings(settingsCache);
  $("sAlert").value = settingsCache.alertMode || "1";
  $("sLogin").checked = !!settingsCache.launchAtLogin;
  $("sHolRows").checked = !!settingsCache.showHolidayRows;
  $("sRegion").value = settingsCache.region || "kw";
  $("sWeekStart").value = String(settingsCache.weekStart ?? 6);
  renderWeekendChips(settingsCache.weekendDays || [5, 6]);
  $("sPlayful").checked = settingsCache.playful !== false;
  $("sName").value = settingsCache.name || "";
  $("sVoice").value = settingsCache.voice || "friendly";
  $("sWeather").checked = settingsCache.weather !== false;
  $("sCity").value = settingsCache.weatherCity || "";
  $("sConfetti").checked = settingsCache.confetti !== false;
  $("sSounds").checked = !!settingsCache.sounds;
  $("playfulOpts").classList.toggle("off", !$("sPlayful").checked);
  $("sDefAlarm").value = settingsCache.defaultAlarm || "15";
  $("sAllDayAlarm").value = settingsCache.allDayAlarm || "dayBefore9";
  $("sTravel").value = settingsCache.travelFrom || "";
  $("sTodos").checked = settingsCache.showTodos !== false;
  $("sHotkey").value = settingsCache.hotkey || "Ctrl+Alt+C";
  $("sTextSize").value = String(settingsCache.textSize || 100);
  const tl = $("sTodoList"); tl.innerHTML = "";
  const dflt = el("option", "", "Default list"); dflt.value = ""; tl.append(dflt);
  (settingsCache.todoLists || []).forEach((l) => { const o = el("option", "", l.name); o.value = l.id; tl.append(o); });
  tl.value = settingsCache.todoList || "";
  renderRemStatus();
  const t = (typeof tab === "string" && tab) || openTab || lastTab || "calendar";
  showTab(t.replace("-check", ""));
  openTab = null;
  pendingAccent = null;
  showSetupDetails = false;
  editingKey = false;
  renderSwatches();
  renderGoogleBox();
  document.querySelector(".settings-body").scrollTop = 0;
  $("aboutVer").textContent = "Version " + ((await go("Version").catch(() => "")) || "");
  loadingSettings = false;
  if (tab === "ai") focusAI();
  if (tab === "about-check") checkUpdate();
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
  $("aiGroup").classList.toggle("attention", !S.hasAI);
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

let openTab = null;
function renderRemStatus() {
  const a = S.remindersAccess;
  $("remStatus").textContent = a === "full" ? "Reminders connected ✓" : a === "notDetermined" ? "Not connected yet" : "Access is off";
  $("allowRem").classList.toggle("hidden", a === "full");
}
function focusAI() {
  showTab("ai");
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
  if (SETTINGS_WIN) { go("CloseSettings"); return; }
  if (pendingAccent) { pendingAccent = null; applyAccent(S.accent); }
  $("settings").classList.add("hidden");
  $("main").classList.remove("hidden");
  $("quick").focus();
}

// Settings save as you change them (Mac style) — no Save button.
let autoSaveTimer = null;
function autoSave() {
  if (loadingSettings || !settingsCache) return;
  clearTimeout(autoSaveTimer);
  autoSaveTimer = setTimeout(() => saveSettings(true), 350);
}
async function saveSettings(silent) {
  const hidden = [...document.querySelectorAll("#calList input[type=checkbox]")].filter((c) => !c.checked).map((c) => c.dataset.cal);
  const def = (document.querySelector("#calList input[type=radio]:checked") || {}).value || "";
  const payload = {
    openaiKey: silent ? "" : $("sKey").value.trim(), // the key saves only on Enter/paste, never mid-typing
    openaiModel: $("sModel").value.trim(),
    clockMode: $("sClock").value,
    language: $("sLang").value,
    showInDock: $("sDock").checked,
    confirmBeforeAdd: $("sConfirm").checked,
    daysAhead: parseInt($("sDays").value, 10),
    hiddenCalendars: hidden,
    defaultCalendar: def,
    accent: pendingAccent || settingsCache.accent || S.accent || "#4169E1",
    ...menuBarValues(),
    alertMode: $("sAlert").value,
    launchAtLogin: $("sLogin").checked,
    showHolidayRows: $("sHolRows").checked,
    region: $("sRegion").value,
    weekStart: parseInt($("sWeekStart").value, 10),
    weekendDays: [...document.querySelectorAll("#sWeekend .on")].map((b) => +b.dataset.d),
    playful: $("sPlayful").checked,
    name: $("sName").value.trim(),
    voice: $("sVoice").value,
    weather: $("sWeather").checked,
    weatherCity: $("sCity").value.trim(),
    confetti: $("sConfetti").checked,
    sounds: $("sSounds").checked,
    defaultAlarm: $("sDefAlarm").value,
    allDayAlarm: $("sAllDayAlarm").value,
    travelFrom: $("sTravel").value.trim(),
    showTodos: $("sTodos").checked,
    todoList: $("sTodoList").value,
    hotkey: $("sHotkey").value,
    textSize: parseInt($("sTextSize").value, 10),
  };
  try {
    pendingAccent = null;
    applyState(await go("SaveSettings", payload));
    settingsCache = await go("GetSettings");
    renderSwatches();
    const keyJustSaved = !!payload.openaiKey;
    if (keyJustSaved) $("sKey").value = "";
    if (keyJustSaved) editingKey = false;
    arrangeSettings();
    if (keyJustSaved) { const k = $("keySaved"); k.classList.remove("just"); void k.offsetWidth; k.classList.add("just"); }
    if (keyJustSaved) toast("Key saved on this Mac ✓ — quick add is ready");
    else if (!silent) toast("Saved ✓");
    else flashSaved();
  } catch (err) { toast(errMsg(err), true); }
}

// ---------- wiring ----------
function wire() {
  $("quickForm").onsubmit = (ev) => { ev.preventDefault(); submitQuick(); };
  // 2-line box: Return adds, Shift+Return makes a new line
  $("quick").addEventListener("keydown", (ev) => {
    if (ev.key === "Enter" && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); submitQuick(); }
  });
  $("prevMonth").onclick = () => stepPeriod(-1);
  $("nextMonth").onclick = () => stepPeriod(1);
  $("todayBtn").onclick = goToday;
  document.querySelectorAll("#views button").forEach((b) => { b.onclick = () => setView(b.dataset.view); });
  $("searchBtn").onclick = () => ($("searchBar").classList.contains("hidden") ? openSearch() : closeSearch());
  $("searchInput").addEventListener("input", () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => runSearch($("searchInput").value), 220); });
  $("searchInput").addEventListener("keydown", (ev) => {
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); closeSearch(); $("quick").focus(); }
    if (ev.key === "ArrowDown") { ev.preventDefault(); $("searchInput").blur(); kbMove(1); }
    if (ev.key === "Enter") { ev.preventDefault(); $("searchInput").blur(); kbIndex = -1; kbMove(1); kbOpen(); }
  });
  $("searchClose").onclick = closeSearch;
  $("openSettings").onclick = () => openSettings();
  $("closeSettings").onclick = () => (SETTINGS_WIN ? go("CloseSettings") : closeSettings());
  $("returnBtn").onclick = () => (SETTINGS_WIN ? go("CloseSettings") : closeSettings());
  // instant save: any change inside Settings
  const sb = document.querySelector(".settings-body");
  sb.addEventListener("change", (ev) => { if (ev.target.id !== "sKey") autoSave(); });
  sb.addEventListener("click", (ev) => { if (ev.target.closest(".mb-opt, .seg button, .swatch, #sWeekend button")) autoSave(); });
  $("aboutBtn").onclick = () => go("ShowAbout");
  $("helpBtn").onclick = () => (SETTINGS_WIN ? showHelp() : showHelp());
  $("quitBtn2").onclick = () => go("Quit");
  document.querySelectorAll(".about-group [data-open]").forEach((a) => { a.onclick = (ev) => { ev.preventDefault(); go("OpenURL", a.dataset.open); }; });
  document.querySelectorAll("#tabs button").forEach((b) => { b.onclick = () => showTab(b.dataset.tab); });
  $("allowRem").onclick = async () => { try { applyState(await go("AllowReminders")); renderRemStatus(); } catch (err) { toast(errMsg(err), true); } };
  $("clipBtn").onclick = () => submitQuick(true);
  $("attachX").onclick = clearAttachment;
  $("quick").addEventListener("paste", (ev) => {
    const items = [...((ev.clipboardData && ev.clipboardData.items) || [])];
    const img = items.find((it) => it.type && it.type.startsWith("image/"));
    if (img) { ev.preventDefault(); attachImage(img.getAsFile()); }
  });
  const shell = document.querySelector(".shell");
  shell.addEventListener("dragover", (ev) => { ev.preventDefault(); shell.classList.add("dropping"); });
  shell.addEventListener("dragleave", (ev) => { if (ev.target === shell) shell.classList.remove("dropping"); });
  shell.addEventListener("drop", (ev) => {
    ev.preventDefault();
    shell.classList.remove("dropping");
    const f = [...(ev.dataTransfer.files || [])].find((x) => x.type.startsWith("image/"));
    if (f) { attachImage(f); return; }
    const t = ev.dataTransfer.getData("text/plain");
    if (t) { $("quick").value = t; $("quick").focus(); }
  });
  Events.On("ui:textsize", () => go("GetState").then(applyState));
  $("sRegion").onchange = () => {
    const r = REGIONS[$("sRegion").value];
    if (r) { $("sWeekStart").value = String(r[0]); renderWeekendChips(r[1]); }
  };
  $("sWeekStart").onchange = () => { $("sRegion").value = matchRegion(); };
  $("sPlayful").onchange = () => $("playfulOpts").classList.toggle("off", !$("sPlayful").checked);
  $("changeKey").onclick = () => { editingKey = true; arrangeSettings(); $("sKey").focus(); };
  $("showSetup").onclick = (ev) => { ev.preventDefault(); showSetupDetails = !showSetupDetails; arrangeSettings(); $("showSetup").textContent = showSetupDetails ? "hide" : "details"; };
  $("sKey").addEventListener("keydown", (ev) => { if (ev.key === "Enter") { ev.preventDefault(); saveSettings(); } });
  // paste the key → it's saved immediately; no need to press Save
  $("sKey").addEventListener("input", () => {
    const v = $("sKey").value.trim();
    if (/^sk-[A-Za-z0-9_-]{20,}$/.test(v)) saveSettings();
  });
  $("saveSettings").onclick = () => saveSettings();
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
      if (attachment) { clearAttachment(); return; }
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
      if (!typing && ev.key === "ArrowLeft" && !ev.metaKey) { ev.preventDefault(); calView === "month" ? openDay(addDays(selectedDay || new Date(), -1)) : stepPeriod(-1); }
      if (!typing && ev.key === "ArrowRight" && !ev.metaKey) { ev.preventDefault(); calView === "month" ? openDay(addDays(selectedDay || new Date(), 1)) : stepPeriod(1); }
    }
    if (ev.metaKey && ev.key.toLowerCase() === "f") { ev.preventDefault(); openSearch(); }
    if (ev.metaKey && ev.shiftKey && ev.key.toLowerCase() === "w") { ev.preventDefault(); go("OpenWeek"); }
    if (ev.metaKey && ev.key === ",") { ev.preventDefault(); openSettings(); }
    if (ev.metaKey && ev.key.toLowerCase() === "r") { ev.preventDefault(); refresh(); }
    if (ev.metaKey && ev.key.toLowerCase() === "n" && !SETTINGS_WIN) { ev.preventDefault(); newEventNow(); }
    if (ev.metaKey && !ev.shiftKey && ev.key.toLowerCase() === "w") { ev.preventDefault(); SETTINGS_WIN ? go("CloseSettings") : go("Hide"); }
    if (ev.metaKey && !ev.shiftKey && ev.key.toLowerCase() === "z" && !typing && !SETTINGS_WIN) { ev.preventDefault(); undoLast(); }
    if (ev.metaKey && ev.shiftKey && ev.key.toLowerCase() === "z" && !typing && !SETTINGS_WIN) { ev.preventDefault(); redoLast(); }
    if (ev.metaKey && ["1", "2", "3", "4"].includes(ev.key) && !SETTINGS_WIN) { ev.preventDefault(); setView(["year", "month", "week", "day"][+ev.key - 1]); }
    if ((ev.metaKey && ev.key === "?") || (ev.key === "?" && !typing)) { ev.preventDefault(); showHelp(); }
    if (ev.metaKey && ev.key.toLowerCase() === "t") { ev.preventDefault(); $("todayBtn").click(); }
  });

  Events.On("state", (ev) => applyState(ev.data));
  Events.On("update", (ev) => renderUpdate(ev.data));
  Events.On("update:progress", (ev) => showUpdateStep(ev.data));
  $("updClose").onclick = () => $("updOverlay").classList.add("hidden");
  Events.On("ui:updates", () => openSettings("about-check"));
  Events.On("settings:open", (ev) => { if (SETTINGS_WIN) loadSettings(ev.data || ""); });
  Events.On("ui:new", () => { if (!SETTINGS_WIN) newEventNow(); });
  Events.On("ui:help", () => { if (!SETTINGS_WIN) showHelp(); });
  Events.On("ui:view", (ev) => { if (!SETTINGS_WIN && ["year", "month", "week", "day"].includes(ev.data)) setView(ev.data); });
  Events.On("ui:today", () => { if (!SETTINGS_WIN) $("todayBtn").click(); });
  $("checkUpdBtn").onclick = checkUpdate;
  $("installUpdBtn").onclick = installUpdate;
  $("updatePill").onclick = () => openSettings("about");
  Events.On("ui:settings", () => openSettings());
  Events.On("alert", (ev) => {
    const a = ev.data;
    toast(`⏰ ${a.title} — ${a.body}`);
  });
  $("scroll").addEventListener("contextmenu", (ev) => {
    let n = ev.target;
    while (n && n !== ev.currentTarget && !n._ev) n = n.parentElement;
    if (!n || !n._ev) return;
    ev.preventDefault();
    eventMenu(n._ev, n, { x: ev.clientX, y: ev.clientY });
  });
  Events.On("popover:shown", () => {
    if (SETTINGS_WIN) return;
    renderToday();
    lastHourScroll = "";
    const now = new Date();
    viewMonth = startOfMonth(now); viewYear = now.getFullYear(); weekAnchor = weekStartFor(now);
    selectedDay = null;
    if (listMode === "day") { listMode = "upcoming"; dayView = null; }
    if (calView === "day") { selectedDay = now; openDay(now, true); }
    else if (calView === "week") { listMode = "week"; loadWeekList(); }
    renderMiniCal();
    renderList();
    if ($("settings").classList.contains("hidden")) setTimeout(() => $("quick").focus(), 30);
    go("Refresh").then(applyState).catch(() => {});
    refreshGreeting();
  });
  $("mascot").onclick = mascotPoke;
  $("recapBtn").onclick = () => { if (listMode !== "upcoming") closeDay(); const l = $("list"); l.querySelectorAll(".recap").forEach((x) => x.remove()); renderRecap(l, true); $("scroll").scrollTo({ top: 0, behavior: "smooth" }); };
  window.addEventListener("focus", () => { if ($("settings").classList.contains("hidden")) $("quick").focus(); });

  // keep countdowns fresh while open
  setInterval(() => { renderUpNext(); }, 15000);
  setInterval(() => { renderMascot(); checkDayClear(); }, 60000);
  setInterval(refreshGreeting, 30 * 60000);
}

async function refresh() {
  try { applyState(await go("Refresh")); } catch (err) { toast(errMsg(err), true); }
}

if (SETTINGS_WIN) {
  document.body.classList.add("settings-win");
  $("closeSettings").classList.add("hidden"); // a Mac Settings window closes with its red button or ⌘W
}
renderToday();
wire();
renderMiniCal();
if (SETTINGS_WIN) loadSettings("");
go("GetState").then(applyState).catch((err) => toast(errMsg(err), true));
go("LatestUpdate").then((u) => renderUpdate(u)).catch(() => {});
setTimeout(refreshGreeting, 400);
setTimeout(() => { if (calView !== "month") setView(calView); }, 300);

// send the panel's emoji for upcoming events to the menu bar label ("👥 2h Meet")
let lastLabelEmojis = "";
function pushLabelEmojis() {
  if (SETTINGS_WIN) return;
  const m = {};
  for (const e of evs().slice(0, 12)) if (!e.allDay && !e.holiday) m[e.title] = emojiFor(e) || (flightOf(e.title, e.location) ? "✈️" : "");
  const j = JSON.stringify(m);
  if (j === lastLabelEmojis) return;
  lastLabelEmojis = j;
  go("SetLabelEmojis", m).catch(() => {});
}
setInterval(pushLabelEmojis, 20000);
setTimeout(pushLabelEmojis, 1500);

// ======================= Menu bar settings =======================
const MB_STYLES = [
  ["compact", "Compact", "emoji · countdown · word"], ["minimal", "Minimal", "emoji · countdown"],
  ["countdown", "Countdown + title", ""], ["time", "Time + title", ""], ["full", "Full", "the original style"],
  ["today", "Today only", "nothing for tomorrow+"], ["lasthour", "Last hour only", "icon until 60 min before"],
  ["date", "Date", ""], ["datecompact", "Date + compact", ""], ["icon", "Icon only", "dot before meetings"],
];
let mb = { menuBarStyle: "compact", barTitle: "word", barAhead: "any", barTime: "countdown", barEmoji: true, barHideInMeeting: false, barDot: "5" };

function loadMenuBarSettings(c) {
  const legacy = { emoji: "compact", next: "full", soon: "lasthour" };
  mb = { menuBarStyle: legacy[c.menuBarStyle] || c.menuBarStyle || "compact", barTitle: c.barTitle || "word", barAhead: c.barAhead || "any",
    barTime: c.barTime || "countdown", barEmoji: c.barEmoji !== false, barHideInMeeting: !!c.barHideInMeeting, barDot: c.barDot || "5" };
  renderMenuBarSettings();
}
function menuBarValues() { return { ...mb }; }

const mbClock = (d) => {
  if (S.use24h) return d.toLocaleTimeString(LOC(), { hour: "2-digit", minute: "2-digit" });
  let h = d.getHours(); const m = d.getMinutes(), ap = h >= 12 ? "p" : "a";
  h = h % 12 || 12;
  return m ? `${h}:${String(m).padStart(2, "0")}${ap}` : `${h}${ap}`;
};
function mbShortWord(t) {
  const skip = new Set(["the", "a", "an", "my", "with", "to", "at", "and", "&", "-", "·"]);
  const ws = t.split(/\s+/);
  for (let i = 0; i < ws.length; i++) {
    const w = ws[i].replace(/^[.,:;!?()[\]"']+|[.,:;!?()[\]"']+$/g, "");
    if (!w || skip.has(w.toLowerCase())) continue;
    if (w.length === 2 && w === w.toUpperCase() && /^\d{1,4}$/.test(ws[i + 1] || "")) return w + ws[i + 1];
    const r = [...w];
    if (r[0].codePointAt(0) > 0x2000 && r.length <= 2) continue;
    return r.length > 8 ? r.slice(0, 7).join("") + "…" : w;
  }
  return "";
}
const mbTrunc = (s, n) => ([...s].length > n ? [...s].slice(0, n - 1).join("") + "…" : s);

// mirrors App.barLabel in main.go
function barLabel(c, e, now) {
  const dateL = now.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" }).replace(",", "");
  if (c.menuBarStyle === "icon") return "";
  if (c.menuBarStyle === "date") return dateL;
  if (c.menuBarStyle === "full") {
    if (!e) return dateL;
    const t = mbTrunc(e.title, 22), d = e.s - now;
    if (d <= 0) return c.barHideInMeeting ? "" : `● ${t} · ${dur(e.e - now)} left`;
    if (d <= 36e5) return `${t} in ${dur(d)}`;
    if (sameDay(e.s, now)) return `${fmtTime(e.s)} · ${t}`;
    const day = sameDay(e.s, addDays(now, 1)) ? "Tomorrow" : e.s.toLocaleDateString(LOC(), { weekday: "short" });
    return `${day} ${fmtTime(e.s)} · ${t}`;
  }
  let part = "";
  if (e) {
    let ahead = c.barAhead;
    if (c.menuBarStyle === "today") ahead = "today";
    if (c.menuBarStyle === "lasthour") ahead = "hour";
    const d = e.s - now;
    const ok = ahead === "hour" ? d <= 36e5 : ahead === "today" ? (d <= 0 || sameDay(e.s, now)) : true;
    if (ok && !(c.barHideInMeeting && d <= 0)) {
      let title = c.barTitle, mode = c.barTime, emoji = c.barEmoji;
      if (c.menuBarStyle === "minimal") title = "none";
      if (c.menuBarStyle === "countdown") { emoji = false; mode = "countdown"; if (title === "word") title = "full"; }
      if (c.menuBarStyle === "time") { emoji = false; mode = "clock"; if (title === "word") title = "full"; }
      const p = [];
      if (emoji) p.push(emojiFor(e) || (flightOf(e.title, e.location) ? "✈️" : "") || "📅");
      let when;
      if (d <= 0) when = "now";
      else if (!sameDay(e.s, now) && d >= 12 * 36e5) when = e.s.toLocaleDateString(LOC(), { weekday: "short" }).slice(0, 2) + " " + mbClock(e.s);
      else if (mode === "clock") when = mbClock(e.s);
      else if (d < 36e5) when = Math.round(d / 6e4) + "m";
      else if (d < 3 * 36e5 && Math.floor(d / 6e4) % 60 >= 30) when = Math.floor(d / 36e5) + "h½";
      else when = Math.floor(d / 36e5) + "h";
      p.push(when);
      if (title === "word") { const w = mbShortWord(e.title); if (w) p.push(w); }
      else if (title === "short") p.push(mbTrunc(e.title, 15));
      else if (title === "full") p.push(mbTrunc(e.title, 30));
      part = p.join(" ");
    }
  }
  if (c.menuBarStyle === "datecompact") return part ? now.toLocaleDateString(LOC(), { weekday: "short", day: "numeric" }).replace(",", "") + " · " + part : dateL;
  return part;
}

function mbBar(label, dark) {
  const b = el("div", "mb-bar" + (dark ? " dark" : ""));
  if (!label) { const i = el("img"); i.src = "icon.png"; b.append(i); } else b.append(label);
  return b;
}

function renderMenuBarSettings() {
  if (!$("mbOpts")) return;
  const now = new Date();
  const real = evs().find((e) => !e.allDay && !e.holiday && e.e > now);
  const title = real ? real.title : "Meet friends", loc = real ? real.location : "";
  const sample = (ms, len = 36e5) => { const s = new Date(+now + ms); return { title, location: loc, s, e: new Date(+s + len), allDay: false }; };
  const tomorrow10 = (() => { const t = addDays(startOfDay(now), 1); t.setHours(10); return { title, location: loc, s: t, e: new Date(+t + 36e5) }; })();
  const states = [["Tomorrow", tomorrow10], ["In 2 hours", sample(2 * 36e5)], ["In 25 min", sample(25 * 6e4)], ["Started", sample(-10 * 6e4)]];
  const name = (MB_STYLES.find((x) => x[0] === mb.menuBarStyle) || MB_STYLES[0])[1];
  $("mbCap").textContent = `Live preview · ${name}${real ? "" : " · sample event"}`;
  const st = $("mbStates"); st.innerHTML = "";
  for (const [k, ev] of states) st.append(el("span", "k", k), mbBar(barLabel(mb, ev, now), k === "Started"));
  const opts = $("mbOpts"); opts.innerHTML = "";
  for (const [id, label, sub] of MB_STYLES) {
    const o = el("button", "mb-opt" + (mb.menuBarStyle === id ? " sel" : ""));
    const nm = el("span", "mb-name", label);
    if (sub) nm.append(el("small", "", sub));
    o.append(el("span", "mb-radio"), nm, mbBar(barLabel({ ...mb, menuBarStyle: id }, states[1][1], now)));
    o.onclick = () => { mb.menuBarStyle = id; renderMenuBarSettings(); };
    opts.append(o);
  }
  document.querySelectorAll(".seg[data-k]").forEach((g) => {
    g.querySelectorAll("button").forEach((b) => {
      b.classList.toggle("on", mb[g.dataset.k] === b.dataset.v);
      b.onclick = () => { mb[g.dataset.k] = b.dataset.v; renderMenuBarSettings(); };
    });
  });
  $("mbEmoji").checked = mb.barEmoji; $("mbEmoji").onchange = () => { mb.barEmoji = $("mbEmoji").checked; renderMenuBarSettings(); };
  $("mbHide").checked = mb.barHideInMeeting; $("mbHide").onchange = () => { mb.barHideInMeeting = $("mbHide").checked; renderMenuBarSettings(); };
  $("mbDot").value = mb.barDot; $("mbDot").onchange = () => { mb.barDot = $("mbDot").value; };
}

// ======================= Mac conventions =======================
// ⌘N: new event at the next full hour
function newEventNow() {
  const s = new Date(); s.setMinutes(0, 0, 0); s.setHours(s.getHours() + 1);
  const def = S.defaultCalendar || ((S.calendars || []).find((x) => x.primary) || {}).id || "";
  renderEdit({ title: "", start: s.toISOString(), end: new Date(+s + 36e5).toISOString(), allDay: false, location: "", notes: "", calendarId: def, color: "" }, "new");
}

// small "Saved" tick in the Settings title bar (settings save as you change them)
let savedTimer = null;
function flashSaved() {
  let tag = $("savedTag");
  if (!tag) { tag = el("span", "saved-tag", "✓ " + tr("Saved")); tag.id = "savedTag"; document.querySelector("#settings header.top").append(tag); }
  tag.classList.add("show");
  clearTimeout(savedTimer);
  savedTimer = setTimeout(() => tag.classList.remove("show"), 1400);
}

// Help & keyboard shortcuts (menu: Smart Cal Help, or ?)
const HELP_KEYS = [
  ["⌃⌥C", "Open or close Smart Cal (anywhere)"], ["⌘N", "New event"], ["⌘F  or  /", "Search"], ["⌘1 – ⌘4", "Year · Month · Week · Day view"],
  ["← →", "Previous / next day (or period)"], ["↑ ↓  ↩", "Move through events · open"], ["⌫", "Delete the selected event"],
  ["⌘Z  ·  ⌘⇧Z", "Undo · redo"], ["⌘T", "Back to today"], ["⌘R", "Refresh"], ["⌘⇧W", "Big week window"], ["⌘,", "Settings"],
  ["?", "This help"], ["Esc", "Back / close"], ["⌘Q", "Quit Smart Cal"],
];
function showHelp() {
  const sh = $("sheet");
  sh.innerHTML = "";
  sh.className = "sheet";
  const head = el("div", "sheet-head");
  const back = el("button", "pill-btn", "‹ " + tr("Back")); back.onclick = closeSheet;
  head.append(back);
  const body = el("div", "sheet-body");
  body.append(el("div", "sheet-title", tr("Keyboard shortcuts")));
  const tbl = el("div", "help-keys");
  for (const [k, d] of HELP_KEYS) { tbl.append(el("kbd", "", k), el("span", "", d)); }
  body.append(tbl);
  const tips = el("div", "help-tips");
  tips.innerHTML = `<b>Tips</b><ul>
    <li>Type naturally in the AI box: “dentist Tue 4pm”, “what's on Friday?”, “move lunch to 3”.</li>
    <li>Paste or drop a screenshot of a booking or invite to turn it into events.</li>
    <li>Click a flight's ✈︎ to follow it live on Flightradar24.</li>
    <li>Drag events in the Day view to move them; drag the bottom edge to resize.</li></ul>`;
  body.append(tips);
  const more = el("button", "btn", "Online guide · go.sala.company");
  more.onclick = () => go("OpenURL", "https://go.sala.company");
  body.append(more);
  sh.append(head, body);
  if (SETTINGS_WIN) sh.classList.add("in-settings");
}

// VoiceOver: icon-only buttons get their tooltip as a spoken label
function labelIconButtons(root = document) {
  root.querySelectorAll("button[title]:not([aria-label])").forEach((b) => {
    if (!b.textContent.trim() || /^[^\w؀-ۿ]+$/.test(b.textContent.trim())) b.setAttribute("aria-label", b.title);
  });
}
let labelPending = false;
new MutationObserver(() => {
  if (labelPending) return;
  labelPending = true;
  requestAnimationFrame(() => { labelPending = false; labelIconButtons(); });
}).observe(document.body, { childList: true, subtree: true });
labelIconButtons();
