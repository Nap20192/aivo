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
