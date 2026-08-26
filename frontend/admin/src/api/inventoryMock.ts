// In-memory inventory mock — nomenclature, calendar-versioned tech-cards,
// perpetual weighted-average stock, and stock documents (impl-contract-2).
// Illustrative: mirrors the backend's moving-average mechanics (§5) and the
// draft→posted→cancelled=reversal lifecycle (§6) so every admin screen has
// believable, internally consistent data. GL posting is a backend concern and
// is not modelled here. The source of truth is `stockMoves`; `onHand` is a
// cache that must equal the fold (invariant enforced by construction).
import { ApiError } from "../../../design-system/shared/api";
import type {
  BaseUnit,
  ConsumptionStrategy,
  DocLineInput,
  FoodCostReport,
  FoodCostRow,
  GoodsReceipt,
  GoodsReceiptInput,
  GoodsReceiptLine,
  OnHand,
  Product,
  ProductInput,
  RecipeCosting,
  StockMove,
  Stocktake,
  StocktakePreview,
  Supplier,
  TechCard,
  TechCardFormat,
  TechCardInput,
  TechCardVersion,
  WriteOff,
  WriteOffInput,
  WriteOffLine,
} from "./types";
import { compatible, toBaseMilli } from "../lib/units";

const TODAY = "2026-08-24";

let seq = 1;
const uid = (p: string) => `${p}-${seq++}`;
const last = <T>(a: T[]): T | undefined => a[a.length - 1];
const notFound = () => new ApiError(404, "not_found", "Not found.");
const err = (status: number, code: string, msg: string) => new ApiError(status, code, msg);

async function delay<T>(v: T): Promise<T> {
  await new Promise((r) => setTimeout(r, 100));
  return v;
}

// ── Nomenclature ──────────────────────────────────────────────────────────

const products: Product[] = [
  mk("BEEF-RIB", "Ribeye beef", "goods", "g"),
  mk("POTATO", "Potatoes", "goods", "g", 5_000_000),
  mk("BUTTER", "Butter", "goods", "g", 2_000_000),
  mk("OIL-OLIVE", "Olive oil", "goods", "ml"),
  mk("SALT", "Sea salt", "goods", "g"),
  mk("CHICKEN", "Whole chicken", "goods", "pcs", 10_000),
  mk("CABBAGE", "Hispi cabbage", "goods", "pcs"),
  mk("FLATBREAD", "Grilled flatbread", "prepared", "pcs"),
  mk("DISH-RIBEYE", "Dry-aged ribeye (plate)", "dish", "pcs", null, "item-ribeye"),
  mk("DISH-CHIPS", "Triple-cooked chips (plate)", "dish", "pcs", null, "item-chips"),
  mk("DISH-CHICKEN", "Half chicken (plate)", "dish", "pcs", null, "item-chicken"),
];

function mk(
  sku: string,
  name: string,
  type: Product["type"],
  stock_unit: BaseUnit,
  min_stock: number | null = null,
  menu_item_id: string | null = null,
): Product {
  return { id: uid("prod"), sku, name, type, stock_unit, menu_item_id, min_stock, archived: false };
}

const productById = (id: string) => products.find((p) => p.id === id);
const bySku = (sku: string) => products.find((p) => p.sku === sku)!.id;

// ── Perpetual stock: moving average + material on-hand cache ─────────────────

interface OnHandState {
  qty: number; // milli base-units, signed
  value_cents: number;
  last_avg_cents: number; // cents per base-unit, last positive
}
const onHand = new Map<string, OnHandState>();
const on = (pid: string): OnHandState => {
  let s = onHand.get(pid);
  if (!s) onHand.set(pid, (s = { qty: 0, value_cents: 0, last_avg_cents: 0 }));
  return s;
};

// Richer than the public StockMove: carries menu_item_id for the food-cost report.
interface Move extends StockMove {
  menu_item_id?: string;
}
const stockMoves: Move[] = [];

// Apply a signed movement to the on-hand cache. Σ moves == cache by construction.
function apply(pid: string, signedQty: number, signedCost: number) {
  const s = on(pid);
  s.qty += signedQty;
  s.value_cents += signedCost;
  if (s.qty > 0) s.last_avg_cents = Math.round((s.value_cents * 1000) / s.qty);
}

// Cost of consuming `qMilli` at the current weighted average (§5 mechanics).
function consumeCost(pid: string, qMilli: number): { cost: number; estimated: boolean } {
  const s = on(pid);
  const estimated = qMilli > s.qty || s.qty <= 0;
  const unit = s.qty > 0 ? (s.value_cents * 1000) / s.qty : s.last_avg_cents;
  return { cost: Math.round((unit * qMilli) / 1000), estimated };
}

