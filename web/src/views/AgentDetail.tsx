import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link, useLocation, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { Agent, AgentConfig } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatsTab as StatsTabComponent } from "../components/StatsTab";
import { WebTerminal } from "../components/WebTerminal";
import { AgentIcon } from "../components/agent-ui";
import { LoopIconButton, RalphLoopModal } from "../components/RalphLoopModal";
import { MCPServerList } from "../components/shared/MCPServerList";
import { SystemPromptEditor } from "../components/shared/SystemPromptEditor";
import { SectionRule } from "../components/shared";
import { AgentToolStream } from "../components/live/AgentToolStream";
import { MONO } from "../utils/typography";

/* ═══════════════════════════════════════════════════════════════════
   Utility
   ═══════════════════════════════════════════════════════════════════ */

function formatTime(t?: string): string {
  if (!t) return "\u2014";
  try {
    const d = new Date(t);
    if (isNaN(d.getTime())) return "\u2014";
    return d.toLocaleString();
  } catch {
    return "\u2014";
  }
}

function formatRelative(t?: string): string {
  if (!t) return "";
  try {
    const d = new Date(t);
    if (isNaN(d.getTime())) return "";
    const diffMs = Date.now() - d.getTime();
    const diffSec = Math.floor(Math.abs(diffMs) / 1000);
    if (diffSec < 60) return `${String(diffSec)}s ago`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${String(diffMin)}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${String(diffHr)}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 30) return `${String(diffDay)}d ago`;
    return d.toLocaleDateString();
  } catch {
    return "";
  }
}

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
        className="text-[9px] font-bold uppercase tracking-[0.2em] text-bc-muted/60"
        style={{ fontFamily: MONO }}
      >
        {label}
      </dt>
      <dd
        className="text-[13px] text-bc-text/90 leading-tight break-all"
        style={mono ? { fontFamily: MONO } : undefined}
      >
        {children}
      </dd>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 2 — Attach
   Direct terminal access. No overlay, no pulsing dot.
   ═══════════════════════════════════════════════════════════════════ */

function AttachTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";

  if (isStopped) {
    return (
      <div className="flex-1 flex items-center justify-center text-bc-muted text-sm">
        Agent is stopped. Start the agent to attach a terminal.
      </div>
    );
  }

  return (
    <div className="flex-1 min-h-0 relative" title="Click anywhere to focus the terminal">
      <WebTerminal agentName={agent.name} />
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 3 — Config
   System prompt, MCP servers, metadata, danger zone
   ═══════════════════════════════════════════════════════════════════ */

function ConfigTab({ agent }: { agent: Agent }) {
  const navigate = useNavigate();
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

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
              : "border-bc-accent/20 bg-bc-accent/[0.03] text-bc-accent/70"
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
            {isDocker ? "Docker container" : "tmux (localhost)"}
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
                className="flex-1 rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1.5 text-[11px] text-bc-text/90 outline-none focus:border-bc-accent/50 transition-colors"
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
                className="px-3 py-1.5 rounded border border-bc-accent/30 bg-bc-accent/10 text-[11px] text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
                style={{ fontFamily: MONO }}
              >
                {syncing ? "Syncing…" : "Sync"}
              </button>
            </div>
            <p className="text-[10px] text-bc-muted/40 mt-1 leading-relaxed" style={{ fontFamily: MONO }}>
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
          <p className="mt-2 text-[10px] text-bc-muted/40 leading-relaxed" style={{ fontFamily: MONO }}>
            {isTmux
              ? "For tmux agents, MCPs are managed via the Claude CLI. Changes here write to the agent\u2019s worktree."
              : "Changes write to .mcp.json in the container."}
          </p>
        </section>

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
              {agent.session || config?.session || "\u2014"}
            </MetaCell>
            <MetaCell label="Created">
              <span className="tabular-nums">{formatTime(agent.created_at)}</span>
            </MetaCell>
            <MetaCell label="Started">
              <span className="tabular-nums">{formatTime(agent.started_at)}</span>
            </MetaCell>
            {agent.stopped_at && (
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
            <p className="mb-3 text-[10px] text-bc-muted/40 italic" style={{ fontFamily: MONO }}>
              Common: ANTHROPIC_API_KEY, GITHUB_TOKEN, AWS_ACCESS_KEY_ID
            </p>
          )}

          {/* Existing env var rows */}
          {envVars.length > 0 && (
            <div className="mb-3 rounded-md border border-bc-border/30 overflow-hidden divide-y divide-bc-border/20">
              {envVars.map((ev, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 px-3 py-2 bg-bc-surface/20 group"
                >
                  <span
                    className="flex-1 min-w-0 text-[11px] font-semibold text-bc-text/80 truncate"
                    style={{ fontFamily: MONO }}
                  >
                    {ev.key}
                  </span>
                  <span className="text-bc-muted/30 text-[11px]">=</span>
                  <span
                    className="flex-1 min-w-0 text-[11px] text-bc-muted/70 truncate"
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
                    className="shrink-0 text-[11px] text-bc-muted/30 hover:text-bc-error transition-colors opacity-0 group-hover:opacity-100"
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
              className="w-32 rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1 text-[11px] text-bc-text/90 placeholder:text-bc-muted/40 outline-none focus:border-bc-accent/50 transition-colors"
              style={{ fontFamily: MONO }}
            />
            <span className="text-bc-muted/40 text-[11px]" style={{ fontFamily: MONO }}>=</span>
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
              className="flex-1 max-w-[200px] rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1 text-[11px] text-bc-text/90 placeholder:text-bc-muted/40 outline-none focus:border-bc-accent/50 transition-colors"
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
              className="px-2.5 py-1 rounded border border-bc-accent/30 bg-bc-accent/10 text-[11px] text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
              style={{ fontFamily: MONO }}
            >
              + Add
            </button>
          </div>

          <p className="mt-2 text-[10px] text-bc-muted/40 leading-relaxed" style={{ fontFamily: MONO }}>
            {isTmux
              ? "Set via provider CLI environment · Env vars are applied on agent restart"
              : "Injected as container environment variables · Env vars are applied on agent restart"}
          </p>
          {agent.tool === "claude" && (
            <div className="mt-2 text-[10px] text-bc-muted/50" style={{ fontFamily: MONO }}>
              <span className="font-medium">Claude requires:</span> ANTHROPIC_API_KEY
            </div>
          )}
          {agent.tool === "gemini" && (
            <div className="mt-2 text-[10px] text-bc-muted/50" style={{ fontFamily: MONO }}>
              <span className="font-medium">Gemini requires:</span> GOOGLE_API_KEY
            </div>
          )}
          {agent.tool === "openai" && (
            <div className="mt-2 text-[10px] text-bc-muted/50" style={{ fontFamily: MONO }}>
              <span className="font-medium">OpenAI requires:</span> OPENAI_API_KEY
            </div>
          )}
        </section>

        {/* ── ACTIONS ── */}
        <section>
          <SectionRule>Actions</SectionRule>
          <div className="rounded-md border border-bc-border/30 bg-bc-surface/20 px-4 py-3">
            <div className="flex flex-wrap gap-2 items-center">
              {/* Clone — placeholder, navigates to /agents */}
              <button
                type="button"
                onClick={() => navigate("/agents")}
                className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors"
                style={{ fontFamily: MONO }}
              >
                Clone
              </button>

              {/* Archive — placeholder */}
              <button
                type="button"
                className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors"
                style={{ fontFamily: MONO }}
              >
                Archive
              </button>
            </div>
          </div>
        </section>

        {/* ── DANGER ZONE ── */}
        <section>
          <SectionRule>Danger Zone</SectionRule>
          <div className="rounded-md border border-bc-error/20 bg-bc-error/[0.02] px-4 py-3">
            {deleteError && (
              <p className="mb-2 text-[11px] text-bc-error" style={{ fontFamily: MONO }}>
                {deleteError}
              </p>
            )}
            <div className="flex flex-wrap gap-2 items-center">
              {/* Delete — with confirmation */}
              {confirmDelete ? (
                <>
                  <span
                    className="text-[11px] text-bc-error/80"
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
                          navigate("/agents");
                        })
                        .catch((err: unknown) => {
                          setDeleting(false);
                          setConfirmDelete(false);
                          setDeleteError(err instanceof Error ? err.message : "Failed to delete agent");
                        });
                    }}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-error/50 bg-bc-error/10 text-bc-error hover:bg-bc-error/20 transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    {deleting ? "Deleting…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    disabled={deleting}
                    onClick={() => setConfirmDelete(false)}
                    className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors disabled:opacity-40"
                    style={{ fontFamily: MONO }}
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmDelete(true)}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-error/30 text-bc-error/80 hover:bg-bc-error/10 transition-colors"
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
  // Defers to the top-level Code view — build an absolute path from the
  // current pathname so we land at /w/<ws>/code regardless of nesting.
  const { pathname } = useLocation();
  const match = /^(\/w\/[^/]+)\//.test(pathname)
    ? pathname.split("/").slice(0, 3).join("/")
    : "";
  const target = `${match}/code?worktree=${encodeURIComponent(agent.name)}&view=diff`;

  return (
    <div className="flex-1 flex items-center justify-center p-6">
      <div className="max-w-md text-center space-y-4" style={{ fontFamily: MONO }}>
        <p className="text-[11px] text-bc-muted/50 uppercase tracking-wider">
          Agent code — diff view
        </p>
        <p className="text-sm text-bc-muted leading-relaxed">
          Open the Code view with <span className="text-bc-text/90">{agent.name}</span>&rsquo;s
          worktree selected to see its uncommitted changes against the main repo.
        </p>
        <Link
          to={target}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-md border border-bc-accent/40 bg-bc-accent/10 text-bc-accent hover:bg-bc-accent/20 transition-colors text-[12px] font-semibold"
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
            className="text-[10px] text-bc-muted/40 italic"
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

  // Clicking a tab updates both state and URL
  const selectTab = useCallback(
    (tab: Tab) => {
      setActiveTab(tab);
      if (!name) return;
      const parts = location.pathname.split("/").filter(Boolean);
      // parts = ["w", wsId, "agents", name, maybeTab]
      const base = parts.slice(0, 4).join("/");
      navigate(`/${base}/${tab}`, { replace: false });
    },
    [name, location.pathname, navigate],
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
          navigate("/agents");
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
    };
  }, [navigate]);

  /* ─── Loading / Error ─── */

  if (loading && !agent) {
    return (
      <div className="flex items-center justify-center h-full">
        <span className="text-sm text-bc-muted/50" style={{ fontFamily: MONO }}>
          loading\u2026
        </span>
      </div>
    );
  }
  if (error && !agent) {
    return (
      <div className="p-6 space-y-3">
        <div className="text-sm text-bc-error" style={{ fontFamily: MONO }}>
          error: {error}
        </div>
        <Link
          to="/agents"
          className="text-xs text-bc-accent hover:underline"
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
      {/* ═══ HEADER — HUD status bar ═══ */}
      <header className="shrink-0 border-b border-bc-border/40">
        <div className="flex items-center gap-2.5 min-w-0 px-6 h-[42px]">
          {/* Back link */}
          <Link
            to="/agents"
            className="text-[10px] text-bc-muted/40 hover:text-bc-text transition-colors shrink-0"
            style={{ fontFamily: MONO }}
          >
            ←
          </Link>

          {/* Shape with provider icon inside */}
          <AgentIcon
            state={agent.state}
            size={30}
            tool={agent.tool}
          />

          {/* Agent name */}
          <span
            className="text-[13px] font-bold text-bc-text tracking-tight shrink-0"
            style={{ fontFamily: MONO }}
          >
            {agent.name}
          </span>

          {/* Runtime icon — monitor for tmux, container for docker */}
          {agent.runtime_backend && (
            <span className="shrink-0 text-bc-muted/40" title={agent.runtime_backend}>
              {agent.runtime_backend === "docker" ? (
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="1" y="4" width="12" height="8" rx="1" />
                  <path d="M4 4V2h6v2" />
                  <path d="M5 7h4M5 9.5h4" opacity="0.5" />
                </svg>
              ) : (
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="1.5" y="1.5" width="11" height="8" rx="1.5" />
                  <path d="M7 9.5v2.5M4 12h6" />
                </svg>
              )}
            </span>
          )}

          {/* Status dot */}
          <span
            className={`shrink-0 w-2 h-2 rounded-full ${
              agent.state === "working" ? "bg-green-500 animate-pulse" :
              agent.state === "idle" ? "bg-green-500/60" :
              agent.state === "stuck" ? "bg-amber-500 animate-pulse" :
              agent.state === "error" ? "bg-red-500" :
              agent.state === "starting" ? "bg-blue-400 animate-pulse" :
              "bg-bc-muted/30"
            }`}
            title={agent.state}
          />

          {/* Loop icon — no background, just the icon */}
          <LoopIconButton agentName={agent.name} agentState={agent.state} onClick={() => setLoopOpen(true)} />

          {/* Task text */}
          {agent.task && (
            <span
              className="text-[10px] text-bc-muted/50 truncate max-w-[220px]"
              title={agent.task}
              style={{ fontFamily: MONO }}
            >
              {agent.task}
            </span>
          )}

          {/* Timestamp */}
          {lastSeen && (
            <span
              className="text-[10px] text-bc-muted/25 tabular-nums shrink-0"
              title={formatTime(lastSeen)}
              style={{ fontFamily: MONO }}
            >
              {formatRelative(lastSeen)}
            </span>
          )}

          {/* Separator */}
          <span className="shrink-0 w-px h-4 bg-bc-border/50 ml-auto" />

          {/* Tab buttons — inline in header */}
          {TABS.map((tab) => {
            const isActive = activeTab === tab.key;
            return (
              <button
                key={tab.key}
                onClick={() => selectTab(tab.key)}
                className={`relative px-3 py-1.5 text-[11px] font-semibold tracking-wide uppercase transition-colors shrink-0 ${
                  isActive
                    ? "text-bc-accent"
                    : "text-bc-muted/50 hover:text-bc-muted"
                }`}
                style={{ fontFamily: MONO }}
              >
                {tab.label}
                <span className="ml-1 text-[9px] opacity-40">{tab.shortcut}</span>
                {/* Active indicator — bottom glow bar */}
                {isActive && (
                  <span className="absolute bottom-0 left-1 right-1 h-[2px] rounded-full bg-bc-accent shadow-[0_0_8px_rgba(var(--bc-accent-rgb,255,165,0),0.5)]" />
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
        {activeTab === "config" && <ConfigTab agent={agent} />}
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
