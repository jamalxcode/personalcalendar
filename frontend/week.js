import { Call, Events } from "/wails/runtime.js";
import { popMenu, pickDate } from "./menu.js";
import { pad, startOfDay, addDays, sameDay, dateVal, timeVal, fromInputs } from "./dates.js";
import { flightOf, flyingSoon } from "./flights.js";

// Smart Cal — week window: 7-day timeline with drag to move (across days), resize, create, details, undo.
const go = (m, ...a) => Call.ByName("main.CalService." + m, ...a);
const errMsg = (e) => (e && (e.message || e.toString())) || "Something went wrong";
const $ = (id) => document.getElementById(id);
const el = (tag, cls, text) => { const n = document.createElement(tag); if (cls) n.className = cls; if (text != null) n.textContent = text; return n; };
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const HOUR = 52, SNAP = 15;

let S = { weekStart: 6, weekendDays: [5, 6], use24h: true, calendars: [] };
let weekStart = null;
let events = [];
let selected = null;

function fmtTime(d) {
  if (S.use24h) return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  const h = d.getHours() % 12 || 12, m = d.getMinutes(), ap = d.getHours() < 12 ? "AM" : "PM";
  return m ? `${h}:${pad(m)} ${ap}` : `${h} ${ap}`;
}
function applyAccent(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || "");
  if (!m) return;
  const n = parseInt(m[1], 16), rgb = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  const r = document.documentElement.style;
  r.setProperty("--accent", "#" + m[1]);
  r.setProperty("--accent-rgb", rgb.join(", "));
}

let toastTimer;
function toast(msg, isErr, action) {
  const t = $("toast");
  t.innerHTML = "";
  t.className = "toast" + (isErr ? " err" : "");
  t.append(el("span", "", msg));
  if (action) {
    const b = el("button", "toast-btn", action.label);
    b.onclick = () => { t.classList.add("hidden"); action.fn(); };
    t.append(b);
  }
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hidden"), action ? 7000 : 3000);
}

function weekStartOf(d) { return addDays(startOfDay(d), -((d.getDay() - (S.weekStart ?? 6) + 7) % 7)); }

async function load() {
  try {
    const list = await go("EventsBetween", weekStart.toISOString(), addDays(weekStart, 7).toISOString());
    events = list.map((e) => ({ ...e, s: new Date(e.start), e: new Date(e.end) }));
  } catch (err) { toast(errMsg(err), true); }
  render();
}