function record(m: Omit<Move, "id" | "recorded_at" | "unit"> & { recorded_at?: string }): Move {
  const p = productById(m.product_id)!;
  const move: Move = {
    ...m,
    id: uid("mv"),
    unit: p.stock_unit,
    product_name: p.name,
    recorded_at: m.recorded_at ?? m.business_date + "T12:00:00Z",
  };
  stockMoves.push(move);
  apply(move.product_id, move.qty, move.cost_cents);
  return move;
}

// Latest posted business_date for a product — for the backdate guard (§5.4).
function lastMoveDate(pid: string): string | null {
  const dates = stockMoves.filter((m) => m.product_id === pid).map((m) => m.business_date);
  return dates.length ? last(dates.sort())! : null;
}
function assertNotBackdated(lines: { product_id: string }[], businessDate: string) {
  for (const l of lines) {
    const last = lastMoveDate(l.product_id);
    if (last && businessDate < last) {
      const p = productById(l.product_id);
      throw err(
        422,
        "backdated_before_last_move",
        `${p?.name ?? l.product_id}: date is before its last stock move (${last}).`,
      );
    }
  }
}

function baseMilli(line: DocLineInput, pid: string): number {
  const p = productById(pid);
  if (!p) throw notFound();
  if (!compatible(line.unit, p.stock_unit))
    throw err(422, "unit_incompatible", `${line.unit} is not compatible with ${p.name} (${p.stock_unit}).`);
  return toBaseMilli(line.qty, line.unit);
}

// ── Tech-cards (calendar-versioned) ─────────────────────────────────────────

interface CardVersion {
  id: string;
  product_id: string;
  valid_from: string;
  valid_to: string | null;
  consumption: ConsumptionStrategy;
  yield_qty: number | null;
  lines: { id: string; ingredient_product_id: string; qty: number; seq: number; yield_permille: number }[];
  costings: RecipeCosting[];
  format: TechCardFormat;
  scope_note: string | null;
  presentation_note: string | null;
  storage_note: string | null;
  organoleptic_note: string | null;
}
const cards: CardVersion[] = [];

function activeVersion(pid: string, date: string): CardVersion | undefined {
  return cards.find(
    (c) => c.product_id === pid && c.valid_from <= date && (c.valid_to === null || c.valid_to > date),
  );
}

// Recursive per-base-unit cost of an ingredient on a date (§4.4).
function unitCost(pid: string, date: string, seen: Set<string>): number {
  const p = productById(pid);
  if (!p) return 0;
  const s = on(pid);
  const avg = s.qty > 0 ? (s.value_cents * 1000) / s.qty : s.last_avg_cents;
  if (p.type === "goods" || p.type === "modifier") return avg;
  // prepared / dish: recurse into its active card (cycle-guard keeps this finite).
  if (seen.has(pid)) return avg;
  seen.add(pid);
  const card = activeVersion(pid, date);
  if (!card) return avg;
  const total = recipeCost(card, date, seen);
  const yieldBase = card.yield_qty && card.yield_qty > 0 ? card.yield_qty / 1000 : 1;
  return total / yieldBase;
}

function recipeCost(card: CardVersion, date: string, seen = new Set<string>()): number {
  return card.lines.reduce(
    (sum, l) => sum + Math.round((unitCost(l.ingredient_product_id, date, new Set(seen)) * l.qty) / 1000),
    0,
  );
}

function pushCosting(card: CardVersion, date: string): RecipeCosting {
  const c: RecipeCosting = {
    id: uid("rc"),
    cost_cents: recipeCost(card, date),
    method: "weighted_avg",
    computed_at: date + "T12:00:00Z",
  };
  card.costings.push(c);
  return c;
}

// DFS cycle guard: is `target` reachable from the proposed ingredient set? (§4)
function wouldCycle(productId: string, ingredientIds: string[], date: string): boolean {
  const stack = [...ingredientIds];
  const visited = new Set<string>();
  while (stack.length) {
    const cur = stack.pop()!;
    if (cur === productId) return true;
    if (visited.has(cur)) continue;
    visited.add(cur);
    const card = activeVersion(cur, date);
    if (card) stack.push(...card.lines.map((l) => l.ingredient_product_id));
  }
  return false;
}

