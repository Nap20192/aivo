import type { Handoff, MenuItem, OrderLineInput } from "./types";

export interface CartLine {
  menuItemId: string;
  name: string;
  unitCents: number;
  qty: number;
  detail: string;
  options: OrderLineInput["options"];
}

// Selections for one item being configured: single groups map group id →
// option id; multi groups map group id → option id list.
export type SingleSel = Record<string, string>;
export type MultiSel = Record<string, string[]>;

export function unitPriceCents(
  item: MenuItem,
  single: SingleSel,
  multi: MultiSel,
): number {
  let cents = item.price_cents;
  for (const g of item.option_groups) {
    if (g.select === "single") {
      const chosen = g.options.find((o) => o.id === single[g.id]) ?? g.options[0];
      cents += chosen?.price_delta_cents ?? 0;
    } else {
      for (const o of g.options) {
        if (multi[g.id]?.includes(o.id)) cents += o.price_delta_cents;
      }
    }
  }
  return cents;
}

/** "300 g · medium rare · béarnaise" — chosen option labels, lowercased. */
export function lineDetail(
  item: MenuItem,
  single: SingleSel,
  multi: MultiSel,
): string {
  const parts: string[] = [];
  for (const g of item.option_groups) {
    if (g.select === "single") {
      const chosen = g.options.find((o) => o.id === single[g.id]) ?? g.options[0];
      if (chosen) parts.push(chosen.label.toLowerCase());
    } else {
      for (const o of g.options) {
        if (multi[g.id]?.includes(o.id)) parts.push(o.label.toLowerCase());
      }
    }
  }
  return parts.join(" · ");
}

export function lineOptions(
  item: MenuItem,
  single: SingleSel,
  multi: MultiSel,
): OrderLineInput["options"] {
  const out: OrderLineInput["options"] = [];
  for (const g of item.option_groups) {
    if (g.select === "single") {
      const chosen = g.options.find((o) => o.id === single[g.id]) ?? g.options[0];
      if (chosen) out.push({ group_id: g.id, option_ids: [chosen.id] });
    } else {
      const ids = g.options.filter((o) => multi[g.id]?.includes(o.id)).map((o) => o.id);
      if (ids.length) out.push({ group_id: g.id, option_ids: ids });
    }
  }
  return out;
}

/** "from" pricing: a single-select group can raise the base price. */
export function hasFromPrice(item: MenuItem): boolean {
  return item.option_groups.some(
    (g) => g.select === "single" && g.options.some((o) => o.price_delta_cents > 0),
  );
}

const cartKey = (token: string) => `aivo:cart:${token}`;
const sentKey = (token: string) => `aivo:sent-at:${token}`;

export function loadCart(token: string): CartLine[] {
  try {
    return JSON.parse(sessionStorage.getItem(cartKey(token)) ?? "[]");
  } catch {
    return [];
  }
}

export function saveCart(token: string, lines: CartLine[]): void {
  try {
    sessionStorage.setItem(cartKey(token), JSON.stringify(lines));
  } catch {
    // storage full/blocked — cart just won't survive refresh
  }
}

// Active handoff + the cart lines it holds, so an expired-unused code can
// put the cart back (and a refresh returns to the code screen).
export interface StoredHandoff {
  handoff: Handoff;
  backup: CartLine[];
}

const handoffKey = (token: string) => `aivo:handoff:${token}`;

export function loadHandoff(token: string): StoredHandoff | null {
  try {
    const raw = sessionStorage.getItem(handoffKey(token));
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

export function saveHandoff(token: string, stored: StoredHandoff): void {
  try {
    sessionStorage.setItem(handoffKey(token), JSON.stringify(stored));
  } catch {
    // ignore
  }
}

export function clearHandoff(token: string): void {
  try {
    sessionStorage.removeItem(handoffKey(token));
  } catch {
    // ignore
  }
}

export function handoffExpired(stored: StoredHandoff, now: number): boolean {
  return now >= new Date(stored.handoff.expires_at).getTime();
}

export function loadSentAt(token: string): number | null {
  const v = Number(sessionStorage.getItem(sentKey(token)));
  return v > 0 ? v : null;
}

export function saveSentAt(token: string, t: number): void {
  try {
    sessionStorage.setItem(sentKey(token), String(t));
  } catch {
    // ignore
  }
}
