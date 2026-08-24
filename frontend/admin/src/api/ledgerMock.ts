// In-memory ledger mock — chart of accounts, account-map, shifts, journals.
// Illustrative GL, not the real posting engine: the shift-acceptance draft folds
// float/cash-movements into the drawer line (like reference §6) so it balances
// on cash + card + variance alone. The backend owns the true formula.
import { ApiError } from "../../../design-system/shared/api";
import type {
  Account,
  AcceptanceOverride,
  AccountMapEntry,
  CostCenter,
  JournalDocument,
  JournalLine,
  JournalSummary,
  ManualJournalInput,
  ShiftAcceptance,
  ShiftRow,
} from "./types";

let seq = 1;
const uid = (p: string) => `${p}-${seq++}`;

// Seed chart of accounts (impl-contract §6).
const accounts: Account[] = [
  { id: "acc-1000", code: "1000", name: "Cash on hand (drawer)", type: "asset", normal_side: "debit", postable: true },
  { id: "acc-1010", code: "1010", name: "Card clearing", type: "asset", normal_side: "debit", postable: true },
  { id: "acc-1020", code: "1020", name: "Undeposited funds", type: "asset", normal_side: "debit", postable: true },
  { id: "acc-1100", code: "1100", name: "House account receivable", type: "asset", normal_side: "debit", postable: true },
  { id: "acc-2000", code: "2000", name: "Gift card liability", type: "liability", normal_side: "credit", postable: true },
  { id: "acc-4000", code: "4000", name: "Sales revenue", type: "revenue", normal_side: "credit", postable: true },
  { id: "acc-4900", code: "4900", name: "Comps / contra-revenue", type: "revenue", normal_side: "debit", postable: true },
  { id: "acc-5900", code: "5900", name: "Cash over/short", type: "expense", normal_side: "debit", postable: true },
  { id: "acc-6000", code: "6000", name: "Cash movements (pay in/out)", type: "expense", normal_side: "debit", postable: true },
  { id: "acc-9999", code: "9999", name: "Unassigned / rounding", type: "expense", normal_side: "debit", postable: true },
];
const accById = (id: string) => accounts.find((a) => a.id === id);

const costCenters: CostCenter[] = [{ id: "cc-main", code: "main", name: "Main" }];

let accountMap: AccountMapEntry[] = [
  ["sales_revenue", "acc-4000"],
  ["cash_drawer", "acc-1000"],
  ["cash_over_short", "acc-5900"],
  ["cash_movement", "acc-6000"],
  ["rounding_unassigned", "acc-9999"],
  ["tender:cash", "acc-1000"],
  ["tender:card", "acc-1010"],
  ["tender:gift_card", "acc-2000"],
  ["tender:comp", "acc-4900"],
  ["tender:house_account", "acc-1100"],
].map(([purpose, account_id]) => ({ purpose, account_id, account_code: accById(account_id)!.code }));

function line(accountId: string, side: "debit" | "credit", amount: number, memo: string, editable = true): JournalLine {
  const a = accById(accountId)!;
  return {
    line_id: uid("ln"),
    account_id: a.id,
    account_code: a.code,
    account_name: a.name,
    side,
    amount_cents: amount,
    cost_center_id: "cc-main",
    memo,
    editable,
  };
}

// Build the shift-acceptance draft lines from cash/card sales and the variance.
function draftLines(cashSales: number, cardSales: number, variance: number): JournalLine[] {
  const lines = [line("acc-1000", "debit", cashSales + variance, "Counted cash to drawer")];
  if (cardSales > 0) lines.push(line("acc-1010", "debit", cardSales, "Card clearing"));
  if (variance < 0) lines.push(line("acc-5900", "debit", -variance, "Cash shortage"));
  if (variance > 0) lines.push(line("acc-5900", "credit", variance, "Cash surplus"));
  lines.push(line("acc-4000", "credit", cashSales + cardSales, "Shift sales"));
  return lines;
}

interface Shift extends ShiftRow {
  cashSales: number;
  cardSales: number;
  document: JournalDocument;
}

