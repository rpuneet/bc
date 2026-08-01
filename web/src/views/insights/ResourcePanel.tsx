/**
 * ResourcePanel — the fleet's compute budget, living in Insights next to
 * the cost budget. It answers "how much CPU/memory have I committed across
 * agents, and where — and how much of that is actually being used?" from
 * the per-agent Docker caps (set on each agent's Settings tab) plus the
 * latest live sample per agent (agent_stats). Agents with no cap override
 * inherit the fleet default, shown for context.
 */

import { useCallback, type ReactNode } from "react";
import { api, type Agent, type AgentMetricTS, type SettingsConfig } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { SectionRule } from "../../components/shared/SectionRule";
import { stripAgentPrefix } from "./chrome";

interface ResourceData {
  agents: Agent[];
  settings: SettingsConfig;
  latest: AgentMetricTS[];
}

/** MB → human ("2048 MB" → "2.0 GB"). */
function fmtMem(mb: number): string {
  if (mb <= 0) return "—";
  if (mb >= 1024) return `${(mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1)} GB`;
  return `${mb} MB`;
}

/** Frame the panel with its section header so error/loading states keep the
 *  same shell as the loaded panel. */
function ResourceShell({ children }: { children: ReactNode }) {
  return (
    <section>
      <SectionRule
        label="Resource budget"
        trailing={<span className="text-[11px] text-mycel-muted">committed caps · Docker agents</span>}
      />
      <div className="rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel-sm p-4 space-y-3">
        {children}
      </div>
    </section>
  );
}

