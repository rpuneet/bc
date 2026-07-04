/**
 * TypeScript types for bc CLI JSON responses
 * Auto-generated from bc CLI output structures
 */

import type { ThemeName, ThemeMode } from '../theme';

// Agent states matching pkg/agent/agent.go
export type AgentState = 'idle' | 'starting' | 'working' | 'done' | 'stuck' | 'error' | 'stopped';

// Agent roles matching pkg/agent/agent.go
export type AgentRole = 'root' | 'product-manager' | 'manager' | 'tech-lead' | 'engineer';

// Agent memory info
export interface AgentMemory {
  loaded_at: string;
  role_prompt: string;
}

// Agent from bc status --json
export interface Agent {
  id: string;
  name: string;
  role: AgentRole;
  state: AgentState;
  task: string;
  session: string;
  tool?: string;
  /** Absolute path of the git repo the agent is bound to. */
  repo?: string;
  worktree_dir: string;
  memory_dir: string;
  log_file?: string;
  started_at: string;
  updated_at: string;
  memory?: AgentMemory;
}

// Response from bc status --json
export interface StatusResponse {
  workspace: string;
  total: number;
  active: number;
  working: number;
  agents: Agent[];
}

// Channel types
export interface Channel {
  name: string;
  members: string[];
  created_at?: string;
  description?: string;
}

export interface ChannelMessage {
  sender: string;
  message: string;
  time: string;
}

export interface ChannelHistory {
  channel: string;
  messages: ChannelMessage[];
}

// Response from bc channel list --json
export interface ChannelsResponse {
  channels: Channel[];
}

// Cost types
export interface CostRecord {
  agent_id: string;
  team_id: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  timestamp: string;
}

export interface AgentCost {
  agent: string;
  input_tokens: number;
  output_tokens: number;
  total_cost: number;
}

export interface CostSummary {
  total_cost: number;
  total_input_tokens: number;
  total_output_tokens: number;
  agent_costs?: AgentCost[];
  period?: string;
  by_agent?: Record<string, number>;
  by_team?: Record<string, number>;
  by_model?: Record<string, number>;
  // ccusage integration fields (#1882)
  cache_hit_rate?: number;
  burn_rate?: number;
  projected_total?: number;
  billing_window_spent?: number;
}

// Cost usage types from ccusage integration (#1882)
export interface CostUsageDaily {
  date: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalTokens: number;
  totalCost: number;
  modelsUsed: string[];
}

export interface CostUsageDailyResponse {
  daily: CostUsageDaily[];
  totals: {
    inputTokens: number;
    outputTokens: number;
    cacheCreationTokens: number;
    cacheReadTokens: number;
    totalTokens: number;
    totalCost: number;
  };
}

export interface CostUsageMonthly {
  month: string;
  models: string[];
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalTokens: number;
  costUSD: number;
}

export interface CostUsageMonthlyResponse {
  type: string;
  data: CostUsageMonthly[];
  summary: {
    totalInputTokens: number;
    totalOutputTokens: number;
    totalCacheCreationTokens: number;
    totalCacheReadTokens: number;
    totalTokens: number;
    totalCostUSD: number;
  };
}

export interface CostUsageSession {
  session: string;
  lastActivity: string;
  models: string[];
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalTokens: number;
  costUSD: number;
}

export interface CostUsageSessionResponse {
  type: string;
  data: CostUsageSession[];
  summary: {
    totalInputTokens: number;
    totalOutputTokens: number;
    totalCacheCreationTokens: number;
    totalCacheReadTokens: number;
    totalTokens: number;
    totalCostUSD: number;
  };
}

// Generic bc command result
export interface BcResult<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
}

// Event types for real-time updates
export interface BcEvent {
  type: string;
  timestamp: string;
  agent: string;
  message: string;
  data?: Record<string, unknown>;
}

// Log entry from bc logs --json
// Note: type/agent/message can be undefined at runtime due to Go's omitempty (#1874)
export interface LogEntry {
  ts: string;
  type?: string;
  agent?: string;
  message?: string;
  data?: Record<string, unknown>;
}

// Response from bc logs --json
export type LogsResponse = LogEntry[];

// Worktree from bc worktree list --json
export interface Worktree {
  agent: string;
  path: string;
  status: 'OK' | 'ORPHANED' | 'MISSING';
  branch?: string;
}

// Response from bc worktree list --json
export type WorktreeListResponse = Worktree[];

// Demon (scheduled task) types
export interface Demon {
  name: string;
  schedule: string;
  command: string;
  description?: string;
  owner?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  last_run?: string;
  next_run?: string;
  run_count: number;
}

export interface DemonRunLog {
  timestamp: string;
  duration_ms: number;
  exit_code: number;
  success: boolean;
}

// Process types matching pkg/process/process.go
export interface Process {
  name: string;
  command: string;
  owner?: string;
  work_dir?: string;
  log_file?: string;
  pid: number;
  port?: number;
  running: boolean;
  started_at: string;
}

// Response from bc process list --json
export interface ProcessListResponse {
  processes: Process[];
}

// Response from bc process logs --json
export interface ProcessLogsResponse {
  name: string;
  lines: string[];
}


// Role types
export interface Role {
  name: string;
  description?: string;
  capabilities: string[];
  parent?: string;
  prompt?: string;
  agent_count?: number;
}

// Response from bc role list --json
export interface RolesResponse {
  roles: Role[];
}

// Performance configuration for polling intervals and cache TTLs
// Matches workspace.PerformanceConfig in Go
export interface PerformanceConfig {
  poll_interval_agents: number;
  poll_interval_channels: number;
  poll_interval_costs: number;
  poll_interval_status: number;
  poll_interval_logs: number;
  poll_interval_demons: number;
  poll_interval_dashboard: number;
  cache_ttl_tmux: number;
  cache_ttl_commands: number;
  adaptive_fast_interval: number;
  adaptive_normal_interval: number;
  adaptive_slow_interval: number;
  adaptive_max_interval: number;
}

// TUI theme configuration for appearance and theming
// Matches workspace.TUIConfig in Go
export interface TUIConfig {
  theme: ThemeName;
  mode: ThemeMode;
}

// Routing types for task routing
export interface RoutingRule {
  task_type: string;
  target_role: string;
  description: string;
}

export interface RoutingConfig {
  rules: RoutingRule[];
}

// Tool types for Tools view (#1866)
export type ToolStatus = 'installed' | 'not found';

export interface ToolInfo {
  name: string;
  status: ToolStatus;
  version: string;
  command: string;
  path?: string;
}

// GitHub Issue types for Issues view (#1754)
export type IssueState = 'OPEN' | 'CLOSED';

export interface IssueLabel {
  name: string;
  color?: string;
  description?: string;
}

export interface IssueAssignee {
  login: string;
}

export interface IssueComment {
  author: { login: string };
  body: string;
  createdAt: string;
}

export interface Issue {
  number: number;
  title: string;
  body?: string;
  state: IssueState;
  labels: IssueLabel[];
  assignees: IssueAssignee[];
  author?: { login: string };
  createdAt: string;
  updatedAt?: string;
  comments?: IssueComment[];
}

export interface IssuesResponse {
  issues: Issue[];
}
