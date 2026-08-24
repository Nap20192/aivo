// Shapes per docs/PLATFORM.md "API surface (JSON, /api/v1)".

import type { AccentName } from "../../../design-system/shared/theme";

export type Role = "owner" | "manager" | "waiter";
export type Plan = "free" | "pro" | "business";
export type Accent = AccentName;

export interface User {
  id: string;
  email: string;
  role: Role;
}

export interface Org {
  id: string;
  name: string;
}

export interface Me {
  user: User;
  org: Org;
  restaurants: Restaurant[];
}

export interface HoursRow {
  label: string;
  open: string;
  close: string;
}

export interface Restaurant {
  id: string;
  org_id: string;
  slug: string;
  name: string;
  hours: HoursRow[];
  address: string;
  phone: string;
  instagram: string;
  custom_domain: string;
}

export interface Theme {
  brand_name: string;
  accent: Accent;
  bold: boolean;
  banner_url: string;
  css_vars: Record<string, string>;
  design_md: string;
}

export interface Menu {
  id: string;
  slug: string;
  name: string;
  position: number;
  is_default: boolean;
}

export interface Category {
  id: string;
  menu_id: string;
  name: string;
  position: number;
}

export interface OptionChoice {
  id: string;
  name: string;
  price_delta_cents: number;
}

export interface OptionGroup {
  id: string;
  name: string;
  type: "single" | "multi";
  choices: OptionChoice[];
}

export interface MenuItem {
  id: string;
  category_id: string;
  name: string;
  description: string;
  price_cents: number;
  image_url: string;
  allergens: string[];
  option_groups: OptionGroup[];
  available: boolean;
}

export interface Table {
  id: string;
  label: string;
  token: string;
}

export interface StaffMember {
  id: string;
  email: string;
  role: Role;
  status: "active" | "invited";
}

export interface Subscription {
  plan: Plan;
  status: "trialing" | "active" | "past_due" | "canceled";
  renews_at: string;
}

export interface ApiErrorBody {
  error: { code: string; message: string };
}

// Assistant actions — allowlist per the assistant contract. Anything else
// is rejected server-side; the client only renders and forwards them.
export type AssistantAction =
  | { type: "create_category"; menu_id: string; name: string }
  | { type: "rename_category"; id: string; name: string }
  | { type: "delete_category"; id: string }
  | {
      type: "create_item";
      category_id: string;
      name: string;
      description?: string;
      price_cents: number;
      allergens?: string[];
      image_url?: string;
    }
  | {
      type: "update_item";
      id: string;
      name?: string;
      description?: string;
      price_cents?: number;
      allergens?: string[];
      image_url?: string;
    }
  | { type: "delete_item"; id: string }
  | { type: "set_item_available"; id: string; available: boolean }
  | { type: "update_theme"; theme: Partial<Omit<Theme, "design_md">> }
  | { type: "create_menu"; name: string; slug: string };

export interface AssistantAttachment {
  name: string;
  url: string;
  mime: string;
}

export interface AssistantMessage {
  id: string;
  role: "user" | "assistant";
  text: string;
  attachments: AssistantAttachment[];
  actions: AssistantAction[];
  action_status: null | "applied" | "discarded";
  created_at: string;
}

// Backend shape: error omitted on success.
export interface AssistantApplyResult {
  index: number;
  type: string;
  ok: boolean;
  error?: string;
}

// Light CRM (manager+). Email/phone are manager-only server-side.
export interface GuestCustomer {
  id: string;
  name: string;
  email: string;
  phone: string | null;
}

export interface GuestSummary {
  customer: GuestCustomer;
  visits: number;
  total_spent_cents: number;
  last_seen: string;
  tags: string[];
}

export interface GuestOrderLine {
  name: string;
  qty: number;
  total_cents: number;
}

export interface GuestOrder {
  created_at: string;
  table_label: string;
  total_cents: number;
  lines: GuestOrderLine[];
}

export interface GuestDetail {
  customer: GuestCustomer;
  visits: number;
  total_spent_cents: number;
  first_seen: string;
  last_seen: string;
  notes: string;
  tags: string[];
  orders: GuestOrder[];
}

