import { useCallback, useEffect, useState } from "react";
import { motion } from "framer-motion";
import type { ToolNode } from "./liveTypes";
import {
  elapsed,
  durationPillClass,
  mcpBadgeColors,
  parseToolName,
  redactSecrets,
  redactValue,
  relativeTime,
} from "./liveHelpers";

/* ── EventRow ────────────────────────────────────────────────────────
   The shared hook-event row used by both the Live page agent cards and
   the agent-detail Activity tab. One event = one line:

     glyph · name · rich summary · relative time · duration

   Rows are flat and chronologically stable — they never reorder or
   regroup as statuses change. Expanding a row reveals the raw
   input/output JSON. All glyphs are monochrome stroke-currentColor
   SVGs in the mycel token style (no emoji).
─────────────────────────────────────────────────────────────────── */

/* ── Ticking timestamps ────────────────────────────────────────────── */

export function ElapsedTimer({ start }: { start: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 200);
    return () => clearInterval(id);
  }, []);
  return <>{elapsed(start)}</>;
}

export function RelativeTimestamp({ ts }: { ts: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, []);
  return (
    <span title={new Date(ts).toISOString()} className="text-[10px] text-mycel-muted font-mono tabular-nums">
      {relativeTime(ts)}
    </span>
  );
}

/* ── Copy button ───────────────────────────────────────────────────── */

export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }).catch(() => {});
  }, [text]);

  return (
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); handleCopy(); }}
      className="text-[10px] text-mycel-muted hover:text-mycel-text px-1.5 py-0.5 rounded-md border border-mycel-border hover:border-mycel-accent transition-colors shrink-0"
      aria-label="Copy to clipboard"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

/* ── Search highlight ──────────────────────────────────────────────── */

export function SearchHighlight({ text, query }: { text: string; query: string }) {
  if (!query || !text) return <>{text}</>;
  const lower = text.toLowerCase();
  const q = query.toLowerCase();
  const idx = lower.indexOf(q);
  if (idx === -1) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="bg-mycel-warning-subtle text-inherit rounded px-0.5">{text.slice(idx, idx + q.length)}</mark>
      {text.slice(idx + q.length)}
    </>
  );
}

/* ── Event kind classification ─────────────────────────────────────── */

export type EventKind =
  | "terminal"
  | "edit"
  | "read"
  | "search"
  | "agent"
  | "web"
  | "task"
  | "prompt"
  | "lifecycle"
  | "permission"
  | "mcp"
  | "tool";

export const LIFECYCLE_LABELS: Record<string, string> = {
  UserPromptSubmit: "Prompt",
  SessionStart: "Session started",
  SessionEnd: "Session ended",
  Stop: "Turn complete",
  TaskCompleted: "Task completed",
  PermissionRequest: "Waiting for permission",
  Elicitation: "Waiting for input",
  SubagentStart: "Subagent started",
  SubagentStop: "Subagent stopped",
};

export function eventGlyphKind(toolName: string): EventKind {
  if (toolName === "Bash" || toolName === "bash" || toolName === "BashOutput") return "terminal";
  if (toolName === "Edit" || toolName === "Write" || toolName === "NotebookEdit") return "edit";
  if (toolName === "Read") return "read";
  if (toolName === "Grep" || toolName === "Glob" || toolName === "ToolSearch" || toolName === "LSP") return "search";
  if (toolName === "WebFetch" || toolName === "WebSearch") return "web";
  if (toolName === "UserPromptSubmit") return "prompt";
  // Lifecycle before the Task* prefix check — TaskCompleted is lifecycle.
  if (toolName === "SessionStart" || toolName === "SessionEnd" || toolName === "Stop" || toolName === "TaskCompleted") return "lifecycle";
  if (toolName === "PermissionRequest" || toolName === "Elicitation") return "permission";
  if (toolName === "Agent" || toolName.startsWith("Agent:") || toolName === "SubagentStart" || toolName === "SubagentStop") return "agent";
  if (toolName.startsWith("Task")) return "task";
  if (toolName.startsWith("mcp__") || toolName.includes("__")) return "mcp";
  return "tool";
}

/* ── Monochrome event glyphs ───────────────────────────────────────── */

