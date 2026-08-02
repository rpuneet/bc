/* ── useProgressiveReveal ──────────────────────────────────────────────
 *
 * Setup is Settings revealing itself — there is no separate /welcome
 * wizard. This hook derives, from the settings the page already loaded
 * plus a little runtime context (agent count, provider install state,
 * connected apps), which real Settings sections are locked / active /
 * complete, in the same order they already render in.
 *
 * Nothing here is "wizard state": every predicate reads real config or
 * real machine state. `replay()` only clears the persisted
 * onboarding.completed acknowledgement flags — it never blanks a field
 * the user actually set, so re-running setup can never lose work.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type SettingsConfig } from "../api/client";
import { useReadiness } from "../hooks/useReadiness";

/** Reveal order mirrors the Settings section order, so the guided reveal
 *  always walks top-down: the "active" card is never below a locked one.
 *  "advanced" is deliberately excluded — it is never gated. */
export const REVEAL_ORDER = ["profile", "providers & tools", "runtime", "apps", "budgets"] as const;
export type RevealSectionId = (typeof REVEAL_ORDER)[number];

export type RevealState = "locked" | "active" | "complete";

export interface ProgressiveReveal {
  /** False until every input (settings, agents, providers, apps) has
   *  loaded at least once. Sections render un-gated (as if complete)
   *  while loading, so the page never flashes locked cards on refresh. */
  loaded: boolean;
  states: Record<RevealSectionId, RevealState>;
  /** The first section that isn't complete, or null once everything is. */
  firstIncomplete: RevealSectionId | null;
  allComplete: boolean;
  /** True once at least one agent exists — an established install has,
   *  by definition, finished setup regardless of section state. */
  hasAgents: boolean;
  /** How many of REVEAL_ORDER's sections are currently complete. */
  completedCount: number;
  /** Acknowledge a skippable/no-required-field section (runtime, apps,
   *  budgets) so reveal can advance past it without inventing a fake
   *  required field. Persisted in onboarding.completed. */
  markTouched: (id: RevealSectionId) => Promise<void>;
  /** Replay the guided reveal: clears onboarding.completed (never touches
   *  any other config) and scrolls to top. */
  replay: () => Promise<void>;
}

function touched(config: SettingsConfig | null, id: RevealSectionId): boolean {
  const completed = config?.onboarding?.completed ?? [];
  return completed.includes(id);
}

/**
 * @param config Live settings snapshot, already loaded by the caller
 *   (Settings owns the fetch/poll; this hook never re-fetches it).
 * @param onRefresh Called after replay()/markTouched() persist, so the
 *   caller can re-pull onboarding-derived state (e.g. its own polling
 *   fetcher) instead of this hook keeping a second copy.
 */
export function useProgressiveReveal(
  config: SettingsConfig | null,
  onRefresh?: () => void,
): ProgressiveReveal {
  // hasAgents reuses the daemon's own computed signal (GET
  // /api/onboarding/state) rather than re-deriving it from a second
  // /api/agents fetch — one source of truth for "this install already
  // runs agents".
  const [hasAgents, setHasAgents] = useState<boolean | null>(null);
  const [appsConnected, setAppsConnected] = useState<boolean | null>(null);
  const { data: readiness, loaded: readinessLoaded } = useReadiness(true);

  useEffect(() => {
    let alive = true;
    api.getOnboardingState()
      .then((s) => { if (alive) setHasAgents(s.hasAgents); })
      .catch(() => { if (alive) setHasAgents(false); });
    api.getApps()
      .then((res) => { if (alive) setAppsConnected((res.instances ?? []).length > 0); })
      .catch(() => { if (alive) setAppsConnected(false); });
    return () => { alive = false; };
  }, []);

  const providerInstalled = readinessLoaded && !!readiness && Object.values(readiness.providers).some(Boolean);

  const loaded = config !== null && hasAgents !== null && appsConnected !== null && readinessLoaded;

  const complete: Record<RevealSectionId, boolean> = useMemo(() => {
    const userName = String(((config?.user ?? {}) as { name?: unknown }).name ?? "").trim();
    return {
      profile: userName !== "",
      runtime: touched(config, "runtime"),
      "providers & tools": providerInstalled,
      apps: (appsConnected ?? false) || touched(config, "apps"),
      // Cost caps are entirely optional — never blocks reveal.
      budgets: true,
    };
    // hasAgents doesn't gate any single section, but an install that
    // already runs agents has, by definition, finished setup once — see
    // allComplete below.
  }, [config, providerInstalled, appsConnected]);

  const allComplete = useMemo(
    () => !!hasAgents || REVEAL_ORDER.every((id) => complete[id]),
    [hasAgents, complete],
  );

  const states: Record<RevealSectionId, RevealState> = useMemo(() => {
    const result = {} as Record<RevealSectionId, RevealState>;
    // While inputs are still loading, never show locked — avoid a flash
    // of gated cards on every background refresh.
    if (!loaded) {
      for (const id of REVEAL_ORDER) result[id] = "complete";
      return result;
    }
    if (allComplete) {
      for (const id of REVEAL_ORDER) result[id] = "complete";
      return result;
    }
    // A section that is already satisfied is never locked, wherever it
    // sits in the order: locking it would hide real, reachable config
    // behind a padlock (e.g. providers & tools with a CLI already
    // installed, or budgets, which is optional and always complete).
    // Only *unsatisfied* sections after the first one are gated.
    let seenIncomplete = false;
    for (const id of REVEAL_ORDER) {
      if (complete[id]) {
        result[id] = "complete";
        continue;
      }
      result[id] = seenIncomplete ? "locked" : "active";
      seenIncomplete = true;
    }
    return result;
  }, [loaded, allComplete, complete]);

  const firstIncomplete = useMemo(
    () => REVEAL_ORDER.find((id) => states[id] === "active") ?? null,
    [states],
  );

  const completedCount = useMemo(
    () => REVEAL_ORDER.filter((id) => complete[id]).length,
    [complete],
  );

  const markTouched = useCallback(
    async (id: RevealSectionId) => {
      const prevCompleted = config?.onboarding?.completed ?? [];
      const nextCompleted = prevCompleted.includes(id) ? prevCompleted : [...prevCompleted, id];
      await api.saveOnboarding(id, nextCompleted).catch(() => { /* best-effort ack */ });
      onRefresh?.();
    },
    [config, onRefresh],
  );

  const replay = useCallback(async () => {
    await api.saveOnboarding("", []).catch(() => { /* best-effort */ });
    onRefresh?.();
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }, [onRefresh]);

  return { loaded, states, firstIncomplete, allComplete, hasAgents: !!hasAgents, completedCount, markTouched, replay };
}
