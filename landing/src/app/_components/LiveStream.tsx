"use client";

import { useEffect, useRef, useState } from "react";
import { SporeLogo } from "./SporeLogo";

/**
 * The animated Live panel — the one genuinely moving surface in the hero
 * (owner decision #1 + #4: the motion budget lives here, not in the
 * background). It mimics the real /live view: a working agent whose tool
 * calls stream in at the top, one row at a time, with a pulsing status dot
 * and an occasional tool call that expands to reveal its output.
 *
 * Cheap by construction: no canvas, no video. A single setInterval prepends
 * rows to a capped list; CSS keyframes handle the entrance and the pulse, so
 * the GPU only ever composites a handful of small elements. The loop pauses
 * when the panel scrolls off-screen and never starts under reduced-motion —
 * in that case a static, fully-populated feed is shown instead.
 */

type Accent = "amber" | "green" | "blue" | "lav";

type Event = {
  tool: string;
  accent: Accent;
  text: string;
  dur: string;
  detail?: string[];
  flag?: boolean;
};

// A rotating script of plausible agent activity. Order matters — it reads as
// one coherent working session looping quietly.
const SCRIPT: Event[] = [
  { tool: "playwright", accent: "amber", text: "browser_evaluate", dur: "0.8s" },
  {
    tool: "Bash",
    accent: "green",
    text: "go test ./pkg/agent/...",
    dur: "2.1s",
    detail: ["ok  github.com/rpuneet/mycel/pkg/agent  2.104s"],
  },
  { tool: "Read", accent: "blue", text: "internal/cmd/agent.go", dur: "0.1s" },
  { tool: "Edit", accent: "amber", text: "server/hub.go  +12 −4", dur: "0.3s" },
  { tool: "playwright", accent: "amber", text: "browser_wait_for", dur: "1.2s" },
  {
    tool: "Bash",
    accent: "green",
    text: 'git commit -m "fix(hub): drain on close"',
    dur: "0.4s",
  },
  { tool: "Read", accent: "blue", text: "pkg/notify/dispatch.go", dur: "0.1s" },
  { tool: "MCP", accent: "lav", text: "slack.postMessage", dur: "0.6s" },
];

// Enough rows to fill the 16:10 frame so the feed doesn't leave dead space.
const MAX_ROWS = 13;
const TICK_MS = 1700;

const accentClass: Record<Accent, string> = {
  amber: "live-chip-amber",
  green: "live-chip-green",
  blue: "live-chip-blue",
  lav: "live-chip-lav",
};

type Row = Event & { key: number };

function seedRows(): Row[] {
  // Start already-populated so the panel is never empty on first paint.
  return Array.from({ length: MAX_ROWS }, (_, i) => ({
    ...SCRIPT[i % SCRIPT.length],
    key: i,
  }));
}

export function LiveStream() {
  const [rows, setRows] = useState<Row[]>(seedRows);
  const containerRef = useRef<HTMLDivElement>(null);
  const cursor = useRef(MAX_ROWS);
  const keyId = useRef(MAX_ROWS);

  useEffect(() => {
    const reduced = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    if (reduced) return; // static feed — no loop

    const el = containerRef.current;
    let timer: ReturnType<typeof setInterval> | null = null;
    let onScreen = true;

    const start = () => {
      if (timer) return;
      timer = setInterval(() => {
        const next = SCRIPT[cursor.current % SCRIPT.length];
        cursor.current += 1;
        keyId.current += 1;
        setRows((prev) =>
          [{ ...next, key: keyId.current }, ...prev].slice(0, MAX_ROWS),
        );
      }, TICK_MS);
    };
    const stop = () => {
      if (timer) {
        clearInterval(timer);
        timer = null;
      }
    };

    // Pause when the panel is off-screen; also pause with the tab.
    const io =
      el && typeof IntersectionObserver !== "undefined"
        ? new IntersectionObserver(
            (entries) => {
              onScreen = entries.some((e) => e.isIntersecting);
              if (onScreen && !document.hidden) start();
              else stop();
            },
            { threshold: 0.15 },
          )
        : null;
    if (io && el) io.observe(el);
    else start();

    const onVisibility = () => {
      if (!document.hidden && onScreen) start();
      else stop();
    };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      stop();
      io?.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  return (
    <div
      ref={containerRef}
      className="flex h-full flex-col bg-surface-container-lowest"
    >
      {/* Agent header — the working character */}
      <div className="flex items-center gap-2.5 border-b border-outline-variant/15 px-4 py-3">
        <span className="grid h-7 w-7 place-items-center rounded-md bg-primary/12">
          <SporeLogo size={18} />
        </span>
        <span className="font-headline text-sm font-semibold text-on-background">
          zen-zebra
        </span>
        <span className="inline-flex items-center gap-1.5 rounded-full bg-success/15 px-2 py-0.5">
          <span className="live-pulse h-1.5 w-1.5 rounded-full bg-success" />
          <span className="font-label text-[10px] uppercase tracking-wider text-success">
            Working
          </span>
        </span>
        <span className="ml-auto font-label text-[10px] text-on-surface-variant">
          streaming · live
        </span>
      </div>

      {/* Event stream — newest at the top */}
      <div className="flex-1 space-y-0.5 overflow-hidden px-2 py-2 sm:space-y-1">
        {rows.map((r) => (
          <div key={r.key} className="live-row px-2 py-2">
            <div className="flex items-center gap-2.5">
              {r.flag ? (
                <span className="font-label text-xs text-primary">◆</span>
              ) : (
                <span
                  className={`live-chip ${accentClass[r.accent]} font-label`}
                >
                  {r.tool}
                </span>
              )}
              <code className="min-w-0 flex-1 truncate font-label text-[12px] text-on-surface">
                {r.text}
              </code>
              <span className="shrink-0 font-label text-[10px] tabular-nums text-on-surface-variant">
                {r.dur}
              </span>
            </div>
            {r.detail && (
              <div className="live-detail mt-1.5 ml-[3.25rem] rounded border border-outline-variant/15 bg-surface-container/60 px-2.5 py-1.5">
                {r.detail.map((line, i) => (
                  <div
                    key={i}
                    className="font-label text-[11px] leading-relaxed text-terminal-success"
                  >
                    {line}
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
