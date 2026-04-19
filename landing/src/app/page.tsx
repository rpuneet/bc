"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { GitBranch, MessageSquare, DollarSign, Layers, Copy, Check, ExternalLink } from "lucide-react";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";
import {
  TerminalWindow,
} from "./_components/TerminalComponents";
import {
  RevealSection,
  FadeUp,
  StaggerChildren,
  StaggerItem,
} from "./_components/Motion";
import { AnimatedBackground } from "./_components/AnimatedBackground";

/* ── Install commands by platform ── */
const installCommands = {
  macOS: "curl -fsSL https://raw.githubusercontent.com/rpuneet/bc/main/scripts/install.sh | bash",
  Linux: "curl -fsSL https://raw.githubusercontent.com/rpuneet/bc/main/scripts/install.sh | bash",
  Homebrew: "brew install rpuneet/bc/bc",
  Docker: "docker run -p 9374:9374 ghcr.io/rpuneet/bc bc up",
} as const;

type Platform = keyof typeof installCommands;

function detectPlatform(ua: string): Platform {
  if (/Mac/i.test(ua)) return "macOS";
  if (/Linux/i.test(ua)) return "Linux";
  return "macOS"; // default
}

/* ── Copy button ── */
function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={handleCopy}
      className="shrink-0 rounded p-1.5 text-muted-foreground hover:text-on-surface transition-colors"
      aria-label="Copy to clipboard"
    >
      {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
    </button>
  );
}

/* ── Feature cards data ── */
const features = [
  {
    icon: GitBranch,
    title: "Git Worktrees",
    desc: "Every agent works in its own git branch. Ten agents, one repo, zero merge conflicts.",
  },
  {
    icon: MessageSquare,
    title: "Channels",
    desc: "Agents coordinate through persistent, searchable channels \u2014 like Slack for your AI team.",
  },
  {
    icon: DollarSign,
    title: "Cost Controls",
    desc: "Per-agent budgets with hard stops. Know exactly what each agent costs in real time.",
  },
  {
    icon: Layers,
    title: "Multi-Provider",
    desc: "Claude Code, Gemini, Cursor, Codex. Mix providers on the same project. Switch anytime.",
  },
];

