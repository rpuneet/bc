import { Fragment, useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { Agent, AgentMetricTS } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { InlineTerminal } from "../components/InlineTerminal";
import { truncate } from "../utils/text";

// --- Create Agent Form ---

interface CreateFormState {
  name: string;
  role: string;
  tool: string;
  runtime: string;
}

function CreateAgentForm({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [roles, setRoles] = useState<string[]>([]);
  const [tools, setTools] = useState<string[]>([]);
  const [form, setForm] = useState<CreateFormState>({
    name: "",
    role: "",
    tool: "",
    runtime: "",
  });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch roles and tools when form opens
  useEffect(() => {
    if (!open) return;
    api
      .listRoles()
      .then((r) => setRoles(Object.keys(r)))
      .catch(() => { /* ignore */ });
    api
      .listCLITools()
      .then((t) => setTools(t.filter((tool) => tool.enabled).map((tool) => tool.name)))
      .catch(() => { /* ignore */ });
  }, [open]);

  const handleCreate = async () => {
    if (!form.role) {
      setError("Role is required");
      return;
    }
    setCreating(true);
    setError(null);
    try {
      await api.createAgent({
        name: form.name || undefined,
        role: form.role,
        tool: form.tool || undefined,
        runtime: form.runtime || undefined,
      });
      setForm({ name: "", role: "", tool: "", runtime: "" });
      setOpen(false);
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create agent");
    } finally {
      setCreating(false);
    }
  };

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="px-3 py-1.5 text-sm rounded bg-bc-accent text-white hover:bg-bc-accent/80 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        aria-label="Create agent"
      >
        + Create Agent
      </button>
    );
  }

  return (
    <div className="rounded border border-bc-border bg-bc-surface p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">Create Agent</h2>
        <button
          onClick={() => {
            setOpen(false);
            setError(null);
          }}
          className="text-bc-muted hover:text-bc-text text-sm"
        >
          Cancel
        </button>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div>
          <label className="block text-xs text-bc-muted mb-1">
            Name (optional)
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            placeholder="auto-generated"
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text placeholder:text-bc-muted/50 focus:outline-none focus:ring-1 focus:ring-bc-accent"
          />
        </div>

        <div>
          <label className="block text-xs text-bc-muted mb-1">Role *</label>
          <select
            value={form.role}
            onChange={(e) => setForm((f) => ({ ...f, role: e.target.value }))}
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
          >
            <option value="">Select role...</option>
            {roles.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs text-bc-muted mb-1">Tool</label>
          <select
            value={form.tool}
            onChange={(e) => setForm((f) => ({ ...f, tool: e.target.value }))}
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
          >
            <option value="">Default</option>
            {(() => {
              const AI_PROVIDERS = new Set(["claude", "codex", "cursor", "gemini", "aider", "openclaw", "opencode"]);
              const providers = tools.filter((t) => AI_PROVIDERS.has(t));
              const cliTools = tools.filter((t) => !AI_PROVIDERS.has(t));
              return (
                <>
                  {providers.length > 0 && (
                    <optgroup label="AI Providers">
                      {providers.map((t) => (
                        <option key={t} value={t}>{t}</option>
                      ))}
                    </optgroup>
                  )}
                  {cliTools.length > 0 && (
                    <optgroup label="CLI Tools">
                      {cliTools.map((t) => (
                        <option key={t} value={t}>{t}</option>
                      ))}
                    </optgroup>
                  )}
                </>
              );
            })()}
          </select>
        </div>

        <div>
          <label className="block text-xs text-bc-muted mb-1">Runtime</label>
          <select
            value={form.runtime}
            onChange={(e) =>
              setForm((f) => ({ ...f, runtime: e.target.value }))
            }
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
          >
            <option value="">Default</option>
            <option value="tmux">tmux</option>
            <option value="docker">docker</option>
            <option value="localhost">localhost</option>
          </select>
        </div>
      </div>

      {error && <p className="text-xs text-bc-error">{error}</p>}

      <div className="flex justify-end">
        <button
          onClick={handleCreate}
          disabled={creating}
          className="px-3 py-1.5 text-sm rounded bg-bc-accent text-white hover:bg-bc-accent/80 transition-colors disabled:opacity-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        >
          {creating ? "Creating..." : "Create"}
        </button>
      </div>
    </div>
  );
}

// --- Inline Rename ---

function InlineAgentName({
  agent,
  onRenamed,
}: {
  agent: Agent;
  onRenamed: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [newName, setNewName] = useState(agent.name);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    const trimmed = newName.trim();
    if (!trimmed || trimmed === agent.name) {
      setEditing(false);
      setNewName(agent.name);
      return;
    }
    setSaving(true);
    try {
      await api.renameAgent(agent.name, trimmed);
      setEditing(false);
      onRenamed();
    } catch {
      setNewName(agent.name);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  if (editing) {
    return (
      <input
        type="text"
        value={newName}
        onChange={(e) => setNewName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") handleSave();
          if (e.key === "Escape") {
            setEditing(false);
            setNewName(agent.name);
          }
        }}
        onBlur={handleSave}
        disabled={saving}
        autoFocus
        onClick={(e) => e.stopPropagation()}
        className="px-1 py-0.5 text-sm font-medium rounded border border-bc-accent bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent w-32"
        aria-label="Rename agent"
      />
    );
  }

  return (
    <button
      type="button"
      className="font-medium cursor-pointer hover:text-bc-accent transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg rounded"
      onClick={(e) => {
        e.stopPropagation();
        setEditing(true);
      }}
      title="Click to rename"
      aria-label={`Rename agent ${agent.name}`}
    >
      {agent.name}
    </button>
  );
}

// --- Agent Action Buttons ---

function AgentActions({ agent, onDone }: { agent: Agent; onDone: () => void }) {
  const [confirming, setConfirming] = useState<"delete" | null>(null);
  const [busy, setBusy] = useState(false);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await action();
      onDone();
    } catch {
      // errors are transient; the list will refresh
    } finally {
      setBusy(false);
      setConfirming(null);
    }
  };

  const isStopped = agent.state === "stopped" || agent.state === "error";
  const isRunning = !isStopped;

  if (confirming === "delete") {
    return (
      <span
        className="inline-flex items-center gap-1"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="text-xs text-bc-error mr-1">Delete?</span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.deleteAgent(agent.name));
          }}
          disabled={busy}
          className="px-1.5 py-0.5 text-xs rounded bg-bc-error/20 text-bc-error hover:bg-bc-error/30 disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
          aria-label={`Confirm delete agent ${agent.name}`}
        >
          {busy ? "..." : "Yes"}
        </button>
        <button
          onClick={(e) => {
            e.stopPropagation();
            setConfirming(null);
          }}
          aria-label="Cancel delete"
          className="px-1.5 py-0.5 text-xs rounded bg-bc-border/50 text-bc-muted hover:text-bc-text focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        >
          No
        </button>
      </span>
    );
  }

  return (
    <span
      className="inline-flex items-center gap-1"
      onClick={(e) => e.stopPropagation()}
    >
      {isStopped && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.startAgent(agent.name));
          }}
          disabled={busy}
          title="Start agent"
          aria-label={`Start agent ${agent.name}`}
          className="px-1.5 py-0.5 text-xs rounded bg-bc-success/20 text-bc-success hover:bg-bc-success/30 disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        >
          {busy ? "..." : "Start"}
        </button>
      )}
      {isRunning && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.stopAgent(agent.name));
          }}
          disabled={busy}
          title="Stop agent"
          aria-label={`Stop agent ${agent.name}`}
          className="px-1.5 py-0.5 text-xs rounded bg-bc-warning/20 text-bc-warning hover:bg-bc-warning/30 disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
        >
          {busy ? "..." : "Stop"}
        </button>
      )}
      <button
        onClick={(e) => {
          e.stopPropagation();
          setConfirming("delete");
        }}
        title="Delete agent"
        aria-label={`Delete agent ${agent.name}`}
        className="px-1.5 py-0.5 text-xs rounded bg-bc-error/10 text-bc-error/70 hover:bg-bc-error/20 hover:text-bc-error focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
      >
        Del
      </button>
    </span>
  );
}

