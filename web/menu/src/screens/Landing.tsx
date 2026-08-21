import type { TableSession } from "../types";
import { Badge, Button, Placeholder } from "../ui";

export function Landing({
  session,
  browse,
  onMenu,
}: {
  session: TableSession;
  browse: boolean;
  onMenu: () => void;
}) {
  const { restaurant, table, theme } = session;
  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0 }}>
        {theme.banner_url ? (
          <img
            src={theme.banner_url}
            alt=""
            style={{ width: "100%", height: 168, objectFit: "cover", display: "block", borderBottom: "1px solid var(--border-default)" }}
          />
        ) : (
          <Placeholder label="banner photo" style={{ height: 168, borderBottom: "1px solid var(--border-default)" }} />
        )}
        <div style={{ padding: "20px 18px 0" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
            <span style={{ font: "600 20px/1 var(--font-sans)", letterSpacing: "-0.01em", color: "var(--ink-900)" }}>
              {theme.brand_name}
              <span style={{ color: "var(--accent-solid)" }}>.</span>
            </span>
            {!browse && table.label ? (
              <Badge tone="outline" uppercase>
                {table.label}
              </Badge>
            ) : null}
          </div>
          {restaurant.tagline ? (
            <p style={{ margin: "12px 0 0", font: "var(--weight-regular) 15px/1.6 var(--font-sans)", color: "var(--ink-700)", textWrap: "pretty" }}>
              {restaurant.tagline}
            </p>
          ) : null}
        </div>
        <div style={{ padding: "20px 18px 20px", display: "flex", flexDirection: "column", gap: 10 }}>
          {restaurant.hours.length > 0 ? (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ font: "600 11px/1.2 var(--font-sans)", letterSpacing: "0.07em", textTransform: "uppercase", color: "var(--ink-400)", marginBottom: 10 }}>
                Open today
              </div>
              {restaurant.hours.map((h, i) => (
                <div
                  key={h.label}
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    ...(i < restaurant.hours.length - 1
                      ? { borderBottom: "1px dashed var(--border-default)", paddingBottom: 8, marginBottom: 8 }
                      : {}),
                  }}
                >
                  <span style={{ font: "var(--type-body)", color: "var(--ink-700)" }}>{h.label}</span>
                  <span style={{ font: "var(--type-numeric)", color: "var(--ink-900)" }}>
                    {h.open} – {h.close}
                  </span>
                </div>
              ))}
            </div>
          ) : null}
          {restaurant.address ? (
            <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, overflow: "hidden" }}>
              <Placeholder label="map" style={{ height: 96, borderBottom: "1px solid var(--border-subtle)", background: "var(--paper-2)" }} />
              <div style={{ padding: "12px 16px", display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
                <span style={{ font: "var(--type-body)", color: "var(--ink-700)" }}>{restaurant.address}</span>
                {restaurant.map_url ? (
                  <a href={restaurant.map_url} target="_blank" rel="noreferrer" style={{ font: "var(--type-label)", color: "var(--text-link)" }}>
                    Directions
                  </a>
                ) : null}
              </div>
            </div>
          ) : null}
          {restaurant.phone || restaurant.instagram ? (
            <div style={{ display: "flex", gap: 8 }}>
              {restaurant.phone ? (
                <a
                  href={`tel:${restaurant.phone.replace(/\s/g, "")}`}
                  style={{ flex: 1, background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: "12px 14px", font: "var(--type-label)", color: "var(--ink-700)" }}
                >
                  Call · {restaurant.phone}
                </a>
              ) : null}
              {restaurant.instagram ? (
                <a
                  href={`https://instagram.com/${restaurant.instagram}`}
                  target="_blank"
                  rel="noreferrer"
                  style={{ flex: "none", background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: "12px 14px", font: "var(--type-label)", color: "var(--ink-700)" }}
                >
                  Instagram
                </a>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
      <div style={{ flex: "none", padding: "12px 16px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
        <Button variant="primary" size="touch" fullWidth iconLeft="utensils" onClick={onMenu}>
          See the menu
        </Button>
      </div>
    </div>
  );
}
