// Hijri dates, local occasions and Ramadan prayer times — computed on the Mac, nothing fetched.
// Shared by the panel; kept separate so the maths can be checked on its own.

const HIJRI_MONTHS = ["Muharram", "Safar", "Rabi al-Awwal", "Rabi al-Thani", "Jumada al-Ula", "Jumada al-Akhirah",
  "Rajab", "Sha'ban", "Ramadan", "Shawwal", "Dhu al-Qa'dah", "Dhu al-Hijjah"];

// hijri(date) → { day, month (1–12), year, name } using the Umm al-Qura calendar (as in the Gulf)
export function hijri(date) {
  try {
    const parts = new Intl.DateTimeFormat("en-u-ca-islamic-umalqura-nu-latn", { day: "numeric", month: "numeric", year: "numeric" }).formatToParts(date);
    const get = (t) => parseInt((parts.find((p) => p.type === t) || {}).value, 10);
    const day = get("day"), month = get("month"), year = get("year");
    if (!day || !month) return null;
    return { day, month, year, name: HIJRI_MONTHS[month - 1] };
  } catch (_) { return null; }
}

// hijriText: "2 Jumada al-Akhirah 1448" (Arabic: the system's Arabic month names)
export function hijriText(date, lang) {
  if (lang === "ar") {
    try { return new Intl.DateTimeFormat("ar-SA-u-ca-islamic-umalqura-nu-latn", { day: "numeric", month: "long", year: "numeric" }).format(date); } catch (_) {}
  }
  const h = hijri(date);
  return h ? `${h.day} ${h.name} ${h.year}` : "";
}

// occasion(date) → { kind, text, emoji } for days worth marking, else null
export function occasion(date) {
  const m = date.getMonth() + 1, d = date.getDate();
  if (m === 2 && d === 25) return { kind: "kw", emoji: "🇰🇼", text: "Happy National Day!" };
  if (m === 2 && d === 26) return { kind: "kw", emoji: "🇰🇼", text: "Happy Liberation Day!" };
  const h = hijri(date);
  if (!h) return null;
  if (h.month === 9) return { kind: "ramadan", emoji: "🌙", text: `Ramadan Kareem · day ${h.day}` };
  if (h.month === 10 && h.day <= 3) return { kind: "eid", emoji: "🌙", text: "Eid Mubarak!" };
  if (h.month === 12 && h.day === 9) return { kind: "arafah", emoji: "🕋", text: "Day of Arafah" };
  if (h.month === 12 && h.day >= 10 && h.day <= 13) return { kind: "eid", emoji: "🐑", text: "Eid al-Adha Mubarak!" };
  if (h.month === 1 && h.day === 1) return { kind: "newyear", emoji: "🌙", text: `Happy Islamic New Year ${h.year}` };
  return null;
}

// prayerTimes(date, lat, lon) → { fajr, maghrib } as Dates (≈ a minute or two), using the sun's
// position: Fajr when the sun is 18° below the horizon (as in Kuwait), Maghrib at sunset.
export function prayerTimes(date, lat, lon) {
  if (!isFinite(lat) || !isFinite(lon) || (lat === 0 && lon === 0)) return null;
  const rad = Math.PI / 180, deg = 180 / Math.PI;
  const y = date.getFullYear(), mo = date.getMonth() + 1, d = date.getDate();
  // Julian day at local noon
  const a = Math.floor((14 - mo) / 12), yy = y + 4800 - a, mm = mo + 12 * a - 3;
  const jdn = d + Math.floor((153 * mm + 2) / 5) + 365 * yy + Math.floor(yy / 4) - Math.floor(yy / 100) + Math.floor(yy / 400) - 32045;
  const D = jdn - 2451545.0;
  const g = (357.529 + 0.98560028 * D) * rad;
  const q = 280.459 + 0.98564736 * D;
  const L = (q + 1.915 * Math.sin(g) + 0.020 * Math.sin(2 * g)) * rad;
  const e = (23.439 - 0.00000036 * D) * rad;
  const RA = (Math.atan2(Math.cos(e) * Math.sin(L), Math.cos(L)) * deg) / 15;
  const decl = Math.asin(Math.sin(e) * Math.sin(L));
  const fix = (h) => ((h % 24) + 24) % 24;
  const eqt = q / 15 - fix(RA);
  const tz = -new Date(y, mo - 1, d, 12).getTimezoneOffset() / 60;
  const noon = fix(12 + tz - lon / 15 - eqt);
  const T = (angle) => {
    const c = (-Math.sin(angle * rad) - Math.sin(decl) * Math.sin(lat * rad)) / (Math.cos(decl) * Math.cos(lat * rad));
    return c < -1 || c > 1 ? NaN : (Math.acos(c) * deg) / 15;
  };
  const at = (h) => { if (!isFinite(h)) return null; const t = new Date(y, mo - 1, d, 0, 0, 0, 0); t.setSeconds(Math.round(h * 3600)); return t; };
  return { fajr: at(noon - T(18)), maghrib: at(noon + T(0.833)) };
}
