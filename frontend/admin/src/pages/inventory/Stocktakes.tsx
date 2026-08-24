import { ClipboardList, Plus } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { DocLineInput, Product, Stocktake, StocktakePreview } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatQty } from "../../lib/units";
import { formatCents } from "../../../../design-system/shared/money";
import { Badge, EmptyState, ErrorBanner, LoadingPage, Modal, NoticeBanner } from "../../ui";
import { StatusBadge, stockable } from "./parts";

export default function Stocktakes() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listStocktakes(r.id), [r.id]);
  const [open, setOpen] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [startErr, setStartErr] = useState<string | null>(null);

  const hasDraft = (data ?? []).some((s) => s.status === "draft");

  const start = () => {
    setBusy(true);
    setStartErr(null);
    api
      .createStocktake(r.id, {})
      .then((s) => {
        reload();
        setOpen(s.id);
      })
      .catch((e: { message?: string }) => setStartErr(e.message ?? "Could not start."))
      .finally(() => setBusy(false));
  };

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn btn-primary" disabled={busy || hasDraft} onClick={start}>
          <Plus size={16} /> Start stocktake
        </button>
      </div>
      {hasDraft && <NoticeBanner>An open stocktake exists — finish or cancel it before starting another.</NoticeBanner>}
      {startErr && <ErrorBanner message={startErr} />}

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && data.length === 0 && (
        <div className="card">
          <EmptyState icon={ClipboardList} title="No stocktakes" message="Count physical stock and reconcile against the books." />
        </div>
      )}
      {data && data.length > 0 && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Date</th>
                <th>Status</th>
                <th>Lines</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((s) => (
                <tr key={s.id}>
                  <td className="num" style={{ fontSize: 13 }}>{s.business_date ?? (s.status === "draft" ? "draft" : "—")}</td>
                  <td>
                    <StatusBadge status={s.status} />
                  </td>
                  <td style={{ color: "var(--text-muted)" }}>{s.lines.length}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setOpen(s.id)}>
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open && <StocktakeModal sid={open} onClose={() => setOpen(null)} onChanged={reload} />}
    </div>
  );
}

