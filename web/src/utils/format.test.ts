import { describe, it, expect } from "vitest";
import { costBasisSub, costSpendLabel, formatCost, formatTokens } from "./format";

describe("formatCost", () => {
  it("formats zero", () => {
    expect(formatCost(0)).toBe("$0.00");
  });
  it("formats sub-cent values with 4 decimals", () => {
    expect(formatCost(0.0001)).toBe("$0.0001");
    expect(formatCost(0.0095)).toBe("$0.0095");
  });
  it("formats ordinary values with 2 decimals", () => {
    expect(formatCost(1.5)).toBe("$1.50");
    expect(formatCost(42)).toBe("$42.00");
    expect(formatCost(99.999)).toBe("$100.00");
  });
  it("inserts thousand separators", () => {
    expect(formatCost(1234.56)).toBe("$1,234.56");
    expect(formatCost(1_000_000)).toBe("$1,000,000.00");
    expect(formatCost(9_876_543.21)).toBe("$9,876,543.21");
  });
});

describe("costSpendLabel", () => {
  it("defaults missing basis to estimated (priced)", () => {
    expect(costSpendLabel(undefined)).toBe("Estimated spend");
    expect(costSpendLabel(undefined, "cost")).toBe("Estimated cost");
  });
  it("labels billed when the API says so", () => {
    expect(costSpendLabel("billed")).toBe("Billed spend");
    expect(costSpendLabel("billed", "cost")).toBe("Billed cost");
  });
  it("explains priced dollars as not provider billing", () => {
    expect(costBasisSub("priced")).toMatch(/not provider billing/);
    expect(costBasisSub(undefined)).toMatch(/not provider billing/);
    expect(costBasisSub("billed")).toMatch(/provider billing/);
  });
});

describe("formatTokens", () => {
  it("formats millions", () => {
    expect(formatTokens(1_500_000)).toBe("1.5M");
  });
  it("formats thousands", () => {
    expect(formatTokens(1500)).toBe("2K");
  });
  it("formats small values literally", () => {
    expect(formatTokens(42)).toBe("42");
  });

  // Boundary checks — guard against a regression where callers pre-scale
  // the value and produce absurd outputs like "44307.0M".
  it("keeps sub-thousand values as literals", () => {
    expect(formatTokens(999)).toBe("999");
  });
  it("renders exactly 1_000 as 1K", () => {
    expect(formatTokens(1000)).toBe("1K");
  });
  it("renders just-below-a-million as K", () => {
    expect(formatTokens(999_999)).toBe("1000K");
  });
  it("renders exactly 1_000_000 as 1.0M", () => {
    expect(formatTokens(1_000_000)).toBe("1.0M");
  });
  it("renders a typical agent total (1.7M tokens)", () => {
    expect(formatTokens(1_703_408)).toBe("1.7M");
  });
  it("does not double-divide huge raw counts", () => {
    // 44_307_000_000 raw tokens is the magic number behind the '44307.0M'
    // audit report. The formatter should render it literally (44307.0M),
    // confirming no second-stage division. The bug was that a caller
    // pre-divided a ~44 trillion raw count down to 44 B and then the
    // formatter divided again; this test pins the contract.
    expect(formatTokens(44_307_000_000)).toBe("44307.0M");
    // And the truly absurd case ("already in millions" double-division)
    // should never appear for real data: a 1.7M raw count formatted twice
    // would yield something like "1.7" — not the audit's value.
    expect(formatTokens(1.7)).not.toBe("44307.0M");
  });
  it("guards negative and NaN input", () => {
    expect(formatTokens(-1)).toBe("0");
    expect(formatTokens(Number.NaN)).toBe("0");
    expect(formatTokens(Number.POSITIVE_INFINITY)).toBe("0");
  });
});
