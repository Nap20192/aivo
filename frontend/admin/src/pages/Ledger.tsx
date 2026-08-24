import { Plus } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { Account, ManualJournalInput, Side } from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents, parseDollars } from "../../../design-system/shared/money";
import { useLoad } from "../lib/useLoad";
import { Badge, ErrorBanner, LoadingPage, Modal } from "../ui";

type Tab = "accounts" | "map" | "journals";

const SUBS: Record<Tab, string> = {
  accounts: "The chart of accounts seeded for this restaurant. Read-only — types and normal sides freeze after the first posting.",
  map: "Which account each posting purpose resolves to. This is the per-restaurant GL interpretation: change cash's mapping and cash posts elsewhere.",
  journals: "Every posted journal — shift acceptances, manual entries, reversals. Corrections never edit a posted document; they post a reversal.",
};

export default function Ledger() {
  const [tab, setTab] = useState<Tab>("journals");
  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Ledger</h1>
          <p className="page-sub">{SUBS[tab]}</p>
        </div>
      </div>
      <div className="tabs" style={{ marginBottom: "var(--gap-section)" }}>
        {(["journals", "accounts", "map"] as Tab[]).map((t) => (
          <button key={t} className={"tab" + (tab === t ? " on" : "")} onClick={() => setTab(t)}>
            {t === "journals" ? "Journals" : t === "accounts" ? "Chart of accounts" : "Account map"}
          </button>
        ))}
      </div>
      {tab === "accounts" && <AccountsTab />}
      {tab === "map" && <AccountMapTab />}
      {tab === "journals" && <JournalsTab />}
    </div>
  );
}

const TYPE_TONE: Record<Account["type"], "neutral" | "ok" | "info" | "warn"> = {
  asset: "info",
  liability: "warn",
  revenue: "ok",
  expense: "neutral",
  equity: "neutral",
  statistical: "neutral",
};

