import { useCallback, useEffect, useMemo, useState, memo } from "react";
import { motion, AnimatePresence } from "framer-motion";
import type {
  AgentActivity,
  AggregatedNode,
  DisplayNode,
  DrillDownTab,
  FilterType,
  RawEvent,
  TaskItem,
  ToolNode,
} from "./liveTypes";
import { isAggregatedNode, AUTO_COLLAPSE_MS } from "./liveTypes";
import {
  aggregateNodes,
  durationColorClass,
  durationPillClass,
  elapsed,
  estimateCost,
  idleDuration,
  mcpBadgeColors,
  mcpServerIcon,
  nodeMatchesSearch,
  parseToolName,
  redactSecrets,
  redactValue,
  relativeTime,
  sortNodes,
  stateBadgeClass,
  toolIcon,
} from "./liveHelpers";

/* ── State Dots ────────────────────────────────────────────────────── */

export function StateDot({ state }: { state: string }) {
  if (state === "working")
    return (
      <span className="relative flex h-2.5 w-2.5">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-bc-success opacity-75" />
        <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-bc-success" />
      </span>
    );
  if (state === "stuck")
    return (
      <span className="relative flex h-2.5 w-2.5">
        <span className="absolute inline-flex h-full w-full animate-pulse rounded-full bg-bc-warning opacity-50" />
        <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-bc-warning" />
      </span>
    );
  if (state === "error" || state === "stopped")
    return <span className="inline-flex h-2.5 w-2.5 rounded-full bg-bc-error" />;
  return <span className="inline-flex h-2.5 w-2.5 rounded-full bg-bc-muted/40" />;
}

export function ToolDot({ status }: { status: ToolNode["status"] }) {
  if (status === "running")
    return (
      <span className="relative flex h-2 w-2 mt-[5px] shrink-0">
        <span className="absolute inline-flex h-full w-full animate-pulse rounded-full bg-bc-accent opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-bc-accent" />
      </span>
    );
  if (status === "failed")
    return <span className="inline-flex h-2 w-2 mt-[5px] shrink-0 rounded-full bg-bc-error" />;
  return <span className="inline-flex h-2 w-2 mt-[5px] shrink-0 rounded-full bg-bc-success" />;
}

/* ── Elapsed Timer ─────────────────────────────────────────────────── */

export function ElapsedTimer({ start }: { start: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 200);
    return () => clearInterval(id);
  }, []);
  return <>{elapsed(start)}</>;
}

/* ── Relative Timestamp ───────────────────────────────────────────── */

export function RelativeTimestamp({ ts }: { ts: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, []);
  return (
    <span title={new Date(ts).toISOString()} className="text-[10px] text-bc-muted/60 font-mono tabular-nums">
      {relativeTime(ts)}
    </span>
  );
}

/* ── Idle Timer ───────────────────────────────────────────────────── */

export function IdleTimer({ lastEventTime }: { lastEventTime: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, []);
  return <>{idleDuration(lastEventTime)}</>;
}

/* ── Copy Button ───────────────────────────────────────────────────── */

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
      className="text-[10px] text-bc-muted hover:text-bc-text px-1.5 py-0.5 rounded border border-bc-border/40 hover:border-bc-accent transition-colors shrink-0"
      aria-label="Copy to clipboard"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

/* ── MCP Badge ─────────────────────────────────────────────────────── */

