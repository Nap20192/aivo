import { describe, expect, it } from "vitest";
import { ACCENT_NAMES, themeVars } from "./theme";

describe("themeVars", () => {
  it("lowercases the accent key once, here", () => {
    expect(themeVars({ accent: "Wine" })["--accent-solid"]).toBe("var(--wine-600)");
    expect(themeVars({ accent: "wine" })["--accent-solid"]).toBe("var(--wine-600)");
    expect(themeVars({ accent: "BLOOD RED" })["--accent-solid"]).toBe("var(--red-600)");
  });
  it("falls back to blood red for unknown/missing accents", () => {
    expect(themeVars({ accent: "nope" })["--accent-solid"]).toBe("var(--red-600)");
    expect(themeVars({})["--accent-solid"]).toBe("var(--red-600)");
  });
  it("applies css_vars overrides last", () => {
    expect(themeVars({ accent: "Fire", css_vars: { "--accent-solid": "#123456" } })["--accent-solid"]).toBe("#123456");
  });
  it("exports the accent list for pickers", () => {
    expect(ACCENT_NAMES).toEqual(["Blood red", "Olive", "Wine", "Fire"]);
  });
});
