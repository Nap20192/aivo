export interface Me {
  user: { id: string; name: string; email: string; role: string };
  restaurant: { id: string; name: string };
}

export interface Shift {
  id: string;
  number: string; // "shift-121"
  till: number;
  cashier: string;
  opened_at: string; // "16:04"
  opening_float_cents: number;
  expected_cents: number;
}

export interface PostedShift {
  number: string;
  expected_cents: number;
  declared_cents: number;
  posted_at: string; // "21:34"
  gl_lines: number;
}

export interface MenuItem {
  id: string;
  name: string;
  price_cents: number;
  mods?: string[]; // doneness options
}

export interface MenuCategory {
  id: string;
  name: string;
  items: MenuItem[];
}

export interface Line {
  id: string;
  menu_item_id: string | null;
  name: string;
  qty: number;
  options: string[];
  unit_price_cents: number;
}

export interface Ticket {
  id: string;
  lines: Line[];
  note: string | null;
  source: string; // "from the diner's phone · 20:16"
  placed_at: string | null; // "20:16"
  fired_at: string | null; // "19:34"
}

export interface Table {
  id: string;
  number: string; // "12"
  covers: number | null;
  ticket: Ticket | null;
}

export interface PosRequest {
  id: string;
  table_id: string;
  table_number: string;
  kind: "waiter" | "bill";
  asked_at: string; // "20:14"
  created_at: number; // epoch ms, for waiting time
  open_total_cents: number | null;
}

export interface PosState {
  restaurant: { id: string; name: string };
  till: number;
  cashier: string;
  shift: Shift | null;
  other_till_shift: { till: number; shift_number: string; cashier: string; opened_at: string } | null;
  tables: Table[];
  requests: PosRequest[];
  menu: MenuCategory[];
}

export interface NewLine {
  menu_item_id: string;
  qty: number;
  options: string[];
}

export interface PosApi {
  login(email: string, password: string): Promise<Me>;
  me(): Promise<Me>;
  state(): Promise<PosState>;
  openShift(openingFloatCents: number): Promise<void>;
  addLines(tableId: string, lines: NewLine[]): Promise<void>;
  fire(ticketId: string): Promise<void>;
  ack(requestId: string): Promise<void>;
  dismiss(requestId: string): Promise<void>;
  closeShift(shiftId: string, declaredCents: number): Promise<PostedShift>;
}
