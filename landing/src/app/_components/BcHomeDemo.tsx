"use client";

import { motion } from "framer-motion";
import { useEffect, useMemo, useState } from "react";

type Frame =
  | { kind: "cmd"; text: string; title?: string }
  | { kind: "screen"; title: string; body: React.ReactNode };

function Dot({ className }: { className: string }) {
  return <span className={`h-2.5 w-2.5 rounded-full ${className}`} />;
}

function TerminalShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-border bg-card shadow-2xl">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <div className="flex gap-1.5">
          <Dot className="bg-[var(--traffic-red)]" />
          <Dot className="bg-[var(--traffic-yellow)]" />
          <Dot className="bg-[var(--traffic-green)]" />
        </div>
        <span className="ml-2 font-mono text-xs text-muted-foreground uppercase tracking-widest">
          mycel terminal
        </span>
      </div>
      <div className="p-5 min-h-[320px]">{children}</div>
    </div>
  );
}

function CmdLine({ text }: { text: string }) {
  return (
    <div className="font-mono text-sm sm:text-[13px] leading-relaxed text-foreground">
      <span className="text-[var(--terminal-prompt)]">➜ </span>
      <span className="text-[var(--terminal-command)]">~ </span>
      <span>{text}</span>
    </div>
  );
}

function DashboardView() {
  return (
    <div className="font-mono text-xs sm:text-[12px] text-foreground">
      <div className="flex items-center justify-between border-b border-border pb-2 mb-4">
        <span className="text-muted-foreground">DASHBOARD: mycel-workspace</span>
        <span className="text-success text-xs sm:text-[10px]">● LIVE</span>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground uppercase text-xs sm:text-[10px] tracking-widest mb-2 font-bold">
            Health
          </div>
          <div className="flex items-end gap-2">
            <span className="text-2xl font-bold">98%</span>
            <span className="text-success text-xs sm:text-[10px] mb-1">
              ↑ 2%
            </span>
          </div>
        </div>
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground uppercase text-xs sm:text-[10px] tracking-widest mb-2 font-bold">
            Costs
          </div>
          <div className="flex items-end gap-2">
            <span className="text-2xl font-bold">$4.21</span>
            <span className="text-muted-foreground text-xs sm:text-[10px] mb-1">
              Today
            </span>
          </div>
        </div>
      </div>
      <div className="mt-4 rounded-xl border border-border bg-muted/40 p-3">
        <div className="text-muted-foreground uppercase text-xs sm:text-[10px] tracking-widest mb-2 font-bold">
          Active Agents
        </div>
        <div className="space-y-1.5">
          <div className="flex justify-between items-center">
            <span>root-prime</span>
            <span className="text-muted-foreground">idle</span>
          </div>
          <div className="flex justify-between items-center">
            <span>manager-atlas</span>
            <span className="text-success italic">working (epic-auth)</span>
          </div>
          <div className="flex justify-between items-center">
            <span>engineer-pixel</span>
            <span className="text-[var(--terminal-command)]">
              task (ui-refactor)
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

function CostView() {
  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="flex items-center justify-between border-b border-border pb-2 mb-4">
        <span className="text-muted-foreground">COST TRACKING: mycel-workspace</span>
        <span className="text-muted-foreground text-[10px]">today</span>
      </div>
      <div className="grid gap-4">
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground mb-2 border-b border-border pb-1">
            Per-Agent Usage
          </div>
          <div className="space-y-1.5">
            <div className="flex justify-between">
              <span>engineer-pixel</span>
              <span className="text-success">$1.87 / $5.00</span>
            </div>
            <div className="flex justify-between">
              <span>engineer-nova</span>
              <span className="text-[var(--terminal-command)]">
                $3.42 / $5.00
              </span>
            </div>
            <div className="flex justify-between">
              <span>manager-atlas</span>
              <span className="text-muted-foreground">$0.92 / $3.00</span>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground mb-2 border-b border-border pb-1">
            Budget Summary
          </div>
          <div className="space-y-1">
            <div className="flex justify-between">
              <span>Daily spend</span>
              <span className="text-success">$6.21 / $15.00</span>
            </div>
            <div className="flex justify-between">
              <span>Monthly spend</span>
              <span className="text-muted-foreground">$142.30 / $500.00</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function MemoryView() {
  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="flex items-center justify-between border-b border-border pb-2 mb-4">
        <span className="text-muted-foreground">MEMORY: engineer-pixel</span>
      </div>
      <div className="space-y-4">
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground uppercase text-[10px] tracking-widest mb-2 font-bold">
            Learnings
          </div>
          <ul className="space-y-1 text-muted-foreground">
            <li>• Use `bc ag send` to assign work to agents</li>
            <li>• Framework v4 requires specific lint flags</li>
            <li>• CI suite fails on memory edge cases</li>
          </ul>
        </div>
        <div className="rounded-xl border border-border bg-muted/40 p-3">
          <div className="text-muted-foreground uppercase text-[10px] tracking-widest mb-2 font-bold">
            Recent Experience
          </div>
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-success"></span>
            <span>Task work-042 completed succesfully</span>
          </div>
          <p className="mt-1 text-muted-foreground italic text-[11px]">
            &quot;Implemented Zod validation - learned this project prefers
            strict schema over ad-hoc checks.&quot;
          </p>
        </div>
      </div>
    </div>
  );
}

function ChatRoom() {
  const msgs = [
    {
      t: "10:23",
      who: "root",
      tag: "root",
      msg: "Morning. Standup in #standup. 3 agents active.",
    },
    {
      t: "10:24",
      who: "atlas",
      tag: "mgr",
      msg: "Auth breakdown ready. Assigning steps to #eng.",
    },
    {
      t: "10:25",
      who: "pixel",
      tag: "eng",
      msg: "I'll take the token logic. Loading project context now.",
    },
    {
      t: "10:28",
      who: "scheduler",
      tag: "cron",
      msg: "Cron: test-suite passed ✅ All green.",
    },
    {
      t: "10:31",
      who: "root",
      tag: "root",
      msg: "Costs at $4.21 today. Well within budget.",
    },
  ];

  const badge = (tag: string) => {
    switch (tag) {
      case "root":
        return "bg-primary text-primary-foreground";
      case "mgr":
        return "border border-[var(--terminal-command)]/50 text-[var(--terminal-command)]";
      case "cron":
        return "border border-[var(--terminal-prompt)]/50 text-[var(--terminal-prompt)] italic";
      default:
        return "border border-border text-muted-foreground";
    }
  };

  return (
    <div className="font-mono text-[12px] text-foreground">
      <div className="flex items-center justify-between border-b border-border pb-2 mb-3">
        <div className="text-muted-foreground">CHANNEL: #general</div>
        <div className="text-muted-foreground text-[10px]">Esc • back</div>
      </div>

      <div className="space-y-3">
        {msgs.map((m, i) => (
          <motion.div
            key={i}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: i * 0.08, duration: 0.3 }}
            className="flex gap-3"
          >
            <span className="text-muted-foreground mt-1">{m.t}</span>
            <div className="flex flex-col gap-1">
              <span
                className={`inline-flex self-start items-center rounded px-1.5 text-[9px] font-bold uppercase tracking-tighter ${badge(m.tag)}`}
              >
                {m.who}
              </span>
              <span className="text-foreground leading-relaxed">{m.msg}</span>
            </div>
          </motion.div>
        ))}
      </div>

      <div className="mt-4 flex items-center gap-2 rounded-xl border border-border bg-card px-3 py-2">
        <span className="text-muted-foreground">&gt;</span>
        <span className="text-muted-foreground italic">Type message...</span>
        <span className="ml-auto text-muted-foreground text-[10px]">
          #channel
        </span>
      </div>
    </div>
  );
}

