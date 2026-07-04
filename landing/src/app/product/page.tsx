"use client";

import Image from "next/image";
import Link from "next/link";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import { RevealSection } from "../_components/TerminalComponents";
import { motion, useInView } from "framer-motion";
import { ArrowRight } from "lucide-react";
import { useRef } from "react";

const fadeUp = {
  hidden: { opacity: 0, y: 30 },
  visible: (i: number) => ({
    opacity: 1,
    y: 0,
    transition: { delay: i * 0.1, duration: 0.6, ease: "easeOut" as const },
  }),
};

const stagger = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.08 } },
};

/* ── Slide-in variants for alternating sections ─────────────────── */

const slideFromLeft = {
  hidden: { opacity: 0, x: -60 },
  visible: {
    opacity: 1,
    x: 0,
    transition: { duration: 0.7, ease: "easeOut" as const },
  },
};

const slideFromRight = {
  hidden: { opacity: 0, x: 60 },
  visible: {
    opacity: 1,
    x: 0,
    transition: { duration: 0.7, ease: "easeOut" as const },
  },
};

/* ── Reusable screenshot frame ─────────────────────────────────────── */

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
      className={`group relative overflow-hidden rounded-xl border border-outline-variant/20 shadow-2xl transition-transform duration-500 ease-out hover:scale-[1.02] ${className}`}
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

/* ── CLI command display ───────────────────────────────────────────── */

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

/* ── Feature section wrapper (alternating layout with scroll animations) */

