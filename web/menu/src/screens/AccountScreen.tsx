import { formatCents } from "../../../design-system/shared/money";
import type { CustomerMe } from "../types";
import { Button, EmptyState } from "../ui";

export function AccountScreen({
  me,
  onLogout,
  onBack,
}: {
  me: CustomerMe;
  onLogout: () => void;
  onBack: () => void;
}) {
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: "none", padding: "6px 8px 0", background: "var(--paper-0)", display: "flex" }}>
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Menu
        </Button>
      </div>
      <div style={{ flex: "none", padding: "8px 18px 12px", background: "var(--paper-0)", borderBottom: "1px solid var(--border-default)" }}>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 22px/1.1 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          {me.customer.name}
        </h2>
        <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)", marginTop: 2 }}>
          {me.customer.email}
        </div>
      </div>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0, padding: "16px 18px", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ font: "600 11px/1.2 var(--font-sans)", letterSpacing: "0.07em", textTransform: "uppercase", color: "var(--ink-500)" }}>
          Past orders
        </div>
        {me.orders.length === 0 ? (
          <EmptyState icon="receipt" title="No orders yet" message="Orders you send while signed in show up here." />
        ) : (
          me.orders.map((o, i) => (
            <div key={i} style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: 14 }}>
              <div style={{ display: "flex", justifyContent: "space-between", gap: 10 }}>
                <span style={{ font: "600 15px/1.25 var(--font-sans)", color: "var(--ink-900)" }}>{o.restaurant_name}</span>
                <span className="aivo-num" style={{ font: "var(--type-numeric)", color: "var(--ink-900)" }}>
                  {formatCents(o.total_cents)}
                </span>
              </div>
              <div style={{ font: "var(--weight-regular) 12px/1.4 var(--font-sans)", color: "var(--ink-400)", marginTop: 2 }}>
                {new Date(o.created_at).toLocaleDateString(undefined, { day: "numeric", month: "long", year: "numeric" })}
              </div>
              <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)", marginTop: 6 }}>
                {o.lines.map((l) => (l.qty > 1 ? `${l.name} ×${l.qty}` : l.name)).join(" · ")}
              </div>
            </div>
          ))
        )}
      </div>
      <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="secondary" size="touch" fullWidth onClick={onLogout}>
          Sign out
        </Button>
      </div>
    </div>
  );
}
