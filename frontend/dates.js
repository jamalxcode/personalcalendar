// Date helpers shared by the panel (app.js) and the week window (week.js).
export const pad = (n) => String(n).padStart(2, "0");
export function startOfMonth(d) { return new Date(d.getFullYear(), d.getMonth(), 1); }
export function startOfDay(d) { return new Date(d.getFullYear(), d.getMonth(), d.getDate()); }
export function addDays(d, n) { const x = new Date(d); x.setDate(x.getDate() + n); return x; }
export function sameDay(a, b) { return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate(); }
export function dayKey(d) { return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`; }
export const dateVal = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
export const timeVal = (d) => `${pad(d.getHours())}:${pad(d.getMinutes())}`;
export const fromInputs = (date, time) => { const [y, m, dd] = date.split("-").map(Number); const [h, mi] = (time || "00:00").split(":").map(Number); return new Date(y, m - 1, dd, h, mi); };
