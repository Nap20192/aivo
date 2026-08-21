import type { Me, MenuCategory, NewLine, PosApi, PosState, PostedShift } from "./types.ts";
import { timeHM } from "./format.ts";

// Fixtures matching docs/prototypes/aivo-pos-prototype.dc.html (Ember & Bone, shift-121).

const DONENESS = ["Rare", "Medium rare", "Medium", "Well done"];

const menu: MenuCategory[] = [
  {
    id: "c-starters",
    name: "Starters",
    items: [
      { id: "m-marrow", name: "Bone marrow, sourdough", price_cents: 1400 },
      { id: "m-tartare", name: "Beef tartare, cured yolk", price_cents: 1800 },
      { id: "m-flatbread", name: "Grilled flatbread, beef fat", price_cents: 800 },
    ],
  },
  {
    id: "c-grill",
    name: "Grill",
    items: [
      { id: "m-ribeye300", name: "Ribeye 300 g", price_cents: 4600, mods: DONENESS },
      { id: "m-ribeye400", name: "Ribeye 400 g", price_cents: 5800, mods: DONENESS },
      { id: "m-bavette", name: "Bavette, chimichurri", price_cents: 3400, mods: DONENESS },
      { id: "m-chicken", name: "Half chicken, brined", price_cents: 2800 },
    ],
  },
  {
    id: "c-sides",
    name: "Sides",
    items: [
      { id: "m-chips", name: "Triple-cooked chips", price_cents: 900 },
      { id: "m-hispi", name: "Hispi cabbage", price_cents: 800 },
      { id: "m-salad", name: "Green salad", price_cents: 700 },
    ],
  },
  {
    id: "c-wine",
    name: "Wine",
    items: [
      { id: "m-malbec", name: "Malbec, glass", price_cents: 1400 },
      { id: "m-gamay", name: "Gamay, Beaujolais", price_cents: 1200 },
      { id: "m-ribolla", name: "Ribolla, 2021", price_cents: 1300 },
    ],
  },
];

const me: Me = {
  user: { id: "u-yana", name: "Yana P.", email: "yana@emberandbone.example", role: "waiter" },
  restaurant: { id: "r-ember", name: "Ember & Bone" },
};

let shiftSeq = 121;
let idSeq = 1;
const uid = (p: string) => p + "-" + idSeq++;

const state: PosState = {
  restaurant: me.restaurant,
  till: 1,
  cashier: me.user.name,
  shift: null,
  other_till_shift: { till: 2, shift_number: "shift-117", cashier: "Marek", opened_at: "16:04" },
  tables: [
    {
      id: "t-12",
      number: "12",
      covers: 4,
      ticket: {
        id: "tk-12",
        lines: [
          {
            id: "l-1",
            menu_item_id: null,
            name: "Dry-aged ribeye",
            qty: 1,
            options: ["400 g", "medium rare", "béarnaise"],
            unit_price_cents: 6100,
          },
          { id: "l-2", menu_item_id: "m-chips", name: "Triple-cooked chips", qty: 2, options: [], unit_price_cents: 900 },
        ],
        note: "Severe nut allergy — no hazelnut on the table. Ribeye after the starters.",
        source: "from the diner's phone · 20:16",
        placed_at: "20:16",
        fired_at: null,
      },
    },
    {
      id: "t-04",
      number: "04",
      covers: 2,
      ticket: {
        id: "tk-04",
        lines: [
          { id: "l-3", menu_item_id: "m-bavette", name: "Bavette, chimichurri", qty: 2, options: [], unit_price_cents: 3400 },
          { id: "l-4", menu_item_id: null, name: "Malbec, bottle", qty: 1, options: [], unit_price_cents: 6000 },
        ],
        note: null,
        source: "taken at the table · 19:32",
        placed_at: "19:32",
        fired_at: "19:34",
      },
    },
    {
      id: "t-07",
      number: "07",
      covers: 3,
      ticket: { id: "tk-07", lines: [], note: null, source: "seated 20:05", placed_at: null, fired_at: null },
    },
    { id: "t-09", number: "09", covers: null, ticket: null },
    { id: "t-03", number: "03", covers: null, ticket: null },
    { id: "t-15", number: "15", covers: null, ticket: null },
  ],
  requests: [
    {
      id: "rq-w12",
      table_id: "t-12",
      table_number: "12",
      kind: "waiter",
      asked_at: "20:14",
      created_at: Date.now() - 60_000,
      open_total_cents: null,
    },
    {
      id: "rq-b04",
      table_id: "t-04",
      table_number: "04",
      kind: "bill",
      asked_at: "20:11",
      created_at: Date.now() - 4 * 60_000,
      open_total_cents: 12800,
    },
  ],
  menu,
};

const notFound = () => Object.assign(new Error("not found"), { code: "not_found", status: 404 });

// Cart handoff codes (single-use, 15 min TTL, A-Z2-9). K7M2PX is the live demo code.
interface MockHandoff {
  code: string;
  table_id: string;
  customer_name: string | null;
  note: string | null;
  lines: { menu_item_id: string | null; name: string; qty: number; options: string[]; unit_price_cents: number }[];
  expires_at_ms: number;
  used: boolean;
}

