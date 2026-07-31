"use client";

import { Eye, GitBranch, HardDrive } from "lucide-react";
import { FadeUp, ScrollReveal } from "./Motion";

/**
 * Positioning beat (teardown #20): answers "why a team of agents, not one" and
 * how mycel differs from a single-agent tool — without naming or trashing
 * competitors. The category claim, stated plainly:
 *   "One agent is a helper. A team you can see is infrastructure."
 */
const POINTS = [
  {
    icon: GitBranch,
    title: "A team, not a helper",
    line: "One agent waits for your next prompt. A team runs in parallel — each in its own worktree, each on its own job — and keeps going while you step away.",
  },
  {
    icon: Eye,
    title: "Seen, not guessed",
    line: "A single chat window hides the work. mycel puts every action, diff, and dollar on screen — so a fleet stays legible instead of becoming a black box.",
  },
  {
    icon: HardDrive,
    title: "Yours, on your machine",
    line: "No cloud tenancy, no black box. mycel is open source and runs locally — the agents, the code, and every dollar of spend stay under your control.",
  },
];

export function WhyMycel() {
  return (
    <section id="why-mycel" className="scroll-mt-24 py-14 sm:py-20">
      <div className="mx-auto max-w-5xl px-4 sm:px-6">
        <FadeUp className="text-center">
          <span className="deck-eyebrow">Why mycel</span>
          <h2 className="mx-auto mt-4 max-w-3xl font-headline text-3xl font-semibold leading-[1.12] tracking-tight text-on-background md:text-5xl">
            One agent is a helper.
            <br className="hidden sm:block" />{" "}
            <span className="text-primary">
              A team you can see is infrastructure.
            </span>
          </h2>
          <p className="mx-auto mt-5 max-w-2xl font-body text-lg text-on-surface-variant">
            Plenty of tools give you a single agent in a chat box. mycel is built
            for the harder thing: running many of them on real repositories,
            without losing the plot.
          </p>
        </FadeUp>

        <div className="mt-12 grid gap-5 sm:grid-cols-3">
          {POINTS.map((p, i) => {
            const Icon = p.icon;
            return (
              <ScrollReveal key={p.title} delay={i * 0.08}>
                <div className="h-full rounded-lg border border-outline-variant/15 bg-surface-container/40 p-6 transition-colors hover:border-primary/25">
                  <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/12 text-primary-text">
                    <Icon className="h-[18px] w-[18px]" aria-hidden="true" />
                  </span>
                  <h3 className="mt-4 font-headline text-lg font-semibold tracking-tight text-on-background">
                    {p.title}
                  </h3>
                  <p className="mt-2 font-body text-[14px] leading-relaxed text-on-surface-variant">
                    {p.line}
                  </p>
                </div>
              </ScrollReveal>
            );
          })}
        </div>
      </div>
    </section>
  );
}
