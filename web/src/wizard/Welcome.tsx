import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, type SettingsConfig } from "../api/client";
import { StepWelcome } from "./steps/StepWelcome";
import { StepSystem } from "./steps/StepSystem";
import { StepRuntime } from "./steps/StepRuntime";
import { StepProviders } from "./steps/StepProviders";
import { StepPreferences } from "./steps/StepPreferences";
import { StepApps } from "./steps/StepApps";
import { StepAgent } from "./steps/StepAgent";
import { StepDone } from "./steps/StepDone";
import { WIZARD_STEPS, stepIndex, type StepId, type StepProps, type WizardDraft, type WizardNav } from "./types";

/* ── First-run setup wizard ───────────────────────────────────────────
 *
 * A full-screen, skippable, resumable onboarding flow. Its signature is the
 * mycelial filament rail on the left: steps are spores threaded on a growing
 * hypha — the thread fills as you progress, echoing mycel's namesake network.
 * Everything else stays quiet and native to the app's design system.
 *
 * The wizard only ever writes config. It never touches agents, secrets it
 * wasn't given, apps, or the database — the underlying flows it reuses
 * (Apps connect, New Agent) own those, unchanged.
 */

const STEP_COMPONENTS: Record<StepId, (p: StepProps) => JSX.Element> = {
  welcome: StepWelcome,
  system: StepSystem,
  runtime: StepRuntime,
  providers: StepProviders,
  preferences: StepPreferences,
  apps: StepApps,
  agent: StepAgent,
  done: StepDone,
};

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/** The mycelial filament rail — the wizard's signature element. */
function FilamentRail({
  current,
  maxReached,
  onJump,
}: {
  current: number;
  maxReached: number;
  onJump: (i: number) => void;
}) {
  return (
    <nav aria-label="Setup progress" className="hidden md:flex flex-col relative pl-1">
      {WIZARD_STEPS.map((step, i) => {
        const done = i < current;
        const active = i === current;
        const reachable = i <= maxReached;
        return (
          <div key={step.id} className="relative flex items-start gap-3 min-h-[3.25rem]">
            {/* filament connector to the next node */}
            {i < WIZARD_STEPS.length - 1 && (
              <span
                aria-hidden
                className={`absolute left-[6.5px] top-4 w-[1.5px] h-[calc(100%-0.5rem)] transition-colors ${
                  i < current ? "bg-mycel-accent" : "bg-mycel-border"
                }`}
              />
            )}
            <button
              type="button"
              disabled={!reachable}
              onClick={() => reachable && onJump(i)}
              aria-current={active ? "step" : undefined}
              className={`relative z-10 mt-0.5 grid place-items-center w-3.5 h-3.5 rounded-full shrink-0 ${reachable ? "cursor-pointer" : "cursor-default"}`}
            >
              {active && (
                <span className="absolute -inset-1 rounded-full bg-mycel-accent-subtle motion-reduce:hidden" />
              )}
              {active && (
                <span className="absolute inline-flex w-full h-full rounded-full bg-mycel-accent opacity-40 animate-ping [animation-duration:2.5s] motion-reduce:hidden" />
              )}
              <span
                className={`relative grid place-items-center w-3.5 h-3.5 rounded-full border-2 transition-all ${
                  done
                    ? "bg-mycel-accent border-mycel-accent"
                    : active
                      ? "bg-mycel-bg border-mycel-accent scale-110"
                      : "bg-mycel-bg border-mycel-border"
                }`}
              >
                {done && (
                  <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="var(--mycel-accent-fg)" strokeWidth={4} strokeLinecap="round" strokeLinejoin="round" aria-hidden><path d="M20 6L9 17l-5-5" /></svg>
                )}
                {active && <span className="w-1 h-1 rounded-full bg-mycel-accent" />}
              </span>
            </button>
            <button
              type="button"
              disabled={!reachable}
              onClick={() => reachable && onJump(i)}
              className={`text-left -mt-0.5 ${reachable ? "cursor-pointer" : "cursor-default"}`}
            >
              <div className={`font-mono text-[10px] tracking-wider tabular-nums ${active ? "text-mycel-accent" : "text-mycel-muted"}`}>
                {pad(i + 1)}
              </div>
              <div className={`text-[12px] leading-tight transition-colors ${active ? "text-mycel-text font-semibold" : done ? "text-mycel-text-2" : "text-mycel-muted"}`}>
                {step.eyebrow}
              </div>
            </button>
          </div>
        );
      })}
    </nav>
  );
}

