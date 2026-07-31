import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  cachedGet,
  invalidate,
  invalidatePrefix,
  peek,
  __resetCache,
} from "../cache";

describe("cachedGet", () => {
  beforeEach(() => {
    __resetCache();
    vi.restoreAllMocks();
  });

  it("collapses concurrent calls for the same key into one fetch (kills refetch storms)", async () => {
    const fetcher = vi.fn().mockResolvedValue(["a"]);
    // Simulate drawer + page + palette all mounting at once.
    const [r1, r2, r3] = await Promise.all([
      cachedGet("agents", fetcher),
      cachedGet("agents", fetcher),
      cachedGet("agents", fetcher),
    ]);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(r1).toBe(r2);
    expect(r2).toBe(r3);
  });

  it("serves the cached value within the TTL without refetching", async () => {
    const fetcher = vi.fn().mockResolvedValue(42);
    const first = await cachedGet("k", fetcher, 1000);
    const second = await cachedGet("k", fetcher, 1000);
    expect(first).toBe(42);
    expect(second).toBe(42);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("refetches once the TTL expires", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce("v1")
      .mockResolvedValueOnce("v2");
    const now = vi.spyOn(Date, "now");

    now.mockReturnValue(1_000);
    expect(await cachedGet("k", fetcher, 100)).toBe("v1");

    // 50ms later — still fresh.
    now.mockReturnValue(1_050);
    expect(await cachedGet("k", fetcher, 100)).toBe("v1");
    expect(fetcher).toHaveBeenCalledTimes(1);

    // 200ms later — stale, refetch.
    now.mockReturnValue(1_200);
    expect(await cachedGet("k", fetcher, 100)).toBe("v2");
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("invalidate() forces the next read to refetch", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce("old")
      .mockResolvedValueOnce("new");
    expect(await cachedGet("agents", fetcher, 10_000)).toBe("old");
    invalidate("agents");
    expect(await cachedGet("agents", fetcher, 10_000)).toBe("new");
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("invalidatePrefix() clears every key sharing the prefix", async () => {
    await cachedGet("costs:day", () => Promise.resolve(1), 10_000);
    await cachedGet("costs:week", () => Promise.resolve(2), 10_000);
    expect(peek("costs:day")).toBe(1);
    invalidatePrefix("costs");
    expect(peek("costs:day")).toBeUndefined();
    expect(peek("costs:week")).toBeUndefined();
  });

  it("does not cache a rejected fetch (next call retries)", async () => {
    const fetcher = vi
      .fn()
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce("ok");
    await expect(cachedGet("k", fetcher)).rejects.toThrow("boom");
    expect(await cachedGet("k", fetcher)).toBe("ok");
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});
