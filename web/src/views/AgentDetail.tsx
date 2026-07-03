import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link, useLocation, useNavigate } from "react-router-dom";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { api } from "../api/client";
import type { Agent, AgentConfig } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatsTab as StatsTabComponent } from "../components/StatsTab";
import { WebTerminal, type TerminalConnectionState, type TerminalConnectionDetail } from "../components/WebTerminal";
import { AgentIcon } from "../components/agent-ui";
import { LoopIconButton, RalphLoopModal } from "../components/RalphLoopModal";
import { MCPServerList } from "../components/shared/MCPServerList";
import { McpEnvEditor } from "../components/shared/McpEnvEditor";
import { SystemPromptEditor } from "../components/shared/SystemPromptEditor";
import { SectionRule } from "../components/shared";
import { AgentToolStream } from "../components/live/AgentToolStream";
import { CreateAgentModal } from "../components/CreateAgentModal";
import { formatAbsolute, formatRelative as sharedFormatRelative } from "../utils/time";
import { MONO } from "../utils/typography";

const formatTime = (t?: string): string => formatAbsolute(t);
const formatRelative = (t?: string): string => sharedFormatRelative(t, { emptyLabel: "" });

/* ═══════════════════════════════════════════════════════════════════
   Tab types — v3: Live / Attach / Config / Metrics
   ═══════════════════════════════════════════════════════════════════ */

type Tab = "attach" | "live" | "config" | "metrics" | "code";

const TABS: { key: Tab; label: string; shortcut: string }[] = [
  { key: "attach", label: "Attach", shortcut: "1" },
  { key: "live", label: "Live", shortcut: "2" },
  { key: "config", label: "Config", shortcut: "3" },
  { key: "metrics", label: "Metrics", shortcut: "4" },
  { key: "code", label: "Code", shortcut: "5" },
];

/* ═══════════════════════════════════════════════════════════════════
   Section chrome
   ═══════════════════════════════════════════════════════════════════ */

function MetaCell({
  label,
  children,
  mono,
}: {
  label: string;
  children: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="space-y-1">
      <dt
        className="text-[9px] font-bold uppercase tracking-[0.2em] text-mycel-muted/60"
        style={{ fontFamily: MONO }}
      >
        {label}
      </dt>
      <dd
        className="text-[13px] text-mycel-text/90 leading-tight break-all"
        style={mono ? { fontFamily: MONO } : undefined}
      >
        {children}
      </dd>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 2 — Attach
   Direct terminal access with a status overlay that explains
   connection state: connecting / stopped / error, plus a Start/Retry
   affordance. The overlay sits on top of the xterm surface so the
   terminal instance (and its scrollback) is preserved across retries.
   ═══════════════════════════════════════════════════════════════════ */

type AttachOverlayKind =
  | "hidden"
  | "connecting"
  | "stopped"
  | "error";

function AttachTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";

  // WS lifecycle reported by WebTerminal. Seeded to "connecting" when
  // we are about to (re)open the socket.
  const [wsState, setWsState] = useState<TerminalConnectionState>("connecting");
  const [wsDetail, setWsDetail] = useState<TerminalConnectionDetail | undefined>(undefined);
  // Bumping this counter asks WebTerminal to close the current socket
  // and open a fresh one — without rebuilding the xterm instance.
  const [reconnectToken, setReconnectToken] = useState(0);
  // Tracks an in-flight POST /start so the overlay can show progress.
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);

  const onStateChange = useCallback(
    (state: TerminalConnectionState, detail?: TerminalConnectionDetail) => {
      setWsState(state);
      setWsDetail(detail);
    },
    [],
  );

  const handleRetry = useCallback(() => {
    setWsState("connecting");
    setWsDetail(undefined);
    setReconnectToken((t) => t + 1);
  }, []);

  const handleStart = useCallback(async () => {
    setStarting(true);
    setStartError(null);
    try {
      await api.startAgent(agent.name);
      // Parent AgentDetail polls and will re-render once state flips to
      // "running"; the WS will then connect on its own.
      setWsState("connecting");
    } catch (err) {
      setStartError(err instanceof Error ? err.message : "Failed to start agent");
    } finally {
      setStarting(false);
    }
  }, [agent.name]);

  // Compute which overlay variant (if any) to show.
  const overlay: AttachOverlayKind = (() => {
    if (isStopped) return "stopped";
    if (wsState === "open") return "hidden";
    if (wsState === "connecting") return "connecting";
    // closed or error
    return "error";
  })();

  return (
    <div className="flex-1 min-h-0 relative" title="Click anywhere to focus the terminal">
      {/* When the agent is stopped, we don't mount the terminal at all
          — opening the WS would just 404. Start the agent to connect. */}
      {!isStopped && (
        <WebTerminal
          agentName={agent.name}
          reconnectToken={reconnectToken}
          onConnectionStateChange={onStateChange}
        />
      )}
      {overlay !== "hidden" && (
        <AttachOverlay
          kind={overlay}
          agent={agent}
          detail={wsDetail}
          starting={starting}
          startError={startError}
          onStart={handleStart}
          onRetry={handleRetry}
        />
      )}
    </div>
  );
}