export function McpBadge({ server, func }: { server: string; func: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-mono ${mcpBadgeColors(server)}`}>
        <span aria-hidden="true">{mcpServerIcon(server)}</span>
        <span>{server}</span>
      </span>
      <span className="font-mono text-[13px] text-bc-text font-medium">{func}</span>
    </span>
  );
}

/* ── Search Highlight ──────────────────────────────────────────────── */

export function SearchHighlight({ text, query }: { text: string; query: string }) {
  if (!query || !text) return <>{text}</>;
  const lower = text.toLowerCase();
  const q = query.toLowerCase();
  const idx = lower.indexOf(q);
  if (idx === -1) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="bg-yellow-500/20 text-inherit rounded px-0.5">{text.slice(idx, idx + q.length)}</mark>
      {text.slice(idx + q.length)}
    </>
  );
}

/* ── Tool Name Display ─────────────────────────────────────────────── */

export function ToolNameDisplay({ toolName, searchQuery }: { toolName: string; searchQuery?: string }) {
  const parsed = parseToolName(toolName);
  if (parsed.type === "mcp" && parsed.mcpServer && parsed.mcpFunction) {
    return <McpBadge server={parsed.mcpServer} func={parsed.mcpFunction} />;
  }
  return (
    <span className="inline-flex items-center gap-1">
      <span className="text-[12px]" aria-hidden="true">{toolIcon(toolName)}</span>
      <span className="font-mono text-[13px] text-bc-text font-semibold">
        {searchQuery ? <SearchHighlight text={parsed.display} query={searchQuery} /> : parsed.display}
      </span>
    </span>
  );
}

/* ── Tool Node Row ─────────────────────────────────────────────────── */

export function ToolNodeRow({ node, depth = 0, searchQuery = "" }: { node: ToolNode; depth?: number; searchQuery?: string }) {
  const [expanded, setExpanded] = useState(false);
  const indent = depth * 20;
  const hasDetails = !!(node.fullInput || node.fullOutput || node.children.length > 0);
  const isSubagentSpawn = node.toolName === "Agent" || node.toolName.startsWith("Agent:");

  // Subagent tree: use AgentTreeNode for nested rendering
  if (isSubagentSpawn) {
    return <AgentTreeNode node={node} depth={depth} />;
  }

  const inputJson = node.fullInput ? JSON.stringify(redactValue(node.fullInput), null, 2) : "";
  const outputJson = node.fullOutput ? JSON.stringify(redactValue(node.fullOutput), null, 2) : "";

  return (
    <>
      <button
        type="button"
        className={`group flex items-center gap-2 py-1.5 px-3 w-full text-left hover:bg-bc-surface-hover cursor-pointer transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent ${node.status === "failed" ? "!bg-bc-error/5 hover:!bg-bc-error/10" : ""}`}
        style={{ paddingLeft: `${indent + 12}px` }}
        onClick={() => setExpanded(!expanded)}
        aria-label={`${expanded ? "Collapse" : "Expand"} tool ${node.toolName}`}
      >
        <span className="text-bc-muted text-xs select-none shrink-0">
          {depth > 0 ? "\u251C\u2500" : ""}
        </span>
        {/* Animated chevron */}
        <motion.span
          className="text-bc-muted/50 text-[10px] select-none shrink-0 w-3 text-center group-hover:text-bc-muted"
          animate={{ rotate: hasDetails ? (expanded ? 90 : 0) : 0 }}
          transition={{ duration: 0.15 }}
        >
          {hasDetails ? "\u25B6" : "\u00B7"}
        </motion.span>
        {/* Tool icon in rounded container */}
        <span className="inline-flex items-center justify-center h-5 w-5 rounded bg-bc-surface border border-bc-border/60 shrink-0">
          <ToolDot status={node.status} />
        </span>
        <span className="shrink-0">
          <ToolNameDisplay toolName={node.toolName} searchQuery={searchQuery} />
        </span>
        {node.args && (
          <span className="text-[12px] text-bc-muted font-mono min-w-0 flex-1 break-words" title={redactSecrets(node.args)}>
            {searchQuery ? <SearchHighlight text={redactSecrets(node.args)} query={searchQuery} /> : redactSecrets(node.args)}
          </span>
        )}
        <span className="flex items-center gap-2 shrink-0">
          <RelativeTimestamp ts={node.startTime} />
          {/* Duration pill with color-coded background */}
          <span className={`text-[11px] tabular-nums font-mono px-1.5 py-0.5 rounded-md ${node.status === "running" ? "bg-bc-muted/10 text-bc-muted" : durationPillClass(node.startTime, node.endTime)}`}>
            {node.status === "running" ? (
              <ElapsedTimer start={node.startTime} />
            ) : (
              elapsed(node.startTime, node.endTime)
            )}
          </span>
        </span>
      </button>

      {node.error && (
        <div
          className="text-[11px] text-bc-error/80 font-mono px-3 py-0.5"
          style={{ paddingLeft: `${indent + 40}px` }}
        >
          {redactSecrets(node.error.length > 120 ? node.error.slice(0, 117) + "..." : node.error)}
        </div>
      )}

      {expanded && node.fullInput && (
        <div
          className="text-[11px] font-mono px-3 py-1 bg-bc-surface mx-3 mb-1 rounded overflow-x-auto max-h-48 overflow-y-auto"
          style={{ marginLeft: `${indent + 12}px` }}
        >
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-bc-muted uppercase tracking-wide font-semibold">Input</span>
            <CopyButton text={inputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-bc-muted">
            {inputJson}
          </pre>
        </div>
      )}

      {expanded && node.fullOutput && (
        <div
          className="text-[11px] font-mono px-3 py-1 bg-bc-surface mx-3 mb-1 rounded overflow-x-auto max-h-48 overflow-y-auto"
          style={{ marginLeft: `${indent + 12}px` }}
        >
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-bc-success uppercase tracking-wide font-semibold">Output</span>
            <CopyButton text={outputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-bc-success/80">
            {outputJson}
          </pre>
        </div>
      )}

      {node.children.map((child) => (
        <ToolNodeRow key={child.id} node={child} depth={depth + 1} searchQuery={searchQuery} />
      ))}
    </>
  );
}

/* ── Agent Tree Node (recursive subagent nesting) ──────────────────── */

export function AgentTreeNode({ node, depth = 0 }: { node: ToolNode; depth?: number }) {
  const [expanded, setExpanded] = useState(true);
  const indent = depth * 16;
  const duration = node.endTime ? elapsed(node.startTime, node.endTime) : undefined;
  const childCount = node.children.length;

  const subagentChildren = node.children.filter(
    (c) => c.toolName === "Agent" || c.toolName.startsWith("Agent:"),
  );
  const toolChildren = node.children.filter(
    (c) => c.toolName !== "Agent" && !c.toolName.startsWith("Agent:"),
  );

  return (
    <div style={{ marginLeft: `${indent}px` }}>
      {/* Subagent header */}
      <button
        type="button"
        className="group flex items-start gap-2 py-1.5 px-3 w-full text-left hover:bg-bc-surface-hover cursor-pointer transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent bg-blue-950/20 rounded-md my-0.5"
        onClick={() => setExpanded(!expanded)}
        aria-label={`${expanded ? "Collapse" : "Expand"} subagent ${node.toolName}`}
      >
        <span className="text-bc-muted/50 text-[10px] select-none mt-[3px] shrink-0 w-3 text-center group-hover:text-bc-muted">
          {childCount > 0 ? (expanded ? "\u25BC" : "\u25B6") : "\u00B7"}
        </span>
        <ToolDot status={node.status} />
        <span className="text-[13px]" aria-hidden="true">{"\uD83E\uDD16"}</span>
        <span className="font-mono text-[13px] text-bc-text font-semibold">{node.toolName}</span>
        {node.args && (
          <span className="text-[12px] text-bc-muted truncate max-w-[300px] font-mono italic">
            &ldquo;{node.args}&rdquo;
          </span>
        )}
        <span className="ml-auto flex items-center gap-2 shrink-0">
          <RelativeTimestamp ts={node.startTime} />
          {node.status === "running" ? (
            <span className="text-[11px] text-blue-400 font-mono tabular-nums">
              {"\u23F1"} <ElapsedTimer start={node.startTime} />
            </span>
          ) : duration ? (
            <span className={`text-[11px] font-mono tabular-nums ${durationColorClass(node.startTime, node.endTime)}`}>
              {"\u23F1"} {duration}
            </span>
          ) : null}
          {node.status === "completed" && (
            <span className="text-[10px] text-bc-success font-mono">{"\u2713"}</span>
          )}
          {node.status === "failed" && (
            <span className="text-[10px] text-bc-error font-mono">{"\u2717"}</span>
          )}
        </span>
      </button>

      {/* Tree children with connector lines */}
      {expanded && childCount > 0 && (
        <div className="border-l-2 border-bc-muted/30 ml-4 pl-3">
          {toolChildren.map((child, idx) => {
            const isLast = idx === toolChildren.length - 1 && subagentChildren.length === 0;
            return (
              <div key={child.id} className="flex items-start gap-0">
                <span className="text-bc-muted/30 text-xs select-none mt-[3px] shrink-0 w-4">
                  {isLast ? "\u2514\u2500" : "\u251C\u2500"}
                </span>
                <div className="flex-1 min-w-0">
                  <ToolNodeRow node={child} depth={0} />
                </div>
              </div>
            );
          })}

          {/* Nested subagent children (recursive) */}
          {subagentChildren.map((child, idx) => {
            const isLast = idx === subagentChildren.length - 1;
            return (
              <div key={child.id} className="flex items-start gap-0">
                <span className="text-bc-muted/30 text-xs select-none mt-[3px] shrink-0 w-4">
                  {isLast ? "\u2514\u2500" : "\u251C\u2500"}
                </span>
                <div className="flex-1 min-w-0">
                  <AgentTreeNode node={child} depth={0} />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ── Aggregated Node Row ───────────────────────────────────────────── */

export function AggregatedChildRow({ child, searchQuery = "" }: { child: ToolNode; searchQuery?: string }) {
  const [showRawJson, setShowRawJson] = useState(false);

  const fullEventJson = JSON.stringify(redactValue({
    id: child.id,
    toolName: child.toolName,
    args: child.args,
    status: child.status,
    startTime: child.startTime,
    endTime: child.endTime,
    error: child.error,
    fullInput: child.fullInput,
    fullOutput: child.fullOutput,
  }), null, 2);

  return (
    <div>
      <ToolNodeRow node={child} depth={1} searchQuery={searchQuery} />
      {!!(child.fullInput || child.fullOutput) && (
        <div className="ml-8 mb-1">
          <button
            type="button"
            onClick={() => setShowRawJson(!showRawJson)}
            className="text-[10px] text-bc-muted hover:text-bc-accent font-mono transition-colors px-2 py-0.5 rounded border border-bc-border/30 hover:border-bc-accent/50"
          >
            {showRawJson ? "Hide Raw JSON" : "Raw JSON"}
          </button>
          {showRawJson && (
            <div className="mt-1 text-[11px] font-mono px-3 py-2 bg-bc-bg rounded border border-bc-border/30 overflow-x-auto max-h-64 overflow-y-auto">
              <div className="flex justify-end mb-1">
                <CopyButton text={fullEventJson} />
              </div>
              <pre className="whitespace-pre-wrap break-all text-bc-muted">
                {fullEventJson}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function AggregatedNodeRow({ node, searchQuery = "" }: { node: AggregatedNode; searchQuery?: string }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <button
        type="button"
        className="group flex items-start gap-2 py-1 px-3 w-full text-left hover:bg-bc-surface-hover cursor-pointer transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent bg-bc-surface/50"
        onClick={() => setExpanded(!expanded)}
        aria-label={`${expanded ? "Collapse" : "Expand"} aggregated ${node.toolName} (${node.count} calls)`}
      >
        <span className="text-bc-muted text-xs select-none mt-[3px] shrink-0">
          {expanded ? "\u25BC" : "\u25B6"}
        </span>
        <span className="inline-flex h-2 w-2 mt-[5px] shrink-0 rounded-full bg-bc-success" />
        <ToolNameDisplay toolName={node.toolName} />
        <span className="text-[12px] font-mono font-semibold text-bc-accent px-1.5 py-0 rounded bg-bc-accent/10">
          &times;{node.count}
        </span>
        <span className="text-[11px] text-bc-muted font-mono tabular-nums flex-1 min-w-0">
          {node.count} total
          {node.totalDuration > 0 && <> &middot; {elapsed(0, node.totalDuration)}</>}
          {node.totalDuration > 0 && node.count > 1 && (
            <> &middot; avg {elapsed(0, Math.round(node.totalDuration / node.count))}</>
          )}
          {node.totalTokens > 0 && <> &middot; {node.totalTokens.toLocaleString()} tok</>}
          {node.failCount > 0 && (
            <span className="text-bc-error"> &middot; {node.failCount} failed</span>
          )}
          <> &middot; {node.successCount}/{node.count} ok</>
        </span>
      </button>

      {expanded && (
        <div className="border-l-2 border-bc-border/40 ml-6">
          {node.children.map((child) => (
            <AggregatedChildRow key={child.id} child={child} searchQuery={searchQuery} />
          ))}
        </div>
      )}
    </>
  );
}

/* ── Display Node Row ──────────────────────────────────────────────── */

export function DisplayNodeRow({ node, searchQuery = "" }: { node: DisplayNode; searchQuery?: string }) {
  if (isAggregatedNode(node)) {
    return <AggregatedNodeRow node={node} searchQuery={searchQuery} />;
  }
  return <ToolNodeRow node={node} searchQuery={searchQuery} />;
}

/* ── Agent Drill-Down View ─────────────────────────────────────────── */

export function DrillDownTasksSection({ tasks, agentName }: { tasks: Map<string, TaskItem>; agentName: string }) {
  const [collapsed, setCollapsed] = useState(false);

  const agentTasks = useMemo(() => {
    const result: TaskItem[] = [];
    for (const [, task] of tasks) {
      if (task.owner === agentName || !task.owner) {
        if (task.status !== "deleted") result.push(task);
      }
    }
    return result;
  }, [tasks, agentName]);

  const completedCount = agentTasks.filter((t) => t.status === "completed").length;
  const total = agentTasks.length;
  const progressPct = total > 0 ? Math.round((completedCount / total) * 100) : 0;

  if (total === 0) return null;

  return (
    <div className="rounded-lg border border-bc-border bg-bc-surface overflow-hidden mb-4">
      <button
        type="button"
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 w-full px-4 py-2.5 text-left hover:bg-bc-surface-hover transition-colors"
      >
        <span className="text-bc-muted/50 text-[10px] select-none w-3 text-center">
          {collapsed ? "\u25B6" : "\u25BC"}
        </span>
        <span className="text-[13px]">{"\u2705"}</span>
        <span className="text-sm font-semibold text-bc-text">Tasks</span>
        <span className="text-xs text-bc-muted font-mono tabular-nums">
          ({completedCount}/{total} complete)
        </span>
        <span className="flex-1 mx-2 h-1.5 bg-bc-bg rounded-full overflow-hidden max-w-[200px]">
          <span
            className="h-full bg-bc-success rounded-full transition-all duration-300"
            style={{ width: `${progressPct}%` }}
          />
        </span>
      </button>

      {!collapsed && (
        <div className="border-t border-bc-border/60 px-4 py-2 space-y-1.5">
          {agentTasks.map((task) => {
            const isBlocked = task.blockedBy && task.blockedBy.length > 0 && task.status !== "completed";
            const borderColor = task.status === "completed" ? "border-l-bc-success" :
              task.status === "in_progress" ? "border-l-bc-accent" :
              task.status === "pending" ? "border-l-bc-muted" :
              "border-l-bc-error";
            return (
              <div key={task.id} className={`flex items-center gap-2 py-1.5 px-2.5 rounded-md bg-bc-bg border-l-2 ${borderColor} ${isBlocked ? "opacity-50" : ""}`}>
                <span className="text-[11px] text-bc-muted font-mono shrink-0">#{task.id}</span>
                <span className={`text-sm font-mono min-w-0 ${
                  task.status === "completed" ? "line-through text-bc-muted/60" :
                  task.status === "in_progress" ? "text-bc-accent font-semibold" :
                  "text-bc-text"
                }`}>
                  {task.subject.length > 80 ? task.subject.slice(0, 77) + "..." : task.subject}
                </span>
                {isBlocked && (
                  <span className="text-[10px] text-bc-warning/80 font-mono shrink-0">
                    Blocked by {task.blockedBy!.map((b) => `#${b}`).join(", ")}
                  </span>
                )}
                <span className={`text-[10px] px-2 py-0.5 rounded-full font-mono capitalize shrink-0 ml-auto ${
                  task.status === "completed" ? "bg-bc-success/15 text-bc-success" :
                  task.status === "in_progress" ? "bg-bc-accent/15 text-bc-accent" :
                  task.status === "pending" ? "bg-bc-muted/15 text-bc-muted" :
                  "bg-bc-error/15 text-bc-error"
                }`}>
                  {task.status.replace("_", " ")}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ── Drill-Down: Full-width individual event row ──────────────────── */

