/**
 * BC CLI service wrapper
 * Executes bc commands and parses JSON responses
 *
 * #1005: Added command result caching with stale-while-revalidate pattern
 * to reduce subprocess overhead from polling operations.
 */

import { spawn as nodeSpawn, spawnSync } from 'child_process';
import type { ChildProcess, SpawnOptions } from 'child_process';
import type {
  StatusResponse,
  ChannelsResponse,
  ChannelHistory,
  CostSummary,
  Demon,
  DemonRunLog,
  ProcessListResponse,
  LogEntry,
  Role,
  RolesResponse,
  Worktree,
  ToolInfo,
  CostUsageDailyResponse,
  CostUsageMonthlyResponse,
  CostUsageSessionResponse,
} from '../types';

// ============================================================================
// Command Result Caching (#1005 - Performance Epic Phase 3)
// ============================================================================

/**
 * Cache entry with timestamp and TTL
 */
interface CacheEntry<T> {
  data: T;
  timestamp: number;
  ttl: number;
}

/**
 * In-memory cache for command results
 */
const commandCache = new Map<string, CacheEntry<unknown>>();

// ============================================================================
// Test Injection Support (#1066 - Fix mock isolation in tests)
// ============================================================================

/**
 * Type for spawn function signature
 */
type SpawnFn = (command: string, args: string[], options?: SpawnOptions) => ChildProcess;

/**
 * Injectable spawn function - defaults to node's spawn
 * Tests can override this via _setSpawnForTesting()
 * Cast required because nodeSpawn has complex overloads with optional args
 */
let spawn: SpawnFn = nodeSpawn as unknown as SpawnFn;

/**
 * Set a custom spawn function for testing
 * @param mockSpawn - Mock spawn function to use
 * @returns Function to restore original spawn
 * @internal
 */
export function _setSpawnForTesting(mockSpawn: SpawnFn): () => void {
  const originalSpawn = spawn;
  spawn = mockSpawn;
  return () => {
    spawn = originalSpawn;
  };
}

/**
 * Default TTLs by command type (in milliseconds)
 * Shorter TTLs for frequently-changing data, longer for stable data
 */
const DEFAULT_TTLS: Record<string, number> = {
  status: 1000, // 1s - agent status changes frequently
  'channel:list': 5000, // 5s - channel list rarely changes
  'channel:history': 2000, // 2s - messages may arrive
  'cost:show': 10000, // 10s - aggregated data
  'role:list': 30000, // 30s - roles rarely change
  'process:list': 5000, // 5s - processes may change
  'demon:list': 10000, // 10s - demons stable
  logs: 2000, // 2s - logs update frequently
  'worktree:list': 30000, // 30s - worktrees stable
};

/**
 * Generate cache key from command arguments
 */
function getCacheKey(args: string[]): string {
  return args.join(':');
}

/**
 * Cache lookup result - distinguishes between cache miss and cached null
 */
interface CacheLookupResult<T> {
  hit: boolean;
  data: T | undefined;
}

/**
 * Get cached result if valid
 * Returns { hit: true, data } for cache hit (even if data is null/undefined)
 * Returns { hit: false, data: undefined } for cache miss
 */
function getCachedResult<T>(key: string): CacheLookupResult<T> {
  const entry = commandCache.get(key);
  if (!entry) {
    return { hit: false, data: undefined };
  }

  const now = Date.now();
  if (now - entry.timestamp < entry.ttl) {
    return { hit: true, data: entry.data as T };
  }

  // Expired - remove from cache
  commandCache.delete(key);
  return { hit: false, data: undefined };
}

/**
 * Store result in cache with TTL
 */
function setCachedResult(key: string, data: unknown, ttl: number): void {
  commandCache.set(key, {
    data,
    timestamp: Date.now(),
    ttl,
  });
}

