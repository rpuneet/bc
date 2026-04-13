import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Agent, BulkResult } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { InlineTerminal } from "../components/InlineTerminal";
import { truncate } from "../utils/text";

function KeyHint({ k, label }: { k: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <kbd className="px-1 py-px rounded border border-bc-border/60 bg-bc-bg text-[9px] font-mono text-bc-muted">
        {k}
      </kbd>
      <span>{label}</span>
    </span>
  );
}

// --- Create Agent Form ---

interface CreateFormState {
  name: string;
  role: string;
  tool: string;
  runtime: string;
  task: string;
}

// Hardcoded templates for common agent setups. Templates pre-fill the
// create form with a role/tool/runtime + optional task prompt.
interface AgentTemplate {
  id: string;
  label: string;
  description: string;
  role: string;
  tool: string;
  runtime: string;
  taskPrompt?: string;
}

const TEMPLATES: AgentTemplate[] = [
  {
    id: "feature-dev",
    label: "Feature developer",
    description: "Claude in Docker, feature-dev role",
    role: "feature-dev",
    tool: "claude",
    runtime: "docker",
  },
  {
    id: "reviewer",
    label: "Code reviewer",
    description: "Claude in tmux, reviewer role",
    role: "reviewer",
    tool: "claude",
    runtime: "tmux",
  },
  {
    id: "manager",
    label: "Manager",
    description: "Gemini in tmux, manager role",
    role: "manager",
    tool: "gemini",
    runtime: "tmux",
  },
  {
    id: "blank",
    label: "Blank",
    description: "Empty form — pick everything manually",
    role: "",
    tool: "",
    runtime: "",
  },
];

