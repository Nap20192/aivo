/** $12.34 / -$12.34 from integer cents, same shape as the prototype. */
export function fmt(cents: number): string {
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100).toFixed(2);
}

/** "20:16" */
export function timeHM(t: number | Date = new Date()): string {
  const d = typeof t === "number" ? new Date(t) : t;
  return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0");
}

/** "waiting 4 min" */
export function waiting(createdAtMs: number, now: number = Date.now()): string {
  const min = Math.max(1, Math.round((now - createdAtMs) / 60000));
  return "waiting " + min + " min";
}

/** "150.00" (or "150", "150.5") -> 15000; null when not a number */
export function parseDollars(s: string): number | null {
  if (!/^\d+(\.\d*)?$/.test(s)) return null;
  const n = parseFloat(s);
  return Number.isNaN(n) ? null : Math.round(n * 100);
}
