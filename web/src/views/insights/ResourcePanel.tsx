/**
 * ResourcePanel — the fleet's compute budget, living in Insights next to
 * the cost budget. It answers "how much CPU/memory have I committed across
 * agents, and where?" from the per-agent Docker caps (set on each agent's
 * Settings tab). Agents with no override inherit the fleet default, shown
 * for context.
 *
 * Live per-agent CPU/memory *usage* is sampled for the Docker runtime
 * (agent_stats) but not yet aggregated into a fleet figure — so this panel
 * is honest about showing configured caps, with a clear seam for usage.
 */

import { useCallback } from "react";
import { api, type Agent, type SettingsConfig } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { SectionRule } from "../../components/shared/SectionRule";
import { stripAgentPrefix } from "./chrome";

interface ResourceData {
  agents: Agent[];
  settings: SettingsConfig | null;
}

/** MB → human ("2048 MB" → "2.0 GB"). */
function fmtMem(mb: number): string {
  if (mb <= 0) return "—";
  if (mb >= 1024) return `${(mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1)} GB`;
  return `${mb} MB`;
}

export function ResourcePanel() {
  const fetcher = useCallback(async (): Promise<ResourceData> => {
    const [agents, settings] = await Promise.allSettled([
      api.listAgents(),
      api.getSettings(),
    ]);
    return {
      agents: agents.status === "fulfilled" && Array.isArray(agents.value) ? agents.value : [],
      settings: settings.status === "fulfilled" ? settings.value : null,
    };
  }, []);

  const { data } = usePolling<ResourceData>(fetcher, 30000);
  const agents = data?.agents ?? [];
  const settings = data?.settings ?? null;

  const defCpu = settings?.runtime?.docker?.cpus ?? 0;
  const defMem = settings?.runtime?.docker?.memory_mb ?? 0;

  // Only Docker agents are actually constrained; tmux agents run
  // unconstrained on the host, so they don't count toward the committed
  // budget (but we still surface them honestly, unconstrained).
  const dockerAgents = agents.filter((a) => (a.runtime_backend ?? "docker") === "docker");

  // Effective cap per agent = its override, else the fleet default.
  const rows = dockerAgents.map((a) => ({
    name: stripAgentPrefix(a.name),
    cpus: a.cpus && a.cpus > 0 ? a.cpus : defCpu,
    memoryMB: a.memory_mb && a.memory_mb > 0 ? a.memory_mb : defMem,
    overridden: (a.cpus ?? 0) > 0 || (a.memory_mb ?? 0) > 0,
  }));

  const totalCpu = rows.reduce((s, r) => s + r.cpus, 0);
  const totalMem = rows.reduce((s, r) => s + r.memoryMB, 0);

  return (
    <section>
      <SectionRule
        label="Resource budget"
        trailing={<span className="text-[11px] text-mycel-muted">committed caps · Docker agents</span>}
      />
      <div className="rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel-sm p-4 space-y-3">
        {/* Committed totals */}
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-px bg-mycel-border rounded-md overflow-hidden">
          <div className="bg-mycel-surface p-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">CPU committed</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-mycel-text">
              {totalCpu > 0 ? `${totalCpu.toFixed(1)} cores` : "—"}
            </div>
          </div>
          <div className="bg-mycel-surface p-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Memory committed</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-mycel-text">{fmtMem(totalMem)}</div>
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
                  <span>{r.cpus > 0 ? `${r.cpus} CPU` : "—"}</span>
                  <span className="text-mycel-border">·</span>
                  <span>{fmtMem(r.memoryMB)}</span>
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
          Set per-agent CPU/memory caps from an agent&apos;s Settings tab. Live usage metrics
          (actual CPU/memory consumed) are coming soon — this panel shows committed caps.
          {/* follow-up: aggregate agent_stats samples into a live fleet usage figure. */}
        </p>
      </div>
    </section>
  );
}
