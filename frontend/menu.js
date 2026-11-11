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
