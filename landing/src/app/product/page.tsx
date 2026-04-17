"use client";

import Image from "next/image";
import Link from "next/link";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import { TerminalWindow } from "../_components/TerminalWindow";
import {
  RevealSection,
  FadeUp,
  SlideIn,
  StaggerChildren,
  StaggerItem,
} from "../_components/Motion";
import { Check, X, ArrowRight } from "lucide-react";

/* ── Feature data ─────────────────────────────────────────────── */

interface Feature {
  title: string;
  description: string;
  bullets: string[];
  screenshot: string;
  screenshotAlt: string;
  imageFirst: boolean;
}

const features: Feature[] = [
  {
    title: "Agent Management",
    description:
      "Spawn agents with any provider. Create isolated worktrees with git. Monitor state and progress in real-time.",
    bullets: [
      "4 providers: Claude Code, Gemini, Cursor, Codex",
      "Isolated git worktrees \u2014 zero merge conflicts",
      "Real-time state: working, idle, done, stuck",
    ],
    screenshot: "/screenshots/dashboard-02-agents.png",
    screenshotAlt: "Agent management dashboard showing agent list with status",
    imageFirst: false,
  },
  {
    title: "Channels & Communication",
    description:
      "SQLite-backed messaging between agents. Slack, Telegram, Discord gateways.",
    bullets: [
      "Persistent message history",
      "Reactions and threading",
      "Gateway integrations for external platforms",
    ],
    screenshot: "/screenshots/dashboard-03-channels.png",
    screenshotAlt: "Channels view showing inter-agent messaging",
    imageFirst: true,
  },
  {
    title: "Cost Tracking & Budgets",
    description:
      "Monitor token usage with per-agent breakdowns. Set budget caps to prevent overspending.",
    bullets: [
      "Per-agent, per-model breakdown",
      "Daily cost trends",
      "Budget enforcement",
    ],
    screenshot: "/screenshots/dashboard-04-costs.png",
    screenshotAlt: "Cost tracking dashboard with per-agent breakdowns",
    imageFirst: false,
  },
  {
    title: "Web Dashboard (15+ views)",
    description:
      "A comprehensive web interface at localhost:9374 with real-time SSE updates.",
    bullets: [
      "Agents, Channels, Costs, Roles, Tools, MCP, Cron, Secrets, Stats",
      "Dark/light theme",
      "Live event streaming",
    ],
    screenshot: "/screenshots/dashboard-10-stats-loaded.png",
    screenshotAlt: "Stats dashboard showing system overview and metrics",
    imageFirst: true,
  },
  {
    title: "MCP Integration",
    description:
      "Agents connect via Model Context Protocol. 6 built-in tools.",
    bullets: [
      "send_message, send_file, list_channels, read_channel",
      "Register external MCP servers",
      "bc itself serves as MCP server for agents",
    ],
    screenshot: "/screenshots/dashboard-07-mcp.png",
    screenshotAlt: "MCP integration view showing configured servers",
    imageFirst: false,
  },
  {
    title: "Cron Scheduling",
    description: "Schedule agent workflows with cron expressions.",
    bullets: [
      "Recurring task execution",
      "Execution history and logs",
      "Flexible scheduling",
    ],
    screenshot: "/screenshots/dashboard-08-cron.png",
    screenshotAlt: "Cron scheduling view showing scheduled jobs",
    imageFirst: true,
  },
];

/* ── Comparison table data ────────────────────────────────────── */

interface ComparisonRow {
  feature: string;
  mycel: string;
  manual: string;
  other: string;
}

const comparisonRows: ComparisonRow[] = [
  { feature: "Worktree isolation", mycel: "yes", manual: "no", other: "no" },
  { feature: "Multi-provider", mycel: "yes (4)", manual: "no", other: "Partial" },
  { feature: "Cost tracking", mycel: "yes", manual: "no", other: "no" },
  { feature: "Inter-agent comms", mycel: "yes", manual: "no", other: "no" },
  { feature: "Gateway integrations", mycel: "yes (3)", manual: "no", other: "no" },
  { feature: "Web dashboard", mycel: "yes (15+ views)", manual: "no", other: "Partial" },
  { feature: "Open source", mycel: "yes MIT", manual: "N/A", other: "Varies" },
];

function CellValue({ value }: { value: string }) {
  if (value.startsWith("yes")) {
    const extra = value.length > 3 ? value.slice(3) : "";
    return (
      <span className="flex items-center gap-1.5 text-primary font-label text-sm">
        <Check className="h-4 w-4 shrink-0" />
        <span>{extra ? extra.trim() : ""}</span>
      </span>
    );
  }
  if (value === "no") {
    return (
      <span className="flex items-center gap-1.5 text-red-400 font-label text-sm">
        <X className="h-4 w-4 shrink-0" />
      </span>
    );
  }
  return (
    <span className="text-on-surface-variant font-label text-sm">{value}</span>
  );
}

/* ── Feature section component ────────────────────────────────── */

