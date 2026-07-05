"use client";

import { useState, useSyncExternalStore, useRef } from "react";
import Link from "next/link";
import Image from "next/image";
import { motion, useInView } from "framer-motion";
import { Copy, Check, ExternalLink, ArrowRight } from "lucide-react";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";
import { InstallSection } from "./_components/InstallSection";
import { TerminalWindow } from "./_components/TerminalComponents";
import { RevealSection, FadeUp } from "./_components/Motion";
import { AnimatedBackground } from "./_components/AnimatedBackground";

/* ── Install commands by platform ── */
const installCommands = {
  macOS: "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
  Linux: "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
  Homebrew: "brew install rpuneet/mycel/mycel",
  Docker: "docker run -p 9374:9374 ghcr.io/rpuneet/mycel mycel up",
} as const;

type Platform = keyof typeof installCommands;

function detectPlatform(ua: string): Platform {
  if (/Mac/i.test(ua)) return "macOS";
  if (/Linux/i.test(ua)) return "Linux";
  return "macOS";
}

/* ── External-store hook for client-only platform detection.
 *    SSR snapshot is "macOS" to match initial render; the client
 *    swaps in the detected platform after hydration. ── */
const subscribePlatform = () => () => {};
const getPlatformSnapshot = (): Platform => detectPlatform(navigator.userAgent);
const getPlatformServerSnapshot = (): Platform => "macOS";

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
      type="button"
      onClick={handleCopy}
      className="shrink-0 rounded p-1.5 text-on-surface-variant hover:text-on-surface transition-colors"
      aria-label="Copy to clipboard"
    >
      {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
    </button>
  );
}

/* ── Slide-in variants for alternating showcase blocks ── */
const slideFromLeft = {
  hidden: { opacity: 0, x: -60 },
  visible: { opacity: 1, x: 0, transition: { duration: 0.7, ease: "easeOut" as const } },
};
const slideFromRight = {
  hidden: { opacity: 0, x: 60 },
  visible: { opacity: 1, x: 0, transition: { duration: 0.7, ease: "easeOut" as const } },
};

/* ── Reusable browser-chrome screenshot frame ── */
function ScreenshotFrame({
  src,
  alt,
  className = "",
}: {
  src: string;
  alt: string;
  className?: string;
}) {
  return (
    <div
      className={`group relative overflow-hidden rounded-xl border border-outline-variant/20 transition-transform duration-500 ease-out hover:scale-[1.01] ${className}`}
      style={{
        boxShadow:
          "0 0 60px rgba(234, 88, 12, 0.06), 0 25px 50px -12px rgba(0, 0, 0, 0.4)",
      }}
    >
      {/* Browser chrome bar */}
      <div className="flex items-center gap-2 border-b border-outline-variant/20 bg-[var(--terminal-header-bg)] px-4 py-2">
        <div className="flex gap-1.5" aria-hidden="true">
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-red)]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-yellow)]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-green)]" />
        </div>
        <div className="mx-auto rounded-md bg-[rgba(255,255,255,0.04)] px-8 py-1 text-[10px] font-label text-[var(--terminal-muted)] tracking-wide">
          localhost:9374
        </div>
        <div className="w-[52px]" />
      </div>
      <Image
        src={src}
        alt={alt}
        width={1280}
        height={800}
        className="w-full h-auto"
        loading="lazy"
      />
    </div>
  );
}

