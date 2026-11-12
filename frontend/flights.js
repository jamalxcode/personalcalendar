// Flight numbers in event titles ("KU 101", "EK857", "flight QR1077") → Flightradar24.
// Shared by the panel (app.js) and the week window (week.js).
export const AIRLINES = new Set(("KU EK QR EY GF WY SV J9 FZ G9 XY F3 MS RJ ME TK PC BA VS LH LX OS SN AF KL IB VY FR U2 W6 AZ " +
  "LO SK AY TP A3 SU PS AA DL UA AC AS B6 WN SQ CX NH JL KE OZ TG MH GA PR CI BR QF NZ VA EI PK AI 6E UK SG " +
  "IX WB UL KQ ET SA AT ZH MU CA CZ HU HX 3U KC HY FJ LY RO JU OU FB BG").split(" "));
export const FLIGHT_RE = /\b([A-Z][A-Z0-9]|[0-9][A-Z])\s?-?(\d{1,4})\b/g;
export function flightOf(...texts) {
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
export const flyingSoon = (e) => { const n = Date.now(); return !e.allDay && n >= +e.s - 36e5 && n <= +e.e + 36e5; };
