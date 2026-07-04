import { useCallback, useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { motion } from "framer-motion";
import { api } from "../api/client";
import type {
  ProviderDetailResponse,
  ProviderCommand,
  ProviderMCPServer,
} from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { StatusBadge } from "../components/StatusBadge";
import { CopyButton } from "../components/CopyButton";
import { ToastContainer, useToast } from "../components/Toast";
import { formatCost, formatTokens } from "../utils/format";

const inputCls =
  "w-full px-2.5 py-1.5 text-sm rounded border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent";

const HEALTHY_STATUSES = new Set(["ok", "active", "connected"]);

function isHealthy(status: string): boolean {
  return HEALTHY_STATUSES.has(status);
}

function providerStatus(provider: ProviderDetailResponse): string {
  if (!provider.installed) return "error";
  if (provider.agent_count > 0) return "running";
  return "stopped";
}


/* ── Section: Header ── */

function ProviderHeader({
  provider,
  onInstall,
  onUpdate,
  installing,
  updating,
}: {
  provider: ProviderDetailResponse;
  onInstall: () => void;
  onUpdate: () => void;
  installing: boolean;
  updating: boolean;
}) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div className="flex items-center gap-3">
        <Link
          to="/tools"
          className="text-mycel-muted hover:text-mycel-text text-sm shrink-0"
        >
          &larr; Tools
        </Link>
        {/* Monogram */}
        <div className="w-9 h-9 rounded-full bg-mycel-accent-subtle flex items-center justify-center shrink-0">
          <span className="text-sm font-bold text-mycel-accent">
            {provider.name.charAt(0).toUpperCase()}
          </span>
        </div>
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-bold">{provider.name}</h1>
            <StatusBadge status={providerStatus(provider)} />
          </div>
          {provider.version && (
            <span className="inline-block mt-0.5 px-2 py-0.5 rounded text-xs font-mono bg-mycel-surface border border-mycel-border text-mycel-muted">
              v{provider.version}
            </span>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        {!provider.installed && provider.install_hint && (
          <button
            type="button"
            onClick={onInstall}
            disabled={installing}
            className="px-3 py-1.5 text-sm rounded bg-mycel-warning-subtle text-mycel-warning transition-colors disabled:opacity-50"
          >
            {installing ? "Installing..." : "Install"}
          </button>
        )}
        {provider.installed && provider.install_hint && (
          <button
            type="button"
            onClick={onUpdate}
            disabled={updating}
            className="px-3 py-1.5 text-sm rounded bg-mycel-info-subtle text-mycel-info transition-colors disabled:opacity-50"
          >
            {updating ? "Checking..." : "Check for Update"}
          </button>
        )}
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium ${
            provider.enabled
              ? "bg-mycel-success-subtle text-mycel-success"
              : "bg-mycel-surface-hover text-mycel-text-2"
          }`}
        >
          <span
            className={`w-2 h-2 rounded-full ${provider.enabled ? "bg-mycel-success" : "bg-mycel-muted"}`}
          />
          {provider.enabled ? "Enabled" : "Disabled"}
        </span>
      </div>
    </div>
  );
}

/* ── Section: Configuration ── */

function ConfigPanel({
  provider,
  onSave,
}: {
  provider: ProviderDetailResponse;
  onSave: (config: Record<string, string>) => Promise<void>;
}) {
  const [command, setCommand] = useState(provider.config?.command ?? provider.command ?? "");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    setCommand(provider.config?.command ?? provider.command ?? "");
    setDirty(false);
  }, [provider.config, provider.command]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave({ command });
      setDirty(false);
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    setCommand(provider.binary ?? provider.name);
    setDirty(true);
  };

  return (
    <section>
      <h2 className="text-xs font-medium text-mycel-muted uppercase tracking-widest mb-3">
        Configuration
      </h2>
      <div className="rounded border border-mycel-border bg-mycel-surface p-4 space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="text-xs text-mycel-muted block mb-1">Binary</label>
            <div className="text-sm font-mono text-mycel-text-2 px-2.5 py-1.5 rounded bg-mycel-bg border border-mycel-border">
              {provider.binary || "\u2014"}
            </div>
          </div>
          <div>
            <label className="text-xs text-mycel-muted block mb-1">Command</label>
            <input
              type="text"
              value={command}
              onChange={(e) => {
                setCommand(e.target.value);
                setDirty(true);
              }}
              className={inputCls}
            />
          </div>
          <div>
            <label className="text-xs text-mycel-muted block mb-1">Description</label>
            <div className="text-sm text-mycel-text-2 px-2.5 py-1.5 rounded bg-mycel-bg border border-mycel-border">
              {provider.description || "\u2014"}
            </div>
          </div>
          <div>
            <label className="text-xs text-mycel-muted block mb-1">Install Hint</label>
            <div className="flex items-center gap-1">
              <div className="flex-1 text-sm font-mono text-mycel-text-2 px-2.5 py-1.5 rounded bg-mycel-bg border border-mycel-border truncate">
                {provider.install_hint || "\u2014"}
              </div>
              {provider.install_hint && <CopyButton text={provider.install_hint} />}
            </div>
          </div>
        </div>
        {provider.config?.default === "true" && (
          <div className="text-xs text-mycel-accent">
            This is the default provider.
          </div>
        )}
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => void handleSave()}
            disabled={saving || !dirty}
            className="px-3 py-1.5 text-sm rounded bg-mycel-accent text-mycel-bg font-medium disabled:opacity-50 transition-colors"
          >
            {saving ? "Saving..." : "Save Configuration"}
          </button>
          <button
            type="button"
            onClick={handleReset}
            disabled={saving}
            className="px-3 py-1.5 text-sm rounded border border-mycel-border text-mycel-muted hover:text-mycel-text transition-colors disabled:opacity-50"
          >
            Reset to Default
          </button>
        </div>
      </div>
    </section>
  );
}

/* ── Section: MCP Servers ── */

type MCPHealthStatus = "connected" | "error" | "unknown";

function MCPHealthBadge({ status, error }: { status: MCPHealthStatus; error?: string }) {
  const styles: Record<MCPHealthStatus, { bg: string; text: string; dot: string; label: string }> = {
    connected: { bg: "bg-mycel-success-subtle", text: "text-mycel-success", dot: "bg-mycel-success", label: "Connected" },
    error:     { bg: "bg-mycel-error-subtle",   text: "text-mycel-error",   dot: "bg-mycel-error",   label: "Error" },
    unknown:   { bg: "bg-mycel-warning-subtle", text: "text-mycel-warning", dot: "bg-mycel-warning", label: "Unknown" },
  };
  const s = styles[status];
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium ${s.bg} ${s.text}`}
      title={error || undefined}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${s.dot}`} />
      {s.label}
    </span>
  );
}

function resolveMCPHealth(server: ProviderMCPServer, healthMap: Record<string, { status: string; error?: string }>): { status: MCPHealthStatus; error?: string } {
  const checked = healthMap[server.name];
  if (checked) {
    if (isHealthy(checked.status)) return { status: "connected" };
    return { status: "error", error: checked.error || checked.status };
  }
  if (server.status) {
    const s = server.status.toLowerCase();
    if (s === "error" || s === "failed") return { status: "error", error: server.error };
    return { status: "unknown" };
  }
  return { status: "unknown" };
}

/* ── MCP Health Summary Bar ── */
function MCPHealthSummary({ servers, healthMap }: { servers: ProviderMCPServer[]; healthMap: Record<string, { status: string; error?: string }> }) {
  if (servers.length === 0) return null;

  const healthy = servers.filter((s) => {
    const h = resolveMCPHealth(s, healthMap);
    return h.status === "connected";
  }).length;
  const pct = Math.round((healthy / servers.length) * 100);

  return (
    <div className="mb-3">
      <div className="flex items-center justify-between text-xs text-mycel-muted mb-1">
        <span>MCP Health</span>
        <span>{healthy}/{servers.length} healthy</span>
      </div>
      <div className="h-1.5 rounded-full bg-mycel-border overflow-hidden">
        <motion.div
          className={`h-full rounded-full ${pct === 100 ? "bg-mycel-success" : pct > 0 ? "bg-mycel-warning" : "bg-mycel-error"}`}
          initial={{ width: 0 }}
          animate={{ width: `${pct}%` }}
          transition={{ duration: 0.5, ease: "easeOut" }}
        />
      </div>
    </div>
  );
}

function MCPSection({
  providerName,
  servers,
  onRefresh,
  onToast,
}: {
  providerName: string;
  servers: ProviderMCPServer[];
  onRefresh: () => void;
  onToast: (level: "success" | "error" | "info", msg: string) => void;
}) {
  const [showAdd, setShowAdd] = useState(false);
  const [mcpName, setMcpName] = useState("");
  const [mcpTransport, setMcpTransport] = useState<"stdio" | "sse">("stdio");
  const [mcpValue, setMcpValue] = useState("");
  const [adding, setAdding] = useState(false);
  const [checking, setChecking] = useState(false);
  const [healthMap, setHealthMap] = useState<Record<string, { status: string; error?: string }>>({});

  const handleAdd = async () => {
    if (!mcpName.trim() || !mcpValue.trim()) return;
    setAdding(true);
    try {
      await api.addProviderMCP(providerName, {
        name: mcpName.trim(),
        transport: mcpTransport,
        ...(mcpTransport === "sse" ? { url: mcpValue.trim() } : { command: mcpValue.trim() }),
      });
      onToast("success", `MCP '${mcpName.trim()}' added`);
      setMcpName("");
      setMcpValue("");
      setShowAdd(false);
      onRefresh();
    } catch (err) {
      onToast("error", err instanceof Error ? err.message : "Failed to add MCP");
    } finally {
      setAdding(false);
    }
  };

  const handleCheckAll = async () => {
    setChecking(true);
    try {
      const tools = await api.checkTools();
      const mcpTools = tools.filter((t) => t.type === "mcp");
      const newMap: Record<string, { status: string; error?: string }> = {};
      for (const t of mcpTools) {
        newMap[t.name] = { status: t.status, error: t.error };
      }
      setHealthMap(newMap);
      const unhealthy = mcpTools.filter((t) => !isHealthy(t.status));
      if (unhealthy.length === 0) {
        onToast("success", `All ${mcpTools.length} MCP server(s) healthy`);
      } else {
        onToast("error", `${unhealthy.length} of ${mcpTools.length} MCP server(s) have issues`);
      }
    } catch (err) {
      onToast("error", err instanceof Error ? err.message : "Health check failed");
    } finally {
      setChecking(false);
    }
  };

  return (
    <section>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-xs font-medium text-mycel-muted uppercase tracking-widest">
          MCP Servers ({servers.length})
        </h2>
        <div className="flex items-center gap-2">
          {servers.length > 0 && (
            <button
              type="button"
              onClick={() => void handleCheckAll()}
              disabled={checking}
              className="text-xs px-2 py-1 rounded bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-50"
            >
              {checking ? "Checking..." : "Check All MCPs"}
            </button>
          )}
          <button
            type="button"
            onClick={() => setShowAdd(!showAdd)}
            className="text-xs px-2 py-1 rounded bg-mycel-info-subtle text-mycel-info transition-colors"
          >
            {showAdd ? "Cancel" : "+ Add MCP"}
          </button>
        </div>
      </div>

      <MCPHealthSummary servers={servers} healthMap={healthMap} />

      {showAdd && (
        <div className="rounded border border-mycel-accent bg-mycel-surface p-4 space-y-3 mb-4">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label className="text-xs text-mycel-muted block mb-1">Name</label>
              <input
                type="text"
                value={mcpName}
                onChange={(e) => setMcpName(e.target.value)}
                placeholder="my-mcp"
                className={inputCls}
              />
            </div>
            <div>
              <label className="text-xs text-mycel-muted block mb-1">Transport</label>
              <select
                value={mcpTransport}
                onChange={(e) => setMcpTransport(e.target.value as "stdio" | "sse")}
                className={inputCls}
              >
                <option value="stdio">stdio</option>
                <option value="sse">SSE</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-mycel-muted block mb-1">
                {mcpTransport === "sse" ? "URL" : "Command"}
              </label>
              <input
                type="text"
                value={mcpValue}
                onChange={(e) => setMcpValue(e.target.value)}
                placeholder={mcpTransport === "sse" ? "http://localhost:3000/sse" : "npx my-mcp"}
                className={inputCls}
              />
            </div>
          </div>
          <button
            type="button"
            onClick={() => void handleAdd()}
            disabled={adding || !mcpName.trim() || !mcpValue.trim()}
            className="px-3 py-1.5 text-sm rounded bg-mycel-accent text-mycel-bg font-medium disabled:opacity-50"
          >
            {adding ? "Adding..." : "Add MCP Server"}
          </button>
        </div>
      )}

      {servers.length === 0 ? (
        <EmptyState
          icon="~"
          title="No MCP servers"
          description={`No MCP servers configured for ${providerName}.`}
        />
      ) : (
        <div className="rounded border border-mycel-border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-mycel-border bg-mycel-surface text-[11px] text-mycel-muted uppercase tracking-wider">
                <th className="px-4 py-2 font-medium text-left">Name</th>
                <th className="px-4 py-2 font-medium text-left">Transport</th>
                <th className="px-4 py-2 font-medium text-left">URL / Command</th>
                <th className="px-4 py-2 font-medium text-left">Status</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s) => {
                const health = resolveMCPHealth(s, healthMap);
                return (
                  <tr key={s.name} className="border-b border-mycel-border hover:bg-mycel-surface-hover transition-colors">
                    <td className="px-4 py-2.5 font-medium">{s.name}</td>
                    <td className="px-4 py-2.5">
                      <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-mycel-surface border border-mycel-border">
                        {s.transport}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-mycel-muted truncate max-w-xs">
                      <div className="flex items-center gap-1">
                        <span className="truncate">{s.url || s.command || "\u2014"}</span>
                        {(s.url || s.command) && <CopyButton text={s.url || s.command || ""} />}
                      </div>
                    </td>
                    <td className="px-4 py-2.5">
                      <MCPHealthBadge status={health.status} error={health.error} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/* ── Stat Tile ── */
function StatTile({ label, value, icon, accent }: { label: string; value: string; icon: React.ReactNode; accent?: boolean }) {
  return (
    <div className="rounded border border-mycel-border bg-mycel-surface p-3">
      <div className="flex items-center gap-2 mb-1">
        <span className="text-mycel-muted">{icon}</span>
        <span className="text-[11px] text-mycel-muted uppercase tracking-wider">{label}</span>
      </div>
      <p className={`text-lg font-bold ${accent ? "text-mycel-accent" : ""}`}>{value}</p>
    </div>
  );
}

/* ── StatBar (4 tiles) ── */
function StatBar({ provider }: { provider: ProviderDetailResponse }) {
  const models = provider.cost_by_model ?? [];
  return (
    <div className="grid grid-cols-2 gap-3">
      <StatTile
        label="Cost"
        value={formatCost(provider.total_cost_usd)}
        accent
        icon={
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1" />
          </svg>
        }
      />
      <StatTile
        label="Tokens"
        value={formatTokens(provider.total_tokens)}
        icon={
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
          </svg>
        }
      />
      <StatTile
        label="Agents"
        value={String(provider.agent_count)}
        icon={
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
          </svg>
        }
      />
      <StatTile
        label="Models"
        value={String(models.length)}
        icon={
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
          </svg>
        }
      />
    </div>
  );
}

/* ── Cost Bars ── */
function CostBars({ provider }: { provider: ProviderDetailResponse }) {
  const models = provider.cost_by_model ?? [];
  if (models.length === 0) return null;

  const maxCost = Math.max(...models.map((m) => m.total_cost_usd), 0.01);

  return (
    <section>
      <h3 className="text-xs font-medium text-mycel-muted uppercase tracking-widest mb-3">
        Cost by Model
      </h3>
      <div className="space-y-2">
        {models.map((m) => {
          const pct = Math.max((m.total_cost_usd / maxCost) * 100, 2);
          return (
            <div key={m.model}>
              <div className="flex items-center justify-between text-xs mb-0.5">
                <span className="font-mono text-mycel-text truncate mr-2">{m.model}</span>
                <span className="text-mycel-muted tabular-nums shrink-0">{formatCost(m.total_cost_usd)}</span>
              </div>
              <div className="h-1.5 rounded-full bg-mycel-border overflow-hidden">
                <div
                  className="h-full rounded-full bg-mycel-accent transition-all duration-500 ease-out"
                  style={{ width: `${pct}%` }}
                />
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

/* ── Section: Agents (sidebar style) ── */

function AgentsSidebar({
  agents,
}: {
  agents: ProviderDetailResponse["agents"];
}) {
  return (
    <section>
      <h3 className="text-xs font-medium text-mycel-muted uppercase tracking-widest mb-3">
        Agents ({agents.length})
      </h3>
      {agents.length === 0 ? (
        <p className="text-xs text-mycel-muted">No agents using this provider.</p>
      ) : (
        <div className="space-y-1">
          {agents.map((a) => {
            const isRunning = a.state === "running" || a.state === "working";
            return (
              <Link
                key={a.name}
                to={`/agents/${encodeURIComponent(a.name)}`}
                className="group flex items-center gap-2 px-3 py-2 rounded border border-mycel-border hover:border-mycel-accent hover:bg-mycel-surface-hover transition-colors"
              >
                {/* Pulse dot */}
                <span className="relative flex h-2 w-2 shrink-0">
                  {isRunning && (
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-mycel-success opacity-75" />
                  )}
                  <span
                    className={`relative inline-flex rounded-full h-2 w-2 ${
                      isRunning ? "bg-mycel-success" : "bg-mycel-muted"
                    }`}
                  />
                </span>
                <div className="flex-1 min-w-0">
                  <span className="text-sm font-medium text-mycel-text truncate block">{a.name}</span>
                  <span className="text-[10px] text-mycel-muted">{a.role}</span>
                </div>
                {/* Hover arrow */}
                <svg
                  className="w-3.5 h-3.5 text-mycel-muted opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </Link>
            );
          })}
        </div>
      )}
    </section>
  );
}

/* ── Section: Commands ── */

function CommandsSection({ commands }: { commands: ProviderCommand[] }) {
  return (
    <section>
      <h2 className="text-xs font-medium text-mycel-muted uppercase tracking-widest mb-3">
        Available Commands ({commands.length})
      </h2>
      {commands.length === 0 ? (
        <EmptyState
          icon=">"
          title="No commands"
          description="No commands available for this provider."
        />
      ) : (
        <div className="rounded border border-mycel-border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-mycel-border bg-mycel-surface text-[11px] text-mycel-muted uppercase tracking-wider">
                <th className="px-4 py-2 font-medium text-left">Command</th>
                <th className="px-4 py-2 font-medium text-left">Description</th>
                <th className="px-4 py-2 font-medium text-left">Usage</th>
              </tr>
            </thead>
            <tbody>
              {commands.map((c) => {
                const fullCmd = c.args ? `${c.command} ${c.args}` : c.command;
                return (
                  <tr key={c.name} className="border-b border-mycel-border hover:bg-mycel-surface-hover transition-colors">
                    <td className="px-4 py-2.5 font-medium">{c.name}</td>
                    <td className="px-4 py-2.5 text-mycel-muted">{c.description}</td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-1">
                        <code className="font-mono text-xs text-mycel-text-2">
                          {c.command}
                          {c.args && <span className="text-mycel-muted ml-1">{c.args}</span>}
                        </code>
                        <CopyButton text={fullCmd} />
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/* ── Main Component ── */

export function ProviderDetail() {
  const { provider: providerName } = useParams<{ provider: string }>();
  const { toasts, addToast, dismiss } = useToast();
  const [installing, setInstalling] = useState(false);
  const [updating, setUpdating] = useState(false);

  const detailFetcher = useCallback(async () => {
    if (!providerName) throw new Error("No provider name");
    return api.getProvider(providerName);
  }, [providerName]);
  const {
    data: provider,
    loading,
    error,
    refresh,
  } = usePolling<ProviderDetailResponse>(detailFetcher, 10000);

  const cmdFetcher = useCallback(async () => {
    if (!providerName) throw new Error("No provider name");
    return api.getProviderCommands(providerName);
  }, [providerName]);
  const { data: commands } = usePolling<ProviderCommand[]>(cmdFetcher, 30000);

  const mcpFetcher = useCallback(async () => {
    if (!providerName) throw new Error("No provider name");
    return api.getProviderMCPs(providerName);
  }, [providerName]);
  const { data: mcpServers, refresh: refreshMCPs } = usePolling<ProviderMCPServer[]>(mcpFetcher, 15000);

  const handleInstall = async () => {
    if (!providerName) return;
    setInstalling(true);
    try {
      const result = await api.installProvider(providerName);
      addToast("info", `Install: ${result.install_cmd}`);
      refresh();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Install failed");
    } finally {
      setInstalling(false);
    }
  };

  const handleUpdate = async () => {
    if (!providerName) return;
    setUpdating(true);
    try {
      const result = await api.checkProviderUpdate(providerName);
      if (result.update_available) {
        addToast("info", `Update available: ${result.latest_version} (current: ${result.current_version})`);
      } else {
        addToast("success", `Already on latest version: ${result.current_version}`);
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Update check failed");
    } finally {
      setUpdating(false);
    }
  };

  const handleSaveConfig = async (config: Record<string, string>) => {
    if (!providerName) return;
    try {
      await api.updateProviderConfig(providerName, config);
      addToast("success", "Configuration saved");
      refresh();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to save config");
      throw err;
    }
  };

  if (loading && !provider) {
    return (
      <div className="p-6 space-y-6">
        <div className="h-6 w-48 animate-pulse rounded bg-mycel-surface-hover" />
        <LoadingSkeleton variant="cards" rows={3} />
      </div>
    );
  }

  if (error && !provider) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load provider"
          description={error}
          actionLabel="Back to Tools"
          onAction={() => window.history.back()}
        />
      </div>
    );
  }

  if (!provider) return null;

  return (
    <div className="p-6 space-y-8">
      <ProviderHeader
        provider={provider}
        onInstall={() => void handleInstall()}
        onUpdate={() => void handleUpdate()}
        installing={installing}
        updating={updating}
      />

      {/* 2-column layout on desktop */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left column: Config + MCP + Commands */}
        <div className="lg:col-span-2 space-y-8">
          <ConfigPanel provider={provider} onSave={handleSaveConfig} />

          <MCPSection
            providerName={provider.name}
            servers={mcpServers ?? []}
            onRefresh={refreshMCPs}
            onToast={addToast}
          />

          <CommandsSection commands={commands ?? []} />
        </div>

        {/* Right column: Stats + Cost bars + Agents */}
        <div className="space-y-6">
          <StatBar provider={provider} />
          <CostBars provider={provider} />
          <AgentsSidebar agents={provider.agents ?? []} />
        </div>
      </div>

      <ToastContainer toasts={toasts} onDismiss={dismiss} />
    </div>
  );
}