interface AttachOverlayProps {
  kind: Exclude<AttachOverlayKind, "hidden">;
  agent: Agent;
  detail?: TerminalConnectionDetail;
  starting: boolean;
  startError: string | null;
  onStart: () => void;
  onRetry: () => void;
}

function AttachOverlay({
  kind,
  agent,
  detail,
  starting,
  startError,
  onStart,
  onRetry,
}: AttachOverlayProps) {
  // Routes are flat (/agents/<name>/<tab>) — link straight to the
  // sibling "live" tab instead of parsing the current path.
  const livePath = `/agents/${agent.name}/live`;
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="attach-overlay"
      data-state={kind}
      className="absolute inset-0 z-10 flex items-center justify-center bg-mycel-bg/70 backdrop-blur-sm"
    >
      <div
        className="w-full max-w-[360px] rounded-lg border border-mycel-border bg-mycel-surface px-5 py-4 shadow-lg"
        style={{ fontFamily: MONO }}
      >
        {kind === "connecting" && (
          <div className="flex items-center gap-3">
            <span
              aria-hidden="true"
              data-testid="attach-overlay-spinner"
              className="inline-block h-3 w-3 rounded-full border-2 border-mycel-accent/60 border-t-transparent animate-spin"
            />
            <span className="text-[12px] text-mycel-text/90">
              Connecting to {agent.name}
              <span className="text-mycel-muted">…</span>
            </span>
          </div>
        )}

        {kind === "stopped" && (
          <div className="flex flex-col gap-3">
            <div>
              <p className="text-[12px] font-semibold text-mycel-text">Agent is stopped</p>
              <p className="mt-1 text-[11px] leading-relaxed text-mycel-muted">
                Start the agent to attach a live terminal.
              </p>
              {startError && (
                <p className="mt-2 text-[11px] text-mycel-error/90 break-words">{startError}</p>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={onStart}
                disabled={starting}
                className="px-3 py-1.5 rounded-md border border-mycel-accent/50 bg-mycel-accent/10 text-[11px] font-semibold text-mycel-accent hover:bg-mycel-accent/20 transition-colors disabled:opacity-40"
              >
                {starting ? "Starting…" : "Start agent"}
              </button>
            </div>
          </div>
        )}

        {kind === "error" && (
          <div className="flex flex-col gap-3">
            <div>
              <p className="text-[12px] font-semibold text-mycel-text">Connection lost</p>
              <p className="mt-1 text-[11px] leading-relaxed text-mycel-muted">
                The terminal stream dropped.
                {detail?.code ? ` (code ${String(detail.code)})` : ""}
              </p>
              {detail?.reason && (
                <p className="mt-1 text-[11px] text-mycel-muted/70 break-words">{detail.reason}</p>
              )}
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={onRetry}
                className="px-3 py-1.5 rounded-md border border-mycel-accent/50 bg-mycel-accent/10 text-[11px] font-semibold text-mycel-accent hover:bg-mycel-accent/20 transition-colors"
              >
                Retry
              </button>
              <Link
                to={livePath}
                className="text-[11px] text-mycel-accent/80 hover:text-mycel-accent hover:underline"
              >
                View logs
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 3 — Config
   System prompt, MCP servers, metadata, danger zone
   ═══════════════════════════════════════════════════════════════════ */

function ConfigTab({ agent, agentsUrl }: { agent: Agent; agentsUrl: string }) {
  const navigate = useNavigate();
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Clone + Archive state
  const [cloneOpen, setCloneOpen] = useState(false);
  const [allAgents, setAllAgents] = useState<Agent[]>([]);
  const [confirmArchive, setConfirmArchive] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [archiveError, setArchiveError] = useState<string | null>(null);
  const isArchived = Boolean(agent.archived_at);

  // MCP server management state
  const [mcpList, setMcpList] = useState<string[] | null>(null);
  const [mcpLoading, setMcpLoading] = useState(true);

  // Env vars state — persisted via API to .bc/agents/<name>/env.json
  const [envVars, setEnvVars] = useState<Array<{ key: string; value: string }>>([]);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const [envSaved, setEnvSaved] = useState(false);
  const envSavedTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Template sync state
  const [templates, setTemplates] = useState<string[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [syncDone, setSyncDone] = useState(false);
  const syncDoneTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [hostname, setHostname] = useState("localhost");

  const fetchMcps = useCallback(() => {
    setMcpLoading(true);
    api
      .getAgentMcps(agent.name)
      .then((data) => {
        setMcpList(data.map((m) => m.name));
      })
      .catch(() => {
        setMcpList(null); // fall back to static chips
      })
      .finally(() => {
        setMcpLoading(false);
      });
  }, [agent.name]);

  useEffect(() => {
    let cancelled = false;
    setConfigLoading(true);
    api
      .getAgentConfig(agent.name)
      .then((data) => {
        if (!cancelled) {
          setConfig(data);
        }
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        if (!cancelled) setConfigLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  useEffect(() => {
    fetchMcps();
  }, [fetchMcps]);

  // Load templates list on mount.
  useEffect(() => {
    let cancelled = false;
    fetch("/api/templates")
      .then((r) => (r.ok ? (r.json() as Promise<Array<{ name: string }>>) : Promise.resolve([])))
      .then((data) => {
        if (!cancelled) {
          const names = data.map((t) => t.name).filter(Boolean) as string[];
          setTemplates(names);
          if (names.length > 0) {
            const nonBlank = names.filter((t) => t !== "blank");
            setSelectedTemplate(nonBlank[0] ?? names[0] ?? "feature-dev");
          }
        }
      })
      .catch(() => {/* best-effort */});
    return () => { cancelled = true; };
  }, []);

  // Fetch hostname from system info endpoint (uses shared API client for workspace header).
  useEffect(() => {
    api.getSystemInfo()
      .then((data) => { if (data?.hostname) setHostname(data.hostname); })
      .catch(() => {/* best-effort */});
  }, []);

  // Load persisted env vars from API on mount.
  useEffect(() => {
    let cancelled = false;
    api
      .getAgentEnv(agent.name)
      .then((data) => {
        if (!cancelled) setEnvVars(data);
      })
      .catch(() => {
        /* best-effort — fall back to empty */
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  // Persist env vars to API and show "Saved" indicator.
  const saveEnvVars = useCallback((vars: Array<{ key: string; value: string }>) => {
    api
      .putAgentEnv(agent.name, vars)
      .then(() => {
        setEnvSaved(true);
        if (envSavedTimer.current) clearTimeout(envSavedTimer.current);
        envSavedTimer.current = setTimeout(() => setEnvSaved(false), 2000);
      })
      .catch(() => {
        /* best-effort */
      });
  }, [agent.name]);

  const handleSync = useCallback(async () => {
    if (!selectedTemplate) return;
    setSyncing(true);
    try {
      const res = await fetch(`/api/templates/${encodeURIComponent(selectedTemplate)}`);
      if (!res.ok) throw new Error(`Failed to fetch template: ${res.status}`);
      const tmpl = await res.json() as { system_prompt?: string };
      if (tmpl.system_prompt !== undefined) {
        await api.patchAgentConfig(agent.name, { system_prompt: tmpl.system_prompt });
        const updated = await api.getAgentConfig(agent.name);
        setConfig(updated);
      }
      setSyncDone(true);
      if (syncDoneTimer.current) clearTimeout(syncDoneTimer.current);
      syncDoneTimer.current = setTimeout(() => setSyncDone(false), 2000);
    } catch {
      /* best-effort */
    } finally {
      setSyncing(false);
    }
  }, [agent.name, selectedTemplate]);

  // If live fetch succeeded use that list; otherwise fall back to static record
  const mcpServers: string[] =
    mcpList !== null
      ? mcpList
      : config?.mcp_servers && config.mcp_servers.length > 0
        ? config.mcp_servers
        : (agent.mcp_servers ?? []);

  const useLiveMcp = mcpList !== null;

  const handleSystemPromptSave = async (newValue: string) => {
    await api.patchAgentConfig(agent.name, { system_prompt: newValue });
    // Refresh config after save
    const updated = await api.getAgentConfig(agent.name);
    setConfig(updated);
  };

  const handleMcpAdd = async (mcpName: string) => {
    await api.addAgentMcp(agent.name, mcpName);
    fetchMcps();
  };

  const handleMcpRemove = async (mcpName: string) => {
    await api.removeAgentMcp(agent.name, mcpName);
    fetchMcps();
  };

  const isDocker = (agent.runtime_backend ?? config?.runtime_backend ?? "") === "docker";
  const isTmux = !isDocker;

  return (
    <div className="flex-1 min-h-0 overflow-y-auto p-6">
      <div className="max-w-3xl mx-auto space-y-10">

        {/* ── RUNTIME BANNER ── */}
        <div
          className={`flex items-center gap-2.5 rounded-md px-4 py-2.5 border text-[11px] ${
            isDocker
              ? "border-blue-500/20 bg-blue-500/[0.04] text-blue-400/80"
              : "border-mycel-accent/20 bg-mycel-accent/[0.03] text-mycel-accent/70"
          }`}
          style={{ fontFamily: MONO }}
        >
          {isDocker ? (
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <rect x="1" y="4" width="12" height="8" rx="1" />
              <path d="M4 4V2h6v2" />
              <path d="M5 7h4M5 9.5h4" opacity="0.6" />
            </svg>
          ) : (
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <rect x="1.5" y="1.5" width="11" height="8" rx="1.5" />
              <path d="M7 9.5v2.5M4 12h6" />
            </svg>
          )}
          <span className="font-semibold">
            {isDocker ? "Docker container" : `tmux (${hostname})`}
          </span>
          <span className="text-current/50">·</span>
          {isDocker ? (
            <span className="text-current/60">
              {agent.session ?? config?.session ?? "isolated container"}
            </span>
          ) : (
            <span className="text-current/60">
              session: {agent.session ?? config?.session ?? agent.name}
            </span>
          )}
        </div>

        {/* ── SYSTEM PROMPT ── */}
        <SystemPromptEditor
          value={config?.system_prompt ?? ""}
          loading={configLoading}
          onSave={handleSystemPromptSave}
        />

        {/* ── TEMPLATE ── */}
        {templates.length > 0 && (
          <section>
            <div className="flex items-center justify-between mb-1">
              <SectionRule>Template</SectionRule>
              {syncDone && (
                <span className="text-[10px] text-green-500/70 transition-opacity" style={{ fontFamily: MONO }}>
                  Synced
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              <select
                value={selectedTemplate}
                onChange={(e) => setSelectedTemplate(e.target.value)}
                className="flex-1 rounded border border-mycel-border/40 bg-mycel-bg px-2.5 py-1.5 text-[11px] text-mycel-text/90 outline-none focus:border-mycel-accent/50 transition-colors"
                style={{ fontFamily: MONO }}
              >
                {templates.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
              <button
                type="button"
                disabled={syncing || !selectedTemplate}
                onClick={() => { void handleSync(); }}
                className="px-3 py-1.5 rounded border border-mycel-accent/30 bg-mycel-accent/10 text-[11px] text-mycel-accent hover:bg-mycel-accent/20 transition-colors disabled:opacity-40"
                style={{ fontFamily: MONO }}
              >
                {syncing ? "Syncing…" : "Sync"}
              </button>
            </div>
            <p className="text-[10px] text-mycel-muted mt-1 leading-relaxed" style={{ fontFamily: MONO }}>
              Re-apply template system prompt and MCP configuration
            </p>
          </section>
        )}

        {/* ── MCP SERVERS ── */}
        <section>
          <SectionRule>MCP Servers</SectionRule>
          <MCPServerList
            servers={mcpServers}
            loading={mcpLoading}
            onAdd={useLiveMcp ? handleMcpAdd : undefined}
            onRemove={useLiveMcp ? handleMcpRemove : undefined}
          />
          <p className="mt-2 text-[10px] text-mycel-muted leading-relaxed" style={{ fontFamily: MONO }}>
            {isTmux
              ? "For tmux agents, MCPs are managed via the Claude CLI. Changes here write to the agent\u2019s worktree."
              : "Changes write to .mcp.json in the container."}
          </p>
        </section>

        {/* ── MCP ENVIRONMENT ── */}
        {mcpServers.length > 0 && (
          <section>
            <SectionRule>MCP Environment</SectionRule>
            <McpEnvEditor serverNames={mcpServers} />
          </section>
        )}

        {/* ── RUNTIME INFO ── */}
        <section>
          <SectionRule>Runtime</SectionRule>
          <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-4">
            <MetaCell label="Provider" mono>
              {agent.tool || "\u2014"}
            </MetaCell>
            <MetaCell label="Backend" mono>
              {isDocker ? "docker" : "tmux"}
            </MetaCell>
            <MetaCell label="Session" mono>
              {/* Session name is the agent name by construction; fall back
                  to it while the agent is running instead of rendering an
                  em-dash next to a live state badge. */}
              {agent.session
                || config?.session
                || (agent.state !== "stopped" && agent.state !== "error" ? agent.name : "")
                || "\u2014"}
            </MetaCell>
            <MetaCell label="Created">
              <span className="tabular-nums">{formatTime(agent.created_at)}</span>
            </MetaCell>
            <MetaCell label="Started">
              <span className="tabular-nums">{formatTime(agent.started_at)}</span>
            </MetaCell>
            {/* Hide a stale stopped_at from a previous run \u2014 rendering
                "Stopped" earlier than "Started" reads as a contradiction. */}
            {agent.stopped_at
              && (!agent.started_at
                || new Date(agent.stopped_at).getTime() >= new Date(agent.started_at).getTime()) && (
              <MetaCell label="Stopped">
                <span className="tabular-nums">{formatTime(agent.stopped_at)}</span>
              </MetaCell>
            )}
          </dl>
        </section>

        {/* ── ENVIRONMENT ── */}
        <section>
          <div className="flex items-center justify-between mb-1">
            <SectionRule>Environment</SectionRule>
            {envSaved && (
              <span className="text-[10px] text-green-500/70 transition-opacity" style={{ fontFamily: MONO }}>
                Saved
              </span>
            )}
          </div>

          {/* Placeholder hint when no env vars are set */}
          {envVars.length === 0 && (
            <p className="mb-3 text-[10px] text-mycel-muted italic" style={{ fontFamily: MONO }}>
              Common: ANTHROPIC_API_KEY, GITHUB_TOKEN, AWS_ACCESS_KEY_ID
            </p>
          )}

          {/* Existing env var rows */}
          {envVars.length > 0 && (
            <div className="mb-3 rounded-md border border-mycel-border/40 overflow-hidden divide-y divide-mycel-border/20">
              {envVars.map((ev, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 px-3 py-2 bg-mycel-surface/20 group"
                >
                  <span
                    className="flex-1 min-w-0 text-[11px] font-semibold text-mycel-text/80 truncate"
                    style={{ fontFamily: MONO }}
                  >
                    {ev.key}
                  </span>
                  <span className="text-mycel-muted/30 text-[11px]">=</span>
                  <span
                    className="flex-1 min-w-0 text-[11px] text-mycel-muted/70 truncate"
                    style={{ fontFamily: MONO }}
                  >
                    {ev.value}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      const next = envVars.filter((_, idx) => idx !== i);
                      setEnvVars(next);
                      saveEnvVars(next);
                    }}
                    className="shrink-0 text-[11px] text-mycel-muted/30 hover:text-mycel-error transition-colors opacity-0 group-hover:opacity-100"
                    aria-label={`Remove ${ev.key}`}
                    title={`Remove ${ev.key}`}
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Add row */}
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={newKey}
              onChange={(e) => setNewKey(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newKey.trim()) {
                  const next = [...envVars, { key: newKey.trim(), value: newValue }];
                  setEnvVars(next);
                  saveEnvVars(next);
                  setNewKey("");
                  setNewValue("");
                }
              }}
              placeholder="KEY"
              className="w-32 rounded border border-mycel-border/40 bg-mycel-bg px-2.5 py-1 text-[11px] text-mycel-text/90 placeholder:text-mycel-muted outline-none focus:border-mycel-accent/50 transition-colors"
              style={{ fontFamily: MONO }}
            />
            <span className="text-mycel-muted text-[11px]" style={{ fontFamily: MONO }}>=</span>
            <input
              type="text"
              value={newValue}
              onChange={(e) => setNewValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newKey.trim()) {
                  const next = [...envVars, { key: newKey.trim(), value: newValue }];
                  setEnvVars(next);
                  saveEnvVars(next);
                  setNewKey("");
                  setNewValue("");
                }
              }}
              placeholder="value"
              className="flex-1 max-w-[200px] rounded border border-mycel-border/40 bg-mycel-bg px-2.5 py-1 text-[11px] text-mycel-text/90 placeholder:text-mycel-muted outline-none focus:border-mycel-accent/50 transition-colors"
              style={{ fontFamily: MONO }}
            />
            <button
              type="button"
              disabled={!newKey.trim()}
              onClick={() => {
                if (!newKey.trim()) return;
                const next = [...envVars, { key: newKey.trim(), value: newValue }];
                setEnvVars(next);
                saveEnvVars(next);
                setNewKey("");
                setNewValue("");
              }}
              className="px-2.5 py-1 rounded border border-mycel-accent/30 bg-mycel-accent/10 text-[11px] text-mycel-accent hover:bg-mycel-accent/20 transition-colors disabled:opacity-40"
              style={{ fontFamily: MONO }}
            >
              + Add
            </button>
          </div>

          <p className="mt-2 text-[10px] text-mycel-muted leading-relaxed" style={{ fontFamily: MONO }}>
            {isTmux
              ? "Set via provider CLI environment · Env vars are applied on agent restart"
              : "Injected as container environment variables · Env vars are applied on agent restart"}
          </p>
          {agent.tool === "claude" && (
            <div className="mt-2 text-[10px] text-mycel-muted" style={{ fontFamily: MONO }}>
              <span className="font-medium">Claude requires:</span> ANTHROPIC_API_KEY
            </div>
          )}
          {agent.tool === "gemini" && (
            <div className="mt-2 text-[10px] text-mycel-muted" style={{ fontFamily: MONO }}>
              <span className="font-medium">Gemini requires:</span> GOOGLE_API_KEY
            </div>
          )}
          {agent.tool === "openai" && (
            <div className="mt-2 text-[10px] text-mycel-muted" style={{ fontFamily: MONO }}>
              <span className="font-medium">OpenAI requires:</span> OPENAI_API_KEY
            </div>
          )}
        </section>

        {/* ── ACTIONS ── */}
        <section>
          <SectionRule>Actions</SectionRule>
          <div className="rounded-md border border-mycel-border/40 bg-mycel-surface/20 px-4 py-3">
            <div className="flex flex-wrap gap-2 items-center">
              {/* Clone — opens CreateAgentModal pre-seeded with this agent */}
              <button
                type="button"
                onClick={() => {
                  api
                    .listAgents()
                    .then((list) => setAllAgents(list))
                    .catch(() => setAllAgents([agent]))
                    .finally(() => setCloneOpen(true));
                }}
                className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-border/40 text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors"
                style={{ fontFamily: MONO }}
              >
                Clone
              </button>

              {/* Archive / Unarchive with confirm flow mirroring Delete */}
              {archiveError && (
                <span className="text-[11px] text-mycel-error" style={{ fontFamily: MONO }}>
                  {archiveError}
                </span>
              )}
              {isArchived ? (
                <button
                  type="button"
                  disabled={archiving}
                  onClick={() => {
                    setArchiving(true);
                    setArchiveError(null);
                    api
                      .unarchiveAgent(agent.name)
                      .then(() => navigate(agentsUrl))
                      .catch((err: unknown) => {
                        setArchiving(false);
                        setArchiveError(
                          err instanceof Error ? err.message : "Failed to unarchive agent",
                        );
                      });
                  }}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-accent/40 text-mycel-accent hover:bg-mycel-accent/10 transition-colors disabled:opacity-40"
                  style={{ fontFamily: MONO }}
                >
                  {archiving ? "Unarchiving…" : "Unarchive"}
                </button>
              ) : confirmArchive ? (
                <>
                  <span className="text-[11px] text-mycel-muted" style={{ fontFamily: MONO }}>
                    Archive this agent?
                  </span>
                  <button
                    type="button"
                    disabled={archiving}
                    onClick={() => {
                      setArchiving(true);
                      setArchiveError(null);
                      api
                        .archiveAgent(agent.name)
                        .then(() => navigate(agentsUrl))
                        .catch((err: unknown) => {
                          setArchiving(false);
                          setConfirmArchive(false);
                          setArchiveError(
                            err instanceof Error ? err.message : "Failed to archive agent",
                          );
                        });
                    }}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-accent/50 bg-mycel-accent/10 text-mycel-accent hover:bg-mycel-accent/20 transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    {archiving ? "Archiving…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    disabled={archiving}
                    onClick={() => setConfirmArchive(false)}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-border/40 text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmArchive(true)}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-border/40 text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors"
                  style={{ fontFamily: MONO }}
                >
                  Archive
                </button>
              )}
            </div>

            <CreateAgentModal
              open={cloneOpen}
              onClose={() => setCloneOpen(false)}
              existingNames={allAgents.map((a) => a.name)}
              existingAgents={allAgents}
              defaultCloneFrom={agent.name}
            />
          </div>
        </section>

        {/* ── DANGER ZONE ── */}
        <section>
          <SectionRule>Danger Zone</SectionRule>
          <div className="rounded-md border border-mycel-error/20 bg-mycel-error/[0.02] px-4 py-3">
            {deleteError && (
              <p className="mb-2 text-[11px] text-mycel-error" style={{ fontFamily: MONO }}>
                {deleteError}
              </p>
            )}
            <div className="flex flex-wrap gap-2 items-center">
              {/* Delete — with confirmation */}
              {confirmDelete ? (
                <>
                  <span
                    className="text-[11px] text-mycel-error/80"
                    style={{ fontFamily: MONO }}
                  >
                    Are you sure?
                  </span>
                  <button
                    type="button"
                    disabled={deleting}
                    onClick={() => {
                      setDeleting(true);
                      setDeleteError(null);
                      api
                        .deleteAgent(agent.name)
                        .then(() => {
                          navigate(agentsUrl);
                        })
                        .catch((err: unknown) => {
                          setDeleting(false);
                          setConfirmDelete(false);
                          setDeleteError(err instanceof Error ? err.message : "Failed to delete agent");
                        });
                    }}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-error/50 bg-mycel-error/10 text-mycel-error hover:bg-mycel-error/20 transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    {deleting ? "Deleting…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    disabled={deleting}
                    onClick={() => setConfirmDelete(false)}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-border/40 text-mycel-muted hover:text-mycel-text hover:border-mycel-border transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmDelete(true)}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-mycel-error/30 text-mycel-error/80 hover:bg-mycel-error/10 transition-colors"
                  style={{ fontFamily: MONO }}
                >
                  Delete
                </button>
              )}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 4 — Stats
   Wraps existing StatsTabComponent, adds stopped-agent context
   ═══════════════════════════════════════════════════════════════════ */

/* ═══════════════════════════════════════════════════════════════════
   Tab 5 — Code (placeholder)
   Shows file tree + Monaco editor (or code-server iframe when available).
   Full implementation in Phase 3 of the multi-workspace proposal.
   ═══════════════════════════════════════════════════════════════════ */

function CodeTabPlaceholder({ agent }: { agent: Agent }) {
  // Defers to the top-level Code view. Routes are flat — no workspace
  // prefix — so the target is simply /code with the worktree preselected.
  const target = `/code?worktree=${encodeURIComponent(agent.name)}&view=diff`;

  return (
    <div className="flex-1 flex items-center justify-center p-6">
      <div className="max-w-md text-center space-y-4" style={{ fontFamily: MONO }}>
        <p className="text-[11px] text-mycel-muted uppercase tracking-wider">
          Agent code — diff view
        </p>
        <p className="text-sm text-mycel-muted leading-relaxed">
          Open the Code view with <span className="text-mycel-text/90">{agent.name}</span>'s
          worktree selected to see its uncommitted changes against the main repo.
        </p>
        <Link
          to={target}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-md border border-mycel-accent/40 bg-mycel-accent/10 text-mycel-accent hover:bg-mycel-accent/20 transition-colors text-[12px] font-semibold"
        >
          Open in Code view
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6">
            <path d="M3 9l6-6M5 3h4v4" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </Link>
      </div>
    </div>
  );
}

function MetricsTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";
  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-4xl mx-auto space-y-4">
        {isStopped && (
          <p
            className="text-[10px] text-mycel-muted italic"
            style={{ fontFamily: MONO }}
          >
            Agent is not running. Stats show last known values.
          </p>
        )}
        <StatsTabComponent agent={agent} />
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Main component
   ═══════════════════════════════════════════════════════════════════ */

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const agentsUrl = "/agents";

  // AgentDetail renders its own comprehensive HUD bar (back link + agent
  // icon + name + state + task + tabs), so the global LayoutHeader is
  // hidden entirely on this view — no empty 42px row + border above our
  // own header. The sidebar still owns its own collapse arrow and the
  // workspace dropdown is reachable from the sidebar header, so nothing
  // navigation-critical is lost.
  useHeaderSlot({ hidden: true });

  // Derive active tab from URL sub-path: /agents/<name>/<tab>
  // Defaults to "attach" when no sub-path is present.
  const tabFromPath = useMemo<Tab>(() => {
    const segments = location.pathname.split("/");
    const last = segments[segments.length - 1];
    const candidates: Tab[] = ["attach", "live", "config", "metrics", "code"];
    return (candidates.includes(last as Tab) ? (last as Tab) : "attach");
  }, [location.pathname]);
  const [activeTab, setActiveTab] = useState<Tab>(tabFromPath);

  // Keep state in sync when URL changes (browser back/forward, deep link)
  useEffect(() => {
    if (tabFromPath !== activeTab) setActiveTab(tabFromPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabFromPath]);

  // Clicking a tab updates both state and URL. Routes are flat
  // (/agents/<name>/<tab>) — build the path absolutely instead of
  // appending to the current one (the old relative slice logic grew
  // /agents/x/config/metrics/code on every click), and replace history
  // so Back leaves the detail page instead of replaying tab switches.
  const selectTab = useCallback(
    (tab: Tab) => {
      setActiveTab(tab);
      if (!name) return;
      navigate(`/agents/${name}/${tab}`, { replace: true });
    },
    [name, navigate],
  );

  const [loopOpen, setLoopOpen] = useState(false);
  const { subscribe } = useWebSocket();

  const agentFetcher = useCallback(async () => {
    if (!name) throw new Error("No agent name");
    return api.getAgent(name);
  }, [name]);

  const {
    data: agent,
    loading,
    error,
    refresh,
  } = usePolling<Agent>(agentFetcher, 3000);

  // Refresh on agent state changes
  useEffect(() => {
    return subscribe("agent.state_changed", () => void refresh());
  }, [subscribe, refresh]);

  // Keyboard shortcuts: 1-4 for tabs, s for start/stop, Esc for back
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      switch (e.key) {
        case "1":
          selectTab("attach");
          break;
        case "2":
          selectTab("live");
          break;
        case "3":
          selectTab("config");
          break;
        case "4":
          selectTab("metrics");
          break;
        case "5":
          selectTab("code");
          break;
        case "Escape":
          navigate(agentsUrl);
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
    };
  }, [navigate, selectTab, agentsUrl]);

  /* ─── Loading / Error ─── */

  if (loading && !agent) {
    return (
      <div className="flex items-center justify-center h-full">
        <span className="text-sm text-mycel-muted" style={{ fontFamily: MONO }}>
          loading\u2026
        </span>
      </div>
    );
  }
  if (error && !agent) {
    return (
      <div className="p-6 space-y-3">
        <div className="text-sm text-mycel-error" style={{ fontFamily: MONO }}>
          error: {error}
        </div>
        <Link
          to={agentsUrl}
          className="text-xs text-mycel-accent hover:underline"
          style={{ fontFamily: MONO }}
        >
          \u2190 back to agents
        </Link>
      </div>
    );
  }
  if (!agent) return null;

  const lastSeen =
    agent.updated_at ?? agent.started_at ?? agent.created_at;

  /* ─── Render ─── */

  return (
    <div className="flex flex-col h-full">
      {/* ═══ HEADER — HUD status bar ═══
          Three-section rhythm: identity → status → tabs. Hairline
          separators between sections give the row explicit structure
          instead of the previous run-on gap-2.5 line. */}
      <header className="shrink-0 border-b border-mycel-border/40 bg-mycel-surface/40 backdrop-blur-sm">
        <div className="flex items-center gap-3 min-w-0 px-4 sm:px-6 h-[48px]">
          {/* ── Identity ── */}
          <Link
            to={agentsUrl}
            className="text-mycel-muted hover:text-mycel-text transition-colors shrink-0"
            title="Back to agents"
            aria-label="Back to agents"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
              <path d="M9 3l-4 4 4 4" />
            </svg>
          </Link>
          <AgentIcon state={agent.state} size={28} tool={agent.tool} />
          <span className="text-[14px] font-semibold text-mycel-text tracking-tight shrink-0">
            {agent.name}
          </span>
          {agent.runtime_backend && (
            <span className="shrink-0 text-mycel-muted" title={`Runtime: ${agent.runtime_backend}`}>
              {agent.runtime_backend === "docker" ? (
                <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="1" y="4" width="12" height="8" rx="1" />
                  <path d="M4 4V2h6v2" />
                  <path d="M5 7h4M5 9.5h4" opacity="0.5" />
                </svg>
              ) : (
                <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="1.5" y="1.5" width="11" height="8" rx="1.5" />
                  <path d="M7 9.5v2.5M4 12h6" />
                </svg>
              )}
            </span>
          )}

          {/* Hairline separator — identity ↔ status */}
          <span className="hidden sm:block h-4 w-px bg-mycel-border/50 shrink-0" aria-hidden />

          {/* ── Status ── state chip + task line */}
          <span
            className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-medium tracking-wide uppercase ring-1 shrink-0 ${
              agent.state === "working" ? "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30" :
              agent.state === "idle" ? "bg-amber-500/15 text-amber-300 ring-amber-500/25" :
              agent.state === "stuck" ? "bg-amber-500/20 text-amber-300 ring-amber-500/30" :
              agent.state === "error" ? "bg-rose-500/15 text-rose-300 ring-rose-500/30" :
              agent.state === "starting" ? "bg-sky-500/15 text-sky-300 ring-sky-500/30" :
              agent.state === "done" ? "bg-sky-500/15 text-sky-300 ring-sky-500/25" :
              "bg-zinc-500/15 text-zinc-300 ring-zinc-500/25"
            }`}
            title={agent.state}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full ${
                agent.state === "working" ? "bg-emerald-400 animate-pulse" :
                agent.state === "starting" ? "bg-sky-400 animate-pulse" :
                agent.state === "stuck" ? "bg-amber-400 animate-pulse" :
                agent.state === "error" ? "bg-rose-400" :
                "bg-current opacity-60"
              }`}
              aria-hidden
            />
            {agent.state}
          </span>

          <LoopIconButton agentName={agent.name} agentState={agent.state} onClick={() => setLoopOpen(true)} />

          {agent.task && (
            <span
              className="text-[11px] text-mycel-muted/70 truncate min-w-0 flex-shrink"
              title={agent.task}
            >
              {agent.task}
            </span>
          )}

          {lastSeen && (
            <span
              className="text-[10px] text-mycel-muted tabular-nums shrink-0"
              title={formatTime(lastSeen)}
              style={{ fontFamily: MONO }}
            >
              {formatRelative(lastSeen)}
            </span>
          )}

          {/* Hairline separator — status ↔ tabs. ml-auto pushes tabs to
              the right edge. */}
          <span className="ml-auto hidden sm:block h-4 w-px bg-mycel-border/50 shrink-0" aria-hidden />

          {/* ── Tabs ── */}
          {TABS.map((tab) => {
            const isActive = activeTab === tab.key;
            return (
              <button
                key={tab.key}
                onClick={() => selectTab(tab.key)}
                className={`relative px-2.5 py-1.5 text-[11px] font-medium tracking-wide uppercase transition-colors shrink-0 ${
                  isActive
                    ? "text-mycel-accent"
                    : "text-mycel-muted/55 hover:text-mycel-text"
                }`}
              >
                <span className="inline-flex items-center gap-1.5">
                  {tab.label}
                  <span
                    className={`inline-flex items-center justify-center min-w-[16px] h-[15px] rounded px-1 text-[9px] leading-none font-mono transition-colors ${
                      isActive
                        ? "bg-mycel-accent/15 text-mycel-accent"
                        : "bg-mycel-border/30 text-mycel-muted/60"
                    }`}
                    aria-hidden
                  >
                    {tab.shortcut}
                  </span>
                </span>
                {/* Active indicator — clean underline instead of the
                    old glow-bar; matches the surrounding restraint. */}
                {isActive && (
                  <span className="absolute -bottom-px left-2 right-2 h-[2px] bg-mycel-accent rounded-full" />
                )}
              </button>
            );
          })}
        </div>
      </header>

      {/* ═══ TAB CONTENT ═══ */}
      <div className="flex-1 min-h-0 flex flex-col">
        {activeTab === "live" && (
          <AgentToolStream
            agentName={agent.name}
            agentState={agent.state}
            agentTask={agent.task}
            stoppedAt={agent.stopped_at}
            updatedAt={agent.updated_at}
            startedAt={agent.started_at}
            createdAt={agent.created_at}
          />
        )}
        {activeTab === "attach" && <AttachTab agent={agent} />}
        {activeTab === "config" && <ConfigTab agent={agent} agentsUrl={agentsUrl} />}
        {activeTab === "metrics" && <MetricsTab agent={agent} />}
        {activeTab === "code" && <CodeTabPlaceholder agent={agent} />}
      </div>

      {/* Ralph Loop modal */}
      <RalphLoopModal
        open={loopOpen}
        agentName={agent.name}
        onClose={() => setLoopOpen(false)}
      />
    </div>
  );
}