function toCardView(c: CardVersion): TechCard {
  return {
    id: c.id,
    product_id: c.product_id,
    valid_from: c.valid_from,
    valid_to: c.valid_to,
    consumption: c.consumption,
    yield_qty: c.yield_qty,
    lines: c.lines.map((l) => ({
      id: l.id,
      ingredient_product_id: l.ingredient_product_id,
      ingredient_name: productById(l.ingredient_product_id)?.name,
      qty: l.qty,
      unit: (productById(l.ingredient_product_id)?.stock_unit ?? "g") as BaseUnit,
      seq: l.seq,
      yield_permille: l.yield_permille,
      net_qty: Math.round((l.qty * l.yield_permille) / 1000),
    })),
    cost_cents: last(c.costings)?.cost_cents ?? 0,
    cost_history: [...c.costings].reverse(),
    format: c.format,
    scope_note: c.scope_note,
    presentation_note: c.presentation_note,
    storage_note: c.storage_note,
    organoleptic_note: c.organoleptic_note,
  };
}

function createCard(pid: string, input: TechCardInput): CardVersion {
  const p = productById(pid);
  if (!p) throw notFound();
  if (input.lines.length === 0) throw err(422, "empty_recipe", "A tech-card needs at least one ingredient.");
  const ingIds = input.lines.map((l) => l.ingredient_product_id);
  if (new Set(ingIds).size !== ingIds.length)
    throw err(422, "duplicate_ingredient", "Each ingredient may appear once.");
  for (const l of input.lines) {
    const ing = productById(l.ingredient_product_id);
    if (!ing) throw notFound();
    if (!compatible(l.unit, ing.stock_unit))
      throw err(422, "unit_incompatible", `${l.unit} is not compatible with ${ing.name}.`);
  }
  if (cards.some((c) => c.product_id === pid && c.valid_from === input.valid_from))
    throw err(409, "version_exists", `A version already starts on ${input.valid_from}.`);
  if (wouldCycle(pid, ingIds, input.valid_from)) throw err(422, "recipe_cycle", "This recipe would form a cycle.");

  // Close the version active on valid_from at midnight of valid_from (§4, D5).
  const prev = activeVersion(pid, input.valid_from);
  if (prev) prev.valid_to = input.valid_from;

  const card: CardVersion = {
    id: uid("tc"),
    product_id: pid,
    valid_from: input.valid_from,
    valid_to: null,
    consumption: input.consumption,
    yield_qty: input.yield_qty ?? null,
    lines: input.lines.map((l, i) => ({
      id: uid("tcl"),
      ingredient_product_id: l.ingredient_product_id,
      qty: toBaseMilli(l.qty, l.unit),
      seq: i,
      yield_permille: l.yield_permille && l.yield_permille > 0 ? l.yield_permille : 1000,
    })),
    costings: [],
    format: input.format ?? "simple",
    scope_note: input.scope_note ?? null,
    presentation_note: input.presentation_note ?? null,
    storage_note: input.storage_note ?? null,
    organoleptic_note: input.organoleptic_note ?? null,
  };
  cards.push(card);
  pushCosting(card, input.valid_from);
  return card;
}

// ── Suppliers ───────────────────────────────────────────────────────────────

const suppliers: Supplier[] = [
  { id: uid("sup"), name: "Smithfield Meats", contacts: { phone: "+44 20 7000 1122" }, archived: false },
  { id: uid("sup"), name: "Covent Garden Produce", contacts: { email: "orders@cgp.co.uk" }, archived: false },
];

// ── Documents ────────────────────────────────────────────────────────────────

const receipts: GoodsReceipt[] = [];
const writeoffs: WriteOff[] = [];
const stocktakes: Stocktake[] = [];

function receiptTotal(lines: GoodsReceiptLine[]) {
  return lines.reduce((a, l) => a + l.line_cost_cents, 0);
}

function buildReceipt(input: GoodsReceiptInput): GoodsReceipt {
  if (input.lines.length === 0) throw err(422, "empty_document", "Add at least one line.");
  const lines: GoodsReceiptLine[] = input.lines.map((l, i) => {
    const qtyBase = baseMilli(l, l.product_id);
    const price = l.unit_price_cents ?? 0;
    return {
      id: uid("grl"),
      product_id: l.product_id,
      product_name: productById(l.product_id)?.name,
      qty_input: l.qty,
      input_unit: l.unit,
      qty_base: qtyBase,
      unit_price_cents: price,
      line_cost_cents: Math.round(price * l.qty),
      seq: i,
    };
  });
  const sup = input.supplier_id ? suppliers.find((s) => s.id === input.supplier_id) : null;
  return {
    id: uid("gr"),
    supplier_id: input.supplier_id ?? null,
    supplier_name: sup?.name ?? null,
    status: "draft",
    business_date: input.business_date,
    note: input.note,
    posted_at: null,
    reversal_of: null,
    total_cents: receiptTotal(lines),
    lines,
  };
}

