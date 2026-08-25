// Closed unit enum with a static conversion table (impl-contract-2 §3).
// Physical constants, not a runtime entity. Quantities live in the domain as
// milli base-units (g/ml/pcs); display units (kg/l) convert on the boundary.
import type { BaseUnit, Unit } from "../api/types";

type Dim = "mass" | "volume" | "count";

export const UNITS: Record<Unit, { dim: Dim; factor: number; base: BaseUnit }> = {
  g: { dim: "mass", factor: 1, base: "g" },
  kg: { dim: "mass", factor: 1000, base: "g" },
  ml: { dim: "volume", factor: 1, base: "ml" },
  l: { dim: "volume", factor: 1000, base: "ml" },
  pcs: { dim: "count", factor: 1, base: "pcs" },
};

// Display units offered for a base stock unit (its own + the bigger sibling).
export const DISPLAY_UNITS: Record<BaseUnit, Unit[]> = {
  g: ["g", "kg"],
  ml: ["ml", "l"],
  pcs: ["pcs"],
};

/** true when `unit` shares dimension with a base stock unit. */
export function compatible(unit: Unit, stockUnit: BaseUnit): boolean {
  return UNITS[unit].dim === UNITS[stockUnit].dim;
}

/** display qty in `unit` → milli base-units. */
export function toBaseMilli(qty: number, unit: Unit): number {
  return Math.round(qty * UNITS[unit].factor * 1000);
}

/** milli base-units → display qty in `unit`. */
export function fromBaseMilli(milli: number, unit: Unit): number {
  return milli / 1000 / UNITS[unit].factor;
}

/** Human quantity in a base unit, e.g. 5000000 g-milli → "5000 g" (or "5 kg"). */
export function formatQty(milli: number, base: BaseUnit): string {
  if (base === "g" && Math.abs(milli) >= 1_000_000)
    return trim(milli / 1_000_000) + " kg";
  if (base === "ml" && Math.abs(milli) >= 1_000_000)
    return trim(milli / 1_000_000) + " l";
  return trim(milli / 1000) + " " + base;
}

function trim(n: number): string {
  return Number(n.toFixed(3)).toString();
}