function AccountsTab() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listAccounts(r.id), [r.id]);
  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;
  return (
    <div className="card" style={{ padding: 0 }}>
      <table className="table-plain">
        <thead>
          <tr>
            <th>Code</th>
            <th>Name</th>
            <th>Type</th>
            <th>Normal side</th>
            <th>Postable</th>
          </tr>
        </thead>
        <tbody>
          {data.map((a) => (
            <tr key={a.id}>
              <td className="num" style={{ font: "var(--type-label)" }}>{a.code}</td>
              <td>{a.name}</td>
              <td>
                <Badge tone={TYPE_TONE[a.type]}>{a.type}</Badge>
              </td>
              <td style={{ color: "var(--text-muted)" }}>{a.normal_side}</td>
              <td>{a.postable ? "yes" : "no"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AccountMapTab() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(
    () => Promise.all([api.getAccountMap(r.id), api.listAccounts(r.id)]).then(([map, accounts]) => ({ map, accounts })),
    [r.id],
  );
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [saveErr, setSaveErr] = useState<string | null>(null);

  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;

  const postable = data.accounts.filter((a) => a.postable);
  const valueFor = (purpose: string, accountId: string) => edits[purpose] ?? accountId;
  const dirty = Object.keys(edits).length > 0;

  const save = () => {
    setBusy(true);
    setSaveErr(null);
    const map = data.map.map((e) => ({ purpose: e.purpose, account_id: valueFor(e.purpose, e.account_id) }));
    api
      .putAccountMap(r.id, map)
      .then(() => {
        setEdits({});
        reload();
      })
      .catch((e: { message?: string }) => setSaveErr(e.message ?? "Could not save the map."))
      .finally(() => setBusy(false));
  };

  return (
    <div className="stack">
      <div className="card" style={{ padding: 0 }}>
        <table className="table-plain">
          <thead>
            <tr>
              <th>Purpose</th>
              <th>Account</th>
            </tr>
          </thead>
          <tbody>
            {data.map.map((e) => (
              <tr key={e.purpose}>
                <td className="num" style={{ font: "var(--type-label)" }}>{e.purpose}</td>
                <td>
                  <select
                    className="select"
                    value={valueFor(e.purpose, e.account_id)}
                    onChange={(ev) => setEdits({ ...edits, [e.purpose]: ev.target.value })}
                  >
                    {postable.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.code} · {a.name}
                      </option>
                    ))}
                  </select>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {saveErr && <ErrorBanner message={saveErr} />}
      <div className="row">
        <button className="btn btn-primary" disabled={!dirty || busy} onClick={save}>
          Save mapping
        </button>
        {dirty && (
          <button className="btn btn-ghost" onClick={() => setEdits({})}>
            Discard
          </button>
        )}
      </div>
    </div>
  );
}

const KIND_TONE = { shift_acceptance: "info", manual: "neutral", reversal: "warn" } as const;

function JournalsTab() {
  const r = useRestaurant();
  const [from, setFrom] = useState("2026-08-01");
  const [open, setOpen] = useState<string | null>(null);
  const [manual, setManual] = useState(false);
  const { data, error, loading, reload } = useLoad(() => api.listJournals(r.id, { from }), [r.id, from]);

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-end" }}>
        <label className="field" style={{ maxWidth: 180 }}>
          <span className="field-label">From date</span>
          <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <button className="btn btn-primary" onClick={() => setManual(true)}>
          <Plus size={16} /> Manual journal
        </button>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}

      {data && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Date</th>
                <th>Kind</th>
                <th>State</th>
                <th style={{ textAlign: "right" }}>Total</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((j) => (
                <tr key={j.id}>
                  <td className="num" style={{ fontSize: 13 }}>{j.accounting_date}</td>
                  <td>
                    <Badge tone={KIND_TONE[j.kind]}>{j.kind.replace("_", " ")}</Badge>
                  </td>
                  <td>
                    <Badge tone={j.state === "posted" ? "ok" : j.state === "cancelled" ? "danger" : "outline"}>{j.state}</Badge>
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(j.total_cents)}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setOpen(j.id)}>
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open && <JournalModal docId={open} onClose={() => setOpen(null)} onChanged={reload} />}
      {manual && <ManualJournalModal onClose={() => setManual(false)} onPosted={reload} />}
    </div>
  );
}

function JournalModal({ docId, onClose, onChanged }: { docId: string; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const { data, error, loading } = useLoad(() => api.getJournal(r.id, docId), [r.id, docId]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const cancel = () => {
    setBusy(true);
    setErr(null);
    api
      .cancelJournal(r.id, docId)
      .then(() => {
        onChanged();
        onClose();
      })
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not reverse the document.");
        setBusy(false);
      });
  };

  const canCancel = data && data.state === "posted" && data.kind !== "reversal";
  const debit = data ? data.lines.filter((l) => l.side === "debit").reduce((a, l) => a + l.amount_cents, 0) : 0;
  const credit = data ? data.lines.filter((l) => l.side === "credit").reduce((a, l) => a + l.amount_cents, 0) : 0;

  return (
    <Modal
      title={data ? `${data.kind.replace("_", " ")} · ${data.accounting_date}` : "Journal"}
      onClose={onClose}
      wide
      footer={
        canCancel ? (
          <button className="btn btn-danger" disabled={busy} onClick={cancel}>
            Reverse this document
          </button>
        ) : null
      }
    >
      {error && <ErrorBanner message={error} />}
      {loading && <LoadingPage />}
      {data && (
        <div className="stack">
          <div className="row" style={{ gap: 16, flexWrap: "wrap" }}>
            <Badge tone={data.state === "posted" ? "ok" : data.state === "cancelled" ? "danger" : "outline"}>{data.state}</Badge>
            {data.reversal_of && <span style={{ color: "var(--text-muted)", font: "var(--type-body)" }}>reverses {data.reversal_of}</span>}
            <span style={{ color: "var(--text-muted)", font: "var(--type-body)" }}>recorded {new Date(data.recorded_at).toLocaleString("en-GB")}</span>
          </div>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Account</th>
                <th>Memo</th>
                <th style={{ textAlign: "right" }}>Debit</th>
                <th style={{ textAlign: "right" }}>Credit</th>
              </tr>
            </thead>
            <tbody>
              {data.lines.map((l, i) => (
                <tr key={l.line_id ?? i}>
                  <td style={{ font: "var(--type-label)" }}>{l.account_code} · {l.account_name}</td>
                  <td style={{ color: "var(--text-muted)", fontSize: 13 }}>{l.memo}</td>
                  <td className="num" style={{ textAlign: "right" }}>{l.side === "debit" ? formatCents(l.amount_cents) : ""}</td>
                  <td className="num" style={{ textAlign: "right" }}>{l.side === "credit" ? formatCents(l.amount_cents) : ""}</td>
                </tr>
              ))}
              <tr style={{ borderTop: "2px solid var(--border-strong)" }}>
                <td colSpan={2} style={{ font: "var(--type-label)" }}>Totals</td>
                <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(debit)}</td>
                <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(credit)}</td>
              </tr>
            </tbody>
          </table>
          {err && <ErrorBanner message={err} />}
        </div>
      )}
    </Modal>
  );
}