const handoffs: MockHandoff[] = [
  {
    code: "K7M2PX",
    table_id: "t-07",
    customer_name: "Mila K.",
    note: "Glasses chilled, please.",
    lines: [
      { menu_item_id: "m-tartare", name: "Beef tartare, cured yolk", qty: 1, options: [], unit_price_cents: 1800 },
      { menu_item_id: "m-malbec", name: "Malbec, glass", qty: 2, options: [], unit_price_cents: 1400 },
    ],
    expires_at_ms: Date.now() + 15 * 60_000,
    used: false,
  },
  {
    // expired fixture for the honest-404 path
    code: "XPRD99",
    table_id: "t-07",
    customer_name: null,
    note: null,
    lines: [{ menu_item_id: "m-salad", name: "Green salad", qty: 1, options: [], unit_price_cents: 700 }],
    expires_at_ms: Date.now() - 60_000,
    used: false,
  },
];

function activeHandoff(code: string): MockHandoff {
  const h = handoffs.find((x) => x.code === code.toUpperCase());
  if (!h || h.used || Date.now() > h.expires_at_ms) throw notFound();
  return h;
}

export const mockApi: PosApi = {
  async login() {
    return me;
  },
  async me() {
    return me;
  },
  async state() {
    // deep-ish copy so callers can't mutate fixtures
    return JSON.parse(JSON.stringify(state)) as PosState;
  },
  async openShift(openingFloatCents: number) {
    if (state.shift) throw Object.assign(new Error("shift already open on this till"), { code: "conflict" });
    state.shift = {
      id: uid("sh"),
      number: "shift-" + shiftSeq,
      till: state.till,
      cashier: state.cashier,
      opened_at: timeHM(),
      opening_float_cents: openingFloatCents,
      // ponytail: fixed takings offset matching the prototype; the real backend computes this
      expected_cents: openingFloatCents + 113450,
    };
  },
  async addLines(tableId: string, lines: NewLine[]) {
    const table = state.tables.find((t) => t.id === tableId);
    if (!table) throw notFound();
    const items = state.menu.flatMap((c) => c.items);
    const newLines = lines.map((l) => {
      const item = items.find((i) => i.id === l.menu_item_id);
      if (!item) throw notFound();
      return {
        id: uid("l"),
        menu_item_id: item.id,
        name: item.name,
        qty: l.qty,
        options: l.options,
        unit_price_cents: item.price_cents,
      };
    });
    const now = timeHM();
    if (!table.ticket) {
      table.covers = table.covers ?? 2;
      table.ticket = { id: uid("tk"), lines: [], note: null, source: "", placed_at: null, fired_at: null };
    }
    table.ticket.lines.push(...newLines);
    table.ticket.source = "taken at the table · " + now;
    table.ticket.placed_at = now;
    table.ticket.fired_at = null; // new items fire separately
  },
  async fire(ticketId: string) {
    const table = state.tables.find((t) => t.ticket?.id === ticketId);
    if (!table || !table.ticket) throw notFound();
    table.ticket.fired_at = timeHM();
  },
  async handoff(code: string) {
    const h = activeHandoff(code);
    const table = state.tables.find((t) => t.id === h.table_id);
    return {
      code: h.code,
      table_id: h.table_id,
      table_number: table?.number ?? "",
      customer_name: h.customer_name,
      note: h.note,
      lines: h.lines.map((l, i) => ({ id: "hl-" + i, ...l })),
      expires_at: new Date(h.expires_at_ms).toISOString(),
    };
  },
  async acceptHandoff(code: string, tableId: string) {
    const h = activeHandoff(code);
    const table = state.tables.find((t) => t.id === tableId);
    if (!table) throw notFound();
    const now = timeHM();
    if (!table.ticket) {
      table.covers = table.covers ?? 2;
      table.ticket = { id: uid("tk"), lines: [], note: null, source: "", placed_at: null, fired_at: null };
    }
    table.ticket.lines.push(...h.lines.map((l) => ({ id: uid("l"), ...l })));
    if (h.note) table.ticket.note = table.ticket.note ? table.ticket.note + " " + h.note : h.note;
    table.ticket.source = "from the diner's phone · " + now;
    table.ticket.placed_at = now;
    table.ticket.fired_at = null; // new items fire separately
    h.used = true;
  },
  async ack(requestId: string) {
    state.requests = state.requests.filter((r) => r.id !== requestId);
  },
  async dismiss(requestId: string) {
    state.requests = state.requests.filter((r) => r.id !== requestId);
  },
  async closeShift(shiftId: string, declaredCents: number): Promise<PostedShift> {
    const s = state.shift;
    if (!s || s.id !== shiftId) throw notFound();
    state.shift = null;
    shiftSeq++;
    return {
      number: s.number,
      expected_cents: s.expected_cents,
      declared_cents: declaredCents,
      posted_at: timeHM(),
      gl_lines: 3,
    };
  },
};
