// Money is integer cents everywhere (docs/PLATFORM.md). Single format helper.
export function formatCents(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  return `${sign}$${(abs / 100).toFixed(2)}`;
}

// "12.50" | "12" | "$12.50" -> 1250; null when unparseable.
export function parseMoney(input: string): number | null {
  const cleaned = input.trim().replace(/^\$/, "").replace(",", ".");
  if (!/^\d+(\.\d{1,2})?$/.test(cleaned)) return null;
  return Math.round(parseFloat(cleaned) * 100);
}
