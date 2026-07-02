import { formatDuration, formatRelative } from "../../utils/time";

import type {
  AgentActivity,
  AggregatedNode,
  DisplayNode,
  HookEvent,
  TaskItem,
  ToolNode,
} from "./liveTypes";
import {
  AGGREGATION_WINDOW_MS,
  FAILED_NEVER_AGGREGATE_MS,
  isAggregatedNode,
  MAX_INDIVIDUAL_NODES,
  TASK_STATUS_MAP,
} from "./liveTypes";

/* ── ID generator ──────────────────────────────────────────────────── */

export let _nodeId = 0;
export function nextId(): string {
  return `n-${++_nodeId}-${Date.now()}`;
}

/* ── Tool name parsing ─────────────────────────────────────────────── */

export interface ParsedTool {
  display: string;
  type: "mcp" | "bash" | "internal";
  mcpServer?: string;
  mcpFunction?: string;
}

export function parseToolName(name: string): ParsedTool {
  if (!name) return { display: "unknown", type: "internal" };
  if (name === "Bash" || name === "bash") return { display: "Bash", type: "bash" };
  if (name.startsWith("mcp__")) {
    const parts = name.split("__");
    let server = parts[1] ?? "mcp";
    const func = parts[parts.length - 1] ?? "call";
    if (server.startsWith("plugin_")) {
      const pluginParts = server.replace("plugin_", "").split("_");
      server = pluginParts[0] ?? server;
    }
    return { display: `${server}:${func}`, type: "mcp", mcpServer: server, mcpFunction: func };
  }
  if (name.includes("__")) {
    const parts = name.split("__");
    const action = parts[parts.length - 1] ?? name;
    return { display: action, type: "mcp", mcpServer: parts[0], mcpFunction: action };
  }
  return { display: name, type: "internal" };
}

export function toolIcon(name: string): string {
  if (name === "Bash" || name === "BashOutput") return "\u2328\uFE0F";
  if (name === "Read") return "\uD83D\uDCD6";
  if (name === "Write" || name === "Edit") return "\u270F\uFE0F";
  if (name === "Glob" || name === "Grep") return "\uD83D\uDD0D";
  if (name === "Agent") return "\uD83E\uDD16";
  if (name === "WebFetch" || name === "WebSearch") return "\uD83C\uDF10";
  if (name.startsWith("Task")) return "\u2705";
  if (name === "NotebookEdit") return "\uD83D\uDCD3";
  if (name === "LSP" || name === "ToolSearch") return "\u2699\uFE0F";
  if (name === "AskUserQuestion") return "\u2753";
  if (name === "Skill") return "\uD83C\uDFAF";
  return "\u2699\uFE0F";
}

export function mcpServerIcon(server: string): string {
  if (server === "playwright" || server === "playwright2") return "\uD83C\uDFAD";
  if (server === "github") return "\uD83D\uDC19";
  if (server === "bc") return "\u26A1";
  return "\uD83D\uDD0C";
}

export function mcpBadgeColors(server: string): string {
  if (server === "playwright" || server === "playwright2") return "bg-purple-900/50 text-purple-300";
  if (server === "github") return "bg-gray-700 text-gray-300";
  if (server === "bc") return "bg-blue-900/50 text-blue-300";
  return "bg-zinc-700 text-zinc-300";
}

/* ── Secret redaction ──────────────────────────────────────────────── */

const SECRET_PATTERNS = [
  /github_pat_[A-Za-z0-9_]{20,}/g,
  /ghp_[A-Za-z0-9]{36,}/g,
  /sk-[A-Za-z0-9]{20,}/g,
  /Bearer\s+[A-Za-z0-9._\-/+=]{20,}/g,
  /(?:password|secret|token|key|auth|credential|api_key)["'=:\s]+["']?[A-Za-z0-9._\-/+=]{8,}["']?/gi,
];

