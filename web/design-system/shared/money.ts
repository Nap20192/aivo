// Money is integer cents everywhere (docs/PLATFORM.md). Canonical helpers —
// apps import these by relative path, same as the tokens. No app-local copies.

/** 1250 → "$12.50", -50 → "-$0.50". */
export function formatCents(cents: number): string {
  return (cents < 0 ? "-$" : "$") + (Math.abs(cents) / 100).toFixed(2);
}

/**
 * "12" | "12.5" | "12.50" | "$12.50" | "12,50" → cents; null when unparseable.
 * Union of the admin ($ prefix, decimal comma) and POS (bare trailing dot)
 * input grammars.
 */
export function parseDollars(input: string): number | null {
  const cleaned = input.trim().replace(/^\$/, "").replace(",", ".");
  if (!/^\d+(\.\d*)?$/.test(cleaned)) return null;
  const n = parseFloat(cleaned);
  return Number.isNaN(n) ? null : Math.round(n * 100);
}