/**
 * Invalidate cache entries matching a prefix
 * Called after write operations to ensure fresh data
 *
 * #1595: Supports granular invalidation with specific keys
 * Examples:
 *   invalidateCache() - clear all
 *   invalidateCache('channel') - clear all channel caches
 *   invalidateCache('channel:history:eng') - clear specific channel history
 */
export function invalidateCache(prefix?: string): void {
  if (!prefix) {
    commandCache.clear();
    return;
  }

  for (const key of commandCache.keys()) {
    if (key.startsWith(prefix)) {
      commandCache.delete(key);
    }
  }
}

/**
 * Invalidate a specific cache key (exact match)
 * #1595: For fine-grained cache control
 */
export function invalidateCacheKey(key: string): void {
  commandCache.delete(key);
}

/**
 * Check if a cache entry exists and is still valid
 * #1595: Useful for stale-while-revalidate patterns
 */
export function isCacheValid(key: string): boolean {
  const entry = commandCache.get(key);
  if (!entry) return false;
  return Date.now() - entry.timestamp < entry.ttl;
}

/**
 * Get cache entry age in milliseconds (or null if not cached)
 * #1595: Useful for debugging and cache inspection
 */
export function getCacheAge(key: string): number | null {
  const entry = commandCache.get(key);
  if (!entry) return null;
  return Date.now() - entry.timestamp;
}

/**
 * Clear all cached command results
 * Exported for testing purposes
 */
export function clearCache(): void {
  commandCache.clear();
}

/**
 * Get TTL for a command based on its type
 */
function getTtlForCommand(args: string[]): number {
  // Try specific command key first (e.g., "channel:list")
  const specificKey = args.slice(0, 2).join(':');
  if (DEFAULT_TTLS[specificKey]) {
    return DEFAULT_TTLS[specificKey];
  }

  // Fall back to command type (e.g., "status")
  const command = args[0];
  return DEFAULT_TTLS[command] ?? 5000; // Default 5s
}

// ============================================================================

/**
 * Execute a bc command and return the raw output
 * @param args - Command arguments (e.g., ['status', '--json'])
 * @returns Promise resolving to stdout string
 * @throws Error if command fails
 */
export async function execBc(args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    // Always add --json flag if not present and command supports it
    const jsonCommands = [
      'status',
      'stats',
      'channel',
      'cost',
      'logs',
      'agent',
      'process',
      'demon',
      'role',
      'worktree',
      'tool',
    ];
    const hasJsonFlag = args.includes('--json');
    const command = args[0];

    const finalArgs = [...args];
    if (!hasJsonFlag && jsonCommands.includes(command)) {
      finalArgs.push('--json');
    }

    // Use MYCEL_BIN if set, otherwise fall back to 'mycel' in PATH (#1612: Validate env vars)
    const bcBin = process.env.MYCEL_BIN ?? 'mycel';
    const bcRoot = process.env.MYCEL_ROOT ?? process.cwd();

    // Validate bcRoot exists before spawning
    // Note: We don't validate bcBin here as spawn error will handle missing executable
    // with a clearer error message from the OS

    const proc = spawn(bcBin, finalArgs, {
      stdio: ['ignore', 'pipe', 'pipe'],
      cwd: bcRoot,
    });

    let stdout = '';
    let stderr = '';
    let finished = false;

    // Timeout after 30 seconds (#1612: Improved process cleanup)
    // Use SIGTERM first, then SIGKILL after grace period
    let killTimeout: NodeJS.Timeout | undefined;
    const timeout = setTimeout(() => {
      if (!finished) {
        // Try graceful termination first
        proc.kill('SIGTERM');
        // Force kill after 2s if process doesn't exit
        killTimeout = setTimeout(() => {
          if (!finished) {
            proc.kill('SIGKILL');
          }
        }, 2000);
        finished = true;
        reject(new Error(`bc command timed out after 30s: ${args.join(' ')}`));
      }
    }, 30000);

    // Helper to clean up all timers
    const cleanupTimers = () => {
      clearTimeout(timeout);
      if (killTimeout) {
        clearTimeout(killTimeout);
      }
    };

    proc.stdout?.on('data', (data: Buffer) => {
      stdout += data.toString();
    });

    proc.stderr?.on('data', (data: Buffer) => {
      stderr += data.toString();
    });

    proc.on('close', (code: number | null) => {
      if (finished) return;
      finished = true;
      cleanupTimers();
      if (code === 0) {
        resolve(stdout.trim());
      } else {
        reject(new Error(stderr || `bc command failed with code ${String(code ?? 'unknown')}`));
      }
    });

    proc.on('error', (err: Error) => {
      if (finished) return;
      finished = true;
      cleanupTimers();
      reject(new Error(`Failed to spawn mycel: ${err.message}`));
    });
  });
}

