import { useState } from "react";
import { api } from "../../api/client";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatQty } from "../../lib/units";
import { formatCents } from "../../../../design-system/shared/money";
import { Badge, ErrorBanner, LoadingPage } from "../../ui";

const MOVE_TONE = {
  receipt: "ok",
  sale: "neutral",
  writeoff: "warn",
  stocktake_surplus: "info",
  stocktake_shortage: "warn",
  reversal: "danger",
} as const;

export function StockLevels() {
  const [view, setView] = useState<"on-hand" | "moves">("on-hand");
  return (
    <div className="stack">
      <div className="seg" style={{ alignSelf: "flex-start" }}>
        <button className={view === "on-hand" ? "on" : ""} onClick={() => setView("on-hand")}>
          On hand
        </button>
        <button className={view === "moves" ? "on" : ""} onClick={() => setView("moves")}>
          Movements
        </button>
      </div>
      {view === "on-hand" ? <OnHandTable /> : <MovesTable />}
    </div>
  );
}

function OnHandTable() {
  const r = useRestaurant();
  const [lowOnly, setLowOnly] = useState(false);
  const { data, error, loading, reload } = useLoad(() => api.onHand(r.id, lowOnly), [r.id, lowOnly]);

  return (
    <div className="stack">
      <label className="row" style={{ gap: 8, font: "var(--type-body)" }}>
        <input type="checkbox" checked={lowOnly} onChange={(e) => setLowOnly(e.target.checked)} />
        Below minimum only
      </label>
      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>SKU</th>
                <th>Name</th>
                <th style={{ textAlign: "right" }}>On hand</th>
                <th style={{ textAlign: "right" }}>Avg cost</th>
                <th style={{ textAlign: "right" }}>Value</th>
              </tr>
            </thead>
            <tbody>
              {data.map((o) => (
                <tr key={o.product_id}>
                  <td className="num" style={{ font: "var(--type-label)" }}>{o.sku}</td>
                  <td>{o.name}</td>
                  <td className="num" style={{ textAlign: "right" }}>
                    {formatQty(o.qty, o.unit)}
                    {o.below_min && (
                      <>
                        {" "}
                        <Badge tone="warn">low</Badge>
                      </>
                    )}
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(o.avg_cents)}/{o.unit}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(o.value_cents)}</td>
                </tr>
              ))}
              {data.length === 0 && (
                <tr>
                  <td colSpan={5} style={{ color: "var(--text-muted)", textAlign: "center", padding: 24 }}>
                    Nothing to show.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function MovesTable() {
  const r = useRestaurant();
  const [from, setFrom] = useState("2026-08-01");
  const { data, error, loading, reload } = useLoad(() => api.stockMoves(r.id, { from }), [r.id, from]);

  return (
    <div className="stack">
      <label className="field" style={{ maxWidth: 180 }}>
        <span className="field-label">From date</span>
        <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
      </label>
      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Business date</th>
                <th>Product</th>
                <th>Kind</th>
                <th style={{ textAlign: "right" }}>Qty</th>
                <th style={{ textAlign: "right" }}>Cost</th>
                <th>Recorded</th>
              </tr>
            </thead>
            <tbody>
              {data.map((m) => (
                <tr key={m.id}>
                  <td className="num" style={{ fontSize: 13 }}>{m.business_date}</td>
                  <td>{m.product_name}</td>
                  <td>
                    <Badge tone={MOVE_TONE[m.kind]}>{m.kind.replace("_", " ")}</Badge>
                    {m.estimated && (
                      <>
                        {" "}
                        <Badge tone="warn">est</Badge>
                      </>
                    )}
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{m.qty > 0 ? "+" : ""}{formatQty(m.qty, m.unit)}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(m.cost_cents)}</td>
                  <td className="num" style={{ fontSize: 12, color: "var(--text-muted)" }}>{new Date(m.recorded_at).toLocaleString("en-GB")}</td>
                </tr>
              ))}
              {data.length === 0 && (
                <tr>
                  <td colSpan={6} style={{ color: "var(--text-muted)", textAlign: "center", padding: 24 }}>
                    No movements in range.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

const pct = (v: number) => (v * 100).toFixed(1) + "%";

export function FoodCost() {
  const r = useRestaurant();
  const [from, setFrom] = useState("2026-08-01");
  const [to, setTo] = useState("2026-08-24");
  const { data, error, loading, reload } = useLoad(() => api.foodCost(r.id, from, to), [r.id, from, to]);

  return (
    <div className="stack">
      <div className="row" style={{ gap: 12 }}>
        <label className="field" style={{ maxWidth: 170 }}>
          <span className="field-label">From</span>
          <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </label>
        <label className="field" style={{ maxWidth: 170 }}>
          <span className="field-label">To</span>
          <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </label>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && (
        <>
          <div className="stat-grid">
            <div className="card">
              <div className="stat-label">Revenue</div>
              <div className="stat-value">{formatCents(data.totals.revenue_cents)}</div>
            </div>
            <div className="card">
              <div className="stat-label">Actual COGS</div>
              <div className="stat-value">{formatCents(data.totals.actual_cogs_cents)}</div>
              <div className="stat-hint">Theoretical {formatCents(data.totals.theoretical_cogs_cents)}</div>
            </div>
            <div className="card">
              <div className="stat-label">Food cost</div>
              <div className="stat-value">{pct(data.totals.food_cost_pct)}</div>
            </div>
            <div className="card">
              <div className="stat-label">Estimated share</div>
              <div className="stat-value">{pct(data.estimated_share)}</div>
              <div className="stat-hint">Sales costed off negative stock</div>
            </div>
          </div>

          <div className="card" style={{ padding: 0 }}>
            <table className="table-plain">
              <thead>
                <tr>
                  <th>Menu item</th>
                  <th style={{ textAlign: "right" }}>Revenue</th>
                  <th style={{ textAlign: "right" }}>Actual COGS</th>
                  <th style={{ textAlign: "right" }}>Theoretical</th>
                  <th style={{ textAlign: "right" }}>Food cost %</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((it) => (
                  <tr key={it.menu_item_id}>
                    <td>{it.name}</td>
                    <td className="num" style={{ textAlign: "right" }}>{formatCents(it.revenue_cents)}</td>
                    <td className="num" style={{ textAlign: "right" }}>{formatCents(it.actual_cogs_cents)}</td>
                    <td className="num" style={{ textAlign: "right", color: "var(--text-muted)" }}>{formatCents(it.theoretical_cogs_cents)}</td>
                    <td className="num" style={{ textAlign: "right" }}>{pct(it.food_cost_pct)}</td>
                  </tr>
                ))}
                {data.items.length === 0 && (
                  <tr>
                    <td colSpan={5} style={{ color: "var(--text-muted)", textAlign: "center", padding: 24 }}>
                      No sales in range.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
