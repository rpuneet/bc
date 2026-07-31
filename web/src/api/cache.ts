// A tiny module-level SWR-ish cache for hot, shared GETs.
//
// Two problems it solves:
//  1. Refetch storms — several components mount at once (nav drawer, page,
//     command palette) and each fires the same GET (e.g. /api/agents). The
//     in-flight map collapses concurrent calls for the same key into a single
//     network request; every caller awaits the same promise.
//  2. Redundant navigation refetches — moving between pages that share data
//     re-requests it. A short TTL (default 5s) serves the last value without a
//     round-trip while keeping the UI fresh enough.
//
// SSE / mutation code invalidates a key when the underlying data changes, so
// the cache never masks a real update. No external dependency — this is a
// hand-rolled ~40-line primitive, deliberately not a full query library.

interface Entry<T> {
  value: T;
  ts: number;
}

const DEFAULT_TTL_MS = 5000;

const store = new Map<string, Entry<unknown>>();
const inflight = new Map<string, Promise<unknown>>();

/**
 * cachedGet returns a cached value when it is younger than ttlMs, joins an
 * in-flight request for the same key when one exists, and otherwise starts a
 * fresh fetch. The fetcher runs at most once per key while pending.
 */
export function cachedGet<T>(
  key: string,
  fetcher: () => Promise<T>,
  ttlMs: number = DEFAULT_TTL_MS,
): Promise<T> {
  const hit = store.get(key) as Entry<T> | undefined;
  if (hit && Date.now() - hit.ts < ttlMs) {
    return Promise.resolve(hit.value);
  }
  const pending = inflight.get(key) as Promise<T> | undefined;
  if (pending) return pending;

  const p = fetcher()
    .then((value) => {
      store.set(key, { value, ts: Date.now() });
      return value;
    })
    .finally(() => {
      inflight.delete(key);
    });
  inflight.set(key, p);
  return p;
}

/** Drop a cached key (and any in-flight promise) so the next read refetches. */
export function invalidate(key: string): void {
  store.delete(key);
  inflight.delete(key);
}

/** Drop every key sharing a prefix — e.g. invalidatePrefix("costs"). */
export function invalidatePrefix(prefix: string): void {
  for (const k of [...store.keys()]) {
    if (k.startsWith(prefix)) store.delete(k);
  }
  for (const k of [...inflight.keys()]) {
    if (k.startsWith(prefix)) inflight.delete(k);
  }
}

/** Synchronously read the last cached value without triggering a fetch. */
export function peek<T>(key: string): T | undefined {
  return (store.get(key) as Entry<T> | undefined)?.value;
}

/** Test-only: reset all cache state. */
export function __resetCache(): void {
  store.clear();
  inflight.clear();
}
