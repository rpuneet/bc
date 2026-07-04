import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { ProviderInfo } from "../api/client";
import { formatCost, formatTokens } from "../utils/format";
import { EmptyState } from "./EmptyState";
import { ProviderCard } from "./ProviderCard";

type SortKey = "name" | "status" | "version" | "agent_count" | "total_tokens" | "total_cost_usd";
type SortDir = "asc" | "desc";
type ViewMode = "cards" | "table";

function statusOrder(p: ProviderInfo): number {
  if (p.installed && p.agent_count > 0) return 0; // active
  if (p.installed) return 1; // idle
  return 2; // not installed
}

function StatusDot({ provider }: { provider: ProviderInfo }) {
  if (!provider.installed) {
    return <span className="inline-flex items-center gap-1.5 text-mycel-error"><span className="text-xs">&#10005;</span> N/A</span>;
  }
  if (provider.agent_count > 0) {
    return <span className="inline-flex items-center gap-1.5 text-mycel-success"><span className="w-2 h-2 rounded-full bg-mycel-success inline-block" /> Active</span>;
  }
  return <span className="inline-flex items-center gap-1.5 text-mycel-muted"><span className="w-2 h-2 rounded-full bg-mycel-muted inline-block" /> Idle</span>;
}

/* ── Sort dropdown for card mode ── */
function SortDropdown({ sortKey, sortDir, onSort }: { sortKey: SortKey; sortDir: SortDir; onSort: (key: SortKey) => void }) {
  const options: { key: SortKey; label: string }[] = [
    { key: "name", label: "Name" },
    { key: "status", label: "Status" },
    { key: "agent_count", label: "Agents" },
    { key: "total_cost_usd", label: "Cost" },
    { key: "total_tokens", label: "Tokens" },
  ];

  return (
    <select
      value={sortKey}
      onChange={(e) => onSort(e.target.value as SortKey)}
      className="text-xs px-2 py-1 rounded border border-mycel-border bg-mycel-bg text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
      aria-label="Sort providers"
    >
      {options.map((o) => (
        <option key={o.key} value={o.key}>
          {o.label} {sortKey === o.key ? (sortDir === "asc" ? "\u2191" : "\u2193") : ""}
        </option>
      ))}
    </select>
  );
}

/* ── View mode toggle icons ── */
function ViewToggle({ mode, onChange }: { mode: ViewMode; onChange: (m: ViewMode) => void }) {
  return (
    <div className="flex items-center border border-mycel-border rounded overflow-hidden">
      <button
        type="button"
        onClick={() => onChange("cards")}
        className={`p-1.5 transition-colors ${mode === "cards" ? "bg-mycel-accent-subtle text-mycel-accent" : "text-mycel-muted hover:text-mycel-text"}`}
        aria-label="Card view"
      >
        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <rect x="3" y="3" width="7" height="7" rx="1" />
          <rect x="14" y="3" width="7" height="7" rx="1" />
          <rect x="3" y="14" width="7" height="7" rx="1" />
          <rect x="14" y="14" width="7" height="7" rx="1" />
        </svg>
      </button>
      <button
        type="button"
        onClick={() => onChange("table")}
        className={`p-1.5 transition-colors ${mode === "table" ? "bg-mycel-accent-subtle text-mycel-accent" : "text-mycel-muted hover:text-mycel-text"}`}
        aria-label="Table view"
      >
        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" d="M3 6h18M3 12h18M3 18h18" />
        </svg>
      </button>
    </div>
  );
}

interface Props {
  providers: ProviderInfo[];
  search: string;
}