function render() {
  const days = [...Array(7)].map((_, i) => addDays(weekStart, i));
  const now = new Date();
  const wk = new Set(S.weekendDays ?? [5, 6]);
  const last = days[6];
  $("wkTitle").textContent = weekStart.getMonth() === last.getMonth()
    ? `${weekStart.getDate()}–${last.getDate()} ${last.toLocaleDateString(LOC(), { month: "long", year: "numeric" })}`
    : `${weekStart.toLocaleDateString(LOC(), { day: "numeric", month: "short" })} – ${last.toLocaleDateString(LOC(), { day: "numeric", month: "short", year: "numeric" })}`;

  // header
  const head = $("wkHead");
  head.innerHTML = "<div></div>";
  days.forEach((d) => {
    const h = el("div", sameDay(d, now) ? "today" : "");
    h.innerHTML = `${d.toLocaleDateString(LOC(), { weekday: "short" })}<span class="num">${d.getDate()}</span>`;
    const w = (S.weather || []).find((x) => x.date === dateVal(d));
    if (w && S.playful !== false) h.insertAdjacentHTML("beforeend", `<span class="wx">${wxIcon(w.code)} ${Math.round(w.max)}°</span>`);
    head.append(h);
  });
  // all-day row
  const ad = $("wkAllDay");
  ad.innerHTML = "<div></div>";
  days.forEach((d) => {
    const c = el("div");
    events.filter((e) => e.allDay && e.s < addDays(d, 1) && e.e > d).forEach((e) => {
      const chip = el("div", "wk-chip" + (e.holiday ? " hol" : ""), (e.holiday ? "🎉 " : "") + e.title);
      chip.style.setProperty("--c", e.color || "var(--accent)");
      chip.onclick = () => openDrawer(e);
      chip.oncontextmenu = (ev) => { ev.preventDefault(); eventMenu(e, chip, { x: ev.clientX, y: ev.clientY }); };
      c.append(chip);
    });
    ad.append(c);
  });
  // grid
  const g = $("wkGrid");
  g.innerHTML = "";
  g.style.height = `${24 * HOUR}px`;
  const hours = el("div", "wk-hours");
  for (let h = 1; h < 24; h++) { const s = el("span", "", fmtTime(new Date(2000, 0, 1, h))); s.style.top = `${h * HOUR}px`; hours.append(s); }
  g.append(hours);
  days.forEach((d, i) => {
    const col = el("div", "wk-col" + (wk.has(d.getDay()) ? " wknd" : "") + (sameDay(d, now) ? " today" : ""));
    col.dataset.i = i;
    for (let h = 0; h < 24; h++) {
      const l = el("div", "wk-line"); l.style.top = `${h * HOUR}px`; col.append(l);
      const hl = el("div", "wk-line half"); hl.style.top = `${h * HOUR + HOUR / 2}px`; col.append(hl);
    }
    if (sameDay(d, now)) { const n = el("div", "wk-now"); n.style.top = `${((now - d) / 36e5) * HOUR}px`; col.append(n); }
    const timed = events.filter((e) => !e.allDay && !e.holiday && e.s < addDays(d, 1) && e.e > d).sort((a, b) => a.s - b.s);
    const cols = [];
    const placed = timed.map((e) => { let c = 0; while (cols[c] && cols[c] > e.s) c++; cols[c] = e.e; return { e, c }; });
    const n = Math.max(1, cols.length);
    placed.forEach(({ e, c }) => col.append(block(e, d, c, n)));
    col.ondblclick = (ev) => {
      if (ev.target !== col && !ev.target.classList.contains("wk-line")) return;
      const y = ev.clientY - col.getBoundingClientRect().top;
      const mins = Math.floor(((y / HOUR) * 60) / 30) * 30;
      const s = new Date(d.getTime() + mins * 6e4);
      openCreate(s, new Date(s.getTime() + 36e5));
    };
    g.append(col);
  });
}

function wxIcon(c) { return c === 0 ? "☀️" : c <= 2 ? "🌤️" : c === 3 ? "☁️" : c <= 48 ? "🌫️" : c <= 67 ? "🌧️" : c <= 77 ? "🌨️" : c <= 82 ? "🌧️" : "⛈️"; }

