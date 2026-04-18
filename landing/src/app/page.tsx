import Link from "next/link";
import { Nav } from "./_components/Nav";
import { DashboardScreenshots } from "./_components/DashboardScreenshots";
import { Footer } from "./_components/Footer";
import { HeroSection } from "./_components/HeroSection";
import { BentoGrid } from "./_components/BentoGrid";
import { InstallSection } from "./_components/InstallSection";
import {
  TerminalWindow,
  CommandOutput,
  RevealSection,
} from "./_components/TerminalComponents";
import { ToolMarquee } from "./_components/ToolLogos";
import { AnimatedBackground } from "./_components/AnimatedBackground";
import { CheckCircle2, XCircle } from "lucide-react";
import { ArrowRight } from "lucide-react";

export default function Home() {
  return (
    <main className="min-h-screen selection:bg-primary/20 selection:text-foreground overflow-x-hidden">
      {/* Animated particle background */}
      <AnimatedBackground />

      {/* Gradient overlay */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.04),transparent)] dark:bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.08),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* Hero */}
        <HeroSection />

        {/* Tool Carousel */}
        <div className="mt-8 sm:mt-12 lg:mt-16 mx-auto max-w-6xl px-4 sm:px-6">
          <ToolMarquee />
        </div>

        <div className="mx-auto max-w-7xl px-4 sm:px-6">
          {/* Problem / Solution */}
          <RevealSection className="py-12 sm:py-16 lg:py-28" id="problem">
            <div className="mb-16">
              <span className="font-mono text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                The problem
              </span>
              <h2 className="mt-3 text-3xl font-bold tracking-tight sm:text-5xl">
                AI agents are powerful alone —
                <br />
                <span className="text-muted-foreground/50">
                  but chaotic when they work together.
                </span>
              </h2>
            </div>

            <div className="grid gap-8 lg:grid-cols-2 lg:gap-16">
              <div className="rounded-xl border border-destructive/20 bg-card/90 backdrop-blur-sm p-6">
                <h3 className="mb-5 flex items-center gap-2 font-mono text-sm font-bold uppercase tracking-wider text-destructive">
                  <XCircle className="h-4 w-4" aria-hidden="true" />
                  Without mycel
                </h3>
                <ul className="space-y-4 text-sm">
                  {[
                    "Serial execution — only one agent at a time",
                    "Context lost between sessions",
                    "Merge conflicts from parallel edits",
                    "No visibility into agent activity or spending",
                    "Surprise cost overruns at end of month",
                  ].map((t) => (
                    <li key={t} className="flex gap-3 text-muted-foreground">
                      <span className="text-destructive/60 shrink-0">
                        &#x2715;
                      </span>
                      {t}
                    </li>
                  ))}
                </ul>
              </div>

              <div className="rounded-xl border border-success/20 bg-card/90 backdrop-blur-sm p-6">
                <h3 className="mb-5 flex items-center gap-2 font-mono text-sm font-bold uppercase tracking-wider text-success">
                  <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                  With mycel
                </h3>
                <ul className="space-y-4 text-sm">
                  {[
                    "Multiple agents working in parallel",
                    "Persistent memory injected on spawn",
                    "Git worktrees — zero merge conflicts",
                    "Real-time visibility into every agent",
                    "Per-agent budgets with automatic hard stops",
                  ].map((t) => (
                    <li key={t} className="flex gap-3 text-muted-foreground">
                      <span className="text-success/60 shrink-0">&#x2713;</span>
                      {t}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </RevealSection>

          {/* How It Works */}
          <RevealSection className="py-12 sm:py-16 lg:py-28" id="how-it-works">
            <div className="mb-16 text-center">
              <span className="font-mono text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                How it works
              </span>
              <h2 className="mt-3 text-3xl font-bold tracking-tight sm:text-5xl">
                How it actually works.
              </h2>
            </div>

            <div className="grid gap-6 md:grid-cols-3 md:gap-8">
              {[
                {
                  step: "01",
                  cmd: "# agents run locally in Docker",
                  title: "Isolated by design",
                  desc: "Each agent gets its own Docker container and git worktree. They can't step on each other's work.",
                  lines: [
                    { text: "eng-01  docker  worktree/eng-01  working", color: "text-terminal-success" },
                    { text: "eng-02  docker  worktree/eng-02  working", color: "text-terminal-success" },
                    { text: "mgr-01  docker  worktree/mgr-01  idle", color: "text-terminal-muted" },
                    { text: "3 agents · 0 conflicts", color: "text-terminal-success" },
                  ],
                },
                {
                  step: "02",
                  cmd: "# agents coordinate through channels",
                  title: "Structured communication",
                  desc: "Agents talk through persistent channels — mentions, reviews, handoffs. Not through you.",
                  lines: [
                    { text: "[#eng @eng-01] PR ready for review", color: "text-terminal-success" },
                    { text: "[#eng @mgr-01] LGTM. Merged.", color: "text-terminal-success" },
                    { text: "[#eng @eng-02] Starting next task.", color: "text-terminal-muted" },
                  ],
                },
                {
                  step: "03",
                  cmd: "# you see everything",
                  title: "Full visibility",
                  desc: "Costs, activity, resource usage, channel messages — all in real time. Trust through transparency.",
                  lines: [
                    { text: "eng-01  $2.14  245k tokens  43% budget", color: "text-terminal-success" },
                    { text: "eng-02  $1.67  189k tokens  33% budget", color: "text-terminal-success" },
                    { text: "total   $3.81  434k tokens", color: "text-terminal-muted" },
                  ],
                },
              ].map((s, i) => (
                <div key={s.step}>
                  <div className="mb-4 font-mono text-xs font-bold uppercase tracking-[0.3em] text-muted-foreground/40">
                    {s.step}
                  </div>
                  <TerminalWindow title={`step ${s.step}`}>
                    <CommandOutput
                      command={s.cmd}
                      lines={s.lines}
                      delay={i * 0.2}
                    />
                  </TerminalWindow>
                  <h3 className="mt-5 mb-2 text-lg font-semibold tracking-tight">
                    {s.title}
                  </h3>
                  <p className="text-sm leading-relaxed text-muted-foreground">
                    {s.desc}
                  </p>
                </div>
              ))}
            </div>
          </RevealSection>

          {/* Feature Bento Grid */}
          <RevealSection className="py-12 sm:py-16 lg:py-28" id="features">
            <div className="mb-16 text-center">
              <span className="font-mono text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                Features
              </span>
              <h2 className="mt-3 text-3xl font-bold tracking-tight sm:text-5xl">
                Everything you need.
              </h2>
            </div>
            <BentoGrid />
          </RevealSection>

          {/* Install */}
          <InstallSection />

          {/* Dashboard Preview */}
          <RevealSection className="py-12 sm:py-16 lg:py-28" id="demo">
            <div className="mb-12 text-center">
              <span className="font-mono text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                Dashboard
              </span>
              <h2 className="mt-3 text-3xl font-bold tracking-tight sm:text-4xl">
                See the real dashboard.
              </h2>
              <p className="mt-4 text-muted-foreground text-lg max-w-xl mx-auto">
                Monitor agents, channels, and costs from localhost:9374.
              </p>
            </div>
            <div className="mx-auto max-w-5xl">
              <DashboardScreenshots />
            </div>
          </RevealSection>

          {/* Final CTA */}
          <RevealSection className="pb-12 sm:pb-16 lg:pb-24">
            <div className="relative overflow-hidden rounded-2xl border border-border bg-card shadow-[var(--card-shadow)]">
              <div className="absolute inset-0 bg-[radial-gradient(circle_at_30%_50%,rgba(234,88,12,0.04),transparent)] dark:bg-[radial-gradient(circle_at_30%_50%,rgba(234,88,12,0.06),transparent)] pointer-events-none" />
              <div className="relative grid items-center gap-8 p-8 sm:p-12 lg:grid-cols-[1fr_auto] lg:gap-16 lg:p-16">
                <div>
                  <h2 className="text-3xl font-bold tracking-tight sm:text-5xl">
                    Start orchestrating in 60 seconds.
                  </h2>
                  <p className="mt-4 max-w-lg text-lg text-muted-foreground">
                    Free. Open source. No login required. Just three commands.
                  </p>
                  <div className="mt-8 flex flex-wrap items-center gap-4">
                    <Link
                      href="https://github.com/rpuneet/bc"
                      className="cta-glow group inline-flex h-12 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-xl active:scale-[0.97]"
                      aria-label="Get started with mycel on GitHub"
                    >
                      Get Started
                      <ArrowRight
                        className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                        aria-hidden="true"
                      />
                    </Link>
                    <Link
                      href="/docs"
                      className="inline-flex h-12 items-center gap-2 rounded-lg border border-border px-8 text-sm font-medium transition-colors hover:bg-accent/20 active:scale-[0.97]"
                      aria-label="Explore the mycel CLI documentation"
                    >
                      Explore the Docs
                    </Link>
                  </div>
                </div>
                <TerminalWindow
                  title="quickstart"
                  className="min-w-[280px]"
                  ariaLabel="Quick start commands: bc init, bc daemon start, bc agent create"
                >
                  <div className="space-y-1.5 text-[13px]">
                    <div>
                      <span className="text-terminal-prompt">$ </span>bc init
                    </div>
                    <div>
                      <span className="text-terminal-prompt">$ </span>bc daemon
                      start
                    </div>
                    <div>
                      <span className="text-terminal-prompt">$ </span>bc agent
                      create eng-01 --role engineer --tool claude
                    </div>
                    <div className="terminal-cursor text-terminal-comment mt-3 text-[12px]">
                      # That&apos;s it. Your agent team is running.
                    </div>
                  </div>
                </TerminalWindow>
              </div>
            </div>
          </RevealSection>
        </div>

        <Footer />
      </div>
    </main>
  );
}