export function EventGlyph({ kind, size = 13, className }: { kind: EventKind; size?: number; className?: string }) {
  const common = {
    width: size,
    height: size,
    viewBox: "0 0 16 16",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.5,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    className,
    "aria-hidden": true,
  };
  switch (kind) {
    case "terminal":
      return (
        <svg {...common}>
          <path d="M3 4l3.5 4L3 12" />
          <path d="M8.5 12H13" />
        </svg>
      );
    case "edit":
      return (
        <svg {...common}>
          <path d="M11.3 2.7l2 2L6 12l-2.8.8.8-2.8z" />
        </svg>
      );
    case "read":
      return (
        <svg {...common}>
          <path d="M3 3.7c1.6-1 3.4-1 5 0 1.6-1 3.4-1 5 0v8.6c-1.6-1-3.4-1-5 0-1.6-1-3.4-1-5 0z" />
          <path d="M8 3.7v8.6" />
        </svg>
      );
    case "search":
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="4" />
          <path d="M10 10l3.5 3.5" />
        </svg>
      );
    case "agent":
      return (
        <svg {...common}>
          <circle cx="4" cy="4" r="1.7" />
          <circle cx="4" cy="12" r="1.7" />
          <circle cx="12" cy="4" r="1.7" />
          <path d="M4 5.7v4.6M12 5.7c0 3-3.5 2.6-6.3 3.3" />
        </svg>
      );
    case "web":
      return (
        <svg {...common}>
          <circle cx="8" cy="8" r="5.5" />
          <path d="M8 2.5c2 1.6 2 9.4 0 11M8 2.5c-2 1.6-2 9.4 0 11M2.5 8h11" />
        </svg>
      );
    case "task":
      return (
        <svg {...common}>
          <path d="M6 4h7M6 8h7M6 12h7" />
          <path d="M3 4h.01M3 8h.01M3 12h.01" />
        </svg>
      );
    case "prompt":
      return (
        <svg {...common}>
          <path d="M3 3.5h10v7H8.5l-3 2.8v-2.8H3z" />
        </svg>
      );
    case "lifecycle":
      return (
        <svg {...common}>
          <path d="M4 14V2.5" />
          <path d="M4 3h8L10.3 5.5 12 8H4" />
        </svg>
      );
    case "permission":
      return (
        <svg {...common}>
          <path d="M8 2.8l5.8 10.2H2.2z" />
          <path d="M8 7v3M8 11.6v.01" />
        </svg>
      );
    case "mcp":
      return (
        <svg {...common}>
          <path d="M6 2v3M10 2v3" />
          <path d="M4.5 5h7v3a3.5 3.5 0 01-7 0z" />
          <path d="M8 11.5V14" />
        </svg>
      );
    default:
      return (
        <svg {...common}>
          <path d="M13.6 4.6a3.4 3.4 0 01-4.5 4.1l-4.9 4.9a1.35 1.35 0 01-1.9-1.9l4.9-4.9a3.4 3.4 0 014.1-4.5L9.2 4.4l2.4 2.4z" />
        </svg>
      );
  }
}

/* ── Rich summaries ────────────────────────────────────────────────── */

/** Compact a long absolute path: keep the basename intact, shorten the
 *  directory to its last two segments. */
export function compactPath(path: string): { dir: string; base: string } {
  const idx = path.lastIndexOf("/");
  if (idx <= 0) return { dir: "", base: path };
  let dir = path.slice(0, idx + 1);
  const base = path.slice(idx + 1);
  if (dir.length > 40) {
    const segs = dir.split("/").filter(Boolean);
    dir = "…/" + segs.slice(-2).join("/") + "/";
  }
  return { dir, base };
}

/** True when args look like a single filesystem path. */
function looksLikePath(args: string): boolean {
  return args.length > 0 && args.includes("/") && !args.includes(" ") && !args.includes("\n");
}

function PathSummary({ path, query }: { path: string; query: string }) {
  // With an active search, fall back to a flat highlighted string so the
  // match is visible wherever it lands in the path.
  if (query) {
    return (
      <span className="font-mono text-[12px] text-mycel-muted min-w-0 truncate" title={path}>
        <SearchHighlight text={path} query={query} />
      </span>
    );
  }
  const { dir, base } = compactPath(path);
  return (
    <span className="font-mono text-[12px] min-w-0 truncate" title={path}>
      <span className="text-mycel-muted">{dir}</span>
      <span className="text-mycel-text">{base}</span>
    </span>
  );
}

function EventSummary({ node, kind, query }: { node: ToolNode; kind: EventKind; query: string }) {
  // Failed events lead with the error — that's the load-bearing bit.
  if (node.status === "failed" && node.error) {
    const err = redactSecrets(node.error.length > 160 ? node.error.slice(0, 157) + "…" : node.error);
    return (
      <span className="font-mono text-[12px] text-mycel-error min-w-0 truncate" title={err}>
        <SearchHighlight text={err} query={query} />
      </span>
    );
  }

  const args = redactSecrets(node.args ?? "");
  if (!args) return null;

  if ((kind === "edit" || kind === "read") && looksLikePath(args)) {
    return <PathSummary path={args} query={query} />;
  }
  if (kind === "prompt") {
    return (
      <span className="text-[12px] text-mycel-text min-w-0 truncate" title={args}>
        <SearchHighlight text={args} query={query} />
      </span>
    );
  }
  return (
    <span className="font-mono text-[12px] text-mycel-muted min-w-0 truncate" title={args}>
      <SearchHighlight text={args} query={query} />
    </span>
  );
}

/* ── Event name ────────────────────────────────────────────────────── */