export function redactSecrets(text: string): string {
  let result = text;
  for (const pattern of SECRET_PATTERNS) {
    result = result.replace(pattern, "***");
  }
  return result;
}

export function redactValue(value: unknown): unknown {
  if (typeof value === "string") return redactSecrets(value);
  if (Array.isArray(value)) return value.map(redactValue);
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) {
      out[k] = redactValue(v);
    }
    return out;
  }
  return value;
}

/* ── Tool metadata extraction ──────────────────────────────────────── */

export function extractToolMetadata(toolName: string, input: unknown): string {
  if (!input || typeof input !== "object") return "";
  const obj = input as Record<string, unknown>;
  const trunc = (s: string, max = 80): string => s.length > max ? s.slice(0, max - 3) + "..." : s;

  if (toolName === "Bash" || toolName === "bash") {
    if (typeof obj.command === "string") return redactSecrets(trunc(obj.command));
  }
  if ((toolName === "Read" || toolName === "Write") && typeof obj.file_path === "string") {
    return trunc(obj.file_path);
  }
  if (toolName === "Edit") {
    let s = typeof obj.file_path === "string" ? obj.file_path : "";
    if (typeof obj.old_string === "string") {
      s += " " + trunc(obj.old_string, 40);
    }
    return trunc(s);
  }
  if ((toolName === "Grep" || toolName === "Glob") && typeof obj.pattern === "string") {
    return trunc(obj.pattern);
  }
  if (toolName === "Agent") {
    const parts: string[] = [];
    if (typeof obj.subagent_type === "string") parts.push(obj.subagent_type);
    if (typeof obj.description === "string") parts.push(trunc(obj.description, 60));
    return parts.join(" ");
  }
  if (toolName === "WebFetch") {
    if (typeof obj.url === "string") {
      try { return new URL(obj.url).hostname; } catch { return trunc(obj.url); }
    }
  }
  if (toolName === "WebSearch") {
    if (typeof obj.query === "string") return trunc(obj.query);
  }
  if (toolName.startsWith("mcp__")) {
    const vals = Object.entries(obj).slice(0, 3).map(([, v]) => {
      if (typeof v === "string") return trunc(v, 30);
      if (typeof v === "number" || typeof v === "boolean") return String(v);
      return "";
    }).filter(Boolean);
    return redactSecrets(vals.join(" "));
  }
  const s = JSON.stringify(obj);
  return redactSecrets(trunc(s));
}

export function summarizeArgs(evt: HookEvent): string {
  if (evt.tool_name && evt.tool_input) {
    return extractToolMetadata(evt.tool_name, evt.tool_input);
  }
  if (evt.command) {
    const s = evt.command.length > 80 ? evt.command.slice(0, 77) + "..." : evt.command;
    return redactSecrets(s);
  }
  if (evt.tool_input && typeof evt.tool_input === "object") {
    const s = JSON.stringify(evt.tool_input);
    return redactSecrets(s.length > 80 ? s.slice(0, 77) + "..." : s);
  }
  return "";
}

/* ── Array / time utilities ────────────────────────────────────────── */

export function findLastIdx<T>(arr: T[], pred: (v: T) => boolean): number {
  for (let i = arr.length - 1; i >= 0; i--) {
    if (pred(arr[i] as T)) return i;
  }
  return -1;
}