/* ── CLI command display ── */
function CLICommands({ commands }: { commands: string[] }) {
  return (
    <div className="mt-8 overflow-hidden rounded-lg border border-outline-variant/20 bg-[var(--terminal-bg)]">
      <div className="flex items-center gap-2 border-b border-outline-variant/10 px-4 py-2">
        <div className="flex gap-1.5" aria-hidden="true">
          <span className="h-2 w-2 rounded-full bg-[var(--traffic-red)]" />
          <span className="h-2 w-2 rounded-full bg-[var(--traffic-yellow)]" />
          <span className="h-2 w-2 rounded-full bg-[var(--traffic-green)]" />
        </div>
        <span className="ml-2 font-label text-[9px] font-bold uppercase tracking-[0.2em] text-[var(--terminal-muted)]">
          terminal
        </span>
      </div>
      <div className="p-4 space-y-1.5 font-label text-[13px] leading-relaxed text-[var(--terminal-text)]">
        {commands.map((cmd) => (
          <div key={cmd}>
            <span className="text-[var(--terminal-prompt)]">$ </span>
            <span className="text-[var(--terminal-command)]">{cmd}</span>
          </div>
        ))}
        <div className="mt-1">
          <span className="text-[var(--terminal-prompt)]">$ </span>
          <span className="inline-block h-4 w-[7px] bg-[var(--terminal-prompt)] animate-[blink_1s_step-end_infinite] align-middle" />
        </div>
      </div>
    </div>
  );
}

/* ── Showcase block: one principle → one feature → one screenshot ── */
function ShowcaseBlock({
  id,
  number,
  label,
  title,
  description,
  commands,
  screenshot,
  screenshotAlt,
  imageFirst = false,
}: {
  id: string;
  number: string;
  label: string;
  title: string;
  description: string;
  commands: string[];
  screenshot: string;
  screenshotAlt: string;
  imageFirst?: boolean;
}) {
  const ref = useRef(null);
  const inView = useInView(ref, { once: true, margin: "-100px" });

  const textVariant = imageFirst ? slideFromRight : slideFromLeft;
  const imageVariant = imageFirst ? slideFromLeft : slideFromRight;

  const textContent = (
    <motion.div
      variants={textVariant}
      initial="hidden"
      animate={inView ? "visible" : "hidden"}
      className={imageFirst ? "order-1 lg:order-2" : ""}
    >
      <span className="inline-flex items-center gap-2 font-label text-[11px] font-bold uppercase tracking-[0.25em] text-primary/80 border-b border-primary/20 pb-1">
        <span className="text-primary/40">{number}</span>
        {label}
      </span>
      <h3 className="mt-5 text-3xl font-bold tracking-tight sm:text-4xl leading-[1.15] font-headline text-on-background">
        {title}
      </h3>
      <p className="mt-5 text-on-surface-variant leading-[1.8] text-[15px] font-body">
        {description}
      </p>
      <CLICommands commands={commands} />
    </motion.div>
  );

  const imageContent = (
    <motion.div
      variants={imageVariant}
      initial="hidden"
      animate={inView ? "visible" : "hidden"}
      className={imageFirst ? "order-2 lg:order-1" : ""}
    >
      <ScreenshotFrame
        src={screenshot}
        alt={screenshotAlt}
        className="rotate-[0.5deg] hover:rotate-0 transition-transform duration-500"
      />
    </motion.div>
  );

  return (
    <section
      ref={ref}
      id={id}
      className="py-14 lg:py-16 border-t border-outline-variant/20"
    >
      <div className="grid items-center gap-12 lg:grid-cols-2 lg:gap-20">
        {imageFirst ? (
          <>
            {imageContent}
            {textContent}
          </>
        ) : (
          <>
            {textContent}
            {imageContent}
          </>
        )}
      </div>
    </section>
  );
}