function postReceipt(r: GoodsReceipt) {
  if (r.status === "cancelled") throw err(409, "already_cancelled", "Document is cancelled.");
  if (r.status === "posted") throw err(409, "already_posted", "Document already posted.");
  assertNotBackdated(r.lines, r.business_date);
  for (const l of r.lines)
    record({ product_id: l.product_id, kind: "receipt", qty: l.qty_base, cost_cents: l.line_cost_cents, estimated: false, business_date: r.business_date, doc_kind: "inventory_receipt", doc_id: r.id });
  r.status = "posted";
  r.posted_at = TODAY + "T09:30:00Z";
}

function buildWriteOff(input: WriteOffInput): WriteOff {
  if (input.lines.length === 0) throw err(422, "empty_document", "Add at least one line.");
  const lines: WriteOffLine[] = input.lines.map((l, i) => ({
    id: uid("wol"),
    product_id: l.product_id,
    product_name: productById(l.product_id)?.name,
    qty_input: l.qty,
    input_unit: l.unit,
    qty_base: baseMilli(l, l.product_id),
    cost_cents: 0,
    seq: i,
  }));
  return {
    id: uid("wo"),
    reason: input.reason,
    note: input.note,
    status: "draft",
    business_date: input.business_date,
    posted_at: null,
    reversal_of: null,
    total_cents: 0,
    lines,
  };
}

function postWriteOff(w: WriteOff) {
  if (w.status === "cancelled") throw err(409, "already_cancelled", "Document is cancelled.");
  if (w.status === "posted") throw err(409, "already_posted", "Document already posted.");
  assertNotBackdated(w.lines, w.business_date);
  let total = 0;
  for (const l of w.lines) {
    const { cost, estimated } = consumeCost(l.product_id, l.qty_base);
    l.cost_cents = cost;
    total += cost;
    record({ product_id: l.product_id, kind: "writeoff", qty: -l.qty_base, cost_cents: -cost, estimated, business_date: w.business_date, doc_kind: "inventory_writeoff", doc_id: w.id });
  }
  w.total_cents = total;
  w.status = "posted";
  w.posted_at = TODAY + "T09:30:00Z";
}

// Reverse every posted move of a document (mirror qty/cost of the originals).
function reverseMoves(docId: string, docKind: string) {
  const originals = stockMoves.filter((m) => m.doc_id === docId && m.kind !== "reversal");
  for (const m of originals)
    record({ product_id: m.product_id, kind: "reversal", qty: -m.qty, cost_cents: -m.cost_cents, estimated: m.estimated, business_date: TODAY, doc_kind: docKind, doc_id: docId + "-rev" });
}

// Compute expected/variance for stocktake lines against current on-hand.
function evalStocktake(st: Stocktake): { total: number } {
  let total = 0;
  for (const l of st.lines) {
    const expected = on(l.product_id).qty;
    const variance = l.counted_qty - expected;
    l.expected_qty = expected;
    l.variance_qty = variance;
    const avg = on(l.product_id);
    const unit = avg.qty > 0 ? (avg.value_cents * 1000) / avg.qty : avg.last_avg_cents;
    l.variance_cost_cents = Math.round((unit * variance) / 1000);
    total += l.variance_cost_cents;
  }
  return { total };
}

// ── Seed initial state: receipts in, tech-cards, then a week of sales ────────