/**
 * Execute bc command and parse JSON response
 * @param args - Command arguments
 * @returns Parsed JSON response
 */
export async function execBcJson<T>(args: string[]): Promise<T> {
  const output = await execBc(args);
  try {
    return JSON.parse(output) as T;
  } catch {
    throw new Error(`Failed to parse bc output as JSON: ${output.slice(0, 100)}`);
  }
}

/**
 * Execute bc command with caching (stale-while-revalidate pattern)
 * #1005: Reduces subprocess overhead for polling operations
 *
 * @param args - Command arguments
 * @param ttl - Optional TTL override (uses default based on command type)
 * @returns Cached or fresh JSON response
 */
export async function execBcJsonCached<T>(args: string[], ttl?: number): Promise<T> {
  const key = getCacheKey(args);

  // Check cache first - use hit flag to handle cached null/undefined (#1612)
  const cached = getCachedResult<T>(key);
  if (cached.hit) {
    return cached.data as T;
  }

  // Cache miss - fetch fresh data
  const data = await execBcJson<T>(args);
  const effectiveTtl = ttl ?? getTtlForCommand(args);
  setCachedResult(key, data, effectiveTtl);

  return data;
}

// ============================================================================
// bcd HTTP API helpers
// ============================================================================

/**
 * Get the bcd daemon base URL.
 * Reads MYCEL_DAEMON_ADDR env var or defaults to http://127.0.0.1:9374.
 */
export function getBcdUrl(): string {
  const addr = process.env.MYCEL_DAEMON_ADDR;
  if (addr) {
    // Normalise: strip trailing slash
    return addr.replace(/\/$/, '');
  }
  return 'http://127.0.0.1:9374';
}

/**
 * Injectable fetch function — defaults to global fetch.
 * Tests can override via _setFetchForTesting() to control HTTP responses.
 */
type FetchFn = typeof fetch;
let _fetch: FetchFn = fetch;

/**
 * Set a custom fetch function for testing.
 * @param mockFetch - Mock fetch function to use
 * @returns Function to restore original fetch
 * @internal
 */
export function _setFetchForTesting(mockFetch: FetchFn): () => void {
  const originalFetch = _fetch;
  _fetch = mockFetch;
  return () => {
    _fetch = originalFetch;
  };
}

// ============================================================================
// Convenience methods for common commands
// ============================================================================

/**
 * Get current agent status
 */
export async function getStatus(): Promise<StatusResponse> {
  // #1005: Use cached version to reduce polling overhead
  return execBcJsonCached<StatusResponse>(['status']);
}

/**
 * Get list of channels via bcd HTTP API, falling back to CLI on failure.
 *
 * The bcd endpoint GET /api/channels returns an array of channel objects.
 * We normalise the response to the {channels: [...]} shape expected by the TUI.
 */