export default function Home() {
  const detected = useSyncExternalStore(
    subscribePlatform,
    getPlatformSnapshot,
    getPlatformServerSnapshot,
  );
  const [override, setPlatform] = useState<Platform | null>(null);
  const platform = override ?? detected;

  const tabs: Platform[] = ["macOS", "Linux", "Homebrew", "Docker"];

  return (
    <main className="min-h-screen overflow-x-hidden">
      {/* Spore network background — fixed, covers entire page */}
      <AnimatedBackground />
      {/* Subtle radial gradient overlay */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.10),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* ════════════════════════════════════════
           Section 1: Hero
           ════════════════════════════════════════ */}
        <section className="pt-24 pb-12 sm:pt-32 sm:pb-16">
          <div className="mx-auto max-w-4xl px-4 sm:px-6 text-center">
            <FadeUp>
              <span className="font-label text-xs tracking-[0.15em] uppercase text-primary">
                CLI-first &middot; Agent-agnostic &middot; Open source
              </span>
            </FadeUp>

            <FadeUp delay={0.1}>
              <h1 className="mt-6 text-4xl md:text-5xl lg:text-6xl font-bold tracking-tight text-on-background font-headline leading-tight">
                Orchestrate AI agent teams<br className="hidden sm:block" /> from your terminal.
              </h1>
            </FadeUp>

            <FadeUp delay={0.15}>
              <p className="mt-5 text-lg md:text-xl text-on-surface-variant max-w-2xl mx-auto leading-relaxed font-body">
                Coordinate Claude, Gemini, Codex, and Cursor agents on a single codebase.
                Isolated worktrees. Shared channels. Real-time cost controls.
              </p>
            </FadeUp>

            {/* Install command */}
            <FadeUp delay={0.2}>
              <div className="mt-10 mx-auto max-w-xl">
                {/* Platform tabs */}
                <div className="flex items-center justify-center gap-1 mb-3">
                  {tabs.map((t) => (
                    <button
                      key={t}
                      type="button"
                      onClick={() => setPlatform(t)}
                      className={`px-3 py-1.5 rounded text-xs font-label font-medium transition-colors ${
                        platform === t
                          ? "bg-surface-container-high text-on-surface"
                          : "text-on-surface-variant hover:text-on-surface"
                      }`}
                    >
                      {t}
                    </button>
                  ))}
                </div>

                {/* Command box */}
                <div className="flex items-center gap-2 rounded-lg border border-outline-variant/30 bg-surface-container px-4 py-3 shadow-[0_0_60px_rgba(234,88,12,0.08),0_0_20px_rgba(234,88,12,0.04)]">
                  <span className="text-on-surface-variant select-none font-label">$</span>
                  <code className="flex-1 text-sm text-on-surface overflow-x-auto whitespace-nowrap scrollbar-none font-label">
                    {installCommands[platform]}
                  </code>
                  <CopyButton text={installCommands[platform]} />
                </div>

                {/* Then run */}
                <p className="mt-3 text-sm text-on-surface-variant font-body">
                  Then run:{" "}
                  <code className="text-primary font-label">mycel up</code>
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
                    href="https://github.com/rpuneet/mycel"
                    className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container hover:border-primary/30 hover:text-primary active:scale-[0.97] font-body"
                  >
                    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                    </svg>
                    GitHub
                  </Link>
                </div>
                {/* GitHub badges */}
                <div className="flex items-center gap-2 flex-wrap justify-center">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/stars/rpuneet/mycel?style=flat-square&color=ea580c&labelColor=1e1b18" alt="GitHub stars" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/license/rpuneet/mycel?style=flat-square&color=ea580c&labelColor=1e1b18" alt="License" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/last-commit/rpuneet/mycel?style=flat-square&color=ea580c&labelColor=1e1b18" alt="Last commit" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/go-mod/go-version/rpuneet/mycel?style=flat-square&color=ea580c&labelColor=1e1b18" alt="Go version" className="h-5" />
                </div>
              </div>
            </FadeUp>
          </div>
        </section>

        {/* Section separator */}
        <div className="mx-auto max-w-5xl px-6"><div className="section-separator" /></div>

        {/* ════════════════════════════════════════
           Section 2: Terminal Demo
           ════════════════════════════════════════ */}
        <RevealSection className="py-12 sm:py-16">
          <div className="mx-auto max-w-3xl px-4 sm:px-6">
            <TerminalWindow title="terminal" className="terminal-glow">
              <div className="space-y-3 text-[13px] leading-7">
                <div>
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel up</span>
                </div>
                <div className="text-terminal-success">&#10003; Workspace bootstrapped (~/.mycel)</div>
                <div className="text-terminal-muted">
                  Server running on <span className="text-primary">http://localhost:9374</span>
                </div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel agent create eng-01 --role engineer --tool claude</span>
                </div>
                <div className="text-terminal-muted">Created worktree .mycel/agents/eng-01/worktree</div>
                <div className="text-terminal-muted">
                  Agent <span className="text-primary">eng-01</span> is online.
                </div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel status</span>
                </div>
                <div className="text-terminal-comment mt-1 font-label">
                  <div className="text-primary/70">AGENT     ROLE       STATE     UPTIME</div>
                  <div className="text-terminal-text">eng-01    engineer   <span className="text-terminal-success">working</span>   2m</div>
                  <div className="text-terminal-text">eng-02    engineer   <span className="text-terminal-success">working</span>   1m</div>
                </div>
                <div className="inline-block w-2 h-[18px] bg-primary/80 animate-pulse" />
              </div>
            </TerminalWindow>
          </div>
        </RevealSection>

        {/* Section separator */}
        <div className="mx-auto max-w-5xl px-6"><div className="section-separator" /></div>

        {/* ════════════════════════════════════════
           Section 3: Product Showcase — the method,
           six principles paired with real features.
           ════════════════════════════════════════ */}
        <section id="product" className="scroll-mt-24 py-12 sm:py-16">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <FadeUp className="text-center mb-4">
              <span className="font-label text-xs tracking-[0.2em] uppercase text-primary">
                The mycel method
              </span>
              <h2 className="mt-4 text-2xl md:text-4xl font-bold tracking-tight text-on-background font-headline">
                Six principles for running an agent team.
              </h2>
              <p className="mt-4 text-on-surface-variant text-lg max-w-2xl mx-auto font-body">
                Each one is a working feature you can see in the dashboard at
                localhost:9374 &mdash; not a promise.
              </p>
            </FadeUp>

            {/* 01 · Isolation → agent detail (worktree + sandbox) */}
            <ShowcaseBlock
              id="isolation"
              number="01"
              label="Isolation"
              title="Every agent in its own sandbox."
              description="Two agents editing the same file will overwrite each other. mycel gives each agent its own git worktree and an isolated tmux or Docker sandbox — so ten agents can work the same codebase with zero merge conflicts. Open any agent to watch its live stream, tools, and cost."
              commands={[
                "mycel agent create eng-01 --role engineer --tool claude",
                'mycel agent send eng-01 "Build the auth module"',
                "mycel agent peek eng-01",
              ]}
              screenshot="/screenshots/agent-detail.png"
              screenshotAlt="mycel agent detail view showing the agent header, tabs, and a live output stream"
            />

            {/* 02 · Communication → notifications hub */}
            <ShowcaseBlock
              id="communication"
              number="02"
              label="Communication"
              title="Coordinate without being the bottleneck."
              description="Agents @mention each other across structured channels to hand off work and converge — every message logged and searchable. Wire in Slack, Telegram, Discord, or WhatsApp, and your agents can reach you (and reply) straight from your phone."
              commands={[
                'mycel channel send engineering "@eng-01 review PR #42"',
                "mycel notify connect slack",
                "mycel channel history engineering --last 20",
              ]}
              screenshot="/screenshots/notifications.png"
              screenshotAlt="mycel notifications hub showing connected apps, channels, and recent activity"
              imageFirst
            />

            {/* 03 · Visibility → Insights cost dashboard (standout) */}
            <ShowcaseBlock
              id="visibility"
              number="03"
              label="Visibility"
              title="Real cost and tokens, in real time."
              description="You can't trust what you can't see. The Insights dashboard reads straight from the cost ledger — live spend, token throughput, burn rate, active agents, and your top cost driver — broken down per agent and per model. Set budgets with hard stops and agents pause themselves when they hit the limit."
              commands={[
                "mycel cost show",
                "mycel cost budget set 50.00 --agent eng-01 --alert-at 0.8",
                "mycel cost usage",
              ]}
              screenshot="/screenshots/insights.png"
              screenshotAlt="mycel Insights dashboard showing spend, tokens, active agents, burn rate, top cost driver, and a per-agent cost table"
            />

            {/* 04 · Control → agents fleet table */}
            <ShowcaseBlock
              id="control"
              number="04"
              label="Control"
              title="Create, command, observe, stop."
              description="Spawn an agent in one command, send it work, watch its output, or stop it — all from the CLI or the browser. The Agents table shows every agent's state, provider, and cost at a glance, and role permissions scope exactly what each one can touch."
              commands={[
                "mycel agent list",
                'mycel agent send eng-01 "Ship the fix"',
                "mycel agent stop eng-01",
              ]}
              screenshot="/screenshots/agents.png"
              screenshotAlt="mycel Agents table showing agents with their states, providers, and costs"
              imageFirst
            />

            {/* 05 · Persistence → encrypted secrets */}
            <ShowcaseBlock
              id="persistence"
              number="05"
              label="Persistence"
              title="State that survives restarts."
              description="Agents crash, machines reboot — mycel doesn't lose your work. Everything lives under ~/.mycel: worktrees, channel history, cost records, and per-agent config. Secrets are encrypted at rest and injected as MYCEL_ environment variables when an agent spawns."
              commands={[
                "mycel secret set OPENAI_API_KEY",
                "mycel secret list",
                "mycel agent start eng-01 --resume",
              ]}
              screenshot="/screenshots/secrets.png"
              screenshotAlt="mycel encrypted secrets manager showing stored secrets and their scopes"
            />

            {/* 06 · Simplicity → providers / tools */}
            <ShowcaseBlock
              id="simplicity"
              number="06"
              label="Simplicity"
              title="One binary. Many providers."
              description="No runtime dependencies, no cloud account, no YAML. One Go binary and two commands stand up your agent team. Mix providers on the same project — Claude, Gemini, Codex, Cursor, Aider — and switch anytime. Build anything with make <verb>-<runtime>-<component>."
              commands={[
                "brew install rpuneet/mycel/mycel",
                "mycel up",
                "mycel agent create eng-01 --tool gemini",
              ]}
              screenshot="/screenshots/tools.png"
              screenshotAlt="mycel providers and tools page showing available AI providers and their tools"
              imageFirst
            />
          </div>
        </section>

        {/* Section separator */}
        <div className="mx-auto max-w-5xl px-6"><div className="section-separator" /></div>

        {/* ════════════════════════════════════════
           Section 4: Install
           ════════════════════════════════════════ */}
        <InstallSection />

        {/* ════════════════════════════════════════
           Section 5: Open Source CTA
           ════════════════════════════════════════ */}
        <RevealSection className="py-12 sm:py-16">
          <div className="mx-auto max-w-3xl px-4 sm:px-6 text-center">
            <h2 className="text-2xl md:text-4xl font-bold tracking-tight text-on-background font-headline">
              Free. Open source. No cloud required.
            </h2>
            <p className="mt-3 text-on-surface-variant text-lg font-body">
              MIT licensed. Run it on your machine.
            </p>
            <div className="mt-8 flex flex-col sm:flex-row items-center justify-center gap-4">
              <Link
                href="https://github.com/rpuneet/mycel"
                className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-[0_0_20px_rgba(234,88,12,0.3)] active:scale-[0.97]"
              >
                <svg className="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                </svg>
                View on GitHub
              </Link>
              <Link
                href="/docs"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-8 text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container hover:text-on-surface active:scale-[0.97] font-body"
              >
                Browse CLI Reference
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