export function DrillDownEventRow({ node }: { node: ToolNode }) {
  const [expanded, setExpanded] = useState(false);
  const hasDetails = !!(node.fullInput || node.fullOutput);
  const inputJson = node.fullInput ? JSON.stringify(redactValue(node.fullInput), null, 2) : "";
  const outputJson = node.fullOutput ? JSON.stringify(redactValue(node.fullOutput), null, 2) : "";

  return (
    <div className={`border-b border-bc-border/30 ${node.status === "failed" ? "bg-bc-error/5" : ""}`}>
      <button
        type="button"
        className="group flex items-center gap-3 py-2.5 px-4 w-full text-left hover:bg-bc-surface-hover cursor-pointer transition-colors"
        onClick={() => setExpanded(!expanded)}
        aria-label={`${expanded ? "Collapse" : "Expand"} ${node.toolName} event`}
      >
        {/* Animated chevron */}
        <motion.span
          className="text-bc-muted/50 text-[10px] select-none shrink-0 w-3 text-center group-hover:text-bc-muted"
          animate={{ rotate: hasDetails ? (expanded ? 90 : 0) : 0 }}
          transition={{ duration: 0.15 }}
        >
          {hasDetails ? "\u25B6" : "\u00B7"}
        </motion.span>
        {/* Icon container */}
        <span className="inline-flex items-center justify-center h-6 w-6 rounded-md bg-bc-surface border border-bc-border/60 shrink-0">
          <ToolDot status={node.status} />
        </span>
        <span className="shrink-0">
          <ToolNameDisplay toolName={node.toolName} />
        </span>
        <span className="text-[12px] text-bc-muted font-mono min-w-0 flex-1 break-words">
          {redactSecrets(node.args)}
        </span>
        <span className="flex items-center gap-2 shrink-0">
          <RelativeTimestamp ts={node.startTime} />
          <span className={`text-[11px] tabular-nums font-mono px-1.5 py-0.5 rounded-md ${node.status === "running" ? "bg-bc-muted/10 text-bc-muted" : durationPillClass(node.startTime, node.endTime)}`}>
            {node.status === "running" ? <ElapsedTimer start={node.startTime} /> : elapsed(node.startTime, node.endTime)}
          </span>
        </span>
      </button>

      {node.error && (
        <div className="text-[11px] text-bc-error/80 font-mono px-4 py-0.5 ml-8">
          {redactSecrets(node.error.length > 200 ? node.error.slice(0, 197) + "..." : node.error)}
        </div>
      )}

      {expanded && !!node.fullInput && (
        <div className="text-[11px] font-mono px-4 py-2 bg-bc-surface mx-4 mb-1 rounded overflow-x-auto max-h-64 overflow-y-auto">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-bc-muted uppercase tracking-wide font-semibold">Input</span>
            <CopyButton text={inputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-bc-muted">{inputJson}</pre>
        </div>
      )}

      {expanded && !!node.fullOutput && (
        <div className="text-[11px] font-mono px-4 py-2 bg-bc-surface mx-4 mb-1 rounded overflow-x-auto max-h-64 overflow-y-auto">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-bc-success uppercase tracking-wide font-semibold">Output</span>
            <CopyButton text={outputJson} />
          </div>
          <pre className="whitespace-pre-wrap break-all text-bc-success/80">{outputJson}</pre>
        </div>
      )}
    </div>
  );
}

/* ── Drill-Down: Raw event type badge colors ──────────────────────── */

export function rawEventBadgeColor(eventType: string): string {
  if (eventType === "PreToolUse") return "bg-blue-900/40 text-blue-300";
  if (eventType === "PostToolUse") return "bg-emerald-900/40 text-emerald-300";
  if (eventType === "PostToolUseFailure") return "bg-red-900/40 text-red-300";
  if (eventType === "UserPromptSubmit") return "bg-purple-900/40 text-purple-300";
  if (eventType.startsWith("Subagent")) return "bg-amber-900/40 text-amber-300";
  if (eventType === "PermissionRequest" || eventType === "Elicitation") return "bg-yellow-900/40 text-yellow-300";
  if (eventType === "SessionStart" || eventType === "SessionEnd") return "bg-zinc-700 text-zinc-300";
  return "bg-zinc-800 text-zinc-400";
}

export function AgentDrillDown({
  activity,
  rawEvents,
  tasks,
  onBack,
}: {
  activity: AgentActivity;
  rawEvents: RawEvent[];
  tasks: Map<string, TaskItem>;
  onBack: () => void;
}) {
  const [activeTab, setActiveTab] = useState<DrillDownTab>("live");
  const [rawExpanded, setRawExpanded] = useState<Set<string>>(new Set());

  const cost = estimateCost(activity);

  // Show ALL events individually in drill-down — no aggregation
  const allNodes = useMemo(() => {
    return [...activity.nodes].sort((a, b) => b.startTime - a.startTime);
  }, [activity.nodes]);

  const toggleRawExpanded = useCallback((key: string) => {
    setRawExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  // Reverse raw events so newest at top
  const reversedRawEvents = useMemo(() => [...rawEvents].reverse(), [rawEvents]);

  const tabs: { key: DrillDownTab; label: string }[] = [
    { key: "live", label: "Live Stream" },
    { key: "raw", label: "Raw Stream" },
  ];

  return (
    <div className="flex flex-col h-full">
      {/* Header bar */}
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1.5 text-sm text-bc-muted hover:text-bc-text px-2 py-1 rounded border border-bc-border hover:border-bc-accent transition-colors shrink-0"
        >
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M8 2l-4 4 4 4" />
          </svg>
          Back
        </button>
        <StateDot state={activity.state} />
        <span className="text-lg font-bold text-bc-text">{activity.name}</span>
        <span className="text-xs text-bc-muted font-mono">({activity.role})</span>
        <span className="text-xs text-bc-muted capitalize font-mono">{activity.state}</span>
        {activity.tokens > 0 && (
          <span className="text-xs text-bc-muted font-mono tabular-nums">
            {activity.tokens.toLocaleString()} tok
          </span>
        )}
        {cost > 0 && (
          <span className="text-xs text-bc-success font-mono tabular-nums">
            ${cost.toFixed(2)}
          </span>
        )}
        {activity.task && (
          <span className="text-xs text-bc-muted truncate max-w-[400px]">{activity.task}</span>
        )}
      </div>

      {/* Tasks section — above tabs */}
      <DrillDownTasksSection tasks={tasks} agentName={activity.name} />

      {/* Tabs */}
      <div className="flex items-center gap-0 border-b border-bc-border mb-4">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab.key
                ? "border-bc-accent text-bc-text"
                : "border-transparent text-bc-muted hover:text-bc-text hover:border-bc-border"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {/* Live Stream tab — ALL events individually, no aggregation */}
        {activeTab === "live" && (
          <div>
            {allNodes.length === 0 ? (
              <div className="text-sm text-bc-muted italic py-8 text-center">
                No tool events yet for this agent.
              </div>
            ) : (
              allNodes.map((node) => (
                <DrillDownEventRow key={node.id} node={node} />
              ))
            )}
          </div>
        )}

        {/* Raw Stream tab — timestamped JSON blocks, newest at top */}
        {activeTab === "raw" && (
          <div className="space-y-1">
            {reversedRawEvents.length === 0 ? (
              <div className="text-sm text-bc-muted italic py-8 text-center">
                No raw events captured for this agent yet.
              </div>
            ) : (
              reversedRawEvents.map((evt) => {
                const evtKey = `${evt.timestamp}-${evt.eventType}`;
                const isOpen = rawExpanded.has(evtKey);
                const jsonStr = JSON.stringify(redactValue(evt.raw), null, 2);
                return (
                  <div key={evtKey} className="border border-bc-border/40 rounded bg-bc-surface overflow-hidden">
                    <button
                      type="button"
                      onClick={() => toggleRawExpanded(evtKey)}
                      className="flex items-center gap-2 w-full px-3 py-1.5 text-left hover:bg-bc-surface-hover transition-colors"
                    >
                      <span className="text-bc-muted/50 text-[10px] select-none w-3 text-center">
                        {isOpen ? "\u25BC" : "\u25B6"}
                      </span>
                      <span className="text-[10px] text-bc-muted font-mono tabular-nums shrink-0">
                        {new Date(evt.timestamp).toISOString().replace("T", " ").slice(0, 23)}
                      </span>
                      <span className={`text-[11px] font-mono font-medium shrink-0 px-1.5 py-0.5 rounded ${rawEventBadgeColor(evt.eventType)}`}>
                        {evt.eventType}
                      </span>
                      <span className="text-[11px] text-bc-muted font-mono truncate flex-1">
                        {jsonStr.length > 100 ? jsonStr.slice(0, 97) + "..." : jsonStr.replace(/\n/g, " ")}
                      </span>
                    </button>
                    {isOpen && (
                      <div className="border-t border-bc-border/40 px-3 py-2 bg-bc-bg">
                        <div className="flex justify-end mb-1">
                          <CopyButton text={jsonStr} />
                        </div>
                        <pre className="text-[11px] font-mono text-bc-muted whitespace-pre-wrap break-all max-h-64 overflow-y-auto">
                          {jsonStr.split("\n").map((line, i) => (
                            <span key={i} className="flex">
                              <span className="select-none text-bc-muted/30 w-8 text-right pr-3 shrink-0 tabular-nums">{i + 1}</span>
                              <span>{line}</span>
                              {"\n"}
                            </span>
                          ))}
                        </pre>
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/* ── Agent Activity Card ───────────────────────────────────────────── */

export const AgentCard = memo(function AgentCard({
  activity,
  onToggle,
  onDrillDown,
  isFilterActive,
  searchTerm,
  typeFilter,
  isPaused,
}: {
  activity: AgentActivity;
  onToggle: () => void;
  onDrillDown: () => void;
  isFilterActive: boolean;
  searchTerm: string;
  typeFilter: FilterType;
  isPaused: boolean;
}) {
  const [collapseOld, setCollapseOld] = useState(true);

  const visibleNodes = searchTerm
    ? activity.nodes.filter((n) => nodeMatchesSearch(n, searchTerm.toLowerCase()))
    : activity.nodes;

  const sortedNodes = sortNodes(visibleNodes);

  const runningCount = sortedNodes.filter((n) => n.status === "running").length;
  const errorCount = activity.nodes.filter((n) => n.status === "failed").length;
  const displayNodes = aggregateNodes(sortedNodes, collapseOld ? AUTO_COLLAPSE_MS : undefined, activity.nodes.length);
  const matchCount = searchTerm ? visibleNodes.length : 0;
  const showToolNodes = typeFilter !== "state";

  const skipAnimation = isPaused || visibleNodes.length > 5;

  const monogram = (activity.name ?? "?").charAt(0).toUpperCase();
  const cost = estimateCost(activity);
  const isIdle = activity.state === "idle" || activity.state === "stopped";

  return (
    <motion.div
      className={`rounded-lg border bg-bc-surface overflow-hidden transition-colors ${isFilterActive ? "border-bc-accent ring-1 ring-bc-accent/30" : "border-bc-border"}`}
      whileHover={{ y: -1 }}
      transition={{ duration: 0.15 }}
    >
      <div className="flex items-center">
        {/* Expand/collapse chevron for tool list */}
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); onToggle(); }}
          className="flex items-center gap-3 px-3 py-3 hover:bg-bc-surface-hover transition-colors text-left focus-visible:ring-2 focus-visible:ring-bc-accent shrink-0"
          aria-label={`${activity.collapsed ? "Expand" : "Collapse"} ${activity.name} tool list`}
        >
          <motion.svg
            width="12" height="12" viewBox="0 0 12 12" fill="none"
            stroke="currentColor" strokeWidth="2"
            className="text-bc-muted"
            animate={{ rotate: activity.collapsed ? 0 : 90 }}
            transition={{ duration: 0.15 }}
          >
            <path d="M4 2l4 4-4 4" />
          </motion.svg>
        </button>

        {/* Clickable header area -- opens drill-down */}
        <button
          type="button"
          onClick={onDrillDown}
          className="group flex-1 flex items-center gap-3 py-3 pr-4 hover:bg-bc-surface-hover transition-colors text-left focus-visible:ring-2 focus-visible:ring-bc-accent min-w-0 cursor-pointer"
          title={`Open ${activity.name} detail view`}
        >
          {/* Monogram circle */}
          <span className="inline-flex items-center justify-center h-8 w-8 rounded-full bg-bc-accent/20 text-bc-accent font-bold text-sm shrink-0">
            {monogram}
          </span>

          {/* Name + role + state */}
          <div className="flex flex-col min-w-0">
            <span className="flex items-center gap-2">
              <span className="font-bold text-[15px] text-bc-text leading-tight">
                {activity.name}
              </span>
              <StateDot state={activity.state} />
              {errorCount > 0 && (
                <span className="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 text-[10px] font-bold text-white bg-bc-error rounded-full leading-none">
                  {errorCount}
                </span>
              )}
            </span>
            <span className="flex items-center gap-2 mt-0.5">
              <span className="text-[11px] text-bc-muted font-mono">{activity.role}</span>
              <span className={`text-[10px] font-mono capitalize px-1.5 py-0.5 rounded-full leading-none ${stateBadgeClass(activity.state)}`}>
                {activity.state}
              </span>
            </span>
          </div>

          {searchTerm && matchCount > 0 && (
            <span className="text-[11px] text-bc-accent font-mono shrink-0">
              {matchCount} {matchCount === 1 ? "match" : "matches"}
            </span>
          )}

          {activity.task && (
            <span className="text-[12px] text-bc-muted truncate max-w-[250px] hidden lg:inline">
              {activity.task}
            </span>
          )}

          <span className="ml-auto flex items-center gap-3 shrink-0">
            {runningCount > 0 && (
              <span className="text-[11px] text-bc-accent font-mono tabular-nums">
                {runningCount} running
              </span>
            )}
            {/* Cost -- prominent */}
            {cost > 0 && (
              <span className="text-xs font-semibold text-bc-success font-mono tabular-nums px-1.5 py-0.5 rounded bg-bc-success/10" title={activity.costUsd > 0 ? "From API" : "Estimated from tokens"}>
                ${cost.toFixed(2)}
              </span>
            )}
            {/* Tokens */}
            {activity.tokens > 0 && (
              <span className="text-[11px] text-bc-muted font-mono tabular-nums">
                {activity.tokens.toLocaleString()} tok
              </span>
            )}
            {/* Idle chip */}
            {isIdle && activity.lastEventTime > 0 && (
              <span className="text-[10px] text-bc-muted/60 font-mono px-1.5 py-0.5 rounded bg-bc-muted/10">
                <IdleTimer lastEventTime={activity.lastEventTime} />
              </span>
            )}
            <span className="text-[11px] text-bc-muted/60 group-hover:text-bc-accent transition-colors hidden sm:inline">
              &rarr;
            </span>
          </span>
        </button>
      </div>

      <AnimatePresence initial={false}>
        {!activity.collapsed && showToolNodes && displayNodes.length > 0 && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: "easeOut" }}
            className="border-t border-bc-border/60 py-1 overflow-hidden"
          >
            {visibleNodes.length > 3 && (
              <div className="flex justify-end px-3 py-1">
                <button
                  type="button"
                  onClick={() => setCollapseOld((prev) => !prev)}
                  className="text-[10px] text-bc-muted hover:text-bc-accent font-mono transition-colors"
                >
                  {collapseOld ? "Show all" : "Collapse old"}
                </button>
              </div>
            )}
            <AnimatePresence mode="popLayout" initial={false}>
              {displayNodes.map((node) => {
                const nodeKey = node.id;
                if (skipAnimation) {
                  return (
                    <div key={nodeKey}>
                      <DisplayNodeRow node={node} searchQuery={searchTerm} />
                    </div>
                  );
                }
                return (
                  <motion.div
                    key={nodeKey}
                    initial={{ opacity: 0, y: -20, height: 0 }}
                    animate={{ opacity: 1, y: 0, height: "auto" }}
                    exit={{ opacity: 0, height: 0 }}
                    transition={{ duration: 0.2, ease: "easeOut" }}
                    layout
                  >
                    <DisplayNodeRow node={node} searchQuery={searchTerm} />
                  </motion.div>
                );
              })}
            </AnimatePresence>
          </motion.div>
        )}
      </AnimatePresence>

      {!activity.collapsed && showToolNodes && visibleNodes.length === 0 && !searchTerm && (
        <div className="border-t border-bc-border/60 py-3 px-4 text-[12px] text-bc-muted italic">
          {activity.lastEventTime > 0 ? (
            <IdleTimer lastEventTime={activity.lastEventTime} />
          ) : (
            "Waiting for activity..."
          )}
        </div>
      )}

      {!activity.collapsed && typeFilter === "state" && (
        <div className="border-t border-bc-border/60 py-3 px-4 text-[12px] text-bc-muted">
          <span className="capitalize font-medium text-bc-text">{activity.state}</span>
          {activity.task && <span className="ml-2">--- {activity.task}</span>}
          {activity.tokens > 0 && (
            <span className="ml-2 font-mono tabular-nums">{activity.tokens.toLocaleString()} tokens</span>
          )}
        </div>
      )}
    </motion.div>
  );
});
