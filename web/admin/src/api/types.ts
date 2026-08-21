// Shapes per docs/PLATFORM.md "API surface (JSON, /api/v1)".

export type Role = "owner" | "manager" | "waiter";
export type Plan = "free" | "pro" | "business";
export type Accent = "Blood red" | "Olive" | "Wine" | "Fire";

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
  | ({ type: "update_theme" } & Partial<Omit<Theme, "design_md">>)
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

export interface AssistantApplyResult {
  index: number;
  ok: boolean;
  detail: string;
}
