// Shared by the panel (app.js) and the week window (week.js): pop-up menus and a small date picker.
const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
};
const zoom = () => parseFloat(document.body.style.zoom || "1") || 1;

// A small pop-up menu, like an NSMenu: anchored above/below a button, or at the pointer for a right-click.
// items: [{ label, hint?, danger?, fn }, "-" (separator)]
export function popMenu(anchor, items, at) {
  document.querySelectorAll(".pop-menu").forEach((m) => m.remove());
  const m = el("div", "pop-menu");
  m.setAttribute("role", "menu");
  const close = () => { m.remove(); document.removeEventListener("mousedown", outside, true); document.removeEventListener("keydown", keys, true); anchor.focus(); };
  const outside = (ev) => { if (!m.contains(ev.target) && ev.target !== anchor) close(); };
  const btns = [];
  for (const it of items) {
    if (it === "-") { m.append(el("div", "pop-sep")); continue; }
    const b = el("button", "pop-item" + (it.danger ? " danger" : ""), it.label);
    if (it.hint) b.append(el("span", "pop-hint", it.hint));
    b.setAttribute("role", "menuitem");
    b.onclick = () => { close(); it.fn(); };
    m.append(b); btns.push(b);
  }
  const keys = (ev) => {
    const i = btns.indexOf(document.activeElement);
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); close(); }
    else if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
      ev.preventDefault(); ev.stopPropagation();
      const n = btns.length, j = i < 0 ? (ev.key === "ArrowDown" ? 0 : n - 1) : ev.key === "ArrowDown" ? (i + 1) % n : (i - 1 + n) % n;
      btns[j].focus();
    }
  };
  document.body.append(m);
  const z = zoom();
  const h = m.offsetHeight, w = m.offsetWidth, vw = window.innerWidth / z, vh = window.innerHeight / z;
  if (at) { // right-click: open at the pointer, like a Mac context menu
    const x = at.x / z, y = at.y / z;
    m.style.left = `${Math.max(4, Math.min(x, vw - w - 4))}px`;
    m.style.top = `${Math.max(4, y + h > vh - 4 ? y - h : y)}px`;
  } else {
    const r = anchor.getBoundingClientRect();
    const top = r.top - h - 4 >= 4 ? r.top - h - 4 : Math.min(r.bottom + 4, vh - h - 4);
    m.style.top = `${top}px`;
    m.style.left = `${Math.max(4, Math.min(r.right - w, vw - w - 4))}px`;
  }
  document.addEventListener("mousedown", outside, true);
  document.addEventListener("keydown", keys, true);
  // like a Mac menu: nothing is highlighted when opened with the mouse; from the keyboard the first item is
  const byKeyboard = !at && window.event && window.event.detail === 0;
  if (byKeyboard && btns[0]) btns[0].focus();
  else { m.tabIndex = -1; m.style.outline = "none"; m.focus(); }
}

// A small box at the pointer with a date field: ↩ or the button calls onPick(day), Esc or a click outside cancels.
export function pickDate(at, label, initial, onPick, button = "Move") {
  document.querySelectorAll(".pop-menu").forEach((x) => x.remove());
  const box = el("div", "pop-menu pop-date");
  const inp = el("input", "todo-date");
  inp.type = "date";
  const p = (n) => String(n).padStart(2, "0");
  inp.value = `${initial.getFullYear()}-${p(initial.getMonth() + 1)}-${p(initial.getDate())}`;
  const ok = el("button", "btn primary small-btn", button);
  box.append(el("div", "small muted", label), inp, ok);
  document.body.append(box);
  const z = zoom(), vw = window.innerWidth / z, vh = window.innerHeight / z;
  box.style.left = `${Math.max(4, Math.min(at ? at.x / z : 40, vw - box.offsetWidth - 4))}px`;
  box.style.top = `${Math.max(4, Math.min(at ? at.y / z : 120, vh - box.offsetHeight - 4))}px`;
  const close = () => { box.remove(); document.removeEventListener("mousedown", outside, true); };
  const outside = (ev) => { if (!box.contains(ev.target)) close(); };
  const save = () => {
    if (!inp.value) return;
    const [y, m, d] = inp.value.split("-").map(Number);
    close();
    onPick(new Date(y, m - 1, d));
  };
  inp.onkeydown = (ev) => {
    if (ev.key === "Enter") { ev.preventDefault(); save(); }
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); close(); }
  };
  ok.onclick = save;
  document.addEventListener("mousedown", outside, true);
  inp.focus();
}

