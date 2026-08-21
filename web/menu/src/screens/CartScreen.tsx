import type { CartLine } from "../cart";
import { fmtCents } from "../format";
import { Button, EmptyState, Icon, QtyStepper } from "../ui";

export function CartScreen({
  tableLabel,
  lines,
  onSetQty,
  onRemove,
  note,
  onNote,
  rateLimited,
  countdown,
  lastSentTime,
  error,
  sending,
  onSend,
  onMenu,
}: {
  tableLabel: string;
  lines: CartLine[];
  onSetQty: (index: number, qty: number) => void;
  onRemove: (index: number) => void;
  note: string;
  onNote: (v: string) => void;
  rateLimited: boolean;
  countdown: string;
  lastSentTime: string;
  error: string | null;
  sending: boolean;
  onSend: () => void;
  onMenu: () => void;
}) {
  const total = lines.reduce((t, l) => t + l.unitCents * l.qty, 0);
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: "none", padding: "6px 8px 0", background: "var(--paper-0)", display: "flex" }}>
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onMenu}>
          Menu
        </Button>
      </div>
      <div style={{ flex: "none", padding: "8px 18px 12px", background: "var(--paper-0)", borderBottom: "1px solid var(--border-default)", display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 22px/1.1 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          Your order
        </h2>
        <span style={{ font: "var(--type-body)", color: "var(--ink-500)" }}>{tableLabel}</span>
      </div>
      {lines.length === 0 ? (
        <>
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 20 }}>
            <EmptyState
              icon="utensils"
              title="Nothing in the cart yet"
              message="Pick something from the menu. Your cart lives on this phone only — it clears if you close the tab."
            />
          </div>
          <div style={{ flex: "none", padding: "10px 14px 14px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
            <Button variant="primary" size="touch" fullWidth onClick={onMenu}>
              Browse menu
            </Button>
          </div>
        </>
      ) : (
        <>
          <div style={{ flex: 1, overflowY: "auto", minHeight: 0, padding: "16px 18px", display: "flex", flexDirection: "column", gap: 12 }}>
            {rateLimited ? (
              <div style={{ background: "var(--paper-0)", border: "1px solid var(--orange-200)", borderRadius: 10, padding: "16px 18px" }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6, color: "var(--orange-700)" }}>
                  <Icon name="clock" size={16} />
                  <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--orange-700)" }}>One moment</span>
                </div>
                <div style={{ font: "var(--weight-regular) 14px/1.5 var(--font-sans)", color: "var(--ink-700)" }}>
                  {tableLabel} sent an order at <span style={{ font: "var(--type-numeric)" }}>{lastSentTime}</span>. To stop
                  double orders reaching the kitchen, the next one can go in{" "}
                  <span style={{ font: "var(--type-numeric)" }}>{countdown}</span>. Nothing was lost.
                </div>
              </div>
            ) : null}
            {error ? (
              <div style={{ background: "var(--red-100)", border: "1px solid var(--red-200)", borderRadius: 10, padding: "14px 16px", font: "var(--weight-regular) 14px/1.5 var(--font-sans)", color: "var(--red-700)" }}>
                {error}
              </div>
            ) : null}
            {lines.map((l, i) => (
              <div key={i} style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: 14 }}>
                <div style={{ display: "flex", justifyContent: "space-between", gap: 10 }}>
                  <span style={{ font: "600 15px/1.25 var(--font-sans)", color: "var(--ink-900)" }}>{l.name}</span>
                  <span className="aivo-num" style={{ font: "var(--type-numeric)", color: "var(--ink-900)" }}>
                    {fmtCents(l.unitCents * l.qty)}
                  </span>
                </div>
                {l.detail ? (
                  <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)", marginTop: 4 }}>
                    {l.detail}
                  </div>
                ) : null}
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginTop: 12 }}>
                  <QtyStepper
                    compact
                    qty={l.qty}
                    onDec={() => onSetQty(i, Math.max(1, l.qty - 1))}
                    onInc={() => onSetQty(i, Math.min(9, l.qty + 1))}
                  />
                  <span style={{ font: "var(--type-label)", color: "var(--text-link)", cursor: "pointer" }} onClick={() => onRemove(i)}>
                    Remove
                  </span>
                </div>
              </div>
            ))}
            <div>
              <div style={{ font: "600 11px/1.2 var(--font-sans)", letterSpacing: "0.07em", textTransform: "uppercase", color: "var(--ink-500)", marginBottom: 8 }}>
                Note for the kitchen · one per order
              </div>
              <textarea
                rows={3}
                placeholder="Anything the kitchen should know — allergies, pacing, one plate to share."
                value={note}
                onChange={(e) => onNote(e.target.value.slice(0, 240))}
                style={{
                  width: "100%",
                  boxSizing: "border-box",
                  background: "var(--paper-0)",
                  border: "1px solid var(--border-strong)",
                  borderRadius: 6,
                  padding: "12px 14px",
                  font: "var(--weight-regular) 14px/1.5 var(--font-sans)",
                  color: "var(--ink-800)",
                  outline: "none",
                }}
              />
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 6 }}>
                <span style={{ font: "var(--weight-regular) 12px/1.4 var(--font-sans)", color: "var(--ink-500)" }}>
                  The waiter reads this before firing.
                </span>
                <span style={{ font: "var(--type-numeric)", fontSize: 11, color: "var(--ink-400)" }}>{note.length} / 240</span>
              </div>
            </div>
            <div style={{ background: "var(--paper-2)", border: "1px solid var(--border-subtle)", borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ font: "var(--type-label)", color: "var(--ink-700)" }}>Total</span>
                <span className="aivo-num" style={{ font: "600 18px/1.3 var(--font-mono)", color: "var(--ink-900)" }}>
                  {fmtCents(total)}
                </span>
              </div>
              <div style={{ font: "var(--weight-regular) 12px/1.5 var(--font-sans)", color: "var(--ink-500)", marginTop: 6 }}>
                You pay at the table, as usual. This screen never takes payment.
              </div>
            </div>
          </div>
          <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
            <Button variant="primary" size="touch" fullWidth disabled={rateLimited || sending} onClick={onSend}>
              {rateLimited ? "Send again in " + countdown : sending ? "Sending…" : "Send to the kitchen"}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
