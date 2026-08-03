import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { ProviderInfo } from "../api/client";
import { canAutoInstall } from "../utils/providerActions";
import { formatCost, formatTokens } from "../utils/format";
import { PROVIDER_LABELS } from "../views/readiness/readiness";
import { ProviderLogo } from "./ProviderLogo";
import { EmptyState } from "./EmptyState";

type SortKey = "name" | "status" | "version" | "agent_count" | "total_tokens" | "total_cost_usd";
type SortDir = "asc" | "desc";

function statusOrder(p: ProviderInfo): number {
  if (p.installed && p.agent_count > 0) return 0; // active
  if (p.installed) return 1; // idle
  return 2; // not installed
}

/* A compact, filled status pill — one glanceable token per row. Active
 * (agents on it) reads success; installed-but-idle reads neutral; not
 * installed reads as an amber call-to-action, not an error. */
function StatusPill({ provider }: { provider: ProviderInfo }) {
  const base = "inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium whitespace-nowrap";
  if (!provider.installed) {
    return (
      <span className={`${base} bg-mycel-warning-subtle text-mycel-warning`}>
        <span className="w-1.5 h-1.5 rounded-full bg-mycel-warning" /> Not installed
      </span>
    );
  }
  if (provider.agent_count > 0) {
    return (
      <span className={`${base} bg-mycel-success-subtle text-mycel-success`}>
        <span className="relative flex h-1.5 w-1.5">
          <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-mycel-success opacity-75" />
          <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-mycel-success" />
        </span>
        Active
      </span>
    );
  }
  return (
    <span className={`${base} bg-mycel-surface-hover text-mycel-text-2`}>
      <span className="w-1.5 h-1.5 rounded-full bg-mycel-muted" /> Idle
    </span>
  );
}

interface Props {
  providers: ProviderInfo[];
  search: string;
}

/* List-only providers table. There is no card/grid mode — the Settings
 * "Providers & Tools" section is deliberately a compact, scannable list;
 * a row click drills into /settings/providers/:name for the full manager. */
export function ProvidersTable({ providers, search }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const navigate = useNavigate();
  const providersBase = "/settings/providers";

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
    return <span className="ml-1 text-mycel-accent">{sortDir === "asc" ? "▲" : "▼"}</span>;
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
      <div className="flex items-center justify-end mb-3">
        <span className="text-xs text-mycel-muted">{sorted.length} provider{sorted.length !== 1 ? "s" : ""}</span>
      </div>

      <div className="rounded-lg border border-mycel-border overflow-x-auto">
        <table className="w-full min-w-[640px] text-sm">
          <thead>
            <tr className="border-b border-mycel-border bg-mycel-surface">
              {columns.map((col) => (
                <th
                  key={col.key}
                  onClick={(e) => { e.stopPropagation(); e.preventDefault(); toggleSort(col.key); }}
                  className={`px-4 py-2.5 text-[11px] uppercase tracking-[0.08em] font-medium text-mycel-muted cursor-pointer select-none hover:text-mycel-text transition-colors text-left ${col.className ?? ""}`}
                >
                  {col.label}{sortIndicator(col.key)}
                </th>
              ))}
              <th className="px-4 py-2.5 text-[11px] uppercase tracking-[0.08em] font-medium text-mycel-muted text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((p) => {
              const label = PROVIDER_LABELS[p.name] ?? p.name;
              const to = `${providersBase}/${encodeURIComponent(p.name)}`;
              return (
              <tr
                key={p.name}
                onClick={() => navigate(to)}
                className="group border-b border-mycel-border last:border-b-0 cursor-pointer hover:bg-mycel-surface-hover transition-colors"
              >
                <td className="px-4 py-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <ProviderLogo name={p.name} size={34} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-mycel-text truncate">{label}</span>
                        {label !== p.name && (
                          <span className="text-[11px] font-mono text-mycel-muted truncate">{p.name}</span>
                        )}
                      </div>
                      {p.description && (
                        <p className="text-xs text-mycel-muted truncate max-w-[36ch]">{p.description}</p>
                      )}
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3"><StatusPill provider={p} /></td>
                <td className="px-4 py-3 text-xs text-mycel-muted font-mono tabular-nums whitespace-nowrap">{p.version ? `v${p.version}` : "—"}</td>
                <td className="px-4 py-3 text-right tabular-nums">{p.agent_count}</td>
                <td className="px-4 py-3 text-right tabular-nums text-mycel-muted">{formatTokens(p.total_tokens)}</td>
                <td className="px-4 py-3 text-right tabular-nums">{formatCost(p.total_cost_usd)}</td>
                <td className="px-4 py-3 text-right">
                  <div className="flex items-center justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
                    {!p.installed && canAutoInstall(p.install_hint) && (
                      <button
                        type="button"
                        onClick={() => navigate(to)}
                        className="inline-flex items-center min-h-[32px] text-xs font-medium px-2.5 py-1 rounded-md bg-mycel-warning-subtle text-mycel-warning hover:bg-mycel-warning hover:text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-warning"
                      >
                        Install
                      </button>
                    )}
                    {p.installed && canAutoInstall(p.install_hint) && (
                      <button
                        type="button"
                        onClick={() => navigate(to)}
                        className="inline-flex items-center min-h-[32px] text-xs font-medium px-2.5 py-1 rounded-md bg-mycel-info-subtle text-mycel-info hover:bg-mycel-info hover:text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-info"
                      >
                        Update
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => navigate(to)}
                      className="inline-flex items-center justify-center min-h-[32px] min-w-[32px] rounded-md border border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-accent hover:bg-mycel-surface transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                      aria-label={`Configure ${label}`}
                      title={`Configure ${label}`}
                    >
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.7} aria-hidden>
                        <circle cx="12" cy="12" r="3" />
                        <path strokeLinecap="round" strokeLinejoin="round" d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 8.4 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H2a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 3.6 8.4a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H8a1.65 1.65 0 0 0 1-1.51V2a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V8a1.65 1.65 0 0 0 1.51 1H22a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
                      </svg>
                    </button>
                  </div>
                </td>
              </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
