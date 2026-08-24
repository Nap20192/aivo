import { CalendarClock, Plus, RefreshCw } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { ConsumptionStrategy, Product, TechCardInput, Unit } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatQty, toBaseMilli } from "../../lib/units";
import { formatCents } from "../../../../design-system/shared/money";
import { Badge, EmptyState, ErrorBanner, Field, LoadingPage, Modal, NoticeBanner } from "../../ui";
import { EditLine, LineEditor } from "./parts";

const TODAY = "2026-08-24";
const CONSUMPTION: { value: ConsumptionStrategy; label: string; hint: string }[] = [
  { value: "assemble", label: "Assemble", hint: "Selling depletes the ingredients listed below." },
  { value: "deplete_finished", label: "Deplete finished", hint: "Selling depletes this product's own stock." },
];

export default function TechCards({
  products,
  selected,
  setSelected,
}: {
  products: Product[];
  selected: string;
  setSelected: (id: string) => void;
}) {
  const cardable = products.filter((p) => !p.archived && (p.type === "dish" || p.type === "prepared"));
  const product = cardable.find((p) => p.id === selected);

  return (
    <div className="stack">
      <Field label="Product">
        <select className="select" style={{ maxWidth: 360 }} value={selected} onChange={(e) => setSelected(e.target.value)}>
          <option value="">Select a dish or prepared item…</option>
          {cardable.map((p) => (
            <option key={p.id} value={p.id}>
              {p.sku} · {p.name}
            </option>
          ))}
        </select>
      </Field>
      {product ? (
        <ProductCards product={product} products={products} />
      ) : (
        <div className="card">
          <EmptyState icon={CalendarClock} title="Pick a product" message="Tech-cards belong to dishes and prepared items." />
        </div>
      )}
    </div>
  );
}

function ProductCards({ product, products }: { product: Product; products: Product[] }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listTechCards(r.id, product.id), [r.id, product.id]);
  const [open, setOpen] = useState<string | null>(null);
  const [create, setCreate] = useState(false);

  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <span style={{ font: "var(--type-section-title)" }}>{product.name} — version timeline</span>
        <button className="btn btn-primary" onClick={() => setCreate(true)}>
          <Plus size={16} /> New version
        </button>
      </div>

      {data.length === 0 ? (
        <div className="card">
          <EmptyState icon={CalendarClock} title="No versions yet" message="Create the first tech-card version with a start date." />
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>Valid</th>
                <th>Consumption</th>
                <th style={{ textAlign: "right" }}>Cost</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {data.map((v) => (
                <tr key={v.id}>
                  <td className="num" style={{ fontSize: 13 }}>
                    {v.valid_from} → {v.valid_to ?? "current"}
                    {!v.valid_to && (
                      <>
                        {" "}
                        <Badge tone="ok">active</Badge>
                      </>
                    )}
                  </td>
                  <td>{v.consumption === "assemble" ? "Assemble" : "Deplete finished"}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatCents(v.cost_cents)}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => setOpen(v.id)}>
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open && <CardModal tcid={open} onClose={() => setOpen(null)} onChanged={reload} />}
      {create && <NewVersionModal product={product} products={products} onClose={() => setCreate(false)} onCreated={reload} />}
    </div>
  );
}