export default function Home() {
  const [platform, setPlatform] = useState<Platform>("macOS");

  useEffect(() => {
    setPlatform(detectPlatform(navigator.userAgent));
  }, []);

  const tabs: Platform[] = ["macOS", "Linux", "Homebrew", "Docker"];

  return (
    <main className="min-h-screen overflow-x-hidden">
      {/* Mycelium-themed animated background */}
      <AnimatedBackground />
      {/* Subtle gradient overlay */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.06),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* ════════════════════════════════════════
           Section 1: Hero
           ════════════════════════════════════════ */}
        <section className="hero-glow pt-28 pb-16 sm:pt-36 sm:pb-20 lg:pt-44 lg:pb-24">
          <div className="mx-auto max-w-4xl px-4 sm:px-6 text-center">
            <FadeUp>
              <span className="font-[var(--font-label)] text-xs tracking-[0.15em] uppercase text-primary-text">
                CLI-first &middot; Agent-agnostic &middot; Open source
              </span>
            </FadeUp>

            <FadeUp delay={0.1}>
              <h1 className="mt-6 text-4xl lg:text-6xl font-bold tracking-tight text-on-background" style={{ fontFamily: "var(--font-headline)" }}>
                Orchestrate AI agent teams from your terminal.
              </h1>
            </FadeUp>

            <FadeUp delay={0.15}>
              <p className="mt-5 text-lg text-stone-400 max-w-2xl mx-auto leading-relaxed">
                Coordinate Claude, Gemini, and Cursor agents on a single codebase.
                Isolated worktrees. Shared channels. Cost controls.
              </p>
            </FadeUp>

            {/* Install command */}
            <FadeUp delay={0.2}>
              <div className="mt-10 mx-auto max-w-2xl">
                {/* Platform tabs */}
                <div className="flex items-center justify-center gap-1 mb-3">
                  {tabs.map((t) => (
                    <button
                      key={t}
                      onClick={() => setPlatform(t)}
                      className={`px-3 py-1 rounded text-xs font-medium transition-colors ${
                        platform === t
                          ? "bg-surface-container-high text-on-surface"
                          : "text-muted-foreground hover:text-on-surface-variant"
                      }`}
                      style={{ fontFamily: "var(--font-label)" }}
                    >
                      {t}
                    </button>
                  ))}
                </div>

                {/* Command box */}
                <div className="flex items-center gap-2 rounded-lg border border-outline-variant/20 bg-surface-container-low px-4 py-3">
                  <span className="text-muted-foreground select-none" style={{ fontFamily: "var(--font-label)" }}>$</span>
                  <code
                    className="flex-1 text-sm text-on-surface overflow-x-auto whitespace-nowrap scrollbar-none"
                    style={{ fontFamily: "var(--font-label)" }}
                  >
                    {installCommands[platform]}
                  </code>
                  <CopyButton text={installCommands[platform]} />
                </div>

                {/* Then run */}
                <p className="mt-3 text-sm text-muted-foreground">
                  Then run:{" "}
                  <code className="text-on-surface-variant" style={{ fontFamily: "var(--font-label)" }}>
                    bc init && bc up
                  </code>
                </p>
              </div>
            </FadeUp>

            {/* CTA buttons */}
            <FadeUp delay={0.25}>
              <div className="mt-8 flex flex-col items-center gap-4">
                <div className="flex items-center gap-4">
                  <Link
                    href="/docs"
                    className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-lg active:scale-[0.97]"
                  >
                    View Docs
                    <ExternalLink className="h-3.5 w-3.5" />
                  </Link>
                  <Link
                    href="https://github.com/rpuneet/bc"
                    className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container active:scale-[0.97]"
                  >
                    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                    </svg>
                    GitHub
                  </Link>
                </div>
                {/* GitHub badges — real-time from shields.io */}
                <div className="flex items-center gap-2">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/stars/rpuneet/bc?style=flat&logo=github&label=Stars&color=ea580c" alt="GitHub stars" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/license/rpuneet/bc?style=flat&color=ea580c" alt="License" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/last-commit/rpuneet/bc?style=flat&label=Last%20commit&color=ea580c" alt="Last commit" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/go-mod/go-version/rpuneet/bc?style=flat&color=ea580c" alt="Go version" className="h-5" />
                </div>
              </div>
            </FadeUp>
          </div>
        </section>

        {/* ════════════════════════════════════════
           Section 2: Quick Demo
           ════════════════════════════════════════ */}
        <RevealSection className="py-12 sm:py-16 lg:py-20">
          <div className="mx-auto max-w-3xl px-4 sm:px-6">
            <TerminalWindow title="mycel" className="terminal-glow">
              <div className="space-y-3 text-[13px]">
                <div>
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">bc init</span>
                </div>
                <div className="text-terminal-success">&#10003; Workspace initialized (.bc/)</div>
                <div className="text-terminal-success">&#10003; Ready to create agents</div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">bc up</span>
                </div>
                <div className="text-terminal-muted">Server running on http://localhost:9374</div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">bc agent create eng-01 --role engineer --tool claude</span>
                </div>
                <div className="text-terminal-muted">Created worktree .bc/agents/eng-01/worktree</div>
                <div className="text-terminal-success">Agent eng-01 is online.</div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">bc agent create eng-02 --role engineer --tool gemini</span>
                </div>
                <div className="text-terminal-muted">Created worktree .bc/agents/eng-02/worktree</div>
                <div className="text-terminal-success">Agent eng-02 is online.</div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">bc status</span>
                </div>
                <div className="text-terminal-comment mt-1" style={{ fontFamily: "var(--font-label)" }}>
                  <div>AGENT     ROLE       STATE     UPTIME</div>
                  <div className="text-terminal-text">eng-01    engineer   working   2m</div>
                  <div className="text-terminal-text">eng-02    engineer   working   1m</div>
                </div>
              </div>
            </TerminalWindow>
          </div>
        </RevealSection>

        {/* ════════════════════════════════════════
           Section 3: Feature Grid
           ════════════════════════════════════════ */}
        <RevealSection className="py-12 sm:py-16 lg:py-24">
          <div className="mx-auto max-w-4xl px-4 sm:px-6">
            <FadeUp>
              <h2
                className="text-2xl font-bold tracking-tight text-on-background text-center mb-10"
                style={{ fontFamily: "var(--font-headline)" }}
              >
                Built for multi-agent workflows
              </h2>
            </FadeUp>

            <StaggerChildren className="grid gap-4 sm:grid-cols-2" stagger={0.08}>
              {features.map((f) => (
                <StaggerItem key={f.title}>
                  <div className="bg-surface-container rounded-lg p-6">
                    <f.icon className="h-5 w-5 text-primary-text mb-3" />
                    <h3
                      className="text-lg font-semibold text-on-surface mb-1.5"
                      style={{ fontFamily: "var(--font-headline)" }}
                    >
                      {f.title}
                    </h3>
                    <p className="text-on-surface-variant text-sm leading-relaxed">
                      {f.desc}
                    </p>
                  </div>
                </StaggerItem>
              ))}
            </StaggerChildren>
          </div>
        </RevealSection>

        {/* ════════════════════════════════════════
           Section 4: Dashboard Screenshot
           ════════════════════════════════════════ */}
        <RevealSection className="py-12 sm:py-16 lg:py-24">
          <div className="mx-auto max-w-5xl px-4 sm:px-6">
            <FadeUp className="text-center mb-10">
              <h2
                className="text-2xl font-bold tracking-tight text-on-background"
                style={{ fontFamily: "var(--font-headline)" }}
              >
                Real-time visibility.
              </h2>
              <p className="mt-3 text-on-surface-variant text-base max-w-xl mx-auto">
                A web dashboard at localhost:9374 shows every agent&apos;s state, costs, and output.
              </p>
            </FadeUp>

            <FadeUp delay={0.1}>
              <div className="glass-panel terminal-glow rounded-xl overflow-hidden">
                <Image
                  src="/screenshots/dashboard-01-home.png"
                  alt="mycel dashboard showing agent status, channels, and costs"
                  width={1920}
                  height={1080}
                  className="w-full h-auto"
                  priority={false}
                />
              </div>
              <div className="text-center mt-5">
                <Link
                  href="/product"
                  className="text-sm text-primary-text hover:text-primary transition-colors inline-flex items-center gap-1"
                >
                  See all 15+ views
                  <ExternalLink className="h-3 w-3" />
                </Link>
              </div>
            </FadeUp>
          </div>
        </RevealSection>

        {/* ════════════════════════════════════════
           Section 5: Open Source CTA
           ════════════════════════════════════════ */}
        <RevealSection className="py-16 sm:py-20 lg:py-28">
          <div className="mx-auto max-w-2xl px-4 sm:px-6 text-center">
            <h2
              className="text-2xl font-bold tracking-tight text-on-background"
              style={{ fontFamily: "var(--font-headline)" }}
            >
              Free. Open source. No cloud required.
            </h2>
            <p className="mt-3 text-on-surface-variant text-base">
              MIT licensed. Run it on your machine. Own your data.
            </p>
            <div className="mt-8">
              <Link
                href="https://github.com/rpuneet/bc"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 text-sm font-medium text-on-surface transition-colors hover:bg-surface-container active:scale-[0.97]"
              >
                <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                </svg>
                View on GitHub
                <ExternalLink className="h-3.5 w-3.5" />
              </Link>
            </div>
          </div>
        </RevealSection>

        <Footer />
      </div>
    </main>
  );
}
