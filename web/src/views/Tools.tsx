import { useCallback, useMemo, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../api/client";
import type { Tool } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { ProvidersTable } from "../components/ProvidersTable";
import { CopyButton } from "../components/CopyButton";
import { ToastContainer, useToast } from "../components/Toast";
import type { ToastLevel } from "../components/Toast";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
const STATUS_CONFIG: Record<string, { dot: string; label: string; textColor: string }> = {
  connected:     { dot: "bg-bc-success", label: "Connected",     textColor: "text-bc-success" },
  configured:    { dot: "bg-bc-success", label: "Configured",    textColor: "text-bc-success" },
  installed:     { dot: "bg-bc-success", label: "Installed",     textColor: "text-bc-success" },
  disabled:      { dot: "bg-bc-muted",   label: "Disabled",      textColor: "text-bc-muted" },
  not_installed: { dot: "bg-bc-error",   label: "Not installed", textColor: "text-bc-error" },
  error:         { dot: "bg-bc-error",   label: "Error",         textColor: "text-bc-error" },
  unknown:       { dot: "bg-bc-muted",   label: "Unknown",       textColor: "text-bc-muted" },
};

const inputCls = "w-full px-2 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent";

function getStatusConfig(s: string) { return STATUS_CONFIG[s] ?? STATUS_CONFIG.unknown!; }

/* ── Animated chevron icon ── */
function ChevronIcon({ expanded }: { expanded: boolean }) {
  return (
    <motion.svg
      animate={{ rotate: expanded ? 90 : 0 }}
      transition={{ type: "spring", stiffness: 300, damping: 20 }}
      className="w-3 h-3 text-bc-muted"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeWidth={2.5}
    >
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
    </motion.svg>
  );
}

function CLIDepsRow({ tool, onToggle, onRemove, toggling, removing, expanded, onExpand }: {
  tool: Tool; onToggle: () => void; onRemove: () => void;
  toggling: boolean; removing: boolean; expanded: boolean; onExpand: () => void;
}) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const cfg = getStatusConfig(tool.status);
  const isDisabled = tool.status === "disabled";

  return (
    <>
      <tr
        className="border-b border-bc-border hover:bg-bc-surface/50 cursor-pointer transition-colors"
        onClick={onExpand}
      >
        <td className="px-3 py-2 text-sm">
          <div className="flex items-center gap-2">
            <ChevronIcon expanded={expanded} />
            <span className="font-medium">{tool.name}</span>
          </div>
        </td>
        <td className="px-3 py-2 text-sm">
          <span className="inline-flex items-center gap-1.5">
            <span className={`w-2 h-2 rounded-full ${cfg.dot}`} />
            <span className={`text-xs ${cfg.textColor}`}>{tool.version || cfg.label}</span>
          </span>
        </td>
        <td className="px-3 py-2 text-xs text-bc-muted font-mono">{tool.version || "\u2014"}</td>
        <td className="px-3 py-2 text-xs">
          {tool.required ? (
            <span className="px-1.5 py-0.5 rounded bg-bc-accent/10 text-bc-accent text-[10px] font-medium">Yes</span>
          ) : (
            <span className="text-bc-muted">No</span>
          )}
        </td>
        <td className="px-3 py-2 text-right" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center justify-end gap-1.5">
            <button type="button" onClick={onToggle} disabled={toggling}
              role="switch" aria-checked={!isDisabled}
              aria-label={isDisabled ? `Enable ${tool.name}` : `Disable ${tool.name}`}
              className={`text-[11px] px-2 py-0.5 rounded transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent disabled:opacity-50 ${isDisabled ? "bg-bc-border text-bc-muted hover:bg-bc-border/80" : "bg-bc-success/10 text-bc-success hover:bg-bc-success/20"}`}>
              {toggling ? "..." : isDisabled ? "Enable" : "Disable"}
            </button>
            {!tool.required && (
              confirmRemove ? (
                <span className="inline-flex items-center gap-1">
                  <button type="button" onClick={() => { onRemove(); setConfirmRemove(false); }} disabled={removing}
                    className="text-[11px] px-2 py-0.5 rounded bg-bc-error/20 text-bc-error disabled:opacity-50" aria-label="Confirm remove">
                    {removing ? "..." : "Yes"}
                  </button>
                  <button type="button" onClick={() => setConfirmRemove(false)} disabled={removing}
                    className="text-[11px] px-2 py-0.5 rounded border border-bc-border text-bc-muted" aria-label="Cancel remove">No</button>
                </span>
              ) : (
                <button type="button" onClick={() => setConfirmRemove(true)}
                  className="text-[11px] px-2 py-0.5 rounded border border-bc-border text-bc-muted hover:text-bc-error hover:border-bc-error/50 transition-colors"
                  aria-label={`Remove ${tool.name}`}>Remove</button>
              )
            )}
          </div>
        </td>
      </tr>
      <AnimatePresence>
        {expanded && (
          <tr className="border-b border-bc-border">
            <td colSpan={5} className="p-0">
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.2, ease: "easeOut" }}
                className="overflow-hidden bg-bc-surface/30"
              >
                <div className="px-8 py-3 space-y-2">
                  {tool.install_cmd && (
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-bc-muted shrink-0">Install:</span>
                      <code className="flex-1 text-xs font-mono text-bc-text bg-bc-bg rounded px-2 py-1 border border-bc-border/50">
                        {tool.install_cmd}
                      </code>
                      <CopyButton text={tool.install_cmd} />
                    </div>
                  )}
                  {tool.command && (
                    <>
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-bc-muted shrink-0">Version cmd:</span>
                        <code className="flex-1 text-xs font-mono text-bc-text bg-bc-bg rounded px-2 py-1 border border-bc-border/50">
                          {tool.command} --version
                        </code>
                        <CopyButton text={`${tool.command} --version`} />
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-bc-muted shrink-0">Path:</span>
                        <code className="flex-1 text-xs font-mono text-bc-text bg-bc-bg rounded px-2 py-1 border border-bc-border/50">
                          {tool.command}
                        </code>
                        <CopyButton text={tool.command} />
                      </div>
                    </>
                  )}
                  {tool.error && (
                    <div className="text-xs text-bc-error bg-bc-error/5 rounded px-2 py-1 border border-bc-error/20">
                      Error: {tool.error}
                    </div>
                  )}
                </div>
              </motion.div>
            </td>
          </tr>
        )}
      </AnimatePresence>
    </>
  );
}

