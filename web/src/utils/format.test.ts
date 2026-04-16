import { describe, it, expect } from "vitest";
import { formatCost, formatTokens } from "./format";

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
});
