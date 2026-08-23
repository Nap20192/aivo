import type { CSSProperties } from "react";
import { formatCents } from "../../../design-system/shared/money";
import { hasFromPrice } from "../cart";
import type { MenuItem, TableSession } from "../types";
import { Badge, Button, Placeholder } from "../ui";

const pillStyle = (selected: boolean): CSSProperties =>
  selected
    ? { flex: "none", padding: "8px 13px", borderRadius: 999, background: "var(--ink-900)", color: "var(--paper-0)", font: "var(--type-label)", cursor: "pointer" }
    : { flex: "none", padding: "8px 13px", borderRadius: 999, background: "var(--paper-2)", border: "1px solid var(--border-default)", color: "var(--ink-700)", font: "var(--type-label)", cursor: "pointer" };

export function MenuScreen({
  session,
  browse,
  menuIdx,
  onPickMenu,
  cat,
  onPickCat,
  cartLabel,
  accountLabel,
  onAccount,
  onOpenItem,
  onLanding,
  onCart,
  onService,
}: {
  session: TableSession;
  browse: boolean;
  menuIdx: number;
  onPickMenu: (i: number) => void;
  cat: number;
  onPickCat: (i: number) => void;
  cartLabel: string;
  accountLabel: string | null;
  onAccount: () => void;
  onOpenItem: (item: MenuItem) => void;
  onLanding: () => void;
  onCart: () => void;
  onService: () => void;
}) {
  const { menus, table, theme } = session;
  const categories = (menus[menuIdx] ?? menus[0])?.categories ?? [];
  const items = (categories[cat] ?? categories[0])?.items ?? [];
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: "none", padding: "12px 16px 10px", background: "var(--paper-0)", display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
        <span
          style={{ font: "600 15px/1 var(--font-sans)", letterSpacing: "-0.01em", color: "var(--ink-900)", cursor: "pointer" }}
          onClick={onLanding}
        >
          {theme.brand_name}
          <span style={{ color: "var(--accent-solid)" }}>.</span>
        </span>
        <span style={{ display: "flex", alignItems: "center", gap: 10 }}>
          {accountLabel ? (
            <span onClick={onAccount} style={{ font: "var(--type-label)", color: "var(--text-link)", cursor: "pointer", whiteSpace: "nowrap" }}>
              {accountLabel}
            </span>
          ) : null}
          {!browse && table.label ? (
            <Badge tone="outline" uppercase>
              {table.label}
            </Badge>
          ) : null}
        </span>
      </div>
      {menus.length > 1 ? (
        <div style={{ flex: "none", display: "flex", gap: 6, padding: "0 16px 10px", background: "var(--paper-0)", overflowX: "auto" }}>
          {menus.map((m, i) => (
            <span key={m.id} onClick={() => onPickMenu(i)} style={pillStyle(i === menuIdx)}>
              {m.name}
            </span>
          ))}
        </div>
      ) : null}
      <div style={{ flex: "none", display: "flex", gap: 6, padding: "0 16px 12px", background: "var(--paper-0)", borderBottom: "1px solid var(--border-default)", overflowX: "auto" }}>
        {categories.map((c, i) => (
          <span key={c.id} onClick={() => onPickCat(i)} style={pillStyle(i === cat)}>
            {c.name}
          </span>
        ))}
      </div>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0, padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
        {items.map((it) => {
          const soldOut = !it.available;
          const dim = soldOut ? { opacity: 0.42 } : {};
          return (
            <div
              key={it.id}
              onClick={() => onOpenItem(it)}
              style={{
                display: "flex",
                gap: 12,
                background: "var(--paper-0)",
                border: soldOut ? "1px dashed var(--border-strong)" : "1px solid var(--border-default)",
                borderRadius: 10,
                padding: 12,
                cursor: "pointer",
              }}
            >
              {it.image_url ? (
                <img src={it.image_url} alt="" style={{ width: 84, height: 84, flex: "none", borderRadius: 8, objectFit: "cover", ...dim }} />
              ) : (
                <Placeholder label="photo" style={{ width: 84, height: 84, flex: "none", borderRadius: 8, ...dim }} />
              )}
              <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
                <div style={{ display: "flex", flexDirection: "column", gap: 4, ...dim }}>
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 8 }}>
                    <span style={{ font: "600 15px/1.25 var(--font-sans)", color: "var(--ink-900)" }}>{it.name}</span>
                    <span className="aivo-num" style={{ font: "var(--type-numeric)", color: "var(--ink-900)" }}>
                      {(hasFromPrice(it) ? "from " : "") + formatCents(it.price_cents)}
                    </span>
                  </div>
                  <span style={{ font: "var(--weight-regular) 13px/1.45 var(--font-sans)", color: "var(--ink-500)", textWrap: "pretty" }}>
                    {it.description}
                  </span>
                </div>
                <div style={{ display: "flex", gap: 6, marginTop: 2, flexWrap: "wrap" }}>
                  {soldOut ? (
                    <Badge tone="warning">off the menu tonight</Badge>
                  ) : (
                    it.allergens.map((a) => (
                      <Badge key={a} tone="neutral">
                        {a}
                      </Badge>
                    ))
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
      {!browse ? (
        <div style={{ flex: "none", display: "grid", gridTemplateColumns: "1fr 1fr 1.25fr", gap: 8, padding: "10px 14px 14px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
          <Button variant="secondary" size="touch" fullWidth iconLeft="bell" onClick={onService}>
            Waiter
          </Button>
          <Button variant="secondary" size="touch" fullWidth iconLeft="receipt" onClick={onService}>
            Bill
          </Button>
          <Button variant="primary" size="touch" fullWidth onClick={onCart}>
            {cartLabel}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
