import { cachedGet, invalidate } from "./cache";

const BASE = "/api";

// Cache keys for the hot, shared GETs routed through the module-level cache.
// Kept here so mutations and the SSE layer can invalidate them by name.
export const CACHE_KEYS = {
  agents: "agents",
  apps: "apps",
} as const;

/** Invalidate the shared agents-list cache after a mutation or SSE event. */
export function invalidateAgents(): void {
  invalidate(CACHE_KEYS.agents);
}

/** Invalidate the shared apps-catalog cache. */
export function invalidateApps(): void {
  invalidate(CACHE_KEYS.apps);
}

// tap invalidates the given cache keys once a mutating request resolves, so a
// follow-up read (or poll) never serves data the mutation just changed.
function tap<T>(p: Promise<T>, keys: string[]): Promise<T> {
  return p.then((v) => {
    for (const k of keys) invalidate(k);
    return v;
  });
}

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
  /** The repo the daemon was booted against — new agents default to it. */
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
  /** Configured environment variables injected at spawn. Values with
   *  `${secret:NAME}` references are returned as the reference — the
   *  daemon never sends resolved secret values. */
  env?: Record<string, string>;
  /** Per-agent Docker CPU cap in cores (0/absent = inherit fleet default). */
  cpus?: number;
  /** Per-agent Docker memory cap in MB (0/absent = inherit fleet default). */
  memory_mb?: number;
}

export interface AgentConfig {
  system_prompt: string;
  mcp_servers: string[];
  runtime_backend: string;
  tool: string;
  model: string;
  session: string;
  worktree_path: string;
  created_at: string;
  started_at: string;
  /** Per-agent Docker CPU cap in cores; 0 = inherit the fleet default. */
  cpus: number;
  /** Per-agent Docker memory cap in MB; 0 = inherit the fleet default. */
  memory_mb: number;
}

export interface NotificationSource {
  name: string;
  description: string;
  members: string[];
  member_count: number;
}

