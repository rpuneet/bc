"use client";

import { useState } from "react";
import { LiveStream } from "./LiveStream";
import { MotionShot } from "./MotionShot";

/**
 * The tabbed product hero (owner decision #1): the page centerpiece. One
 * ProductFrame whose panel switches in place between the app's four core
 * views — Agents, Live, Code, Insights — collapsing what used to be six
 * stacked static screenshots into a single interactive frame.
 *
 * Three tabs show fresh, real captures of the running app (theme-swapped
 * dark/light via the .shot-* classes). The Live tab is genuinely animated —
 * a lightweight streaming mock (LiveStream) — so the hero moves without a
 * heavy video. Tab changes crossfade; the frame keeps a fixed 16:10 body so
 * nothing reflows on switch.
 *
 * Mobile: screenshots would be illegible if shrunk to 390px, so on small
 * screens the panel becomes horizontally pannable at a readable min width
 * (a "crop to the active panel" you can swipe). The Live panel is real text
 * and stays fully responsive.
 */

type TabId = "agents" | "live" | "code" | "insights";

type Tab = {
  id: TabId;
  label: string;
  title: string;
  caption: string;
  shot?: { dark: string; light: string; alt: string };
  /** public/motion/<motion>-{dark,light}.{webm,mp4}: a live recording of
   * this view, played in place of the static screenshot. */
  motion?: { name: string; alt: string };
};

const TABS: Tab[] = [
  {
    id: "agents",
    label: "Agents",
    title: "mycel — agents",
    caption:
      "Meet the fleet — every agent has a face, a status, and a running cost.",
    motion: {
      name: "agents",
      alt: "The mycel agents view, live: a fleet of agents with breathing, blinking character faces, grouped by repository, each with status, last activity, and cost",
    },
  },
  {
    id: "live",
    label: "Live",
    title: "mycel — live",
    caption:
      "Every action the moment it happens — commands, reads, and tool calls, streaming in.",
  },
  {
    id: "code",
    label: "Code",
    title: "mycel — code",
    caption:
      "Every change, reviewable before it ships — a real side-by-side diff.",
    shot: {
      dark: "/screenshots/code-dark.png",
      light: "/screenshots/code-light.png",
      alt: "An agent's code view showing a side-by-side diff of a changed file with the full working-copy file tree",
    },
  },
  {
    id: "insights",
    label: "Insights",
    title: "mycel — insights",
    caption:
      "Spend read straight from the source — by agent, by model, over time.",
    motion: {
      name: "insights",
      alt: "The insights view, live: total spend, tokens, and cache hit rate recomputing as the period switches between 7, 30, and 90 days",
    },
  },
];

export function HeroShowcase() {
  const [active, setActive] = useState<TabId>("agents");
  const tab = TABS.find((t) => t.id === active) ?? TABS[0];

  return (
    <div className="mx-auto w-full max-w-6xl">
      {/* Tab bar */}
      <div
        role="tablist"
        aria-label="Product views"
        className="mb-4 flex items-center justify-center gap-1.5 sm:gap-2"
      >
        {TABS.map((t) => {
          const isActive = t.id === active;
          return (
            <button
              key={t.id}
              role="tab"
              id={`hero-tab-${t.id}`}
              aria-selected={isActive}
              aria-controls="hero-panel"
              onClick={() => setActive(t.id)}
              className={`inline-flex items-center gap-1.5 rounded-full px-3.5 py-1.5 font-label text-[13px] font-medium transition-all sm:px-4 ${
                isActive
                  ? "bg-primary/12 text-primary shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--primary)_30%,transparent)]"
                  : "text-on-surface-variant hover:bg-surface-container/60 hover:text-on-surface"
              }`}
            >
              {t.id === "live" && (
                <span
                  className={`h-1.5 w-1.5 rounded-full ${isActive ? "live-pulse bg-primary" : "bg-on-surface-variant/50"}`}
                  aria-hidden="true"
                />
              )}
              {t.label}
            </button>
          );
        })}
      </div>

      {/* Frame */}
      <div className="hero-stage">
        <div className="hero-tilt">
          <figure className="product-frame hero-glow">
            <div className="product-frame-bar" aria-hidden="true">
              <span className="product-frame-dot" />
              <span className="product-frame-dot" />
              <span className="product-frame-dot" />
              <span className="product-frame-title">{tab.title}</span>
            </div>

            {/* Fixed-ratio body; screenshots pan horizontally on mobile */}
            <div className="hero-panel-scroll overflow-x-auto sm:overflow-visible">
              <div
                key={active}
                id="hero-panel"
                role="tabpanel"
                aria-labelledby={`hero-tab-${active}`}
                className="hero-panel aspect-[16/10] min-w-[680px] sm:min-w-0"
              >
                {tab.motion ? (
                  <>
                    <MotionShot
                      name={tab.motion.name}
                      theme="dark"
                      alt={tab.motion.alt}
                      width={1440}
                      height={900}
                      priority={active === "agents"}
                      className="shot-dark h-full w-full object-cover object-top"
                    />
                    <MotionShot
                      name={tab.motion.name}
                      theme="light"
                      alt=""
                      ariaHidden
                      width={1440}
                      height={900}
                      className="shot-light h-full w-full object-cover object-top"
                    />
                  </>
                ) : tab.shot ? (
                  <>
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={tab.shot.dark}
                      alt={tab.shot.alt}
                      width={1440}
                      height={900}
                      loading={active === "agents" ? "eager" : "lazy"}
                      decoding="async"
                      className="shot-dark h-full w-full object-cover object-top"
                    />
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={tab.shot.light}
                      alt=""
                      aria-hidden="true"
                      width={1440}
                      height={900}
                      loading="lazy"
                      decoding="async"
                      className="shot-light h-full w-full object-cover object-top"
                    />
                  </>
                ) : (
                  <LiveStream />
                )}
              </div>
            </div>
          </figure>
        </div>
      </div>

      {/* Caption + mobile pan hint */}
      <div className="mt-5 text-center">
        <p className="font-body text-sm text-on-surface-variant">
          {tab.caption}
        </p>
        {(tab.shot || tab.motion) && (
          <p className="mt-1 font-label text-[11px] text-on-surface-variant/60 sm:hidden">
            swipe the panel to explore ›
          </p>
        )}
      </div>
    </div>
  );
}
