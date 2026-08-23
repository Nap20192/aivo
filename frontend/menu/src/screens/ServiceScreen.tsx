import { Button, Icon } from "../ui";

export function ServiceScreen({
  tableLabel,
  waiterAt,
  billAt,
  onCallWaiter,
  onRequestBill,
  onMenu,
}: {
  tableLabel: string;
  waiterAt: string | null;
  billAt: string | null;
  onCallWaiter: () => void;
  onRequestBill: () => void;
  onMenu: () => void;
}) {
  const table = tableLabel.toLowerCase();
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0, display: "flex", flexDirection: "column", padding: "24px 20px" }}>
        <h2 style={{ margin: 0, font: "var(--weight-regular) 27px/1.15 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
          Need something?
        </h2>
        <p style={{ margin: "10px 0 24px", font: "var(--weight-regular) 15px/1.55 var(--font-sans)", color: "var(--ink-600)", textWrap: "pretty" }}>
          One request per table. Everyone at {table} sees the same state, so there's no need for two of you to ask.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {waiterAt ? (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--orange-200)", borderRadius: 10, padding: "16px 18px" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6, color: "var(--orange-700)" }}>
                <Icon name="bell-ring" size={16} />
                <span style={{ font: "600 15px/1.2 var(--font-sans)", color: "var(--orange-700)" }}>Already asked</span>
              </div>
              <div style={{ font: "var(--weight-regular) 14px/1.5 var(--font-sans)", color: "var(--ink-700)", marginBottom: 14 }}>
                Someone at {table} called the waiter at <span style={{ font: "var(--type-numeric)" }}>{waiterAt}</span>. One
                open request per table — we won't send a second.
              </div>
              <Button variant="primary" size="touch" fullWidth disabled iconLeft="bell">
                Waiter on the way
              </Button>
            </div>
          ) : (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: 18 }}>
              <div style={{ font: "600 16px/1.25 var(--font-sans)", color: "var(--ink-900)", marginBottom: 4 }}>Call the waiter</div>
              <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)", marginBottom: 14 }}>
                Someone comes over. No reason needed.
              </div>
              <Button variant="primary" size="touch" fullWidth iconLeft="bell" onClick={onCallWaiter}>
                Call the waiter
              </Button>
            </div>
          )}
          {billAt ? (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: 18 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 4 }}>
                <Icon name="receipt" size={16} />
                <span style={{ font: "600 16px/1.25 var(--font-sans)", color: "var(--ink-900)" }}>Bill on the way</span>
              </div>
              <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)" }}>
                Asked at <span style={{ font: "var(--type-numeric)" }}>{billAt}</span>. Pay however you like — cash or card.
              </div>
            </div>
          ) : (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: 18 }}>
              <div style={{ font: "600 16px/1.25 var(--font-sans)", color: "var(--ink-900)", marginBottom: 4 }}>Ask for the bill</div>
              <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-500)", marginBottom: 14 }}>
                Brought to the table. Pay however you like — cash or card.
              </div>
              <Button variant="secondary" size="touch" fullWidth iconLeft="receipt" onClick={onRequestBill}>
                Ask for the bill
              </Button>
            </div>
          )}
        </div>
      </div>
      <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="ghost" size="touch" fullWidth onClick={onMenu}>
          Back to the menu
        </Button>
      </div>
    </div>
  );
}