// ── Ledger / shift acceptance (docs/research/rms/impl-contract.md §4) ──

export type AccountType =
  | "asset"
  | "liability"
  | "revenue"
  | "expense"
  | "equity"
  | "statistical";
export type Side = "debit" | "credit";

export interface Account {
  id: string;
  code: string;
  name: string;
  type: AccountType;
  normal_side: Side;
  postable: boolean;
}

export interface CostCenter {
  id: string;
  code: string;
  name: string;
}

export interface AccountMapEntry {
  purpose: string;
  account_id: string;
  account_code: string;
}

export type JournalKind = "shift_acceptance" | "manual" | "reversal";
export type JournalState = "draft" | "posted" | "cancelled";
export type ShiftState = "closed" | "accepted";

export interface JournalLine {
  line_id?: string; // present on draft acceptance lines (override target)
  account_id: string;
  account_code: string;
  account_name?: string;
  side: Side;
  amount_cents: number;
  cost_center_id: string;
  memo?: string;
  editable?: boolean; // draft lines only
}

export interface JournalSummary {
  id: string;
  kind: JournalKind;
  state: JournalState;
  accounting_date: string; // YYYY-MM-DD
  recorded_at: string; // ISO
  source_kind: "shift" | "manual" | null;
  source_id: string | null;
  reversal_of: string | null;
  total_cents: number;
}

export interface JournalDocument {
  id: string;
  kind: JournalKind;
  state: JournalState;
  accounting_date: string;
  recorded_at: string;
  posted_at: string | null;
  cancelled_at: string | null;
  reversal_of: string | null;
  lines: JournalLine[];
}

export interface ShiftRow {
  id: string;
  number: string;
  cashier: string;
  opened_at: string;
  closed_at: string;
  accepted_at: string | null;
  state: ShiftState;
  expected_cents: number;
  declared_cents: number;
  variance_cents: number;
}

export interface ShiftAcceptance {
  shift: ShiftRow;
  document: {
    id: string;
    state: JournalState; // "draft"
    accounting_date: string;
    recorded_at: string;
    lines: JournalLine[];
  };
  variance_cents: number;
  balanced: boolean;
}

// PATCH acceptance body — per-line override of account / cost-center.
export interface AcceptanceOverride {
  line_id: string;
  account_id?: string;
  cost_center_id?: string;
}

// POST manual journal body.
export interface ManualJournalInput {
  accounting_date: string;
  memo: string;
  lines: {
    account_id: string;
    side: Side;
    amount_cents: number;
    cost_center_id?: string;
    memo?: string;
  }[];
}

// ── Inventory: stock & tech-cards (docs/research/rms/impl-contract-2.md §10) ──

// Base stock unit (g/ml/pcs). Display units (kg/l) convert on the wire — §3.
export type BaseUnit = "g" | "ml" | "pcs";
export type Unit = BaseUnit | "kg" | "l";
export type ProductType = "goods" | "dish" | "prepared" | "modifier";

export interface OnHand {
  product_id: string;
  sku: string;
  name: string;
  qty: number; // milli base-units, signed
  unit: BaseUnit;
  value_cents: number;
  avg_cents: number; // per base-unit
  below_min: boolean;
}

export interface Product {
  id: string;
  sku: string;
  name: string;
  type: ProductType;
  stock_unit: BaseUnit;
  menu_item_id: string | null;
  min_stock: number | null; // milli base-units
  archived: boolean;
  has_moves?: boolean; // set on detail: locks stock_unit
  on_hand?: { qty: number; unit: BaseUnit; value_cents: number; avg_cents: number };
}

export interface ProductInput {
  sku: string;
  name: string;
  type: ProductType;
  stock_unit: BaseUnit;
  menu_item_id?: string | null;
  min_stock?: number | null;
}

export type ConsumptionStrategy = "assemble" | "deplete_finished";

export interface TechCardLine {
  id: string;
  ingredient_product_id: string;
  ingredient_name?: string;
  qty: number; // milli base-units of ingredient
  unit: BaseUnit;
  seq: number;
}

export interface RecipeCosting {
  id: string;
  cost_cents: number;
  method: "weighted_avg";
  computed_at: string;
}

