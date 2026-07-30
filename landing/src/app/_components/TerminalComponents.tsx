"use client";

import { motion, useInView } from "framer-motion";
import { useRef, useEffect, useState, type ReactNode } from "react";

/* ═══════════════════════════════════════════════════════════════════
   1. TERMINAL WINDOW — Dark container with traffic lights
   ═══════════════════════════════════════════════════════════════════ */

export function TerminalWindow({
  title = "mycel terminal",
  children,
  className = "",
  ariaLabel,
}: {
  title?: string;
  children: ReactNode;
  className?: string;
  ariaLabel?: string;
}) {
  return (
    <div
      role="img"
      aria-label={ariaLabel || `Terminal window showing ${title}`}
      className={`overflow-hidden rounded-xl border border-border bg-terminal-bg shadow-2xl dark:border-[rgba(232,163,61,0.14)] ${className}`}
    >
      <div className="flex items-center gap-2 border-b border-[rgba(210,180,140,0.08)] bg-terminal-header px-4 py-2.5">
        <div className="flex gap-1.5" aria-hidden="true">
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-red)]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-yellow)]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[var(--traffic-green)]" />
        </div>
        <span className="ml-2 font-mono text-[10px] font-bold uppercase tracking-[0.2em] text-terminal-muted">
          {title}
        </span>
      </div>
      <div className="p-5 font-mono text-[13px] leading-[1.7] text-terminal-text">
        {children}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   2. TERMINAL TYPING — Character-by-character reveal
   ═══════════════════════════════════════════════════════════════════ */