function AddCLIToolForm({ onClose, onAdded, onToast }: { onClose: () => void; onAdded: () => void; onToast: (level: ToastLevel, text: string) => void }) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [installCmd, setInstallCmd] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await api.upsertTool({ name: name.trim(), command: command.trim(), install_cmd: installCmd.trim(), enabled: true });
      onToast("info", `Tool '${name.trim()}' added`);
      onAdded();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to add tool";
      setError(msg);
      onToast("error", msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: -8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8 }}
      transition={{ duration: 0.2 }}
      className="rounded border border-bc-accent bg-bc-surface p-4 space-y-3"
    >
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Add CLI Tool</h3>
        <button type="button" onClick={onClose} className="text-xs text-bc-muted hover:text-bc-text">Cancel</button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <label className="text-xs text-bc-muted block mb-1">Name</label>
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="gh" className={inputCls} />
        </div>
        <div>
          <label className="text-xs text-bc-muted block mb-1">Command</label>
          <input type="text" value={command} onChange={(e) => setCommand(e.target.value)} placeholder="gh" className={inputCls} />
        </div>
        <div className="md:col-span-2">
          <label className="text-xs text-bc-muted block mb-1">Install Command (optional)</label>
          <input type="text" value={installCmd} onChange={(e) => setInstallCmd(e.target.value)}
            placeholder="apt-get install -y gh" className={inputCls} />
        </div>
      </div>
      {error && <p className="text-xs text-bc-error">{error}</p>}
      <button type="button" onClick={() => void handleSubmit()} disabled={submitting || !name.trim()}
        className="px-3 py-1.5 text-sm rounded bg-bc-accent text-bc-bg font-medium disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-bc-accent">
        {submitting ? "Adding..." : "Add CLI Tool"}
      </button>
    </motion.div>
  );
}

/* ── Spinner icon ── */
function Spinner() {
  return (
    <svg className="animate-spin w-4 h-4 text-bc-muted" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  );
}

