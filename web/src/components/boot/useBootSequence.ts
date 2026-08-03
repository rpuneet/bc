import { useCallback, useEffect, useRef, useState } from "react";
import { api, type DoctorReport, type HealthReport } from "../../api/client";
import { deriveReadiness } from "../../views/readiness/readiness";

/* ── Boot stream model ────────────────────────────────────────────────
 *
 * The boot console shows REAL daemon state, never invented log lines:
 *   1. a "connecting" line while /api/health is still refused;
 *   2. "daemon online" the moment the health probe first answers;
 *   3. one line per readiness check derived from /api/doctor + /api/health
 *      (runtime, git, each agent CLI) — the same model the Readiness page
 *      renders, so every status/detail is a live machine fact;
 *   4. any recent daemon event-log entries (/api/logs) appended verbatim.
 *
 * The daemon has no dedicated startup-log SSE stream, so the readiness
 * probe results ARE the boot content — which is the honest signal a boot
 * splash exists to convey ("is the machine ready to run agents yet?").
 */

export type BootStatus = "ok" | "warn" | "fail" | "info";

export interface BootLine {
  id: number;
  status: BootStatus;
  label: string;
  detail?: string;
}

export interface BootSequence {
  /** True once /api/health has answered — the app can be handed off. */
  healthy: boolean;
  /** Newest-last stream of real readiness/log lines. */
  lines: BootLine[];
  /**
   * True once health probes have failed for `stallMs` without a single
   * success. Lets the splash escape the "connecting…" spinner instead of
   * retrying silently forever when the daemon never comes up.
   */
  stalled: boolean;
  /** Reset the stall flag and restart the probe loop immediately. */
  retry: () => void;
}

export interface BootTimings {
  /** Delay between health probes while the daemon is still booting. */
  pollMs: number;
  /** Stagger between appended readiness lines, for the streaming cascade. */
  paceMs: number;
  /**
   * Time with no successful health probe before surfacing a stall state.
   * Optional so existing partial timing objects (e.g. in tests) keep
   * compiling; defaults to 9s when omitted.
   */
  stallMs?: number;
}

export const DEFAULT_BOOT_TIMINGS: BootTimings = { pollMs: 500, paceMs: 130, stallMs: 9000 };

function rStatusToBoot(s: "ok" | "warn" | "fail"): BootStatus {
  return s;
}

/**
 * useBootSequence — polls the daemon to readiness and streams real status
 * lines. Returns `healthy` (flips true on the first successful health
 * probe) plus the growing `lines` array. Self-contained and idempotent:
 * every probe/timer is torn down on unmount.
 */
export function useBootSequence(
  timings: BootTimings = DEFAULT_BOOT_TIMINGS,
): BootSequence {
  const [healthy, setHealthy] = useState(false);
  const [lines, setLines] = useState<BootLine[]>([]);
  const [stalled, setStalled] = useState(false);
  // Bumped by retry() to restart the probe effect below.
  const [retryNonce, setRetryNonce] = useState(0);
  const idRef = useRef(0);
  // Dedupes the opening banner across StrictMode's double-invoke, which would
  // otherwise say "connecting" twice. The ready guard is deliberately NOT a ref
  // — see the note on `handled` inside the effect.
  const startedWaiting = useRef(false);

  const retry = useCallback(() => {
    startedWaiting.current = false;
    setStalled(false);
    setRetryNonce((n) => n + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    /**
     * "This effect instance has already handled readiness." Scoped to the
     * effect, not held in a ref, because a ref outlives the instance that set
     * it and that wedged the splash for good in development:
     *
     * StrictMode mounts, probes, marks handled, then immediately tears the
     * instance down — so the probe's own `cancelled` check returned before it
     * could set `healthy`. The re-mounted instance then saw the ref already
     * marked, skipped probing entirely, and nothing was left to flip `healthy`
     * or to trip the stall timer. The splash sat on "connecting to the mycel
     * daemon" forever against a daemon that was answering every request.
     */
    let handled = false;
    const timers: ReturnType<typeof setTimeout>[] = [];

    const push = (line: Omit<BootLine, "id">) => {
      if (cancelled) return;
      setLines((prev) => [...prev, { id: idRef.current++, ...line }]);
    };

    // A paced cascade so a fast, already-up daemon still reads as a
    // sequence rather than a single flash of every line at once.
    const pushPaced = (items: Omit<BootLine, "id">[], base: number) => {
      items.forEach((line, i) => {
        timers.push(setTimeout(() => push(line), base + i * timings.paceMs));
      });
    };

    if (!startedWaiting.current) {
      startedWaiting.current = true;
      push({ status: "info", label: "connecting to the mycel daemon" });
    }

    // Surface a fallback state after a long stretch of failed probes —
    // otherwise a daemon that never comes up leaves the splash spinning
    // silently forever with no escape.
    const stallMs = timings.stallMs ?? 9000;
    const stallTimer = setTimeout(() => {
      if (!cancelled && !handled) setStalled(true);
    }, stallMs);
    timers.push(stallTimer);

    async function onReady() {
      if (cancelled || handled) return;
      handled = true;
      setHealthy(true);
      setStalled(false);
      push({ status: "ok", label: "daemon online", detail: "http responding" });

      // Derive the real readiness model and stream it line by line.
      const [report, health] = await Promise.all([
        api.getDoctor().catch(() => null) as Promise<DoctorReport | null>,
        api.getHealth().catch(() => null) as Promise<HealthReport | null>,
      ]);
      if (cancelled) return;

      const readinessLines: Omit<BootLine, "id">[] = [];
      if (report) {
        const r = deriveReadiness(report, health);
        for (const group of r.groups) {
          for (const item of group.items) {
            readinessLines.push({
              status: rStatusToBoot(item.status),
              label: item.label,
              detail: item.detail,
            });
          }
        }
      }
      readinessLines.push({
        status: health?.status === "degraded" ? "warn" : "ok",
        label: "readiness",
        detail:
          health?.status === "degraded"
            ? Object.values(health.degraded ?? {})[0] ?? "running degraded"
            : "all systems go",
      });
      pushPaced(readinessLines, timings.paceMs);

      // Append any real recent daemon events verbatim, after the checks.
      const logs = await api.getLogs(8).catch(() => []);
      if (cancelled || logs.length === 0) return;
      pushPaced(
        logs.map((l) => ({
          status: "info" as BootStatus,
          label: l.type || "event",
          detail: l.message,
        })),
        readinessLines.length * timings.paceMs + timings.paceMs,
      );
    }

    async function poll() {
      if (cancelled || handled) return;
      try {
        await api.getHealth();
        await onReady();
      } catch {
        // Daemon not up yet — probe again shortly.
        if (!cancelled) timers.push(setTimeout(() => void poll(), timings.pollMs));
      }
    }

    void poll();

    return () => {
      cancelled = true;
      for (const t of timers) clearTimeout(t);
    };
    // Depend on the primitive timing fields (not the `timings` object,
    // which callers may recreate each render) so a genuine change to
    // polling/pacing/stall restarts the probe with the new values.
    // retryNonce is the one deliberate manual re-run trigger, bumped by
    // retry().
  }, [retryNonce, timings.pollMs, timings.paceMs, timings.stallMs]);

  return { healthy, lines, stalled, retry };
}