function block(e, day, col, cols) {
  const b = el("div", "wk-ev" + (e.e < new Date() ? " past" : "") + (selected && selected.id === e.id && +selected.s === +e.s ? " sel" : ""));
  const s = e.s < day ? day : e.s, en = e.e > addDays(day, 1) ? addDays(day, 1) : e.e;
  const top = ((s - day) / 36e5) * HOUR, height = Math.max(18, ((en - s) / 36e5) * HOUR - 2);
  Object.assign(b.style, { top: `${top}px`, height: `${height}px`, left: `calc(${(col / cols) * 100}% + 2px)`, width: `calc(${100 / cols}% - 4px)` });
  b.style.setProperty("--c", e.color || "var(--accent)");
  const t = el("div", "t", e.title + (e.recurring ? " 🔁" : ""));
  const m = el("div", "m", `${fmtTime(e.s)} – ${fmtTime(e.e)}${e.location ? " · " + e.location : ""}`);
  b.append(t, m);
  b.oncontextmenu = (ev) => { ev.preventDefault(); eventMenu(e, b, { x: ev.clientX, y: ev.clientY }); };
  const editable = e.writable !== false;
  if (editable) b.append(el("div", "rz"));

  let mode = null, x0 = 0, y0 = 0, moved = false, dm = 0, dd = 0, de = 0, colW = 0;
  const snap = (px) => Math.round(((px / HOUR) * 60) / SNAP) * SNAP;
  b.onpointerdown = (ev) => {
    if (ev.button !== 0 || ev.ctrlKey) return; // right-click / Control-click opens the menu instead
    mode = !editable ? "click" : ev.target.classList.contains("rz") ? "resize" : "move";
    x0 = ev.clientX; y0 = ev.clientY; moved = false; dm = dd = de = 0;
    colW = b.parentElement.getBoundingClientRect().width;
    b.setPointerCapture(ev.pointerId);
    ev.preventDefault();
  };
  b.onpointermove = (ev) => {
    if (!mode || mode === "click") return;
    const dx = ev.clientX - x0, dy = ev.clientY - y0;
    if (Math.abs(dx) + Math.abs(dy) > 4) moved = true;
    if (!moved) return;
    b.classList.add("dragging");
    if (mode === "move") {
      dm = snap(dy); dd = Math.round(dx / colW);
      b.style.transform = `translate(${dd * colW}px, ${(dm / 60) * HOUR}px)`;
    } else {
      de = Math.max(snap(dy), SNAP - (e.e - e.s) / 6e4);
      b.style.height = `${Math.max(18, height + (de / 60) * HOUR)}px`;
    }
    const ns = new Date(e.s.getTime() + (mode === "move" ? dm * 6e4 + dd * 864e5 : 0));
    const ne = new Date(e.e.getTime() + (mode === "move" ? dm * 6e4 + dd * 864e5 : de * 6e4));
    m.textContent = `${fmtTime(ns)} – ${fmtTime(ne)}${dd ? " · " + ns.toLocaleDateString(LOC(), { weekday: "short" }) : ""}`;
  };
  b.onpointerup = async () => {
    const md = mode; mode = null;
    if (!md) return; // the press wasn't ours (right-click / Control-click)
    b.classList.remove("dragging");
    if (!moved) { openDrawer(e); return; }
    if (md === "click") return;
    const shift = md === "move" ? dm * 6e4 + dd * 864e5 : 0;
    const ns = new Date(e.s.getTime() + shift), ne = new Date(e.e.getTime() + (md === "move" ? shift : de * 6e4));
    if (+ns === +e.s && +ne === +e.e) { render(); return; }
    await moveEvent(e, ns, ne);
  };
  return b;
}

async function fullDraft(e) {
  let d = e;
  try { d = await go("GetEvent", e.id, e.start); } catch (_) {}
  return {
    full: d,
    draft: { title: d.title, start: d.start, end: d.end, allDay: !!d.allDay, location: d.location || "", notes: d.notes || "",
      calendarId: d.calendarId, timeZone: d.timeZone || "", alarm: "", repeat: { freq: "none", interval: 1, weekdays: [], until: "", count: 0 }, setRepeat: false, reminderMinutes: 0 },
  };
}

function scope(title, verb) {
  return new Promise((res) => {
    const card = $("wkModalCard");
    card.innerHTML = `<b>${esc(title)}</b><span class="muted">This is a repeating event.</span><div class="btns"></div>`;
    const btns = card.querySelector(".btns");
    [["Cancel", "", null], [`${verb} all future`, "", "future"], [`${verb} this one`, verb === "Delete" ? "danger" : "primary", "this"]].forEach(([l, c, v]) => {
      const b = el("button", "btn " + c, l);
      b.onclick = () => { $("wkModal").classList.add("hidden"); res(v); };
      btns.append(b);
    });
    $("wkModal").classList.remove("hidden");
  });
}

async function moveEvent(e, ns, ne) {
  const { draft } = await fullDraft(e);
  let allFuture = false;
  if (e.recurring) {
    const sc = await scope(`Move “${e.title}”`, "Move");
    if (!sc) { render(); return; }
    allFuture = sc === "future";
  }
  const prev = { ...draft };
  const next = { ...draft, start: ns.toISOString(), end: ne.toISOString() };
  try {
    await go("UpdateEvent", e.id, e.start, next, allFuture);
    let note = "";
    try { const cl = await go("CheckClashes", next, e.id); if (cl.length) note = ` · overlaps “${cl[0].title}”`; } catch (_) {}
    const day = ns.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
    toast(`Moved to ${day}${e.allDay ? "" : " " + fmtTime(ns)}${note}`, false, {
      label: "Undo", fn: async () => { try { await go("UpdateEvent", e.id, next.start, prev, allFuture); load(); } catch (err) { toast(errMsg(err), true); } },
    });
  } catch (err) { toast(errMsg(err), true); }
  load();
}