function FeatureSection({
  id,
  label,
  title,
  description,
  commands,
  screenshot,
  screenshotAlt,
  docsLink,
  imageFirst = false,
}: {
  id: string;
  label: string;
  title: string;
  description: string;
  commands: string[];
  screenshot: string;
  screenshotAlt: string;
  docsLink?: string;
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
      <span className="inline-block font-label text-[11px] font-bold uppercase tracking-[0.25em] text-primary/80 border-b border-primary/20 pb-1">
        {label}
      </span>
      <h2 className="mt-5 text-3xl font-bold tracking-tight sm:text-4xl leading-[1.15] font-headline text-on-background">
        {title}
      </h2>
      <p className="mt-5 text-on-surface-variant leading-[1.8] text-[15px] font-body">
        {description}
        {docsLink && (
          <>
            {" "}
            <Link
              href={docsLink}
              className="text-on-background underline underline-offset-4 hover:text-primary transition-colors"
            >
              See docs &rarr;
            </Link>
          </>
        )}
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
      className="py-14 lg:py-16 border-t border-outline-variant/20"
      id={id}
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

/* ── Comparison card colors ─────────────────────────────────────── */

const cardAccents = [
  { border: "hover:border-primary/40", glow: "hover:shadow-[0_0_30px_rgba(234,88,12,0.08)]" },
  { border: "hover:border-[var(--info)]/30", glow: "hover:shadow-[0_0_30px_rgba(56,189,248,0.06)]" },
  { border: "hover:border-[var(--success)]/30", glow: "hover:shadow-[0_0_30px_rgba(34,197,94,0.06)]" },
];

export default function Product() {
  return (
    <main className="min-h-screen selection:bg-primary/20 selection:text-on-background overflow-x-hidden">
      <div className="pointer-events-none fixed inset-0 bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.04),transparent)] dark:bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.08),transparent)]" />

      <Nav />

      {/* ═══════════════════ HERO ═══════════════════ */}
      <motion.section
        initial="hidden"
        animate="visible"
        variants={stagger}
        className="relative px-6 py-14 lg:py-16 text-center"
      >
        {/* Hero glow */}
        <div className="pointer-events-none absolute inset-x-0 top-0 h-[600px] bg-[radial-gradient(ellipse_60%_40%_at_50%_20%,rgba(234,88,12,0.1),transparent)]" />

        <motion.div variants={fadeUp} custom={0} className="relative mx-auto max-w-4xl">
          <span className="inline-block font-label text-[11px] font-bold uppercase tracking-[0.3em] text-primary border border-primary/20 rounded-full px-4 py-1.5 mb-6">
            Product
          </span>
          <h1 className="text-4xl font-bold tracking-tight sm:text-6xl lg:text-7xl leading-[1.05] font-headline text-on-background">
            The complete platform for
            <br />
            <span className="text-on-surface-variant/60">
              multi-agent orchestration.
            </span>
          </h1>
          <p className="mx-auto mt-8 max-w-2xl text-lg text-on-surface-variant leading-relaxed font-body">
            Agents, channels, roles, cost tracking, secrets, and cron jobs.
            Everything you need to run parallel AI agents.
          </p>
        </motion.div>

        {/* Hero dashboard screenshot */}
        <motion.div
          variants={fadeUp}
          custom={1}
          className="relative mx-auto mt-12 max-w-5xl"
        >
          <ScreenshotFrame
            src="/screenshots/dashboard-01-home.png"
            alt="mycel Web UI dashboard showing workspace overview with agent status, channels, and system health"
          />
        </motion.div>
      </motion.section>

      <div className="mx-auto max-w-7xl px-6">
        {/* ═══════════════════ AGENT LIFECYCLE ═══════════════════ */}
        <FeatureSection
          id="agents"
          label="Agent Lifecycle"
          title="Create, command, observe, stop."
          description="Spawn an agent in one command. It gets its own tmux session, git worktree, and role. Send it work, peek at its output, or stop it — all from the CLI."
          commands={[
            'mycel agent create eng-01 --role engineer --tool claude',
            'mycel agent send eng-01 "Build the auth module"',
            'mycel agent peek eng-01',
            'mycel agent list',
          ]}
          screenshot="/screenshots/dashboard-02-agents.png"
          screenshotAlt="mycel Agents view showing a table of agents with their roles, tools, states, and tasks"
          docsLink="/docs#commands"
        />

        {/* ═══════════════════ COMMUNICATION ═══════════════════ */}
        <FeatureSection
          id="channels"
          label="Communication"
          title="Structured coordination via channels."
          description="Agents @mention each other, hand off work, and converge — without going through you. Every message is logged and searchable."
          commands={[
            'mycel channel create deploys',
            'mycel channel send engineering "@eng-01 review PR #42"',
            'mycel channel history engineering --last 20',
          ]}
          screenshot="/screenshots/dashboard-03-channels.png"
          screenshotAlt="mycel Channels view showing inter-agent communication with message history and reactions"
          docsLink="/docs#commands"
          imageFirst
        />

        {/* ═══════════════════ COST TRACKING ═══════════════════ */}
        <FeatureSection
          id="costs"
          label="Cost Tracking"
          title="Track spending across every agent."
          description="See total cost and per-agent breakdowns in real time. Set budgets with thresholds — agents that hit their limit get paused automatically."
          commands={[
            'mycel cost show',
            'mycel cost budget set 50.00 --agent eng-01 --alert-at 0.8',
            'mycel cost usage',
          ]}
          screenshot="/screenshots/dashboard-04-costs.png"
          screenshotAlt="mycel Costs dashboard showing total spend, daily cost trends, and per-agent cost breakdown with bar chart"
          docsLink="/docs#commands"
        />

        {/* ═══════════════════ WEB DASHBOARD ═══════════════════ */}
        <FeatureSection
          id="dashboard"
          label="Web Dashboard"
          title="15+ dashboard views, out of the box."
          description="Agents, channels, costs, roles, tools, secrets, logs, cron jobs, and system health — all at localhost:9374. Roles define permissions as markdown. Tools are mix-and-match. Secrets are encrypted at rest."
          commands={[
            'mycel up',
            'mycel status',
          ]}
          screenshot="/screenshots/dashboard-05-roles.png"
          screenshotAlt="mycel Web Dashboard showing roles, tools, secrets, logs, and system health views"
          imageFirst
        />

        {/* ═══════════════════ MCP INTEGRATION ═══════════════════ */}
        <FeatureSection
          id="mcp"
          label="MCP Integration"
          title="Connect tools natively via MCP."
          description="Add MCP servers once, and agents connect on spawn. Supports stdio and SSE transport. Attach servers to roles so every agent gets the right capabilities."
          commands={[
            'mycel mcp add github-server',
            'mycel mcp list',
            'mycel mcp enable github-server',
          ]}
          screenshot="/screenshots/dashboard-07-mcp.png"
          screenshotAlt="mycel MCP view showing configured MCP servers with transport type and connection status"
          imageFirst
        />

        {/* ═══════════════════ CRON JOBS ═══════════════════ */}
        <FeatureSection
          id="cron"
          label="Scheduled Tasks"
          title="Cron-powered automation."
          description="Run lints every morning. Deploy previews on push. Schedule any prompt to any agent with cron syntax, and review the execution history."
          commands={[
            'mycel cron add daily-lint --schedule "0 9 * * *" --agent qa-01 --prompt "Run make lint"',
            'mycel cron list',
            'mycel cron logs daily-lint --last 10',
          ]}
          screenshot="/screenshots/dashboard-08-cron.png"
          screenshotAlt="mycel Cron Jobs view showing scheduled jobs with their schedules, run counts, and last execution times"
        />

        {/* ═══════════════════ WHY BC ═══════════════════ */}
        <RevealSection
          className="py-14 lg:py-16 border-t border-outline-variant/20"
          id="why-bc"
        >
          <div className="mb-10">
            <span className="inline-block font-label text-[11px] font-bold uppercase tracking-[0.25em] text-primary/80 border-b border-primary/20 pb-1">
              Why mycel?
            </span>
            <h2 className="mt-5 text-3xl font-bold tracking-tight sm:text-4xl leading-[1.15] font-headline text-on-background">
              Not a new IDE. Not a framework. An orchestration layer.
            </h2>
            <p className="mt-5 max-w-2xl text-on-surface-variant leading-[1.8] text-[15px] font-body">
              mycel coordinates your existing tools. Keep using Claude Code,
              Cursor, Codex, or any CLI agent. mycel handles the multi-agent
              complexity.
            </p>
          </div>
          <div className="grid gap-6 sm:grid-cols-3">
            {[
              {
                title: "vs. Manual setup",
                desc: "Running Claude Code or Cursor one agent at a time is fine until you need five. mycel runs many in parallel, each on its own branch, with zero conflicts.",
              },
              {
                title: "vs. Custom scripts",
                desc: "Shell scripts break when you add the fifth agent. mycel gives you structured channels, cost tracking, and role-based control out of the box.",
              },
              {
                title: "vs. Agent frameworks",
                desc: "CrewAI and LangGraph require you to build agents from scratch in Python. mycel orchestrates the tools you already use. No SDK, no code changes.",
              },
            ].map((item, i) => (
              <div
                key={item.title}
                className={`rounded-xl border border-outline-variant/20 bg-surface-container p-7 transition-all duration-300 cursor-default ${cardAccents[i].border} ${cardAccents[i].glow}`}
              >
                <h3 className="font-headline font-semibold text-sm mb-3 text-on-background">{item.title}</h3>
                <p className="text-sm text-on-surface-variant leading-[1.8] font-body">
                  {item.desc}
                </p>
              </div>
            ))}
          </div>
        </RevealSection>

        {/* ═══════════════════ CTA ═══════════════════ */}
        <RevealSection className="py-14 lg:py-16">
          <div
            className="relative overflow-hidden rounded-2xl border border-outline-variant/20 bg-surface-container p-10 sm:p-14 text-center"
            style={{
              boxShadow:
                "0 0 80px rgba(234, 88, 12, 0.06), 0 25px 50px -12px rgba(0, 0, 0, 0.3)",
            }}
          >
            {/* CTA glow */}
            <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_50%_50%_at_50%_0%,rgba(234,88,12,0.08),transparent)]" />

            <div className="relative">
              <h2 className="text-3xl font-bold tracking-tight sm:text-4xl lg:text-5xl font-headline text-on-background">
                Start orchestrating in 60 seconds.
              </h2>
              <p className="mx-auto mt-5 max-w-xl text-lg text-on-surface-variant leading-relaxed font-body">
                Install mycel, run three commands, and your agent team is live.
              </p>

              {/* Terminal quickstart */}
              <div className="mx-auto mt-10 max-w-md overflow-hidden rounded-lg border border-outline-variant/20 bg-[var(--terminal-bg)] text-left">
                <div className="flex items-center gap-2 border-b border-outline-variant/10 px-4 py-2">
                  <div className="flex gap-1.5" aria-hidden="true">
                    <span className="h-2 w-2 rounded-full bg-[var(--traffic-red)]" />
                    <span className="h-2 w-2 rounded-full bg-[var(--traffic-yellow)]" />
                    <span className="h-2 w-2 rounded-full bg-[var(--traffic-green)]" />
                  </div>
                </div>
                <div className="p-4 font-label text-[13px] leading-relaxed text-[var(--terminal-text)]">
                  <div>
                    <span className="text-[var(--terminal-prompt)]">$ </span>
                    <span className="text-[var(--terminal-command)]">curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash</span>
                  </div>
                  <div>
                    <span className="text-[var(--terminal-prompt)]">$ </span>
                    <span className="text-[var(--terminal-command)]">mycel up</span>
                  </div>
                  <div className="mt-1">
                    <span className="text-[var(--terminal-prompt)]">$ </span>
                    <span className="inline-block h-4 w-[7px] bg-[var(--terminal-prompt)] animate-[blink_1s_step-end_infinite] align-middle" />
                  </div>
                </div>
              </div>

              <div className="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
                <Link
                  href="/waitlist"
                  className="group inline-flex h-12 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-lg transition-all hover:shadow-xl hover:shadow-primary/20 active:scale-[0.97]"
                >
                  Request Early Access
                  <ArrowRight
                    className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                    aria-hidden="true"
                  />
                </Link>
                <Link
                  href="/docs"
                  className="inline-flex h-12 items-center gap-2 rounded-lg border border-outline-variant/20 px-8 text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container hover:text-on-surface active:scale-[0.97] font-body"
                  aria-label="Browse the mycel CLI reference documentation"
                >
                  Browse CLI Reference
                </Link>
              </div>
            </div>
          </div>
        </RevealSection>
      </div>

      <Footer />
    </main>
  );
}
