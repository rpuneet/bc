/**
 * Shared drill-down primitives for the Insights page.
 *
 * - `useHashPanel` keeps "which panel is open" in the URL hash
 *   (`#sys=cpu&tokens=1&row=agent:zen-zebra`) so a refresh restores the
 *   expanded state without touching router state or history length.
 * - `Disclosure` is the inline expand/collapse container: grid-rows
 *   height animation (180ms ease), Esc to close, lazy children (content
 *   mounts on first open so drill-downs fetch nothing upfront).
 * - `Chevron` is the shared clickability affordance.
 */

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";

// ── Hash state ──────────────────────────────────────────────────────────────

function readHash(): URLSearchParams {
  return new URLSearchParams(window.location.hash.replace(/^#/, ""));
}

// The hash is written with history.replaceState, which fires no event —
// notify subscribers ourselves so multiple panels stay in sync.
const hashListeners = new Set<() => void>();

function writeHash(params: URLSearchParams): void {
  const s = params.toString();
  const url = window.location.pathname + window.location.search + (s ? `#${s}` : "");
  window.history.replaceState(window.history.state, "", url);
  hashListeners.forEach((l) => l());
}

function subscribeHash(cb: () => void): () => void {
  hashListeners.add(cb);
  window.addEventListener("hashchange", cb);
  return () => {
    hashListeners.delete(cb);
    window.removeEventListener("hashchange", cb);
  };
}

/**
 * One expanded-panel slot persisted in the URL hash under `key`.
 * Returns the current value (null = closed) and a setter; setting the
 * same value again closes (toggle semantics for tile/row clicks).
 */
export function useHashPanel(key: string): [string | null, (v: string | null) => void] {
  const value = useSyncExternalStore(
    subscribeHash,
    () => readHash().get(key),
    () => null,
  );

  const set = useCallback(
    (v: string | null) => {
      const params = readHash();
      const next = v !== null && params.get(key) === v ? null : v;
      if (next === null) params.delete(key);
      else params.set(key, next);
      writeHash(params);
    },
    [key],
  );

  return [value, set];
}

// ── Disclosure ──────────────────────────────────────────────────────────────

export function Disclosure({
  open,
  onClose,
  children,
  label,
}: {
  open: boolean;
  onClose: () => void;
  children: React.ReactNode;
  /** Accessible name for the expanded region. */
  label: string;
}) {
  // Children stay mounted through the collapse so the height animation
  // has content to shrink over; they unmount after the transition.
  const [mounted, setMounted] = useState(open);
  useEffect(() => {
    if (open) setMounted(true);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  return (
    <div
      role="region"
      aria-label={label}
      aria-hidden={!open}
      style={{
        display: "grid",
        gridTemplateRows: open ? "1fr" : "0fr",
        transition: "grid-template-rows 180ms ease",
      }}
      onTransitionEnd={() => {
        if (!open) setMounted(false);
      }}
    >
      <div className="min-h-0 overflow-hidden">{mounted ? children : null}</div>
    </div>
  );
}

// ── Chevron affordance ──────────────────────────────────────────────────────

export function Chevron({ open, className = "" }: { open: boolean; className?: string }) {
  return (
    <svg
      width="10"
      height="10"
      viewBox="0 0 10 10"
      aria-hidden
      className={`shrink-0 text-mycel-muted transition-transform duration-200 ease-out ${open ? "rotate-90" : ""} ${className}`}
    >
      <path d="M3.5 1.5 7 5 3.5 8.5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
