import { Button, EmptyState } from "../ui";

export function ExpiredScreen() {
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 20 }}>
        <EmptyState
          icon="qr-code"
          title="This table link has expired"
          message="Table tokens end with the service. Scan the code on your table again, or ask a staff member for a fresh one."
        />
      </div>
      <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="secondary" size="touch" fullWidth iconLeft="refresh-cw" onClick={() => location.reload()}>
          Try again
        </Button>
      </div>
    </div>
  );
}