// ---------- right-click an event: reschedule it (same menu as the panel) ----------
// Days count from today; the event keeps its time of day and length.
function eventMenu(e, node, at) {
  const today = startOfDay(new Date());
  const short = (d) => d.toLocaleDateString(LOC(), { weekday: "short", day: "numeric", month: "short" });
  const items = [];
  if (e.writable !== false && !e.holiday) {
    for (const [label, n] of [["Today", 0], ["Tomorrow", 1], ["In 2 days", 2], ["In 3 days", 3], ["Next week", 7]]) {
      const day = addDays(today, n);
      if (sameDay(day, e.s)) continue; // already on that day
      items.push({ label, hint: short(day), fn: () => moveToDay(e, day) });
    }
    items.push({ label: "Pick a date…", fn: () => pickDate(at, `Move “${e.title}” to`, e.s, (day) => moveToDay(e, day)) }, "-");
  }
  items.push({ label: "Open", fn: () => openDrawer(e) });
  if (e.writable !== false) items.push({ label: "Edit…", fn: async () => { const { draft } = await fullDraft(e); openEdit(e, draft); } });
  items.push({ label: "Duplicate", fn: () => duplicate(e) });
  if (e.writable !== false) items.push("-", { label: "Delete", danger: true, fn: async () => { const { full, draft } = await fullDraft(e); del(e, full, draft); } });
  popMenu(node, items, at);
}

// whole calendar days, so a DST change can't move the clock time
function moveToDay(e, day) {
  const shift = Math.round((startOfDay(day) - startOfDay(e.s)) / 864e5);
  if (!shift) return;
  const ns = new Date(e.s), ne = new Date(e.e);
  ns.setDate(ns.getDate() + shift); ne.setDate(ne.getDate() + shift);
  moveEvent(e, ns, ne);
}

async function duplicate(e) {
  const { draft } = await fullDraft(e);
  modalForm("Duplicate event", { ...draft }, async (d) => {
    try {
      const r = await go("Add", [d]);
      toast(`Added “${d.title}”`, false, { label: "Undo", fn: async () => { for (const c of r.created) await go("Delete", c.id, c.start, false); load(); } });
      load();
    } catch (err) { toast(errMsg(err), true); }
  });
}