function makeShift(number: string, cashier: string, opened: string, closed: string, cashSales: number, cardSales: number, declared: number, expected: number): Shift {
  const variance = declared - expected;
  return {
    id: uid("sh"),
    number,
    cashier,
    opened_at: opened,
    closed_at: closed,
    accepted_at: null,
    state: "closed",
    expected_cents: expected,
    declared_cents: declared,
    variance_cents: variance,
    cashSales,
    cardSales,
    document: {
      id: uid("jd"),
      kind: "shift_acceptance",
      state: "draft",
      accounting_date: closed.slice(0, 10),
      recorded_at: closed,
      posted_at: null,
      cancelled_at: null,
      reversal_of: null,
      lines: draftLines(cashSales, cardSales, variance),
    },
  };
}

// Closed shifts awaiting acceptance; move to `shiftsAccepted` once posted.
const shifts: Shift[] = [
  makeShift("shift-120", "Yana P.", "2026-08-23T16:02:00Z", "2026-08-23T23:41:00Z", 68000, 45450, 77800, 78000),
  makeShift("shift-119", "Marek D.", "2026-08-22T15:58:00Z", "2026-08-22T23:20:00Z", 51200, 38900, 66200, 66200),
];
const shiftsAccepted: Shift[] = [];
const rowOf = ({ cashSales, cardSales, document, ...r }: Shift): ShiftRow => (void cashSales, void cardSales, void document, r);

// A previously accepted shift + a manual journal, so the journal list isn't empty.
const journals: JournalDocument[] = [
  (() => {
    const s = makeShift("shift-118", "Yana P.", "2026-08-21T16:00:00Z", "2026-08-21T23:30:00Z", 60000, 40000, 74400, 74000);
    s.document.state = "posted";
    s.document.posted_at = "2026-08-22T09:12:00Z";
    s.document.lines.forEach((l) => (l.editable = false));
    return s.document;
  })(),
  {
    id: uid("jd"),
    kind: "manual",
    state: "posted",
    accounting_date: "2026-08-20",
    recorded_at: "2026-08-20T10:00:00Z",
    posted_at: "2026-08-20T10:00:00Z",
    cancelled_at: null,
    reversal_of: null,
    lines: [
      line("acc-6000", "debit", 12000, "Petty cash — cleaning supplies", false),
      line("acc-1000", "credit", 12000, "From drawer", false),
    ],
  },
];

const total = (lines: JournalLine[]) => lines.filter((l) => l.side === "debit").reduce((a, l) => a + l.amount_cents, 0);
const summary = (d: JournalDocument): JournalSummary => ({
  id: d.id,
  kind: d.kind,
  state: d.state,
  accounting_date: d.accounting_date,
  recorded_at: d.recorded_at,
  source_kind: d.kind === "manual" ? "manual" : d.kind === "shift_acceptance" ? "shift" : null,
  source_id: null,
  reversal_of: d.reversal_of,
  total_cents: total(d.lines),
});

const acceptance = (s: Shift): ShiftAcceptance => {
  const { cashSales, cardSales, document, ...row } = s;
  void cashSales;
  void cardSales;
  return {
    shift: row,
    document: { id: document.id, state: document.state, accounting_date: document.accounting_date, recorded_at: document.recorded_at, lines: document.lines },
    variance_cents: s.variance_cents,
    balanced: true,
  };
};

const notFound = () => new ApiError(404, "not_found", "Not found.");

async function delay<T>(v: T): Promise<T> {
  await new Promise((r) => setTimeout(r, 120));
  return v;
}