export async function getChannels(): Promise<ChannelsResponse> {
  const url = `${getBcdUrl()}/api/channels`;
  const res = await _fetch(url).catch((err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to connect to bcd at ${getBcdUrl()}: ${msg}. Is bcd running?`);
  });
  if (!res.ok) {
    throw new Error(`bcd returned HTTP ${String(res.status)} for GET /api/channels`);
  }
  const raw = (await res.json()) as {
    name: string;
    description?: string;
    members?: string[];
  }[];
  return { channels: raw.map((ch) => ({
    name: ch.name,
    description: ch.description,
    members: ch.members ?? [],
  })) };
}

/**
 * Get channel message history via bcd HTTP API, falling back to CLI on failure.
 *
 * The bcd endpoint GET /api/channels/{name}/history?limit=N returns an array
 * of message objects.  We normalise to the {channel, messages} shape the TUI expects.
 *
 * @param channelName - Name of channel
 * @param limit - Maximum number of messages to return (default: 50)
 */
export async function getChannelHistory(
  channelName: string,
  limit?: number
): Promise<ChannelHistory> {
  const limitParam = limit !== undefined && limit > 0 ? limit : 50;
  const url = `${getBcdUrl()}/api/channels/${encodeURIComponent(channelName)}/history?limit=${String(limitParam)}`;
  const res = await _fetch(url).catch((err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to connect to bcd at ${getBcdUrl()}: ${msg}. Is bcd running?`);
  });
  if (!res.ok) {
    throw new Error(`bcd returned HTTP ${String(res.status)} for GET /api/channels/${channelName}/history`);
  }
  const raw = (await res.json()) as {
    sender: string;
    content: string;
    created_at: string;
  }[];
  return {
    channel: channelName,
    messages: raw.map((m) => ({
      sender: m.sender,
      message: m.content,
      time: m.created_at,
    })),
  };
}

/**
 * Send message to channel
 * @param channelName - Name of channel
 * @param message - Message to send
 */
export async function sendChannelMessage(channelName: string, message: string): Promise<void> {
  await execBc(['channel', 'send', channelName, message]);
  // #1595: Granular cache invalidation - only invalidate this channel's history
  invalidateCacheKey(`channel:history:${channelName}`);
}

// ============================================================================
// Gateway & Notify API methods (channels revamp)
// ============================================================================

export interface GatewayInfo {
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

export interface NotifySubscription {
  id: number;
  channel: string;
  agent: string;
  mention_only: boolean;
  created_at: string;
}

export async function getGateways(): Promise<GatewayInfo[]> {
  const url = `${getBcdUrl()}/api/gateways`;
  const res = await _fetch(url).catch((err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to fetch gateways: ${msg}`);
  });
  if (!res.ok) throw new Error(`HTTP ${String(res.status)} fetching gateways`);
  return (await res.json()) as GatewayInfo[];
}

export async function getGatewayHealth(platform: string): Promise<GatewayHealth> {
  const url = `${getBcdUrl()}/api/gateways/${encodeURIComponent(platform)}/health`;
  const res = await _fetch(url).catch((err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to fetch gateway health: ${msg}`);
  });
  if (!res.ok) throw new Error(`HTTP ${String(res.status)} fetching gateway health`);
  return (await res.json()) as GatewayHealth;
}

export async function getChannelSubscriptions(channel: string): Promise<NotifySubscription[]> {
  const parts = channel.split(':');
  if (parts.length === 2) {
    const url = `${getBcdUrl()}/api/gateways/${parts[0]}/channels/${parts[1]}/agents`;
    const res = await _fetch(url).catch(() => null);
    if (res?.ok) return (await res.json()) as NotifySubscription[];
  }
  const url = `${getBcdUrl()}/api/notify/subscriptions/${encodeURIComponent(channel)}`;
  const res = await _fetch(url).catch((err: unknown) => {
    throw new Error(`Failed to fetch subscriptions: ${err instanceof Error ? err.message : String(err)}`);
  });
  if (!res.ok) throw new Error(`HTTP ${String(res.status)} fetching subscriptions`);
  return (await res.json()) as NotifySubscription[];
}

