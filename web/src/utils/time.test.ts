import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { formatAbsolute, formatDuration, formatRelative } from "./time";

const NOW = new Date("2026-07-02T12:00:00Z");

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("formatRelative", () => {
  it("returns em-dash for null / undefined / empty", () => {
    expect(formatRelative(null)).toBe("—");
    expect(formatRelative(undefined)).toBe("—");
    expect(formatRelative("")).toBe("—");
  });

  it("honors a custom empty label", () => {
    expect(formatRelative(null, { emptyLabel: "never" })).toBe("never");
  });

  it("returns em-dash for invalid input", () => {
    expect(formatRelative("not-a-date")).toBe("—");
  });

  it("returns 'just now' for sub-second deltas", () => {
    expect(formatRelative(NOW)).toBe("just now");
    expect(formatRelative(new Date(NOW.getTime() - 500))).toBe("just now");
  });

  it("formats seconds / minutes / hours / days with floor", () => {
    expect(formatRelative(new Date(NOW.getTime() - 5 * 1000))).toBe("5s ago");
    expect(formatRelative(new Date(NOW.getTime() - 59 * 1000))).toBe("59s ago");
    expect(formatRelative(new Date(NOW.getTime() - 60 * 1000))).toBe("1m ago");
    expect(formatRelative(new Date(NOW.getTime() - 90 * 1000))).toBe("1m ago");
    expect(formatRelative(new Date(NOW.getTime() - 2 * 3_600_000))).toBe("2h ago");
    expect(formatRelative(new Date(NOW.getTime() - 3 * 86_400_000))).toBe("3d ago");
  });

  it("falls back to a date string beyond maxDays (default 30)", () => {
    const long = new Date(NOW.getTime() - 45 * 86_400_000);
    const result = formatRelative(long);
    expect(result).not.toContain("ago");
    expect(result).toBe(
      long.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }),
    );
  });

  it("respects a custom maxDays", () => {
    const week = new Date(NOW.getTime() - 8 * 86_400_000);
    expect(formatRelative(week, { maxDays: 7 })).toBe(
      week.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }),
    );
  });

  it("collapses future dates to 'just now' by default", () => {
    const future = new Date(NOW.getTime() + 5 * 60_000);
    expect(formatRelative(future)).toBe("just now");
  });

  it("renders future dates with 'in X' when allowFuture is set", () => {
    const future = new Date(NOW.getTime() + 5 * 60_000);
    expect(formatRelative(future, { allowFuture: true })).toBe("in 5m");
  });

  it("accepts numeric milliseconds and ISO strings", () => {
    const ts = NOW.getTime() - 3 * 60_000;
    expect(formatRelative(ts)).toBe("3m ago");
    expect(formatRelative(new Date(ts).toISOString())).toBe("3m ago");
  });
});

describe("formatAbsolute", () => {
  it("returns em-dash for null / invalid", () => {
    expect(formatAbsolute(null)).toBe("—");
    expect(formatAbsolute("nope")).toBe("—");
  });

  it("returns toLocaleString for a valid date", () => {
    expect(formatAbsolute(NOW)).toBe(NOW.toLocaleString());
  });

  it("returns a short-form date when dateOnly is set", () => {
    expect(formatAbsolute(NOW, { dateOnly: true })).toBe(
      NOW.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }),
    );
  });
});

describe("formatDuration", () => {
  it("returns '0s' for sub-second / negative / non-finite values", () => {
    expect(formatDuration(500)).toBe("0s");
    expect(formatDuration(-1000)).toBe("0s");
    expect(formatDuration(Number.NaN)).toBe("0s");
  });

  it("formats seconds / minutes / hours / days", () => {
    expect(formatDuration(5_000)).toBe("5s");
    expect(formatDuration(90_000)).toBe("1m");
    expect(formatDuration(2 * 3_600_000)).toBe("2h");
    expect(formatDuration(3 * 86_400_000)).toBe("3d");
  });

  it("applies an optional prefix", () => {
    expect(formatDuration(5 * 60_000, { prefix: "Idle " })).toBe("Idle 5m");
  });
});
