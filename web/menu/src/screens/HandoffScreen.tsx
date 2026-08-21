import { countdownStr } from "../format";
import type { Handoff } from "../types";
import { Button } from "../ui";

/** Full-screen pickup code after "Show to waiter" — QR + big code + expiry countdown. */
export function HandoffScreen({
  handoff,
  now,
  tableLabel,
  onMenu,
}: {
  handoff: Handoff;
  now: number;
  tableLabel: string;
  onMenu: () => void;
}) {
  const left = Math.max(0, new Date(handoff.expires_at).getTime() - now);
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "24px 22px", textAlign: "center" }}>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          Show this to the waiter
        </h2>
        <p style={{ margin: "10px 0 20px", font: "var(--weight-regular) 15px/1.55 var(--font-sans)", color: "var(--ink-600)", textWrap: "pretty" }}>
          They'll add it to {tableLabel.toLowerCase()}'s order. Nothing reaches the kitchen until they do.
        </p>
        <div style={{ background: "#fff", border: "1px solid var(--border-default)", borderRadius: 14, padding: 14 }}>
          <img src={handoff.qr_url} alt={"Pickup code " + handoff.code} style={{ width: 208, height: 208, display: "block" }} />
        </div>
        <div className="aivo-num" style={{ margin: "18px 0 4px", font: "600 40px/1 var(--font-mono)", letterSpacing: "0.14em", color: "var(--ink-900)" }}>
          {handoff.code}
        </div>
        <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)" }}>
          Code expires in <span style={{ font: "var(--type-numeric)" }}>{countdownStr(left)}</span>. If it runs out unused,
          your cart comes back.
        </div>
      </div>
      <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="secondary" size="touch" fullWidth onClick={onMenu}>
          Order more
        </Button>
      </div>
    </div>
  );
}