export async function subscribeAgent(channel: string, agent: string): Promise<void> {
  const parts = channel.split(':');
  if (parts.length === 2) {
    const url = `${getBcdUrl()}/api/gateways/${parts[0]}/channels/${parts[1]}/agents`;
    const res = await _fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agent }) });
    if (!res.ok) throw new Error(`HTTP ${String(res.status)} subscribing agent`);
    return;
  }
  throw new Error('Invalid channel format — expected platform:channel');
}

export async function unsubscribeAgent(channel: string, agent: string): Promise<void> {
  const parts = channel.split(':');
  if (parts.length === 2) {
    const url = `${getBcdUrl()}/api/gateways/${parts[0]}/channels/${parts[1]}/agents/${encodeURIComponent(agent)}`;
    const res = await _fetch(url, { method: 'DELETE' });
    if (!res.ok) throw new Error(`HTTP ${String(res.status)} unsubscribing agent`);
    return;
  }
  throw new Error('Invalid channel format — expected platform:channel');
}

export async function patchGateway(platform: string, config: Record<string, unknown>): Promise<void> {
  const url = `${getBcdUrl()}/api/gateways/${encodeURIComponent(platform)}`;
  const res = await _fetch(url, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`HTTP ${String(res.status)} patching gateway: ${text}`);
  }
}

/**
 * Get cost summary
 * Note: bc cost show returns text when empty, handle gracefully
 */
export async function getCostSummary(): Promise<CostSummary> {
  try {
    // #1005: Use cached version to reduce polling overhead
    return await execBcJsonCached<CostSummary>(['cost', 'show']);
  } catch {
    // Return empty cost summary when no records exist
    return {
      total_cost: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      by_agent: {},
      by_model: {},
      cache_hit_rate: 0,
      burn_rate: 0,
      projected_total: 0,
      billing_window_spent: 0,
    };
  }
}

/**
 * Get cost usage data from ccusage integration (#1882)
 * @param period - 'daily' | 'monthly' | 'session'
 */
export async function getCostUsage(
  period: 'daily' | 'monthly' | 'session' = 'daily'
): Promise<CostUsageDailyResponse | CostUsageMonthlyResponse | CostUsageSessionResponse> {
  return await execBcJsonCached(['cost', 'usage', '--period', period], 60000);
}

/**
 * Report agent state
 * @param state - New state (working, done, stuck, idle, error)
 * @param message - Status message
 */
export async function reportState(state: string, message: string): Promise<void> {
  await execBc(['report', state, message]);
  // #1005: Invalidate status cache after state change
  invalidateCache('status');
}

/**
 * Get list of demons (scheduled tasks)
 */
export async function getDemons(): Promise<Demon[]> {
  try {
    return await execBcJson<Demon[]>(['demon', 'list']);
  } catch {
    // If no demons exist, bc returns text not JSON
    return [];
  }
}

/**
 * Get demon details
 * @param name - Demon name
 */
export async function getDemon(name: string): Promise<Demon | null> {
  try {
    return await execBcJson<Demon>(['demon', 'show', name]);
  } catch {
    return null;
  }
}

/**
 * Get demon run logs
 * @param name - Demon name
 * @param tail - Number of recent entries (optional)
 */
export async function getDemonLogs(name: string, tail?: number): Promise<DemonRunLog[]> {
  try {
    const args = ['demon', 'logs', name];
    if (tail) {
      args.push('--tail', String(tail));
    }
    return await execBcJson<DemonRunLog[]>(args);
  } catch {
    return [];
  }
}

/**
 * Enable a demon
 * @param name - Demon name
 */
export async function enableDemon(name: string): Promise<void> {
  await execBc(['demon', 'enable', name]);
}

/**
 * Disable a demon
 * @param name - Demon name
 */
export async function disableDemon(name: string): Promise<void> {
  await execBc(['demon', 'disable', name]);
}

/**
 * Manually run a demon
 * @param name - Demon name
 */
