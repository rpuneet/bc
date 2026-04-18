import { useState, useEffect, useCallback } from "react";
import type { Agent, Channel, GatewayStatus, NotifySubscription } from "../../api/client";
import { api } from "../../api/client";
import { channelPlatform, agentColor, getRoleColor } from "./messageUtils";
import { SetupWizard } from "./SetupWizard";

const PLATFORM_META: Record<string, { label: string; color: string }> = {
  slack: { label: "Slack", color: "#E01E5A" },
  telegram: { label: "Telegram", color: "#26A5E4" },
  discord: { label: "Discord", color: "#5865F2" },
  github: { label: "GitHub", color: "#8B949E" },
  gmail: { label: "Gmail", color: "#EA4335" },
};

function getMeta(p: string) {
  return PLATFORM_META[p] ?? { label: p, color: "#8c7e72" };
}

function displayName(name: string): string {
  const idx = name.indexOf(":");
  return idx > 0 ? name.slice(idx + 1) : name;
}

export function ChannelSidebar({
  channels,
  selected,
  onSelect,
}: {
  channels: Channel[];
  selected: string | null;
  onSelect: (name: string) => void;
}) {
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [allSubs, setAllSubs] = useState<NotifySubscription[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);
  const [view, setView] = useState<"channels" | "agents">("channels");
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
  const channelSubs = new Map<string, Set<string>>();
  for (const sub of allSubs) {
    subCountMap.set(sub.channel, (subCountMap.get(sub.channel) ?? 0) + 1);
    if (!channelSubs.has(sub.channel)) channelSubs.set(sub.channel, new Set());
    channelSubs.get(sub.channel)!.add(sub.agent);
  }

  // Build gateway buckets
  const gwMap = new Map<string, GatewayStatus>();
  for (const gw of gateways) gwMap.set(gw.platform, gw);

  const bucketMap = new Map<string, Channel[]>();
  for (const ch of channels) {
    const p = channelPlatform(ch.name);
    if (p === "internal") continue;
    const list = bucketMap.get(p) ?? [];
    list.push(ch);
    bucketMap.set(p, list);
  }
  for (const gw of gateways) {
    if (!bucketMap.has(gw.platform)) bucketMap.set(gw.platform, []);
  }

  const configuredPlatforms = new Set(bucketMap.keys());
  const unconfigured = Object.keys(PLATFORM_META).filter(p => !configuredPlatforms.has(p));

  // Agents for current channel
  const currentChannelAgents = selected ? channelSubs.get(selected) ?? new Set() : new Set();

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
          onClick={() => setView("channels")}
          className={`flex-1 py-2.5 text-[10px] font-bold uppercase tracking-[0.12em] transition-colors ${
            view === "channels"
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
          Agents {currentChannelAgents.size > 0 && (
            <span className="text-bc-success ml-1">{currentChannelAgents.size}</span>
          )}
        </button>
      </div>

      <div className="flex-1 overflow-auto py-1">
        {view === "channels" ? (
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
                {unconfigured.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => setSetupPlatform(p)}
                    className="w-full flex items-center gap-2 py-1 text-[10px] text-bc-muted/20 hover:text-bc-muted/40 transition-colors"
                  >
                    <span className="w-1.5 h-1.5 rounded-full bg-bc-muted/10" />
                    <span className="uppercase tracking-[0.08em] font-medium">{getMeta(p).label}</span>
                    <span className="ml-auto opacity-50">+</span>
                  </button>
                ))}
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
                {agents.filter(a => currentChannelAgents.has(a.name)).length > 0 && (
                  <div>
                    <div className="px-3 pt-2 pb-1">
                      <span className="text-[9px] font-bold text-bc-success/60 uppercase tracking-[0.08em]">
                        Listening ({agents.filter(a => currentChannelAgents.has(a.name)).length})
                      </span>
                    </div>
                    {agents.filter(a => currentChannelAgents.has(a.name)).map((agent) => {
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
                {agents.filter(a => currentChannelAgents.has(a.name)).length > 0 &&
                  agents.filter(a => !currentChannelAgents.has(a.name)).length > 0 && (
                  <div className="mx-3 my-1.5 border-t border-bc-border/15" />
                )}

                {/* Available agents */}
                <div>
                  <div className="px-3 pt-2 pb-1">
                    <span className="text-[9px] font-bold text-bc-muted/30 uppercase tracking-[0.08em]">
                      Add to notifications
                    </span>
                  </div>
                  {agents.filter(a => !currentChannelAgents.has(a.name)).sort((a, b) => a.name.localeCompare(b.name)).map((agent) => {
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
          <div className="bg-bc-bg border border-bc-border/50 rounded-xl p-5 max-w-sm w-full mx-4 shadow-2xl">
            <h2 className="text-[14px] font-semibold text-bc-text mb-4">Connect an app</h2>
            <div className="grid grid-cols-2 gap-2">
              {Object.entries(PLATFORM_META).map(([key, meta]) => (
                <button key={key} type="button" onClick={() => setSetupPlatform(key)}
                  className="p-3 border border-bc-border/30 rounded-lg hover:border-bc-border/50 hover:bg-bc-surface/20 transition-all text-left group"
                >
                  <div className="w-6 h-6 rounded flex items-center justify-center text-[11px] font-bold mb-1.5"
                    style={{ backgroundColor: `${meta.color}12`, color: meta.color }}
                  >
                    {meta.label.charAt(0)}
                  </div>
                  <span className="text-[11px] text-bc-muted/50 group-hover:text-bc-text">{meta.label}</span>
                </button>
              ))}
            </div>
            <button type="button" onClick={() => setSetupPlatform(null)}
              className="mt-3 w-full py-1.5 text-[10px] text-bc-muted/30 hover:text-bc-text transition-colors"
            >Cancel</button>
          </div>
        </div>
      )}
    </nav>
  );
}
