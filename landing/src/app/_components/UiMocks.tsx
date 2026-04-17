"use client";

import { motion } from "framer-motion";

function Chip({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center rounded-full border border-border bg-white px-3 py-1 text-xs text-terminal-comment shadow-sm">
      {children}
    </span>
  );
}

export function TerminalMock() {
  const lines = [
    "$ bc init",
    "✓ workspace initialized (.bc/)",
    "$ bc up",
    "✓ root agent started",
    "$ bc agent create manager-atlas --role manager",
    "✓ spawned manager-atlas (tool: cursor)",
    "$ bc cost usage",
    "manager-atlas: epic-auth-system (in-progress)",
    "$ bc cron list",
    "manager-atlas: task-3-branch (conflict)  ← reject + notify",
  ];

  return (
    <div className="rounded-2xl border border-border bg-terminal-bg p-4 shadow-sm">
      <div className="mb-3 flex items-center gap-2">
        <span className="h-2.5 w-2.5 rounded-full bg-muted-foreground/50" />
        <span className="h-2.5 w-2.5 rounded-full bg-muted-foreground/50" />
        <span className="h-2.5 w-2.5 rounded-full bg-muted-foreground/50" />
        <span className="ml-2 text-xs text-terminal-muted">terminal</span>
      </div>
      <div className="space-y-1 font-mono text-xs sm:text-[12px] leading-relaxed text-terminal-text">
        {lines.map((l, i) => (
          <motion.div
            key={l}
            initial={{ opacity: 0, x: -8 }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true }}
            transition={{ delay: i * 0.06, duration: 0.35 }}
          >
            {l}
          </motion.div>
        ))}
      </div>
    </div>
  );
}

export function DashboardMock() {
  return (
    <div className="rounded-2xl border border-border bg-white p-5 shadow-sm">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="text-sm font-semibold">Workspace dashboard</div>
          <div className="mt-1 text-xs text-terminal-muted">
            status • costs • cron • agents • channels
          </div>
        </div>
        <Chip>bc home</Chip>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        {[
          { k: "Agents", v: "7 active", s: "root + teams" },
          { k: "Daily cost", v: "$4.21", s: "under budget" },
          { k: "Memory", v: "warm", s: "retrieval enabled" },
        ].map((x) => (
          <div
            key={x.k}
            className="rounded-xl border border-border bg-muted p-4"
          >
            <div className="text-xs text-terminal-muted">{x.k}</div>
            <div className="mt-1 text-lg font-semibold">{x.v}</div>
            <div className="mt-1 text-xs text-terminal-muted">{x.s}</div>
          </div>
        ))}
      </div>

      <div className="mt-4 rounded-xl border border-border bg-white p-4">
        <div className="mb-2 text-xs font-semibold text-terminal-comment">
          Recent activity
        </div>
        <div className="space-y-2 text-xs text-terminal-comment">
          <div className="flex items-center justify-between">
            <span>manager-atlas submitted epic-auth-system</span>
            <span className="text-terminal-muted">2m</span>
          </div>
          <div className="flex items-center justify-between">
            <span>qa-nova flagged edge case (token refresh)</span>
            <span className="text-terminal-muted">7m</span>
          </div>
          <div className="flex items-center justify-between">
            <span>root rejected task-3-branch (conflict)</span>
            <span className="text-terminal-muted">11m</span>
          </div>
        </div>
      </div>
    </div>
  );
}

export function CostMock() {
  return (
    <div className="rounded-2xl border border-border bg-white p-5 shadow-sm">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm font-semibold">Cost tracking overview</div>
          <div className="mt-1 text-xs text-terminal-muted">
            per-agent budgets · alerts · hard stops
          </div>
        </div>
        <Chip>bc cost</Chip>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <div className="rounded-xl border border-border bg-muted p-4">
          <div className="text-xs font-semibold text-terminal-comment">
            AGENT COSTS (today)
          </div>
          <div className="mt-3 space-y-2 text-xs">
            <Row left="epic-auth-system" right="in-progress" />
            <Row left="epic-payments" right="pending" />
          </div>
        </div>
        <div className="rounded-xl border border-border bg-muted p-4">
          <div className="text-xs font-semibold text-terminal-comment">
            BUDGET STATUS
          </div>
          <div className="mt-3 space-y-2 text-xs">
            <Row left="task-1-branch" right="pending" />
            <Row left="task-2-branch" right="pending" />
            <Row left="task-3-branch" right="conflict" bad />
          </div>
        </div>
      </div>
    </div>
  );
}

function Row({
  left,
  right,
  bad,
}: {
  left: string;
  right: string;
  bad?: boolean;
}) {
  return (
    <motion.div
      whileHover={{ scale: 1.01 }}
      transition={{ duration: 0.15 }}
      className="flex items-center justify-between rounded-lg border border-border bg-white px-3 py-2"
    >
      <span className="font-mono text-[12px] text-terminal-comment">
        {left}
      </span>
      <span
        className={
          bad
            ? "rounded-full bg-primary px-2 py-0.5 text-[11px] text-white"
            : "rounded-full border border-border bg-white px-2 py-0.5 text-[11px] text-terminal-comment"
        }
      >
        {right}
      </span>
    </motion.div>
  );
}
