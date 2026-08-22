import { describe, expect, it } from "vitest";

// Node test env has no sessionStorage; the mock client needs one.
if (typeof sessionStorage === "undefined") {
  const store = new Map<string, string>();
  (globalThis as { sessionStorage?: unknown }).sessionStorage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, String(v)),
    removeItem: (k: string) => void store.delete(k),
  };
}
import { genHandoffCode, HANDOFF_CHARSET, mockClient, normalizeSession, pseudoQrDataUri } from "./api";
import { handoffExpired, hasFromPrice, lineDetail, lineOptions, unitPriceCents } from "./cart";
import { demoSession } from "./fixtures";
import { countdownStr, fmtCents } from "./format";
import { themeVars } from "./theme";
import type { TableSession } from "./types";

const dinner = demoSession.menus[0];
const ribeye = dinner.categories[1].items[0];

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
    const bavette = dinner.categories[1].items[1];
    expect(hasFromPrice(bavette)).toBe(false); // doneness free, sauces are multi
  });
});

describe("multi-menu contract", () => {
  it("fixtures: default menu first, unique slugs", () => {
    expect(demoSession.menus[0].is_default).toBe(true);
    expect(demoSession.menus.map((m) => m.slug)).toEqual(["dinner", "bar"]);
  });
  it("normalizeSession wraps a legacy flat menu into a default menu", () => {
    const legacy = {
      ...demoSession,
      menus: undefined,
      menu: dinner.categories,
    } as unknown as TableSession;
    const s = normalizeSession(legacy);
    expect(s.menus).toHaveLength(1);
    expect(s.menus[0].is_default).toBe(true);
    expect(s.menus[0].categories).toBe(dinner.categories);
  });
  it("mock browse returns one menu by slug, 404 otherwise", async () => {
    const b = await mockClient.getBrowse("ember-and-bone", "bar");
    expect(b.menu.name).toBe("Bar");
    expect(b.restaurant.slug).toBe("ember-and-bone");
    await expect(mockClient.getBrowse("ember-and-bone", "brunch")).rejects.toMatchObject({ status: 404 });
    await expect(mockClient.getBrowse("nope", "dinner")).rejects.toMatchObject({ status: 404 });
  });
});

describe("handoff", () => {
  it("codes are 6 chars from the lookalike-free alphabet", () => {
    expect(HANDOFF_CHARSET).not.toMatch(/[0O1I]/);
    for (let i = 0; i < 50; i++) {
      expect(genHandoffCode()).toMatch(/^[A-HJ-NP-Z2-9]{6}$/);
    }
  });
  it("pseudo QR is a deterministic svg data uri", () => {
    expect(pseudoQrDataUri("K7M2PX")).toBe(pseudoQrDataUri("K7M2PX"));
    expect(pseudoQrDataUri("K7M2PX")).not.toBe(pseudoQrDataUri("AAAAAA"));
    expect(pseudoQrDataUri("K7M2PX").startsWith("data:image/svg+xml,")).toBe(true);
  });
  it("expiry check", () => {
    const stored = {
      handoff: { code: "K7M2PX", qr_url: "", expires_at: "2026-08-22T12:15:00Z" },
      backup: [],
    };
    const t = new Date("2026-08-22T12:15:00Z").getTime();
    expect(handoffExpired(stored, t - 1)).toBe(false);
    expect(handoffExpired(stored, t)).toBe(true);
  });
  it("mock order cooldown matches the server's 30s debounce via retry_after", async () => {
    await mockClient.submitOrder("cool-token", { lines: [] });
    const err = await mockClient.submitOrder("cool-token", { lines: [] }).catch((e) => e);
    expect(err.status).toBe(429);
    expect(err.retryAfterSeconds).toBeGreaterThan(0);
    expect(err.retryAfterSeconds).toBeLessThanOrEqual(30);
    expect(demoSession.cooldown_seconds).toBe(30);
  });
  it("mock handoff replaces the previous active code", async () => {
    const order = { lines: [] };
    const a = await mockClient.submitHandoff("tt", order);
    const b = await mockClient.submitHandoff("tt", order);
    expect(sessionStorage.getItem("aivo:mock:tt:handoff")).toContain(b.code);
    expect(a.code).not.toBe(b.code);
  });
});

describe("customer auth (mock)", () => {
  it("login validates the seeded credentials, me reflects session, logout clears", async () => {
    await expect(mockClient.login("guest@ember.test", "wrong")).rejects.toMatchObject({ status: 401 });
    expect(await mockClient.me()).toBeNull();
    const c = await mockClient.login("guest@ember.test", "embertest1");
    expect(c.name).toBe("Alex Guest");
    const m = await mockClient.me();
    expect(m?.orders).toHaveLength(2);
    expect(m?.orders[0].total_cents).toBe(8700);
    await mockClient.logout();
    expect(await mockClient.me()).toBeNull();
  });
  it("register validates at the boundary", async () => {
    await expect(mockClient.register("bad", "short", "")).rejects.toMatchObject({ status: 422 });
    const c = await mockClient.register("new@example.com", "longenough", "Nia");
    expect(c.email).toBe("new@example.com");
    await mockClient.logout();
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