export function ProvidersTable({ providers, search }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [viewMode, setViewMode] = useState<ViewMode>("cards");
  const navigate = useNavigate();
  const toolsBase = "/tools";

  const filtered = useMemo(() => {
    const q = search.toLowerCase().trim();
    if (!q) return providers;
    return providers.filter((p) => p.name.toLowerCase().includes(q));
  }, [providers, search]);

  const sorted = useMemo(() => {
    const arr = [...filtered];
    arr.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name":
          cmp = a.name.localeCompare(b.name);
          break;
        case "status":
          cmp = statusOrder(a) - statusOrder(b);
          break;
        case "version":
          cmp = (a.version || "").localeCompare(b.version || "");
          break;
        case "agent_count":
          cmp = a.agent_count - b.agent_count;
          break;
        case "total_tokens":
          cmp = a.total_tokens - b.total_tokens;
          break;
        case "total_cost_usd":
          cmp = a.total_cost_usd - b.total_cost_usd;
          break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
    return arr;
  }, [filtered, sortKey, sortDir]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("asc");
    }
  };

  const sortIndicator = (key: SortKey) => {
    if (sortKey !== key) return null;
    return <span className="ml-1 text-mycel-accent">{sortDir === "asc" ? "\u25B2" : "\u25BC"}</span>;
  };

  if (sorted.length === 0) {
    return (
      <EmptyState
        icon="*"
        title={search ? "No matching providers" : "No providers"}
        description={search ? "Try a different search term." : "No AI providers configured."}
      />
    );
  }

  const columns: { key: SortKey; label: string; className?: string }[] = [
    { key: "name", label: "Provider" },
    { key: "status", label: "Status" },
    { key: "version", label: "Version" },
    { key: "agent_count", label: "Agents", className: "text-right" },
    { key: "total_tokens", label: "Tokens", className: "text-right" },
    { key: "total_cost_usd", label: "Cost", className: "text-right" },
  ];

  return (
    <div>
      {/* Controls row */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <ViewToggle mode={viewMode} onChange={setViewMode} />
          {viewMode === "cards" && (
            <SortDropdown sortKey={sortKey} sortDir={sortDir} onSort={toggleSort} />
          )}
        </div>
        <span className="text-xs text-mycel-muted">{sorted.length} provider{sorted.length !== 1 ? "s" : ""}</span>
      </div>

      {/* Card grid view */}
      {viewMode === "cards" ? (
        <div
          className="grid gap-3"
          style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}
        >
          {sorted.map((p) => (
            <ProviderCard
              key={p.name}
              provider={p}
              onClick={() => navigate(`${toolsBase}/${encodeURIComponent(p.name)}`)}
            />
          ))}
        </div>
      ) : (
        /* Table view */
        <div className="rounded border border-mycel-border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-mycel-border bg-mycel-surface">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    onClick={(e) => { e.stopPropagation(); e.preventDefault(); toggleSort(col.key); }}
                    className={`px-4 py-2 font-medium text-mycel-muted cursor-pointer select-none hover:text-mycel-text transition-colors text-left ${col.className ?? ""}`}
                  >
                    {col.label}{sortIndicator(col.key)}
                  </th>
                ))}
                <th className="px-4 py-2 font-medium text-mycel-muted text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((p) => (
                <tr
                  key={p.name}
                  onClick={() => navigate(`${toolsBase}/${encodeURIComponent(p.name)}`)}
                  className="border-b border-mycel-border cursor-pointer hover:bg-mycel-surface transition-colors"
                >
                  <td className="px-4 py-2.5 font-medium">{p.name}</td>
                  <td className="px-4 py-2.5 text-xs"><StatusDot provider={p} /></td>
                  <td className="px-4 py-2.5 text-xs text-mycel-muted font-mono">{p.version || "\u2014"}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums">{p.agent_count}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-mycel-muted">{formatTokens(p.total_tokens)}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums">{formatCost(p.total_cost_usd)}</td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="flex items-center justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
                      {!p.installed && p.install_hint && (
                        <button
                          type="button"
                          onClick={() => navigate(`${toolsBase}/${encodeURIComponent(p.name)}`)}
                          className="text-xs px-2 py-0.5 rounded bg-mycel-warning-subtle text-mycel-warning hover:bg-mycel-surface-hover transition-colors"
                        >
                          Install
                        </button>
                      )}
                      {p.installed && p.install_hint && (
                        <span className="text-xs px-2 py-0.5 rounded bg-mycel-info-subtle text-mycel-info">
                          Update
                        </span>
                      )}
                      <button
                        type="button"
                        onClick={() => navigate(`${toolsBase}/${encodeURIComponent(p.name)}`)}
                        className="text-xs px-1.5 py-0.5 rounded border border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-accent transition-colors"
                        aria-label={`Configure ${p.name}`}
                      >
                        &#9881;
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
