import { Plus, Trash } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { WriteOffInput, WriteOffReason } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatCents } from "../../../../design-system/shared/money";
import { EmptyState, ErrorBanner, Field, LoadingPage, Modal } from "../../ui";
import { EditLine, LineEditor, StatusBadge, stockable, useProducts } from "./parts";

const TODAY = "2026-08-24";
const REASONS: { value: WriteOffReason; label: string }[] = [
  { value: "spoilage", label: "Spoilage" },
  { value: "expiry", label: "Expiry" },
  { value: "staff_meal", label: "Staff meal" },
  { value: "loss", label: "Loss" },
  { value: "other", label: "Other" },
];
const reasonLabel = (v: WriteOffReason) => REASONS.find((x) => x.value === v)?.label ?? v;

export default function WriteOffs() {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listWriteOffs(r.id), [r.id]);
  const [create, setCreate] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn btn-primary" onClick={() => setCreate(true)}>
          <Plus size={16} /> New write-off
        </button>
      </div>

      {error && <ErrorBanner message={error} onRetry={reload} />}
      {loading && <LoadingPage />}
      {data && data.length === 0 && (
        <div className="card">
          <EmptyState icon={Trash} title="No write-offs" message="Record spoilage, staff meals and losses." />
        </div>
      )}
      {data && data.length > 0 && (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Date</th>
                <th>Reason</th>
                <th>Status</th>
                <th style={{ textAlign: "right" }}>Cost</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((w) => (
                <tr key={w.id}>
                  <td className="num" style={{ fontSize: 13 }}>{w.business_date}</td>
                  <td>{reasonLabel(w.reason)}</td>
                  <td>
                    <StatusBadge status={w.status} />
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{w.status === "posted" ? formatCents(w.total_cents) : "—"}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setOpen(w.id)}>
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {create && <NewWriteOffModal onClose={() => setCreate(false)} onSaved={reload} />}
      {open && <WriteOffModal wid={open} onClose={() => setOpen(null)} onChanged={reload} />}
    </div>
  );
}

function NewWriteOffModal({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const r = useRestaurant();
  const { data: products } = useProducts();
  const [reason, setReason] = useState<WriteOffReason>("spoilage");
  const [date, setDate] = useState(TODAY);
  const [note, setNote] = useState("");
  const [lines, setLines] = useState<EditLine[]>([{ product_id: "", qty: "", unit: "g" }]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const valid = lines.filter((l) => l.product_id && parseFloat(l.qty) > 0);

  const save = () => {
    if (valid.length === 0) {
      setErr("Add at least one line.");
      return;
    }
    setBusy(true);
    setErr(null);
    const input: WriteOffInput = {
      reason,
      note: note || undefined,
      business_date: date,
      lines: valid.map((l) => ({ product_id: l.product_id, qty: parseFloat(l.qty), unit: l.unit })),
    };
    api
      .createWriteOff(r.id, input)
      .then(() => {
        onSaved();
        onClose();
      })
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not save.");
        setBusy(false);
      });
  };

  return (
    <Modal
      title="New write-off"
      onClose={onClose}
      wide
      footer={
        <button className="btn btn-primary" disabled={busy} onClick={save}>
          Save draft
        </button>
      }
    >
      <div className="stack">
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <Field label="Reason">
            <select className="select" value={reason} onChange={(e) => setReason(e.target.value as WriteOffReason)}>
              {REASONS.map((x) => (
                <option key={x.value} value={x.value}>
                  {x.label}
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
        <LineEditor products={stockable(products ?? [])} lines={lines} setLines={setLines} />
        <span className="field-hint">Cost is computed at the current weighted average when the document is posted.</span>
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}

function WriteOffModal({ wid, onClose, onChanged }: { wid: string; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.getWriteOff(r.id, wid), [r.id, wid]);
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
      title={data ? `Write-off · ${data.business_date}` : "Write-off"}
      onClose={onClose}
      wide
      footer={
        data && data.status === "draft" ? (
          <button className="btn btn-primary" disabled={busy} onClick={() => act(() => api.postWriteOff(r.id, wid))}>
            Post write-off
          </button>
        ) : data && data.status === "posted" ? (
          <button className="btn btn-danger" disabled={busy} onClick={() => act(() => api.cancelWriteOff(r.id, wid))}>
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
            <span style={{ color: "var(--text-muted)" }}>{reasonLabel(data.reason)}</span>
            {data.note && <span style={{ color: "var(--text-muted)" }}>{data.note}</span>}
          </div>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Product</th>
                <th style={{ textAlign: "right" }}>Qty</th>
                <th style={{ textAlign: "right" }}>Cost</th>
              </tr>
            </thead>
            <tbody>
              {data.lines.map((l) => (
                <tr key={l.id}>
                  <td>{l.product_name}</td>
                  <td className="num" style={{ textAlign: "right" }}>{l.qty_input} {l.input_unit}</td>
                  <td className="num" style={{ textAlign: "right" }}>{data.status === "posted" ? formatCents(l.cost_cents) : "—"}</td>
                </tr>
              ))}
              {data.status === "posted" && (
                <tr style={{ borderTop: "2px solid var(--border-strong)" }}>
                  <td colSpan={2} style={{ font: "var(--type-label)" }}>Total</td>
                  <td className="num" style={{ textAlign: "right", fontWeight: 600 }}>{formatCents(data.total_cents)}</td>
                </tr>
              )}
            </tbody>
          </table>
          {err && <ErrorBanner message={err} />}
        </div>
      )}
    </Modal>
  );
}