export function ResourcePanel() {
  // Agents and settings must both succeed: a failed listAgents would
  // understate agents, and a failed getSettings would drop the fleet
  // defaults and understate committed caps. Reject on either so usePolling
  // exposes the error state instead of silently rendering "no resources".
  // Live usage (stats/latest) is best-effort — its absence just means the
  // "used" figures fall back to "—", never a hard error for the whole panel.
  const fetcher = useCallback(async (): Promise<ResourceData> => {
    const [agents, settings, latest] = await Promise.all([
      api.listAgents(),
      api.getSettings(),
      api.getAgentStatsLatest().catch(() => [] as AgentMetricTS[]),
    ]);
    return {
      agents: Array.isArray(agents) ? agents : [],
      settings,
      latest: Array.isArray(latest) ? latest : [],
    };
  }, []);

  const { data, loading, error, refresh } = usePolling<ResourceData>(fetcher, 30000);

  // First-load error (no data yet) → an explicit unavailable state with a
  // retry, never a misleading empty panel.
  if (error && !data) {
    return (
      <ResourceShell>
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-mycel-muted">Couldn&apos;t load resource budget.</p>
          <button
            type="button"
            onClick={refresh}
            className="inline-flex items-center h-7 px-2.5 rounded-md text-[11px] font-medium border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:border-mycel-muted transition-colors"
          >
            Retry
          </button>
        </div>
      </ResourceShell>
    );
  }

  // First load in flight → a quiet placeholder, not a "no agents" claim.
  if (loading && !data) {
    return (
      <ResourceShell>
        <p className="text-xs text-mycel-muted">Loading resource budget…</p>
      </ResourceShell>
    );
  }

  const agents = data?.agents ?? [];
  const settings = data?.settings ?? null;
  const latest = data?.latest ?? [];

  const defCpu = settings?.runtime?.docker?.cpus ?? 0;
  const defMem = settings?.runtime?.docker?.memory_mb ?? 0;

  // Only Docker agents are actually constrained; tmux agents run
  // unconstrained on the host, so they don't count toward the committed
  // budget (but we still surface them honestly, unconstrained).
  const dockerAgents = agents.filter((a) => (a.runtime_backend ?? "docker") === "docker");

  // Live usage keyed by raw agent name (matches AgentMetricTS.agent_name,
  // the same key StatsTab uses when querying per-agent stats). cpu_percent
  // follows the Docker-stats convention — 100% == one full core — so
  // dividing by 100 yields core-equivalents comparable to the cpus cap.
  const usageByAgent = new Map<string, { cpuCores: number; memMB: number }>();
  for (const m of latest) {
    usageByAgent.set(m.agent_name, {
      cpuCores: (m.cpu_percent ?? 0) / 100,
      memMB: (m.mem_used_bytes ?? 0) / 1024 / 1024,
    });
  }

  // Effective cap per agent = its override, else the fleet default.
  const rows = dockerAgents.map((a) => {
    const usage = usageByAgent.get(a.name);
    return {
      name: stripAgentPrefix(a.name),
      cpus: a.cpus && a.cpus > 0 ? a.cpus : defCpu,
      memoryMB: a.memory_mb && a.memory_mb > 0 ? a.memory_mb : defMem,
      overridden: (a.cpus ?? 0) > 0 || (a.memory_mb ?? 0) > 0,
      usedCpuCores: usage?.cpuCores,
      usedMemMB: usage?.memMB,
    };
  });

  const totalCpu = rows.reduce((s, r) => s + r.cpus, 0);
  const totalMem = rows.reduce((s, r) => s + r.memoryMB, 0);
  const totalUsedCpu = rows.reduce((s, r) => s + (r.usedCpuCores ?? 0), 0);
  const totalUsedMem = rows.reduce((s, r) => s + (r.usedMemMB ?? 0), 0);
  const hasLiveUsage = rows.some((r) => r.usedCpuCores != null);

  return (
    <ResourceShell>
      <>
        {/* Committed totals */}
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-px bg-mycel-border rounded-md overflow-hidden">
          <div className="bg-mycel-surface p-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">CPU committed</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-mycel-text">
              {totalCpu > 0 ? `${totalCpu.toFixed(1)} cores` : "—"}
            </div>
            {hasLiveUsage && (
              <div className="text-[11px] tabular-nums text-mycel-muted">{totalUsedCpu.toFixed(1)} used</div>
            )}
          </div>
          <div className="bg-mycel-surface p-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Memory committed</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-mycel-text">{fmtMem(totalMem)}</div>
            {hasLiveUsage && (
              <div className="text-[11px] tabular-nums text-mycel-muted">{fmtMem(totalUsedMem)} used</div>
            )}
          </div>
          <div className="bg-mycel-surface p-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Fleet default</div>
            <div className="mt-1 text-sm tabular-nums text-mycel-text-2">
              {defCpu > 0 ? `${defCpu} cores` : "—"} · {fmtMem(defMem)}
            </div>
          </div>
        </div>

        {/* Per-agent breakdown */}
        {rows.length > 0 ? (
          <div className="rounded-lg border border-mycel-border bg-mycel-bg divide-y divide-mycel-border overflow-hidden">
            {rows.map((r) => (
              <div key={r.name} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
                <span className="text-mycel-text truncate">{r.name}</span>
                <span className="flex items-center gap-2 shrink-0 tabular-nums text-mycel-muted">
                  <span>
                    {r.cpus > 0 ? `${r.cpus} CPU cap` : "—"}
                    {r.usedCpuCores != null && ` · ${r.usedCpuCores.toFixed(1)} used`}
                  </span>
                  <span className="text-mycel-border">·</span>
                  <span>
                    {fmtMem(r.memoryMB)} cap
                    {r.usedMemMB != null && ` · ${fmtMem(r.usedMemMB)} used`}
                  </span>
                  {!r.overridden && (
                    <span className="text-[10px] text-mycel-muted/70 italic">default</span>
                  )}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-xs text-mycel-muted">No Docker agents to budget yet.</p>
        )}

        <p className="text-[11px] text-mycel-muted leading-relaxed">
          Set per-agent CPU/memory caps from an agent&apos;s Settings tab.
          {hasLiveUsage
            ? " \"used\" figures are the latest live sample (agent_stats) — actual CPU/memory consumed right now."
            : " Live usage will appear here once the stats sampler reports data for these agents."}
        </p>
      </>
    </ResourceShell>
  );
}
