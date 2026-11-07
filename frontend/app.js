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
function renderMiniCal() {
  const now = new Date();
  $("monthName").textContent = viewMonth.toLocaleDateString("en-GB", { month: "long" });
  $("yearName").textContent = viewMonth.getFullYear();
  $("todayBtn").style.visibility = sameDay(viewMonth, startOfMonth(now)) && !selectedDay ? "hidden" : "visible";

  const dow = $("dow");
  if (!dow.childElementCount) {
    ["S", "M", "T", "W", "T", "F", "S"].forEach((d, i) => {
      dow.append(el("span", i === 5 || i === 6 ? "wknd" : "", d));
    });
  }

  // colour dots per day from loaded events
  const dots = new Map();
  for (const e of evs()) {
    let d = startOfDay(e.s);
    const last = e.allDay ? addDays(startOfDay(e.e), -1) : startOfDay(e.e);
    for (let i = 0; d <= last && i < 31; i++, d = addDays(d, 1)) {
      const k = dayKey(d);
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
    const cell = el("div", "day");
    if (d.getMonth() !== first.getMonth()) cell.classList.add("out");
    if (d.getDay() === 5 || d.getDay() === 6) cell.classList.add("wknd");
    if (sameDay(d, now)) cell.classList.add("today");
    if (selectedDay && sameDay(d, selectedDay)) cell.classList.add("sel");
    cell.append(el("span", "num", d.getDate()));
    const ds = dots.get(dayKey(d));
    if (ds) {
      const w = el("span", "dots");
      ds.forEach((c) => { const i = el("i"); i.style.background = c || "var(--accent)"; w.append(i); });
      cell.append(w);
    }
    cell.onclick = () => jumpToDay(d);
    cell.ondblclick = () => {
      // double-click a day: pre-fill quick add with that date
      const q = $("quick");
      q.value = `${q.value ? q.value + " " : ""}on ${d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" })} `;
      q.focus();
    };
    grid.append(cell);
  }
}

function jumpToDay(d) {
  selectedDay = sameDay(d, new Date()) ? null : d;
  renderMiniCal();
  const target = document.querySelector(`[data-day="${dayKey(d)}"]`);
  const scroll = $("scroll");
  if (target) {
    scroll.scrollTo({ top: target.offsetTop - scroll.offsetTop, behavior: "smooth" });
  } else if (sameDay(d, new Date())) {
    scroll.scrollTo({ top: 0, behavior: "smooth" });
  } else {
    toast(`Nothing on ${d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" })} — double-click a day to add`);
  }
}

// ---------- up next + list ----------
function renderUpNext() {
  const now = new Date();
  const box = $("upnext");
  box.innerHTML = "";
  const next = evs().find((e) => !e.allDay && e.e > now);
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
  card.onclick = () => go("OpenURL", next.link);
  box.append(card);
}

function renderList() {
  const list = $("list");
  list.innerHTML = "";
  const now = new Date();
  const all = evs();

  if (!S.connected) { list.append(emptyState("📅", "Allow calendar access to see what's coming up.")); return; }
  if (!all.length) {
    list.append(emptyState("🌴", S.error ? "Couldn't load events." : `Nothing in the next ${S.daysAhead} days.`));
    return;
  }

  // group by day; multi-day all-day events appear on each day they cover (within range)
  const groups = new Map();
  const push = (d, e) => { const k = dayKey(d); if (!groups.has(k)) groups.set(k, { d, items: [] }); groups.get(k).items.push(e); };
  for (const e of all) {
    if (e.allDay) {
      let d = startOfDay(e.s) < startOfDay(now) ? startOfDay(now) : startOfDay(e.s);
      for (let i = 0; d < startOfDay(e.e) && i < 14; i++, d = addDays(d, 1)) push(d, e);
    } else {
      push(startOfDay(e.s < now ? now : e.s), e);
    }
  }
  const keys = [...groups.values()].sort((a, b) => a.d - b.d);
  for (const g of keys) {
    g.items.sort((a, b) => (b.allDay - a.allDay) || (a.s - b.s));
    const head = el("div", "day-head" + (sameDay(g.d, now) ? " is-today" : ""));
    head.dataset.day = dayKey(g.d);
    const timed = g.items.filter((e) => !e.allDay).length;
    head.append(el("span", "", dayHead(g.d)), el("span", "count", timed ? `${timed} event${timed > 1 ? "s" : ""}` : ""));
    list.append(head);
    for (const e of g.items) list.append(eventRow(e, now));
  }
}

function emptyState(icon, text) {
  const d = el("div", "empty");
  d.append(el("div", "big", icon), el("div", "", text));
  return d;
}

function eventRow(e, now) {
  const row = el("div", "ev");
  if (e.allDay) row.classList.add("allday");
  if (!e.allDay && e.s <= now && e.e > now) row.classList.add("now");
  const bar = el("div", "bar");
  bar.style.background = e.color || "var(--accent)";
  const when = el("div", "when");
  if (e.allDay) when.textContent = "all-day";
  else {
    when.append(el("div", "", fmtTime(e.s)), el("div", "end", fmtTime(e.e)));
  }
  const body = el("div", "body");
  body.append(el("div", "title", e.title));
  const meta = [e.location, e.meetLink && !e.location ? "Google Meet" : "", S.calendars.length > 1 ? e.calendar : ""].filter(Boolean).join(" · ");
  if (meta) body.append(el("div", "meta", meta));
  const tools = el("div", "tools");
  if (e.meetLink) {
    const j = el("button", "", "▶︎");
    j.title = "Join Google Meet";
    j.onclick = (ev) => { ev.stopPropagation(); go("OpenURL", e.meetLink); };
    tools.append(j);
  }
  const del = el("button", "del", "🗑");
  del.title = "Delete";
  del.onclick = async (ev) => {
    ev.stopPropagation();
    const ok = await confirmBox(`<b>Delete “${esc(e.title)}”?</b><span class="muted">${esc(whenText(e.start, e.end, e.allDay))}<br>This removes it from your calendar${e.calendar ? " (" + esc(e.calendar) + ")" : ""}.</span>`);
    if (!ok) return;
    try { applyState(await go("Delete", e.id, e.start)); toast("Deleted"); }
    catch (err) { toast(errMsg(err), true); }
  };
  tools.append(del);
  row.append(bar, when, body, tools);
  row.title = "Open in Calendar";
  row.onclick = () => go("OpenURL", e.link);
  return row;
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
  if (!S.hasKey) parts.push(`Add an AI key for quick add — <a href="#" id="bannerGo">Settings</a>`);
  if (S.error) parts.push(esc(S.error));
  if (!parts.length) { b.classList.add("hidden"); return; }
  b.innerHTML = parts.join("<br>");
  b.classList.remove("hidden");
  if ($("bannerGo")) $("bannerGo").onclick = (ev) => { ev.preventDefault(); openSettings(); };
  if ($("bannerGoogle")) $("bannerGoogle").onclick = () => go("AddGoogleAccount");
}

function renderFooter() {
  $("updated").textContent = S.now ? `Updated ${fmtTime(new Date(S.now))}` : "";
  $("hotkeyHint").textContent = S.hotkey ? S.hotkey.replace("Ctrl", "⌃").replace("Alt", "⌥").replace(/\+/g, "") + " to open" : "";
}

function applyState(st) {
  if (!st) return;
  S = st;
  renderMiniCal();
  renderUpNext();
  renderList();
  renderBanner();
  renderFooter();
  if (drafts.length) renderDrafts();
  if (!$("settings").classList.contains("hidden")) renderGoogleBox();
}

// ---------- quick add ----------
function setBusy(b) {
  busy = b;
  $("spark").classList.toggle("busy", b);
  $("quick").disabled = b;
}

async function submitQuick() {
  const text = $("quick").value.trim();
  if (busy) return;
  if (!text) { if (drafts.length) addDrafts(); return; }
  setBusy(true);
  try {
    drafts = await go("Parse", text);
    $("quick").value = "";
    if (!S.confirmBeforeAdd) { await addDrafts(); return; }
    renderDrafts();
  } catch (err) {
    toast(errMsg(err), true);
  } finally {
    setBusy(false);
    $("quick").focus();
  }
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

function clearDrafts() { drafts = []; renderDrafts(); $("quick").focus(); }

async function addDrafts() {
  if (busy || !drafts.length) return;
  setBusy(true);
  const n = drafts.length, first = drafts[0];
  try {
    const st = await go("Add", drafts);
    drafts = [];
    renderDrafts();
    applyState(st);
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

// ---------- settings ----------
async function openSettings() {
  $("main").classList.add("hidden");
  $("settings").classList.remove("hidden");
  settingsCache = await go("GetSettings");
  $("sKey").value = "";
  $("sKey").placeholder = settingsCache.openaiKeyHint || "sk-…";
  $("sModel").value = settingsCache.openaiModel;
  $("s24").checked = settingsCache.use24h;
  $("sConfirm").checked = settingsCache.confirmBeforeAdd;
  $("sDays").value = String(settingsCache.daysAhead);
  renderGoogleBox();
}

function renderGoogleBox() {
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
  };
  try {
    applyState(await go("SaveSettings", payload));
    toast("Settings saved");
    closeSettings();
  } catch (err) { toast(errMsg(err), true); }
}

// ---------- wiring ----------
function wire() {
  $("quickForm").onsubmit = (ev) => { ev.preventDefault(); submitQuick(); };
  $("prevMonth").onclick = () => { viewMonth = new Date(viewMonth.getFullYear(), viewMonth.getMonth() - 1, 1); renderMiniCal(); };
  $("nextMonth").onclick = () => { viewMonth = new Date(viewMonth.getFullYear(), viewMonth.getMonth() + 1, 1); renderMiniCal(); };
  $("todayBtn").onclick = () => { viewMonth = startOfMonth(new Date()); jumpToDay(new Date()); };
  $("openSettings").onclick = openSettings;
  $("closeSettings").onclick = closeSettings;
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
    if (ev.key === "Escape") {
      ev.preventDefault();
      if (!$("confirm").classList.contains("hidden")) { $("confirmNo").click(); return; }
      if (inSettings) { closeSettings(); return; }
      if (drafts.length) { clearDrafts(); return; }
      if ($("quick").value) { $("quick").value = ""; return; }
      go("Hide");
    }
    if (ev.metaKey && ev.key === ",") { ev.preventDefault(); openSettings(); }
    if (ev.metaKey && ev.key.toLowerCase() === "r") { ev.preventDefault(); refresh(); }
    if (ev.metaKey && ev.key.toLowerCase() === "n") { ev.preventDefault(); closeSettings(); }
    if (ev.metaKey && ev.key.toLowerCase() === "w") { ev.preventDefault(); go("Hide"); }
    if (ev.metaKey && ev.key.toLowerCase() === "t") { ev.preventDefault(); $("todayBtn").click(); }
  });

  Events.On("state", (ev) => applyState(ev.data));
  Events.On("ui:settings", () => openSettings());
  Events.On("popover:shown", () => {
    viewMonth = startOfMonth(new Date());
    selectedDay = null;
    renderMiniCal();
    renderUpNext();
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