// ---- "After…": move something later by an amount of time ----

const UNIT_LABEL = { minutes: ["minute", "minutes"], hours: ["hour", "hours"], days: ["day", "days"], weeks: ["week", "weeks"] };
export const durationText = (n, unit) => `${n} ${UNIT_LABEL[unit][n === 1 ? 0 : 1]}`;

// shiftBy: date + n units (days and weeks by calendar days, so daylight saving can't move the clock time)
export function shiftBy(date, n, unit) {
  const d = new Date(date);
  if (unit === "minutes") d.setMinutes(d.getMinutes() + n);
  else if (unit === "hours") d.setHours(d.getHours() + n);
  else if (unit === "days") d.setDate(d.getDate() + n);
  else if (unit === "weeks") d.setDate(d.getDate() + 7 * n);
  return d;
}

// laterBy: n units after `base` — or after now, when that would still be in the past
// (an overdue to-do "after 2 hours" means two hours from now, not two hours after it was due)
export function laterBy(base, n, unit, now = new Date()) {
  const t = shiftBy(base, n, unit);
  if (t > now) return t;
  if (unit === "days" || unit === "weeks") { // keep the time of day, count from today
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate(), base.getHours(), base.getMinutes(), 0, 0);
    return shiftBy(today, n, unit);
  }
  const r = shiftBy(now, n, unit);
  r.setSeconds(0, 0);
  return r;
}

// A small box at the pointer: quick choices plus "[n] [unit]"; onPick(n, unit). timeUnits=false → days/weeks only.
export function pickDuration(at, label, onPick, { timeUnits = true } = {}) {
  document.querySelectorAll(".pop-menu").forEach((x) => x.remove());
  const box = el("div", "pop-menu pop-date pop-after");
  box.append(el("div", "small muted", label));
  const chips = el("div", "after-chips");
  const quick = timeUnits
    ? [[15, "minutes", "15 min"], [30, "minutes", "30 min"], [1, "hours", "1 hour"], [2, "hours", "2 hours"], [1, "days", "1 day"], [3, "days", "3 days"], [1, "weeks", "1 week"]]
    : [[1, "days", "1 day"], [2, "days", "2 days"], [3, "days", "3 days"], [1, "weeks", "1 week"], [2, "weeks", "2 weeks"]];
  const close = () => { box.remove(); document.removeEventListener("mousedown", outside, true); };
  const outside = (ev) => { if (!box.contains(ev.target)) close(); };
  for (const [n, unit, txt] of quick) {
    const b = el("button", "btn small-btn", txt);
    b.onclick = () => { close(); onPick(n, unit); };
    chips.append(b);
  }
  const row = el("div", "after-row");
  const num = el("input", "after-n");
  num.type = "number"; num.min = "1"; num.max = "999"; num.value = timeUnits ? "2" : "3";
  const sel = el("select", "after-unit");
  for (const u of timeUnits ? ["minutes", "hours", "days", "weeks"] : ["days", "weeks"]) {
    const o = el("option", "", u); o.value = u; if (u === (timeUnits ? "hours" : "days")) o.selected = true; sel.append(o);
  }
  const ok = el("button", "btn primary small-btn", "Move");
  const go = () => { const n = parseInt(num.value, 10); if (!(n >= 1 && n <= 999)) { num.focus(); return; } close(); onPick(n, sel.value); };
  ok.onclick = go;
  row.append(el("span", "small", "After"), num, sel, ok);
  box.append(chips, row);
  box.addEventListener("keydown", (ev) => {
    if (ev.key === "Enter") { ev.preventDefault(); go(); }
    if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); close(); }
  });
  document.body.append(box);
  const z = zoom(), vw = window.innerWidth / z, vh = window.innerHeight / z;
  box.style.left = `${Math.max(4, Math.min(at ? at.x / z : 40, vw - box.offsetWidth - 4))}px`;
  box.style.top = `${Math.max(4, Math.min(at ? at.y / z : 120, vh - box.offsetHeight - 4))}px`;
  document.addEventListener("mousedown", outside, true);
  num.focus(); num.select();
}
