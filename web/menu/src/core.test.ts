import { describe, expect, it } from "vitest";
import { hasFromPrice, lineDetail, lineOptions, unitPriceCents } from "./cart";
import { demoSession } from "./fixtures";
import { countdownStr, fmtCents } from "./format";
import { themeVars } from "./theme";

const ribeye = demoSession.menu[1].items[0];

describe("money", () => {
  it("formats integer cents", () => {
    expect(fmtCents(4600)).toBe("$46.00");
    expect(fmtCents(300)).toBe("$3.00");
    expect(fmtCents(0)).toBe("$0.00");
  });
});

describe("countdown", () => {
  it("rounds up to whole seconds, m:ss", () => {
    expect(countdownStr(90_000)).toBe("1:30");
    expect(countdownStr(59_500)).toBe("1:00");
    expect(countdownStr(1_000)).toBe("0:01");
    expect(countdownStr(0)).toBe("0:00");
  });
});

describe("item pricing", () => {
  it("base price with defaults", () => {
    expect(unitPriceCents(ribeye, { size: "300g", doneness: "rare" }, {})).toBe(4600);
  });
  it("single-select delta plus multi-select add-ons", () => {
    const single = { size: "600g", doneness: "medium-rare" };
    const multi = { sauces: ["bearnaise", "marrow-butter"] };
    expect(unitPriceCents(ribeye, single, multi)).toBe(4600 + 2600 + 300 + 400);
    expect(lineDetail(ribeye, single, multi)).toBe(
      "600 g · to share · medium rare · béarnaise · bone marrow butter",
    );
    expect(lineOptions(ribeye, single, multi)).toEqual([
      { group_id: "size", option_ids: ["600g"] },
      { group_id: "doneness", option_ids: ["medium-rare"] },
      { group_id: "sauces", option_ids: ["bearnaise", "marrow-butter"] },
    ]);
  });
  it("from-price only when a single-select group raises the base", () => {
    expect(hasFromPrice(ribeye)).toBe(true);
    const bavette = demoSession.menu[1].items[1];
    expect(hasFromPrice(bavette)).toBe(false); // doneness free, sauces are multi
  });
});

describe("theme", () => {
  it("maps accents case-insensitively with blood red fallback", () => {
    expect(themeVars({ brand_name: "x", accent: "Wine", bold: false })["--accent-solid"]).toBe("var(--wine-600)");
    expect(themeVars({ brand_name: "x", accent: "nope", bold: false })["--accent-solid"]).toBe("var(--red-600)");
  });
  it("applies css_vars overrides last", () => {
    const v = themeVars({ brand_name: "x", accent: "Fire", bold: true, css_vars: { "--accent-solid": "#123456" } });
    expect(v["--accent-solid"]).toBe("#123456");
  });
});