export function Tools() {
  useHeaderSlot({ title: <TabHeaderTitle>Tools</TabHeaderTitle> });

  const providerFetcher = useCallback(() => api.listProviders(), []);
  const { data: providers, loading: providersLoading } = usePolling(providerFetcher, 10000);

  const fetcher = useCallback(() => api.listTools(), []);
  const { data: tools, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);
  const [showAddForm, setShowAddForm] = useState(false);
  const [checking, setChecking] = useState(false);
  const [checkedTools, setCheckedTools] = useState<Tool[] | null>(null);
  const [optimisticToggles, setOptimisticToggles] = useState<Map<string, string>>(new Map());
  const [togglingSet, setTogglingSet] = useState<Set<string>>(new Set());
  const [removingSet, setRemovingSet] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const { toasts, addToast, dismiss } = useToast();

  const handleCheck = async () => {
    setChecking(true);
    try {
      const checked = await api.checkTools();
      const checkMap = new Map(checked.map((t) => [t.name, t]));
      setCheckedTools((tools ?? []).map((t) => {
        const c = checkMap.get(t.name);
        if (c) {
          return {
            ...t,
            status: c.status,
            version: c.version ?? t.version,
            command: c.command ?? t.command,
            error: c.error ?? t.error,
          };
        }
        return t;
      }));
      addToast("success", "Health check complete");
    } catch {
      addToast("error", "Health check failed");
    } finally {
      setChecking(false);
    }
  };

  const allTools = useMemo(() => {
    const source = checkedTools ?? tools ?? [];
    const seen = new Set<string>();
    const deduped: Tool[] = [];
    for (const t of source) {
      if (seen.has(t.name)) continue;
      seen.add(t.name);
      const optimistic = optimisticToggles.get(t.name);
      deduped.push(optimistic ? { ...t, status: optimistic } : t);
    }
    return deduped;
  }, [checkedTools, tools, optimisticToggles]);

  const searchLower = useMemo(() => search.toLowerCase().trim(), [search]);

  const { cliTools, filteredCli } = useMemo(() => {
    const matchesSearch = (t: Tool) => !searchLower || t.name.toLowerCase().includes(searchLower);
    const cli = allTools.filter((t) => t.type !== "provider" && t.type !== "mcp");
    return {
      cliTools: cli,
      filteredCli: cli.filter(matchesSearch),
    };
  }, [allTools, searchLower]);

  const providerList = providers ?? [];

  if (loading && !tools && providersLoading && !providers) {
    return (
      <div className="p-6 space-y-6">
        <div className="h-6 w-32 animate-pulse rounded bg-bc-border/50" />
        <LoadingSkeleton variant="cards" rows={4} />
      </div>
    );
  }
  if (timedOut && !tools) {
    return <div className="p-6"><EmptyState icon="!" title="Tools timed out" actionLabel="Retry" onAction={refresh} /></div>;
  }
  if (error && !tools) {
    return <div className="p-6"><EmptyState icon="!" title="Failed to load tools" description={error} actionLabel="Retry" onAction={refresh} /></div>;
  }

  const totalCount = providerList.length + allTools.length;
  const matchCount = providerList.filter((p) => !searchLower || p.name.toLowerCase().includes(searchLower)).length + filteredCli.length;

  const handleToggle = async (tool: Tool) => {
    const wasDisabled = tool.status === "disabled" || tool.status === "not_installed";
    const newStatus = wasDisabled ? "installed" : "disabled";
    const oldStatus = tool.status;

    setOptimisticToggles((prev) => new Map(prev).set(tool.name, newStatus));
    setTogglingSet((prev) => new Set(prev).add(tool.name));

    try {
      wasDisabled ? await api.enableTool(tool.name) : await api.disableTool(tool.name);
      addToast("success", `${tool.name} ${wasDisabled ? "enabled" : "disabled"}`);
      setCheckedTools(null);
      refresh();
    } catch (err) {
      setOptimisticToggles((prev) => new Map(prev).set(tool.name, oldStatus));
      const msg = err instanceof Error ? err.message : `Failed to toggle ${tool.name}`;
      addToast("error", msg);
    } finally {
      setTogglingSet((prev) => { const next = new Set(prev); next.delete(tool.name); return next; });
      setTimeout(() => {
        setOptimisticToggles((prev) => {
          const next = new Map(prev);
          next.delete(tool.name);
          return next;
        });
      }, 1500);
    }
  };

  const handleRemove = async (tool: Tool) => {
    setRemovingSet((prev) => new Set(prev).add(tool.name));
    try {
      await api.deleteTool(tool.name);
      addToast("success", `${tool.name} removed`);
      setCheckedTools(null);
      refresh();
    } catch (err) {
      const msg = err instanceof Error ? err.message : `Failed to remove ${tool.name}`;
      addToast("error", msg);
    } finally {
      setRemovingSet((prev) => { const next = new Set(prev); next.delete(tool.name); return next; });
    }
  };

  return (
    <div className="p-6 space-y-8">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-bc-muted hidden sm:block">
          {searchLower
            ? `${matchCount} of ${totalCount} tools`
            : <>{providerList.length} Providers &middot; {cliTools.length} CLI{checkedTools && " \u00b7 checked"}</>
          }
        </p>
        <div className="flex items-center gap-2">
          {/* Search with magnifying glass icon */}
          <div className="relative">
            <svg
              className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-bc-muted pointer-events-none"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <circle cx="11" cy="11" r="8" />
              <path strokeLinecap="round" d="M21 21l-4.35-4.35" />
            </svg>
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search tools..."
              className="w-40 sm:w-52 pl-7 pr-7 py-1.5 text-sm rounded border border-bc-border bg-bc-bg text-bc-text placeholder:text-bc-muted focus:outline-none focus:ring-1 focus:ring-bc-accent"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch("")}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-bc-muted hover:text-bc-text text-sm leading-none px-1"
                aria-label="Clear search"
              >
                &times;
              </button>
            )}
          </div>
          <button type="button" onClick={() => void handleCheck()} disabled={checking}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded border border-bc-border text-bc-muted hover:text-bc-text transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-bc-accent">
            {checking ? <Spinner /> : null}
            {checking ? "Checking..." : "Health Check"}
          </button>
          <button type="button" onClick={() => setShowAddForm(!showAddForm)}
            className="px-3 py-1.5 text-sm rounded bg-bc-info/10 text-bc-info hover:bg-bc-info/20 transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent">
            + CLI Tool
          </button>
        </div>
      </div>

      <AnimatePresence>
        {showAddForm && <AddCLIToolForm onClose={() => setShowAddForm(false)} onAdded={() => { setCheckedTools(null); refresh(); }} onToast={addToast} />}
      </AnimatePresence>

      <section>
        <h2 className="text-xs font-medium text-bc-muted uppercase tracking-widest mb-3">
          Providers ({providerList.length}) &mdash; AI model providers
        </h2>
        <ProvidersTable providers={providerList} search={search} />
      </section>

      <section>
        <h2 className="text-xs font-medium text-bc-muted uppercase tracking-widest mb-3">
          CLI Dependencies ({filteredCli.length}{searchLower ? `/${cliTools.length}` : ""})
        </h2>
        {filteredCli.length === 0 ? (
          <EmptyState icon=">" title={searchLower ? "No matching CLI tools" : "No CLI dependencies"} description={searchLower ? "Try a different search term." : "Add CLI tools like gh, aws, or wrangler."} />
        ) : (
          <div className="rounded border border-bc-border overflow-hidden">
            <table className="w-full text-left">
              <thead>
                <tr className="bg-bc-surface border-b border-bc-border text-[11px] text-bc-muted uppercase tracking-wider">
                  <th className="px-3 py-2 font-medium">Tool</th>
                  <th className="px-3 py-2 font-medium">Status</th>
                  <th className="px-3 py-2 font-medium">Version</th>
                  <th className="px-3 py-2 font-medium">Required</th>
                  <th className="px-3 py-2 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredCli.map((t) => (
                  <CLIDepsRow
                    key={t.name}
                    tool={t}
                    expanded={expandedRow === t.name}
                    onExpand={() => setExpandedRow(expandedRow === t.name ? null : t.name)}
                    onToggle={() => void handleToggle(t)}
                    onRemove={() => void handleRemove(t)}
                    toggling={togglingSet.has(t.name)}
                    removing={removingSet.has(t.name)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <ToastContainer toasts={toasts} onDismiss={dismiss} />
    </div>
  );
}