export interface ChannelMessage {
  id: number;
  sender: string;
  /** Loopback image-proxy path for the sender's real avatar, when the
   *  platform resolved one (e.g. Slack users.info). Absent → initials. */
  avatar_url?: string;
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

/* ── Apps — external platform integrations (/api/apps) ─────────── */

/** How an app authenticates, from its backend descriptor. */
export type AppAuthKind = "token" | "oauth" | "qr" | "webhook-secret" | "none";

/** One config/credential field of an app descriptor. Secret fields are
 *  stored in the encrypted vault server-side and never echoed back. */
export interface AppFieldSpec {
  key: string;
  label: string;
  placeholder?: string;
  secret: boolean;
  required: boolean;
}

/** An app's static self-description from GET /api/apps — drives the
 *  connect flow (fields, docs, auth kind); no per-app UI code. */
export interface AppDescriptor {
  id: string;
  label: string;
  auth: AppAuthKind;
  /** Allows labeled instances ("telegram:alerts"). */
  multi: boolean;
  fields: AppFieldSpec[];
  docs: string[];
  /** True when the plugin supports browser sign-in (app.OAuthFlow) —
   *  the wizard offers "Sign in with <app>" alongside manual fields. */
  oauth_available?: boolean;
}

/** One connected app instance with live adapter status. `config` holds
 *  plain fields plus server-computed `has_<field>` booleans for secret
 *  fields; `channels` are the adapter's discovered bc channel keys. */
export interface AppInstance {
  name: string;
  app: string;
  enabled: boolean;
  config?: Record<string, string | boolean>;
  connected: boolean;
  bot_name?: string;
  error?: string;
  channels?: string[];
}

export interface AppsCatalog {
  catalog: AppDescriptor[];
  instances: AppInstance[];
}

/** QR/OAuth pairing progress from /api/apps/{name}/auth. */
export interface AppPairInfo {
  state: string;
  qr_data_url?: string;
  phone?: string;
  error?: string;
}

/** A begun browser-auth session from POST /api/apps/{name}/auth on an
 *  OAuth-capable app. Device flow carries verification_url + user_code;
 *  callback flow carries auth_url. */
export interface AppAuthSession {
  id: string;
  kind: string;
  state: string;
  auth_url?: string;
  verification_url?: string;
  user_code?: string;
  expires_at?: string;
  interval_seconds?: number;
}

/** OAuth progress from GET /api/apps/{name}/auth/status?session=<id>.
 *  Secrets never cross the wire — the server persists them to the vault. */
export interface AppAuthResult {
  state: string;
  error?: string;
  warning?: string;
}

/** Flattened per-instance view used by the drawer tree and Apps home —
 *  derived from AppsCatalog.instances (platform = instance name). */
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

export function instancesToStatuses(instances: AppInstance[]): GatewayStatus[] {
  return instances.map((i) => ({
    platform: i.name,
    enabled: i.enabled,
    channels: i.channels ?? [],
    bot_name: i.bot_name,
    config: i.config,
  }));
}

/** One connected/configured app on the notifications overview (#3310). */
export interface OverviewApp {
  name: string;
  platform: string;
  connected: boolean;
  disconnect_reason?: string;
  channel_count: number;
  last_activity?: string;
}

/** One channel row on the notifications overview, enriched with the
 *  display metadata resolved by the platform adapter (#3310). All
 *  metadata fields are optional — the page degrades to raw channel ids
 *  when the backend has not resolved identities yet. */
export interface OverviewChannel {
  channel: string;
  platform: string;
  display_name?: string;
  /** "group" | "person" when the adapter could classify the channel. */
  kind?: string;
  /** Loopback image-proxy path for the channel's real picture (a person's
   *  profile photo or a group icon), when the adapter resolved one. */
  avatar_url?: string;
  participant_count?: number;
  subscriber_count?: number;
  message_count?: number;
  last_activity?: string;
}

export interface NotificationsOverview {
  apps?: OverviewApp[];
  channels?: OverviewChannel[];
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
  /** input + output; cache tokens excluded. */
  total_tokens: number;
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
  // The /costs/daily endpoint emits this as `cost_usd` (unlike the agent /
  // model summaries, which use total_cost_usd).
  cost_usd: number;
  total_tokens: number;
  record_count: number;
  input_tokens: number;
  output_tokens: number;
}

/** One agent-scoped ledger day from GET /costs/agent/{id}. */
export interface AgentDailyCost {
  agent_id: string;
  date: string;
  cost_usd: number;
  total_tokens: number;
  record_count: number;
  input_tokens: number;
  output_tokens: number;
}

/** GET /costs/agent/{id} — lifetime summary + last-30d daily ledger. */
export interface AgentCostDetail {
  summary: AgentCostSummary | null;
  daily: AgentDailyCost[] | null;
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

/** One detected host package manager. */
export interface PackageManager {
  id: string;
  name: string;
  version: string;
  available: boolean;
  /** true = this manager exposes a registry search the UI could drive. */
  searchable: boolean;
}

export interface PackageManagersResponse {
  os: string;
  arch: string;
  managers: PackageManager[];
}

/** One registry search hit. */
export interface PackageSearchResult {
  name: string;
  description: string;
}

export interface PackageSearchResponse {
  manager: string;
  query: string;
  results: PackageSearchResult[];
  /** Present when the search command errored or found nothing. */
  error?: string;
}

/** Managers whose install the server runs directly (no sudo, non-interactive).
 *  Others surface a copyable command instead. Mirrors the backend installSpecs. */
export const DIRECT_INSTALL_MANAGERS = new Set(["brew", "npm", "cargo"]);

/** A provider model with live availability status. */
export interface ModelInfo {
  id: string;
  /** true = confirmed live from provider CLI; false = static fallback (auth unverified). */
  available: boolean;
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
  models?: ModelInfo[];
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
  /** true = needs a TTY / mutates auth state; the UI must not auto-run it. */
  interactive: boolean;
  /** true = safe to execute inline via runProviderCommand (no TTY, no args). */
  runnable: boolean;
}

/** Result of running one allowlisted provider subcommand inline. */
export interface ProviderRunResult {
  command: string;
  output: string;
  exit_code: number;
  truncated: boolean;
  timed_out: boolean;
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
    /** Persisted default model id; empty = provider's own default. */
    default_model?: string;
    providers: Record<string, { command: string }>;
  };
  gateways: {
    telegram?: { enabled: boolean; bot_token: string; mode: string };
    discord?: { enabled: boolean; bot_token: string };
    slack?: { enabled: boolean; bot_token: string; app_token: string; mode: string };
  };
  storage: {
    default: string;
    sqlite: { path: string };
    sql?: { host: string; port: number; user: string; password: string; database: string };
    timescale?: { host: string; port: number; user: string; password: string; database: string };
  };
  logs: { path: string; max_bytes: number };
  ui: { theme: string; mode: string; default_view: string };
  /** Operator delivery preferences: which channel reaches you, on/off. */
  notifications?: { default_channel: string; enabled: boolean };
  onboarding?: { step: string; completed: string[] };
}

/* ── Onboarding — first-run setup wizard state (/api/onboarding) ─────── */

export interface OnboardingState {
  /** True when the app should route to /welcome instead of Home. */
  firstRun: boolean;
  hasAgents: boolean;
  prefsValid: boolean;
  /** Ids of finished steps; ends with "done" once the wizard completes. */
  completed: string[];
  /** Id of the last-visited step, for resume. */
  step: string;
}

/* ── Doctor / health ──────────────────────────────────────────────────
   The daemon's machine-readiness report (GET /api/doctor) and the
   degraded-services health probe (GET /api/health). Shapes mirror
   pkg/doctor and server/server.go's apiHealth handler. */

export type DoctorSeverity = "ok" | "warn" | "fail";

export interface DoctorItem {
  name: string;
  message: string;
  fix?: string;
  severity: DoctorSeverity;
}

export interface DoctorCategory {
  name: string;
  items: DoctorItem[];
}

export interface DoctorReport {
  categories: DoctorCategory[];
}

export interface HealthReport {
  status: "ok" | "degraded";
  db?: string;
  version?: string;
  commit?: string;
  /** service name → reason, present only when status is "degraded". */
  degraded?: Record<string, string>;
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
  /** List all agents. the daemon is single-tenant: agents carry their repo as
   *  a property, so the list is always global. */
  listAgents: () =>
    cachedGet(CACHE_KEYS.agents, () => request<Agent[]>("/agents")),
  getAgent: (name: string) =>
    request<Agent>(`/agents/${encodeURIComponent(name)}`),
  getAgentPeek: (name: string, lines = 50) =>
    request<{ output: string }>(
      `/agents/${encodeURIComponent(name)}/peek?${new URLSearchParams({ lines: String(lines) })}`,
    ),
  startAgent: (name: string) =>
    tap(
      request<Agent>(`/agents/${encodeURIComponent(name)}/start`, {
        method: "POST",
      }),
      [CACHE_KEYS.agents],
    ),
  stopAgent: (name: string) =>
    tap(
      request<void>(`/agents/${encodeURIComponent(name)}/stop`, {
        method: "POST",
      }),
      [CACHE_KEYS.agents],
    ),
  createAgent: (opts: {
    name?: string;
    role: string;
    tool?: string;
    model?: string;
    runtime?: string;
    /** Environment variables for the agent. Values may hold
     *  `${secret:NAME}` references resolved from the vault at spawn. */
    env?: Record<string, string>;
  }) =>
    tap(
      request<Agent>("/agents", {
        method: "POST",
        body: JSON.stringify(opts),
      }),
      [CACHE_KEYS.agents],
    ),
  generateAgentName: () => request<{ name: string }>("/agents/generate-name"),
  deleteAgent: (name: string, force = false) =>
    tap(
      request<void>(
        `/agents/${encodeURIComponent(name)}${force ? "?force=true" : ""}`,
        { method: "DELETE" },
      ),
      [CACHE_KEYS.agents],
    ),
  renameAgent: (name: string, newName: string) =>
    tap(
      request<Agent>(`/agents/${encodeURIComponent(name)}/rename`, {
        method: "POST",
        body: JSON.stringify({ new_name: newName }),
      }),
      [CACHE_KEYS.agents],
    ),
  archiveAgent: (name: string) =>
    tap(
      request<void>(`/agents/${encodeURIComponent(name)}/archive`, { method: "POST" }),
      [CACHE_KEYS.agents],
    ),
  unarchiveAgent: (name: string) =>
    tap(
      request<void>(`/agents/${encodeURIComponent(name)}/unarchive`, { method: "POST" }),
      [CACHE_KEYS.agents],
    ),

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
  stopAllAgents: () =>
    tap(request<void>("/agents/stop-all", { method: "POST" }), [CACHE_KEYS.agents]),