export function TerminalTyping({
  text,
  delay = 0,
  speed = 50,
  className = "",
  prefix = "$ ",
}: {
  text: string;
  delay?: number;
  speed?: number;
  className?: string;
  prefix?: string;
}) {
  const [displayed, setDisplayed] = useState("");
  const [started, setStarted] = useState(false);
  const ref = useRef(null);
  const inView = useInView(ref, { once: true });

  useEffect(() => {
    if (!inView) return;
    const t = setTimeout(() => setStarted(true), delay);
    return () => clearTimeout(t);
  }, [inView, delay]);

  useEffect(() => {
    if (!started) return;
    let i = 0;
    const id = setInterval(() => {
      i++;
      setDisplayed(text.slice(0, i));
      if (i >= text.length) clearInterval(id);
    }, speed);
    return () => clearInterval(id);
  }, [started, text, speed]);

  return (
    <div ref={ref} className={`font-mono text-[13px] ${className}`}>
      <span className="text-terminal-prompt">{prefix}</span>
      <span className="text-terminal-text">{displayed}</span>
      {started && displayed.length < text.length && (
        <span className="ml-0.5 inline-block h-4 w-[2px] animate-pulse bg-terminal-prompt align-middle" />
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   3. COMMAND OUTPUT — Command + colored output lines
   ═══════════════════════════════════════════════════════════════════ */

export function CommandOutput({
  command,
  lines,
  delay = 0,
}: {
  command: string;
  lines: { text: string; color?: string }[];
  delay?: number;
}) {
  const ref = useRef(null);
  const inView = useInView(ref, { once: true });

  return (
    <div ref={ref} className="space-y-1">
      <div className="font-mono text-[13px]">
        <span className="text-terminal-prompt">$ </span>
        <span className="text-terminal-text">{command}</span>
      </div>
      {lines.map((line, i) => (
        <motion.div
          key={i}
          initial={{ opacity: 0 }}
          animate={inView ? { opacity: 1 } : {}}
          transition={{ delay: delay + i * 0.08, duration: 0.3 }}
          className={`font-mono text-[12px] ${line.color || "text-terminal-muted"}`}
        >
          {line.text}
        </motion.div>
      ))}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   4. STATUS TABLE — Agent list with colored state badges
   ═══════════════════════════════════════════════════════════════════ */

const stateColors: Record<string, string> = {
  working: "text-terminal-command",
  idle: "text-terminal-muted",
  done: "text-terminal-success",
  error: "text-terminal-error",
  stuck: "text-terminal-prompt",
  stopped: "text-terminal-comment",
  starting: "text-terminal-command",
  scheduled: "text-terminal-prompt",
  tool: "text-terminal-command",
};

const stateDots: Record<string, string> = {
  working: "bg-terminal-command animate-pulse",
  idle: "bg-terminal-comment",
  done: "bg-terminal-success",
  error: "bg-terminal-error animate-pulse",
  stuck: "bg-terminal-prompt animate-pulse",
  stopped: "bg-terminal-comment",
  starting: "bg-terminal-command animate-pulse",
  scheduled: "bg-terminal-prompt",
  tool: "bg-terminal-command animate-pulse",
};

export interface AgentRow {
  name: string;
  role: string;
  state: string;
  detail?: string;
}

export function StatusTable({ agents }: { agents: AgentRow[] }) {
  return (
    <div className="overflow-hidden rounded-lg border border-[rgba(210,180,140,0.06)] bg-[#0D0806]">
      <div className="grid grid-cols-[1fr_100px_90px_1fr] gap-3 border-b border-[rgba(210,180,140,0.06)] px-4 py-2 text-[9px] font-bold uppercase tracking-[0.2em] text-terminal-comment">
        <div>Agent</div>
        <div>Role</div>
        <div>State</div>
        <div>Detail</div>
      </div>
      {agents.map((a, i) => (
        <motion.div
          key={a.name}
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: i * 0.06, duration: 0.3 }}
          className="grid grid-cols-[1fr_100px_90px_1fr] gap-3 border-b border-[rgba(210,180,140,0.04)] px-4 py-2.5 text-[12px] last:border-0"
        >
          <div className="flex items-center gap-2 text-terminal-text font-medium">
            <span
              className={`h-1.5 w-1.5 rounded-full ${stateDots[a.state] || "bg-terminal-comment"}`}
            />
            {a.name}
          </div>
          <div className="text-terminal-muted capitalize">{a.role}</div>
          <div
            className={`font-medium ${stateColors[a.state] || "text-terminal-muted"}`}
          >
            {a.state}
          </div>
          <div className="text-terminal-comment truncate">
            {a.detail || "—"}
          </div>
        </motion.div>
      ))}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   5. CHANNEL VIEW — Mock Slack-like channel
   ═══════════════════════════════════════════════════════════════════ */

export interface ChannelMessage {
  time: string;
  agent: string;
  message: string;
  role?: "root" | "manager" | "engineer" | "cron" | "qa" | "you";
}

const roleBadge: Record<string, string> = {
  root: "bg-[rgba(232,223,212,0.1)] text-terminal-text",
  manager: "border border-terminal-command/40 text-terminal-command",
  engineer: "border border-terminal-comment text-terminal-muted",
  qa: "border border-terminal-prompt/40 text-terminal-prompt",
  cron: "border border-terminal-prompt/40 text-terminal-prompt italic",
  you: "bg-terminal-success/20 text-terminal-success",
};

export function ChannelView({
  name,
  messages,
  members,
}: {
  name: string;
  messages: ChannelMessage[];
  members?: number;
}) {
  return (
    <div>
      <div className="mb-4 flex items-center justify-between border-b border-[rgba(210,180,140,0.06)] pb-2">
        <span className="text-[10px] font-bold uppercase tracking-[0.2em] text-terminal-muted">
          #{name}
        </span>
        {members && (
          <span className="text-[10px] text-terminal-comment">
            {members} members
          </span>
        )}
      </div>
      <div className="space-y-3">
        {messages.map((m, i) => (
          <motion.div
            key={i}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: i * 0.06, duration: 0.3 }}
            className="flex gap-3"
          >
            <span className="mt-0.5 shrink-0 text-[11px] text-terminal-comment">
              {m.time}
            </span>
            <div>
              <span
                className={`inline-flex rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-tight ${roleBadge[m.role || "engineer"]}`}
              >
                {m.agent}
              </span>
              <p className="mt-1 text-[12px] leading-relaxed text-[#D4C4B0]">
                {m.message}
              </p>
            </div>
          </motion.div>
        ))}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   6. COST TABLE — Per-agent cost with budget bars
   ═══════════════════════════════════════════════════════════════════ */

export interface CostRow {
  agent: string;
  tokensIn: string;
  tokensOut: string;
  cost: string;
  budget: string;
  percent: number;
}

function budgetColor(pct: number) {
  if (pct > 80) return "bg-terminal-error";
  if (pct > 50) return "bg-terminal-prompt";
  return "bg-terminal-success";
}

export function CostTable({
  rows,
  total,
}: {
  rows: CostRow[];
  total?: { cost: string; budget: string };
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-[rgba(210,180,140,0.06)] bg-[#0D0806]">
      <div className="grid grid-cols-[1fr_80px_80px_60px_60px_80px] gap-2 border-b border-[rgba(210,180,140,0.06)] px-4 py-2 text-[9px] font-bold uppercase tracking-[0.15em] text-terminal-comment">
        <div>Agent</div>
        <div className="text-right">Tokens In</div>
        <div className="text-right">Tokens Out</div>
        <div className="text-right">Cost</div>
        <div className="text-right">Budget</div>
        <div>Usage</div>
      </div>
      {rows.map((r, i) => (
        <motion.div
          key={r.agent}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: i * 0.08 }}
          className="grid grid-cols-[1fr_80px_80px_60px_60px_80px] gap-2 border-b border-[rgba(210,180,140,0.04)] px-4 py-2.5 text-[12px] items-center last:border-0"
        >
          <div className="text-terminal-text font-medium">{r.agent}</div>
          <div className="text-right text-terminal-muted">{r.tokensIn}</div>
          <div className="text-right text-terminal-muted">{r.tokensOut}</div>
          <div className="text-right text-terminal-text font-medium">
            {r.cost}
          </div>
          <div className="text-right text-terminal-muted">{r.budget}</div>
          <div className="flex items-center gap-2">
            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-[#1A1209]">
              <div
                className={`h-full rounded-full ${budgetColor(r.percent)}`}
                style={{ width: `${r.percent}%` }}
              />
            </div>
            <span className="text-[10px] text-terminal-comment w-7 text-right">
              {r.percent}%
            </span>
          </div>
        </motion.div>
      ))}
      {total && (
        <div className="grid grid-cols-[1fr_80px_80px_60px_60px_80px] gap-2 border-t border-[rgba(210,180,140,0.08)] bg-[rgba(232,223,212,0.02)] px-4 py-2.5 text-[12px] font-bold">
          <div className="text-[#D4C4B0]">Total</div>
          <div />
          <div />
          <div className="text-right text-terminal-text">{total.cost}</div>
          <div className="text-right text-terminal-muted">{total.budget}</div>
          <div />
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   7. TREE DIAGRAM — Animated role hierarchy
   ═══════════════════════════════════════════════════════════════════ */

export interface TreeNode {
  label: string;
  role: string;
  state?: string;
  children?: TreeNode[];
}

function TreeNodeItem({
  node,
  depth = 0,
  index = 0,
}: {
  node: TreeNode;
  depth?: number;
  index?: number;
}) {
  const roleColors: Record<string, string> = {
    "product-manager":
      "bg-[rgba(232,223,212,0.1)] text-terminal-text border-[rgba(232,223,212,0.2)]",
    manager:
      "bg-terminal-command/10 text-terminal-command border-terminal-command/20",
    engineer: "bg-[#1A1209] text-[#D4C4B0] border-terminal-comment",
    qa: "bg-terminal-prompt/10 text-terminal-prompt border-terminal-prompt/20",
    ux: "bg-terminal-prompt/10 text-terminal-prompt border-terminal-prompt/20",
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: depth * 0.15 + index * 0.08, duration: 0.4 }}
    >
      <div className="flex items-center gap-3">
        {depth > 0 && (
          <div className="flex items-center gap-1">
            {Array.from({ length: depth }).map((_, i) => (
              <div key={i} className="h-px w-3 bg-terminal-comment" />
            ))}
            <div className="h-3 w-px bg-terminal-comment" />
          </div>
        )}
        <div
          className={`inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-[12px] font-medium ${
            roleColors[node.role] || roleColors.engineer
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${stateDots[node.state || "idle"]}`}
          />
          <span>{node.label}</span>
          <span className="text-[9px] uppercase tracking-wider opacity-50">
            {node.role}
          </span>
        </div>
      </div>
      {node.children && (
        <div className="ml-4 mt-2 space-y-2 border-l border-terminal-comment pl-2">
          {node.children.map((child, i) => (
            <TreeNodeItem
              key={child.label}
              node={child}
              depth={depth + 1}
              index={i}
            />
          ))}
        </div>
      )}
    </motion.div>
  );
}

export function TreeDiagram({ root }: { root: TreeNode }) {
  return (
    <div className="space-y-2">
      <TreeNodeItem node={root} />
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   8. ANIMATED COUNTER — Count up on scroll
   ═══════════════════════════════════════════════════════════════════ */

export function AnimatedCounter({
  target,
  suffix = "",
  duration = 1200,
}: {
  target: number;
  suffix?: string;
  duration?: number;
}) {
  const [count, setCount] = useState(0);
  const ref = useRef(null);
  const inView = useInView(ref, { once: true });

  useEffect(() => {
    if (!inView) return;
    let start = 0;
    const step = target / (duration / 16);
    const id = setInterval(() => {
      start += step;
      if (start >= target) {
        setCount(target);
        clearInterval(id);
      } else {
        setCount(Math.floor(start));
      }
    }, 16);
    return () => clearInterval(id);
  }, [inView, target, duration]);

  return (
    <span ref={ref} className="tabular-nums">
      {count.toLocaleString()}
      {suffix}
    </span>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   9. SECTION WRAPPER — Scroll-triggered reveal
   ═══════════════════════════════════════════════════════════════════ */

export function RevealSection({
  children,
  className = "",
  id,
  delay = 0,
}: {
  children: ReactNode;
  className?: string;
  id?: string;
  delay?: number;
}) {
  const ref = useRef(null);
  const inView = useInView(ref, { once: true, margin: "-80px" });

  return (
    <motion.section
      ref={ref}
      id={id}
      initial={{ opacity: 0, y: 40 }}
      animate={inView ? { opacity: 1, y: 0 } : {}}
      transition={{ duration: 0.7, ease: "easeOut", delay }}
      className={className}
    >
      {children}
    </motion.section>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   10. CRON TABLE — Scheduled tasks
   ═══════════════════════════════════════════════════════════════════ */

export interface CronRow {
  name: string;
  schedule: string;
  nextRun: string;
  lastRun: string;
  status: "enabled" | "disabled" | "running";
}

export function CronTable({ jobs }: { jobs: CronRow[] }) {
  const statusStyle: Record<string, string> = {
    enabled: "text-terminal-success",
    disabled: "text-terminal-comment",
    running: "text-terminal-command animate-pulse",
  };

  return (
    <div className="overflow-hidden rounded-lg border border-[rgba(210,180,140,0.06)] bg-[#0D0806]">
      <div className="grid grid-cols-[1fr_120px_100px_100px_70px] gap-2 border-b border-[rgba(210,180,140,0.06)] px-4 py-2 text-[9px] font-bold uppercase tracking-[0.15em] text-terminal-comment">
        <div>Job</div>
        <div>Schedule</div>
        <div>Next Run</div>
        <div>Last Run</div>
        <div>Status</div>
      </div>
      {jobs.map((d, i) => (
        <motion.div
          key={d.name}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: i * 0.08 }}
          className="grid grid-cols-[1fr_120px_100px_100px_70px] gap-2 border-b border-[rgba(210,180,140,0.04)] px-4 py-2.5 text-[12px] last:border-0"
        >
          <div className="text-terminal-text font-medium">{d.name}</div>
          <div className="text-terminal-muted font-mono text-[11px]">
            {d.schedule}
          </div>
          <div className="text-terminal-muted">{d.nextRun}</div>
          <div className="text-terminal-muted">{d.lastRun}</div>
          <div className={`font-medium ${statusStyle[d.status]}`}>
            {d.status}
          </div>
        </motion.div>
      ))}
    </div>
  );
}