// --- Main Agents View ---

export function Agents() {
  const fetcher = useCallback(async () => {
    const res = await api.listAgents();
    return res;
  }, []);
  const {
    data: agents,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, 5000);
  const { subscribe } = useWebSocket();
  const navigate = useNavigate();

  const [peekAgent, setPeekAgent] = useState<string | null>(null);
  const [stoppingAll, setStoppingAll] = useState(false);
  const [latestStats, setLatestStats] = useState<Record<string, AgentMetricTS>>({});

  // Fetch latest CPU/Mem stats for all agents
  useEffect(() => {
    const fetchStats = () => {
      api.getAgentStatsLatest().then((metrics) => {
        const map: Record<string, AgentMetricTS> = {};
        for (const m of metrics) {
          map[m.agent_name] = m;
        }
        setLatestStats(map);
      }).catch(() => {
        // Stats unavailable — not critical
      });
    };
    fetchStats();
    const interval = setInterval(fetchStats, 30000);
    return () => clearInterval(interval);
  }, []);

  const handleStopAll = async () => {
    setStoppingAll(true);
    try {
      await api.stopAllAgents();
      refresh();
    } catch {
      // transient error; list will refresh
    } finally {
      setStoppingAll(false);
    }
  };

  // Refresh on agent lifecycle events via SSE
  useEffect(() => {
    const unsubs = [
      subscribe("agent.state_changed", () => void refresh()),
      subscribe("agent.created", () => void refresh()),
      subscribe("agent.stopped", () => void refresh()),
      subscribe("agent.deleted", () => void refresh()),
    ];
    return () => unsubs.forEach((fn) => fn());
  }, [subscribe, refresh]);

  const handlePeekToggle = (agentName: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setPeekAgent((prev) => (prev === agentName ? null : agentName));
  };

  const columns = [
    "Name",
    "Role",
    "Tool",
    "Status",
    "Task",
    "Tokens",
    "CPU %",
    "Mem %",
    "MCP",
    "",
    "",
  ] as const;

  if (loading && !agents) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-6 w-24 animate-pulse rounded bg-bc-border/50" />
        <LoadingSkeleton variant="table" rows={4} />
      </div>
    );
  }
  if (timedOut && !agents) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Agents took too long to load"
          description="The server may be unavailable. Check your connection and try again."
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }
  if (error && !agents) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load agents"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  const agentList = agents ?? [];

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Agents</h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-bc-muted">
            {agentList.length} agents
          </span>
          {agentList.some(
            (a) => a.state !== "stopped" && a.state !== "error",
          ) && (
            <button
              onClick={handleStopAll}
              disabled={stoppingAll}
              className="px-3 py-1.5 text-sm rounded bg-bc-error/20 text-bc-error hover:bg-bc-error/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-error focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
              aria-label="Stop all agents"
            >
              {stoppingAll ? "Stopping..." : "Stop All"}
            </button>
          )}
        </div>
      </div>

      <CreateAgentForm onCreated={refresh} />

      <div className="rounded border border-bc-border overflow-x-auto">
        {agentList.length === 0 ? (
          <EmptyState
            icon=">"
            title="No agents yet"
            description="Create your first agent using the form above."
          />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-bc-border text-left">
                <th className="px-4 py-2 font-medium text-bc-muted">Name</th>
                <th className="px-4 py-2 font-medium text-bc-muted">Role</th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden sm:table-cell">
                  Tool
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted">Status</th>
                <th className="px-4 py-2 font-medium text-bc-muted">Task</th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden md:table-cell">
                  Tokens
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden md:table-cell">
                  CPU %
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden md:table-cell">
                  Mem %
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden md:table-cell">
                  MCP
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted">Actions</th>
                <th className="px-4 py-2 font-medium text-bc-muted w-10"></th>
              </tr>
            </thead>
            <tbody>
              {agentList.map((a) => (
                <Fragment key={a.name}>
                  <tr
                    onClick={() =>
                      navigate(`/agents/${encodeURIComponent(a.name)}`)
                    }
                    onKeyDown={(e) => {
                      if (e.key === "Enter") navigate(`/agents/${encodeURIComponent(a.name)}`);
                    }}
                    role="link"
                    tabIndex={0}
                    className="border-b border-bc-border/50 cursor-pointer hover:bg-bc-surface transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
                  >
                    <td className="px-4 py-2">
                      <InlineAgentName agent={a} onRenamed={refresh} />
                    </td>
                    <td className="px-4 py-2">
                      <span className="text-bc-muted">{a.role}</span>
                    </td>
                    <td className="px-4 py-2 hidden sm:table-cell">
                      <span className="text-bc-muted">
                        {a.tool || "\u2014"}
                      </span>
                    </td>
                    <td className="px-4 py-2">
                      <StatusBadge status={a.state} />
                    </td>
                    <td className="px-4 py-2">
                      <span className="text-bc-muted" title={a.task}>
                        {a.task ? truncate(a.task, 50) : "\u2014"}
                      </span>
                    </td>
                    <td className="px-4 py-2 hidden md:table-cell">
                      <span className="text-bc-muted">
                        {a.total_tokens != null && a.total_tokens > 0
                          ? a.total_tokens.toLocaleString()
                          : "\u2014"}
                      </span>
                    </td>
                    <td className="px-4 py-2 hidden md:table-cell">
                      <span className="text-bc-muted">
                        {latestStats[a.name] != null
                          ? `${latestStats[a.name]!.cpu_percent.toFixed(1)}%`
                          : "\u2014"}
                      </span>
                    </td>
                    <td className="px-4 py-2 hidden md:table-cell">
                      <span className="text-bc-muted">
                        {latestStats[a.name] != null
                          ? `${latestStats[a.name]!.mem_percent.toFixed(1)}%`
                          : "\u2014"}
                      </span>
                    </td>
                    <td className="px-4 py-2 hidden md:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {(a.mcp_servers ?? []).length === 0 ? (
                          <span className="text-bc-muted">{"\u2014"}</span>
                        ) : (a.mcp_servers ?? []).length <= 3 ? (
                          (a.mcp_servers ?? []).map((s) => (
                            <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-bc-accent/10 text-bc-accent font-medium">
                              {s.replace(/^mcp__/, "")}
                            </span>
                          ))
                        ) : (
                          <>
                            {(a.mcp_servers ?? []).slice(0, 2).map((s) => (
                              <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-bc-accent/10 text-bc-accent font-medium">
                                {s.replace(/^mcp__/, "")}
                              </span>
                            ))}
                            <span className="text-[10px] text-bc-muted">+{(a.mcp_servers ?? []).length - 2}</span>
                          </>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-2">
                      <AgentActions agent={a} onDone={refresh} />
                    </td>
                    <td className="px-4 py-2 text-center">
                      <button
                        onClick={(e) => handlePeekToggle(a.name, e)}
                        className={`inline-flex items-center justify-center w-7 h-7 rounded transition-colors focus:ring-2 focus:ring-bc-accent focus:outline-none ${
                          peekAgent === a.name
                            ? "bg-bc-accent/20 text-bc-accent"
                            : "text-bc-muted hover:text-bc-text hover:bg-bc-surface"
                        }`}
                        title={
                          peekAgent === a.name ? "Hide output" : "Peek output"
                        }
                        aria-label={
                          peekAgent === a.name ? "Hide output" : "Peek output"
                        }
                      >
                        {peekAgent === a.name ? "\u2296" : "\u2295"}
                      </button>
                    </td>
                  </tr>
                  {peekAgent === a.name && (
                    <tr
                      key={`${a.name}-peek`}
                      className="border-b border-bc-border/50"
                    >
                      <td colSpan={columns.length} className="p-0">
                        <InlineTerminal agentName={a.name} lines={10} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