  // Bulk agent operations — parallel ops with per-agent results
  bulkStartAgents: (agents: string[]) =>
    tap(
      request<{ results: BulkResult[] }>("/agents/bulk/start", {
        method: "POST",
        body: JSON.stringify({ agents }),
      }),
      [CACHE_KEYS.agents],
    ),
  bulkStopAgents: (agents: string[]) =>
    tap(
      request<{ results: BulkResult[] }>("/agents/bulk/stop", {
        method: "POST",
        body: JSON.stringify({ agents }),
      }),
      [CACHE_KEYS.agents],
    ),
  bulkDeleteAgents: (agents: string[], force = false) =>
    tap(
      request<{ results: BulkResult[] }>("/agents/bulk/delete", {
        method: "POST",
        body: JSON.stringify({ agents, force }),
      }),
      [CACHE_KEYS.agents],
    ),
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
  patchAgentConfig: (
    name: string,
    patch: { system_prompt?: string; model?: string; cpus?: number; memory_mb?: number },
  ) =>
    request<AgentConfig>(`/agents/${encodeURIComponent(name)}/config`, { method: "PATCH", body: JSON.stringify(patch) }),
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

  listNotificationSources: () => request<NotificationSource[]>("/apps/channels"),
  getChannelHistory: (
    name: string,
    limit = 50,
    before?: number,
  ) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (before !== undefined) params.set("before", String(before));
    return request<ChannelMessage[]>(
      `/apps/channels/${encodeURIComponent(name)}/history?${params}`,
    );
  },
  // App-scoped subscription API — channels live under their app instance.
  listSubscriptions: () =>
    request<NotifySubscription[]>("/notify/subscriptions"),
  getChannelSubscriptions: (channel: string) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<NotifySubscription[]>(`/apps/${gw}/channels/${ch}/agents`);
    }
    return request<NotifySubscription[]>(`/notify/subscriptions/${encodeURIComponent(channel)}`);
  },
  subscribe: (channel: string, agent: string, mentionOnly = false) => {
    const { gw, ch } = splitChannel(channel);
    if (gw && ch) {
      return request<{ status: string }>(`/apps/${gw}/channels/${ch}/agents`, {
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
      return request<{ status: string }>(`/apps/${gw}/channels/${ch}/agents/${encodeURIComponent(agent)}`, {
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
      return request<{ status: string }>(`/apps/${gw}/channels/${ch}/agents/${encodeURIComponent(agent)}`, {
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
      return request<DeliveryEntry[]>(`/apps/${gw}/channels/${ch}/activity?limit=${limit}`);
    }
    return request<DeliveryEntry[]>(`/notify/activity/${encodeURIComponent(channel)}?limit=${limit}`);
  },

  /** Descriptor catalog + connected instances with live status. */
  getApps: () =>
    cachedGet(CACHE_KEYS.apps, () => request<AppsCatalog>("/apps"), 10000),
  /** Connect or update an app instance. The server splits secret fields
   *  into the vault and plain fields into preferences, then hot-restarts
   *  the adapter. Empty secret values keep the stored secret. */
  connectApp: (
    name: string,
    opts: { app?: string; config: Record<string, string>; enabled?: boolean },
  ) =>
    request<{ status: string; name: string; app: string; enabled: boolean; warning?: string }>(
      `/apps/${encodeURIComponent(name)}`,
      { method: "POST", body: JSON.stringify(opts) },
    ),
  /** Disconnect an instance: stops the adapter, purges its vault keys
   *  and state directory. */
  disconnectApp: (name: string) =>
    request<{ status: string; name: string }>(`/apps/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  /** Begin the instance's auth flow (QR pairing for WhatsApp). */
  startAppAuth: (name: string) =>
    request<AppPairInfo>(`/apps/${encodeURIComponent(name)}/auth`, { method: "POST" }),
  /** Poll pairing/auth progress on the running adapter. */
  getAppAuthStatus: (name: string) =>
    request<AppPairInfo>(`/apps/${encodeURIComponent(name)}/auth/status`),
  /** Begin a browser sign-in (OAuth) for an OAuth-capable app. Plain
   *  descriptor fields (e.g. oauth_client_id) ride along in config and
   *  persist with the instance. */
  beginAppOAuth: (name: string, config: Record<string, string>) =>
    request<AppAuthSession>(`/apps/${encodeURIComponent(name)}/auth`, {
      method: "POST",
      body: JSON.stringify({ config }),
    }),
  /** Poll an OAuth session; on "complete" the server has already stored
   *  the credentials and hot-started the adapter. */
  getAppOAuthStatus: (name: string, sessionId: string) =>
    request<AppAuthResult>(
      `/apps/${encodeURIComponent(name)}/auth/status?session=${encodeURIComponent(sessionId)}`,
    ),
  /** Connected instances in the drawer/home's flattened shape. */
  listAppInstances: async (): Promise<GatewayStatus[]> => {
    const res = await request<AppsCatalog>("/apps");
    return instancesToStatuses(res.instances ?? []);
  },
  getAppHealth: (name: string) =>
    request<GatewayHealth>(`/apps/${encodeURIComponent(name)}/health`),
  /** Aggregated apps + channels for the Apps home (#3310).
   *  Callers must tolerate the endpoint being absent and fall back to
   *  composing the same data from the individual endpoints. */
  getNotificationsOverview: () =>
    request<NotificationsOverview>("/notifications/overview"),

  // Cost summaries accept an optional `since` (RFC3339 or YYYY-MM-DD)
  // to scope totals to a period; omitted = all time.
  getCostSummary: (opts: { since?: string } = {}) =>
    request<CostSummary>(`/costs${opts.since ? `?since=${encodeURIComponent(opts.since)}` : ""}`),
  getCostByAgent: (opts: { since?: string; limit?: number } = {}) =>
    request<AgentCostSummary[]>(`/costs/agents${qs({
      ...(opts.since ? { since: opts.since } : {}),
      ...(opts.limit ? { limit: String(opts.limit) } : {}),
    })}`),
  getCostByModel: (opts: { since?: string } = {}) =>
    request<ModelCostSummary[]>(`/costs/models${qs(opts.since ? { since: opts.since } : {})}`),
  getCostDaily: (days = 14) =>
    request<DailyCost[]>(`/costs/daily?days=${days}`),
  // Per-entity drill-down: lifetime summary + last-30d daily ledger for
  // one ledger agent id (namespaced, e.g. "mycel-a1b2c3-zen-zebra").
  getCostAgentDetail: (agentId: string) =>
    request<AgentCostDetail>(`/costs/agent/${encodeURIComponent(agentId)}`),
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
  getProviderModels: (name: string) =>
    request<ModelInfo[]>(`/providers/${encodeURIComponent(name)}/models`),
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

  /** Full machine-readiness report — tmux, git, provider CLIs, docker images. */
  getDoctor: () => request<DoctorReport>("/doctor"),
  /** Daemon health incl. degraded-service reasons (e.g. docker runtime fallback). */
  getHealth: () => request<HealthReport>("/health"),

  getSystemInfo: () => request<{ hostname: string; os: string; arch: string }>("/system/info"),
  /** Autodetected host package managers (brew/apt/npm/…) with versions. */
  getPackageManagers: () =>
    request<PackageManagersResponse>("/system/package-managers"),
  /**
   * Guarded registry search. The server validates the query charset, runs the
   * manager's own `search` subcommand with an argv slice (no shell), times it
   * out, and caps results. Managers without a vetted search spec 400.
   */
  searchPackages: (manager: string, query: string) =>
    request<PackageSearchResponse>("/system/package-search", {
      method: "POST",
      body: JSON.stringify({ manager, query }),
    }),
  /**
   * Run one allowlisted, non-interactive, no-argument provider subcommand and
   * return its output inline. Interactive/arg commands are refused by the
   * server (the UI must not offer Run for them).
   */
  runProviderCommand: (name: string, command: string) =>
    request<ProviderRunResult>(`/providers/${encodeURIComponent(name)}/run`, {
      method: "POST",
      body: JSON.stringify({ command }),
    }),
  getSettings: () => request<SettingsConfig>("/settings"),
  updateSettings: (patch: Record<string, unknown>) =>
    request<SettingsConfig>("/settings", {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),

  /**
   * Persist the fleet-wide provider defaults — the provider (and optionally
   * the model) new agents inherit when none is given. A partial providers
   * patch: the server leaves the per-provider `providers` map untouched, so
   * this never clobbers per-provider command overrides.
   */
  setProviderDefaults: (patch: { default?: string; default_model?: string }) =>
    api.updateSettings({ providers: patch }),

  /** First-run setup state: whether to show the wizard and where to resume. */
  getOnboardingState: () => request<OnboardingState>("/onboarding/state"),
  /** Persist wizard progress. Writes only the onboarding config section. */
  saveOnboarding: (step: string, completed: string[]) =>
    api.updateSettings({ onboarding: { step, completed } }),

  getInjectedInstructions: () =>
    request<{ injected_instructions: string }>("/settings/injected-instructions"),
  updateInjectedInstructions: (text: string) =>
    request<{ injected_instructions: string }>("/settings/injected-instructions", {
      method: "PUT",
      body: JSON.stringify({ injected_instructions: text }),
    }),

};
