import type { SentOrder } from "../App";
import { Button, Icon } from "../ui";

export function SentScreen({
  sent,
  tableLabel,
  onMenu,
  onService,
}: {
  sent: SentOrder;
  tableLabel: string;
  onMenu: () => void;
  onService: () => void;
}) {
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", padding: "0 22px" }}>
        <div style={{ width: 52, height: 52, borderRadius: "50%", background: "var(--green-100)", border: "1px solid var(--green-200)", display: "grid", placeItems: "center", marginBottom: 20, color: "var(--green-700)" }}>
          <Icon name="check" size={24} />
        </div>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          Sent to the kitchen
        </h2>
        <p style={{ margin: "10px 0 20px", font: "var(--weight-regular) 15px/1.55 var(--font-sans)", color: "var(--ink-600)", textWrap: "pretty" }}>
          {tableLabel} · {sent.count} {sent.count === 1 ? "item" : "items"} · {sent.time}. It'll come out when it's ready —
          nothing else to do.
        </p>
        <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: "4px 16px" }}>
          {sent.lines.map((l, i) => (
            <div
              key={i}
              style={{
                display: "flex",
                justifyContent: "space-between",
                padding: "11px 0",
                borderBottom: i === sent.lines.length - 1 ? "none" : "1px dashed var(--border-default)",
              }}
            >
              <span style={{ font: "var(--type-body)", color: "var(--ink-600)" }}>{l.name}</span>
              <span className="aivo-num" style={{ font: "var(--type-numeric)", color: "var(--ink-900)" }}>×{l.qty}</span>
            </div>
          ))}
        </div>
      </div>
      <div style={{ flex: "none", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8, padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="secondary" size="touch" fullWidth onClick={onMenu}>
          Order more
        </Button>
        <Button variant="secondary" size="touch" fullWidth iconLeft="receipt" onClick={onService}>
          Ask for the bill
        </Button>
      </div>
    </div>
  );
}
