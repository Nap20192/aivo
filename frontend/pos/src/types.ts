export interface Me {
  user: { id: string; name: string; email: string; role: string };
  restaurant: { id: string; name: string };
}

export type ShiftState = "open" | "closed" | "accepted";

export interface Shift {
  id: string;
  number: string; // "shift-121"
  till: number;
  cashier: string;
  opened_at: string; // "16:04"
  opening_float_cents: number;
  expected_cents: number;
  state: ShiftState;
}

// Tenders / payments (§4). GL semantics keyed by payment_group.
export type PaymentGroup = "cash" | "card" | "gift_card" | "comp" | "void" | "house_account";

export interface PaymentMethod {
  id: string;
  code: string; // "cash", "card"
  name: string; // "Cash", "Card"
  payment_group: PaymentGroup;
}

/** A tender line entered at ticket close. */
export interface Tender {
  method_id: string;
  amount_cents: number;
  tip_cents: number;
}

export interface TicketPayment {
  method_id: string;
  payment_group: PaymentGroup;
  amount_cents: number;
  tip_cents: number;
}

export interface ClosedTicket {
  id: string;
  status: "closed";
  closed_at: string;
  total_cents: number;
  payments: TicketPayment[];
}

// Cash movements inside a shift (§4).
export type CashKind = "pay_in" | "pay_out" | "drop";

export interface CashOperation {
  id: string;
  kind: CashKind;
  amount_cents: number;
  reason: string;
  recorded_at: string;
}

/** Response of POST /pos/shifts/{id}/close — the shift, now closed (draft journal built). */
export interface ShiftClose {
  id: string;
  number: string;
  state: ShiftState; // "closed"
  expected_cents: number;
  declared_cents: number;
  variance_cents: number;
  closed_at: string;
  journal_document_id: string;
}

export interface TenderBreakdown {
  payment_group: PaymentGroup;
  amount_cents: number;
  tip_cents: number;
}

/** Z-report — cashier-facing shift summary (§4). */
export interface ZReport {
  opening_float_cents: number;
  tenders: TenderBreakdown[];
  cash_operations: { kind: CashKind; amount_cents: number }[];
  expected_cash_cents: number;
  declared_cents: number;
  variance_cents: number;
  state: ShiftState;
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
  // §4 has no list-payment-methods endpoint; POS needs the tender buttons, so
  // /pos/state carries them. Flagged to backend as an open question.
  payment_methods: PaymentMethod[];
}

export interface HandoffPreview {
  code: string;
  table_id: string;
  table_number: string;
  customer_name: string | null;
  note: string | null;
  lines: Line[];
  expires_at: string; // ISO
}

export interface NewLine {
  menu_item_id: string;
  qty: number;
  options: string[];
}

export interface PosApi {
  login(email: string, password: string): Promise<Me>;
  me(): Promise<Me>;
  /** null = unchanged since the last poll (304 or identical body) — skip the update */
  state(): Promise<PosState | null>;
  openShift(openingFloatCents: number): Promise<void>;
  addLines(tableId: string, lines: NewLine[]): Promise<void>;
  fire(ticketId: string): Promise<void>;
  handoff(code: string): Promise<HandoffPreview>;
  acceptHandoff(code: string, tableId: string): Promise<void>;
  ack(requestId: string): Promise<void>;
  dismiss(requestId: string): Promise<void>;
  closeTicket(ticketId: string, tenders: Tender[]): Promise<ClosedTicket>;
  cashOperation(shiftId: string, kind: CashKind, amountCents: number, reason: string): Promise<CashOperation>;
  closeShift(shiftId: string, declaredCents: number): Promise<ShiftClose>;
  zReport(shiftId: string): Promise<ZReport>;
}
