"use client";

import React, { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";

/** ============================================================
 *  Types
 *  ============================================================ */

type Point = { title: string; desc?: string };

interface FeatureSectionProps {
  id: string;
  eyebrow: string;
  headline: string;
  points: Point[];
  tab: "Agents" | "Channels" | "Cron";
  renderFrame: (stepIndex: number) => React.ReactNode;
  cmdForStep?: (stepIndex: number) => string | undefined;
}

/** ============================================================
 *  Modular Atomic Components
 *  ============================================================ */

function Dot({ className }: { className: string }) {
  return <span className={`h-2.5 w-2.5 rounded-full ${className}`} />;
}

function TerminalHeader({ title }: { title: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-terminal-comment px-6 py-4 relative z-10">
      <div className="flex items-center gap-1.5">
        <Dot className="bg-[#FF5F56]" />
        <Dot className="bg-[#FFBD2E]" />
        <Dot className="bg-[#27C93F]" />
        <span className="ml-4 font-mono text-[10px] uppercase tracking-[0.2em] text-terminal-muted font-bold">
          {title}
        </span>
      </div>
      <span className="font-mono text-[10px] text-terminal-comment bg-terminal-bg/50 px-2 py-0.5 rounded uppercase tracking-widest">
        mycel home
      </span>
    </div>
  );
}

function Breadcrumbs({ activeTab }: { activeTab: string }) {
  const tabs = ["Agents", "Channels", "Cron"];
  return (
    <div className="mb-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="font-mono text-[11px] text-terminal-muted flex items-center gap-2">
          <span className="opacity-50">WORKSPACES</span>
          <span className="text-terminal-comment">/</span>
          <span className="text-terminal-text font-bold">mycel-workspace</span>
        </div>
        <div className="flex items-center gap-1 rounded-xl border border-terminal-comment bg-terminal-bg/40 p-1 font-mono">
          {tabs.map((t) => (
            <div
              key={t}
              className={`rounded-lg px-4 py-1.5 text-[10px] uppercase tracking-wider transition-all duration-300 ${
                t === activeTab
                  ? "bg-primary text-primary-foreground font-bold shadow-lg"
                  : "text-terminal-muted hover:text-terminal-text hover:bg-terminal-header/60"
              }`}
            >
              {t}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function CmdLine({ text }: { text: string }) {
  return (
    <div className="font-mono text-[11px] md:text-[13px] leading-relaxed text-terminal-text mb-4 md:mb-6 flex items-center gap-2 md:gap-3 overflow-x-auto break-words">
      <span className="text-[var(--terminal-prompt)] font-bold italic flex-shrink-0">
        ➜
      </span>
      <span className="text-terminal-muted tracking-tight">$ {text}</span>
    </div>
  );
}

/** ============================================================
 *  Layout & Behavior Components
 *  ============================================================ */

function NarrativeBox({
  eyebrow,
  headline,
  activePoint,
  points,
  onPointSelect,
  sectionId,
}: {
  eyebrow: string;
  headline: string;
  activePoint: number;
  points: Point[];
  onPointSelect: (idx: number) => void;
  sectionId: string;
}) {
  return (
    <div className="flex flex-col justify-center py-2 md:py-4">
      <div className="text-[10px] md:text-xs font-bold uppercase tracking-[0.3em] text-primary/40 mb-2 md:mb-3">
        {eyebrow}
      </div>
      <h2 className="text-2xl md:text-4xl lg:text-4xl font-semibold tracking-tight lg:sm:text-5xl leading-[1.05] mb-4 md:mb-8">
        {headline}
      </h2>

      <div className="relative h-40 md:h-48">
        <AnimatePresence mode="wait">
          <motion.div
            key={activePoint}
            initial={{ opacity: 0, x: -20 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: 20 }}
            transition={{ duration: 0.3, ease: "easeInOut" }}
            className="absolute inset-0"
          >
            <div className="group rounded-[1.5rem] md:rounded-[2rem] border border-primary/20 bg-background/80 backdrop-blur-sm shadow-2xl p-4 md:p-6 lg:p-8 relative overflow-hidden h-full flex flex-col justify-center">
              <div className="absolute left-0 top-0 bottom-0 w-1.5 bg-primary" />
              <div className="text-lg md:text-xl font-semibold tracking-tight mb-2 md:mb-3 text-foreground">
                {points[activePoint].title}
              </div>
              {points[activePoint].desc && (
                <div className="text-xs md:text-sm text-muted-foreground leading-relaxed">
                  {points[activePoint].desc}
                </div>
              )}
            </div>
          </motion.div>
        </AnimatePresence>
      </div>

      <div className="mt-6 md:mt-12 flex items-center gap-2 md:gap-3">
        {points.map((_, i) => (
          <button
            key={i}
            onClick={() => onPointSelect(i)}
            className="group relative flex h-1.5 md:h-2 flex-1 items-center touch-target-44"
            aria-label={`Step ${i + 1}`}
            aria-current={i === activePoint ? "step" : undefined}
          >
            <div
              className={`h-full w-full rounded-full transition-all duration-500 ${i === activePoint ? "bg-primary" : "bg-border/50 hover:bg-border"}`}
            />
            {i === activePoint && (
              <motion.div
                layoutId={`active-point-${sectionId}`}
                className="absolute inset-0 rounded-full bg-primary shadow-[0_0_12px_rgba(var(--primary-rgb),0.4)]"
              />
            )}
          </button>
        ))}
        <span className="ml-2 md:ml-4 font-mono text-[9px] md:text-[10px] text-muted-foreground font-bold tracking-widest uppercase">
          0{activePoint + 1}
        </span>
      </div>
    </div>
  );
}

function TerminalDisplay({
  tab,
  activePoint,
  cmdText,
  content,
}: {
  tab: string;
  activePoint: number;
  cmdText?: string;
  content: React.ReactNode;
}) {
  return (
    <div className="relative group">
      <div className="absolute -inset-4 rounded-[1.5rem] md:rounded-[2.5rem] bg-primary/5 blur-2xl opacity-0 group-hover:opacity-100 transition-opacity" />
      <div className="rounded-2xl md:rounded-3xl border border-border bg-terminal-bg shadow-2xl relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-tr from-primary/5 via-transparent to-transparent pointer-events-none" />
        <TerminalHeader title={tab} />
        <div className="p-4 md:p-6 lg:p-8 relative z-10 min-h-[360px] md:min-h-[420px] lg:min-h-[440px] overflow-y-auto">
          <Breadcrumbs activeTab={tab} />
          <AnimatePresence mode="wait">
            <motion.div
              key={`${tab}-${activePoint}`}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              transition={{ duration: 0.4, ease: "easeOut" }}
              className="h-full flex flex-col"
            >
              {cmdText && <CmdLine text={cmdText} />}
              <div className="flex-1 overflow-auto">{content}</div>
            </motion.div>
          </AnimatePresence>
        </div>
      </div>
    </div>
  );
}

/** ============================================================
 *  Main Feature Section Orchestrator
 *  ============================================================ */

function FeatureSection({
  id,
  eyebrow,
  headline,
  points,
  tab,
  renderFrame,
  cmdForStep,
}: FeatureSectionProps) {
  const [active, setActive] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => {
      setActive((prev) => (prev + 1) % points.length);
    }, 5500);
    return () => clearInterval(interval);
  }, [points.length]);

  return (
    <section id={id} className="py-12 md:py-24 border-b border-border/10">
      <div className="mx-auto max-w-7xl px-4 md:px-6 w-full">
        <div className="grid gap-8 md:gap-12 lg:gap-16 md:grid-cols-[300px_1fr] lg:grid-cols-[400px_1fr] items-start">
          <div className="md:sticky md:top-20 lg:top-24">
            <NarrativeBox
              sectionId={id}
              eyebrow={eyebrow}
              headline={headline}
              activePoint={active}
              points={points}
              onPointSelect={setActive}
            />
          </div>
          <TerminalDisplay
            tab={tab}
            activePoint={active}
            cmdText={cmdForStep?.(active)}
            content={renderFrame(active)}
          />
        </div>
      </div>
    </section>
  );
}

/** ============================================================
 *  Content Data & Sections
 *  ============================================================ */

const AGENTS_LIVE = [
  { agent: "root-prime", role: "root", state: "idle", uptime: "4d 1h" },
  {
    agent: "manager-atlas",
    role: "manager",
    state: "working",
    uptime: "2d 4h",
  },
  {
    agent: "engineer-pixel",
    role: "engineer",
    state: "tool",
    uptime: "1d 12h",
  },
  { agent: "qa-nova", role: "qa", state: "working", uptime: "1d 8h" },
  { agent: "cron-nightly", role: "cron", state: "scheduled", uptime: "—" },
];

function AgentsFrame({ step }: { step: number }) {
  if (step === 0) {
    return (
      <div className="font-mono text-[12px] leading-relaxed text-terminal-text">
        <div className="text-terminal-comment uppercase text-[10px] tracking-widest font-bold mb-4">
          View: Agents / List
        </div>
        <div className="rounded-2xl border border-terminal-comment bg-terminal-bg/40 p-6 overflow-hidden">
          <div className="grid grid-cols-[1fr_120px_100px_100px] gap-4 text-terminal-muted uppercase text-[9px] tracking-widest font-bold border-b border-terminal-comment pb-3 mb-4">
            <div>Agent Name</div>
            <div>Role</div>
            <div>State</div>
            <div>Uptime</div>
          </div>
          <div className="space-y-4">
            {AGENTS_LIVE.map((r, i) => (
              <motion.div
                key={r.agent}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.1 }}
                className="grid grid-cols-[1fr_120px_100px_100px] gap-4"
              >
                <div className="flex items-center gap-2">
                  <span
                    className={
                      r.state === "working" || r.state === "tool"
                        ? "text-terminal-success animate-pulse"
                        : "text-terminal-muted"
                    }
                  >
                    ●
                  </span>
                  <span className="font-bold">{r.agent}</span>
                </div>
                <div className="text-terminal-muted capitalize">{r.role}</div>
                <div
                  className={
                    r.state === "working"
                      ? "text-terminal-success font-bold"
                      : r.state === "tool"
                        ? "text-terminal-command font-bold"
                        : "text-terminal-muted"
                  }
                >
                  {r.state}
                </div>
                <div className="text-terminal-comment">{r.uptime}</div>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    );
  }
  if (step === 1) {
    return (
      <div className="font-mono text-[12px] leading-relaxed text-terminal-text">
        <div className="text-terminal-comment uppercase text-[10px] tracking-widest font-bold mb-4">
          View: Agents / Peek / engineer-pixel
        </div>
        <div className="rounded-2xl border border-terminal-comment bg-terminal-bg/40 p-6">
          <div className="flex items-center justify-between mb-6">
            <div className="space-y-1">
              <div className="text-lg font-bold">engineer-pixel</div>
              <div className="text-[10px] text-terminal-muted uppercase tracking-widest">
                State:{" "}
                <span className="text-terminal-command underline decoration-terminal-command/20">
                  Tool Activity
                </span>
              </div>
            </div>
            <div className="h-10 w-10 flex items-center justify-center rounded-xl bg-terminal-command/10 text-terminal-command font-bold border border-terminal-command/20">
              PX
            </div>
          </div>
          <div className="space-y-3">
            <div className="text-[11px] text-terminal-muted border-l-2 border-terminal-success pl-4 bg-terminal-success/5 py-2">
              Loading project memory: search &quot;Zod patterns in api-v1&quot;
            </div>
            <div className="text-[11px] text-terminal-muted border-l-2 border-terminal-command pl-4 bg-terminal-command/5 py-2">
              Applying learnings from experience-42: &quot;Prefer strict schema
              validation&quot;
            </div>
            <div className="text-[11px] text-white border-l-2 border-primary pl-4 bg-primary/10 py-2 font-bold animate-pulse">
              ⏺ Writing tests/zod_validation_test.go
            </div>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="font-mono text-[12px] leading-relaxed text-terminal-text">
      <div className="text-terminal-comment uppercase text-[10px] tracking-widest font-bold mb-4">
        View: Agents / Command
      </div>
      <div className="grid gap-6">
        <div className="rounded-2xl border border-terminal-comment bg-terminal-bg p-4 border-l-4 border-l-primary">
          <div className="text-[10px] text-terminal-muted font-bold mb-1">
            YOU
          </div>
          <div className="text-sm">
            @engineer-pixel implement validation and report back.
          </div>
        </div>
        <div className="rounded-2xl border border-terminal-comment bg-terminal-bg/40 p-4 border-l-4 border-l-terminal-success">
          <div className="flex items-center justify-between mb-2">
            <div className="text-[10px] text-[var(--terminal-prompt)] font-bold uppercase tracking-wider">
              engineer-pixel
            </div>
            <div className="text-[9px] text-terminal-comment">JUST NOW</div>
          </div>
          <div className="text-sm text-terminal-text">
            Starting validation work. retrieved past Zod patterns. Branch
            isolated in{" "}
            <span className="text-terminal-success">
              .mycel/worktrees/pixel/feat-validate
            </span>
            .
          </div>
        </div>
      </div>
    </div>
  );
}

function ChannelsFrame({ step }: { step: number }) {
  const msgs = [
    {
      who: "root-prime",
      m: "Standup in #standup. 3 agents active, costs on track.",
    },
    {
      who: "manager-atlas",
      m: "Refining Epic roadmap. @pixel, thoughts on v2?",
    },
    { who: "engineer-pixel", m: "Clear. I've loaded the project context." },
  ];
  const active =
    step === 0
      ? msgs
      : [
          ...msgs,
          { who: "you", m: "@atlas please review the auth-flow refactor." },
        ];
  return (
    <div className="font-mono text-[12px] text-terminal-text">
      <div className="text-terminal-comment uppercase text-[10px] tracking-widest font-bold mb-4">
        View: Channels / #general
      </div>
      <div className="space-y-4">
        {active.map((m, i) => (
          <motion.div
            key={i}
            initial={{ opacity: 0, x: -10 }}
            animate={{ opacity: 1, x: 0 }}
            className={`flex flex-col gap-1 p-3 rounded-xl border border-terminal-comment ${m.who === "you" ? "bg-primary/5" : "bg-terminal-bg/20"}`}
          >
            <span className="text-[9px] uppercase tracking-tighter font-black text-terminal-muted">
              {m.who}
            </span>
            <div className="text-terminal-text leading-relaxed text-sm">
              {m.m}
            </div>
          </motion.div>
        ))}
      </div>
    </div>
  );
}

function CronFrame() {
  const data = [
    {
      name: "nightly-audit",
      state: "scheduled",
      task: "Full codebase audit + drift detection.",
    },
    {
      name: "build-sync",
      state: "running",
      task: "Sync staging builds with root state.",
    },
    {
      name: "clean-trash",
      state: "idle",
      task: "Cleanup of stale worktrees (>7 days).",
    },
  ];
  return (
    <div className="font-mono text-[12px] text-terminal-text">
      <div className="text-terminal-comment uppercase text-[10px] tracking-widest font-bold mb-4">
        View: Cron / Master Schedule
      </div>
      <div className="space-y-3">
        {data.map((d) => (
          <div
            key={d.name}
            className="p-4 rounded-2xl border border-terminal-comment bg-terminal-bg/40"
          >
            <div className="flex items-center justify-between mb-2">
              <span className="font-bold text-sm tracking-tight">{d.name}</span>
              <span className="text-[9px] uppercase font-black px-2 py-0.5 rounded bg-terminal-header text-terminal-muted">
                {d.state}
              </span>
            </div>
            <div className="text-[11px] text-terminal-muted leading-relaxed italic">
              {d.task}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export function AgentsSection() {
  const points = [
    {
      title: "Real-time Summary",
      desc: "Monitor status, roles, and live activity of every Claude Code agent in your dev org.",
    },
    {
      title: "Context Inspection",
      desc: "Zoom into any agent to see their current chain of thought and persistent memory across sessions.",
    },
    {
      title: "Unified Interface",
      desc: "Bridge the gap between human and AI agent. Send direct instructions to coordinate development.",
    },
  ];
  return (
    <FeatureSection
      id="agents"
      eyebrow="Agents"
      headline="Visibility into every thought."
      points={points}
      tab="Agents"
      cmdForStep={(i) =>
        i === 0
          ? "mycel status"
          : i === 1
            ? "mycel agent peek engineer-pixel"
            : "mycel agent send pixel 'status'"
      }
      renderFrame={(i) => <AgentsFrame step={i} />}
    />
  );
}

export function ChannelsSection() {
  const points = [
    {
      title: "Organic Coordination",
      desc: "AI agent coordination through Slack-like channels. Agents hand off tasks and share context automatically.",
    },
    {
      title: "Direct Participation",
      desc: "Step into any channel to guide your AI team or ask for a review — true AI team coordination.",
    },
    {
      title: "Full Traceability",
      desc: "Every interaction is logged and searchable. Revisit decisions across your Claude Code workflow.",
    },
  ];
  return (
    <FeatureSection
      id="channels"
      eyebrow="Channels"
      headline="Conversational Handoffs."
      points={points}
      tab="Channels"
      cmdForStep={(i) =>
        i === 0
          ? "mycel home"
          : i === 1
            ? "mycel channel send #general 'review'"
            : "mycel channel history #general"
      }
      renderFrame={(i) => <ChannelsFrame step={i} />}
    />
  );
}

export function CronSection() {
  const points = [
    {
      title: "Self-Healing Workflows",
      desc: "Configure scheduled agents to run periodic audits and automated tests as part of your parallel AI development pipeline.",
    },
    {
      title: "Observable Automation",
      desc: "Watch background agents execute in real-time with full log retention and persistent memory.",
    },
    {
      title: "One-Command Deploy",
      desc: "Create new automated workflows via the CLI. Persistent scheduling for your agent orchestration platform.",
    },
  ];
  return (
    <FeatureSection
      id="cron"
      eyebrow="Cron"
      headline="Reliable background work."
      points={points}
      tab="Cron"
      cmdForStep={(i) =>
        i === 0
          ? "mycel cron list"
          : i === 1
            ? "mycel cron run build-sync"
            : "mycel cron add audit"
      }
      renderFrame={() => <CronFrame />}
    />
  );
}
