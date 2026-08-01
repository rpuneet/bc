import { formatDuration, formatRelative } from "../../utils/time";

import type { AgentActivityItem } from "../../api/client";
import type {
  AgentActivity,
  HookEvent,
  TaskItem,
  ToolNode,
} from "./liveTypes";
import { TASK_STATUS_MAP } from "./liveTypes";

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

export function mcpBadgeColors(server: string): string {
  if (server === "playwright" || server === "playwright2") return "bg-mycel-info-subtle text-mycel-info";
  if (server === "github") return "bg-mycel-surface-hover text-mycel-text-2";
  if (server === "mycel") return "bg-mycel-accent-subtle text-mycel-accent";
  return "bg-mycel-surface-hover text-mycel-text-2";
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
    // Prefer the human-written description over the raw command — it is
    // the richer one-line summary when the hook payload carries it.
    if (typeof obj.description === "string" && obj.description) return redactSecrets(trunc(obj.description, 60));
    if (typeof obj.command === "string") return redactSecrets(trunc(obj.command));
  }
  if ((toolName === "Read" || toolName === "Write" || toolName === "Edit" || toolName === "NotebookEdit") && typeof obj.file_path === "string") {
    return obj.file_path;
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

/** Compact one-line label for mycel-internal / informational events that
 *  don't carry a tool_name — channel chatter, cost updates, notifications,
 *  config/worktree changes, compaction boundaries, etc. Used so every known
 *  server event surfaces a row in the timeline instead of only being visible
 *  in the Raw tab (#2674). */
export function summarizeInternalEvent(evt: HookEvent): string {
  const trunc = (s: string, max = 100): string => (s.length > max ? s.slice(0, max - 3) + "..." : s);

  switch (evt.event) {
    case "ChannelMessage":
    case "ChannelSent": {
      const who = evt.sender ? `@${evt.sender}` : "";
      const chan = evt.channel ? `#${evt.channel}` : "";
      const body = evt.message ? trunc(evt.message) : "";
      return [chan, who, body].filter(Boolean).join(" ");
    }
    case "AgentMessage": {
      const who = evt.sender ? `from @${evt.sender}` : "";
      const body = evt.message ? trunc(evt.message) : "";
      return [who, body].filter(Boolean).join(" ");
    }
    case "CostUpdate":
      return typeof evt.cost_usd === "number" ? `+$${evt.cost_usd.toFixed(4)}` : "";
    case "Notification":
      return evt.message ? trunc(evt.message) : "";
    case "ConfigChange":
      return evt.file ?? evt.message ?? "";
    case "WorktreeCreate":
    case "WorktreeRemove":
      return evt.file ?? evt.message ?? "";
    case "PreCompact":
      return "compacting context…";
    case "PostCompact":
      return "context compacted";
    default:
      return evt.message ?? evt.task ?? evt.command ?? "";
  }
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

/* ── Path helpers ──────────────────────────────────────────────────── */

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

export function durationPillClass(start: number, end?: number): string {
  const ms = (end ?? Date.now()) - start;
  if (ms < 500) return "bg-mycel-success-subtle text-mycel-success";
  if (ms < 2000) return "bg-mycel-warning-subtle text-mycel-warning";
  if (ms < 10000) return "bg-mycel-accent-subtle text-mycel-accent";
  return "bg-mycel-error-subtle text-mycel-error";
}

export function stateBadgeClass(state: string): string {
  if (state === "working") return "bg-mycel-success-subtle text-mycel-success";
  if (state === "stuck") return "bg-mycel-warning-subtle text-mycel-warning";
  if (state === "error" || state === "stopped") return "bg-mycel-error-subtle text-mycel-error";
  return "bg-mycel-surface-hover text-mycel-text-2";
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

/* ── Node search / flatten ──────────────────────────────────────── */

export function nodeMatchesSearch(node: ToolNode, query: string): boolean {
  const hay = `${node.toolName} ${node.args}`.toLowerCase();
  return hay.includes(query);
}

/** Flatten a node tree (subagent children nest under their parent) into
 *  a single flat list so every hook event renders as one stream row. */
export function flattenNodes(nodes: ToolNode[]): ToolNode[] {
  const out: ToolNode[] = [];
  const walk = (list: ToolNode[]) => {
    for (const n of list) {
      out.push(n);
      if (n.children.length > 0) walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

/** Split a flat node list into currently-running rows and everything
 *  else, preserving input order within each bucket. Running rows get
 *  pinned above the chronological stream so long feeds never bury an
 *  in-flight tool call or subagent. */
export function partitionRunning(nodes: ToolNode[]): { running: ToolNode[]; rest: ToolNode[] } {
  const running: ToolNode[] = [];
  const rest: ToolNode[] = [];
  for (const n of nodes) {
    if (n.status === "running") running.push(n);
    else rest.push(n);
  }
  return { running, rest };
}

/* ── Historical activity → ToolNode ─────────────────────────────────── */

/**
 * Turn a persisted activity row into a ToolNode for the Live feed.
 *
 * Historical rows come from the append-only event log, so they carry
 * whatever the hook recorded — `tool_name`, `tool_input`, `tool_response`,
 * `error` — but no Pre/Post pairing, so duration is unknown. We surface
 * every field that IS stored and gracefully omit the rest. This is the same
 * ToolNode shape the live stream builds, so both render through one
 * EventRow.
 */
export function activityItemToNode(item: AgentActivityItem): ToolNode {
  const data = (item.data ?? {}) as Record<string, unknown>;
  const dataToolName = typeof data.tool_name === "string" ? data.tool_name : "";
  const toolName = dataToolName || parseHistoricalToolName(item) || item.event || "unknown";

  const toolInput = data.tool_input ?? null;
  const toolResponse = data.tool_response ?? null;
  const error = typeof data.error === "string" && data.error ? data.error : undefined;

  const args = toolInput
    ? extractToolMetadata(toolName, toolInput)
    : item.message || "";

  return {
    id: nextId(),
    toolName,
    args,
    fullInput: toolInput,
    fullOutput: toolResponse,
    startTime: item.timestamp ? new Date(item.timestamp).getTime() : Date.now(),
    endTime: undefined,
    status: error ? "failed" : "completed",
    error,
    children: [],
  };
}

/** Derive a tool name from a historical row's message ("Bash: cmd" or a
 *  bare word) when the structured tool_name field is absent. */
function parseHistoricalToolName(item: AgentActivityItem): string {
  if (!item.message) return "";
  const colonIdx = item.message.indexOf(":");
  if (colonIdx > 0) return item.message.slice(0, colonIdx).trim();
  const spaceIdx = item.message.indexOf(" ");
  if (spaceIdx > 0) return item.message.slice(0, spaceIdx).trim();
  return item.message;
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