// Version row for the timeline (list endpoint).
export interface TechCardVersion {
  id: string;
  valid_from: string; // YYYY-MM-DD
  valid_to: string | null;
  consumption: ConsumptionStrategy;
  cost_cents: number;
}

// Full version with lines + cost history (detail / active endpoint).
export interface TechCard {
  id: string;
  product_id: string;
  valid_from: string;
  valid_to: string | null;
  consumption: ConsumptionStrategy;
  yield_qty: number | null;
  lines: TechCardLine[];
  cost_cents: number; // latest costing
  cost_history: RecipeCosting[];
}

export interface TechCardInput {
  valid_from: string;
  consumption: ConsumptionStrategy;
  yield_qty?: number | null;
  lines: { ingredient_product_id: string; qty: number; unit: Unit }[];
}

export interface Supplier {
  id: string;
  name: string;
  contacts: Record<string, string>;
  note?: string;
  archived: boolean;
}

export type DocStatus = "draft" | "posted" | "cancelled";

export interface DocLineInput {
  product_id: string;
  qty: number; // display quantity
  unit: Unit;
  unit_price_cents?: number; // receipts only
}

export interface GoodsReceiptLine {
  id: string;
  product_id: string;
  product_name?: string;
  qty_input: number;
  input_unit: Unit;
  qty_base: number; // milli base-units
  unit_price_cents: number;
  line_cost_cents: number;
  seq: number;
}

export interface GoodsReceipt {
  id: string;
  supplier_id: string | null;
  supplier_name?: string | null;
  status: DocStatus;
  business_date: string;
  note?: string;
  posted_at: string | null;
  reversal_of: string | null;
  total_cents: number;
  lines: GoodsReceiptLine[];
}

export interface GoodsReceiptInput {
  supplier_id?: string | null;
  business_date: string;
  note?: string;
  lines: DocLineInput[];
}

export type WriteOffReason =
  | "spoilage"
  | "expiry"
  | "staff_meal"
  | "loss"
  | "other";

export interface WriteOffLine {
  id: string;
  product_id: string;
  product_name?: string;
  qty_input: number;
  input_unit: Unit;
  qty_base: number;
  cost_cents: number;
  seq: number;
}

export interface WriteOff {
  id: string;
  reason: WriteOffReason;
  note?: string;
  status: DocStatus;
  business_date: string;
  posted_at: string | null;
  reversal_of: string | null;
  total_cents: number;
  lines: WriteOffLine[];
}

export interface WriteOffInput {
  reason: WriteOffReason;
  note?: string;
  business_date: string;
  lines: DocLineInput[];
}

export interface StocktakeLine {
  id: string;
  product_id: string;
  product_name?: string;
  unit: BaseUnit;
  counted_qty: number; // milli base-units
  expected_qty: number | null;
  variance_qty: number | null;
  variance_cost_cents: number | null;
}

export interface Stocktake {
  id: string;
  status: DocStatus;
  note?: string;
  business_date: string | null;
  posted_at: string | null;
  reversal_of: string | null;
  lines: StocktakeLine[];
}

// dry-run response: same line shape, nothing persisted.
export interface StocktakePreview {
  lines: StocktakeLine[];
  total_variance_cost_cents: number;
}

export interface StockMove {
  id: string;
  product_id: string;
  product_name?: string;
  kind:
    | "receipt"
    | "sale"
    | "writeoff"
    | "stocktake_surplus"
    | "stocktake_shortage"
    | "reversal";
  qty: number; // milli base-units, signed
  unit: BaseUnit;
  cost_cents: number; // signed magnitude
  estimated: boolean;
  business_date: string;
  recorded_at: string;
  doc_kind: string;
  doc_id: string;
}

export interface FoodCostRow {
  menu_item_id: string;
  name: string;
  revenue_cents: number;
  actual_cogs_cents: number;
  theoretical_cogs_cents: number;
  food_cost_pct: number; // actual_cogs / revenue, 0..1
}

export interface FoodCostReport {
  items: FoodCostRow[];
  totals: {
    revenue_cents: number;
    actual_cogs_cents: number;
    theoretical_cogs_cents: number;
    food_cost_pct: number;
  };
  estimated_share: number; // 0..1 share of sale moves flagged estimated
}
