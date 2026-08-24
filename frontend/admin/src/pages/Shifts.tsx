import { ClipboardCheck } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { ShiftState } from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents } from "../../../design-system/shared/money";
import { useLoad } from "../lib/useLoad";
import { Badge, EmptyState, ErrorBanner, LoadingPage } from "../ui";

function fmtDateTime(iso: string): string {
  return new Date(iso).toLocaleString("en-GB", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

export function Variance({ cents }: { cents: number }) {
  const color = cents === 0 ? "var(--green-700)" : "var(--red-700)";
  return (
    <span className="num" style={{ color, fontWeight: cents === 0 ? 400 : 600 }}>
      {cents > 0 ? "+" : ""}
      {formatCents(cents)}
    </span>
  );
}

export default function Shifts() {
  const restaurant = useRestaurant();
  const [state, setState] = useState<ShiftState>("closed");
  const { data, error, loading, reload } = useLoad(() => api.listShifts(restaurant.id, state), [restaurant.id, state]);

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Shift acceptance</h1>
          <p className="page-sub">
            Cashiers close shifts on the POS; each builds a draft journal. Review it here, override account assignments if needed, then accept to post
            it to the ledger.
          </p>
        </div>
      </div>

      <div className="stack">
        <div className="seg" style={{ alignSelf: "flex-start" }}>
          <button className={state === "closed" ? "on" : ""} onClick={() => setState("closed")}>
            Awaiting acceptance
          </button>
          <button className={state === "accepted" ? "on" : ""} onClick={() => setState("accepted")}>
            Accepted
          </button>
        </div>

        {error && <ErrorBanner message={error} onRetry={reload} />}
        {loading && <LoadingPage />}

        {data && data.length === 0 && (
          <div className="card">
            <EmptyState
              icon={ClipboardCheck}
              title={state === "closed" ? "Nothing to accept" : "No accepted shifts yet"}
              message={state === "closed" ? "Closed shifts appear here for review the moment a cashier ends one." : "Shifts you accept move here with their posted journal."}
            />
          </div>
        )}

        {data && data.length > 0 && (
          <div className="card" style={{ padding: 0 }}>
            <table className="table-plain">
              <thead>
                <tr>
                  <th>Shift</th>
                  <th>Cashier</th>
                  <th>Closed</th>
                  <th style={{ textAlign: "right" }}>Expected</th>
                  <th style={{ textAlign: "right" }}>Declared</th>
                  <th style={{ textAlign: "right" }}>Variance</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {data.map((s) => (
                  <tr key={s.id}>
                    <td style={{ font: "var(--type-label)" }}>{s.number}</td>
                    <td style={{ color: "var(--text-muted)" }}>{s.cashier}</td>
                    <td className="num" style={{ fontSize: 12 }}>{fmtDateTime(s.closed_at)}</td>
                    <td className="num" style={{ textAlign: "right" }}>{formatCents(s.expected_cents)}</td>
                    <td className="num" style={{ textAlign: "right" }}>{formatCents(s.declared_cents)}</td>
                    <td style={{ textAlign: "right" }}>
                      <Variance cents={s.variance_cents} />
                    </td>
                    <td style={{ textAlign: "right" }}>
                      {s.state === "closed" ? (
                        <Link to={`/shifts/${s.id}`} style={{ font: "var(--type-label)", borderBottom: "none" }}>
                          Review →
                        </Link>
                      ) : (
                        <Badge tone="ok">accepted</Badge>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
