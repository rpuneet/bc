"use client";

import { type ReactNode } from "react";
import Link from "next/link";
import { ArrowRight, Github } from "lucide-react";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";
import { InstallSection } from "./_components/InstallSection";
import { ProductFrame } from "./_components/ProductFrame";
import { HeroShowcase } from "./_components/HeroShowcase";
import { SocialProof } from "./_components/SocialProof";
import { DownloadButtons } from "./_components/DownloadButtons";
import { Convictions } from "./_components/Convictions";
import { WhyMycel } from "./_components/WhyMycel";
import { GettingStarted } from "./_components/GettingStarted";
import { ChangelogPill } from "./_components/ChangelogPill";
import { ChatThread } from "./_components/ChatThread";
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
  showNode = true,
}: {
  index: string;
  eyebrow: string;
  title: ReactNode;
  body: ReactNode;
  artifact: ReactNode;
  imageFirst?: boolean;
  last?: boolean;
  // A lone panel (no series) shouldn't carry the hyphae-rail node marker.
  showNode?: boolean;
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
      {/* Ember-rail node marker (desktop) — only within a panel series */}
      {showNode && (
        <span
          aria-hidden="true"
          className="ember-node absolute left-1/2 top-16 hidden h-3 w-3 -translate-x-1/2 rounded-full bg-primary lg:block"
        />
      )}
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
      {/* Drifting spore field — fixed, covers the page. Calmed (decision #4)
         so it never competes with the animated hero below. */}
      <AnimatedBackground />
      {/* Warm radial wash above the fold */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,color-mix(in_srgb,var(--primary)_10%,transparent),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* ════════════════════════════════════════
           Hero — the thesis, the download, the live product
           ════════════════════════════════════════ */}
        <section className="pt-28 pb-10 sm:pt-32 sm:pb-14">
          <div className="mx-auto max-w-4xl px-4 text-center sm:px-6">
            <FadeUp>
              <div className="mb-6 flex justify-center">
                <ChangelogPill />
              </div>
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
                Every agent gets a name, a face, and a job &mdash; writing code
                in your repositories, reaching you on Slack or WhatsApp, with
                every action, every change, and every dollar on screen.
              </p>
            </FadeUp>

            {/* Primary conversion: download the desktop app (decision #3) */}
            <FadeUp delay={0.2}>
              <div className="mt-9">
                <DownloadButtons />
              </div>
              <div className="mt-5 flex flex-col items-center gap-2">
                <div className="flex items-center gap-5">
                  <Link
                    href="https://github.com/rpuneet/mycel"
                    className="inline-flex items-center gap-1.5 font-body text-sm text-on-surface-variant transition-colors hover:text-primary"
                  >
                    <Github className="h-4 w-4" aria-hidden="true" />
                    GitHub
                  </Link>
                  <span aria-hidden="true" className="text-outline-variant/30">
                    ·
                  </span>
                  <Link
                    href="/#install"
                    className="inline-flex items-center gap-1.5 font-body text-sm text-on-surface-variant transition-colors hover:text-primary"
                  >
                    Prefer the terminal? Install the CLI
                  </Link>
                </div>
                {/* Honest cost line (decision #5 — pricing page removed). */}
                <p className="font-body text-xs text-on-surface-variant/70">
                  Free and open source &mdash; you only pay your model
                  providers.
                </p>
              </div>
            </FadeUp>

            {/* Social proof: live GitHub numbers (decision #2) */}
            <FadeUp delay={0.26}>
              <div className="mt-8">
                <SocialProof />
              </div>
            </FadeUp>
          </div>

          {/* The centerpiece: tabbed, live product frame (decision #1) */}
          <FadeUp delay={0.32}>
            <div className="mt-14 px-4 sm:px-6">
              <HeroShowcase />
            </div>
          </FadeUp>
        </section>

        {/* Section separator */}
        <SporeDivider />

        {/* ════════════════════════════════════════
           Why mycel — the positioning beat (team, not one agent)
           ════════════════════════════════════════ */}
        <WhyMycel />

        {/* Section separator */}
        <SporeDivider />

        {/* ════════════════════════════════════════
           Beyond the dashboard — reachable everywhere
           ════════════════════════════════════════ */}
        <section id="product" className="deck-veil scroll-mt-24 py-14 sm:py-20">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <FadeUp className="mb-6 text-center">
              <span className="deck-eyebrow">More than a window</span>
              <h2 className="mx-auto mt-4 max-w-3xl font-headline text-3xl font-semibold tracking-tight text-on-background md:text-5xl">
                Not just a place to watch.
                <br className="hidden sm:block" />{" "}
                A place your team lives.
              </h2>
              <p className="mx-auto mt-5 max-w-2xl font-body text-lg text-on-surface-variant">
                The four views above are one app. It also reaches into the
                tools your team already uses.
              </p>
            </FadeUp>

            <div className="relative">
              {/* Apps — the one view not in the hero tabs, and a real
                 differentiator: agents reachable where you already talk. */}
              <DeckPanel
                index="01"
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
                    <span className="mt-6 flex flex-wrap gap-2">
                      {[
                        "Slack",
                        "WhatsApp",
                        "Telegram",
                        "Discord",
                        "GitHub",
                        "IRC",
                        "+20 more",
                      ].map((p) => (
                        <span
                          key={p}
                          className="inline-flex items-center rounded-full border border-outline-variant/20 bg-surface-container/50 px-3 py-1 font-label text-[11px] text-on-surface-variant"
                        >
                          {p}
                        </span>
                      ))}
                    </span>
                  </>
                }
                artifact={
                  <div className="relative">
                    <ProductFrame
                      srcDark="/screenshots/apps-dark.png"
                      motion="apps"
                      alt="The apps view, live: connected platforms including Slack, Telegram, IRC, and WhatsApp, with real channel activity and notifications scrolling by"
                      title="Apps"
                      width={1100}
                      height={900}
                    />
                    {/* Concrete proof (teardown #14): a real ping→reply thread,
                       overlapping the screenshot so the "reachable where you
                       talk" claim reads as tangible, not abstract. */}
                    <div className="relative z-10 -mt-10 ml-auto w-[94%] sm:-mt-16 sm:w-[78%]">
                      <ChatThread />
                    </div>
                  </div>
                }
                last
                showNode={false}
              />
            </div>

            {/* One-app parity — a short callout, not a redundant screenshot */}
            <FadeUp className="mx-auto mt-6 max-w-2xl text-center">
              <p className="deck-serif text-2xl leading-snug text-on-surface-variant sm:text-3xl">
                On your desktop or in your browser, light or dark &mdash;{" "}
                <span className="text-primary">
                  it&rsquo;s one app, and it runs on your machine.
                </span>
              </p>
            </FadeUp>
          </div>
        </section>

        {/* ════════════════════════════════════════
           The six convictions — dense, in-product, no essay
           ════════════════════════════════════════ */}
        <Convictions />

        {/* Section separator */}
        <SporeDivider />

        {/* ════════════════════════════════════════
           Getting started — the 3-step bridge into the docs
           ════════════════════════════════════════ */}
        <GettingStarted />

        {/* ════════════════════════════════════════
           Get mycel — CLI power-user path (demoted below the download hero)
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
            {/* Honest cost signal — the old /pricing page (Cloud/Enterprise
               tiers that don't exist) was removed (decision #5). Pricing
               returns as its own surface when mycel-cloud ships. */}
            <p className="mt-3 font-body text-lg text-on-surface-variant">
              MIT licensed. No cloud account, no sign-up. You only pay your
              model providers.
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
