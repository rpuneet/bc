"use client";

import { type ReactNode } from "react";
import Link from "next/link";
import { ArrowRight, Github } from "lucide-react";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";
import { InstallSection } from "./_components/InstallSection";
import { ProductFrame } from "./_components/ProductFrame";
import { RevealSection, FadeUp, ScrollReveal } from "./_components/Motion";
import { AnimatedBackground } from "./_components/AnimatedBackground";
import { SporeLogo } from "./_components/SporeLogo";

/* ── Section divider with the mushroom mark as a fleuron ── */
function SporeDivider() {
  return (
    <div className="mx-auto max-w-5xl px-6">
      <div className="relative flex items-center justify-center">
        <div className="section-separator w-full" />
        <span className="absolute flex items-center gap-3 rounded-full bg-background px-4 py-1.5">
          <span
            aria-hidden="true"
            className="h-1 w-1 rounded-full bg-primary/60"
          />
          <SporeLogo size={28} className="opacity-90" />
          <span
            aria-hidden="true"
            className="h-1 w-1 rounded-full bg-primary/60"
          />
        </span>
      </div>
    </div>
  );
}

/* ── One deck panel: numbered principle → thesis → live artifact ── */
function DeckPanel({
  index,
  eyebrow,
  title,
  body,
  artifact,
  imageFirst = false,
  last = false,
}: {
  index: string;
  eyebrow: string;
  title: ReactNode;
  body: ReactNode;
  artifact: ReactNode;
  imageFirst?: boolean;
  last?: boolean;
}) {
  const text = (
    <ScrollReveal
      from={imageFirst ? "right" : "left"}
      distance={40}
      className={imageFirst ? "lg:order-2" : ""}
    >
      <div className="flex items-baseline gap-4">
        <span className="deck-index text-5xl sm:text-6xl">{index}</span>
        <span className="deck-eyebrow pb-1">{eyebrow}</span>
      </div>
      <h3 className="mt-6 font-headline text-3xl font-semibold leading-[1.12] tracking-tight text-on-background sm:text-4xl lg:text-[2.75rem]">
        {title}
      </h3>
      <p className="mt-5 max-w-xl font-body text-[15px] leading-[1.8] text-on-surface-variant">
        {body}
      </p>
    </ScrollReveal>
  );

  const art = (
    <ScrollReveal
      from="up"
      distance={32}
      delay={0.12}
      className={imageFirst ? "lg:order-1" : ""}
    >
      {artifact}
    </ScrollReveal>
  );

  return (
    <div className="relative">
      {/* Ember-rail node marker (desktop) */}
      <span
        aria-hidden="true"
        className="ember-node absolute left-1/2 top-16 hidden h-3 w-3 -translate-x-1/2 rounded-full bg-primary lg:block"
      />
      <div className="grid items-center gap-10 py-16 sm:py-20 lg:grid-cols-2 lg:gap-20">
        {imageFirst ? (
          <>
            {art}
            {text}
          </>
        ) : (
          <>
            {text}
            {art}
          </>
        )}
      </div>
      {!last && (
        <div
          aria-hidden="true"
          className="ember-rail absolute left-1/2 bottom-0 hidden h-16 w-px -translate-x-1/2 lg:block"
        />
      )}
    </div>
  );
}