export const ledgerMock = {
  listShifts(state?: "closed" | "accepted"): Promise<ShiftRow[]> {
    const rows = [...shifts, ...shiftsAccepted].filter((s) => !state || s.state === state).map(rowOf);
    return delay(rows);
  },
  getAcceptance(shiftId: string): Promise<ShiftAcceptance> {
    const s = shifts.find((x) => x.id === shiftId);
    if (!s) throw notFound();
    return delay(acceptance(s));
  },
  patchAcceptance(shiftId: string, overrides: AcceptanceOverride[]): Promise<ShiftAcceptance> {
    const s = shifts.find((x) => x.id === shiftId);
    if (!s) throw notFound();
    if (s.state !== "closed") throw new ApiError(409, "document_posted", "Shift already accepted.");
    for (const ov of overrides) {
      const l = s.document.lines.find((x) => x.line_id === ov.line_id);
      if (!l) continue;
      if (ov.account_id) {
        const a = accById(ov.account_id);
        if (!a) throw notFound();
        if (!a.postable) throw new ApiError(422, "account_not_postable", `${a.code} is not postable.`);
        l.account_id = a.id;
        l.account_code = a.code;
        l.account_name = a.name;
      }
      if (ov.cost_center_id) l.cost_center_id = ov.cost_center_id;
    }
    return delay(acceptance(s));
  },
  acceptShift(shiftId: string): Promise<{ shift: ShiftRow; document: JournalDocument }> {
    const s = shifts.find((x) => x.id === shiftId);
    if (!s) throw notFound();
    if (s.state === "accepted") throw new ApiError(409, "already_accepted", "Shift already accepted.");
    const now = "2026-08-24T09:30:00Z";
    s.state = "accepted";
    s.accepted_at = now;
    s.document.state = "posted";
    s.document.posted_at = now;
    s.document.lines.forEach((l) => (l.editable = false));
    journals.unshift(s.document);
    const idx = shifts.indexOf(s);
    if (idx >= 0) shifts.splice(idx, 1);
    shiftsAccepted.push(s);
    return delay({ shift: rowOf(s), document: s.document });
  },
  listAccounts(): Promise<Account[]> {
    return delay(accounts);
  },
  listCostCenters(): Promise<CostCenter[]> {
    return delay(costCenters);
  },
  getAccountMap(): Promise<AccountMapEntry[]> {
    return delay(accountMap);
  },
  putAccountMap(map: { purpose: string; account_id: string }[]): Promise<AccountMapEntry[]> {
    accountMap = map.map((m) => {
      const a = accById(m.account_id);
      if (!a) throw new ApiError(422, "unknown_purpose", "Unknown account.");
      if (!a.postable) throw new ApiError(422, "account_not_postable", `${a.code} is not postable.`);
      return { purpose: m.purpose, account_id: a.id, account_code: a.code };
    });
    return delay(accountMap);
  },
  listJournals(): Promise<JournalSummary[]> {
    return delay(journals.map(summary));
  },
  getJournal(docId: string): Promise<JournalDocument> {
    const d = journals.find((x) => x.id === docId);
    if (!d) throw notFound();
    return delay(d);
  },
  postManualJournal(input: ManualJournalInput, post: boolean): Promise<{ document: JournalDocument }> {
    const debit = input.lines.filter((l) => l.side === "debit").reduce((a, l) => a + l.amount_cents, 0);
    const credit = input.lines.filter((l) => l.side === "credit").reduce((a, l) => a + l.amount_cents, 0);
    if (debit !== credit) throw new ApiError(422, "unbalanced", "Manual journals must balance (Σ debit = Σ credit).");
    for (const l of input.lines) {
      const a = accById(l.account_id);
      if (!a) throw notFound();
      if (!a.postable) throw new ApiError(422, "account_not_postable", `${a.code} is not postable.`);
    }
    const doc: JournalDocument = {
      id: uid("jd"),
      kind: "manual",
      state: post ? "posted" : "draft",
      accounting_date: input.accounting_date,
      recorded_at: input.accounting_date + "T00:00:00Z",
      posted_at: post ? input.accounting_date + "T00:00:00Z" : null,
      cancelled_at: null,
      reversal_of: null,
      lines: input.lines.map((l) => line(l.account_id, l.side, l.amount_cents, l.memo ?? input.memo, false)),
    };
    if (post) journals.unshift(doc);
    return delay({ document: doc });
  },
  cancelJournal(docId: string): Promise<{ reversal: JournalDocument; original: JournalDocument }> {
    const d = journals.find((x) => x.id === docId);
    if (!d) throw notFound();
    if (d.state === "cancelled") throw new ApiError(409, "already_cancelled", "Already cancelled.");
    if (d.state !== "posted") throw new ApiError(409, "not_posted", "Only posted documents can be reversed.");
    const reversal: JournalDocument = {
      id: uid("jd"),
      kind: "reversal",
      state: "posted",
      accounting_date: "2026-08-24", // revalidated at current date (§15.1)
      recorded_at: "2026-08-24T09:30:00Z",
      posted_at: "2026-08-24T09:30:00Z",
      cancelled_at: null,
      reversal_of: d.id,
      lines: d.lines.map((l) => line(l.account_id, l.side === "debit" ? "credit" : "debit", l.amount_cents, "Reversal of " + (l.memo ?? ""), false)),
    };
    d.state = "cancelled";
    d.cancelled_at = "2026-08-24T09:30:00Z";
    journals.unshift(reversal);
    return delay({ reversal, original: d });
  },
};
