"use client";

import Link from "next/link";
import { ArrowRight, Download, Play, UserPlus } from "lucide-react";
import { FadeUp, ScrollReveal } from "./Motion";

/**
 * "Getting started in 3 steps" bridge (teardown #7): a compact strip that
 * carries a visitor from the install area into the docs, closing the
 * landing→docs cliff. Each step deep-links into the relevant docs anchor.
 */
const STEPS = [
  {
    n: "01",
    icon: Download,
    title: "Install it",
    body: (
      <>
        One line for your platform — <code className="gs-code">curl</code>,{" "}
        <code className="gs-code">brew</code>, or the desktop app. No sign-up.
      </>
    ),
    href: "/docs#installation",
    cta: "Installation guide",
  },
  {
    n: "02",
    icon: Play,
    title: "Run mycel up",
    body: (
      <>
        <code className="gs-code">mycel up</code> starts everything and opens the
        app at <code className="gs-code">localhost:9374</code>.
      </>
    ),
    href: "/docs#your-first-agent",
    cta: "Quick start",
  },
  {
    n: "03",
    icon: UserPlus,
    title: "Hire your first agent",
    body: (
      <>
        Point it at a repo, pick a tool, give it a job. It gets a name, a face,
        and its own worktree.
      </>
    ),
    href: "/docs#tutorials/first-agent",
    cta: "First-agent tutorial",
  },
];

export function GettingStarted() {
  return (
    <section
      id="getting-started"
      className="scroll-mt-24 py-14 sm:py-16"
    >
      <div className="mx-auto max-w-5xl px-4 sm:px-6">
        <FadeUp>
          <span className="deck-eyebrow">Getting started</span>
          <h2 className="mt-4 font-headline text-3xl font-semibold tracking-tight text-on-background sm:text-4xl">
            Up and running in three steps.
          </h2>
          <p className="mt-4 max-w-2xl font-body text-on-surface-variant">
            From an empty terminal to your first working agent. Each step links
            straight into the docs.
          </p>
        </FadeUp>

        <div className="relative mt-10 grid gap-5 sm:grid-cols-3">
          {STEPS.map((s, i) => {
            const Icon = s.icon;
            return (
              <ScrollReveal key={s.n} delay={i * 0.08}>
                <Link
                  href={s.href}
                  className="group flex h-full flex-col rounded-lg border border-outline-variant/15 bg-surface-container/40 p-6 transition-colors hover:border-primary/30 hover:bg-surface-container/60"
                >
                  <div className="flex items-center gap-3">
                    <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/12 text-primary-text">
                      <Icon className="h-[18px] w-[18px]" aria-hidden="true" />
                    </span>
                    <span className="deck-index text-3xl">{s.n}</span>
                  </div>
                  <h3 className="mt-4 font-headline text-lg font-semibold tracking-tight text-on-background">
                    {s.title}
                  </h3>
                  <p className="mt-2 flex-1 font-body text-[14px] leading-relaxed text-on-surface-variant">
                    {s.body}
                  </p>
                  <span className="mt-4 inline-flex items-center gap-1.5 font-label text-[12px] font-semibold text-primary-text">
                    {s.cta}
                    <ArrowRight
                      className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5"
                      aria-hidden="true"
                    />
                  </span>
                </Link>
              </ScrollReveal>
            );
          })}
        </div>
      </div>
    </section>
  );
}
