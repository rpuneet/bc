/* ── Types ─────────────────────────────────────────────────────────── */

export interface ToolNode {
  id: string;
  toolName: string;
  args: string;
  fullInput: unknown;
  fullOutput: unknown;
  status: "running" | "completed" | "failed";
  error?: string;
  startTime: number;
  endTime?: number;
  children: ToolNode[];
}

export interface AgentActivity {
  name: string;
  state: string;
  task: string;
  tool: string;
  role: string;
  tokens: number;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  lastEventTime: number;
  nodes: ToolNode[];
  collapsed: boolean;
  /** Index of the currently-active subagent node in nodes[], for nesting */
  activeSubagentIdx?: number;
  /** Template-declared secrets absent at create (#3558 create-degraded). */
  missingSecrets?: string[];
}

export interface HookEvent {
  agent: string;
  event: string;
  tool_name?: string;
  command?: string;
  error?: string;
  task?: string;
  subagent_id?: string;
  subagent_type?: string;
  tool_input?: unknown;
  tool_response?: unknown;
  input_tokens?: number;
  output_tokens?: number;
  prompt?: string;
  /** mycel-internal event fields (ChannelMessage/Sent, AgentMessage,
   *  CostUpdate, Notification, ConfigChange, Worktree*, …). */
  channel?: string;
  sender?: string;
  message?: string;
  mentions?: string[];
  cost_usd?: number;
  file?: string;
  model?: string;
  state?: string;
  task_title?: string;
}

export interface TaskItem {
  id: string;
  subject: string;
  status: "pending" | "in_progress" | "completed" | "deleted";
  owner?: string;
  description?: string;
  blockedBy?: string[];
}

export type FilterType = "all" | "tools" | "state";

export type DrillDownTab = "live" | "raw";

export interface RawEvent {
  timestamp: number;
  eventType: string;
  raw: unknown;
}

/* ── Constants ─────────────────────────────────────────────────────── */

export const MAX_NODES = 50;
export const AUTO_COLLAPSE_MS = 30_000;
export const FLUSH_INTERVAL = 150;

export const TASK_STATUS_MAP: Record<string, TaskItem["status"]> = {
  pending: "pending",
  in_progress: "in_progress",
  "in-progress": "in_progress",
  inProgress: "in_progress",
  completed: "completed",
  done: "completed",
  deleted: "deleted",
  cancelled: "deleted",
  canceled: "deleted",
};
