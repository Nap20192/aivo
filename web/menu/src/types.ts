// Contract types for GET /api/v1/t/{table_token} and the diner POST endpoints
// (docs/PLATFORM.md, "API surface"). Money is integer cents everywhere.

export interface HoursRow {
  label: string;
  open: string;
  close: string;
}

export interface Restaurant {
  name: string;
  slug: string;
  tagline?: string | null;
  hours: HoursRow[];
  address?: string | null;
  map_url?: string | null;
  phone?: string | null;
  instagram?: string | null;
}

export interface Theme {
  brand_name: string;
  /** "Blood red" | "Olive" | "Wine" | "Fire" (case-insensitive) */
  accent: string;
  bold: boolean;
  banner_url?: string | null;
  /** Raw CSS custom property overrides, applied last. */
  css_vars?: Record<string, string> | null;
}

export interface Option {
  id: string;
  label: string;
  price_delta_cents: number;
}

export interface OptionGroup {
  id: string;
  name: string;
  select: "single" | "multi";
  options: Option[];
}

export interface MenuItem {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  image_url?: string | null;
  allergens: string[];
  option_groups: OptionGroup[];
  available: boolean;
  /** "HH:MM" the kitchen 86'd it, when available is false. */
  sold_out_at?: string | null;
}

export interface Category {
  id: string;
  name: string;
  items: MenuItem[];
}

export interface OpenRequest {
  type: "waiter" | "bill";
  created_at: string;
}

export interface TableSession {
  restaurant: Restaurant;
  table: { id: string; label: string };
  theme: Theme;
  menu: Category[];
  /** Open service requests for this table ("one open request per table" state). */
  open_requests: OpenRequest[];
}

export interface OrderLineInput {
  menu_item_id: string;
  qty: number;
  options: { group_id: string; option_ids: string[] }[];
}

export interface OrderInput {
  lines: OrderLineInput[];
  note?: string;
}