export async function runDemon(name: string): Promise<void> {
  await execBc(['demon', 'run', name]);
}

/**
 * Get list of managed processes
 * Note: bc process list returns text when empty, handle gracefully
 */
export async function getProcesses(): Promise<ProcessListResponse> {
  try {
    // #1005: Use cached version to reduce polling overhead
    return await execBcJsonCached<ProcessListResponse>(['process', 'list']);
  } catch {
    return { processes: [] };
  }
}

/**
 * Get logs for a specific process
 * @param name - Process name
 * @param lines - Number of lines to return (optional)
 */
export async function getProcessLogs(name: string, lines?: number): Promise<string[]> {
  const args = ['process', 'logs', name];
  if (lines) {
    args.push('--lines', String(lines));
  }
  const response = await execBcJson<{ name: string; lines: string[] }>(args);
  return response.lines;
}


/**
 * Get event logs
 * @param tail - Number of recent entries (optional, default 50)
 * @param agent - Filter by agent name (optional)
 * @param eventType - Filter by event type (optional)
 */
export async function getLogs(
  tail?: number,
  agent?: string,
  eventType?: string
): Promise<LogEntry[]> {
  try {
    const args = ['logs'];
    if (tail) {
      args.push('--tail', String(tail));
    }
    if (agent) {
      args.push('--agent', agent);
    }
    if (eventType) {
      args.push('--type', eventType);
    }
    return await execBcJson<LogEntry[]>(args);
  } catch {
    return [];
  }
}

/**
 * Get list of worktrees
 * @param orphanedOnly - Only show orphaned worktrees
 */
export async function getWorktrees(orphanedOnly = false): Promise<Worktree[]> {
  try {
    const args = ['worktree', 'list'];
    if (orphanedOnly) {
      args.push('--orphaned');
    }
    return await execBcJson<Worktree[]>(args);
  } catch {
    return [];
  }
}

/**
 * Prune orphaned worktrees
 * @param force - Actually remove (vs dry run)
 */
export async function pruneWorktrees(force = false): Promise<string> {
  const args = ['worktree', 'prune'];
  if (force) {
    args.push('--force');
  }
  return execBc(args);
}

/**
 * Attach to an agent's session via the bc CLI.
 * Routes through the Go backend which uses the configured runtime (tmux or docker).
 * @param agentName - Agent name to attach to
 * @throws Error if session doesn't exist or attachment fails
 */
export function attachToAgentSession(agentName: string): void {
  const bcBin = process.env.MYCEL_BIN ?? 'mycel';
  spawnSync(bcBin, ['agent', 'attach', agentName], {
    stdio: 'inherit',
  });
  // Exit after attachment ends
  process.exit(0);
}

/**
 * Get list of roles
 */
export async function getRoles(): Promise<RolesResponse> {
  try {
    // #1005: Use cached version to reduce polling overhead
    return await execBcJsonCached<RolesResponse>(['role', 'list']);
  } catch {
    return { roles: [] };
  }
}

/**
 * Get role details
 * @param name - Role name
 */
export async function getRole(name: string): Promise<Role | null> {
  try {
    return await execBcJson<Role>(['role', 'show', name]);
  } catch {
    return null;
  }
}

/**
 * Delete a role
 * @param name - Role name
 */
export async function deleteRole(name: string): Promise<void> {
  await execBc(['role', 'delete', name]);
}

/**
 * Validate all role files
 */
export async function validateRoles(): Promise<string> {
  return await execBc(['role', 'validate']);
}

// ============================================================================
// Tool Commands (#1866 - Tools View)
// ============================================================================

/**
 * Get list of installed tools/providers
 * #1866: Returns array of tool info objects
 */
export async function getToolList(): Promise<ToolInfo[]> {
  try {
    return await execBcJsonCached<ToolInfo[]>(['tool', 'list'], 30000);
  } catch {
    return [];
  }
}

