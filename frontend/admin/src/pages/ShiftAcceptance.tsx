import { ArrowLeft, Check, Lock } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Account, CostCenter, ShiftAcceptance } from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents } from "../../../design-system/shared/money";
import { useLoad } from "../lib/useLoad";
import { Badge, ErrorBanner, LoadingPage, NoticeBanner } from "../ui";
import { Variance } from "./Shifts";

export default function ShiftAcceptancePage() {
  const restaurant = useRestaurant();
  const navigate = useNavigate();
  const { shiftId = "" } = useParams();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const { data, setData, error, loading, reload } = useLoad(
    () =>
      Promise.all([
        api.getAcceptance(restaurant.id, shiftId),
        api.listAccounts(restaurant.id),
        api.listCostCenters(restaurant.id),
      ]).then(([acc, accounts, costCenters]) => ({ acc, accounts, costCenters })),
    [restaurant.id, shiftId],
  );

  const override = (lineId: string, patch: { account_id?: string; cost_center_id?: string }) => {
    setErr(null);
    api
      .patchAcceptance(restaurant.id, shiftId, [{ line_id: lineId, ...patch }])
      .then((acc) => setData((d) => (d ? { ...d, acc } : d)))
      .catch((e: { message?: string }) => setErr(e.message ?? "Could not apply the override."));
  };

  const accept = () => {
    setBusy(true);
    setErr(null);
    api
      .acceptShift(restaurant.id, shiftId)
      .then(() => navigate("/shifts"))
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not accept the shift.");
        setBusy(false);
      });
  };

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <Link to="/shifts" className="row" style={{ gap: 6, font: "var(--type-label)", borderBottom: "none", marginBottom: 8 }}>
            <ArrowLeft size={15} /> Shift acceptance
          </Link>
          <h1 className="page-title">{data ? data.acc.shift.number : "Shift"}</h1>
          <p className="page-sub">
            Every line of the draft journal, before it hits the ledger. Reassign an account or cost-centre if the cashier's mapping was wrong — that
            is the only writable moment. Accepting posts it, immutable thereafter (corrections go through a reversal).
          </p>
        </div>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}

      {data && (
        <AcceptanceBody
          data={data}
          busy={busy}
          err={err}
          onOverride={override}
          onAccept={accept}
        />
      )}
    </div>
  );
}

function AcceptanceBody({
  data,
  busy,
  err,
  onOverride,
  onAccept,
}: {
  data: { acc: ShiftAcceptance; accounts: Account[]; costCenters: CostCenter[] };
  busy: boolean;
  err: string | null;
  onOverride: (lineId: string, patch: { account_id?: string; cost_center_id?: string }) => void;
  onAccept: () => void;
}) {
  const { acc, accounts, costCenters } = data;
  const { shift, document, variance_cents, balanced } = acc;
  const debit = document.lines.filter((l) => l.side === "debit").reduce((a, l) => a + l.amount_cents, 0);
  const credit = document.lines.filter((l) => l.side === "credit").reduce((a, l) => a + l.amount_cents, 0);
  const postable = accounts.filter((a) => a.postable);

  return (
    <div className="stack">
      <div className="card row" style={{ justifyContent: "space-between", flexWrap: "wrap", gap: 16 }}>
        <Kv label="Cashier" value={shift.cashier} />
        <Kv label="Expected cash" value={formatCents(shift.expected_cents)} />
        <Kv label="Declared cash" value={formatCents(shift.declared_cents)} />
        <div>
          <div className="field-label">Variance</div>
          <Variance cents={variance_cents} />
        </div>
        <div>
          <div className="field-label">Journal</div>
          {balanced ? <Badge tone="ok">balanced</Badge> : <Badge tone="danger">unbalanced</Badge>}
        </div>
      </div>

      {variance_cents !== 0 && (
        <NoticeBanner>
          Variance of {formatCents(Math.abs(variance_cents))} posts as a cash over/short line ({variance_cents < 0 ? "shortage" : "surplus"}) — the
          journal balances only because that entry is present.
        </NoticeBanner>
      )}

      <div className="card" style={{ padding: 0 }}>
        <table className="table-plain">
          <thead>
            <tr>
              <th>Account</th>
              <th>Cost centre</th>
              <th>Memo</th>
              <th style={{ textAlign: "right" }}>Debit</th>
              <th style={{ textAlign: "right" }}>Credit</th>
            </tr>
          </thead>
          <tbody>
            {document.lines.map((l) => (
              <tr key={l.line_id}>
                <td>
                  {l.editable ? (
                    <select
                      className="select"
                      value={l.account_id}
                      onChange={(e) => onOverride(l.line_id!, { account_id: e.target.value })}
                    >
                      {postable.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.code} · {a.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <span style={{ font: "var(--type-label)" }}>
                      {l.account_code} · {l.account_name}
                    </span>
                  )}
                </td>
                <td>
                  {l.editable ? (
                    <select
                      className="select"
                      value={l.cost_center_id}
                      onChange={(e) => onOverride(l.line_id!, { cost_center_id: e.target.value })}
                    >
                      {costCenters.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    costCenters.find((c) => c.id === l.cost_center_id)?.name ?? l.cost_center_id
                  )}
                </td>
                <td style={{ color: "var(--text-muted)", fontSize: 13 }}>{l.memo}</td>
                <td className="num" style={{ textAlign: "right" }}>{l.side === "debit" ? formatCents(l.amount_cents) : ""}</td>
                <td className="num" style={{ textAlign: "right" }}>{l.side === "credit" ? formatCents(l.amount_cents) : ""}</td>
              </tr>
            ))}
            <tr style={{ borderTop: "2px solid var(--border-strong)" }}>
              <td colSpan={3} style={{ font: "var(--type-label)" }}>Totals</td>
              <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(debit)}</td>
              <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(credit)}</td>
            </tr>
          </tbody>
        </table>
      </div>

      {err && <ErrorBanner message={err} />}

      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <span className="row" style={{ gap: 6, color: "var(--text-muted)", font: "var(--type-body)" }}>
          <Lock size={14} /> Accounting date {document.accounting_date}. Posting is irreversible except by reversal.
        </span>
        <button className="btn btn-primary" disabled={busy || !balanced} onClick={onAccept}>
          <Check size={16} /> Accept &amp; post
        </button>
      </div>
    </div>
  );
}

function Kv({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="field-label">{label}</div>
      <div className="num" style={{ font: "var(--type-label)" }}>{value}</div>
    </div>
  );
}