export function Welcome() {
  const navigate = useNavigate();
  const [idx, setIdx] = useState(0);
  const [maxReached, setMaxReached] = useState(0);
  const [ready, setReady] = useState(false);
  const [settings, setSettings] = useState<SettingsConfig | null>(null);
  const [draft, setDraftState] = useState<WizardDraft>({
    provider: "claude",
    model: "",
    runtime: "docker",
    name: "",
  });

  const reloadSettings = useCallback(async () => {
    try {
      setSettings(await api.getSettings());
    } catch {
      /* wizard still works without it */
    }
  }, []);

  // Resume where the user left off + prefill from prefs.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [state, cfg] = await Promise.all([
        api.getOnboardingState().catch(() => null),
        api.getSettings().catch(() => null),
      ]);
      if (cancelled) return;
      if (cfg) {
        setSettings(cfg);
        setDraftState((d) => ({
          ...d,
          name: cfg.user?.name ?? d.name,
          provider: cfg.providers?.default ?? d.provider,
          runtime: (cfg.runtime?.default as WizardDraft["runtime"]) ?? d.runtime,
        }));
      }
      if (state && state.step && state.step !== "done") {
        const resumeAt = stepIndex(state.step as StepId);
        if (resumeAt > 0) {
          setIdx(resumeAt);
          setMaxReached(resumeAt);
        }
      }
      setReady(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const persist = useCallback((stepId: StepId, reached: number, done: boolean) => {
    const completed = WIZARD_STEPS.slice(0, reached).map((s) => s.id) as string[];
    if (done) completed.push("done");
    void api.saveOnboarding(stepId, completed).catch(() => {
      /* progress is best-effort */
    });
  }, []);

  const goTo = useCallback(
    (i: number) => {
      const clamped = Math.max(0, Math.min(WIZARD_STEPS.length - 1, i));
      setIdx(clamped);
      const reached = Math.max(maxReached, clamped);
      setMaxReached(reached);
      persist(WIZARD_STEPS[clamped]!.id, reached, false);
      if (typeof window !== "undefined") window.scrollTo({ top: 0 });
    },
    [maxReached, persist],
  );

  const setDraft = useCallback((patch: Partial<WizardDraft>) => {
    setDraftState((d) => ({ ...d, ...patch }));
  }, []);

  const nav: WizardNav = useMemo(
    () => ({
      next: () => goTo(idx + 1),
      back: () => goTo(idx - 1),
      skip: () => goTo(idx + 1),
      goTo: (id) => goTo(stepIndex(id)),
      finish: () => {
        persist("done", WIZARD_STEPS.length, true);
        navigate("/");
      },
      exit: () => {
        persist(WIZARD_STEPS[idx]!.id, maxReached, false);
        navigate("/");
      },
      isFirst: idx === 0,
      isLast: idx === WIZARD_STEPS.length - 1,
    }),
    [idx, maxReached, goTo, persist, navigate],
  );

  const step = WIZARD_STEPS[idx]!;
  const StepComponent = STEP_COMPONENTS[step.id];

  return (
    <div className="fixed inset-0 z-40 overflow-y-auto bg-mycel-bg text-mycel-text">
      <div className="min-h-full flex flex-col">
        <header className="flex items-center justify-between px-5 sm:px-8 py-4 border-b border-mycel-border">
          <div className="flex items-center gap-2">
            <span className="grid place-items-center w-5 h-5 rounded-full bg-mycel-accent-subtle" aria-hidden>
              <span className="w-2 h-2 rounded-full bg-mycel-accent" />
            </span>
            <span className="font-display text-[18px] text-mycel-text">mycel</span>
            <span className="text-[11px] text-mycel-muted font-mono">setup</span>
          </div>
          <button
            type="button"
            onClick={() => nav.exit()}
            className="text-[12px] text-mycel-muted hover:text-mycel-text cursor-pointer transition-colors"
          >
            Skip setup →
          </button>
        </header>

        {/* Mobile progress bar (rail is desktop-only). */}
        <div className="md:hidden h-0.5 bg-mycel-border">
          <div
            className="h-full bg-mycel-accent transition-all"
            style={{ width: `${((idx + 1) / WIZARD_STEPS.length) * 100}%` }}
          />
        </div>

        <div className="flex-1 w-full max-w-4xl mx-auto px-5 sm:px-8 py-8 grid md:grid-cols-[10rem_1fr] gap-8 md:gap-12">
          <div className="md:pt-2">
            <FilamentRail current={idx} maxReached={maxReached} onJump={goTo} />
          </div>

          <main className="min-w-0 max-w-2xl">
            <div key={step.id} className="animate-reveal">
              <div className="font-mono text-[11px] tracking-widest text-mycel-accent uppercase tabular-nums">
                Step {pad(idx + 1)} / {pad(WIZARD_STEPS.length)} · {step.eyebrow}
              </div>
              <h1 className="mt-2 font-display text-[28px] sm:text-[34px] text-mycel-text leading-[1.1]">
                {step.title}
              </h1>
              <div className="mt-6">
                {ready && (
                  <StepComponent nav={nav} draft={draft} setDraft={setDraft} settings={settings} reloadSettings={reloadSettings} />
                )}
                {!ready && <div className="text-sm text-mycel-muted py-8">Loading setup…</div>}
              </div>
            </div>
          </main>
        </div>
      </div>
    </div>
  );
}
