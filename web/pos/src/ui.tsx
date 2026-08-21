import type { CSSProperties, ReactNode } from "react";

// Lucide glyphs (ISC) inlined so the PWA needs no icon CDN.
const PATHS: Record<string, ReactNode> = {
  "arrow-left": (
    <>
      <path d="m12 19-7-7 7-7" />
      <path d="M19 12H5" />
    </>
  ),
  bell: (
    <>
      <path d="M10.268 21a2 2 0 0 0 3.464 0" />
      <path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326" />
    </>
  ),
  "bell-ring": (
    <>
      <path d="M10.268 21a2 2 0 0 0 3.464 0" />
      <path d="M22 8c0-2.3-.8-4.3-2-6" />
      <path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326" />
      <path d="M4 2C2.8 3.7 2 5.7 2 8" />
    </>
  ),
  check: <path d="M20 6 9 17l-5-5" />,
  flame: (
    <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z" />
  ),
  lock: (
    <>
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  plus: (
    <>
      <path d="M5 12h14" />
      <path d="M12 5v14" />
    </>
  ),
  receipt: (
    <>
      <path d="M4 2v20l2-1 2 1 2-1 2 1 2-1 2 1 2-1 2 1V2l-2 1-2-1-2 1-2-1-2 1-2-1-2 1Z" />
      <path d="M16 8h-6a2 2 0 1 0 0 4h4a2 2 0 1 1 0 4H8" />
      <path d="M12 17.5v-11" />
    </>
  ),
  split: (
    <>
      <path d="M16 3h5v5" />
      <path d="M8 3H3v5" />
      <path d="M12 22v-8.3a4 4 0 0 0-1.172-2.872L3 3" />
      <path d="m15 9 6-6" />
    </>
  ),
  "triangle-alert": (
    <>
      <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 20h16a2 2 0 0 0 1.73-2" />
      <path d="M12 9v4" />
      <path d="M12 17h.01" />
    </>
  ),
  utensils: (
    <>
      <path d="M3 2v7c0 1.1.9 2 2 2h4a2 2 0 0 0 2-2V2" />
      <path d="M7 2v20" />
      <path d="M21 15V2a5 5 0 0 0-5 5v6c0 1.1.9 2 2 2h3Zm0 0v7" />
    </>
  ),
};

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
    >
      {PATHS[name]}
    </svg>
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
