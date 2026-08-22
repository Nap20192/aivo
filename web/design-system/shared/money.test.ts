import { describe, expect, it } from "vitest";
import { formatCents, parseDollars } from "./money";

describe("formatCents", () => {
  it("formats negatives as -$x.xx", () => {
    expect(formatCents(-50)).toBe("-$0.50");
    expect(formatCents(1250)).toBe("$12.50");
    expect(formatCents(0)).toBe("$0.00");
  });
});

describe("parseDollars", () => {
  it("accepts the union grammar", () => {
    expect(parseDollars("12.50")).toBe(1250);
    expect(parseDollars("$12.50")).toBe(1250);
    expect(parseDollars("12,50")).toBe(1250);
    expect(parseDollars("12")).toBe(1200);
    expect(parseDollars("150.")).toBe(15000);
  });
  it("rejects garbage", () => {
    expect(parseDollars("")).toBeNull();
    expect(parseDollars("abc")).toBeNull();
    expect(parseDollars("-5")).toBeNull();
    expect(parseDollars("1.2.3")).toBeNull();
  });
});