function FeatureSection({ feature }: { feature: Feature }) {
  const { title, description, bullets, screenshot, screenshotAlt, imageFirst } =
    feature;

  const textBlock = (
    <SlideIn direction={imageFirst ? "right" : "left"}>
      <div>
        <h3 className="font-headline text-2xl lg:text-3xl font-bold text-on-background tracking-tight">
          {title}
        </h3>
        <p className="mt-4 text-on-surface-variant font-body text-base leading-relaxed max-w-lg">
          {description}
        </p>
        <ul className="mt-6 space-y-3">
          {bullets.map((bullet) => (
            <li key={bullet} className="flex items-start gap-3">
              <Check className="h-5 w-5 text-primary shrink-0 mt-0.5" />
              <span className="text-on-background font-body text-sm leading-relaxed">
                {bullet}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </SlideIn>
  );

  const imageBlock = (
    <SlideIn direction={imageFirst ? "left" : "right"}>
      <div className="glass-panel rounded-sm overflow-hidden terminal-glow">
        <Image
          src={screenshot}
          alt={screenshotAlt}
          width={1280}
          height={800}
          className="w-full h-auto"
          loading="lazy"
        />
      </div>
    </SlideIn>
  );

  return (
    <RevealSection className="py-24 px-6">
      <div className="mx-auto max-w-screen-xl grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
        {imageFirst ? (
          <>
            {imageBlock}
            {textBlock}
          </>
        ) : (
          <>
            {textBlock}
            {imageBlock}
          </>
        )}
      </div>
    </RevealSection>
  );
}

/* ── Main page ────────────────────────────────────────────────── */

export default function Product() {
  return (
    <main className="min-h-screen bg-background overflow-x-hidden">
      <Nav />

      {/* ═══ Section 1: Hero ═══ */}
      <section className="hero-glow py-24 lg:py-32">
        <div className="mx-auto max-w-screen-xl px-6 text-center">
          <FadeUp>
            <h1 className="font-headline text-4xl lg:text-6xl font-bold text-on-background tracking-tight leading-tight">
              The complete platform for
              <br />
              multi-agent orchestration
            </h1>
          </FadeUp>
          <FadeUp delay={0.15}>
            <p className="mx-auto mt-6 max-w-2xl text-on-surface-variant font-body text-lg leading-relaxed">
              Everything you need to coordinate AI agents on real codebases.
            </p>
          </FadeUp>

          {/* Hero screenshot */}
          <FadeUp delay={0.3}>
            <div className="mx-auto mt-16 max-w-5xl glass-panel rounded-sm overflow-hidden terminal-glow">
              <Image
                src="/screenshots/dashboard-01-home.png"
                alt="mycel dashboard home view showing workspace overview"
                width={1920}
                height={1080}
                className="w-full h-auto"
                priority
              />
            </div>
          </FadeUp>
        </div>
      </section>

      {/* ═══ Section 2: Feature Sections ═══ */}
      {features.map((feature) => (
        <FeatureSection key={feature.title} feature={feature} />
      ))}

      {/* ═══ Section 3: Comparison Table ═══ */}
      <RevealSection className="py-24 bg-surface-container-low">
        <div className="mx-auto max-w-screen-xl px-6">
          <FadeUp>
            <h2 className="font-headline text-3xl lg:text-4xl font-bold text-on-background tracking-tight text-center mb-16">
              Why mycel?
            </h2>
          </FadeUp>

          <FadeUp delay={0.15}>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-outline-variant/20">
                    <th className="py-4 px-6 text-left font-label text-xs uppercase tracking-widest text-on-surface-variant">
                      Feature
                    </th>
                    <th className="py-4 px-6 text-left font-label text-xs uppercase tracking-widest text-primary">
                      mycel
                    </th>
                    <th className="py-4 px-6 text-left font-label text-xs uppercase tracking-widest text-on-surface-variant">
                      Manual
                    </th>
                    <th className="py-4 px-6 text-left font-label text-xs uppercase tracking-widest text-on-surface-variant">
                      Other tools
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {comparisonRows.map((row, i) => (
                    <tr
                      key={row.feature}
                      className={
                        i % 2 === 0
                          ? "bg-surface-container/50"
                          : "bg-surface-container-low"
                      }
                    >
                      <td className="py-4 px-6 font-body text-sm text-on-background">
                        {row.feature}
                      </td>
                      <td className="py-4 px-6">
                        <CellValue value={row.mycel} />
                      </td>
                      <td className="py-4 px-6">
                        <CellValue value={row.manual} />
                      </td>
                      <td className="py-4 px-6">
                        <CellValue value={row.other} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </FadeUp>
        </div>
      </RevealSection>

      {/* ═══ Section 4: CTA ═══ */}
      <RevealSection className="py-32">
        <div className="mx-auto max-w-screen-xl px-6 text-center">
          <h2 className="font-headline text-3xl lg:text-5xl font-bold text-on-background tracking-tight leading-tight">
            <span className="accent-instrument">Intelligence is orchestration.</span>
            <br />
            Start building today.
          </h2>

          <div className="mx-auto mt-12 max-w-xl">
            <TerminalWindow title="quickstart">
              <div className="space-y-1">
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">
                    curl -fsSL https://raw.githubusercontent.com/rpuneet/bc/main/scripts/install.sh | bash
                  </span>
                </div>
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">bc init</span>
                </div>
                <div>
                  <span className="text-terminal-prompt">$ </span>
                  <span className="text-terminal-command">bc up</span>
                </div>
              </div>
            </TerminalWindow>
          </div>

          <FadeUp delay={0.2}>
            <div className="mt-10">
              <Link
                href="/waitlist"
                className="inline-flex h-12 items-center gap-2 rounded-sm bg-primary px-8 font-label text-sm font-semibold text-on-background shadow-lg transition-all hover:shadow-xl hover:shadow-primary/20 active:scale-[0.97]"
              >
                Get Started
                <ArrowRight className="h-4 w-4" />
              </Link>
            </div>
          </FadeUp>
        </div>
      </RevealSection>

      <Footer />
    </main>
  );
}