function CreateAgentForm({
  onCreated,
  existingAgents,
}: {
  onCreated: () => void;
  existingAgents: Agent[];
}) {
  const [open, setOpen] = useState(false);
  const [roles, setRoles] = useState<string[]>([]);
  const [tools, setTools] = useState<string[]>([]);
  const [form, setForm] = useState<CreateFormState>({
    name: "",
    role: "",
    tool: "",
    runtime: "",
    task: "",
  });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch roles and tools when form opens
  useEffect(() => {
    if (!open) return;
    api
      .listRoles()
      .then((r) => { setRoles(Object.keys(r)); })
      .catch(() => { /* ignore */ });
    api
      .listCLITools()
      .then((t) => { setTools(t.filter((tool) => tool.enabled).map((tool) => tool.name)); })
      .catch(() => { /* ignore */ });
  }, [open]);

  const applyTemplate = (t: AgentTemplate) => {
    setForm((f) => ({
      ...f,
      role: t.role,
      tool: t.tool,
      runtime: t.runtime,
      task: t.taskPrompt ?? f.task,
    }));
  };

  const copyFromAgent = (a: Agent) => {
    setForm((f) => ({
      ...f,
      role: a.role,
      tool: a.tool,
      runtime: a.runtime_backend ?? "",
    }));
  };

  // Top 3 most recent agents as "copy config" suggestions
  const recentAgents = useMemo(() => {
    return [...existingAgents]
      .filter((a) => a.created_at)
      .sort((a, b) => (b.created_at > a.created_at ? 1 : -1))
      .slice(0, 3);
  }, [existingAgents]);

  const handleCreate = async () => {
    if (!form.role) {
      setError("Role is required");
      return;
    }
    setCreating(true);
    setError(null);
    try {
      const created = await api.createAgent({
        name: form.name || undefined,
        role: form.role,
        tool: form.tool || undefined,
        runtime: form.runtime || undefined,
      });
      // If a task was entered, send it immediately after creation so the
      // user doesn't have to open the agent and attach work manually.
      const task = form.task.trim();
      if (task) {
        try {
          await api.sendToAgent(created.name, task);
        } catch {
          // Best-effort — the agent is already created, surface only create errors.
        }
      }
      setForm({ name: "", role: "", tool: "", runtime: "", task: "" });
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
        onClick={() => { setOpen(true); }}
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
          type="button"
          onClick={() => { setOpen(false); setError(null); }}
          className="text-bc-muted hover:text-bc-text text-sm"
        >
          Cancel
        </button>
      </div>

      {/* Templates — quick presets */}
      <div>
        <div className="text-[10px] font-semibold text-bc-muted uppercase tracking-wider mb-1.5">
          Start from template
        </div>
        <div className="flex flex-wrap gap-1.5">
          {TEMPLATES.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => { applyTemplate(t); }}
              title={t.description}
              className="px-2.5 py-1 text-xs rounded-md border border-bc-border bg-bc-bg text-bc-text hover:border-bc-accent/50 hover:bg-bc-accent/5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {/* Recent config chips — copy from existing agents */}
      {recentAgents.length > 0 && (
        <div>
          <div className="text-[10px] font-semibold text-bc-muted uppercase tracking-wider mb-1.5">
            Or copy config from
          </div>
          <div className="flex flex-wrap gap-1.5">
            {recentAgents.map((a) => (
              <button
                key={a.name}
                type="button"
                onClick={() => { copyFromAgent(a); }}
                title={`${a.role} · ${a.tool} · ${a.runtime_backend ?? "default"}`}
                className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md border border-bc-border bg-bc-bg hover:border-bc-accent/40 hover:bg-bc-accent/5 transition-colors text-[11px] text-bc-muted hover:text-bc-text focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
              >
                <span className="w-1 h-1 rounded-full bg-bc-accent/60" />
                {a.name}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div>
          <label className="block text-xs text-bc-muted mb-1">
            Name (optional)
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => { setForm((f) => ({ ...f, name: e.target.value })); }}
            placeholder="auto-generated"
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text placeholder:text-bc-muted/50 focus:outline-none focus:ring-1 focus:ring-bc-accent"
          />
        </div>

        <div>
          <label className="block text-xs text-bc-muted mb-1">Role *</label>
          <select
            value={form.role}
            onChange={(e) => { setForm((f) => ({ ...f, role: e.target.value })); }}
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
            onChange={(e) => { setForm((f) => ({ ...f, tool: e.target.value })); }}
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
            onChange={(e) => { setForm((f) => ({ ...f, runtime: e.target.value })); }}
            className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
          >
            <option value="">Default</option>
            <option value="tmux">tmux</option>
            <option value="docker">docker</option>
            <option value="localhost">localhost</option>
          </select>
        </div>
      </div>

      {/* Task (optional) — sent to the agent immediately after creation */}
      <div>
        <label className="block text-xs text-bc-muted mb-1">
          Initial task <span className="text-bc-muted/50">(optional, sent to the agent on create)</span>
        </label>
        <textarea
          value={form.task}
          onChange={(e) => { setForm((f) => ({ ...f, task: e.target.value })); }}
          placeholder="e.g. Review PR #428 and leave comments on the auth flow"
          rows={3}
          className="w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text placeholder:text-bc-muted/50 focus:outline-none focus:ring-1 focus:ring-bc-accent resize-none"
        />
      </div>

      {error && <p className="text-xs text-bc-error">{error}</p>}

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => { void handleCreate(); }}
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
  const [searchParams, setSearchParams] = useSearchParams();

  const [peekAgent, setPeekAgent] = useState<string | null>(null);
  const [stoppingAll, setStoppingAll] = useState(false);

  // Search + filter + bulk state (URL-synced where useful)
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const roleFilter = searchParams.get("role") ?? "";
  const stateFilter = searchParams.get("state") ?? "";
  const toolFilter = searchParams.get("tool") ?? "";
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState<string | null>(null);
  const [focusIndex, setFocusIndex] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const updateFilter = (key: "role" | "state" | "tool", value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    setSearchParams(next, { replace: true });
  };

  // Debounced search → URL sync
  useEffect(() => {
    const t = setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (search) next.set("q", search);
      else next.delete("q");
      setSearchParams(next, { replace: true });
    }, 250);
    return () => { clearTimeout(t); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  // Keyboard shortcuts now live after displayRows is declared below —
  // see the useEffect labelled "Global keyboard shortcuts".

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
    "Select",
    "Name",
    "Runtime",
    "Provider",
    "Status",
    "Task",
    "MCP",
    "Actions",
    "",
  ] as const;

  // Compute filter options from agent list
  const allAgents = useMemo(() => agents ?? [], [agents]);
  const { availableRoles, availableStates, availableTools } = useMemo(() => {
    const r = new Set<string>();
    const s = new Set<string>();
    const t = new Set<string>();
    for (const a of allAgents) {
      if (a.role) r.add(a.role);
      if (a.state) s.add(a.state);
      if (a.tool) t.add(a.tool);
    }
    return {
      availableRoles: Array.from(r).sort((x, y) => x.localeCompare(y)),
      availableStates: Array.from(s).sort((x, y) => x.localeCompare(y)),
      availableTools: Array.from(t).sort((x, y) => x.localeCompare(y)),
    };
  }, [allAgents]);

  // Apply filters + search
  const filteredAgents = useMemo(() => {
    const q = search.trim().toLowerCase();
    return allAgents.filter((a) => {
      if (q && !a.name.toLowerCase().includes(q) && !(a.task ?? "").toLowerCase().includes(q)) {
        return false;
      }
      if (roleFilter && a.role !== roleFilter) return false;
      if (stateFilter && a.state !== stateFilter) return false;
      if (toolFilter && a.tool !== toolFilter) return false;
      return true;
    });
  }, [allAgents, search, roleFilter, stateFilter, toolFilter]);

  // Flat list only — no tree view.
  const displayRows = useMemo(() => filteredAgents, [filteredAgents]);

  // Clamp focusIndex when displayRows shrinks (e.g. after filtering).
  useEffect(() => {
    if (focusIndex >= displayRows.length && displayRows.length > 0) {
      setFocusIndex(displayRows.length - 1);
    }
  }, [displayRows.length, focusIndex]);

  // Global keyboard shortcuts. These work when focus is anywhere on the page,
  // except inside inputs/textareas/contenteditable elements.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const isInput =
        target != null &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable);

      // "/" always focuses search even from inputs? no — only outside.
      if (e.key === "/" && !isInput) {
        e.preventDefault();
        searchInputRef.current?.focus();
        return;
      }
      if (e.key === "Escape" && selected.size > 0) {
        setSelected(new Set());
        return;
      }
      if (isInput) return;

      // Row navigation
      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        setFocusIndex((i) => Math.min(i + 1, Math.max(0, displayRows.length - 1)));
        return;
      }
      if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        setFocusIndex((i) => Math.max(i - 1, 0));
        return;
      }
      // Enter opens the focused agent
      if (e.key === "Enter") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          navigate(`/agents/${encodeURIComponent(row.name)}`);
        }
        return;
      }
      // Space toggles peek for the focused row
      if (e.key === " ") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          setPeekAgent((prev) => (prev === row.name ? null : row.name));
        }
        return;
      }
      // x toggles selection on the focused row
      if (e.key === "x") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(row.name)) next.delete(row.name);
            else next.add(row.name);
            return next;
          });
        }
        return;
      }
      // a selects all visible
      if (e.key === "a") {
        e.preventDefault();
        setSelected((prev) => {
          const next = new Set(prev);
          const names = displayRows.map((r) => r.name);
          const allSel = names.every((n) => next.has(n));
          if (allSel) {
            for (const n of names) next.delete(n);
          } else {
            for (const n of names) next.add(n);
          }
          return next;
        });
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => { window.removeEventListener("keydown", onKeyDown); };
  }, [selected.size, displayRows, focusIndex, navigate]);

  // Bulk action helpers
  const visibleNames = filteredAgents.map((a) => a.name);
  const allVisibleSelected = visibleNames.length > 0 && visibleNames.every((n) => selected.has(n));
  const toggleAllVisible = () => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const n of visibleNames) next.delete(n);
      } else {
        for (const n of visibleNames) next.add(n);
      }
      return next;
    });
  };
  const toggleOne = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };
  const summarizeResults = (results: BulkResult[]): string | null => {
    const failed = results.filter((r) => r.status === "error");
    if (failed.length === 0) return null;
    return `${String(failed.length)}/${String(results.length)} failed: ${failed.slice(0, 3).map((f) => `${f.agent} (${f.error ?? "error"})`).join(", ")}`;
  };
  const runBulk = async (fn: () => Promise<{ results: BulkResult[] }>) => {
    setBulkBusy(true);
    setBulkError(null);
    try {
      const { results } = await fn();
      const err = summarizeResults(results);
      if (err) setBulkError(err);
      refresh();
    } catch (e) {
      setBulkError(e instanceof Error ? e.message : "Bulk operation failed");
    } finally {
      setBulkBusy(false);
    }
  };
  const handleBulkStart = () => runBulk(() => api.bulkStartAgents(Array.from(selected)));
  const handleBulkStop = () => runBulk(() => api.bulkStopAgents(Array.from(selected)));
  const handleBulkDelete = () => {
    if (!window.confirm(`Delete ${String(selected.size)} agent(s)? This cannot be undone.`)) return;
    void runBulk(() => api.bulkDeleteAgents(Array.from(selected), false)).then(() => {
      setSelected(new Set());
    });
  };
  const handleBulkMessage = () => {
    const msg = window.prompt(`Send message to ${String(selected.size)} agent(s):`);
    if (msg == null || msg.trim() === "") return;
    void runBulk(() => api.bulkMessageAgents(Array.from(selected), msg.trim()));
  };
  const clearSelection = () => { setSelected(new Set()); };
  const clearFilters = () => {
    setSearch("");
    setSearchParams(new URLSearchParams(), { replace: true });
  };
  const hasFilters = search !== "" || roleFilter !== "" || stateFilter !== "" || toolFilter !== "";

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

  return (
    <div className="p-6 space-y-4 pb-24">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">Agents</h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-bc-muted">
            {hasFilters
              ? `${String(filteredAgents.length)} of ${String(allAgents.length)} agents`
              : `${String(allAgents.length)} agents`}
          </span>
          {allAgents.some(
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

      <CreateAgentForm onCreated={refresh} existingAgents={allAgents} />

      {/* Search + filter toolbar */}
      {allAgents.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-[200px]">
            <input
              ref={searchInputRef}
              type="text"
              value={search}
              onChange={(e) => { setSearch(e.target.value); }}
              placeholder="Search by name or task...  (press / to focus)"
              className="w-full px-3 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text placeholder:text-bc-muted/60 focus:outline-none focus:ring-1 focus:ring-bc-accent"
              aria-label="Search agents"
            />
          </div>
          <select
            value={roleFilter}
            onChange={(e) => { updateFilter("role", e.target.value); }}
            className="px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
            aria-label="Filter by role"
          >
            <option value="">All roles</option>
            {availableRoles.map((r) => (
              <option key={r} value={r}>{r}</option>
            ))}
          </select>
          <select
            value={stateFilter}
            onChange={(e) => { updateFilter("state", e.target.value); }}
            className="px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
            aria-label="Filter by state"
          >
            <option value="">All states</option>
            {availableStates.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <select
            value={toolFilter}
            onChange={(e) => { updateFilter("tool", e.target.value); }}
            className="px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent"
            aria-label="Filter by tool"
          >
            <option value="">All tools</option>
            {availableTools.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="px-2 py-1.5 text-xs text-bc-muted hover:text-bc-text border border-bc-border rounded focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg"
              aria-label="Clear filters"
            >
              Clear
            </button>
          )}
        </div>
      )}

      {/* Keyboard hints — only shown when the list is visible */}
      {allAgents.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[10px] text-bc-muted/60">
          <KeyHint k="/" label="search" />
          <KeyHint k="j / k" label="nav" />
          <KeyHint k="↵" label="open" />
          <KeyHint k="space" label="peek" />
          <KeyHint k="x" label="select" />
          <KeyHint k="a" label="select all" />
          <KeyHint k="esc" label="clear" />
        </div>
      )}

      <div className="rounded border border-bc-border overflow-x-auto">
        {allAgents.length === 0 ? (
          <EmptyState
            icon=">"
            title="No agents yet"
            description="Create your first agent using the form above."
          />
        ) : filteredAgents.length === 0 ? (
          <EmptyState
            icon=">"
            title="No agents match your filters"
            description="Try adjusting your search or clearing the filters."
            actionLabel="Clear filters"
            onAction={clearFilters}
          />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-bc-border text-left">
                <th className="px-2 py-2 font-medium text-bc-muted w-8">
                  <input
                    type="checkbox"
                    checked={allVisibleSelected}
                    onChange={toggleAllVisible}
                    className="cursor-pointer accent-bc-accent"
                    aria-label="Select all visible agents"
                  />
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted">Name</th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden sm:table-cell">
                  Runtime
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden sm:table-cell">
                  Provider
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted">Status</th>
                <th className="px-4 py-2 font-medium text-bc-muted">Task</th>
                <th className="px-4 py-2 font-medium text-bc-muted hidden md:table-cell">
                  MCP
                </th>
                <th className="px-4 py-2 font-medium text-bc-muted">Actions</th>
                <th className="px-4 py-2 font-medium text-bc-muted w-10"></th>
              </tr>
            </thead>
            <tbody>
              {displayRows.map((a, rowIdx) => (
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
                    className={`border-b border-bc-border/50 cursor-pointer transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg ${
                      rowIdx === focusIndex ? "ring-1 ring-inset ring-bc-accent/40 " : ""
                    }${
                      peekAgent === a.name ? "bg-bc-accent/5 " : ""
                    }${
                      selected.has(a.name) ? "bg-bc-accent/10 hover:bg-bc-accent/15" : "hover:bg-bc-surface"
                    }`}
                  >
                    <td
                      className="px-2 py-2"
                      onClick={(e) => { e.stopPropagation(); }}
                    >
                      <input
                        type="checkbox"
                        checked={selected.has(a.name)}
                        onChange={() => { toggleOne(a.name); }}
                        className="cursor-pointer accent-bc-accent"
                        aria-label={`Select agent ${a.name}`}
                      />
                    </td>
                    <td className="px-4 py-2">
                      <InlineAgentName agent={a} onRenamed={refresh} />
                    </td>
                    <td className="px-4 py-2 hidden sm:table-cell">
                      {a.runtime_backend ? (
                        <span
                          className="text-[11px] px-1.5 py-0.5 rounded border border-bc-border/30 bg-bc-surface/30 text-bc-muted"
                          style={{ fontFamily: "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
                        >
                          {a.runtime_backend}
                        </span>
                      ) : (
                        <span className="text-bc-muted">{"\u2014"}</span>
                      )}
                    </td>
                    <td className="px-4 py-2 hidden sm:table-cell">
                      {a.tool ? (
                        <span
                          className="text-[11px] px-1.5 py-0.5 rounded border border-bc-border/30 bg-bc-surface/30 text-bc-muted"
                          style={{ fontFamily: "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
                        >
                          {a.tool}
                        </span>
                      ) : (
                        <span className="text-bc-muted">{"\u2014"}</span>
                      )}
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
                      {(() => {
                        const servers = a.mcp_servers ?? [];
                        if (servers.length === 0) {
                          return <span className="text-bc-muted">{"\u2014"}</span>;
                        }
                        const fullList = servers.map((s) => s.replace(/^mcp__/, "")).join(", ");
                        if (servers.length <= 3) {
                          return (
                            <div className="flex flex-wrap gap-1" title={fullList}>
                              {servers.map((s) => (
                                <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-bc-accent/10 text-bc-accent font-medium">
                                  {s.replace(/^mcp__/, "")}
                                </span>
                              ))}
                            </div>
                          );
                        }
                        const rest = servers.slice(2).map((s) => s.replace(/^mcp__/, "")).join(", ");
                        return (
                          <div className="flex flex-wrap gap-1" title={fullList}>
                            {servers.slice(0, 2).map((s) => (
                              <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-bc-accent/10 text-bc-accent font-medium">
                                {s.replace(/^mcp__/, "")}
                              </span>
                            ))}
                            <span
                              className="text-[10px] px-1.5 py-0.5 rounded border border-bc-border text-bc-muted cursor-help"
                              title={rest}
                            >
                              +{String(servers.length - 2)}
                            </span>
                          </div>
                        );
                      })()}
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

      {/* Bulk action bar */}
      {selected.size > 0 && (
        <div className="fixed left-0 right-0 bottom-0 z-40 border-t border-bc-border bg-bc-surface/95 backdrop-blur shadow-bc-lg">
          <div className="max-w-6xl mx-auto px-6 py-3 flex items-center gap-3 flex-wrap">
            <span className="text-sm font-medium text-bc-text">
              {selected.size} selected
            </span>
            {bulkError && (
              <span className="text-xs text-bc-error truncate max-w-md" title={bulkError}>
                {bulkError}
              </span>
            )}
            <div className="flex items-center gap-2 ml-auto">
              <button
                onClick={handleBulkStart}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-bc-success/20 text-bc-success hover:bg-bc-success/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
                aria-label="Start selected agents"
              >
                {bulkBusy ? "..." : "Start"}
              </button>
              <button
                onClick={handleBulkStop}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-bc-warning/20 text-bc-warning hover:bg-bc-warning/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
                aria-label="Stop selected agents"
              >
                {bulkBusy ? "..." : "Stop"}
              </button>
              <button
                onClick={handleBulkMessage}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-bc-accent/20 text-bc-accent hover:bg-bc-accent/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
                aria-label="Send message to selected agents"
              >
                {bulkBusy ? "..." : "Message"}
              </button>
              <button
                onClick={handleBulkDelete}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-bc-error/20 text-bc-error hover:bg-bc-error/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
                aria-label="Delete selected agents"
              >
                {bulkBusy ? "..." : "Delete"}
              </button>
              <button
                onClick={clearSelection}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded border border-bc-border text-bc-muted hover:text-bc-text disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-bc-accent"
                aria-label="Clear selection"
              >
                Clear
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
