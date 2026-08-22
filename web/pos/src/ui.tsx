import type { CSSProperties, ReactNode } from "react";
import { GLYPHS } from "../../design-system/shared/icons";

export function Icon({ name, size = 18, style }: { name: string; size?: number; style?: CSSProperties }) {
  return (
    <svg
      aria-hidden
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ flex: "none", ...style }}
      dangerouslySetInnerHTML={{ __html: GLYPHS[name] ?? "" }}
    />
  );
}

export function Button({
  children,
  variant = "secondary",
  size = "touch",
  iconLeft,
  disabled,
  fullWidth,
  onClick,
  style,
}: {
  children: ReactNode;
  variant?: "primary" | "secondary" | "ghost";
  size?: "touch" | "sm";
  iconLeft?: string;
  disabled?: boolean;
  fullWidth?: boolean;
  onClick?: () => void;
  style?: CSSProperties;
}) {
  return (
    <button
      type="button"
      className={`btn btn-${variant} btn-${size}${fullWidth ? " btn-full" : ""}`}
      disabled={disabled}
      onClick={disabled ? undefined : onClick}
      style={style}
    >
      {iconLeft ? <Icon name={iconLeft} size={size === "sm" ? 14 : 20} /> : null}
      {children}
    </button>
  );
}

const PILL: Record<string, { fg: string; bg: string }> = {
  open: { fg: "var(--status-open-fg)", bg: "var(--status-open-bg)" },
  closed: { fg: "var(--status-closed-fg)", bg: "var(--status-closed-bg)" },
  accepted: { fg: "var(--status-accepted-fg)", bg: "var(--status-accepted-bg)" },
  cancelled: { fg: "var(--status-cancelled-fg)", bg: "var(--status-cancelled-bg)" },
};

export function StatusPill({ status, label, dot }: { status: string; label: string; dot?: boolean }) {
  const c = PILL[status] ?? PILL.cancelled;
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "3px 9px",
        borderRadius: "var(--radius-pill)",
        background: c.bg,
        color: c.fg,
        font: "var(--weight-semibold) var(--text-micro)/1.4 var(--font-sans)",
        letterSpacing: "var(--tracking-caps)",
        textTransform: "uppercase",
        whiteSpace: "nowrap",
      }}
    >
      {dot ? <span style={{ width: 5, height: 5, borderRadius: "50%", background: "currentColor" }} /> : null}
      {label}
    </span>
  );
}

export function Badge({ children }: { children: ReactNode }) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        padding: "2px 8px",
        borderRadius: "var(--radius-pill)",
        border: "1px solid var(--border-default)",
        font: "var(--weight-semibold) var(--text-micro)/1.5 var(--font-sans)",
        letterSpacing: "var(--tracking-caps)",
        textTransform: "uppercase",
        whiteSpace: "nowrap",
        color: "var(--ink-800)",
      }}
    >
      {children}
    </span>
  );
}

export function EmptyState({ icon, title, message }: { icon: string; title: string; message: string }) {
  return (
    <div style={{ display: "grid", justifyItems: "center", gap: "var(--space-5)", textAlign: "center", padding: "var(--space-11) var(--space-8)" }}>
      <span
        style={{
          display: "grid",
          placeItems: "center",
          width: 40,
          height: 40,
          borderRadius: "var(--radius-pill)",
          background: "var(--paper-2)",
          color: "var(--text-subtle)",
        }}
      >
        <Icon name={icon} size={19} />
      </span>
      <div style={{ display: "grid", gap: 5 }}>
        <span style={{ font: "var(--weight-regular) var(--text-title-md)/1.3 var(--font-display)", color: "var(--text-strong)" }}>{title}</span>
        <p style={{ font: "var(--weight-regular) var(--text-body-md)/var(--leading-relaxed) var(--font-sans)", color: "var(--text-muted)", maxWidth: "46ch" }}>
          {message}
        </p>
      </div>
    </div>
  );
}