function StocktakeModal({ sid, onClose, onChanged }: { sid: string; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(
    () => Promise.all([api.getStocktake(r.id, sid), api.listProducts(r.id)]).then(([st, products]) => ({ st, products })),
    [r.id, sid],
  );
  if (error) return <Modal title="Stocktake" onClose={onClose}><ErrorBanner message={error} /></Modal>;
  if (loading || !data) return <Modal title="Stocktake" onClose={onClose}><LoadingPage /></Modal>;
  return <StocktakeBody st={data.st} products={data.products} onClose={onClose} onChanged={() => { reload(); onChanged(); }} />;
}

function StocktakeBody({ st, products, onClose, onChanged }: { st: Stocktake; products: Product[]; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const draft = st.status === "draft";
  const items = stockable(products);
  // counts keyed by product id; seed from saved lines.
  const [counts, setCounts] = useState<Record<string, string>>(() => {
    const c: Record<string, string> = {};
    for (const l of st.lines) c[l.product_id] = String(l.counted_qty / 1000);
    return c;
  });
  const [preview, setPreview] = useState<StocktakePreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const linesInput = (): DocLineInput[] =>
    items
      .filter((p) => counts[p.id]?.trim())
      .map((p) => ({ product_id: p.id, qty: parseFloat(counts[p.id]), unit: p.stock_unit }));

  const run = (after: (id: string) => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    api
      .patchStocktake(r.id, st.id, linesInput())
      .then(() => after(st.id))
      .then((res) => {
        onChanged();
        return res;
      })
      .catch((e: { message?: string }) => setErr(e.message ?? "Action failed."))
      .finally(() => setBusy(false));
  };

  const doDryRun = () => {
    setBusy(true);
    setErr(null);
    api
      .patchStocktake(r.id, st.id, linesInput())
      .then(() => api.dryRunStocktake(r.id, st.id))
      .then((p) => setPreview(p))
      .catch((e: { message?: string }) => setErr(e.message ?? "Dry-run failed."))
      .finally(() => setBusy(false));
  };

  const doPost = () => run((id) => api.postStocktake(r.id, id).then(() => onClose()));
  const doCancel = () => {
    setBusy(true);
    setErr(null);
    api
      .cancelStocktake(r.id, st.id)
      .then(() => {
        onChanged();
        onClose();
      })
      .catch((e: { message?: string }) => setErr(e.message ?? "Cancel failed."))
      .finally(() => setBusy(false));
  };

  const previewFor = (pid: string) => preview?.lines.find((l) => l.product_id === pid);

  return (
    <Modal
      title={`Stocktake · ${st.status}`}
      onClose={onClose}
      wide
      footer={
        draft ? (
          <div className="row" style={{ gap: 8, width: "100%" }}>
            <button className="btn btn-secondary" disabled={busy} onClick={doDryRun}>
              Preview (dry-run)
            </button>
            <button className="btn btn-ghost" disabled={busy} onClick={() => run(async () => {})}>
              Save counts
            </button>
            <button className="btn btn-primary" disabled={busy} style={{ marginLeft: "auto" }} onClick={doPost}>
              Post
            </button>
          </div>
        ) : st.status === "posted" ? (
          <button className="btn btn-danger" disabled={busy} onClick={doCancel}>
            Cancel (reverse)
          </button>
        ) : null
      }
    >
      <div className="stack">
        {draft ? (
          <NoticeBanner>
            Enter counted quantities in each product's stock unit. Dry-run previews variances without saving any movement; posting fixes expected quantities at that moment and books surpluses/shortages.
          </NoticeBanner>
        ) : (
          <div className="row" style={{ gap: 12 }}>
            <StatusBadge status={st.status} />
            {st.business_date && <span style={{ color: "var(--text-muted)", font: "var(--type-body)" }}>{st.business_date}</span>}
          </div>
        )}

        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Product</th>
                <th style={{ textAlign: "right" }}>Counted</th>
                {(preview || !draft) && <th style={{ textAlign: "right" }}>Expected</th>}
                {(preview || !draft) && <th style={{ textAlign: "right" }}>Variance</th>}
                {(preview || !draft) && <th style={{ textAlign: "right" }}>Cost</th>}
              </tr>
            </thead>
            <tbody>
              {draft
                ? items.map((p) => {
                    const pv = previewFor(p.id);
                    return (
                      <tr key={p.id}>
                        <td>
                          {p.sku} · {p.name}
                        </td>
                        <td style={{ textAlign: "right" }}>
                          <input
                            className="input num"
                            style={{ maxWidth: 110, textAlign: "right" }}
                            inputMode="decimal"
                            placeholder={p.stock_unit}
                            value={counts[p.id] ?? ""}
                            onChange={(e) => setCounts({ ...counts, [p.id]: e.target.value.replace(/[^0-9.]/g, "") })}
                          />
                        </td>
                        {preview && <td className="num" style={{ textAlign: "right" }}>{pv ? formatQty(pv.expected_qty ?? 0, p.stock_unit) : ""}</td>}
                        {preview && <td className="num" style={{ textAlign: "right" }}>{pv && pv.variance_qty != null ? <VarQty milli={pv.variance_qty} unit={p.stock_unit} /> : ""}</td>}
                        {preview && <td className="num" style={{ textAlign: "right" }}>{pv && pv.variance_cost_cents != null ? formatCents(pv.variance_cost_cents) : ""}</td>}
                      </tr>
                    );
                  })
                : st.lines.map((l) => (
                    <tr key={l.id}>
                      <td>{l.product_name}</td>
                      <td className="num" style={{ textAlign: "right" }}>{formatQty(l.counted_qty, l.unit)}</td>
                      <td className="num" style={{ textAlign: "right" }}>{formatQty(l.expected_qty ?? 0, l.unit)}</td>
                      <td className="num" style={{ textAlign: "right" }}>{l.variance_qty != null ? <VarQty milli={l.variance_qty} unit={l.unit} /> : ""}</td>
                      <td className="num" style={{ textAlign: "right" }}>{l.variance_cost_cents != null ? formatCents(l.variance_cost_cents) : ""}</td>
                    </tr>
                  ))}
            </tbody>
          </table>
        </div>
        {preview && (
          <span className="field-hint">
            Preview only — nothing saved. Net variance {formatCents(preview.total_variance_cost_cents)}.
          </span>
        )}
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}

function VarQty({ milli, unit }: { milli: number; unit: Product["stock_unit"] }) {
  if (milli === 0) return <span style={{ color: "var(--text-muted)" }}>0</span>;
  const tone = milli > 0 ? "ok" : "danger";
  return <Badge tone={tone}>{milli > 0 ? "+" : ""}{formatQty(milli, unit)}</Badge>;
}