function seed() {
  post(buildReceipt({
    supplier_id: suppliers[0].id,
    business_date: "2026-08-10",
    note: "Opening stock — meat",
    lines: [
      { product_id: bySku("BEEF-RIB"), qty: 20, unit: "kg", unit_price_cents: 3200 }, // 32.00/kg
      { product_id: bySku("CHICKEN"), qty: 40, unit: "pcs", unit_price_cents: 450 },
      { product_id: bySku("BUTTER"), qty: 5, unit: "kg", unit_price_cents: 900 },
    ],
  }));
  post(buildReceipt({
    supplier_id: suppliers[1].id,
    business_date: "2026-08-10",
    note: "Opening stock — produce",
    lines: [
      { product_id: bySku("POTATO"), qty: 50, unit: "kg", unit_price_cents: 120 },
      { product_id: bySku("OIL-OLIVE"), qty: 10, unit: "l", unit_price_cents: 800 },
      { product_id: bySku("SALT"), qty: 5, unit: "kg", unit_price_cents: 200 },
      { product_id: bySku("CABBAGE"), qty: 30, unit: "pcs", unit_price_cents: 140 },
      { product_id: bySku("FLATBREAD"), qty: 24, unit: "pcs", unit_price_cents: 90 },
    ],
  }));
  function post(r: GoodsReceipt) {
    postReceipt(r);
    receipts.push(r);
  }

  // Tech-cards, active from 2026-08-11.
  createCard(bySku("DISH-RIBEYE"), {
    valid_from: "2026-08-11",
    consumption: "assemble",
    yield_qty: 1000,
    lines: [
      { ingredient_product_id: bySku("BEEF-RIB"), qty: 320, unit: "g" },
      { ingredient_product_id: bySku("BUTTER"), qty: 20, unit: "g" },
      { ingredient_product_id: bySku("SALT"), qty: 4, unit: "g" },
    ],
  });
  createCard(bySku("DISH-CHIPS"), {
    valid_from: "2026-08-11",
    consumption: "assemble",
    yield_qty: 1000,
    lines: [
      { ingredient_product_id: bySku("POTATO"), qty: 250, unit: "g" },
      { ingredient_product_id: bySku("OIL-OLIVE"), qty: 30, unit: "ml" },
      { ingredient_product_id: bySku("SALT"), qty: 3, unit: "g" },
    ],
  });
  createCard(bySku("DISH-CHICKEN"), {
    valid_from: "2026-08-11",
    consumption: "assemble",
    yield_qty: 1000,
    lines: [
      { ingredient_product_id: bySku("CHICKEN"), qty: 1, unit: "pcs" },
      { ingredient_product_id: bySku("OIL-OLIVE"), qty: 20, unit: "ml" },
    ],
  });

  // A second ribeye version from 2026-08-18 (yield tweak) — timeline has ≥2 rows.
  createCard(bySku("DISH-RIBEYE"), {
    valid_from: "2026-08-18",
    consumption: "assemble",
    yield_qty: 1000,
    lines: [
      { ingredient_product_id: bySku("BEEF-RIB"), qty: 300, unit: "g" },
      { ingredient_product_id: bySku("BUTTER"), qty: 25, unit: "g" },
      { ingredient_product_id: bySku("SALT"), qty: 4, unit: "g" },
    ],
  });

  // A week of sales → genuine sale moves + revenue log for the food-cost report.
  sell("item-ribeye", bySku("DISH-RIBEYE"), "Dry-aged ribeye", 4600, 6, "2026-08-15");
  sell("item-ribeye", bySku("DISH-RIBEYE"), "Dry-aged ribeye", 4600, 9, "2026-08-20");
  sell("item-chips", bySku("DISH-CHIPS"), "Triple-cooked chips", 900, 14, "2026-08-16");
  sell("item-chips", bySku("DISH-CHIPS"), "Triple-cooked chips", 900, 11, "2026-08-21");
  sell("item-chicken", bySku("DISH-CHICKEN"), "Half chicken, brined", 2800, 7, "2026-08-19");
}

interface Sale {
  menu_item_id: string;
  name: string;
  qty: number;
  revenue_cents: number;
  business_date: string;
}
const sales: Sale[] = [];

// Expand a dish's active card and consume ingredients at the moving average (§7).
function sell(menuItemId: string, dishPid: string, name: string, unitPrice: number, qty: number, date: string) {
  sales.push({ menu_item_id: menuItemId, name, qty, revenue_cents: unitPrice * qty, business_date: date });
  const card = activeVersion(dishPid, date);
  if (!card) return;
  if (card.consumption === "deplete_finished") {
    consumeOne(dishPid, qty * 1000, menuItemId, date, dishPid);
    return;
  }
  for (const l of card.lines) consumeOne(l.ingredient_product_id, l.qty * qty, menuItemId, date, dishPid);
}
function consumeOne(pid: string, qMilli: number, menuItemId: string, date: string, dishPid: string) {
  const { cost, estimated } = consumeCost(pid, qMilli);
  const m = record({ product_id: pid, kind: "sale", qty: -qMilli, cost_cents: -cost, estimated, business_date: date, doc_kind: "cogs", doc_id: "ticket-" + dishPid + "-" + date });
  m.menu_item_id = menuItemId;
}

seed();

// ── Public API ────────────────────────────────────────────────────────────

function onHandRow(p: Product): OnHand {
  const s = on(p.id);
  return {
    product_id: p.id,
    sku: p.sku,
    name: p.name,
    qty: s.qty,
    unit: p.stock_unit,
    value_cents: s.value_cents,
    avg_cents: s.qty > 0 ? Math.round((s.value_cents * 1000) / s.qty) : s.last_avg_cents,
    below_min: p.min_stock != null && s.qty < p.min_stock,
  };
}