function EventName({ node, kind, query }: { node: ToolNode; kind: EventKind; query: string }) {
  const label = LIFECYCLE_LABELS[node.toolName];
  if (label) {
    return (
      <span className="text-[12px] text-mycel-text font-medium shrink-0">
        {label}
      </span>
    );
  }
  if (kind === "mcp") {
    const parsed = parseToolName(node.toolName);
    if (parsed.mcpServer && parsed.mcpFunction) {
      return (
        <span className="inline-flex items-center gap-1.5 shrink-0 min-w-0">
          <span className={`px-1.5 py-0.5 rounded-full text-[10px] font-mono leading-none ${mcpBadgeColors(parsed.mcpServer)}`}>
            {parsed.mcpServer}
          </span>
          <span className="font-mono text-[12px] text-mycel-text font-medium">
            <SearchHighlight text={parsed.mcpFunction} query={query} />
          </span>
        </span>
      );
    }
  }
  return (
    <span className="font-mono text-[12px] text-mycel-text font-semibold shrink-0">
      <SearchHighlight text={parseToolName(node.toolName).display} query={query} />
    </span>
  );
}

/* ── The row ───────────────────────────────────────────────────────── */

function glyphColorClass(status: ToolNode["status"]): string {
  if (status === "running") return "text-mycel-accent animate-pulse";
  if (status === "failed") return "text-mycel-error";
  return "text-mycel-muted";
}

export function EventRow({ node, searchQuery = "" }: { node: ToolNode; searchQuery?: string }) {
  const [expanded, setExpanded] = useState(false);
  const kind = eventGlyphKind(node.toolName);
  const hasDetails = !!(node.fullInput || node.fullOutput || node.error);
  const inputJson = node.fullInput ? JSON.stringify(redactValue(node.fullInput), null, 2) : "";
  const outputJson = node.fullOutput ? JSON.stringify(redactValue(node.fullOutput), null, 2) : "";

  return (
    <div className={`border-b border-mycel-border last:border-b-0 ${node.status === "failed" ? "bg-mycel-error-subtle" : ""}`}>
      <button
        type="button"
        className="group flex items-center gap-2.5 py-1.5 px-3 w-full text-left hover:bg-mycel-surface-hover cursor-pointer transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent"
        onClick={() => setExpanded(!expanded)}
        aria-label={`${expanded ? "Collapse" : "Expand"} ${node.toolName} event`}
        aria-expanded={expanded}
      >
        {/* Expand affordance — dot when there is nothing to expand */}
        <motion.span
          className="text-mycel-muted text-[9px] select-none shrink-0 w-2.5 text-center"
          animate={{ rotate: hasDetails && expanded ? 90 : 0 }}
          transition={{ duration: 0.15 }}
        >
          {hasDetails ? "▶" : "·"}
        </motion.span>

        {/* Kind glyph — monochrome, colored by status */}
        <span className={`inline-flex items-center justify-center shrink-0 ${glyphColorClass(node.status)}`}>
          <EventGlyph kind={kind} />
        </span>

        <EventName node={node} kind={kind} query={searchQuery} />
        <EventSummary node={node} kind={kind} query={searchQuery} />

        <span className="flex items-center gap-2 shrink-0 ml-auto pl-2">
          <RelativeTimestamp ts={node.startTime} />
          {node.status === "running" ? (
            <span className="text-[11px] tabular-nums font-mono px-1.5 py-0.5 rounded-md bg-mycel-accent-subtle text-mycel-accent">
              <ElapsedTimer start={node.startTime} />
            </span>
          ) : node.endTime != null ? (
            <span className={`text-[11px] tabular-nums font-mono px-1.5 py-0.5 rounded-md ${durationPillClass(node.startTime, node.endTime)}`}>
              {elapsed(node.startTime, node.endTime)}
            </span>
          ) : null}
        </span>
      </button>

      {expanded && node.error && (
        <div className="text-[11px] font-mono px-3 py-2 bg-mycel-surface mx-3 mb-1 rounded-md overflow-x-auto max-h-48 overflow-y-auto">
          <span className="text-[10px] text-mycel-error uppercase tracking-wide font-semibold block mb-1">Error</span>
          <pre className="whitespace-pre-wrap break-all text-mycel-error">{redactSecrets(node.error)}</pre>
        </div>
      )}

      {expanded && !!node.fullInput && (
        <div className="text-[11px] font-mono px-3 py-2 bg-mycel-surface mx-3 mb-1 rounded-md overflow-x-auto max-h-48 overflow-y-auto">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-mycel-muted uppercase tracking-wide font-semibold">Input</span>
            <CopyButton text={inputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-mycel-muted">{inputJson}</pre>
        </div>
      )}

      {expanded && !!node.fullOutput && (
        <div className="text-[11px] font-mono px-3 py-2 bg-mycel-surface mx-3 mb-1 rounded-md overflow-x-auto max-h-48 overflow-y-auto">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-mycel-success uppercase tracking-wide font-semibold">Output</span>
            <CopyButton text={outputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-mycel-success">{outputJson}</pre>
        </div>
      )}
    </div>
  );
}
