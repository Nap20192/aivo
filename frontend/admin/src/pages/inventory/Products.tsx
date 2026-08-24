import { Package, Plus } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { BaseUnit, MenuItem, Product, ProductInput, ProductType } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { formatCents } from "../../../../design-system/shared/money";
import { Badge, EmptyState, ErrorBanner, Field, LoadingPage, Modal } from "../../ui";

const TYPES: { value: ProductType; label: string }[] = [
  { value: "goods", label: "Goods" },
  { value: "prepared", label: "Prepared" },
  { value: "dish", label: "Dish" },
  { value: "modifier", label: "Modifier" },
];
const TYPE_TONE: Record<ProductType, "info" | "ok" | "neutral" | "outline"> = {
  goods: "info",
  prepared: "ok",
  dish: "neutral",
  modifier: "outline",
};
const UNITS: BaseUnit[] = ["g", "ml", "pcs"];

export default function Products({ onOpenCard }: { onOpenCard: (pid: string) => void }) {
  const r = useRestaurant();
  const { data, error, loading, reload } = useLoad(() => api.listProducts(r.id), [r.id]);
  const [create, setCreate] = useState(false);
  const [showArchived, setShowArchived] = useState(false);

  if (error) return <ErrorBanner message={error} onRetry={reload} />;
  if (loading || !data) return <LoadingPage />;

  const rows = data.filter((p) => showArchived || !p.archived);

  return (
    <div className="stack">
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <label className="row" style={{ gap: 8, font: "var(--type-body)" }}>
          <input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} />
          Show archived
        </label>
        <button className="btn btn-primary" onClick={() => setCreate(true)}>
          <Plus size={16} /> New product
        </button>
      </div>

      {rows.length === 0 ? (
        <div className="card">
          <EmptyState icon={Package} title="No products" message="Add goods, prepared items, dishes and modifiers." />
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <table className="table-plain">
            <thead>
              <tr>
                <th>SKU</th>
                <th>Name</th>
                <th>Type</th>
                <th>Unit</th>
                <th>Menu link</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <ProductRow key={p.id} product={p} onOpenCard={onOpenCard} onChanged={reload} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {create && <ProductModal onClose={() => setCreate(false)} onSaved={reload} />}
    </div>
  );
}

function ProductRow({ product, onOpenCard, onChanged }: { product: Product; onOpenCard: (pid: string) => void; onChanged: () => void }) {
  const [edit, setEdit] = useState(false);
  const hasCard = product.type === "dish" || product.type === "prepared";
  return (
    <tr style={product.archived ? { opacity: 0.55 } : undefined}>
      <td className="num" style={{ font: "var(--type-label)" }}>{product.sku}</td>
      <td>
        {product.name}
        {product.archived && <span style={{ color: "var(--text-muted)" }}> · archived</span>}
      </td>
      <td>
        <Badge tone={TYPE_TONE[product.type]}>{product.type}</Badge>
      </td>
      <td style={{ color: "var(--text-muted)" }}>{product.stock_unit}</td>
      <td style={{ color: "var(--text-muted)" }}>{product.menu_item_id ? "linked" : product.type === "dish" ? "—" : ""}</td>
      <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
        {hasCard && (
          <button className="btn btn-ghost btn-sm" onClick={() => onOpenCard(product.id)}>
            Tech-card
          </button>
        )}
        <button className="btn btn-ghost btn-sm" onClick={() => setEdit(true)}>
          Edit
        </button>
      </td>
      {edit && <ProductModal product={product} onClose={() => setEdit(false)} onSaved={onChanged} />}
    </tr>
  );
}

function ProductModal({ product, onClose, onSaved }: { product?: Product; onClose: () => void; onSaved: () => void }) {
  const r = useRestaurant();
  const editing = !!product;
  const { data: items } = useLoad(() => api.listItems(r.id), [r.id]);
  const { data: detail } = useLoad(() => (product ? api.getProduct(r.id, product.id) : Promise.resolve(undefined)), [r.id, product?.id]);

  const [sku, setSku] = useState(product?.sku ?? "");
  const [name, setName] = useState(product?.name ?? "");
  const [type, setType] = useState<ProductType>(product?.type ?? "goods");
  const [unit, setUnit] = useState<BaseUnit>(product?.stock_unit ?? "g");
  const [menuItemId, setMenuItemId] = useState<string>(product?.menu_item_id ?? "");
  const [minStock, setMinStock] = useState(product?.min_stock != null ? String(product.min_stock / 1000) : "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const unitLocked = editing && detail?.has_moves;

  const save = () => {
    if (!name.trim() || (!editing && !sku.trim())) {
      setErr("SKU and name are required.");
      return;
    }
    setBusy(true);
    setErr(null);
    const min = minStock.trim() ? Math.round(parseFloat(minStock) * 1000) : null;
    const done = () => {
      onSaved();
      onClose();
    };
    const fail = (e: { message?: string }) => {
      setErr(e.message ?? "Could not save the product.");
      setBusy(false);
    };
    if (editing) {
      const patch: Partial<ProductInput> = { name, min_stock: min };
      if (type === "dish") patch.menu_item_id = menuItemId || null;
      if (!unitLocked) patch.stock_unit = unit;
      api.updateProduct(r.id, product!.id, patch).then(done).catch(fail);
    } else {
      const input: ProductInput = {
        sku,
        name,
        type,
        stock_unit: unit,
        menu_item_id: type === "dish" && menuItemId ? menuItemId : null,
        min_stock: min,
      };
      api.createProduct(r.id, input).then(done).catch(fail);
    }
  };

  return (
    <Modal
      title={editing ? `Edit ${product!.sku}` : "New product"}
      onClose={onClose}
      footer={
        <div className="row" style={{ justifyContent: "space-between", width: "100%" }}>
          {editing && (
            <button
              className="btn btn-ghost"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                api
                  .updateProduct(r.id, product!.id, { archived: !product!.archived })
                  .then(() => {
                    onSaved();
                    onClose();
                  })
                  .catch((e: { message?: string }) => {
                    setErr(e.message ?? "Could not archive.");
                    setBusy(false);
                  });
              }}
            >
              {product!.archived ? "Restore" : "Archive"}
            </button>
          )}
          <button className="btn btn-primary" disabled={busy} onClick={save} style={{ marginLeft: "auto" }}>
            {editing ? "Save" : "Create"}
          </button>
        </div>
      }
    >
      <div className="stack">
        <div className="row" style={{ gap: 12 }}>
          <Field label="SKU">
            <input className="input" value={sku} disabled={editing} onChange={(e) => setSku(e.target.value)} placeholder="BEEF-RIB" />
          </Field>
          <div style={{ flex: 1 }}>
            <Field label="Name">
              <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Ribeye beef" />
            </Field>
          </div>
        </div>
        <div className="row" style={{ gap: 12 }}>
          <Field label="Type">
            <select className="select" value={type} disabled={editing} onChange={(e) => setType(e.target.value as ProductType)}>
              {TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Stock unit" hint={unitLocked ? "Locked — this product has stock movements." : "Base unit for the ledger."}>
            <select className="select" value={unit} disabled={unitLocked} onChange={(e) => setUnit(e.target.value as BaseUnit)}>
              {UNITS.map((u) => (
                <option key={u} value={u}>
                  {u}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Min stock" hint={`Alert threshold, in ${unit}.`}>
            <input
              className="input num"
              style={{ maxWidth: 120 }}
              inputMode="decimal"
              placeholder="—"
              value={minStock}
              onChange={(e) => setMinStock(e.target.value.replace(/[^0-9.]/g, ""))}
            />
          </Field>
        </div>
        {type === "dish" && (
          <Field label="Menu item" hint="Sales of this menu item deplete stock through the tech-card.">
            <select className="select" value={menuItemId} onChange={(e) => setMenuItemId(e.target.value)}>
              <option value="">Not linked</option>
              {(items ?? []).map((it: MenuItem) => (
                <option key={it.id} value={it.id}>
                  {it.name} · {formatCents(it.price_cents)}
                </option>
              ))}
            </select>
          </Field>
        )}
        {err && <ErrorBanner message={err} />}
      </div>
    </Modal>
  );
}
