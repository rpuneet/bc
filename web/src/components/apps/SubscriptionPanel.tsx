import { useState, useEffect, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../../api/client";
import type { Agent, NotifySubscription } from "../../api/client";
import { getRoleColor } from "./messageUtils";
import { AgentChip } from "../agent-ui";

function AgentRow({
  agent,
  sub,
  loading,
  onSubscribe,
  onUnsubscribe,
  onToggleMention,
  onMute,
  onUnmute,
}: {
  agent: Agent;
  sub?: NotifySubscription;
  loading: boolean;
  onSubscribe: () => void;
  onUnsubscribe: () => void;
  onToggleMention: () => void;
  onMute: () => void;
  onUnmute: () => void;
}) {
  const isStopped = agent.state === "stopped";
  const roleColor = getRoleColor(agent.role);
  const muted = Boolean(sub?.muted);
  const active = Boolean(sub && !sub.muted);

  return (
    <motion.div
      layout
      initial={{ opacity: 0, x: 8 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: -8 }}
      transition={{ duration: 0.12 }}
      className={`px-3 py-2 transition-colors duration-100 hover:bg-mycel-surface-hover ${isStopped && !sub ? "opacity-50" : ""}`}
    >
      <div className="flex items-center gap-2">
        <AgentChip
          name={agent.name}
          state={agent.state}
          size={20}
          className="flex-1 text-mycel-text"
        />
        <span
          className={`text-[8px] px-1.5 py-0.5 rounded-md ${roleColor.bg} ${roleColor.text} font-semibold uppercase tracking-wider shrink-0`}
        >
          {agent.role}
        </span>
      </div>

      <div className="flex items-center gap-1.5 mt-1.5 ml-7">
        {muted ? (
          <>
            <span className="text-[10px] px-2 py-0.5 rounded-md border border-mycel-border text-mycel-muted">
              muted
            </span>
            <button
              type="button"
              onClick={onUnmute}
              disabled={loading}
              className="text-[10px] text-mycel-muted hover:text-mycel-accent transition-colors ml-auto"
            >
              unmute
            </button>
          </>
        ) : active ? (
          <>
            <button
              type="button"
              onClick={onToggleMention}
              className={`text-[10px] px-2 py-0.5 rounded-md border transition-all duration-150 ${
                sub!.mention_only
                  ? "border-mycel-accent bg-mycel-accent-subtle text-mycel-accent"
                  : "border-mycel-border text-mycel-muted hover:border-mycel-border-strong"
              }`}
            >
              {sub!.mention_only ? "@ mentions" : "all msgs"}
            </button>
            <button
              type="button"
              onClick={onUnsubscribe}
              disabled={loading}
              className="text-[10px] text-mycel-muted hover:text-mycel-error transition-colors ml-auto"
            >
              remove
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={onSubscribe}
              disabled={loading}
              className="text-[10px] text-mycel-muted hover:text-mycel-accent transition-colors"
            >
              + subscribe
            </button>
            <button
              type="button"
              onClick={onMute}
              disabled={loading}
              className="text-[10px] text-mycel-muted hover:text-mycel-text transition-colors ml-auto"
              title="Suppress catch-all delivery for this channel"
            >
              mute
            </button>
          </>
        )}
      </div>
    </motion.div>
  );
}

export function SubscriptionPanel({
  channelName,
}: {
  channelName: string;
}) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [subscriptions, setSubscriptions] = useState<NotifySubscription[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchData = useCallback(async () => {
    try {
      const [agentList, subs] = await Promise.all([
        api.listAgents(),
        api.getChannelSubscriptions(channelName),
      ]);
      setAgents(agentList ?? []);
      setSubscriptions(subs ?? []);
    } catch {
      // keep previous state
    }
  }, [channelName]);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 8000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const subMap = new Map<string, NotifySubscription>();
  for (const sub of subscriptions) subMap.set(sub.agent, sub);

  const handleSubscribe = async (agentName: string) => {
    setLoading(true);
    try {
      await api.subscribe(channelName, agentName, false);
      await fetchData();
    } catch { /* */ }
    setLoading(false);
  };

  const handleUnsubscribe = async (agentName: string) => {
    setLoading(true);
    try {
      await api.unsubscribe(channelName, agentName);
      await fetchData();
    } catch { /* */ }
    setLoading(false);
  };

  const handleToggleMention = async (agentName: string, current: boolean) => {
    try {
      await api.setMentionOnly(channelName, agentName, !current);
      await fetchData();
    } catch { /* */ }
  };

  const handleMute = async (agentName: string) => {
    setLoading(true);
    try {
      await api.setMuted(channelName, agentName, true);
      await fetchData();
    } catch { /* */ }
    setLoading(false);
  };

  const handleUnmute = async (agentName: string) => {
    setLoading(true);
    try {
      await api.setMuted(channelName, agentName, false);
      await fetchData();
    } catch { /* */ }
    setLoading(false);
  };

  const agentSortOrder = (agent: { state: string }) => {
    if (agent.state === "working" || agent.state === "running") return 0;
    if (agent.state === "stopped") return 2;
    return 1;
  };

  const listeningAgents = agents
    .filter((a) => {
      const s = subMap.get(a.name);
      return s && !s.muted;
    })
    .sort((a, b) => agentSortOrder(a) - agentSortOrder(b) || a.name.localeCompare(b.name));

  const mutedAgents = agents
    .filter((a) => subMap.get(a.name)?.muted)
    .sort((a, b) => a.name.localeCompare(b.name));

  const availableAgents = agents
    .filter((a) => !subMap.has(a.name))
    .sort((a, b) => agentSortOrder(a) - agentSortOrder(b) || a.name.localeCompare(b.name));

  const rowProps = (agent: Agent) => ({
    agent,
    sub: subMap.get(agent.name),
    loading,
    onSubscribe: () => handleSubscribe(agent.name),
    onUnsubscribe: () => handleUnsubscribe(agent.name),
    onToggleMention: () =>
      handleToggleMention(agent.name, subMap.get(agent.name)?.mention_only ?? false),
    onMute: () => handleMute(agent.name),
    onUnmute: () => handleUnmute(agent.name),
  });

  return (
    <aside
      className="w-56 shrink-0 border-l border-mycel-border flex flex-col bg-mycel-bg"
      style={{ scrollbarWidth: "thin", scrollbarColor: "var(--mycel-scrollbar-thumb) transparent" }}
    >
      <div className="px-3 py-3 border-b border-mycel-border">
        <h3 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
          Agents
        </h3>
      </div>

      <div className="flex-1 overflow-auto">
        <AnimatePresence>
          {listeningAgents.length > 0 && (
            <div>
              <div className="px-3 pt-3 pb-1">
                <div className="flex items-center gap-1.5">
                  <span className="w-1 h-1 rounded-full bg-mycel-success" />
                  <span className="text-[10px] font-medium text-mycel-success uppercase tracking-[0.08em]">
                    Listening ({listeningAgents.length})
                  </span>
                </div>
              </div>
              {listeningAgents.map((agent) => (
                <AgentRow key={agent.name} {...rowProps(agent)} />
              ))}
            </div>
          )}

          {mutedAgents.length > 0 && (
            <div>
              <div className="px-3 pt-3 pb-1">
                <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
                  Muted ({mutedAgents.length})
                </span>
              </div>
              {mutedAgents.map((agent) => (
                <AgentRow key={agent.name} {...rowProps(agent)} />
              ))}
            </div>
          )}

          {(listeningAgents.length > 0 || mutedAgents.length > 0) && availableAgents.length > 0 && (
            <div className="mx-3 my-2 border-t border-mycel-border" />
          )}

          {availableAgents.length > 0 && (
            <div>
              <div className="px-3 pt-2 pb-1">
                <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
                  Available ({availableAgents.length})
                </span>
              </div>
              {availableAgents.map((agent) => (
                <AgentRow key={agent.name} {...rowProps(agent)} />
              ))}
            </div>
          )}
        </AnimatePresence>

        {agents.length === 0 && (
          <div className="p-6 text-center text-xs text-mycel-muted">
            No agents
          </div>
        )}
      </div>
    </aside>
  );
}