// ============================================================================
// MCP Server Commands (#1927 - MCP View)
// ============================================================================

export interface MCPServer {
  name: string;
  transport: string;
  command?: string;
  url?: string;
  args?: string[];
  env?: Record<string, string>;
  enabled: boolean;
}

/**
 * Get MCP server list
 */
export async function getMCPList(): Promise<MCPServer[]> {
  try {
    const result = await execBcJsonCached<{ servers?: MCPServer[] }>(['mcp', 'list'], 30000);
    return result.servers ?? [];
  } catch {
    return [];
  }
}

// ============================================================================
// Secret Commands (#1927 - Secrets View)
// ============================================================================

export interface SecretMeta {
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
}

/**
 * Get secret list (metadata only, no values)
 */
export async function getSecretList(): Promise<SecretMeta[]> {
  try {
    const result = await execBcJsonCached<{ secrets?: SecretMeta[] }>(['secret', 'list'], 30000);
    return result.secrets ?? [];
  } catch {
    return [];
  }
}

// ============================================================================
// Process Commands (#1927 - Processes View)
// ============================================================================

export interface ProcessInfo {
  name: string;
  command: string;
  status: string;
  pid?: number;
  started_at?: string;
}

/**
 * Get process list
 */
export async function getProcessList(): Promise<ProcessInfo[]> {
  try {
    const result = await execBcJsonCached<{ processes?: ProcessInfo[] }>(
      ['process', 'list'],
      30000
    );
    return result.processes ?? [];
  } catch {
    return [];
  }
}

// ============================================================================
// GitHub Issue Commands (#1754 - Issues View)
// ============================================================================

/**
 * GitHub issue type from gh CLI JSON output
 */
export interface GHIssue {
  number: number;
  title: string;
  body?: string;
  state: string;
  labels: { name: string }[];
  assignees: { login: string }[];
  author?: { login: string };
  createdAt: string;
  updatedAt?: string;
  comments?: { author: { login: string }; body: string; createdAt: string }[];
}

/**
 * List GitHub issues
 * @param labels - Optional label filter (comma-separated)
 * @param assignee - Optional assignee filter
 * @param state - Issue state filter (open, closed, all)
 */
export async function getIssues(
  labels?: string,
  assignee?: string,
  state: 'open' | 'closed' | 'all' = 'open'
): Promise<GHIssue[]> {
  try {
    const args = ['issue', 'list', '--state', state];
    if (labels) {
      args.push('--labels', labels);
    }
    if (assignee) {
      args.push('--assignee', assignee);
    }
    // bc issue list already adds --json flag internally
    return await execBcJson<GHIssue[]>(args);
  } catch {
    return [];
  }
}

/**
 * Get details for a specific issue
 * @param issueNumber - Issue number
 * @param includeComments - Whether to include comments
 */
export async function getIssue(
  issueNumber: number,
  includeComments = true
): Promise<GHIssue | null> {
  try {
    const args = ['issue', 'view', String(issueNumber)];
    if (includeComments) {
      args.push('--comments');
    }
    return await execBcJson<GHIssue>(args);
  } catch {
    return null;
  }
}

/**
 * Close a GitHub issue
 * @param issueNumber - Issue number
 * @param reason - Close reason (completed, not_planned)
 * @param comment - Optional closing comment
 */
export async function closeIssue(
  issueNumber: number,
  reason: 'completed' | 'not_planned' = 'completed',
  comment?: string
): Promise<void> {
  const args = ['issue', 'close', String(issueNumber), '--reason', reason];
  if (comment) {
    args.push('--comment', comment);
  }
  await execBc(args);
}

/**
 * Assign a GitHub issue
 * @param issueNumber - Issue number
 * @param assignee - User to assign (or --unassign to remove)
 */
export async function assignIssue(issueNumber: number, assignee: string): Promise<void> {
  await execBc(['issue', 'assign', String(issueNumber), assignee]);
}
