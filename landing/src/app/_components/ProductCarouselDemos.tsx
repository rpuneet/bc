"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";

/** ============================================================
 *  Terminal chrome
 *  ============================================================ */

function Dot({ className }: { className: string }) {
  return <span className={`h-2.5 w-2.5 rounded-full ${className}`} />;
}

function TerminalShell({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-3xl border border-border bg-card shadow-2xl">
      <div className="flex items-center justify-between gap-3 border-b border-border px-6 py-4">
        <div className="flex items-center gap-2">
          <Dot className="bg-muted" />
          <Dot className="bg-muted" />
          <Dot className="bg-muted" />
          <span className="ml-3 font-mono text-xs text-muted-foreground">
            {title}
          </span>
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">
          bc home
        </span>
      </div>
      <div className="p-6">{children}</div>
    </div>
  );
}

function BreadcrumbAndTabs({
  activeTab,
}: {
  activeTab: "Agents" | "Channels" | "Cron";
}) {
  const tabs: Array<"Agents" | "Channels" | "Cron"> = [
    "Agents",
    "Channels",
    "Cron",
  ];
  return (
    <div className="mb-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="font-mono text-[12px] text-muted-foreground">
          Workspaces <span className="text-muted-foreground">›</span> mycel-workspace
        </div>

        <div className="flex items-center gap-2 rounded-xl border border-border bg-muted/40 p-1">
          {tabs.map((t) => (
            <div
              key={t}
              className={`rounded-lg px-3 py-1 font-mono text-[12px] transition ${
                t === activeTab
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground"
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

/** ============================================================
 *  Demo frames (reusing yours, but embedded here)
 *  ============================================================ */

type AgentState =
  | "working"
  | "thinking"
  | "tool"
  | "waiting"
  | "idle"
  | "error";
type AgentRow = {
  agent: string;
  role: string;
  state: AgentState;
  uptime: string;
  symbol: string; // ✻ ✳ ✽ · ⏺ ❯
  task: string;
};

const AGENTS_LIVE: AgentRow[] = [
  {
    agent: "coordinator",
    role: "coordinator",
    state: "working",
    uptime: "1h12m",
    symbol: "✽",
    task: "Routing PRs #347–#351. Watching CI + approvals. Handing off review to TLs + QA.",
  },
  {
    agent: "manager-atlas",
    role: "manager",
    state: "working",
    uptime: "1h05m",
    symbol: "✽",
    task: "Integrating child branches into epic #322. Preparing final review.",
  },
  {
    agent: "tech-lead-01",
    role: "tech-lead",
    state: "tool",
    uptime: "39m",
    symbol: "⏺",
    task: "Web UI perf: lazy-load per screen (#324). Profiling + benchmarks. Updating PR.",
  },
  {
    agent: "engineer-01",
    role: "engineer",
    state: "tool",
    uptime: "27m",
    symbol: "⏺",
    task: "Channels send shortcuts. Fixing multiline edge-cases + scroll. Running tests.",
  },
  {
    agent: "engineer-02",
    role: "engineer",
    state: "working",
    uptime: "18m",
    symbol: "✽",
    task: "Chat UI polish (#307): wrapping, grouping, timestamps. PR incoming.",
  },
  {
    agent: "engineer-03",
    role: "engineer",
    state: "waiting",
    uptime: "33m",
    symbol: "❯",
    task: "Blocked on review: PR #334 needs TL signoff. Ready to respond.",
  },
  {
    agent: "qa-nova",
    role: "qa",
    state: "working",
    uptime: "22m",
    symbol: "✻",
    task: "QA pass on PRs #347–#351. Approve when CI green + TL acked.",
  },
  {
    agent: "product-manager",
    role: "product-manager",
    state: "thinking",
    uptime: "54m",
    symbol: "✳",
    task: "Backlog triage: epic #322 (Web UI), #314 (channels). Tightening acceptance criteria.",
  },
];

function stateColor(state: AgentState) {
  switch (state) {
    case "working":
      return "text-terminal-success";
    case "thinking":
      return "text-terminal-prompt";
    case "tool":
      return "text-sky-300";
    case "waiting":
      return "text-terminal-text";
    case "idle":
      return "text-terminal-muted";
    case "error":
      return "text-rose-300";
  }
}

function pad(s: string, n: number) {
  if (s.length >= n) return s.slice(0, n);
  return s + " ".repeat(n - s.length);
}
function truncate(s: string, n: number) {
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "…";
}

function AgentsStatusFrame() {
  return (
    <div className="font-mono text-[12px] leading-relaxed text-foreground">
      <div className="text-muted-foreground">View: Agents</div>
      <div className="mt-4 rounded-2xl border border-border bg-muted/40 p-5">
        <div className="whitespace-pre text-muted-foreground">
          {pad("AGENT", 16)}
          {pad("ROLE", 16)}
          {pad("STATE", 12)}
          {pad("UPTIME", 10)}
          {"TASK"}
          {"\n"}
          {
            "--------------------------------------------------------------------------------"
          }
        </div>
        <div className="mt-3 space-y-2">
          {AGENTS_LIVE.map((r) => (
            <div key={r.agent} className="flex gap-2">
              <span className="w-[2ch] text-muted-foreground">{r.symbol}</span>
              <span className="whitespace-pre">
                <span className="text-foreground">{pad(r.agent, 16)}</span>
                <span className="text-foreground">{pad(r.role, 16)}</span>
                <span className={stateColor(r.state)}>{pad(r.state, 12)}</span>
                <span className="text-foreground">{pad(r.uptime, 10)}</span>
              </span>
              <span className="text-foreground">{truncate(r.task, 140)}</span>
            </div>
          ))}
        </div>
        <div className="mt-5 border-t border-border pt-4 text-muted-foreground">
          Symbols: ✻ ✳ ✽ · thinking • ⏺ tool call • ❯ waiting
        </div>
      </div>
    </div>
  );
}

function AgentsPeekFrame() {
  return (
    <div className="font-mono text-[12px] leading-relaxed text-foreground">
      <div className="text-muted-foreground">
        View: Agents • peek: engineer-01
      </div>
      <div className="mt-4 rounded-2xl border border-border bg-muted/40 p-5">
        <div className="text-muted-foreground">
          engineer-01 • state: <span className="text-sky-300">tool</span> •
          uptime: 27m
        </div>
        <div className="mt-4 rounded-xl border border-border bg-card p-4">
          <div className="text-muted-foreground">Latest output</div>
          <div className="mt-2 space-y-1">
            <div>⏺ running tests: channel send + keymap</div>
            <div>✻ thinking: multiline paste edge-case</div>
            <div>⏺ updated: Ctrl+J and Ctrl+Enter send</div>
            <div>✽ pushing branch: engineer-01/channel-send-shortcuts</div>
            <div className="text-muted-foreground">…</div>
          </div>
        </div>
      </div>
    </div>
  );
}

function AgentsChatFrame() {
  return (
    <div className="font-mono text-[12px] leading-relaxed text-foreground">
      <div className="text-muted-foreground">View: Agents • message</div>
      <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_420px]">
        <div className="rounded-2xl border border-border bg-muted/40 p-5">
          <div className="text-muted-foreground">agent feed</div>
          <div className="mt-3 space-y-2">
            <div>
              <span className="text-muted-foreground">10:31</span> engineer-01 ⏺
              tests green; PR draft up
            </div>
            <div>
              <span className="text-muted-foreground">10:32</span> tech-lead-01
              ✻ perf win: first render -38%
            </div>
            <div>
              <span className="text-muted-foreground">10:33</span> qa-nova ✽
              smoke pass; waiting CI
            </div>
          </div>
        </div>
        <div className="rounded-2xl border border-border bg-muted/40 p-5">
          <div className="text-muted-foreground">type message</div>
          <div className="mt-3 rounded-xl border border-border bg-card px-4 py-3">
            <span className="text-muted-foreground">&gt; </span>
            engineer-01 status + PR link?
            <span className="ml-2 animate-pulse text-muted-foreground">▌</span>
          </div>
          <div className="mt-4">
            <div className="text-muted-foreground">engineer-01:</div>
            <div className="mt-1 text-foreground">
              PR draft is up. Shortcut sends working. Adding regression test;
              marking ready soon.
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// Channels
type Msg = {
  t: string;
  who: string;
  msg: string;
  tag?: "root" | "mgr" | "eng" | "qa" | "you";
};
function badgeClass(tag?: Msg["tag"]) {
  if (tag === "root") return "bg-primary text-primary-foreground";
  if (tag === "you") return "bg-secondary text-secondary-foreground";
  return "border border-border text-muted-foreground";
}
function ChannelsFrame(step: 0 | 1 | 2) {
  const channels = ["#general", "#product", "#eng", "#qa", "#standup"];
  const base: Msg[] = [
    {
      t: "10:23",
      who: "root",
      tag: "root",
      msg: "Standup in #standup. 3 agents active, costs on track.",
    },
    {
      t: "10:24",
      who: "manager-atlas",
      tag: "mgr",
      msg: "Assigning tasks for auth epic to #eng.",
    },
    {
      t: "10:25",
      who: "qa-nova",
      tag: "qa",
      msg: "Preparing edge-case matrix. Ping when merged.",
    },
  ];
  const sent: Msg = {
    t: "10:27",
    who: "you",
    tag: "you",
    msg: "@tech-lead-01 review PR #349 and unblock merge?",
  };
  const reply: Msg = {
    t: "10:28",
    who: "tech-lead-01",
    tag: "eng",
    msg: "Reviewing now. Will approve once CI is green.",
  };
  const messages =
    step === 0 ? base : step === 1 ? [...base, sent] : [...base, sent, reply];

  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="text-muted-foreground">View: Channels • #general</div>
      <div className="mt-4 grid gap-4 lg:grid-cols-[320px_1fr]">
        <div className="rounded-2xl border border-border bg-muted/40 p-5">
          <div className="text-muted-foreground">Channels</div>
          <div className="mt-3 space-y-1">
            {channels.map((c) => (
              <div
                key={c}
                className={
                  c === "#general" ? "text-foreground" : "text-muted-foreground"
                }
              >
                {c === "#general" ? "▸ " : "  "}
                {c}
              </div>
            ))}
          </div>
        </div>
        <div className="rounded-2xl border border-border bg-muted/40 p-5">
          <div className="flex items-center justify-between">
            <div className="text-muted-foreground">Chat</div>
            <div className="text-muted-foreground">Ctrl+Enter • send</div>
          </div>
          <div className="mt-4 space-y-2">
            {messages.map((m, i) => (
              <motion.div
                key={i}
                initial={{ opacity: 0, x: -8 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ duration: 0.2, delay: i * 0.02 }}
                className="flex gap-3"
              >
                <span className="text-muted-foreground">{m.t}</span>
                <span
                  className={`inline-flex h-5 items-center rounded px-2 text-[11px] ${badgeClass(m.tag)}`}
                >
                  {m.who}
                </span>
                <span className="text-foreground">{m.msg}</span>
              </motion.div>
            ))}
          </div>
          <div className="mt-5 rounded-xl border border-border bg-card px-4 py-3">
            <span className="text-muted-foreground">&gt; </span>
            {step === 0 ? (
              <span className="text-muted-foreground">Type message…</span>
            ) : (
              <>
                @tech-lead-01 review PR #349…{" "}
                <span className="animate-pulse text-muted-foreground">▌</span>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// Cron Jobs
type CronRow = {
  name: string;
  role: string;
  state: "running" | "scheduled" | "failed" | "idle";
  nextRun: string;
  lastRun: string;
  task: string;
};
const CRON_JOBS: CronRow[] = [
  {
    name: "cron-nightly",
    role: "engineer",
    state: "scheduled",
    nextRun: "01:00 (7h 32m)",
    lastRun: "✅ 12m",
    task: "Nightly test suite + flake summary",
  },
  {
    name: "cron-release",
    role: "manager",
    state: "scheduled",
    nextRun: "09:00 (15h)",
    lastRun: "✅ 8m",
    task: "Daily release build + changelog",
  },
  {
    name: "cron-deps",
    role: "engineer",
    state: "idle",
    nextRun: "Mon 10:00",
    lastRun: "✅ 6m",
    task: "Weekly dependency updates",
  },
  {
    name: "cron-health",
    role: "qa",
    state: "running",
    nextRun: "—",
    lastRun: "—",
    task: "Hourly smoke checks",
  },
];
function cronStateColor(s: CronRow["state"]) {
  if (s === "running") return "text-sky-300";
  if (s === "scheduled") return "text-terminal-prompt";
  if (s === "failed") return "text-rose-300";
  return "text-terminal-text";
}
function CronListFrame() {
  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="text-muted-foreground">View: Cron Jobs</div>
      <div className="mt-4 rounded-2xl border border-border bg-muted/40 p-5">
        <div className="whitespace-pre text-muted-foreground">
          {pad("JOB", 16)}
          {pad("ROLE", 12)}
          {pad("STATE", 12)}
          {pad("NEXT RUN", 18)}
          {pad("LAST", 10)}
          {"TASK"}
          {"\n"}
          {
            "--------------------------------------------------------------------------------"
          }
        </div>
        <div className="mt-3 space-y-2">
          {CRON_JOBS.map((d) => (
            <div key={d.name} className="flex gap-2">
              <span className="whitespace-pre">
                <span className="text-foreground">{pad(d.name, 16)}</span>
                <span className="text-foreground">{pad(d.role, 12)}</span>
                <span className={cronStateColor(d.state)}>
                  {pad(d.state, 12)}
                </span>
                <span className="text-foreground">{pad(d.nextRun, 18)}</span>
                <span className="text-foreground">{pad(d.lastRun, 10)}</span>
              </span>
              <span className="text-foreground">{truncate(d.task, 120)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
function CronRunFrame() {
  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="text-muted-foreground">
        View: Cron Jobs • running: cron-health
      </div>
      <div className="mt-4 rounded-2xl border border-border bg-muted/40 p-5">
        <div className="text-muted-foreground">
          cron-health • state: <span className="text-sky-300">running</span>
        </div>
        <div className="mt-4 rounded-xl border border-border bg-card p-4">
          <div className="text-muted-foreground">Logs</div>
          <div className="mt-2 space-y-1">
            <div>⏺ starting smoke: agent health</div>
            <div>⏺ checking: channels send / history</div>
            <div>⏺ verifying: cost budgets</div>
            <div>⏺ report: 0 regressions</div>
            <div className="text-muted-foreground">…</div>
          </div>
        </div>
      </div>
    </div>
  );
}
function CronScheduleFrame() {
  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="text-muted-foreground">View: Cron Jobs • schedule</div>
      <div className="mt-4 rounded-2xl border border-border bg-muted/40 p-5">
        <div className="text-foreground">Add a cron job</div>
        <div className="mt-3 rounded-xl border border-border bg-card p-4">
          <div className="text-muted-foreground">
            $ bc cron add cron-health --schedule &quot;0 * * * *&quot; --cmd
            &quot;npm test&quot;
          </div>
        </div>
      </div>
    </div>
  );
}

/** ============================================================
 *  Carousel controller
 *  ============================================================ */

type Slide = {
  tab: "Agents" | "Channels" | "Cron";
  title: string; // small caption
  render: () => React.ReactNode;
};

export default function ProductCarouselDemos({
  intervalMs = 4200,
}: {
  intervalMs?: number;
}) {
  const slides: Slide[] = useMemo(
    () => [
      {
        tab: "Agents",
        title: "Agents: live org status",
        render: () => <AgentsStatusFrame />,
      },
      {
        tab: "Agents",
        title: "Agents: peek an agent’s work",
        render: () => <AgentsPeekFrame />,
      },
      {
        tab: "Agents",
        title: "Agents: message + next step",
        render: () => <AgentsChatFrame />,
      },

      {
        tab: "Channels",
        title: "Channels: #general as timeline",
        render: () => ChannelsFrame(0),
      },
      {
        tab: "Channels",
        title: "Channels: send the handoff",
        render: () => ChannelsFrame(1),
      },
      {
        tab: "Channels",
        title: "Channels: reply with action",
        render: () => ChannelsFrame(2),
      },

      {
        tab: "Cron",
        title: "Cron: scheduled tasks",
        render: () => <CronListFrame />,
      },
      {
        tab: "Cron",
        title: "Cron: run + logs",
        render: () => <CronRunFrame />,
      },
      {
        tab: "Cron",
        title: "Cron: define schedule",
        render: () => <CronScheduleFrame />,
      },
    ],
    [],
  );

  const [idx, setIdx] = useState(0);
  const [paused, setPaused] = useState(false);
  const timer = useRef<number | null>(null);

  useEffect(() => {
    if (paused) return;
    timer.current = window.setInterval(() => {
      setIdx((i) => (i + 1) % slides.length);
    }, intervalMs);

    return () => {
      if (timer.current) window.clearInterval(timer.current);
    };
  }, [paused, intervalMs, slides.length]);

  const active = slides[idx];
  const activeTab = active.tab;

  return (
    <section className="mx-auto max-w-7xl px-6 py-16">
      <div className="text-sm font-medium text-muted-foreground">Product</div>
      <h2 className="mt-3 text-4xl font-semibold tracking-tight sm:text-5xl">
        A dev org you can actually operate
      </h2>

      {/* Small caption + controls (not big ugly cards) */}
      <div className="mt-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="text-sm text-muted-foreground">
          <span className="font-semibold text-foreground">{active.title}</span>
          <span className="text-muted-foreground"> · </span>
          <span>Autoplay demo</span>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() =>
              setIdx((i) => (i - 1 + slides.length) % slides.length)
            }
            className="inline-flex h-11 w-11 sm:h-10 sm:w-auto sm:px-4 items-center justify-center rounded-xl border border-border bg-secondary text-sm text-secondary-foreground shadow-sm hover:bg-secondary/80 transition-colors"
            aria-label="Previous slide"
          >
            Prev
          </button>
          <button
            onClick={() => setPaused((p) => !p)}
            className="inline-flex h-11 w-11 sm:h-10 sm:w-auto sm:px-4 items-center justify-center rounded-xl border border-border bg-secondary text-sm text-secondary-foreground shadow-sm hover:bg-secondary/80 transition-colors"
            aria-label={paused ? "Play carousel" : "Pause carousel"}
          >
            {paused ? "Play" : "Pause"}
          </button>
          <button
            onClick={() => setIdx((i) => (i + 1) % slides.length)}
            className="inline-flex h-11 w-11 sm:h-10 sm:w-auto sm:px-4 items-center justify-center rounded-xl border border-border bg-secondary text-sm text-secondary-foreground shadow-sm hover:bg-secondary/80 transition-colors"
            aria-label="Next slide"
          >
            Next
          </button>
        </div>
      </div>

      {/* One terminal only */}
      <div
        className="mt-6"
        onMouseEnter={() => setPaused(true)}
        onMouseLeave={() => setPaused(false)}
      >
        <TerminalShell title={activeTab}>
          <BreadcrumbAndTabs activeTab={activeTab} />

          <AnimatePresence mode="wait">
            <motion.div
              key={`slide-${idx}`}
              initial={{ opacity: 0, y: 14 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              transition={{ duration: 0.5, ease: "easeOut" }}
              className="min-h-[380px] sm:min-h-[520px]"
            >
              {active.render()}
            </motion.div>
          </AnimatePresence>

          {/* carousel navigation dots */}
          <div className="mt-6 flex items-center justify-center gap-2">
            {slides.map((_, i) => (
              <button
                key={i}
                onClick={() => setIdx(i)}
                className={`inline-flex h-11 w-11 items-center justify-center rounded-lg transition ${
                  i === idx
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted hover:bg-muted/80 text-foreground"
                }`}
                aria-label={`Go to slide ${i + 1}`}
              >
                <span className="text-xs font-semibold">{i + 1}</span>
              </button>
            ))}
          </div>
        </TerminalShell>
      </div>
    </section>
  );
}
