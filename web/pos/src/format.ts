// Money helpers live in web/design-system/shared/money.ts (canonical).

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

/** Default single-select mod for a menu item: its own first label, verbatim. */
export function defaultMod(item: { mods?: string[] }): string | null {
  return item.mods?.[0] ?? null;
}
