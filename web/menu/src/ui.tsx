// Small ports of the design-system Button / Badge / Icon / EmptyState
// (web/design-system/_ds_bundle.js) — token-driven, no runtime CDN fetches.

import { type CSSProperties, type ReactElement, type ReactNode, useState } from "react";

const GLYPHS: Record<string, ReactElement> = {
  check: <path d="M20 6 9 17l-5-5" />,
  "arrow-left": (
    <>
      <path d="m12 19-7-7 7-7" />
      <path d="M19 12H5" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 6v6l4 2" />
    </>
  ),
  bell: (
    <>
      <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
      <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
    </>
  ),
  "bell-ring": (
    <>
      <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
      <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
      <path d="M4 2C2.8 3.7 2 5.7 2 8" />
      <path d="M22 8c0-2.3-.8-4.3-2-6" />
    </>
  ),
  receipt: (
    <>
      <path d="M4 2v20l2-1 2 1 2-1 2 1 2-1 2 1 2-1 2 1V2l-2 1-2-1-2 1-2-1-2 1-2-1-2 1Z" />
      <path d="M16 8h-6a2 2 0 1 0 0 4h4a2 2 0 1 1 0 4H8" />
      <path d="M12 17.5v-11" />
    </>
  ),
  utensils: (
    <>
      <path d="M3 2v7c0 1.1.9 2 2 2h4a2 2 0 0 0 2-2V2" />
      <path d="M7 2v20" />
      <path d="M21 15V2a5 5 0 0 0-5 5v6c0 1.1.9 2 2 2h3Zm0 0v7" />
    </>
  ),
  "qr-code": (
    <>
      <rect width="5" height="5" x="3" y="3" rx="1" />
      <rect width="5" height="5" x="16" y="3" rx="1" />
      <rect width="5" height="5" x="3" y="16" rx="1" />
      <path d="M21 16h-3a2 2 0 0 0-2 2v3" />
      <path d="M21 21v.01" />
      <path d="M12 7v3a2 2 0 0 1-2 2H7" />
      <path d="M3 12h.01" />
      <path d="M12 3h.01" />
      <path d="M12 16v.01" />
      <path d="M16 12h1" />
      <path d="M21 12v.01" />
      <path d="M12 21v-1" />
    </>
  ),
  "refresh-cw": (
    <>
      <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
      <path d="M21 3v5h-5" />
      <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
      <path d="M8 16H3v5" />
    </>
  ),
};

export function Icon({
  name,
  size = 18,
  style,
}: {
  name: string;
  size?: number;
  style?: CSSProperties;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      style={{ flex: "none", ...style }}
    >
      {GLYPHS[name]}
    </svg>
  );
}

const BUTTON_SIZES = {
  sm: { height: "var(--control-h-sm)", padding: "0 10px", font: "var(--weight-medium) var(--text-body-sm)/1 var(--font-sans)", gap: 6, icon: 14 },
  md: { height: "var(--control-h-md)", padding: "0 14px", font: "var(--weight-medium) var(--text-body-md)/1 var(--font-sans)", gap: 8, icon: 16 },
  touch: { height: "var(--control-h-touch)", padding: "0 24px", font: "var(--weight-semibold) var(--text-title-sm)/1 var(--font-sans)", gap: 10, icon: 20 },
};

const BUTTON_VARIANTS: Record<string, { base: CSSProperties; hover: CSSProperties }> = {
  primary: {
    base: { background: "var(--accent-solid)", color: "var(--accent-on-solid)", border: "1px solid var(--accent-solid)" },
    hover: { background: "var(--accent-solid-hover)", borderColor: "var(--accent-solid-hover)" },
  },
  secondary: {
    base: { background: "var(--surface-card)", color: "var(--text-strong)", border: "1px solid var(--border-default)" },
    hover: { background: "var(--surface-hover)", borderColor: "var(--border-strong)" },
  },
  ghost: {
    base: { background: "transparent", color: "var(--text-body)", border: "1px solid transparent" },
    hover: { background: "var(--surface-hover)" },
  },
};

