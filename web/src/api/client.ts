const BASE = "/api";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { ...headers, ...(init?.headers as Record<string, string> ?? {}) },
  });
  if (!res.ok) {
    // Try to extract the error message from the JSON response body
    let message = `API error: ${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body && typeof body.error === "string") {
        message = body.error;
      }
    } catch {
      // Response body wasn't valid JSON; use the default message
    }
    throw new Error(message);
  }
  return res.json() as Promise<T>;
}

export interface RepoView {
  path: string;
  name: string;
  agent_count: number;
}

export interface ReposResponse {
  repos: RepoView[];
  /** The repo bcd was booted against — new agents default to it. */
  default: string;
}

export interface BulkResult {
  agent: string;
  status: "ok" | "error";
  error?: string;
}

export interface AgentActivityItem {
  timestamp: string;
  event: string;
  message?: string;
  data?: Record<string, unknown>;
  // Present on the cross-agent /api/agents/activity response (#3138).
  // Omitted on the per-agent /api/agents/{name}/activity response since
  // the agent is implied by the URL.
  agent?: string;
}

export interface Agent {
  name: string;
  role: string;
  tool: string;
  /** Provider model identifier (e.g. "fable"); empty/absent = provider default. */
  model?: string;
  state: string;
  total_cost_usd: number;
  started_at: string;
  created_at: string;
  updated_at: string;
  stopped_at?: string;
  archived_at?: string;
  task?: string;
  session?: string;
  session_id?: string;
  parent_id?: string;
  children?: string[];
  total_tokens?: number;
  runtime_backend?: string;
  mcp_servers?: string[];
  /** Absolute path of the git repo this agent is bound to. Grouping
   *  the Agents page by repo uses this. */
  repo?: string;
}

export interface AgentConfig {
  system_prompt: string;
  mcp_servers: string[];
  runtime_backend: string;
  tool: string;
  session: string;
  worktree_path: string;
  created_at: string;
  started_at: string;
}

export interface NotificationSource {
  name: string;
  description: string;
  members: string[];
  member_count: number;
}

/** @deprecated Use NotificationSource instead */
export type Channel = NotificationSource;

export interface ChannelMessage {
  id: number;
  sender: string;
  content: string;
  created_at: string;
}

export interface NotifySubscription {
  id: number;
  channel: string;
  agent: string;
  mention_only: boolean;
  created_at: string;
}

export interface DeliveryEntry {
  id: number;
  logged_at: string;
  channel: string;
  agent: string;
  status: "delivered" | "failed" | "pending";
  error?: string;
  preview?: string;
}

export interface GatewayStatus {
  platform: string;
  enabled: boolean;
  channels: string[];
  bot_name?: string;
  config?: Record<string, unknown>;
}

export interface GatewayHealth {
  platform: string;
  connected: boolean;
  status: string;
  error?: string;
  last_message_at?: string;
}

export interface CostSummary {
  input_tokens: number;
  output_tokens: number;
  /** Cache tokens are reported separately — total_tokens is input + output only. */
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  total_tokens: number;
  total_cost_usd: number;
  record_count: number;
}

export interface AgentCostSummary {
  agent_id: string;
  total_cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  record_count: number;
}

export interface ModelCostSummary {
  model: string;
  total_cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
  /** input + output; cache tokens excluded. */
  total_tokens: number;
  record_count: number;
}

export interface AgentStatsSummary {
  agent_name: string;
  role: string;
  tool: string;
  runtime: string;
  state: string;
  cpu: {
    avg_percent: number;
    max_percent: number;
  };
  memory: {
    avg_bytes: number;
    max_bytes: number;
    avg_percent: number;
  };
  disk: {
    read_bytes: number;
    write_bytes: number;
  };
  network: {
    rx_bytes: number;
    tx_bytes: number;
  };
  tokens: {
    input: number;
    output: number;
    cache_read: number;
    cache_create: number;
  };
  cost: {
    total_usd: number;
  };
  models?: AgentModelCostBreakdown[];
}

export interface AgentModelCostBreakdown {
  model: string;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
}

export interface FileAttachment {
  id: string;
  filename: string;
  mime_type: string;
  size: number;
  channel: string;
  sender: string;
  created_at: string;
}

export interface DailyCost {
  date: string;
  total_cost_usd: number;
  total_tokens: number;
  record_count: number;
  input_tokens: number;
  output_tokens: number;
}

export interface BudgetStatus {
  scope: string;
  period: string;
  limit_usd: number;
  alert_at: number;
  hard_stop: boolean;
  id: number;
  updated_at: string;
}

// ResolvedRole — BFS-resolved role with inherited fields merged
export interface Role {
  Name: string;
  Prompt: string;
  MCPServers: string[];
  Secrets: string[];
  Plugins: string[];
  PromptCreate: string;
  PromptStart: string;
  PromptStop: string;
  PromptDelete: string;
  Commands: Record<string, string>;
  Skills: Record<string, string>;
  Agents: Record<string, string>;
  Rules: Record<string, string>;
  Settings: Record<string, unknown>;
  Review: string;
  CLITools?: string[];
}

export interface CLITool {
  name: string;
  command: string;
  install_cmd: string;
  builtin: boolean;
  enabled: boolean;
}

export interface MCPServer {
  name: string;
  transport: string;
  command: string;
  url: string;
  env?: Record<string, string>;
  args?: string[];
  enabled: boolean;
}

export interface Tool {
  name: string;
  type: "provider" | "mcp" | "cli";
  status: string;
  transport?: string;
  command?: string;
  url?: string;
  version?: string;
  error?: string;
  required?: boolean;
  install_cmd?: string;
  upgrade_cmd?: string;
}

export interface ProviderInfo {
  name: string;
  description: string;
  binary: string;
  command: string;
  install_hint: string;
  version: string;
  status: string;
  /** Curated model list for UI pickers; empty = no model selection. */
  models?: string[];
  total_cost_usd: number;
  total_tokens: number;
  agent_count: number;
  installed: boolean;
  enabled: boolean;
}

export interface ProviderAgentSummary {
  name: string;
  role: string;
  state: string;
}

export interface ProviderModelCost {
  model: string;
  total_tokens: number;
  total_cost_usd: number;
}

export interface ProviderDetailResponse {
  name: string;
  description: string;
  binary: string;
  command: string;
  install_hint: string;
  version: string;
  status: string;
  total_cost_usd: number;
  total_tokens: number;
  agent_count: number;
  installed: boolean;
  enabled: boolean;
  config: Record<string, string>;
  agents: ProviderAgentSummary[];
  cost_by_model: ProviderModelCost[];
}

export interface ProviderCommand {
  name: string;
  command: string;
  description: string;
  args?: string;
}

export interface ProviderMCPServer {
  name: string;
  transport: string;
  url?: string;
  command?: string;
  enabled: boolean;
  status?: string;
  error?: string;
}

export interface ProviderUpdateCheck {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  update_command: string;
}

export interface EventLogEntry {
  id: number;
  type: string;
  agent: string;
  message: string;
  created_at: string;
}

export interface CronJob {
  name: string;
  schedule: string;
  command: string;
  enabled: boolean;
  running: boolean;
  run_count: number;
  last_run: string | null;
  next_run: string | null;
  created_at: string;
}

export interface CronLogEntry {
  id: number;
  job_name: string;
  status: string;
  output: string;
  run_at: string;
  duration_ms: number;
}

export interface Secret {
  name: string;
  description: string;
  backend: string;
  created_at: string;
}

export interface SystemStats {
  hostname: string;
  os: string;
  arch: string;
  cpus: number;
  cpu_usage_percent: number;
  memory_total_bytes: number;
  memory_used_bytes: number;
  memory_usage_percent: number;
  disk_total_bytes: number;
  disk_used_bytes: number;
  disk_usage_percent: number;
  go_version: string;
  uptime_seconds: number;
  goroutines: number;
}

export interface StatsSummary {
  agents_total: number;
  agents_running: number;
  agents_stopped: number;
  channels_total: number;
  messages_total: number;
  total_cost_usd: number;
  roles_total: number;
  tools_total: number;
  uptime_seconds: number;
}

export interface ChannelTopSender {
  sender: string;
  count: number;
}

export interface ChannelStats {
  name: string;
  message_count: number;
  member_count: number;
  last_activity: string;
  top_senders: ChannelTopSender[];
}


// ComputedStats — computed from hook events in SQLite, no TimescaleDB required.
// Token and cost fields are populated from the cost store when available.
// CPU/mem are sampled live via ps aux as a fallback when TimescaleDB is unavailable.
export interface ComputedStats {
  total_events: number;
  tool_calls: number;
  tool_breakdown: Record<string, number>;
  session_duration_sec: number;
  last_active: string;
  input_tokens: number;
  output_tokens: number;
  tokens: number;
  cost_usd: number;
  disk_bytes: number;
  channel_sent: number;
  channel_received: number;
  network_note?: string;
  cpu_percent?: number;
  mem_used_bytes?: number;
}

// TimescaleDB timeseries types
export interface SystemMetricTS {
  time: string;
  system_name: string;
  cpu_percent: number;
  mem_used_bytes: number;
  mem_limit_bytes: number;
  mem_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  disk_read_bytes: number;
  disk_write_bytes: number;
}

export interface AgentMetricTS {
  time: string;
  agent_name: string;
  role: string;
  tool: string;
  runtime: string;
  state: string;
  cpu_percent: number;
  mem_used_bytes: number;
  mem_limit_bytes: number;
  mem_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  disk_read_bytes: number;
  disk_write_bytes: number;
}

export interface TokenMetricTS {
  time: string;
  agent_name: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_create: number;
  total_cost_usd: number;
}

export interface ChannelMetricTS {
  time: string;
  channel_name: string;
  message_count: number;
  member_count: number;
  reaction_count: number;
}

function qs(params?: Record<string, string>): string {
  if (!params) return "";
  const s = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ""),
  ).toString();
  return s ? `?${s}` : "";
}

export interface SettingsConfig {
  version: number;
  user: { name: string };
  server: { host: string; port: number; cors_origin: string };
  runtime: {
    default: string;
    docker: {
      image: string;
      network: string;
      docker_socket_path: string;
      extra_mounts: string[];
      cpus: number;
      memory_mb: number;
    };
    tmux: {
      session_prefix: string;
      history_limit: number;
      default_shell: string;
    };
  };
  providers: {
    default: string;
    providers: Record<string, { command: string }>;
  };
  gateways: {
    telegram?: { enabled: boolean; bot_token: string; mode: string };
    discord?: { enabled: boolean; bot_token: string };
    slack?: { enabled: boolean; bot_token: string; app_token: string; mode: string };
  };
  cron: { poll_interval_seconds: number; job_timeout_seconds: number };
  storage: {
    default: string;
    sqlite: { path: string };
    sql?: { host: string; port: number; user: string; password: string; database: string };
    timescale?: { host: string; port: number; user: string; password: string; database: string };
  };
  logs: { path: string; max_bytes: number };
  ui: { theme: string; mode: string; default_view: string };
}

/** Split "slack:eng" into { gw: "slack", ch: "eng" }. Any "platform:channel" pattern works. */
function splitChannel(channel: string): { gw: string; ch: string } {
  const idx = channel.indexOf(":");
  if (idx > 0) {
    return { gw: channel.slice(0, idx), ch: channel.slice(idx + 1) };
  }
  return { gw: "", ch: "" };
}

export const api = {
  /** List all agents. bcd is single-tenant: agents carry their repo as
   *  a property, so the list is always global. */
  listAgents: () => request<Agent[]>("/agents"),
  getAgent: (name: string) =>
    request<Agent>(`/agents/${encodeURIComponent(name)}`),
  getAgentPeek: (name: string, lines = 50) =>
    request<{ output: string }>(
      `/agents/${encodeURIComponent(name)}/peek?${new URLSearchParams({ lines: String(lines) })}`,
    ),
  startAgent: (name: string) =>
    request<Agent>(`/agents/${encodeURIComponent(name)}/start`, {
      method: "POST",
    }),
  stopAgent: (name: string) =>
    request<void>(`/agents/${encodeURIComponent(name)}/stop`, {
      method: "POST",
    }),
  createAgent: (opts: {
    name?: string;
    role: string;
    tool?: string;
    model?: string;
    runtime?: string;
  }) =>
    request<Agent>("/agents", {
      method: "POST",
      body: JSON.stringify(opts),
    }),
  generateAgentName: () => request<{ name: string }>("/agents/generate-name"),
  deleteAgent: (name: string, force = false) =>
    request<void>(
      `/agents/${encodeURIComponent(name)}${force ? "?force=true" : ""}`,
      { method: "DELETE" },
    ),
  renameAgent: (name: string, newName: string) =>
    request<Agent>(`/agents/${encodeURIComponent(name)}/rename`, {
      method: "POST",
      body: JSON.stringify({ new_name: newName }),
    }),
  archiveAgent: (name: string) =>
    request<void>(`/agents/${encodeURIComponent(name)}/archive`, { method: "POST" }),
  unarchiveAgent: (name: string) =>
    request<void>(`/agents/${encodeURIComponent(name)}/unarchive`, { method: "POST" }),

  // Cross-repo cost rollup.
  globalCosts: (opts: { start?: string; groupBy?: "repo" | "project" } = {}) => {
    const q = new URLSearchParams();
    if (opts.start) q.set("start", opts.start);
    if (opts.groupBy) q.set("groupBy", opts.groupBy);
    const qs = q.toString();
    return request<{
      range: { start: string };
      groupBy: "repo" | "project";
      rows: Array<{ key: string; label: string; total: number }>;
    }>(`/global/costs${qs ? "?" + qs : ""}`);
  },
  stopAllAgents: () => request<void>("/agents/stop-all", { method: "POST" }),

  // Bulk agent operations — parallel ops with per-agent results
  bulkStartAgents: (agents: string[]) =>
    request<{ results: BulkResult[] }>("/agents/bulk/start", {
      method: "POST",
      body: JSON.stringify({ agents }),
    }),
  bulkStopAgents: (agents: string[]) =>
    request<{ results: BulkResult[] }>("/agents/bulk/stop", {
      method: "POST",
      body: JSON.stringify({ agents }),
    }),
  bulkDeleteAgents: (agents: string[], force = false) =>
    request<{ results: BulkResult[] }>("/agents/bulk/delete", {
      method: "POST",
      body: JSON.stringify({ agents, force }),
    }),
  bulkMessageAgents: (agents: string[], message: string) =>
    request<{ results: BulkResult[] }>("/agents/bulk/message", {
      method: "POST",
      body: JSON.stringify({ agents, message }),
    }),

  // Agent activity timeline — newest first, capped at `limit` entries (default 50, max 1000).
  getAgentActivity: (name: string, limit = 50) =>
    request<AgentActivityItem[]>(`/agents/${encodeURIComponent(name)}/activity?limit=${limit}`),
  // Cross-agent recent activity for Live page hydration (#3138). Newest
  // first; each item carries an `agent` field so callers can route to
  // the right card.
  getActivity: (limit = 200) =>
    request<AgentActivityItem[]>(`/agents/activity?limit=${limit}`),

  getAgentConfig: (name: string) =>
    request<AgentConfig>(`/agents/${encodeURIComponent(name)}/config`),
  patchAgentConfig: (name: string, patch: { system_prompt?: string }) =>
    request<void>(`/agents/${encodeURIComponent(name)}/config`, { method: "PATCH", body: JSON.stringify(patch) }),
  getAgentMcps: (name: string) =>
    request<Array<{ name: string }>>(`/agents/${encodeURIComponent(name)}/mcps`),
  addAgentMcp: (name: string, mcpName: string) =>
    request<void>(`/agents/${encodeURIComponent(name)}/mcps`, { method: "POST", body: JSON.stringify({ name: mcpName }) }),
  removeAgentMcp: (name: string, mcpName: string) =>
    request<void>(`/agents/${encodeURIComponent(name)}/mcps/${encodeURIComponent(mcpName)}`, { method: "DELETE" }),
  getAgentEnv: (name: string) =>
    request<Array<{ key: string; value: string }>>(`/agents/${encodeURIComponent(name)}/env`),
  putAgentEnv: (name: string, vars: Array<{ key: string; value: string }>) =>
    request<Array<{ key: string; value: string }>>(`/agents/${encodeURIComponent(name)}/env`, { method: "PUT", body: JSON.stringify(vars) }),

  listNotificationSources: () => request<NotificationSource[]>("/channels"),
  /** @deprecated Use listNotificationSources instead */
  listChannels: () => request<NotificationSource[]>("/channels"),
  getChannelHistory: (
    name: string,
    limit = 50,
    before?: number,
  ) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (before !== undefined) params.set("before", String(before));
    return request<ChannelMessage[]>(
      `/channels/${encodeURIComponent(name)}/history?${params}`,
    );
  },
  // Gateway-scoped subscription API (proposal-aligned)
  listSubscriptions: () =>
    request<NotifySubscription[]>("/notify/subscriptions"),
  getChannelSubscriptions: (channel: string) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<NotifySubscription[]>(`/gateways/${gw}/channels/${ch}/agents`);
    }
    return request<NotifySubscription[]>(`/notify/subscriptions/${encodeURIComponent(channel)}`);
  },
  subscribe: (channel: string, agent: string, mentionOnly = false) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<{ status: string }>(`/gateways/${gw}/channels/${ch}/agents`, {
        method: "POST",
        body: JSON.stringify({ agent, mention_only: mentionOnly }),
      });
    }
    return request<{ status: string }>("/notify/subscriptions", {
      method: "POST",
      body: JSON.stringify({ channel, agent, mention_only: mentionOnly }),
    });
  },
  unsubscribe: (channel: string, agent: string) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<{ status: string }>(`/gateways/${gw}/channels/${ch}/agents/${encodeURIComponent(agent)}`, {
        method: "DELETE",
      });
    }
    return request<{ status: string }>(
      `/notify/subscriptions/${encodeURIComponent(channel)}?agent=${encodeURIComponent(agent)}`,
      { method: "DELETE" },
    );
  },
  setMentionOnly: (channel: string, agent: string, mentionOnly: boolean) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<{ status: string }>(`/gateways/${gw}/channels/${ch}/agents/${encodeURIComponent(agent)}`, {
        method: "PATCH",
        body: JSON.stringify({ mention_only: mentionOnly }),
      });
    }
    return request<{ status: string }>(`/notify/subscriptions/${encodeURIComponent(channel)}`, {
      method: "PATCH",
      body: JSON.stringify({ agent, mention_only: mentionOnly }),
    });
  },
  getChannelActivity: (channel: string, limit = 50) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<DeliveryEntry[]>(`/gateways/${gw}/channels/${ch}/activity?limit=${limit}`);
    }
    return request<DeliveryEntry[]>(`/notify/activity/${encodeURIComponent(channel)}?limit=${limit}`);
  },
  listGateways: () =>
    request<GatewayStatus[]>("/gateways"),
  getGatewayHealth: (platform: string) =>
    request<GatewayHealth>(`/gateways/${encodeURIComponent(platform)}/health`),

  getCostSummary: () => request<CostSummary>("/costs"),
  getCostByAgent: () => request<AgentCostSummary[]>("/costs/agents"),
  getCostByModel: () => request<ModelCostSummary[]>("/costs/models"),
  getCostDaily: (days = 14) =>
    request<DailyCost[]>(`/costs/daily?days=${days}`),
  getCostBudgets: () => request<BudgetStatus[]>("/costs/budgets"),
  createCostBudget: (budget: {
    scope: string;
    period: string;
    limit_usd: number;
    alert_at?: number;
    hard_stop?: boolean;
  }) =>
    request<BudgetStatus>("/costs/budgets", {
      method: "POST",
      body: JSON.stringify(budget),
    }),
  deleteCostBudget: (scope: string) =>
    request<void>(`/costs/budgets/${encodeURIComponent(scope)}`, {
      method: "DELETE",
    }),

  listRoles: () => request<Record<string, Role>>("/roles"),
  createRole: (role: {
    name: string;
    description?: string;
    prompt?: string;
    parent_roles?: string[];
    mcp_servers?: string[];
    secrets?: string[];
    plugins?: string[];
    rules?: Record<string, string>;
    commands?: Record<string, string>;
    skills?: Record<string, string>;
    agents?: Record<string, string>;
    prompt_start?: string;
    prompt_stop?: string;
    prompt_create?: string;
    prompt_delete?: string;
    review?: string;
  }) => request<Role>("/roles", { method: "POST", body: JSON.stringify(role) }),
  updateRole: (
    name: string,
    role: {
      description?: string;
      prompt?: string;
      parent_roles?: string[];
      mcp_servers?: string[];
      secrets?: string[];
      plugins?: string[];
      rules?: Record<string, string>;
      commands?: Record<string, string>;
      skills?: Record<string, string>;
      agents?: Record<string, string>;
      prompt_start?: string;
      prompt_stop?: string;
      prompt_create?: string;
      prompt_delete?: string;
      review?: string;
    },
  ) =>
    request<Role>(`/roles/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(role),
    }),
  deleteRole: (name: string) =>
    request<void>(`/roles/${encodeURIComponent(name)}`, { method: "DELETE" }),
  listProviders: () => request<ProviderInfo[]>("/providers"),
  getProvider: (name: string) =>
    request<ProviderDetailResponse>(`/providers/${encodeURIComponent(name)}`),
  getProviderCommands: (name: string) =>
    request<ProviderCommand[]>(`/providers/${encodeURIComponent(name)}/commands`),
  getProviderMCPs: (name: string) =>
    request<ProviderMCPServer[]>(`/providers/${encodeURIComponent(name)}/mcps`),
  addProviderMCP: (name: string, mcp: { name: string; transport?: string; url?: string; command?: string }) =>
    request<{ status: string; provider: string; mcp: string }>(`/providers/${encodeURIComponent(name)}/mcps`, {
      method: "POST",
      body: JSON.stringify(mcp),
    }),
  installProvider: (name: string) =>
    request<{ status: string; provider: string; install_cmd: string }>(`/providers/${encodeURIComponent(name)}/install`, {
      method: "POST",
    }),
  updateProvider: (name: string) =>
    request<{ status: string; provider: string; update_cmd: string }>(`/providers/${encodeURIComponent(name)}/update`, {
      method: "POST",
    }),
  checkProviderUpdate: (name: string) =>
    request<ProviderUpdateCheck>(`/providers/${encodeURIComponent(name)}/check-update`, {
      method: "POST",
    }),
  updateProviderConfig: (name: string, config: Record<string, string>) =>
    request<{ status: string; provider: string; command: string }>(`/providers/${encodeURIComponent(name)}/config`, {
      method: "PATCH",
      body: JSON.stringify(config),
    }),
  listCLITools: () => request<CLITool[]>("/tools"),
  enableTool: (name: string) =>
    request<{ enabled: boolean }>(`/tools/${encodeURIComponent(name)}/enable`, {
      method: "POST",
    }),
  disableTool: (name: string) =>
    request<{ enabled: boolean }>(
      `/tools/${encodeURIComponent(name)}/disable`,
      { method: "POST" },
    ),
  deleteTool: (name: string) =>
    request<void>(`/tools/${encodeURIComponent(name)}`, { method: "DELETE" }),
  listMCP: () => request<MCPServer[]>("/mcp"),
  registerMCP: (server: Omit<MCPServer, "enabled"> & { enabled?: boolean }) =>
    request<MCPServer>("/mcp", {
      method: "POST",
      body: JSON.stringify(server),
    }),
  removeMCP: (name: string) =>
    request<void>(`/mcp/${encodeURIComponent(name)}`, { method: "DELETE" }),
  enableMCP: (name: string) =>
    request<void>(`/mcp/${encodeURIComponent(name)}/enable`, {
      method: "POST",
    }),
  disableMCP: (name: string) =>
    request<void>(`/mcp/${encodeURIComponent(name)}/disable`, {
      method: "POST",
    }),
  updateMCPEnv: (name: string, env: Record<string, string>) =>
    request<MCPServer>(`/mcp/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify({ env }),
    }),

  /** Tool list — merges MCP + CLI tools with status. */
  listTools: () => request<Tool[]>("/tools/unified"),

  /** Run live health checks on all tools. */
  checkTools: () =>
    request<Tool[]>("/tools/unified/check", { method: "POST" }),

  /** Create or update a CLI tool. */
  upsertTool: (tool: Partial<CLITool> & { name: string }) =>
    request<CLITool>(`/tools/${encodeURIComponent(tool.name)}`, {
      method: "PUT",
      body: JSON.stringify(tool),
    }),

  getLogs: (tail = 50) =>
    request<EventLogEntry[]>(
      `/logs?${new URLSearchParams({ tail: String(tail) })}`,
    ),
  getAgentLogs: (agent: string, tail = 50) =>
    request<EventLogEntry[]>(
      `/logs?${new URLSearchParams({ tail: String(tail), agent })}`,
    ),
  listCron: () => request<CronJob[]>("/cron"),
  createCron: (job: { name: string; schedule: string; command: string }) =>
    request<CronJob>("/cron", { method: "POST", body: JSON.stringify(job) }),
  runCron: (name: string) =>
    request<void>(`/cron/${encodeURIComponent(name)}/run`, { method: "POST" }),
  enableCron: (name: string) =>
    request<void>(`/cron/${encodeURIComponent(name)}/enable`, {
      method: "POST",
    }),
  disableCron: (name: string) =>
    request<void>(`/cron/${encodeURIComponent(name)}/disable`, {
      method: "POST",
    }),
  deleteCron: (name: string) =>
    request<void>(`/cron/${encodeURIComponent(name)}`, { method: "DELETE" }),
  getCronLogs: (name: string) =>
    request<CronLogEntry[]>(`/cron/${encodeURIComponent(name)}/logs`),
  listSecrets: () => request<Secret[]>("/secrets"),
  createSecret: (name: string, value: string, description?: string) =>
    request<Secret>("/secrets", {
      method: "POST",
      body: JSON.stringify({ name, value, description: description ?? "" }),
    }),
  updateSecret: (name: string, value: string) =>
    request<Secret>(`/secrets/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),
  deleteSecret: (name: string) =>
    request<void>(`/secrets/${encodeURIComponent(name)}`, { method: "DELETE" }),
  /** List repos known to the daemon (every repo referenced by an agent
   *  plus the boot repo) and the default repo for new agents. */
  getRepos: () => request<ReposResponse>("/repos"),

  getStatsSystem: () => request<SystemStats>("/stats/system"),
  getStatsSummary: () => request<StatsSummary>("/stats/summary"),
  getStatsChannels: () => request<ChannelStats[]>("/stats/channels"),

  // Resource-scoped stats (TimescaleDB timeseries)
  getSystemStats: (metric: string, params?: Record<string, string>) =>
    request<SystemMetricTS[]>(`/system/stats/${metric}${qs(params)}`),
  getAgentStats: (metric: string, params?: Record<string, string>) =>
    request<AgentMetricTS[]>(`/agents/stats/${metric}${qs(params)}`),
  getAgentStatsLatest: () =>
    request<AgentMetricTS[]>("/agents/stats/latest"),
  getAgentTokenStats: (params?: Record<string, string>) =>
    request<TokenMetricTS[]>(`/agents/stats/tokens${qs(params)}`),
  getAgentCostStats: (params?: Record<string, string>) =>
    request<TokenMetricTS[]>(`/agents/stats/cost${qs(params)}`),
  getChannelStats: (metric: string, params?: Record<string, string>) =>
    request<ChannelMetricTS[]>(`/channels/stats/${metric}${qs(params)}`),

  /** Unified per-agent stats summary — single call for drill-down. */
  getAgentStatsSummary: (name: string, params?: Record<string, string>) =>
    request<AgentStatsSummary>(
      `/agents/stats/summary/${encodeURIComponent(name)}${qs(params)}`,
    ),

  /** Computed stats from hook events — works without TimescaleDB. */
  getAgentComputedStats: (name: string) =>
    request<ComputedStats>(`/agents/${encodeURIComponent(name)}/stats-computed`),

  /** Upload a file attachment. */
  uploadFile: async (file: File, channel: string, sender: string) => {
    const form = new FormData();
    form.append("file", file);
    form.append("channel", channel);
    form.append("sender", sender);
    const res = await fetch(`${BASE}/files/upload`, { method: "POST", body: form });
    if (!res.ok) throw new Error(`Upload failed: ${res.status}`);
    return res.json() as Promise<FileAttachment>;
  },

  /** Get file download URL. */
  getFileUrl: (id: string) => `${BASE}/files/${encodeURIComponent(id)}`,

  getSystemInfo: () => request<{ hostname: string; os: string; arch: string }>("/system/info"),
  getSettings: () => request<SettingsConfig>("/settings"),
  updateSettings: (patch: Record<string, unknown>) =>
    request<SettingsConfig>("/settings", {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),

};
