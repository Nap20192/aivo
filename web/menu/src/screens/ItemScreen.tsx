import { useState } from "react";
import {
  hasFromPrice,
  lineDetail,
  lineOptions,
  unitPriceCents,
  type CartLine,
  type MultiSel,
  type SingleSel,
} from "../cart";
import { fmtCents } from "../format";
import type { MenuItem, OptionGroup } from "../types";
import { Badge, Button, Icon, Placeholder, QtyStepper } from "../ui";

function SingleGroup({
  group,
  selected,
  onPick,
}: {
  group: OptionGroup;
  selected: string;
  onPick: (optionId: string) => void;
}) {
  // A group where every option is free shows no per-row price; otherwise
  // free options read "included" next to the priced ones.
  const allFree = group.options.every((o) => o.price_delta_cents === 0);
  return (
    <div style={{ padding: "18px 18px 0" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
        <span style={{ font: "600 11px/1.2 var(--font-sans)", letterSpacing: "0.07em", textTransform: "uppercase", color: "var(--ink-500)" }}>
          {group.name}
        </span>
        <span style={{ font: "var(--weight-regular) 11px/1.2 var(--font-sans)", color: "var(--ink-400)" }}>pick one</span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {group.options.map((o) => {
          const isSel = o.id === selected;
          return (
            <div
              key={o.id}
              onClick={() => onPick(o.id)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 12,
                padding: "12px 14px",
                border: isSel ? "1px solid var(--accent-solid)" : "1px solid var(--border-strong)",
                background: isSel ? "var(--red-50)" : "var(--paper-0)",
                borderRadius: 6,
                cursor: "pointer",
              }}
            >
              <span
                style={{
                  width: 18,
                  height: 18,
                  flex: "none",
                  borderRadius: "50%",
                  border: isSel ? "2px solid var(--accent-solid)" : "2px solid var(--border-strong)",
                  display: "grid",
                  placeItems: "center",
                }}
              >
                {isSel ? <span style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--accent-solid)" }} /> : null}
              </span>
              <span style={{ flex: 1, font: "var(--type-label)", color: "var(--ink-800)" }}>{o.label}</span>
              <span
                className="aivo-num"
                style={{ font: "var(--type-numeric)", color: o.price_delta_cents ? "var(--ink-700)" : "var(--ink-400)" }}
              >
                {o.price_delta_cents ? "+" + fmtCents(o.price_delta_cents) : allFree ? "" : "included"}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function MultiGroup({
  group,
  selected,
  onToggle,
}: {
  group: OptionGroup;
  selected: string[];
  onToggle: (optionId: string) => void;
}) {
  return (
    <div style={{ padding: "18px 18px 0" }}>
      <div style={{ font: "600 11px/1.2 var(--font-sans)", letterSpacing: "0.07em", textTransform: "uppercase", color: "var(--ink-500)", marginBottom: 10 }}>
        {group.name} · any number
      </div>
      <div style={{ background: "var(--paper-0)", border: "1px solid var(--border-default)", borderRadius: 10, padding: "4px 14px" }}>
        {group.options.map((o, i) => {
          const checked = selected.includes(o.id);
          return (
            <div
              key={o.id}
              onClick={() => onToggle(o.id)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 12,
                padding: "11px 0",
                borderBottom: i === group.options.length - 1 ? "none" : "1px dashed var(--border-default)",
                cursor: "pointer",
              }}
            >
              <span
                style={{
                  width: 18,
                  height: 18,
                  flex: "none",
                  borderRadius: 4,
                  border: checked ? "1px solid var(--accent-solid)" : "1px solid var(--border-strong)",
                  background: checked ? "var(--accent-solid)" : "var(--paper-0)",
                  display: "grid",
                  placeItems: "center",
                  color: "#fff",
                }}
              >
                {checked ? <Icon name="check" size={12} /> : null}
              </span>
              <span style={{ flex: 1, font: "var(--type-label)", color: "var(--ink-800)" }}>{o.label}</span>
              <span className="aivo-num" style={{ font: "var(--type-numeric)", color: "var(--ink-700)" }}>
                +{fmtCents(o.price_delta_cents)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function ItemScreen({
  item,
  onAdd,
  onBack,
}: {
  item: MenuItem;
  onAdd: (line: CartLine) => void;
  onBack: () => void;
}) {
  const [single, setSingle] = useState<SingleSel>(() => {
    const init: SingleSel = {};
    for (const g of item.option_groups) {
      if (g.select === "single" && g.options[0]) init[g.id] = g.options[0].id;
    }
    return init;
  });
  const [multi, setMulti] = useState<MultiSel>({});
  const [qty, setQty] = useState(1);

  const soldOut = !item.available;
  const dim = soldOut ? { opacity: 0.42 } : {};
  const unit = unitPriceCents(item, single, multi);
  const singles = item.option_groups.filter((g) => g.select === "single");
  const multis = item.option_groups.filter((g) => g.select === "multi");

  return (
    <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ flex: "none", padding: "6px 8px", background: "var(--paper-0)", borderBottom: "1px solid var(--border-subtle)", display: "flex" }}>
        <Button variant="ghost" size="sm" iconLeft="arrow-left" onClick={onBack}>
          Menu
        </Button>
      </div>
      <div style={{ flex: 1, overflowY: "auto", minHeight: 0, paddingBottom: 20 }}>
        {item.image_url ? (
          <img src={item.image_url} alt="" style={{ width: "100%", height: 150, objectFit: "cover", display: "block", ...dim }} />
        ) : (
          <Placeholder label="photo" style={{ height: 150, ...dim }} />
        )}
        <div style={{ padding: "16px 18px 0" }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 12, ...dim }}>
            <h2 style={{ margin: 0, font: "var(--weight-regular) 22px/1.2 var(--font-display)", letterSpacing: "-0.02em", color: "var(--ink-900)" }}>
              {item.name}
            </h2>
            <span className="aivo-num" style={{ font: "500 15px/1.3 var(--font-mono)", color: "var(--ink-900)", flex: "none" }}>
              {(hasFromPrice(item) ? "from " : "") + fmtCents(item.price_cents)}
            </span>
          </div>
          {soldOut ? (
            <div style={{ marginTop: 14, background: "var(--yellow-100)", border: "1px solid var(--yellow-200)", borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ font: "600 14px/1.3 var(--font-sans)", color: "var(--yellow-800)", marginBottom: 4 }}>
                Off the menu tonight
              </div>
              <div style={{ font: "var(--weight-regular) 13px/1.5 var(--font-sans)", color: "var(--ink-700)" }}>
                The kitchen sold out at {item.sold_out_at}. We're leaving it here so you know it exists — ask the waiter about tomorrow.
              </div>
            </div>
          ) : null}
          <p
            style={{
              margin: soldOut ? "16px 0 0" : "8px 0 0",
              font: "var(--weight-regular) 14px/1.5 var(--font-sans)",
              color: soldOut ? "var(--ink-500)" : "var(--ink-600)",
              textWrap: "pretty",
              ...(soldOut ? { opacity: 0.42 } : {}),
            }}
          >
            {item.description}
          </p>
          <div style={{ display: "flex", gap: 6, marginTop: 10, flexWrap: "wrap" }}>
            {item.allergens.map((a) => (
              <Badge key={a} tone="neutral">
                {a}
              </Badge>
            ))}
          </div>
        </div>
        {!soldOut ? (
          <>
            {singles.map((g) => (
              <SingleGroup
                key={g.id}
                group={g}
                selected={single[g.id] ?? ""}
                onPick={(id) => setSingle({ ...single, [g.id]: id })}
              />
            ))}
            {multis.map((g) => (
              <MultiGroup
                key={g.id}
                group={g}
                selected={multi[g.id] ?? []}
                onToggle={(id) => {
                  const cur = multi[g.id] ?? [];
                  setMulti({
                    ...multi,
                    [g.id]: cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id],
                  });
                }}
              />
            ))}
          </>
        ) : null}
      </div>
      {!soldOut ? (
        <div style={{ flex: "none", display: "flex", alignItems: "center", gap: 12, padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
          <QtyStepper qty={qty} onDec={() => setQty(Math.max(1, qty - 1))} onInc={() => setQty(Math.min(9, qty + 1))} />
          <div style={{ flex: 1 }}>
            <Button
              variant="primary"
              size="touch"
              fullWidth
              onClick={() =>
                onAdd({
                  menuItemId: item.id,
                  name: item.name,
                  unitCents: unit,
                  qty,
                  detail: lineDetail(item, single, multi),
                  options: lineOptions(item, single, multi),
                })
              }
            >
              Add · {fmtCents(unit * qty)}
            </Button>
          </div>
        </div>
      ) : (
        <div style={{ flex: "none", padding: "12px 14px 16px", background: "var(--paper-0)", borderTop: "1px solid var(--border-default)" }}>
          <Button variant="primary" size="touch" fullWidth disabled>
            Can't be ordered tonight
          </Button>
        </div>
      )}
    </div>
  );
}