function CardModal({ tcid, onClose, onChanged }: { tcid: string; onClose: () => void; onChanged: () => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.getTechCard(r.id, tcid), [r.id, tcid]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const recost = () => {
    setBusy(true);
    setErr(null);
    api
      .recostTechCard(r.id, tcid)
      .then(() => {
        reload();
        onChanged();
      })
      .catch((e: { message?: string }) => setErr(e.message ?? "Could not recost."))
      .finally(() => setBusy(false));
  };

  return (
    <Modal
      title={data ? `Version ${data.valid_from} → ${data.valid_to ?? "current"}` : "Tech-card"}
      onClose={onClose}
      wide
      footer={
        <button className="btn btn-secondary" disabled={busy} onClick={recost}>
          <RefreshCw size={15} /> Recost at today's average
        </button>
      }
    >
      {error && <ErrorBanner message={error} />}
      {loading && <LoadingPage />}
      {data && (
        <div className="stack">
          <div className="row" style={{ gap: 16, flexWrap: "wrap", font: "var(--type-body)" }}>
            <Badge tone={data.consumption === "assemble" ? "info" : "neutral"}>
              {data.consumption === "assemble" ? "assemble" : "deplete finished"}
            </Badge>
            {data.yield_qty != null && <span style={{ color: "var(--text-muted)" }}>yield {data.yield_qty / 1000}</span>}
            <span style={{ marginLeft: "auto", font: "var(--type-label)" }}>Cost {formatCents(data.cost_cents)}</span>
          </div>

          <table className="table-plain">
            <thead>
              <tr>
                <th>Ingredient</th>
                <th style={{ textAlign: "right" }}>Quantity</th>
              </tr>
            </thead>
            <tbody>
              {data.lines.map((l) => (
                <tr key={l.id}>
                  <td>{l.ingredient_name}</td>
                  <td className="num" style={{ textAlign: "right" }}>{formatQty(l.qty, l.unit)}</td>
                </tr>
              ))}
            </tbody>
          </table>

          <div>
            <span className="field-label">Cost history (append-only)</span>
            <div className="card" style={{ padding: 0, marginTop: 6 }}>
              <table className="table-plain">
                <tbody>
                  {data.cost_history.map((c) => (
                    <tr key={c.id}>
                      <td className="num" style={{ fontSize: 13 }}>{new Date(c.computed_at).toLocaleString("en-GB")}</td>
                      <td style={{ color: "var(--text-muted)" }}>{c.method}</td>
                      <td className="num" style={{ textAlign: "right" }}>{formatCents(c.cost_cents)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          {err && <ErrorBanner message={err} />}
        </div>
      )}
    </Modal>
  );
}

function NewVersionModal({
  product,
  products,
  onClose,
  onCreated,
}: {
  product: Product;
  products: Product[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const r = useRestaurant();
  const ingredients = products.filter((p) => !p.archived && p.id !== product.id && p.type !== "dish");
  const [validFrom, setValidFrom] = useState(TODAY);
  const [consumption, setConsumption] = useState<ConsumptionStrategy>("assemble");
  const [yieldQty, setYieldQty] = useState("1");
  const [lines, setLines] = useState<EditLine[]>([{ product_id: "", qty: "", unit: "g" }]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const validLines = lines.filter((l) => l.product_id && parseFloat(l.qty) > 0);
  const canSave = consumption === "deplete_finished" || validLines.length > 0;

  const create = () => {
    setBusy(true);
    setErr(null);
    const input: TechCardInput = {
      valid_from: validFrom,
      consumption,
      yield_qty: yieldQty.trim() ? toBaseMilli(parseFloat(yieldQty), "pcs") : null,
      lines: validLines.map((l) => ({ ingredient_product_id: l.product_id, qty: parseFloat(l.qty), unit: l.unit as Unit })),
    };
    api
      .createTechCard(r.id, product.id, input)
      .then(() => {
        onCreated();
        onClose();
      })
      .catch((e: { message?: string }) => {
        setErr(e.message ?? "Could not create the version.");
        setBusy(false);
      });
  };

  return (
    <Modal
      title={`New version — ${product.name}`}
      onClose={onClose}
      wide
      footer={
        <button className="btn btn-primary" disabled={busy || !canSave} onClick={create}>
          Create version
        </button>
      }
    >
      <div className="stack">
        <NoticeBanner>
          Versions are immutable once created. Editing a recipe means creating a new version, which closes the current one.
        </NoticeBanner>
        <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
          <Field label="Valid from" hint="Back-dating closes the previous version at this date.">
            <input className="input" type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />
          </Field>
          <Field label="Consumption" hint={CONSUMPTION.find((c) => c.value === consumption)!.hint}>
            <select className="select" value={consumption} onChange={(e) => setConsumption(e.target.value as ConsumptionStrategy)}>
              {CONSUMPTION.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Yield" hint="Portions produced (informational).">
            <input className="input num" style={{ maxWidth: 90 }} inputMode="decimal" value={yieldQty} onChange={(e) => setYieldQty(e.target.value.replace(/[^0-9.]/g, ""))} />
          </Field>
        </div>

        {consumption === "assemble" && <LineEditor products={ingredients} lines={lines} setLines={setLines} />}
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}
