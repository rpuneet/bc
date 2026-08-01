import { useEffect, useRef, useState } from "react";
import { prefersReducedMotion } from "../agent-ui/useAgentPulse";
import { BootMark } from "./BootMark";
import {
  useBootSequence,
  type BootTimings,
  DEFAULT_BOOT_TIMINGS,
} from "./useBootSequence";
import "./boot.css";

/**
 * BootSplash — the full-screen branded boot sequence.
 *
 * A four-phase machine:
 *   draw   — the mushroom mark draws itself in, centred and full-screen;
 *   stream — real server-readiness lines stream in below it (see
 *            {@link useBootSequence}); held for a minimum so the console is
 *            legible even when the daemon is already up;
 *   rise   — once the daemon is healthy AND the minimum is met, the mark
 *            travels up into the header brand position (shared-element);
 *   fade   — the splash fades out and {@link onReady} hands off to the app.
 *
 * `prefers-reduced-motion` collapses the drawing/travel to plain opacity
 * (via boot.css) and zeroes the motion timers here, so the sequence still
 * runs — just without animation.
 */

export interface SplashTimings {
  /** How long the mark draws in before the console appears. */
  drawMs: number;
  /** Minimum time the readiness console stays up before the rise. */
  minStreamMs: number;
  /** Duration of the rise-into-header travel. */
  riseMs: number;
  /** Fade-out before hand-off. */
  fadeMs: number;
  boot: BootTimings;
}

export const DEFAULT_SPLASH_TIMINGS: SplashTimings = {
  drawMs: 850,
  minStreamMs: 650,
  riseMs: 720,
  fadeMs: 480,
  boot: DEFAULT_BOOT_TIMINGS,
};

type Phase = "draw" | "stream" | "rise" | "fade";

export function BootSplash({
  onReady,
  timings = DEFAULT_SPLASH_TIMINGS,
}: {
  onReady: () => void;
  timings?: SplashTimings;
}) {
  const reduce = prefersReducedMotion();
  const drawMs = reduce ? 0 : timings.drawMs;
  const riseMs = reduce ? 0 : timings.riseMs;
  const fadeMs = reduce ? 60 : timings.fadeMs;

  const { healthy, lines, stalled, retry } = useBootSequence(timings.boot);
  const [phase, setPhase] = useState<Phase>("draw");
  const [minReached, setMinReached] = useState(false);
  const onReadyRef = useRef(onReady);
  onReadyRef.current = onReady;

  // draw → stream, and arm the minimum-stream gate.
  useEffect(() => {
    const toStream = setTimeout(() => setPhase("stream"), drawMs);
    const minGate = setTimeout(() => setMinReached(true), drawMs + timings.minStreamMs);
    return () => {
      clearTimeout(toStream);
      clearTimeout(minGate);
    };
  }, [drawMs, timings.minStreamMs]);

  // stream → rise, once the daemon is healthy and the minimum is met.
  // (Split from the rise-duration timer below: mutating `phase` here while
  // also holding the fade timer would clear that timer before it fired.)
  useEffect(() => {
    if (phase === "stream" && healthy && minReached) setPhase("rise");
  }, [phase, healthy, minReached]);

  // rise → fade after the travel completes.
  useEffect(() => {
    if (phase !== "rise") return;
    const toFade = setTimeout(() => setPhase("fade"), riseMs);
    return () => clearTimeout(toFade);
  }, [phase, riseMs]);

  // fade → hand off to the app.
  useEffect(() => {
    if (phase !== "fade") return;
    const done = setTimeout(() => onReadyRef.current(), fadeMs);
    return () => clearTimeout(done);
  }, [phase, fadeMs]);

  const rising = phase === "rise" || phase === "fade";
  const consoleHidden = rising;

  return (
    <div
      className="boot-splash"
      data-fading={phase === "fade"}
      role="status"
      aria-live="polite"
      aria-label="Starting mycel"
    >
      <div className="boot-mark" data-rising={rising}>
        <BootMark />
      </div>

      <div className="boot-wordmark font-display" data-hidden={consoleHidden}>
        mycel
      </div>

      <div className="boot-stream" data-hidden={consoleHidden} data-testid="boot-stream">
        {/* column-reverse: render newest first so it pins to the bottom
            and older lines scroll up out of the mask. */}
        {[...lines].reverse().map((line) => (
          <div className="boot-line" key={line.id}>
            <span className="boot-line-status" data-s={line.status} aria-hidden="true" />
            <span className="boot-line-label">{line.label}</span>
            {line.detail && <span className="boot-line-detail">{line.detail}</span>}
          </div>
        ))}
      </div>

      {/* Stall fallback — the daemon never answered a health probe within
          the stall window. Escape the silent spinner with a clear message
          plus a way forward, instead of retrying invisibly forever. */}
      {stalled && !healthy && (
        <div
          data-testid="boot-stall"
          style={{
            position: "fixed",
            top: "calc(50% + 96px)",
            left: "50%",
            transform: "translateX(-50%)",
            width: "min(420px, 88vw)",
            textAlign: "center",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: "0.75rem",
          }}
        >
          <p style={{ margin: 0, fontSize: 13, color: "var(--mycel-text-2)" }}>
            Still trying to reach the daemon&hellip; it may have failed to start.
          </p>
          <div style={{ display: "flex", gap: "0.5rem" }}>
            <button
              type="button"
              onClick={retry}
              style={{
                font: "inherit",
                fontSize: 12.5,
                fontWeight: 600,
                padding: "0.4rem 0.9rem",
                borderRadius: 6,
                border: "1px solid var(--mycel-border)",
                background: "var(--mycel-surface)",
                color: "var(--mycel-text)",
                cursor: "pointer",
              }}
            >
              Retry
            </button>
            <button
              type="button"
              onClick={() => onReadyRef.current()}
              style={{
                font: "inherit",
                fontSize: 12.5,
                padding: "0.4rem 0.9rem",
                borderRadius: 6,
                border: "1px solid transparent",
                background: "transparent",
                color: "var(--mycel-text-2)",
                cursor: "pointer",
                textDecoration: "underline",
              }}
            >
              Continue anyway
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
