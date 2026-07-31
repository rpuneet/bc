"use client";

import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { FadeUp, ScrollReveal } from "./Motion";

/**
 * A small taste of the Method essay on the home page — the teardown rated the
 * essay the site's strongest asset, so we surface three of its six convictions
 * here and link through. Kept deliberately light: three lines, not the whole
 * manifesto.
 */
const CONVICTIONS = [
  {
    n: "01",
    title: "Isolation",
    line: "Shared state is the enemy of parallel work. Every agent gets its own git worktree.",
  },
  {
    n: "02",
    title: "Visibility",
    line: "What you cannot see, you cannot trust. Every action lands in the live feed.",
  },
  {
    n: "03",
    title: "Cost",
    line: "An unwatched agent is an unbounded bill. Spend is read straight from the source.",
  },
];

export function MethodTeaser() {
  return (
    <section className="scroll-mt-24 py-14 sm:py-20">
      <div className="mx-auto max-w-5xl px-4 sm:px-6">
        <FadeUp className="text-center">
          <span className="deck-eyebrow">The convictions</span>
          <h2 className="mx-auto mt-4 max-w-2xl font-headline text-3xl font-semibold tracking-tight text-on-background md:text-4xl">
            Not features. Convictions.
          </h2>
          <p className="mx-auto mt-4 max-w-xl font-body text-base text-on-surface-variant">
            Every view and command exists because it serves one of six beliefs
            about how a team of agents should work. Three of them:
          </p>
        </FadeUp>

        <div className="mt-10 grid gap-5 sm:grid-cols-3">
          {CONVICTIONS.map((c, i) => (
            <ScrollReveal key={c.n} delay={i * 0.08}>
              <div className="h-full rounded-lg border border-outline-variant/15 bg-surface-container/50 p-6 transition-colors hover:border-primary/25">
                <div className="flex items-baseline gap-3">
                  <span className="deck-index text-3xl">{c.n}</span>
                  <span className="font-label text-[11px] uppercase tracking-[0.2em] text-primary-text">
                    {c.title}
                  </span>
                </div>
                <p className="mt-4 font-body text-[15px] leading-relaxed text-on-surface-variant">
                  {c.line}
                </p>
              </div>
            </ScrollReveal>
          ))}
        </div>

        <FadeUp className="mt-9 text-center">
          <Link
            href="/method"
            className="group inline-flex items-center gap-1.5 font-body text-sm text-on-surface-variant transition-colors hover:text-primary"
          >
            Read the method — all six convictions
            <ArrowRight
              className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5"
              aria-hidden="true"
            />
          </Link>
        </FadeUp>
      </div>
    </section>
  );
}
