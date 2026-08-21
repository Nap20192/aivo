// Live phone-frame preview of the diner menu. Mirrors
// docs/prototypes/aivo-menu-prototype.dc.html — same themeVars mapping,
// same card markup, driven by the restaurant's real categories and items.
import { CSSProperties, useState } from "react";
import type { Accent, Category, MenuItem, Theme } from "../api/types";
import { formatCents } from "../lib/money";

const ACCENT_MAP: Record<Accent, [string, string, string, string, string]> = {
  "Blood red": ["--red-600", "--red-700", "--red-800", "--red-100", "--red-50"],
  Olive: ["--olive-600", "--olive-700", "--olive-800", "--olive-100", "--olive-100"],
  Wine: ["--wine-600", "--wine-700", "--wine-800", "--wine-100", "--wine-100"],
  Fire: ["--orange-600", "--orange-700", "--orange-800", "--orange-100", "--orange-100"],
};

// Exactly the prototype's themeVars, then custom overrides on top.
export function themeVars(theme: Theme): CSSProperties {
  const [solid, hover, active, soft, wash] =
    ACCENT_MAP[theme.accent] ?? ACCENT_MAP["Blood red"];
  const vars: Record<string, string> = {
    "--accent-solid": `var(${solid})`,
    "--accent-solid-hover": `var(${hover})`,
    "--accent-solid-active": `var(${active})`,
    "--accent-soft": `var(${soft})`,
    "--red-50": `var(${wash})`,
    "--text-link": `var(${hover})`,
    "--text-link-hover": `var(${active})`,
    ...theme.css_vars,
  };
  return vars as CSSProperties;
}

export default function MenuPreview(props: {
  theme: Theme;
  categories: Category[];
  items: MenuItem[];
}) {
  const { theme } = props;
  const cats = [...props.categories].sort((a, b) => a.position - b.position);
  const [catIdx, setCatIdx] = useState(0);
  const active = cats[Math.min(catIdx, Math.max(0, cats.length - 1))];
  const items = active
    ? props.items.filter((i) => i.category_id === active.id)
    : [];

  return (
    <div className="phone-frame">
      <div
        className={
          "phone-inner" + (theme.bold ? " theme-bold" : "")
        }
        style={themeVars(theme)}
      >
        {theme.banner_url && (
          <div
            style={{
              height: 96,
              flex: "none",
              background: `url(${theme.banner_url}) center/cover`,
              borderBottom: "1px solid var(--border-default)",
            }}
          />
        )}
        <div
          style={{
            flex: "none",
            padding: "12px 14px 10px",
            background: "var(--paper-0)",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 12,
          }}
        >
          <span
            style={{
              font: "600 14px/1 var(--font-sans)",
              letterSpacing: "-0.01em",
              color: "var(--ink-900)",
            }}
          >
            {theme.brand_name || "Your restaurant"}
            <span style={{ color: "var(--accent-solid)" }}>.</span>
          </span>
          <span
            className="badge badge-outline badge-caps"
            style={{ flex: "none" }}
          >
            Table 12
          </span>
        </div>
        <div
          style={{
            flex: "none",
            display: "flex",
            gap: 6,
            padding: "0 14px 10px",
            background: "var(--paper-0)",
            borderBottom: "1px solid var(--border-default)",
            overflowX: "auto",
          }}
        >
          {cats.map((c, i) => (
            <span
              key={c.id}
              onClick={() => setCatIdx(i)}
              style={
                c.id === active?.id
                  ? {
                      flex: "none",
                      padding: "6px 11px",
                      borderRadius: 999,
                      background: "var(--ink-900)",
                      color: "var(--paper-0)",
                      font: "var(--type-label)",
                      fontSize: 12,
                      cursor: "pointer",
                    }
                  : {
                      flex: "none",
                      padding: "6px 11px",
                      borderRadius: 999,
                      background: "var(--paper-2)",
                      border: "1px solid var(--border-default)",
                      color: "var(--ink-700)",
                      font: "var(--type-label)",
                      fontSize: 12,
                      cursor: "pointer",
                    }
              }
            >
              {c.name}
            </span>
          ))}
          {cats.length === 0 && (
            <span
              style={{
                font: "var(--type-body)",
                fontSize: 12,
                color: "var(--text-subtle)",
                padding: "6px 0",
              }}
            >
              Menu categories appear here
            </span>
          )}
        </div>
        <div
          style={{
            flex: 1,
            minHeight: 0,
            padding: 12,
            display: "flex",
            flexDirection: "column",
            gap: 10,
            background: "var(--paper-1)",
          }}
        >
          {items.map((mi) => (
            <div
              key={mi.id}
              style={{
                display: "flex",
                gap: 10,
                background: "var(--paper-0)",
                border: mi.available
                  ? "1px solid var(--border-default)"
                  : "1px dashed var(--border-strong)",
                borderRadius: 10,
                padding: 10,
              }}
            >
              <div
                style={{
                  width: 56,
                  height: 56,
                  flex: "none",
                  borderRadius: 8,
                  background: mi.image_url
                    ? `url(${mi.image_url}) center/cover`
                    : "var(--paper-3)",
                  display: "grid",
                  placeItems: "center",
                  font: "500 9px/1 var(--font-sans)",
                  letterSpacing: "0.06em",
                  textTransform: "uppercase",
                  color: "var(--ink-300)",
                  opacity: mi.available ? 1 : 0.42,
                }}
              >
                {mi.image_url ? "" : "photo"}
              </div>
              <div style={{ flex: 1, minWidth: 0, opacity: mi.available ? 1 : 0.42 }}>
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    gap: 8,
                  }}
                >
                  <span
                    style={{
                      font: "600 13px/1.25 var(--font-sans)",
                      color: "var(--ink-900)",
                    }}
                  >
                    {mi.name}
                  </span>
                  <span
                    className="aivo-num"
                    style={{ font: "var(--type-numeric)", fontSize: 12, color: "var(--ink-900)" }}
                  >
                    {formatCents(mi.price_cents)}
                  </span>
                </div>
                <span
                  style={{
                    font: "var(--weight-regular) 11px/1.45 var(--font-sans)",
                    color: "var(--ink-500)",
                    display: "block",
                    marginTop: 2,
                  }}
                >
                  {mi.description}
                </span>
                {!mi.available && (
                  <span
                    className="badge badge-warn"
                    style={{ marginTop: 4 }}
                  >
                    off the menu tonight
                  </span>
                )}
              </div>
            </div>
          ))}
          {active && items.length === 0 && (
            <span
              style={{
                font: "var(--type-body)",
                fontSize: 12,
                color: "var(--text-subtle)",
                textAlign: "center",
                padding: 20,
              }}
            >
              No items in {active.name} yet
            </span>
          )}
        </div>
        <div
          style={{
            flex: "none",
            display: "grid",
            gridTemplateColumns: "1fr 1fr 1.25fr",
            gap: 6,
            padding: "8px 10px 10px",
            background: "var(--paper-0)",
            borderTop: "1px solid var(--border-default)",
          }}
        >
          <span className="btn btn-secondary btn-sm" style={{ pointerEvents: "none", fontSize: 11 }}>
            Waiter
          </span>
          <span className="btn btn-secondary btn-sm" style={{ pointerEvents: "none", fontSize: 11 }}>
            Bill
          </span>
          <span className="btn btn-primary btn-sm" style={{ pointerEvents: "none", fontSize: 11 }}>
            Cart
          </span>
        </div>
      </div>
    </div>
  );
}
