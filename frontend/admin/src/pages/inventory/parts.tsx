// Shared bits for the inventory screens: product loading, status badges, and
// the product/qty/unit line editor reused by receipts, write-offs and counts.
import { Plus, Trash2 } from "lucide-react";
import { api } from "../../api/client";
import type { DocStatus, Product, Unit } from "../../api/types";
import { useRestaurant } from "../../auth";
import { useLoad } from "../../lib/useLoad";
import { DISPLAY_UNITS } from "../../lib/units";
import { Badge } from "../../ui";

export function useProducts() {
  const r = useRestaurant();
  return useLoad(() => api.listProducts(r.id), [r.id]);
}

// Products that carry stock (goods / prepared) — the ones documents move.
export function stockable(products: Product[]): Product[] {
  return products.filter((p) => !p.archived && (p.type === "goods" || p.type === "prepared"));
}

const STATUS_TONE: Record<DocStatus, "outline" | "ok" | "danger"> = {
  draft: "outline",
  posted: "ok",
  cancelled: "danger",
};
export function StatusBadge({ status }: { status: DocStatus }) {
  return <Badge tone={STATUS_TONE[status]}>{status}</Badge>;
}

export interface EditLine {
  product_id: string;
  qty: string;
  unit: Unit;
  unit_price?: string; // dollars, receipts only
}

export function emptyLine(): EditLine {
  return { product_id: "", qty: "", unit: "g" };
}

// Editable table of document lines. `withPrice` adds the per-unit price column.
export function LineEditor(props: {
  products: Product[];
  lines: EditLine[];
  setLines: (l: EditLine[]) => void;
  withPrice?: boolean;
}) {
  const { products, lines, setLines, withPrice } = props;
  const byId = (id: string) => products.find((p) => p.id === id);
  const set = (i: number, patch: Partial<EditLine>) =>
    setLines(lines.map((l, x) => (x === i ? { ...l, ...patch } : l)));

  return (
    <table className="table-plain">
      <thead>
        <tr>
          <th>Product</th>
          <th style={{ textAlign: "right" }}>Qty</th>
          <th>Unit</th>
          {withPrice && <th style={{ textAlign: "right" }}>Price / unit</th>}
          <th></th>
        </tr>
      </thead>
      <tbody>
        {lines.map((l, i) => {
          const p = byId(l.product_id);
          const units = p ? DISPLAY_UNITS[p.stock_unit] : (["g"] as Unit[]);
          return (
            <tr key={i}>
              <td>
                <select
                  className="select"
                  value={l.product_id}
                  onChange={(e) => {
                    const np = byId(e.target.value);
                    set(i, { product_id: e.target.value, unit: np ? np.stock_unit : "g" });
                  }}
                >
                  <option value="">Select product…</option>
                  {products.map((op) => (
                    <option key={op.id} value={op.id}>
                      {op.sku} · {op.name}
                    </option>
                  ))}
                </select>
              </td>
              <td style={{ textAlign: "right" }}>
                <input
                  className="input num"
                  style={{ maxWidth: 100, textAlign: "right" }}
                  inputMode="decimal"
                  placeholder="0"
                  value={l.qty}
                  onChange={(e) => set(i, { qty: e.target.value.replace(/[^0-9.]/g, "").slice(0, 10) })}
                />
              </td>
              <td>
                <select className="select" value={l.unit} onChange={(e) => set(i, { unit: e.target.value as Unit })}>
                  {units.map((u) => (
                    <option key={u} value={u}>
                      {u}
                    </option>
                  ))}
                </select>
              </td>
              {withPrice && (
                <td style={{ textAlign: "right" }}>
                  <input
                    className="input num"
                    style={{ maxWidth: 110, textAlign: "right" }}
                    inputMode="decimal"
                    placeholder="0.00"
                    value={l.unit_price ?? ""}
                    onChange={(e) => set(i, { unit_price: e.target.value.replace(/[^0-9.]/g, "").slice(0, 12) })}
                  />
                </td>
              )}
              <td style={{ textAlign: "right" }}>
                {lines.length > 1 && (
                  <button className="btn btn-ghost btn-icon" aria-label="Remove line" onClick={() => setLines(lines.filter((_, x) => x !== i))}>
                    <Trash2 size={15} />
                  </button>
                )}
              </td>
            </tr>
          );
        })}
        <tr>
          <td colSpan={withPrice ? 5 : 4}>
            <button className="btn btn-ghost btn-sm" onClick={() => setLines([...lines, emptyLine()])}>
              <Plus size={14} /> Add line
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  );
}