export function BcHomeDemo() {
  const frames: Frame[] = useMemo(
    () => [
      { kind: "cmd", text: "bc up" },
      { kind: "screen", title: "dashboard", body: <DashboardView /> },
      { kind: "screen", title: "costs", body: <CostView /> },
      { kind: "screen", title: "memory", body: <MemoryView /> },
      { kind: "screen", title: "chat", body: <ChatRoom /> },
    ],
    [],
  );

  const [idx, setIdx] = useState(0);

  useEffect(() => {
    const t = setInterval(() => setIdx((i) => (i + 1) % frames.length), 4500);
    return () => clearInterval(t);
  }, [frames.length]);

  const f = frames[idx];

  return (
    <div className="flex flex-col gap-4">
      <TerminalShell>
        <motion.div
          key={idx}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, ease: "easeOut" }}
        >
          {f.kind === "cmd" ? (
            <div className="flex flex-col gap-4">
              <CmdLine text={f.text} />
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.5 }}
                className="space-y-1 font-mono text-[12px] text-muted-foreground"
              >
                <div>[mycel] Starting orchestration engine...</div>
                <div>[mycel] Initializing Root Agent (root-prime)</div>
                <div className="text-success">
                  [mycel] Environment ready. Workspace active.
                </div>
              </motion.div>
            </div>
          ) : (
            f.body
          )}
        </motion.div>
      </TerminalShell>

      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          {frames.map((_, i) => (
            <button
              key={i}
              onClick={() => setIdx(i)}
              className={`h-1 rounded-full transition-all ${
                i === idx ? "w-8 bg-primary" : "w-4 bg-muted hover:bg-muted/80"
              }`}
              aria-label={`View demo step ${i + 1}: ${frames[i].title || "init"}`}
            />
          ))}
        </div>
        <span className="font-mono text-[10px] text-muted-foreground uppercase tracking-widest hidden sm:inline">
          {frames[idx].title || "init"}
        </span>
      </div>
    </div>
  );
}