export const inventoryMock = {
  // Products
  listProducts(): Promise<Product[]> {
    return delay(products.map((p) => ({ ...p })));
  },
  getProduct(pid: string): Promise<Product> {
    const p = productById(pid);
    if (!p) throw notFound();
    const s = on(pid);
    return delay({
      ...p,
      has_moves: stockMoves.some((m) => m.product_id === pid),
      on_hand: {
        qty: s.qty,
        unit: p.stock_unit,
        value_cents: s.value_cents,
        avg_cents: s.qty > 0 ? Math.round((s.value_cents * 1000) / s.qty) : s.last_avg_cents,
      },
    });
  },
  createProduct(input: ProductInput): Promise<Product> {
    if (products.some((p) => p.sku.toLowerCase() === input.sku.toLowerCase()))
      throw err(422, "sku_taken", `SKU "${input.sku}" is already in use.`);
    if (input.menu_item_id) {
      if (input.type !== "dish") throw err(422, "unit_incompatible", "Only dishes link to a menu item.");
      if (products.some((p) => p.menu_item_id === input.menu_item_id))
        throw err(409, "menu_item_taken", "That menu item is already linked to a product.");
    }
    const p: Product = {
      id: uid("prod"),
      sku: input.sku,
      name: input.name,
      type: input.type,
      stock_unit: input.stock_unit,
      menu_item_id: input.type === "dish" ? input.menu_item_id ?? null : null,
      min_stock: input.min_stock ?? null,
      archived: false,
    };
    products.push(p);
    return delay({ ...p });
  },
  updateProduct(pid: string, patch: Partial<ProductInput> & { archived?: boolean }): Promise<Product> {
    const p = productById(pid);
    if (!p) throw notFound();
    if (patch.menu_item_id !== undefined && patch.menu_item_id) {
      if (products.some((x) => x.id !== pid && x.menu_item_id === patch.menu_item_id))
        throw err(409, "menu_item_taken", "That menu item is already linked to a product.");
    }
    if (patch.stock_unit && patch.stock_unit !== p.stock_unit && stockMoves.some((m) => m.product_id === pid))
      throw err(422, "unit_locked", "Stock unit is locked after the first movement.");
    if (patch.name !== undefined) p.name = patch.name;
    if (patch.min_stock !== undefined) p.min_stock = patch.min_stock;
    if (patch.menu_item_id !== undefined) p.menu_item_id = patch.menu_item_id;
    if (patch.stock_unit !== undefined) p.stock_unit = patch.stock_unit;
    if (patch.archived !== undefined) p.archived = patch.archived;
    return delay({ ...p });
  },

  // Tech-cards
  listTechCards(pid: string): Promise<TechCardVersion[]> {
    const rows = cards
      .filter((c) => c.product_id === pid)
      .sort((a, b) => a.valid_from.localeCompare(b.valid_from))
      .map((c) => ({
        id: c.id,
        valid_from: c.valid_from,
        valid_to: c.valid_to,
        consumption: c.consumption,
        cost_cents: last(c.costings)?.cost_cents ?? 0,
      }));
    return delay(rows);
  },
  activeTechCard(pid: string, on: string): Promise<TechCard | null> {
    const c = activeVersion(pid, on);
    return delay(c ? toCardView(c) : null);
  },
  getTechCard(tcid: string): Promise<TechCard> {
    const c = cards.find((x) => x.id === tcid);
    if (!c) throw notFound();
    return delay(toCardView(c));
  },
  createTechCard(pid: string, input: TechCardInput): Promise<TechCard> {
    return delay(toCardView(createCard(pid, input)));
  },
  recost(tcid: string): Promise<TechCard> {
    const c = cards.find((x) => x.id === tcid);
    if (!c) throw notFound();
    pushCosting(c, TODAY);
    return delay(toCardView(c));
  },

  // Suppliers
  listSuppliers(): Promise<Supplier[]> {
    return delay(suppliers.map((s) => ({ ...s })));
  },
  createSupplier(input: { name: string; contacts?: Record<string, string>; note?: string }): Promise<Supplier> {
    if (suppliers.some((s) => s.name.toLowerCase() === input.name.toLowerCase()))
      throw err(409, "supplier_name_taken", "A supplier with that name already exists.");
    const s: Supplier = { id: uid("sup"), name: input.name, contacts: input.contacts ?? {}, note: input.note, archived: false };
    suppliers.push(s);
    return delay({ ...s });
  },
  updateSupplier(sid: string, patch: Partial<Pick<Supplier, "name" | "contacts" | "archived">>): Promise<Supplier> {
    const s = suppliers.find((x) => x.id === sid);
    if (!s) throw notFound();
    if (patch.name && suppliers.some((x) => x.id !== sid && x.name.toLowerCase() === patch.name!.toLowerCase()))
      throw err(409, "supplier_name_taken", "A supplier with that name already exists.");
    Object.assign(s, patch);
    return delay({ ...s });
  },

  // Receipts
  listReceipts(status?: string): Promise<GoodsReceipt[]> {
    return delay(receipts.filter((r) => !status || r.status === status).map((r) => ({ ...r })).reverse());
  },
  getReceipt(rid: string): Promise<GoodsReceipt> {
    const r = receipts.find((x) => x.id === rid);
    if (!r) throw notFound();
    return delay({ ...r });
  },
  createReceipt(input: GoodsReceiptInput): Promise<GoodsReceipt> {
    const r = buildReceipt(input);
    receipts.push(r);
    return delay({ ...r });
  },
  postReceipt(rid: string): Promise<GoodsReceipt> {
    const r = receipts.find((x) => x.id === rid);
    if (!r) throw notFound();
    postReceipt(r);
    return delay({ ...r });
  },
  cancelReceipt(rid: string): Promise<GoodsReceipt> {
    const r = receipts.find((x) => x.id === rid);
    if (!r) throw notFound();
    if (r.status !== "posted") throw err(409, "not_posted", "Only posted documents can be cancelled.");
    reverseMoves(r.id, "inventory_receipt");
    r.status = "cancelled";
    r.reversal_of = null;
    return delay({ ...r });
  },

  // Write-offs
  listWriteOffs(status?: string): Promise<WriteOff[]> {
    return delay(writeoffs.filter((w) => !status || w.status === status).map((w) => ({ ...w })).reverse());
  },
  getWriteOff(wid: string): Promise<WriteOff> {
    const w = writeoffs.find((x) => x.id === wid);
    if (!w) throw notFound();
    return delay({ ...w });
  },
  createWriteOff(input: WriteOffInput): Promise<WriteOff> {
    const w = buildWriteOff(input);
    writeoffs.push(w);
    return delay({ ...w });
  },
  postWriteOff(wid: string): Promise<WriteOff> {
    const w = writeoffs.find((x) => x.id === wid);
    if (!w) throw notFound();
    postWriteOff(w);
    return delay({ ...w });
  },
  cancelWriteOff(wid: string): Promise<WriteOff> {
    const w = writeoffs.find((x) => x.id === wid);
    if (!w) throw notFound();
    if (w.status !== "posted") throw err(409, "not_posted", "Only posted documents can be cancelled.");
    reverseMoves(w.id, "inventory_writeoff");
    w.status = "cancelled";
    return delay({ ...w });
  },

  // Stocktakes
  listStocktakes(status?: string): Promise<Stocktake[]> {
    return delay(stocktakes.filter((s) => !status || s.status === status).map((s) => ({ ...s })).reverse());
  },
  getStocktake(sid: string): Promise<Stocktake> {
    const s = stocktakes.find((x) => x.id === sid);
    if (!s) throw notFound();
    return delay({ ...s });
  },
  createStocktake(input: { note?: string }): Promise<Stocktake> {
    if (stocktakes.some((s) => s.status === "draft"))
      throw err(409, "stocktake_open_exists", "An open stocktake already exists.");
    const s: Stocktake = { id: uid("st"), status: "draft", note: input.note, business_date: null, posted_at: null, reversal_of: null, lines: [] };
    stocktakes.push(s);
    return delay({ ...s });
  },
  patchStocktake(sid: string, lines: DocLineInput[]): Promise<Stocktake> {
    const s = stocktakes.find((x) => x.id === sid);
    if (!s) throw notFound();
    if (s.status !== "draft") throw err(409, "already_posted", "Counts can only be entered on a draft.");
    s.lines = lines.map((l) => ({
      id: uid("stl"),
      product_id: l.product_id,
      product_name: productById(l.product_id)?.name,
      unit: (productById(l.product_id)?.stock_unit ?? "g") as BaseUnit,
      counted_qty: baseMilli(l, l.product_id),
      expected_qty: null,
      variance_qty: null,
      variance_cost_cents: null,
    }));
    return delay({ ...s });
  },
  dryRunStocktake(sid: string): Promise<StocktakePreview> {
    const s = stocktakes.find((x) => x.id === sid);
    if (!s) throw notFound();
    // Read-only: compute against a throwaway copy, persist nothing (§6.4 refuted §15.2).
    const copy: Stocktake = { ...s, lines: s.lines.map((l) => ({ ...l })) };
    const { total } = evalStocktake(copy);
    return delay({ lines: copy.lines, total_variance_cost_cents: total });
  },
  postStocktake(sid: string): Promise<Stocktake> {
    const s = stocktakes.find((x) => x.id === sid);
    if (!s) throw notFound();
    if (s.status !== "draft") throw err(409, "already_posted", "Document already posted.");
    s.business_date = TODAY;
    evalStocktake(s); // fixes expected_qty + variance at post time
    for (const l of s.lines) {
      if (!l.variance_qty) continue;
      const surplus = l.variance_qty > 0;
      record({
        product_id: l.product_id,
        kind: surplus ? "stocktake_surplus" : "stocktake_shortage",
        qty: l.variance_qty,
        cost_cents: l.variance_cost_cents ?? 0,
        estimated: false,
        business_date: TODAY,
        doc_kind: "inventory_stocktake",
        doc_id: s.id,
      });
    }
    s.status = "posted";
    s.posted_at = TODAY + "T09:30:00Z";
    return delay({ ...s });
  },
  cancelStocktake(sid: string): Promise<Stocktake> {
    const s = stocktakes.find((x) => x.id === sid);
    if (!s) throw notFound();
    if (s.status !== "posted") throw err(409, "not_posted", "Only posted documents can be cancelled.");
    reverseMoves(s.id, "inventory_stocktake");
    s.status = "cancelled";
    return delay({ ...s });
  },

  // On-hand & moves
  onHandList(lowStock?: boolean): Promise<OnHand[]> {
    const rows = products
      .filter((p) => !p.archived && p.type !== "dish")
      .map(onHandRow)
      .filter((r) => !lowStock || r.below_min);
    return delay(rows);
  },
  stockMoveList(params: { from?: string; product?: string }): Promise<StockMove[]> {
    const rows = stockMoves
      .filter((m) => (!params.from || m.business_date >= params.from) && (!params.product || m.product_id === params.product))
      .map(({ menu_item_id, ...m }) => (void menu_item_id, m))
      .sort((a, b) => b.recorded_at.localeCompare(a.recorded_at));
    return delay(rows);
  },

  // Food cost report
  foodCost(from: string, to: string): Promise<FoodCostReport> {
    const inRange = (d: string) => d >= from && d <= to;
    const saleMoves = stockMoves.filter((m) => m.kind === "sale" && inRange(m.business_date));
    const estimated = saleMoves.filter((m) => m.estimated).length;
    const estimated_share = saleMoves.length ? estimated / saleMoves.length : 0;

    const byItem = new Map<string, FoodCostRow>();
    for (const sale of sales.filter((s) => inRange(s.business_date))) {
      const row = byItem.get(sale.menu_item_id) ?? {
        menu_item_id: sale.menu_item_id,
        name: sale.name,
        revenue_cents: 0,
        actual_cogs_cents: 0,
        theoretical_cogs_cents: 0,
        food_cost_pct: 0,
      };
      row.revenue_cents += sale.revenue_cents;
      // Theoretical = qty × recipe cost of the dish's card active on the sale date.
      const dish = products.find((p) => p.menu_item_id === sale.menu_item_id);
      const card = dish ? activeVersion(dish.id, sale.business_date) : undefined;
      row.theoretical_cogs_cents += (last(card?.costings ?? [])?.cost_cents ?? 0) * sale.qty;
      byItem.set(sale.menu_item_id, row);
    }
    // Actual = Σ |sale move cost| grouped by the move's menu item.
    for (const m of saleMoves) {
      if (!m.menu_item_id) continue;
      const row = byItem.get(m.menu_item_id);
      if (row) row.actual_cogs_cents += Math.abs(m.cost_cents);
    }
    const items = [...byItem.values()].map((r) => ({
      ...r,
      food_cost_pct: r.revenue_cents ? r.actual_cogs_cents / r.revenue_cents : 0,
    }));
    const totals = items.reduce(
      (t, r) => ({
        revenue_cents: t.revenue_cents + r.revenue_cents,
        actual_cogs_cents: t.actual_cogs_cents + r.actual_cogs_cents,
        theoretical_cogs_cents: t.theoretical_cogs_cents + r.theoretical_cogs_cents,
        food_cost_pct: 0,
      }),
      { revenue_cents: 0, actual_cogs_cents: 0, theoretical_cogs_cents: 0, food_cost_pct: 0 },
    );
    totals.food_cost_pct = totals.revenue_cents ? totals.actual_cogs_cents / totals.revenue_cents : 0;
    return delay({ items, totals, estimated_share });
  },
};

// ponytail: invariant self-check — cache equals the fold of the move book (§5).
// Runs once at module load in dev; a broken apply() trips it immediately.
if (import.meta.env.DEV) {
  for (const p of products) {
    const qty = stockMoves.filter((m) => m.product_id === p.id).reduce((a, m) => a + m.qty, 0);
    const val = stockMoves.filter((m) => m.product_id === p.id).reduce((a, m) => a + m.cost_cents, 0);
    const s = on(p.id);
    if (qty !== s.qty || val !== s.value_cents)
      console.error(`[inventoryMock] fold mismatch for ${p.sku}: moves(${qty},${val}) != cache(${s.qty},${s.value_cents})`);
  }
}
