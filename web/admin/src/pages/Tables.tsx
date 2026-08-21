import { Download, Plus, QrCode, RefreshCw } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api, isMocked } from "../api/client";
import type { Table } from "../api/types";
import { useRestaurant } from "../auth";
import { useLoad } from "../lib/useLoad";
import {
  EmptyState,
  ErrorBanner,
  Field,
  LoadingPage,
  Modal,
  NoticeBanner,
} from "../ui";

export default function Tables() {
  const restaurant = useRestaurant();
  const { data, setData, error, loading, reload } = useLoad(
    () => api.listTables(restaurant.id),
    [restaurant.id],
  );
  const [adding, setAdding] = useState(false);
  const [qrTable, setQrTable] = useState<Table | null>(null);
  const [regenTable, setRegenTable] = useState<Table | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  const tableUrl = (t: Table) =>
    `${location.origin}/${restaurant.slug}/t/${t.token}`;

  async function regenerate() {
    if (!regenTable) return;
    setBusy(true);
    setActionError(null);
    try {
      const updated = await api.regenerateTableToken(
        restaurant.id,
        regenTable.id,
      );
      setData(data!.map((t) => (t.id === updated.id ? updated : t)));
      setRegenTable(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Regenerate failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Tables & QR</h1>
          <p className="page-sub">
            One QR per table. The token in the link is the diner's credential —
            regenerating it kills the old link.
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setAdding(true)}>
          <Plus size={15} />
          Add table
        </button>
      </div>

      {actionError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={actionError} />
        </div>
      )}

      {data.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={QrCode}
            title="No tables yet"
            message="Add your tables to get QR codes diners scan to open the menu."
            action={
              <button
                className="btn btn-primary btn-sm"
                onClick={() => setAdding(true)}
              >
                <Plus size={14} />
                Add table
              </button>
            }
          />
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Table</th>
                <th>Link</th>
                <th style={{ width: 1 }} />
              </tr>
            </thead>
            <tbody>
              {data.map((t) => (
                <tr key={t.id}>
                  <td style={{ font: "var(--type-label)", color: "var(--text-strong)" }}>
                    {t.label}
                  </td>
                  <td>
                    <span className="num" style={{ fontSize: 12, color: "var(--ink-600)" }}>
                      /{restaurant.slug}/t/{t.token}
                    </span>
                  </td>
                  <td>
                    <div className="row" style={{ justifyContent: "flex-end" }}>
                      <button
                        className="btn btn-secondary btn-sm"
                        onClick={() => setQrTable(t)}
                      >
                        <QrCode size={14} />
                        QR
                      </button>
                      <button
                        className="btn btn-ghost btn-sm"
                        onClick={() => setRegenTable(t)}
                      >
                        <RefreshCw size={14} />
                        Regenerate
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {adding && (
        <AddTableModal
          restaurantId={restaurant.id}
          suggested={`Table ${data.length + 1}`}
          onClose={() => setAdding(false)}
          onAdded={(t) => {
            setData([...data, t]);
            setAdding(false);
          }}
        />
      )}

      {qrTable && (
        <Modal title={qrTable.label} onClose={() => setQrTable(null)}>
          <div className="stack" style={{ alignItems: "center" }}>
            {isMocked() ? (
              <>
                <FakeQr token={qrTable.token} />
                <NoticeBanner>
                  Demo mode preview — the scannable QR image comes from the
                  server (GET …/tables/{"{id}"}/qr).
                </NoticeBanner>
              </>
            ) : (
              <div className="qr-box">
                <img
                  src={api.qrUrl(restaurant.id, qrTable.id)}
                  alt={`QR for ${qrTable.label}`}
                />
              </div>
            )}
            <span className="num" style={{ fontSize: 12 }}>
              {tableUrl(qrTable)}
            </span>
            <div className="row">
              <a
                className={"btn btn-secondary btn-sm" + (isMocked() ? " " : "")}
                href={isMocked() ? undefined : api.qrUrl(restaurant.id, qrTable.id)}
                download={`${restaurant.slug}-${qrTable.label.toLowerCase().replace(/\s+/g, "-")}-qr.png`}
                aria-disabled={isMocked()}
                onClick={(e) => {
                  if (isMocked()) e.preventDefault();
                }}
                style={isMocked() ? { opacity: 0.5, cursor: "not-allowed" } : undefined}
              >
                <Download size={14} />
                Download PNG
              </a>
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => navigator.clipboard.writeText(tableUrl(qrTable))}
              >
                Copy link
              </button>
            </div>
          </div>
        </Modal>
      )}

      {regenTable && (
        <Modal
          title="Regenerate token"
          onClose={() => setRegenTable(null)}
          footer={
            <>
              <button
                className="btn btn-secondary"
                onClick={() => setRegenTable(null)}
              >
                Cancel
              </button>
              <button
                className="btn btn-danger"
                onClick={regenerate}
                disabled={busy}
              >
                {busy ? "Regenerating…" : "Regenerate token"}
              </button>
            </>
          }
        >
          <div className="stack">
            <p style={{ font: "var(--type-body)" }}>
              This gives <strong>{regenTable.label}</strong> a new link and QR
              code.
            </p>
            <div className="notice-banner">
              The old link dies immediately. Any printed QR for this table stops
              working — reprint it after regenerating.
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

function AddTableModal(props: {
  restaurantId: string;
  suggested: string;
  onAdded: (t: Table) => void;
  onClose: () => void;
}) {
  const [label, setLabel] = useState(props.suggested);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function add() {
    if (!label.trim()) {
      setError("Label is required.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      props.onAdded(
        await api.createTable(props.restaurantId, { label: label.trim() }),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add table.");
      setBusy(false);
    }
  }

  return (
    <Modal
      title="Add table"
      onClose={props.onClose}
      footer={
        <>
          <button className="btn btn-secondary" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={add} disabled={busy}>
            {busy ? "Adding…" : "Add table"}
          </button>
        </>
      }
    >
      <Field label="Label" hint="Shown to diners and staff." error={error ?? undefined}>
        <input
          className="input"
          value={label}
          autoFocus
          aria-invalid={!!error}
          onChange={(e) => setLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") add();
          }}
        />
      </Field>
    </Modal>
  );
}

// Demo-mode stand-in: deterministic QR-looking pattern from the token.
// Not scannable; clearly labeled in the modal.
function FakeQr(props: { token: string }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const n = 21;
    const cell = 200 / n;
    const ctx = canvas.getContext("2d")!;
    ctx.fillStyle = "#fff";
    ctx.fillRect(0, 0, 200, 200);
    ctx.fillStyle = "#12100f";
    let h = 0;
    for (const c of props.token) h = (h * 31 + c.charCodeAt(0)) >>> 0;
    const rand = () => {
      h = (h * 1103515245 + 12345) >>> 0;
      return h / 4294967296;
    };
    const finder = (x: number, y: number) => {
      ctx.fillRect(x * cell, y * cell, 7 * cell, 7 * cell);
      ctx.fillStyle = "#fff";
      ctx.fillRect((x + 1) * cell, (y + 1) * cell, 5 * cell, 5 * cell);
      ctx.fillStyle = "#12100f";
      ctx.fillRect((x + 2) * cell, (y + 2) * cell, 3 * cell, 3 * cell);
    };
    for (let y = 0; y < n; y++)
      for (let x = 0; x < n; x++)
        if (rand() > 0.5) ctx.fillRect(x * cell, y * cell, cell, cell);
    finder(0, 0);
    finder(n - 7, 0);
    finder(0, n - 7);
  }, [props.token]);
  return (
    <div className="qr-box">
      <canvas ref={ref} width={200} height={200} />
    </div>
  );
}
