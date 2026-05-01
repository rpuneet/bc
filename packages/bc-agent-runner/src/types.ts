// HTTP API contract for bc-agent-runner.
//
// One running process == one Claude agent. bcd POSTs prompts in, listens to
// /events for typed activity, and reads /messages for the conversation log.

export type AgentState =
  | "idle" // no session yet, ready for first /query
  | "working" // a query is in flight
  | "waiting" // streaming-input mode, awaiting next user message
  | "stopped" // /stop was called or process is shutting down
  | "error"; // last query failed; see lastError

export interface QueryRequest {
  prompt: string;
  // Optional system-prompt override; falls back to BC_ROLE_PROMPT env var.
  system_prompt?: string;
  // Hard cap on conversation turns for this query.
  max_turns?: number;
  // Hard cap on USD spend for this query.
  max_budget_usd?: number;
  // Resume a prior session by id (returned from a previous query).
  resume_session?: string;
  // Override allowed-tools list for this query. Falls back to BC_ALLOWED_TOOLS.
  allowed_tools?: string[];
  // Override permission mode for this query.
  permission_mode?: "default" | "acceptEdits" | "bypassPermissions" | "plan";
}

export interface QueryResponse {
  session_id: string | null;
  state: AgentState;
  started_at: string;
}

export interface StatusResponse {
  agent_name: string;
  state: AgentState;
  session_id: string | null;
  current_turn: number;
  tokens_used: number;
  cost_usd: number;
  started_at: string | null;
  last_activity_at: string | null;
  last_error: string | null;
  working_dir: string;
}

export interface StopResponse {
  state: AgentState;
  session_id: string | null;
}

export interface HealthResponse {
  ok: true;
  agent_name: string;
  uptime_seconds: number;
  sdk_version: string;
}

// Persisted message log entry. Stored in memory as the conversation grows so
// /messages can replay history without re-querying the SDK.
export interface MessageLogEntry {
  ts: string;
  type: string; // "system" | "assistant" | "user" | "result" | "stream_event"
  session_id: string | null;
  // Raw SDK message, JSON-serializable.
  raw: unknown;
}

// SSE event envelope written to /events listeners.
export interface RunnerEvent {
  ts: string;
  type:
    | "session_start"
    | "assistant_message"
    | "tool_use"
    | "tool_result"
    | "result"
    | "error"
    | "stop";
  session_id: string | null;
  data: unknown;
}