export function Button({
  children,
  variant = "secondary",
  size = "md",
  iconLeft,
  disabled,
  fullWidth,
  onClick,
  style,
}: {
  children: ReactNode;
  variant?: keyof typeof BUTTON_VARIANTS;
  size?: keyof typeof BUTTON_SIZES;
  iconLeft?: string;
  disabled?: boolean;
  fullWidth?: boolean;
  onClick?: () => void;
  style?: CSSProperties;
}) {
  const [hover, setHover] = useState(false);
  const [press, setPress] = useState(false);
  const s = BUTTON_SIZES[size];
  const v = BUTTON_VARIANTS[variant];
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={disabled ? undefined : onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => {
        setHover(false);
        setPress(false);
      }}
      onMouseDown={() => setPress(true)}
      onMouseUp={() => setPress(false)}
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        width: fullWidth ? "100%" : undefined,
        height: s.height,
        padding: s.padding,
        font: s.font,
        gap: s.gap,
        letterSpacing: "var(--tracking-snug)",
        whiteSpace: "nowrap",
        borderRadius: "var(--radius-sm)",
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.42 : 1,
        transition: "var(--motion-hover), transform var(--dur-instant) var(--ease-out)",
        transform: press && !disabled ? "translateY(1px)" : "none",
        boxShadow: press && !disabled ? "var(--shadow-press)" : "none",
        ...v.base,
        ...(hover && !disabled ? v.hover : null),
        ...style,
      }}
    >
      {iconLeft ? <Icon name={iconLeft} size={s.icon} /> : null}
      {children}
    </button>
  );
}

const BADGE_TONES: Record<string, CSSProperties> = {
  neutral: { color: "var(--ink-700)", background: "var(--paper-3)" },
  warning: { color: "var(--status-closed-fg)", background: "var(--status-closed-bg)" },
  outline: {
    color: "var(--text-muted)",
    background: "transparent",
    boxShadow: "inset 0 0 0 1px var(--border-default)",
  },
};

export function Badge({
  children,
  tone = "neutral",
  uppercase,
}: {
  children: ReactNode;
  tone?: keyof typeof BADGE_TONES;
  uppercase?: boolean;
}) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        padding: "2px 8px",
        borderRadius: "var(--radius-pill)",
        font: "var(--weight-semibold) var(--text-micro)/1.5 var(--font-sans)",
        letterSpacing: uppercase ? "var(--tracking-caps)" : "var(--tracking-normal)",
        textTransform: uppercase ? "uppercase" : "none",
        whiteSpace: "nowrap",
        ...BADGE_TONES[tone],
      }}
    >
      {children}
    </span>
  );
}

export function EmptyState({
  icon,
  title,
  message,
}: {
  icon: string;
  title: string;
  message: string;
}) {
  return (
    <div style={{ display: "grid", justifyItems: "center", gap: "var(--space-5)", textAlign: "center", padding: "var(--space-11) var(--space-8)" }}>
      <span style={{ display: "grid", placeItems: "center", width: 40, height: 40, borderRadius: "var(--radius-pill)", background: "var(--paper-2)", color: "var(--text-subtle)" }}>
        <Icon name={icon} size={19} />
      </span>
      <div style={{ display: "grid", gap: 5 }}>
        <span style={{ font: "var(--weight-regular) var(--text-title-md)/1.3 var(--font-display)", color: "var(--text-strong)" }}>
          {title}
        </span>
        <p style={{ font: "var(--weight-regular) var(--text-body-md)/var(--leading-relaxed) var(--font-sans)", color: "var(--text-muted)", maxWidth: "46ch" }}>
          {message}
        </p>
      </div>
    </div>
  );
}

export function QtyStepper({
  qty,
  onInc,
  onDec,
  compact,
}: {
  qty: number;
  onInc: () => void;
  onDec: () => void;
  compact?: boolean;
}) {
  const h = compact ? 42 : 46;
  const numW = compact ? 38 : 40;
  const btnFont = compact ? "500 17px/1 var(--font-mono)" : "500 18px/1 var(--font-mono)";
  const numFont = compact ? "600 14px/1 var(--font-mono)" : "600 15px/1 var(--font-mono)";
  const btn: CSSProperties = {
    width: 44,
    height: h,
    display: "grid",
    placeItems: "center",
    font: btnFont,
    color: "var(--ink-700)",
    cursor: "pointer",
    background: "none",
    border: "none",
    padding: 0,
  };
  return (
    <div style={{ display: "flex", alignItems: "center", border: "1px solid var(--border-strong)", borderRadius: 6, overflow: "hidden", background: "var(--paper-0)" }}>
      <button type="button" aria-label="Decrease quantity" onClick={onDec} style={{ ...btn, borderRight: "1px solid var(--border-subtle)" }}>
        −
      </button>
      <span className="aivo-num" style={{ width: numW, textAlign: "center", font: numFont, color: "var(--ink-900)" }}>
        {qty}
      </span>
      <button type="button" aria-label="Increase quantity" onClick={onInc} style={{ ...btn, borderLeft: "1px solid var(--border-subtle)" }}>
        +
      </button>
    </div>
  );
}

export function Placeholder({
  label,
  style,
}: {
  label: string;
  style?: CSSProperties;
}) {
  return (
    <div
      style={{
        background: "var(--paper-3)",
        display: "grid",
        placeItems: "center",
        font: "500 10px/1 var(--font-sans)",
        letterSpacing: "0.06em",
        textTransform: "uppercase",
        color: "var(--ink-300)",
        ...style,
      }}
    >
      {label}
    </div>
  );
}