interface DraftLine {
  account_id: string;
  side: Side;
  amount: string;
  memo: string;
}

function ManualJournalModal({ onClose, onPosted }: { onClose: () => void; onPosted: () => void }) {
  const r = useRestaurant();
  const { data: accounts } = useLoad(() => api.listAccounts(r.id), [r.id]);
  const [date, setDate] = useState("2026-08-24");
  const [memo, setMemo] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([
    { account_id: "", side: "debit", amount: "", memo: "" },
    { account_id: "", side: "credit", amount: "", memo: "" },
  ]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const postable = (accounts ?? []).filter((a) => a.postable);
  const cents = (v: string) => parseDollars(v) ?? 0;
  const debit = lines.filter((l) => l.side === "debit").reduce((a, l) => a + cents(l.amount), 0);
  const credit = lines.filter((l) => l.side === "credit").reduce((a, l) => a + cents(l.amount), 0);
  const balanced = debit === credit && debit > 0;
  const complete = lines.every((l) => l.account_id && cents(l.amount) > 0);

  const setLine = (i: number, patch: Partial<DraftLine>) => setLines(lines.map((l, x) => (x === i ? { ...l, ...patch } : l)));

  const post = () => {
    if (!balanced || !complete || busy) return;
    setBusy(true);
    setErr(null);
    const input: ManualJournalInput = {
      accounting_date: date,
      memo,
      lines: lines.map((l) => ({ account_id: l.account_id, side: l.side, amount_cents: cents(l.amount), memo: l.memo || undefined })),
    };
    api
      .postManualJournal(r.id, input, true)
      .then(() => {
        onPosted();
        onClose();
      })
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not post the journal.");
        setBusy(false);
      });
  };

  return (
    <Modal
      title="Manual journal"
      onClose={onClose}
      wide
      footer={
        <div className="row" style={{ justifyContent: "space-between", width: "100%", alignItems: "center" }}>
          <span className="row" style={{ gap: 12, font: "var(--type-body)" }}>
            <span>Debit {formatCents(debit)}</span>
            <span>Credit {formatCents(credit)}</span>
            {balanced ? <Badge tone="ok">balanced</Badge> : <Badge tone="danger">must balance</Badge>}
          </span>
          <button className="btn btn-primary" disabled={!balanced || !complete || busy} onClick={post}>
            Post journal
          </button>
        </div>
      }
    >
      <div className="stack">
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <label className="field" style={{ maxWidth: 180 }}>
            <span className="field-label">Accounting date</span>
            <input className="input" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <label className="field" style={{ flex: 1, minWidth: 220 }}>
            <span className="field-label">Memo</span>
            <input className="input" value={memo} onChange={(e) => setMemo(e.target.value)} placeholder="What is this entry for?" />
          </label>
        </div>

        <table className="table-plain">
          <thead>
            <tr>
              <th>Account</th>
              <th>Side</th>
              <th style={{ textAlign: "right" }}>Amount</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {lines.map((l, i) => (
              <tr key={i}>
                <td>
                  <select className="select" value={l.account_id} onChange={(e) => setLine(i, { account_id: e.target.value })}>
                    <option value="">Select account…</option>
                    {postable.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.code} · {a.name}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <div className="seg">
                    <button className={l.side === "debit" ? "on" : ""} onClick={() => setLine(i, { side: "debit" })}>
                      Debit
                    </button>
                    <button className={l.side === "credit" ? "on" : ""} onClick={() => setLine(i, { side: "credit" })}>
                      Credit
                    </button>
                  </div>
                </td>
                <td style={{ textAlign: "right" }}>
                  <input
                    className="input num"
                    style={{ maxWidth: 110, textAlign: "right" }}
                    inputMode="decimal"
                    placeholder="0.00"
                    value={l.amount}
                    onChange={(e) => setLine(i, { amount: e.target.value.replace(/[^0-9.]/g, "").slice(0, 12) })}
                  />
                </td>
                <td style={{ textAlign: "right" }}>
                  {lines.length > 2 && (
                    <button className="btn btn-ghost btn-sm" onClick={() => setLines(lines.filter((_, x) => x !== i))}>
                      Remove
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div>
          <button className="btn btn-ghost btn-sm" onClick={() => setLines([...lines, { account_id: "", side: "debit", amount: "", memo: "" }])}>
            <Plus size={14} /> Add line
          </button>
        </div>
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}
