export function fmtCents(cents: number): string {
  return "$" + (cents / 100).toFixed(2);
}

export function hhmm(t: number | string | Date): string {
  const d = new Date(t);
  return (
    String(d.getHours()).padStart(2, "0") +
    ":" +
    String(d.getMinutes()).padStart(2, "0")
  );
}

/** m:ss, rounded up to the next whole second. */
export function countdownStr(ms: number): string {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
}
