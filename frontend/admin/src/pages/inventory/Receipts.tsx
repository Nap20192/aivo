import { FileText, Plus } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { GoodsReceiptInput, Product, Supplier } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatCents, parseDollars } from "../../../../design-system/shared/money";
import { EmptyState, ErrorBanner, Field, LoadingPage, Modal } from "../../ui";
import { EditLine, LineEditor, StatusBadge, stockable } from "./parts";

const TODAY = "2026-08-24";

export default function Receipts() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listReceipts(r.id), [r.id]);
  const [create, setCreate] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn btn-primary" onClick={() => setCreate(true)}>
          <Plus size={16} /> New receipt
        </button>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && data.length === 0 && (
        <div className="card">
          <EmptyState icon={FileText} title="No receipts" message="Record deliveries from suppliers as goods receipts." />
        </div>
      )}
      {data && data.length > 0 && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Date</th>
                <th>Supplier</th>
                <th>Status</th>
                <th style={{ textAlign: "right" }}>Total</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((rc) => (
                <tr key={rc.id}>
                  <td className="num" style={{ fontSize: 13 }}>{rc.business_date}</td>
                  <td>{rc.supplier_name ?? "—"}</td>
                  <td>
                    <StatusBadge status={rc.status} />
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(rc.total_cents)}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setOpen(rc.id)}>
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {create && <NewReceiptModal onClose={() => setCreate(false)} onSaved={reload} />}
      {open && <ReceiptModal rid={open} onClose={() => setOpen(null)} onChanged={reload} />}
    </div>
  );
}

function NewReceiptModal({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const r = useRestaurant();
  const { data } = useLoad(
    () => Promise.all([api.listProducts(r.id), api.listSuppliers(r.id)]).then(([products, suppliers]) => ({ products, suppliers })),
    [r.id],
  );
  const [supplierId, setSupplierId] = useState("");
  const [date, setDate] = useState(TODAY);
  const [note, setNote] = useState("");
  const [lines, setLines] = useState<EditLine[]>([{ product_id: "", qty: "", unit: "g", unit_price: "" }]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const products: Product[] = data ? stockable(data.products) : [];
  const suppliers: Supplier[] = data ? data.suppliers.filter((s) => !s.archived) : [];
  const valid = lines.filter((l) => l.product_id && parseFloat(l.qty) > 0);
  const total = valid.reduce((a, l) => a + (parseDollars(l.unit_price ?? "0") ?? 0) * parseFloat(l.qty), 0);

  const save = () => {
    if (valid.length === 0) {
      setErr("Add at least one line with a product and quantity.");
      return;
    }
    setBusy(true);
    setErr(null);
    const input: GoodsReceiptInput = {
      supplier_id: supplierId || null,
      business_date: date,
      note: note || undefined,
      lines: valid.map((l) => ({
        product_id: l.product_id,
        qty: parseFloat(l.qty),
        unit: l.unit,
        unit_price_cents: parseDollars(l.unit_price ?? "0") ?? 0,
      })),
    };
    api
      .createReceipt(r.id, input)
      .then(() => {
        onSaved();
        onClose();
      })
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not save the receipt.");
        setBusy(false);
      });
  };

  return (
    <Modal
      title="New goods receipt"
      onClose={onClose}
      wide
      footer={
        <div className="row" style={{ justifyContent: "space-between", width: "100%", alignItems: "center" }}>
          <span style={{ font: "var(--type-label)" }}>Total {formatCents(Math.round(total))}</span>
          <button className="btn btn-primary" disabled={busy} onClick={save}>
            Save draft
          </button>
        </div>
      }
    >
      <div className="stack">
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <Field label="Supplier">
            <select className="select" value={supplierId} onChange={(e) => setSupplierId(e.target.value)}>
              <option value="">No supplier</option>
              {suppliers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Business date">
            <input className="input" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
          <div style={{ flex: 1, minWidth: 200 }}>
            <Field label="Note">
              <input className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Optional" />
            </Field>
          </div>
        </div>
        <LineEditor products={products} lines={lines} setLines={setLines} withPrice />
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}

function ReceiptModal({ rid, onClose, onChanged }: { rid: string; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.getReceipt(r.id, rid), [r.id, rid]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const act = (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    fn()
      .then(() => {
        reload();
        onChanged();
      })
      .catch((e: { message?: string }) => setErr(e.message ?? "Action failed."))
      .finally(() => setBusy(false));
  };

  return (
    <Modal
      title={data ? `Receipt · ${data.business_date}` : "Receipt"}
      onClose={onClose}
      wide
      footer={
        data && data.status === "draft" ? (
          <button className="btn btn-primary" disabled={busy} onClick={() => act(() => api.postReceipt(r.id, rid))}>
            Post receipt
          </button>
        ) : data && data.status === "posted" ? (
          <button className="btn btn-danger" disabled={busy} onClick={() => act(() => api.cancelReceipt(r.id, rid))}>
            Cancel (reverse)
          </button>
        ) : null
      }
    >
      {error && <ErrorBanner message={error} />}
      {loading && <LoadingPage />}
      {data && (
        <div className="stack">
          <div className="row" style={{ gap: 16, flexWrap: "wrap", font: "var(--type-body)" }}>
            <StatusBadge status={data.status} />
            <span style={{ color: "var(--text-muted)" }}>{data.supplier_name ?? "No supplier"}</span>
            {data.posted_at && <span style={{ color: "var(--text-muted)" }}>posted {new Date(data.posted_at).toLocaleString("en-GB")}</span>}
            {data.note && <span style={{ color: "var(--text-muted)" }}>{data.note}</span>}
          </div>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Product</th>
                <th style={{ textAlign: "right" }}>Qty</th>
                <th style={{ textAlign: "right" }}>Price / unit</th>
                <th style={{ textAlign: "right" }}>Line cost</th>
              </tr>
            </thead>
            <tbody>
              {data.lines.map((l) => (
                <tr key={l.id}>
                  <td>{l.product_name}</td>
                  <td className="num" style={{ textAlign: "right" }}>{l.qty_input} {l.input_unit}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(l.unit_price_cents)}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(l.line_cost_cents)}</td>
                </tr>
              ))}
              <tr style={{ borderTop: "2px solid var(--border-strong)" }}>
                <td colSpan={3} style={{ font: "var(--type-label)" }}>Total</td>
                <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(data.total_cents)}</td>
              </tr>
            </tbody>
          </table>
          {data.status === "posted" && (
            <span className="field-hint">
              Posting added {data.lines.length} stock move{data.lines.length === 1 ? "" : "s"} and posted GL: debit Inventory / credit Accounts payable.
            </span>
          )}
          {err && <ErrorBanner message={err} />}
        </div>
      )}
    </Modal>
  );
}