// ---------- details drawer ----------
async function openDrawer(e) {
  selected = e;
  render();
  const dr = $("wkDrawer");
  dr.classList.remove("hidden");
  dr.innerHTML = `<div class="sheet-body"><div class="muted">Loading…</div></div>`;
  const { full, draft } = await fullDraft(e);
  if (selected !== e) return;
  const s = new Date(full.start), en = new Date(full.end);
  const when = full.allDay ? s.toLocaleDateString(LOC(), { weekday: "long", day: "numeric", month: "long" })
    : `${s.toLocaleDateString(LOC(), { weekday: "long", day: "numeric", month: "long" })} · ${fmtTime(s)} – ${fmtTime(en)}`;
  dr.innerHTML = `
    <div class="sheet-head"><button class="pill-btn" id="drClose">✕ Close</button><span class="sheet-dot" style="background:${esc(full.color || "var(--accent)")}"></span></div>
    <div class="sheet-body">
      <div class="sheet-title">${esc(full.title)}</div>
      <div class="sheet-when">${esc(when)}</div>
      <div class="sheet-rows">
        <div class="sheet-row"><span class="ico">📅</span><div>${esc(full.calendar || "")}</div></div>
        ${full.recurring ? `<div class="sheet-row"><span class="ico">🔁</span><div>Repeating event</div></div>` : ""}
        ${full.location ? `<div class="sheet-row"><span class="ico">📍</span><div><a href="#" data-open="https://maps.apple.com/?q=${encodeURIComponent(full.location)}">${esc(full.location)}</a></div></div>` : ""}
        ${(() => { const f = flightOf(full.title, full.location, full.notes); return f ? `<div class="sheet-row"><span class="ico">✈️</span><div><button class="btn primary small-btn" data-open="${esc(f.url)}">Track ${esc(f.label)} on Flightradar24</button></div></div>` : ""; })()}
        ${full.meetLink ? `<div class="sheet-row"><span class="ico">🎥</span><div><button class="btn primary small-btn" data-open="${esc(full.meetLink)}">Join meeting</button></div></div>` : ""}
        ${full.notes ? `<div class="sheet-row"><span class="ico">📝</span><div class="notes">${esc(full.notes).replace(/\n/g, "<br>")}</div></div>` : ""}
      </div>
    </div>
    <div class="sheet-actions"></div>`;
  const acts = dr.querySelector(".sheet-actions");
  const mk = (l, c, fn) => { const b = el("button", "btn " + c, l); b.onclick = fn; acts.append(b); };
  if (full.writable !== false) mk("Delete", "danger-ghost", () => del(e, full, draft));
  mk("Open in Calendar", "", () => go("OpenURL", full.link));
  if (full.writable !== false) mk("Edit", "primary", () => openEdit(e, draft));
  $("drClose").onclick = closeDrawer;
  dr.querySelectorAll("[data-open]").forEach((a) => { a.onclick = (ev) => { ev.preventDefault(); go("OpenURL", a.dataset.open); }; });
}
function closeDrawer() { $("wkDrawer").classList.add("hidden"); selected = null; render(); }

async function del(e, full, draft) {
  let allFuture = false;
  if (full.recurring) { const sc = await scope(`Delete “${e.title}”?`, "Delete"); if (!sc) return; allFuture = sc === "future"; }
  try {
    await go("Delete", e.id, e.start, allFuture);
    closeDrawer();
    if (allFuture) toast("Deleted this and all future events");
    else toast(`Deleted “${e.title}”`, false, { label: "Undo", fn: async () => { try { await go("Add", [draft]); load(); } catch (err) { toast(errMsg(err), true); } } });
    load();
  } catch (err) { toast(errMsg(err), true); }
}

// ---------- create / edit modal ----------
function modalForm(title, d, onSave) {
  const card = $("wkModalCard");
  const s = new Date(d.start), en = new Date(d.end);
  const writable = (S.calendars || []).filter((c) => c.writable);
  card.innerHTML = `
    <div class="sheet-kicker">${esc(title)}</div>
    <input class="f-title" id="mTitle" placeholder="Title">
    <div class="row"><input type="date" id="mSD"><input type="time" id="mST"></div>
    <div class="row"><input type="date" id="mED"><input type="time" id="mET"></div>
    <input id="mLoc" placeholder="Location">
    <select id="mCal"></select>
    <textarea id="mNotes" rows="3" placeholder="Notes"></textarea>
    <div class="btns"><button class="btn" id="mCancel">Cancel</button><button class="btn primary" id="mSave">Save ↩</button></div>`;
  $("mTitle").value = d.title || "";
  $("mSD").value = dateVal(s); $("mST").value = timeVal(s);
  $("mED").value = dateVal(en); $("mET").value = timeVal(en);
  $("mLoc").value = d.location || "";
  $("mNotes").value = d.notes || "";
  writable.forEach((c) => { const o = el("option", "", c.name); o.value = c.id; if (c.id === d.calendarId || (!d.calendarId && c.primary)) o.selected = true; $("mCal").append(o); });
  $("wkModal").classList.remove("hidden");
  const close = () => $("wkModal").classList.add("hidden");
  $("mCancel").onclick = close;
  const save = async () => {
    const ns = fromInputs($("mSD").value, $("mST").value), ne = fromInputs($("mED").value, $("mET").value);
    if (ne <= ns) { toast("The end must be after the start", true); return; }
    const t = $("mTitle").value.trim();
    if (!t) { toast("Give it a title", true); return; }
    await onSave({ ...d, title: t, start: ns.toISOString(), end: ne.toISOString(), allDay: false, location: $("mLoc").value.trim(), notes: $("mNotes").value, calendarId: $("mCal").value });
    close();
  };
  $("mSave").onclick = save;
  card.onkeydown = (ev) => { if (ev.key === "Enter" && ev.target.tagName !== "TEXTAREA") { ev.preventDefault(); save(); } if (ev.key === "Escape") close(); };
  setTimeout(() => $("mTitle").focus(), 30);
}