export function elapsed(start: number, end?: number): string {
  const ms = (end ?? Date.now()) - start;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60_000).toFixed(1)}m`;
}

export function durationColorClass(start: number, end?: number): string {
  const ms = (end ?? Date.now()) - start;
  if (ms < 500) return "text-mycel-success";
  if (ms < 2000) return "text-mycel-warning";
  if (ms < 10000) return "text-mycel-accent";
  return "text-mycel-error";
}

export function durationPillClass(start: number, end?: number): string {
  const ms = (end ?? Date.now()) - start;
  if (ms < 500) return "bg-mycel-success/15 text-mycel-success";
  if (ms < 2000) return "bg-mycel-warning/15 text-mycel-warning";
  if (ms < 10000) return "bg-mycel-accent/15 text-mycel-accent";
  return "bg-mycel-error/15 text-mycel-error";
}

export function stateBadgeClass(state: string): string {
  if (state === "working") return "bg-mycel-success/15 text-mycel-success";
  if (state === "stuck") return "bg-mycel-warning/15 text-mycel-warning";
  if (state === "error" || state === "stopped") return "bg-mycel-error/15 text-mycel-error";
  return "bg-mycel-muted/15 text-mycel-muted";
}

export function relativeTime(ts: number): string {
  // Unbounded ladder (no max-day fallback) — matches the pre-consolidation
  // liveHelpers behavior used by the always-relative live view timeline.
  return formatRelative(ts, { maxDays: Number.POSITIVE_INFINITY });
}

const INPUT_COST_PER_TOKEN = 3 / 1_000_000;
const OUTPUT_COST_PER_TOKEN = 15 / 1_000_000;

export function estimateCost(activity: AgentActivity): number {
  if (activity.costUsd > 0) return activity.costUsd;
  if (activity.inputTokens > 0 || activity.outputTokens > 0) {
    return activity.inputTokens * INPUT_COST_PER_TOKEN + activity.outputTokens * OUTPUT_COST_PER_TOKEN;
  }
  return 0;
}

export function idleDuration(lastEventTime: number): string {
  return formatDuration(Date.now() - lastEventTime, { prefix: "Idle " });
}

/* ── Node search / sort ────────────────────────────────────────────── */

export function nodeMatchesSearch(node: ToolNode, query: string): boolean {
  const hay = `${node.toolName} ${node.args}`.toLowerCase();
  return hay.includes(query);
}

export function sortNodes(nodes: ToolNode[]): ToolNode[] {
  return [...nodes].sort((a, b) => {
    // Running first (newest first among running)
    if (a.status === "running" && b.status !== "running") return -1;
    if (b.status === "running" && a.status !== "running") return 1;
    if (a.status === "running" && b.status === "running") return b.startTime - a.startTime;
    // Failed second (newest first among failed)
    if (a.status === "failed" && b.status !== "failed") return -1;
    if (b.status === "failed" && a.status !== "failed") return 1;
    if (a.status === "failed" && b.status === "failed") return b.startTime - a.startTime;
    // Completed: sort by duration (longest first)
    const aDur = (a.endTime ?? a.startTime) - a.startTime;
    const bDur = (b.endTime ?? b.startTime) - b.startTime;
    return bDur - aDur;
  });
}

/* ── Aggregation ──────────────────────────────────────────────────── */

const AGGREGATION_MIN_COUNT = 3;

export const NEVER_AGGREGATE_EVENTS = new Set([
  "SubagentStart", "SubagentStop", "Agent",
  "PermissionRequest", "Elicitation",
  "UserPromptSubmit", "SessionStart", "SessionEnd",
  "Stop", "TaskCompleted",
]);

export function shouldNeverAggregate(node: ToolNode, now?: number): boolean {
  // Failed tools: never aggregate for 2 minutes after creation
  if (node.status === "failed") {
    const ts = now ?? Date.now();
    if (ts - node.startTime < FAILED_NEVER_AGGREGATE_MS) return true;
  }
  if (NEVER_AGGREGATE_EVENTS.has(node.toolName)) return true;
  if (node.toolName.startsWith("Agent:")) return true;
  return false;
}

export interface AggStats {
  totalDuration: number;
  totalTokens: number;
  successCount: number;
  failCount: number;
  minStart: number;
  maxEnd: number;
}

export function computeToolNodeStats(nodes: ToolNode[]): AggStats {
  let totalDuration = 0;
  let successCount = 0;
  let failCount = 0;
  let minStart = Infinity;
  let maxEnd = 0;

  for (const n of nodes) {
    totalDuration += n.endTime ? n.endTime - n.startTime : 0;
    if (n.status === "completed") successCount++;
    if (n.status === "failed") failCount++;
    if (n.startTime < minStart) minStart = n.startTime;
    if (n.endTime && n.endTime > maxEnd) maxEnd = n.endTime;
  }

  return { totalDuration, totalTokens: 0, successCount, failCount, minStart, maxEnd };
}

export function buildAggregatedNode(idPrefix: string, toolName: string, children: ToolNode[], stats: AggStats): AggregatedNode {
  return {
    type: "aggregate",
    id: `${idPrefix}-${children[0]!.id}`,
    toolName,
    count: children.length,
    children,
    totalDuration: stats.totalDuration,
    totalTokens: stats.totalTokens,
    successCount: stats.successCount,
    failCount: stats.failCount,
    startTime: stats.minStart,
    endTime: stats.maxEnd || Date.now(),
  };
}

export function flattenDisplayNodes(group: DisplayNode[]): { children: ToolNode[]; stats: AggStats } {
  const allChildren: ToolNode[] = [];
  let totalDuration = 0;
  let totalTokens = 0;
  let successCount = 0;
  let failCount = 0;
  let minStart = Infinity;
  let maxEnd = 0;

  for (const g of group) {
    if (isAggregatedNode(g)) {
      allChildren.push(...g.children);
      totalDuration += g.totalDuration;
      totalTokens += g.totalTokens;
      successCount += g.successCount;
      failCount += g.failCount;
      if (g.startTime < minStart) minStart = g.startTime;
      if (g.endTime > maxEnd) maxEnd = g.endTime;
    } else {
      allChildren.push(g);
      totalDuration += g.endTime ? g.endTime - g.startTime : 0;
      if (g.status === "completed") successCount++;
      if (g.status === "failed") failCount++;
      if (g.startTime < minStart) minStart = g.startTime;
      if (g.endTime && g.endTime > maxEnd) maxEnd = g.endTime;
    }
  }

  return { children: allChildren, stats: { totalDuration, totalTokens, successCount, failCount, minStart, maxEnd } };
}

export function aggregateNodes(nodes: ToolNode[], collapseOlderThan?: number, totalNodeCount?: number): DisplayNode[] {
  if (nodes.length === 0) return [];

  const now = Date.now();
  const threshold = collapseOlderThan ?? 0;
  const agentTotalNodes = totalNodeCount ?? nodes.length;

  // If agent has fewer than MAX_INDIVIDUAL_NODES total calls, show all individually
  if (agentTotalNodes < MAX_INDIVIDUAL_NODES) {
    return nodes;
  }

  if (threshold > 0) {
    const recentNodes: ToolNode[] = [];
    const oldByTool = new Map<string, ToolNode[]>();

    for (const n of nodes) {
      const age = now - n.startTime;
      if (age <= threshold || n.status === "running" || shouldNeverAggregate(n, now)) {
        recentNodes.push(n);
      } else {
        const key = n.toolName;
        if (!oldByTool.has(key)) oldByTool.set(key, []);
        oldByTool.get(key)!.push(n);
      }
    }

    const oldAggregated: DisplayNode[] = [];
    for (const [toolName, group] of oldByTool) {
      if (group.length >= 2) {
        const stats = computeToolNodeStats(group);
        oldAggregated.push(buildAggregatedNode("agg-old", toolName, group, stats));
      } else {
        oldAggregated.push(...group);
      }
    }

    oldAggregated.sort((a, b) => b.startTime - a.startTime);

    const recentAggregated = aggregateConsecutive(recentNodes);
    return aggregateByType([...recentAggregated, ...oldAggregated], agentTotalNodes);
  }

  return aggregateByType(aggregateConsecutive(nodes), agentTotalNodes);
}

/** Post-completion aggregation: collapse completed tool calls of the same type
 *  across non-consecutive positions when count >= AGGREGATION_MIN_COUNT.
 *  Running events are always shown individually.
 *  Failed events are pinned (shown individually) for FAILED_NEVER_AGGREGATE_MS,
 *  then become eligible for type-based aggregation. */
export function aggregateByType(displayNodes: DisplayNode[], totalNodeCount?: number): DisplayNode[] {
  // If agent has fewer than MAX_INDIVIDUAL_NODES total calls, skip aggregation
  if (totalNodeCount !== undefined && totalNodeCount < MAX_INDIVIDUAL_NODES) {
    return displayNodes;
  }

  const now = Date.now();
  const pinned: DisplayNode[] = [];
  const candidates: DisplayNode[] = [];

  for (const node of displayNodes) {
    if (isAggregatedNode(node)) {
      candidates.push(node);
    } else {
      const tn = node as ToolNode;
      if (tn.status === "running") {
        pinned.push(node);
      } else if (tn.status === "failed" && (now - tn.startTime) < FAILED_NEVER_AGGREGATE_MS) {
        // Failed events within 2 minutes are always pinned individually
        pinned.push(node);
      } else {
        candidates.push(node);
      }
    }
  }

  const byTool = new Map<string, DisplayNode[]>();
  const ungroupable: DisplayNode[] = [];

  for (const node of candidates) {
    if (isAggregatedNode(node)) {
      const key = node.toolName;
      if (!byTool.has(key)) byTool.set(key, []);
      byTool.get(key)!.push(node);
    } else {
      const tn = node as ToolNode;
      if (shouldNeverAggregate(tn, now)) {
        ungroupable.push(node);
      } else {
        const key = tn.toolName;
        if (!byTool.has(key)) byTool.set(key, []);
        byTool.get(key)!.push(node);
      }
    }
  }

  const aggregated: DisplayNode[] = [];
  for (const [toolName, group] of byTool) {
    let totalIndividual = 0;
    for (const g of group) {
      totalIndividual += isAggregatedNode(g) ? g.count : 1;
    }

    if (totalIndividual >= AGGREGATION_MIN_COUNT) {
      const { children: allChildren, stats } = flattenDisplayNodes(group);
      aggregated.push(buildAggregatedNode("agg-type", toolName, allChildren, stats));
    } else {
      ungroupable.push(...group);
    }
  }

  // Pinned (running/failed) first, then ungroupable individuals, then aggregated summaries at bottom
  return [...pinned, ...ungroupable, ...aggregated];
}

export function aggregateConsecutive(nodes: ToolNode[]): DisplayNode[] {
  if (nodes.length === 0) return [];

  const now = Date.now();
  const result: DisplayNode[] = [];
  let i = 0;

  while (i < nodes.length) {
    const current = nodes[i];
    if (!current) { i++; continue; }

    if (shouldNeverAggregate(current, now) || current.status === "running") {
      result.push(current);
      i++;
      continue;
    }

    const group: ToolNode[] = [current];
    let j = i + 1;
    while (j < nodes.length) {
      const next = nodes[j];
      if (!next) break;
      if (next.toolName !== current.toolName) break;
      if (shouldNeverAggregate(next, now) || next.status === "running") break;
      const prev = group[group.length - 1];
      if (!prev) break;
      if (Math.abs(next.startTime - prev.startTime) > AGGREGATION_WINDOW_MS) break;
      group.push(next);
      j++;
    }

    if (group.length >= 2) {
      const stats = computeToolNodeStats(group);
      result.push(buildAggregatedNode("agg", current.toolName, group, stats));
      i = j;
    } else {
      result.push(current);
      i++;
    }
  }

  return result;
}

/* ── Task parsing helpers ──────────────────────────────────────────── */

export function parseTaskCreate(
  toolInput: unknown,
  toolResponse: unknown,
  agentName: string,
): TaskItem | null {
  const inp = toolInput as Record<string, unknown> | null;
  const resp = toolResponse as Record<string, unknown> | null;
  if (!inp) return null;

  let id = "task-" + Date.now();

  // First, try to parse numeric ID from string response like "Task #125 created successfully: ..."
  if (typeof toolResponse === "string") {
    const numMatch = (toolResponse as string).match(/Task\s+#(\d+)/);
    if (numMatch) id = numMatch[1]!;
  }

  // If still a fallback ID, try structured response fields
  if (id.startsWith("task-") && resp) {
    if (typeof resp.id === "string") id = resp.id;
    else if (typeof resp.task_id === "string") id = resp.task_id;
    else if (typeof resp === "string") {
      try {
        const parsed = JSON.parse(resp as unknown as string) as Record<string, unknown>;
        if (typeof parsed.id === "string") id = parsed.id;
      } catch { /* ignore */ }
    }
  }

  const subject = typeof inp.subject === "string"
    ? inp.subject
    : typeof inp.description === "string"
      ? inp.description
      : typeof inp.title === "string"
        ? (inp.title as string)
        : "Untitled task";

  const description = typeof inp.description === "string" ? inp.description : undefined;

  return { id, subject, status: "pending", owner: agentName, description };
}

export function parseTaskUpdate(toolInput: unknown): { taskId: string; status: TaskItem["status"]; blockedBy?: string[] } | null {
  const inp = toolInput as Record<string, unknown> | null;
  if (!inp) return null;

  const taskId = typeof inp.taskId === "string"
    ? inp.taskId
    : typeof inp.task_id === "string"
      ? inp.task_id
      : typeof inp.id === "string"
        ? inp.id
        : null;

  if (!taskId) return null;

  const rawStatus = typeof inp.status === "string" ? inp.status : null;
  if (!rawStatus) return null;

  const status = TASK_STATUS_MAP[rawStatus] ?? "pending";
  const blockedBy = Array.isArray(inp.addBlockedBy) ? inp.addBlockedBy as string[] : undefined;
  return { taskId, status, blockedBy };
}

export function parseTaskListResponse(text: string): TaskItem[] {
  const tasks: TaskItem[] = [];
  const lines = text.split("\n");
  for (const line of lines) {
    const match = line.match(/^#(\d+)\s+\[(\w+)]\s+(.+)$/);
    if (match) {
      const id = match[1]!;
      const rawStatus = match[2]!.toLowerCase();
      const subject = match[3]!.trim();
      const status = TASK_STATUS_MAP[rawStatus] ?? "pending";
      tasks.push({ id, subject, status });
    }
  }
  return tasks;
}

/* ── Event processing helpers ──────────────────────────────────────── */

/** Try to update a running child node inside the active subagent. Returns true if found. */
export function updateSubagentChild(
  activity: AgentActivity,
  predicate: (n: ToolNode) => boolean,
  updater: (child: ToolNode) => ToolNode,
): boolean {
  if (activity.activeSubagentIdx === undefined || activity.activeSubagentIdx < 0) return false;
  const parentNode = activity.nodes[activity.activeSubagentIdx];
  if (!parentNode) return false;

  const childIdx = findLastIdx(parentNode.children, predicate);
  if (childIdx < 0) return false;

  const updatedChildren = [...parentNode.children];
  updatedChildren[childIdx] = updater(updatedChildren[childIdx]!);
  activity.nodes[activity.activeSubagentIdx] = { ...parentNode, children: updatedChildren };
  return true;
}

/** Find and update a running top-level node. Returns true if found. */
export function updateTopLevelNode(
  activity: AgentActivity,
  predicate: (n: ToolNode) => boolean,
  updater: (node: ToolNode) => ToolNode,
): boolean {
  const idx = findLastIdx(activity.nodes, predicate);
  if (idx < 0) return false;
  activity.nodes[idx] = updater(activity.nodes[idx]!);
  return true;
}
