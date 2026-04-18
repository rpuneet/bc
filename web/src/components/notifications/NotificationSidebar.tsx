import { useState, useEffect, useCallback } from "react";
import type { Agent, NotificationSource, GatewayStatus, NotifySubscription } from "../../api/client";
import { api } from "../../api/client";
import { sourcePlatform, agentColor, getRoleColor } from "./messageUtils";
import { SetupWizard, PlatformChooser, PLATFORM_MAP, PLATFORMS } from "./SetupWizard";

function getMeta(p: string) {
  const def = PLATFORM_MAP[p];
  if (def) return { label: def.label, color: def.color };
  return { label: p, color: "#8c7e72" };
}

function displayName(name: string): string {
  const idx = name.indexOf(":");
  return idx > 0 ? name.slice(idx + 1) : name;
}

export function NotificationSidebar({
  channels,
  selected,
  onSelect,
}: {
  channels: NotificationSource[];
  selected: string | null;
  onSelect: (name: string) => void;
}) {
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [allSubs, setAllSubs] = useState<NotifySubscription[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);
  const [view, setView] = useState<"sources" | "agents">("sources");
  const [expandedGw, setExpandedGw] = useState<Set<string>>(new Set(["slack", "telegram", "discord"]));

  const fetchData = useCallback(async () => {
    try {
      const [gw, subs, agentList] = await Promise.all([
        api.listGateways(),
        api.listSubscriptions().catch(() => []),
        api.listAgents().catch(() => []),
      ]);
      setGateways(gw ?? []);
      setAllSubs(subs ?? []);
      setAgents(agentList ?? []);
    } catch { /* */ }
  }, []);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 12000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const toggleGw = (p: string) => {
    setExpandedGw((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p); else next.add(p);
      return next;
    });
  };

  // Subscription counts
  const subCountMap = new Map<string, number>();
  const sourceSubs = new Map<string, Set<string>>();
  for (const sub of allSubs) {
    subCountMap.set(sub.channel, (subCountMap.get(sub.channel) ?? 0) + 1);
    if (!sourceSubs.has(sub.channel)) sourceSubs.set(sub.channel, new Set());
    sourceSubs.get(sub.channel)!.add(sub.agent);
  }

  // Build gateway buckets
  const gwMap = new Map<string, GatewayStatus>();
  for (const gw of gateways) gwMap.set(gw.platform, gw);

  const bucketMap = new Map<string, NotificationSource[]>();
  for (const ch of channels) {
    const p = sourcePlatform(ch.name);
    if (p === "internal") continue;
    const list = bucketMap.get(p) ?? [];
    list.push(ch);
    bucketMap.set(p, list);
  }
  for (const gw of gateways) {
    if (!bucketMap.has(gw.platform)) bucketMap.set(gw.platform, []);
  }

  const configuredPlatforms = new Set(bucketMap.keys());
  const unconfigured = PLATFORMS.filter(p => !configuredPlatforms.has(p.key));

  // Agents for current channel
  const currentSourceAgents = selected ? sourceSubs.get(selected) ?? new Set() : new Set();

  const handleSubscribe = async (agent: string) => {
    if (!selected) return;
    try {
      await api.subscribe(selected, agent, false);
      await fetchData();
    } catch { /* */ }
  };

  const handleUnsubscribe = async (agent: string) => {
    if (!selected) return;
    try {
      await api.unsubscribe(selected, agent);
      await fetchData();
    } catch { /* */ }
  };

  return (
    <nav
      className="w-56 shrink-0 border-r border-bc-border/40 flex flex-col bg-bc-bg"
      style={{ scrollbarWidth: "thin", scrollbarColor: "rgba(255,255,255,0.04) transparent" }}
    >
      {/* Tab toggle: Notifications | Agents */}
      <div className="flex border-b border-bc-border/30">
        <button
          type="button"
          onClick={() => setView("sources")}
          className={`flex-1 py-2.5 text-[10px] font-bold uppercase tracking-[0.12em] transition-colors ${
            view === "sources"
              ? "text-bc-text border-b-2 border-bc-accent"
              : "text-bc-muted/40 hover:text-bc-muted/70"
          }`}
        >
          # Notifications
        </button>
        <button
          type="button"
          onClick={() => setView("agents")}
          className={`flex-1 py-2.5 text-[10px] font-bold uppercase tracking-[0.12em] transition-colors ${
            view === "agents"
              ? "text-bc-text border-b-2 border-bc-accent"
              : "text-bc-muted/40 hover:text-bc-muted/70"
          }`}
        >
          Agents {currentSourceAgents.size > 0 && (
            <span className="text-bc-success ml-1">{currentSourceAgents.size}</span>
          )}
        </button>
      </div>

      <div className="flex-1 overflow-auto py-1">
        {view === "sources" ? (
          /* ── Notifications view ──────────────────────────── */
          <>
            {[...bucketMap.entries()].map(([platform, chs]) => {
              const meta = getMeta(platform);
              const gwStatus = gwMap.get(platform);
              const isConnected = gwStatus?.enabled && (gwStatus?.channels?.length ?? 0) > 0 || chs.length > 0;
              const isExpanded = expandedGw.has(platform);

              return (
                <div key={platform} className="mb-0.5">
                  <button
                    type="button"
                    onClick={() => toggleGw(platform)}
                    className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-bc-surface/20 transition-colors"
                  >
                    <svg width="8" height="8" viewBox="0 0 8 8"
                      className={`text-bc-muted/30 transition-transform duration-150 ${isExpanded ? "" : "-rotate-90"}`}
                    >
                      <path d="M1.5 2L4 5L6.5 2" stroke="currentColor" strokeWidth="1.2" fill="none" strokeLinecap="round" />
                    </svg>
                    <span
                      className="w-1.5 h-1.5 rounded-full shrink-0"
                      style={{ backgroundColor: isConnected ? "#22c55e" : gwStatus?.enabled ? "#fb923c" : "rgba(140,126,114,0.2)" }}
                    />
                    <span className="text-[10px] font-bold uppercase tracking-[0.08em]" style={{ color: meta.color }}>
                      {meta.label}
                    </span>
                    <span className="text-[9px] text-bc-muted/25 ml-auto tabular-nums">{chs.length}</span>
                  </button>

                  {isExpanded && (
                    <div className="pb-0.5">
                      {chs.length === 0 && (
                        <div className="px-3 py-1 text-[10px] text-bc-muted/20 italic pl-8">No notifications</div>
                      )}
                      {chs.map((ch) => {
                        const isActive = selected === ch.name;
                        const count = subCountMap.get(ch.name) ?? 0;
                        return (
                          <button
                            key={ch.name}
                            onClick={() => onSelect(ch.name)}
                            className={`w-full text-left pl-7 pr-3 py-[5px] text-[12px] flex items-center gap-1.5 transition-all duration-100 ${
                              isActive
                                ? "bg-bc-surface/60 text-bc-text font-medium"
                                : "text-bc-muted/60 hover:text-bc-text/80 hover:bg-bc-surface/20"
                            }`}
                            style={{ borderLeft: isActive ? `2px solid ${meta.color}` : "2px solid transparent" }}
                          >
                            <span className="text-bc-muted/25 text-[10px]">#</span>
                            <span className="truncate">{displayName(ch.name)}</span>
                            {count > 0 && (
                              <span className="ml-auto text-[9px] text-bc-success/40 tabular-nums">{count}</span>
                            )}
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}

            {/* Unconfigured */}
            {unconfigured.length > 0 && (
              <div className="pt-1 mt-1 border-t border-bc-border/15 mx-3">
                {unconfigured.slice(0, 5).map((p) => (
                  <button
                    key={p.key}
                    type="button"
                    onClick={() => setSetupPlatform(p.key)}
                    className="w-full flex items-center gap-2 py-1 text-[10px] text-bc-muted/20 hover:text-bc-muted/40 transition-colors"
                  >
                    <span className="w-1.5 h-1.5 rounded-full bg-bc-muted/10" />
                    <span className="uppercase tracking-[0.08em] font-medium">{p.label}</span>
                    <span className="ml-auto opacity-50">+</span>
                  </button>
                ))}
                {unconfigured.length > 5 && (
                  <button
                    type="button"
                    onClick={() => setSetupPlatform("_choose")}
                    className="w-full py-1 text-[9px] text-bc-muted/15 hover:text-bc-accent transition-colors"
                  >
                    + {unconfigured.length - 5} more...
                  </button>
                )}
              </div>
            )}
          </>
        ) : (
          /* ── Agents view ────────────────────────────── */
          <>
            {!selected ? (
              <div className="p-4 text-[11px] text-bc-muted/30 text-center">
                Select a notification source first
              </div>
            ) : (
              <>
                {/* Listening/subscribed agents */}
                {agents.filter(a => currentSourceAgents.has(a.name)).length > 0 && (
                  <div>
                    <div className="px-3 pt-2 pb-1">
                      <span className="text-[9px] font-bold text-bc-success/60 uppercase tracking-[0.08em]">
                        Listening ({agents.filter(a => currentSourceAgents.has(a.name)).length})
                      </span>
                    </div>
                    {agents.filter(a => currentSourceAgents.has(a.name)).map((agent) => {
                      const isOnline = agent.state === "running" || agent.state === "working";
                      const roleColor = getRoleColor(agent.role);
                      return (
                        <div key={agent.name} className="px-3 py-1.5 flex items-center gap-2 hover:bg-bc-surface/20 transition-colors">
                          <span className="w-4 h-4 rounded text-[8px] font-bold flex items-center justify-center shrink-0"
                            style={{ backgroundColor: `${agentColor(agent.name)}12`, color: agentColor(agent.name) }}
                          >
                            {agent.name.charAt(0).toUpperCase()}
                          </span>
                          <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${isOnline ? "bg-bc-success" : "bg-bc-muted/20"}`} />
                          <span className="text-[11px] text-bc-text/80 truncate flex-1">{agent.name}</span>
                          <span className={`text-[8px] px-1 py-0.5 rounded ${roleColor.bg} ${roleColor.text} font-medium`}>
                            {agent.role}
                          </span>
                          <button
                            type="button"
                            onClick={() => handleUnsubscribe(agent.name)}
                            className="text-[8px] text-bc-muted/20 hover:text-bc-error/50 transition-colors"
                          >
                            &times;
                          </button>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Divider */}
                {agents.filter(a => currentSourceAgents.has(a.name)).length > 0 &&
                  agents.filter(a => !currentSourceAgents.has(a.name)).length > 0 && (
                  <div className="mx-3 my-1.5 border-t border-bc-border/15" />
                )}

                {/* Available agents */}
                <div>
                  <div className="px-3 pt-2 pb-1">
                    <span className="text-[9px] font-bold text-bc-muted/30 uppercase tracking-[0.08em]">
                      Add to notifications
                    </span>
                  </div>
                  {agents.filter(a => !currentSourceAgents.has(a.name)).sort((a, b) => a.name.localeCompare(b.name)).map((agent) => {
                    const isOnline = agent.state === "running" || agent.state === "working";
                    const isStopped = agent.state === "stopped";
                    return (
                      <div key={agent.name} className="px-3 py-1.5 flex items-center gap-2 hover:bg-bc-surface/15 transition-colors group">
                        <span className="w-4 h-4 rounded text-[8px] font-bold flex items-center justify-center shrink-0 opacity-40"
                          style={{ backgroundColor: `${agentColor(agent.name)}08`, color: agentColor(agent.name) }}
                        >
                          {agent.name.charAt(0).toUpperCase()}
                        </span>
                        <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                          isOnline ? "bg-bc-success" : isStopped ? "bg-bc-error/40" : "bg-bc-muted/15"
                        }`} />
                        <span className="text-[11px] text-bc-muted/40 truncate flex-1">{agent.name}</span>
                        <button
                          type="button"
                          onClick={() => handleSubscribe(agent.name)}
                          className="text-[9px] text-bc-muted/20 hover:text-bc-accent opacity-0 group-hover:opacity-100 transition-all"
                        >
                          + add
                        </button>
                      </div>
                    );
                  })}
                </div>
              </>
            )}
          </>
        )}
      </div>

      {/* Bottom: Connect app */}
      <div className="p-2 border-t border-bc-border/20">
        <button
          type="button"
          onClick={() => setSetupPlatform("_choose")}
          className="w-full py-1.5 text-[10px] font-medium text-bc-muted/30 hover:text-bc-accent border border-bc-border/20 rounded-lg hover:border-bc-accent/20 transition-all"
        >
          + Connect app
        </button>
      </div>

      {/* Setup wizard */}
      {setupPlatform && setupPlatform !== "_choose" && (
        <SetupWizard platform={setupPlatform} onClose={() => setSetupPlatform(null)} onConnected={() => void fetchData()} />
      )}
      {setupPlatform === "_choose" && (
        <PlatformChooser
          onSelect={(key) => setSetupPlatform(key)}
          onClose={() => setSetupPlatform(null)}
        />
      )}
    </nav>
  );
}