function openCreate(s, e) {
  const base = { title: "", start: s.toISOString(), end: e.toISOString(), allDay: false, location: "", notes: "", calendarId: S.defaultCalendar || "",
    timeZone: "", alarm: "", repeat: { freq: "none", interval: 1, weekdays: [], until: "", count: 0 }, setRepeat: false, reminderMinutes: 0 };
  modalForm("New event", base, async (d) => {
    try {
      const r = await go("Add", [d]);
      toast(`Added “${d.title}”`, false, { label: "Undo", fn: async () => { for (const c of r.created) await go("Delete", c.id, c.start, false); load(); } });
      load();
    } catch (err) { toast(errMsg(err), true); }
  });
}

function openEdit(e, draft) {
  modalForm("Edit event", draft, async (d) => {
    let allFuture = false;
    if (e.recurring) { const sc = await scope("Save changes", "Save"); if (!sc) return; allFuture = sc === "future"; }
    try {
      await go("UpdateEvent", e.id, e.start, d, allFuture);
      closeDrawer();
      toast("Saved", false, { label: "Undo", fn: async () => { try { await go("UpdateEvent", e.id, d.start, draft, allFuture); load(); } catch (err) { toast(errMsg(err), true); } } });
      load();
    } catch (err) { toast(errMsg(err), true); }
  });
}

// ---------- wiring ----------
function goWeek(n) { weekStart = addDays(weekStart, 7 * n); load(); }
function scrollToMorning() {
  const now = new Date();
  const h = sameDay(weekStartOf(now), weekStart) ? Math.max(0, now.getHours() - 2) : 7;
  $("wkScroll").scrollTop = h * HOUR;
}
$("wkPrev").onclick = () => goWeek(-1);
$("wkNext").onclick = () => goWeek(1);
$("wkToday").onclick = () => { weekStart = weekStartOf(new Date()); load().then(scrollToMorning); };
$("wkNew").onclick = () => { const s = new Date(); s.setMinutes(s.getMinutes() < 30 ? 30 : 60, 0, 0); openCreate(s, new Date(s.getTime() + 36e5)); };
document.addEventListener("keydown", (ev) => {
  const typing = ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement && document.activeElement.tagName);
  if (typing || !$("wkModal").classList.contains("hidden")) return;
  if (ev.key === "ArrowLeft") goWeek(-1);
  if (ev.key === "ArrowRight") goWeek(1);
  if (ev.key.toLowerCase() === "t") $("wkToday").click();
  if (ev.key.toLowerCase() === "n") $("wkNew").click();
  if (ev.key === "Escape") closeDrawer();
  if ((ev.key === "Backspace" || ev.key === "Delete") && selected) fullDraft(selected).then(({ full, draft }) => del(selected, full, draft));
});

function applyState(st) {
  if (!st) return;
  const firstLoad = !weekStart;
  S = st;
  if ((S.lang || "en") !== LANG) i18nApply(S.lang || "en");
  document.body.classList.toggle("reduce-motion", !!S.reduceMotion);
  applyAccent(S.accent);
  if (firstLoad) weekStart = weekStartOf(new Date());
  load().then(() => { if (firstLoad) scrollToMorning(); });
}
Events.On("state", (ev) => { S = ev.data; if ((S.lang || "en") !== LANG) i18nApply(S.lang || "en"); applyAccent(S.accent); if (weekStart) load(); });
Events.On("week:shown", () => { go("GetState").then(applyState); });
go("GetState").then(applyState).catch((err) => toast(errMsg(err), true));
setInterval(render, 60000);

// ---------- flights: "KU 101", "EK857", "flight QR1077" → Flightradar24 ----------