export default function Home() {
  return (
    <main className="min-h-screen overflow-x-hidden">
      {/* Drifting spore field — fixed, covers the page */}
      <AnimatedBackground />
      {/* Warm radial wash above the fold */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,color-mix(in_srgb,var(--primary)_10%,transparent),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* ════════════════════════════════════════
           Hero — the thesis
           ════════════════════════════════════════ */}
        <section className="pt-28 pb-10 sm:pt-36 sm:pb-14">
          <div className="mx-auto max-w-4xl px-4 text-center sm:px-6">
            <FadeUp>
              <span className="deck-eyebrow">
                Open source &middot; Free &middot; Runs on your machine
              </span>
            </FadeUp>

            <FadeUp delay={0.1}>
              <h1 className="mt-6 font-headline text-4xl font-semibold leading-[1.08] tracking-tight text-on-background md:text-6xl lg:text-[4.25rem]">
                Your team of AI agents,
                <br className="hidden sm:block" />{" "}
                run from <span className="text-primary">one place.</span>
              </h1>
            </FadeUp>

            <FadeUp delay={0.15}>
              <p className="mx-auto mt-6 max-w-2xl font-body text-lg leading-relaxed text-on-surface-variant md:text-xl">
                mycel gives every agent a name, a face, and a job. They write
                code in your repositories, reach you on Slack or WhatsApp, and
                everything they do &mdash; every action, every change, every
                dollar &mdash; stays on screen.
              </p>
            </FadeUp>

            {/* CTAs */}
            <FadeUp delay={0.2}>
              <div className="mt-9 flex items-center justify-center gap-4">
                <Link
                  href="/#install"
                  className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-lg active:scale-[0.97]"
                >
                  Get mycel
                  <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
                </Link>
                <Link
                  href="https://github.com/rpuneet/mycel"
                  className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 font-body text-sm font-medium text-on-surface-variant transition-colors hover:border-primary/30 hover:bg-surface-container hover:text-primary active:scale-[0.97]"
                >
                  <Github className="h-4 w-4" aria-hidden="true" />
                  GitHub
                </Link>
              </div>
              <div className="mt-5">
                <Link
                  href="/method"
                  className="group inline-flex items-center gap-1.5 font-body text-sm text-on-surface-variant transition-colors hover:text-primary"
                >
                  See how it works &mdash; the mycel method
                  <ArrowRight
                    className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5"
                    aria-hidden="true"
                  />
                </Link>
              </div>
            </FadeUp>
          </div>

          {/* Hero shot — the fleet, live */}
          <FadeUp delay={0.28}>
            <div className="hero-stage mx-auto mt-14 max-w-6xl px-4 sm:px-6">
              <div className="hero-tilt">
                <ProductFrame
                  srcDark="/screenshots/agents-dark.png"
                  srcLight="/screenshots/agents-light.png"
                  alt="The mycel agents view: a fleet of agents with living character faces, grouped by repository, each with its status, last activity, and cost"
                  title="mycel — agents"
                  width={1440}
                  height={700}
                  priority
                  className="hero-glow"
                />
              </div>
              <p className="mt-5 text-center font-body text-sm text-on-surface-variant">
                The fleet at a glance &mdash; every agent, its status, and
                what it&rsquo;s spending.
              </p>
            </div>
          </FadeUp>
        </section>

        {/* Section separator */}
        <SporeDivider />

        {/* ════════════════════════════════════════
           The deck — one capability per panel
           ════════════════════════════════════════ */}
        <section id="product" className="deck-veil scroll-mt-24 py-14 sm:py-20">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <FadeUp className="mb-6 text-center">
              <span className="deck-eyebrow">What it does</span>
              <h2 className="mx-auto mt-4 max-w-3xl font-headline text-3xl font-semibold tracking-tight text-on-background md:text-5xl">
                One window into everything
                <br className="hidden sm:block" />{" "}
                your agents do.
              </h2>
              <p className="mx-auto mt-5 max-w-2xl font-body text-lg text-on-surface-variant">
                Six views, one app &mdash; every screenshot captured live
                from a working team.
              </p>
            </FadeUp>

            <div className="relative">
              {/* 01 — Agents */}
              <DeckPanel
                index="01"
                eyebrow="Agents"
                title="Meet the fleet. Every agent has a face."
                body={
                  <>
                    Each agent is a living character &mdash; one glance at the
                    roster tells you who&rsquo;s working, who&rsquo;s idle, and
                    who&rsquo;s done before you read a word. Agents are grouped
                    by the repository they work in, with status, last activity,
                    and cost side by side. Start, stop, or inspect any of them
                    from the same row.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/agents-dark.png"
                    alt="Agent roster grouped by repository, showing character avatars, status badges, activity, and per-agent cost"
                    title="Agents"
                    width={1440}
                    height={700}
                  />
                }
              />

              {/* 02 — Apps */}
              <DeckPanel
                index="02"
                eyebrow="Apps"
                title="Reachable where you already talk."
                body={
                  <>
                    Connect Slack, WhatsApp, Telegram, Discord, GitHub, and
                    twenty-plus other apps. Your agents join the channels you
                    already use, post updates as themselves, and answer when
                    you reply &mdash; from your phone, in the thread you were
                    already in. One screen shows every connection and every
                    conversation flowing through it.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/apps-dark.png"
                    alt="The apps view: connected platforms including Slack, Telegram, IRC, and WhatsApp, with channels and live message activity"
                    title="Apps"
                    width={1100}
                    height={900}
                  />
                }
                imageFirst
              />

              {/* 03 — Live */}
              <DeckPanel
                index="03"
                eyebrow="Live"
                title="Every action, the moment it happens."
                body={
                  <>
                    The Live feed streams what each agent is doing right now
                    &mdash; every command it runs, every file it reads, every
                    tool it calls, timed to the millisecond. There is no black
                    box: if an agent did it, it&rsquo;s in the feed, and you
                    can expand any line to see exactly what happened.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/live-dark.png"
                    alt="The live feed: a working agent's stream of commands, file reads, and tool calls with timing for each action"
                    title="Live"
                    width={1440}
                    height={900}
                  />
                }
              />

              {/* 04 — Code */}
              <DeckPanel
                index="04"
                eyebrow="Code"
                title="Every change, reviewable before it ships."
                body={
                  <>
                    Open any agent&rsquo;s working copy and read its changes as
                    a side-by-side diff &mdash; added lines, removed lines,
                    file by file. You review agents the way you review
                    colleagues: look at the work, not a summary of it.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/code-dark.png"
                    alt="An agent's code view showing a side-by-side diff of a changed file, with the full file tree of its working copy"
                    title="Code"
                    width={1440}
                    height={900}
                  />
                }
                imageFirst
              />

              {/* 05 — Costs */}
              <DeckPanel
                index="05"
                eyebrow="Costs"
                title="Spend, read straight from the source."
                body={
                  <>
                    mycel reads usage from each agent&rsquo;s own records
                    &mdash; no estimates, no bookkeeping. See spend by agent,
                    by model, and over time, with burn rate and your biggest
                    cost driver called out for whatever range you pick. The
                    bill never surprises you, because you watched it happen.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/insights-dark.png"
                    alt="The insights view: total spend, token counts, burn rate, and cost broken down by agent and by model"
                    title="Insights"
                    width={1440}
                    height={860}
                  />
                }
              />

              {/* 06 — One app */}
              <DeckPanel
                index="06"
                eyebrow="One app"
                title="On your desktop, in your browser — the same app."
                body={
                  <>
                    mycel runs as a desktop app and serves the same interface
                    in your browser on localhost. Light or dark, laptop or
                    second screen &mdash; it&rsquo;s one app, it runs on your
                    machine, and it runs everything.
                  </>
                }
                artifact={
                  <ProductFrame
                    srcDark="/screenshots/agents-light.png"
                    alt="The same agents view in the light theme, showing the app adapts to your preference"
                    title="mycel — light theme"
                    width={1440}
                    height={700}
                  />
                }
                imageFirst
                last
              />
            </div>

            {/* Connective serif accent */}
            <FadeUp className="mx-auto mt-10 max-w-2xl text-center">
              <p className="deck-serif text-2xl leading-snug text-on-surface-variant sm:text-3xl">
                A team you can see is a team you can trust &mdash;{" "}
                <span className="text-primary">
                  every agent, every change, every cent.
                </span>
              </p>
              <Link
                href="/method"
                className="group mt-6 inline-flex items-center gap-1.5 font-body text-sm text-on-surface-variant transition-colors hover:text-primary"
              >
                Why it&rsquo;s built this way &mdash; read the method
                <ArrowRight
                  className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5"
                  aria-hidden="true"
                />
              </Link>
            </FadeUp>
          </div>
        </section>

        {/* Section separator */}
        <SporeDivider />

        {/* ════════════════════════════════════════
           Get mycel — compact, terminal-flavored
           ════════════════════════════════════════ */}
        <InstallSection />

        {/* ════════════════════════════════════════
           Open-source CTA
           ════════════════════════════════════════ */}
        <RevealSection className="py-14 sm:py-20">
          <div className="mx-auto max-w-3xl px-4 text-center sm:px-6">
            <h2 className="font-headline text-3xl font-semibold tracking-tight text-on-background md:text-4xl">
              Free, open source, and yours to run.
            </h2>
            <p className="mt-3 font-body text-lg text-on-surface-variant">
              MIT licensed. No cloud account. It runs on your machine.
            </p>
            <div className="mt-8 flex flex-col items-center justify-center gap-4 sm:flex-row">
              <Link
                href="https://github.com/rpuneet/mycel"
                className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-[0_0_20px_color-mix(in_srgb,var(--primary)_30%,transparent)] active:scale-[0.97]"
              >
                <Github className="h-5 w-5" aria-hidden="true" />
                View on GitHub
              </Link>
              <Link
                href="/docs"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-8 font-body text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container hover:text-on-surface active:scale-[0.97]"
              >
                Browse the docs
                <ArrowRight className="h-4 w-4" aria-hidden="true" />
              </Link>
            </div>
          </div>
        </RevealSection>

        {/* Footer gradient separator */}
        <div className="mx-auto max-w-6xl px-6"><div className="footer-separator" /></div>

        <Footer />
      </div>
    </main>
  );
}
