"use client";

import { useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { TerminalWindow } from "./_components/TerminalWindow";
import {
  RevealSection,
  FadeUp,
  SlideIn,
  StaggerChildren,
  StaggerItem,
} from "./_components/Motion";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";

/* ═══════════════════════════════════════════════════════════
   SECTION 1 — HERO
   ═══════════════════════════════════════════════════════════ */

function HeroSection() {
  return (
    <section className="hero-glow pt-20 pb-32">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="grid items-center gap-12 lg:grid-cols-2 lg:gap-16">
          {/* Left — text */}
          <FadeUp>
            {/* Version badge */}
            <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-outline-variant/20 bg-surface-container px-4 py-1.5 text-sm font-label">
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
              </span>
              <span className="text-on-surface-variant">v0.1.0 is live</span>
            </div>

            <h1 className="font-headline text-5xl font-bold tracking-tight lg:text-7xl text-on-background">
              AI in a Box.
            </h1>
            <p className="mt-6 max-w-lg text-lg leading-relaxed text-on-surface-variant">
              Orchestrate teams of AI agents from your terminal. Isolated
              worktrees, structured channels, hard budget caps &mdash; all from
              a single CLI.
            </p>

            <div className="mt-8 flex flex-wrap items-center gap-4">
              <Link
                href="/docs"
                className="cta-glow inline-flex h-12 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-xl active:scale-[0.97]"
              >
                Get Started
              </Link>
              <Link
                href="https://github.com/rpuneet/bc"
                className="inline-flex h-12 items-center gap-2 rounded-lg border border-outline-variant/20 px-8 text-sm font-medium text-on-background transition-colors hover:bg-surface-container active:scale-[0.97]"
              >
                View on GitHub
              </Link>
            </div>
          </FadeUp>

          {/* Right — terminal */}
          <SlideIn direction="right" delay={0.2}>
            <TerminalWindow title="bash — 80x24">
              <div className="space-y-1 text-[13px] leading-relaxed">
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">bc init</span>
                </div>
                <div className="text-terminal-muted">
                  Initializing mycel workspace...
                </div>
                <div className="text-terminal-muted">
                  Created .bc/settings.json
                </div>
                <div className="text-terminal-success">Ready.</div>
                <div className="h-3" />
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">bc up</span>
                </div>
                <div className="text-terminal-muted">
                  Starting server on :9374...{" "}
                  <span className="text-terminal-success">[OK]</span>
                </div>
                <div className="text-terminal-muted">
                  Web UI available at http://localhost:9374
                </div>
                <div className="h-3" />
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">
                    bc agent create eng-01 --role engineer --tool claude
                  </span>
                </div>
                <div className="text-terminal-muted">
                  Spawning agent eng-01...
                </div>
                <div className="text-terminal-muted">
                  Created worktree at .bc/agents/eng-01/worktree
                </div>
                <div className="text-terminal-success">
                  Agent eng-01 is online.
                </div>
                <div className="h-3" />
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="terminal-cursor" />
                </div>
              </div>
            </TerminalWindow>
          </SlideIn>
        </div>
      </div>
    </section>
  );
}

/* ═══════════════════════════════════════════════════════════
   SECTION 2 — PROVIDER MARQUEE
   ═══════════════════════════════════════════════════════════ */

function ProviderMarquee() {
  const providers = ["Claude", "Gemini", "Cursor", "Codex"];
  return (
    <section className="border-y border-outline-variant/20 bg-surface-container py-12">
      <div className="mx-auto flex items-center justify-center gap-8 sm:gap-12 md:gap-16">
        {providers.map((name, i) => (
          <span key={name} className="flex items-center gap-8 sm:gap-12 md:gap-16">
            {i > 0 && (
              <span className="text-on-surface-variant/30 text-lg select-none">
                &bull;
              </span>
            )}
            <span className="font-headline text-sm uppercase tracking-[0.25em] text-on-surface-variant/60 sm:text-base">
              {name}
            </span>
          </span>
        ))}
      </div>
    </section>
  );
}

/* ═══════════════════════════════════════════════════════════
   SECTION 3 — PROBLEM HOOK
   ═══════════════════════════════════════════════════════════ */

function ProblemSection() {
  return (
    <RevealSection className="py-24">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <h2 className="mx-auto max-w-3xl text-center font-headline text-3xl font-bold tracking-tight text-on-background lg:text-4xl">
          AI agents are powerful alone &mdash; but chaotic when they work
          together.
        </h2>

        <div className="mt-16 grid gap-8 lg:grid-cols-2 lg:gap-12">
          {/* WITHOUT */}
          <div className="relative overflow-hidden rounded-lg bg-surface-container p-6 pl-8">
            <div className="absolute left-0 top-0 h-full w-1 bg-destructive" />
            <h3 className="mb-5 flex items-center gap-2 font-label text-xs font-bold uppercase tracking-wider text-destructive">
              <svg
                className="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path d="M18 6 6 18M6 6l12 12" />
              </svg>
              Without Mycel
            </h3>
            <ul className="space-y-4 text-sm">
              {[
                "Git conflicts overriding human work",
                "Infinite loops of agents talking past each other",
                "Unpredictable API costs skyrocketing",
              ].map((t) => (
                <li
                  key={t}
                  className="flex items-start gap-3 text-on-surface-variant"
                >
                  <span className="mt-0.5 shrink-0 text-destructive/60">
                    &#x2715;
                  </span>
                  {t}
                </li>
              ))}
            </ul>
          </div>

          {/* WITH */}
          <div className="relative overflow-hidden rounded-lg bg-surface-container-high p-6 pl-8">
            <div className="absolute left-0 top-0 h-full w-1 bg-primary" />
            <h3 className="mb-5 flex items-center gap-2 font-label text-xs font-bold uppercase tracking-wider text-primary">
              <svg
                className="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path d="M20 6 9 17l-5-5" />
              </svg>
              With Mycel
            </h3>
            <ul className="space-y-4 text-sm">
              {[
                "Isolated worktrees for every agent",
                "Structured communication channels",
                "Hard budget caps per agent",
              ].map((t) => (
                <li
                  key={t}
                  className="flex items-start gap-3 text-on-surface-variant"
                >
                  <span className="mt-0.5 shrink-0 text-primary/60">
                    &#x2713;
                  </span>
                  {t}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </RevealSection>
  );
}

/* ═══════════════════════════════════════════════════════════
   SECTION 4 — BENTO GRID
   ═══════════════════════════════════════════════════════════ */

const bentoCards = [
  {
    title: "Dashboard",
    desc: "See everything at a glance. Real-time telemetry.",
    span: "lg:col-span-2",
    visual: (
      <div className="relative mt-4 h-40 overflow-hidden rounded-md">
        <Image
          src="/screenshots/dashboard-01-home.png"
          alt="Dashboard screenshot"
          fill
          className="object-cover object-top opacity-60"
        />
      </div>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="4" rx="1" />
        <rect x="14" y="11" width="7" height="10" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
      </svg>
    ),
  },
  {
    title: "Agents",
    desc: "Spawn, monitor, and manage AI agents.",
    visual: (
      <div className="mt-4 flex flex-col gap-2 font-label text-xs">
        <div className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-terminal-success" />
          <span className="text-on-surface-variant">eng-01 working</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-terminal-success" />
          <span className="text-on-surface-variant">eng-02 working</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-terminal-muted" />
          <span className="text-on-surface-variant">mgr-01 idle</span>
        </div>
      </div>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <circle cx="12" cy="8" r="4" />
        <path d="M6 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2" />
      </svg>
    ),
  },
  {
    title: "Channels",
    desc: "Real-time inter-agent communication.",
    visual: (
      <div className="mt-4 flex flex-col gap-2 font-label text-xs text-on-surface-variant">
        <div>#eng-sync</div>
        <div>#design-review</div>
      </div>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
      </svg>
    ),
  },
  {
    title: "Worktrees",
    desc: "Each agent on its own git branch. No conflicts.",
    visual: (
      <svg className="mt-4 h-16 w-full" viewBox="0 0 200 60" fill="none">
        {/* trunk */}
        <line x1="100" y1="55" x2="100" y2="25" stroke="var(--on-surface-variant)" strokeWidth="2" strokeOpacity="0.4" />
        {/* branches out */}
        <line x1="100" y1="25" x2="50" y2="5" stroke="var(--primary)" strokeWidth="2" strokeOpacity="0.6" />
        <line x1="100" y1="25" x2="100" y2="5" stroke="var(--primary)" strokeWidth="2" strokeOpacity="0.6" />
        <line x1="100" y1="25" x2="150" y2="5" stroke="var(--primary)" strokeWidth="2" strokeOpacity="0.6" />
        {/* dots */}
        <circle cx="50" cy="5" r="4" fill="var(--primary)" />
        <circle cx="100" cy="5" r="4" fill="var(--primary)" />
        <circle cx="150" cy="5" r="4" fill="var(--primary)" />
        <circle cx="100" cy="55" r="4" fill="var(--on-surface-variant)" fillOpacity="0.4" />
      </svg>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path d="M6 3v12" />
        <circle cx="18" cy="6" r="3" />
        <circle cx="6" cy="18" r="3" />
        <path d="M18 9a9 9 0 0 1-9 9" />
      </svg>
    ),
  },
  {
    title: "Costs",
    desc: "Per-agent cost tracking with budgets.",
    visual: (
      <div className="mt-4 font-label text-xs">
        <div className="flex items-baseline justify-between">
          <span className="text-on-surface-variant">eng-01</span>
          <span className="text-primary">$12.40 / $50.00</span>
        </div>
        <div className="mt-2 h-1.5 w-full rounded-full bg-surface-container-highest">
          <div className="h-1.5 w-1/4 rounded-full bg-primary" />
        </div>
      </div>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
      </svg>
    ),
  },
  {
    title: "Cron",
    desc: "Schedule recurring agent tasks.",
    visual: (
      <div className="mt-4 font-label text-xs text-on-surface-variant">
        <code className="rounded bg-surface-container-highest px-2 py-1">
          0 0 * * * run_tests
        </code>
      </div>
    ),
    icon: (
      <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <circle cx="12" cy="12" r="10" />
        <path d="M12 6v6l4 2" />
      </svg>
    ),
  },
];

function BentoSection() {
  return (
    <RevealSection className="bg-surface-container-lowest py-24">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <h2 className="mb-16 text-center font-headline text-3xl font-bold tracking-tight text-on-background">
          Core Subsystems
        </h2>

        <StaggerChildren className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {bentoCards.map((card) => (
            <StaggerItem
              key={card.title}
              className={`bento-card group relative overflow-hidden rounded-lg bg-surface-container p-6 ${card.span ?? ""}`}
            >
              {/* Hover accent bar */}
              <div className="absolute left-0 top-0 h-full w-0.5 bg-primary opacity-0 transition-opacity group-hover:opacity-100" />
              {/* Icon */}
              <div className="absolute right-4 top-4 text-on-surface-variant/30">
                {card.icon}
              </div>
              <h3 className="font-headline text-lg font-semibold text-on-background">
                {card.title}
              </h3>
              <p className="mt-1 text-sm text-on-surface-variant">{card.desc}</p>
              {card.visual}
            </StaggerItem>
          ))}
        </StaggerChildren>
      </div>
    </RevealSection>
  );
}

/* ═══════════════════════════════════════════════════════════
   SECTION 5 — DASHBOARD PREVIEW
   ═══════════════════════════════════════════════════════════ */

const dashboardTabs = [
  { label: "Dashboard", src: "/screenshots/dashboard-01-home.png" },
  { label: "Agents", src: "/screenshots/dashboard-02-agents.png" },
  { label: "Channels", src: "/screenshots/dashboard-03-channels.png" },
  { label: "Costs", src: "/screenshots/dashboard-04-costs.png" },
  { label: "Stats", src: "/screenshots/dashboard-10-stats-loaded.png" },
];

function DashboardPreview() {
  const [activeTab, setActiveTab] = useState(0);

  return (
    <RevealSection className="py-24">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <h2 className="mb-12 text-center font-headline text-3xl font-bold tracking-tight text-on-background">
          Dashboard Preview
        </h2>

        {/* Tab bar */}
        <div className="mb-8 flex items-center justify-center gap-1">
          {dashboardTabs.map((tab, i) => (
            <button
              key={tab.label}
              onClick={() => setActiveTab(i)}
              className={`rounded-md px-4 py-2 font-label text-xs transition-colors ${
                i === activeTab
                  ? "bg-primary text-primary-foreground"
                  : "text-on-surface-variant hover:bg-surface-container"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Screenshot */}
        <div className="glass-panel terminal-glow mx-auto max-w-5xl overflow-hidden rounded-lg">
          <div className="relative aspect-[16/10] w-full">
            <Image
              src={dashboardTabs[activeTab].src}
              alt={`${dashboardTabs[activeTab].label} view`}
              fill
              className="object-cover object-top"
              priority={activeTab === 0}
            />
          </div>
        </div>
      </div>
    </RevealSection>
  );
}

/* ═══════════════════════════════════════════════════════════
   SECTION 6 — INSTALL
   ═══════════════════════════════════════════════════════════ */

const installTabs = [
  {
    label: "macOS / Linux",
    code: "curl -fsSL https://raw.githubusercontent.com/rpuneet/bc/main/scripts/install.sh | bash",
  },
  {
    label: "Homebrew",
    code: "brew install rpuneet/bc/bc",
  },
  {
    label: "Docker",
    code: "docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/bc bc up --addr 0.0.0.0:9374",
  },
];

function InstallSection() {
  const [activeTab, setActiveTab] = useState(0);

  return (
    <RevealSection className="py-32">
      <div className="mx-auto max-w-screen-md px-4 sm:px-6">
        <h2 className="text-center font-headline text-3xl font-bold tracking-tight text-on-background">
          Get started in 30 seconds
        </h2>
        <p className="mt-3 text-center text-lg text-on-surface-variant">
          Get the CLI up and running in seconds.
        </p>

        {/* Tab switcher */}
        <div className="mt-10 flex items-center justify-center gap-1">
          {installTabs.map((tab, i) => (
            <button
              key={tab.label}
              onClick={() => setActiveTab(i)}
              className={`rounded-md px-4 py-2 font-label text-xs transition-colors ${
                i === activeTab
                  ? "bg-primary text-primary-foreground"
                  : "text-on-surface-variant hover:bg-surface-container"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Code block */}
        <div className="mt-6 overflow-x-auto rounded-lg bg-terminal-bg p-4 font-label text-sm text-terminal-text">
          <pre className="whitespace-pre-wrap break-all">
            <span className="text-terminal-prompt">$ </span>
            {installTabs[activeTab].code}
          </pre>
        </div>

        {/* Then run */}
        <div className="mt-10">
          <p className="mb-4 text-center font-label text-xs uppercase tracking-widest text-on-surface-variant">
            Then run:
          </p>
          <div className="overflow-hidden rounded-lg bg-terminal-bg p-4 font-label text-sm text-terminal-text">
            <div>
              <span className="text-terminal-prompt">$ </span>
              <span className="text-terminal-command">bc init</span>
            </div>
            <div className="mt-1">
              <span className="text-terminal-prompt">$ </span>
              <span className="text-terminal-command">bc up</span>
            </div>
            <div className="mt-1">
              <span className="text-terminal-prompt">$ </span>
              <span className="text-terminal-command">
                bc agent create eng-01 --role engineer --tool claude
              </span>
            </div>
          </div>
        </div>
      </div>
    </RevealSection>
  );
}

/* ═══════════════════════════════════════════════════════════
   PAGE
   ═══════════════════════════════════════════════════════════ */

export default function Home() {
  return (
    <main className="min-h-screen overflow-x-hidden">
      <Nav />
      <HeroSection />
      <ProviderMarquee />
      <ProblemSection />
      <BentoSection />
      <DashboardPreview />
      <InstallSection />
      <Footer />
    </main>
  );
}
