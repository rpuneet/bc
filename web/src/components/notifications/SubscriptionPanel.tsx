import { useState, useEffect, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../../api/client";
import type { Agent, NotifySubscription } from "../../api/client";
import { getRoleColor, agentColor } from "./messageUtils";

function AgentRow({
  agent,
  sub,
  loading,
  onSubscribe,
  onUnsubscribe,
  onToggleMention,
}: {
  agent: Agent;
  sub?: NotifySubscription;
  loading: boolean;
  onSubscribe: () => void;
  onUnsubscribe: () => void;
  onToggleMention: () => void;
}) {
  const isOnline = agent.state === "running" || agent.state === "working";
  const roleColor = getRoleColor(agent.role);
  const nameColor = agentColor(agent.name);

  return (
    <motion.div
      layout
      initial={{ opacity: 0, x: 8 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: -8 }}
      transition={{ duration: 0.12 }}
      className={`px-3 py-2 transition-colors duration-100 ${
        sub ? "hover:bg-bc-surface/30" : "hover:bg-bc-surface/15"
      }`}
    >
      <div className="flex items-center gap-2">
        {/* Avatar initial */}
        <span
          className="w-5 h-5 rounded-md flex items-center justify-center text-[9px] font-bold shrink-0"
          style={{
            backgroundColor: `${nameColor}12`,
            color: nameColor,
          }}
        >
          {agent.name.charAt(0).toUpperCase()}
        </span>

        {/* Name + status */}
        <div className="flex-1 min-w-0 flex items-center gap-1.5">
          <span
            className={`w-1.5 h-1.5 rounded-full shrink-0 ${
              isOnline ? "bg-bc-success" : "bg-bc-muted/20"
            }`}
          />
          <span className="text-[12px] text-bc-text/90 truncate font-medium">
            {agent.name}
          </span>
        </div>

        {/* Role */}
        <span
          className={`text-[8px] px-1.5 py-0.5 rounded-md ${roleColor.bg} ${roleColor.text} font-semibold uppercase tracking-wider shrink-0`}
        >
          {agent.role}
        </span>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1.5 mt-1.5 ml-7">
        {sub ? (
          <>
            <button
              type="button"
              onClick={onToggleMention}
              className={`text-[9px] px-2 py-0.5 rounded-md border transition-all duration-150 ${
                sub.mention_only
                  ? "border-bc-accent/30 bg-bc-accent/8 text-bc-accent"
                  : "border-bc-border/30 text-bc-muted/50 hover:border-bc-border/50 hover:text-bc-muted"
              }`}
            >
              {sub.mention_only ? "@ mentions" : "all msgs"}
            </button>
            <button
              type="button"
              onClick={onUnsubscribe}
              disabled={loading}
              className="text-[9px] text-bc-muted/25 hover:text-bc-error/60 transition-colors ml-auto"
            >
              remove
            </button>
          </>
        ) : (
          <button
            type="button"
            onClick={onSubscribe}
            disabled={loading}
            className="text-[9px] text-bc-muted/30 hover:text-bc-accent transition-colors"
          >
            + subscribe
          </button>
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

  const subscribedAgents = agents.filter((a) => subMap.has(a.name));
  const availableAgents = agents
    .filter((a) => !subMap.has(a.name))
    .sort((a, b) => a.name.localeCompare(b.name));

  return (
    <aside
      className="w-56 shrink-0 border-l border-bc-border/40 flex flex-col bg-bc-bg"
      style={{ scrollbarWidth: "thin", scrollbarColor: "rgba(255,255,255,0.04) transparent" }}
    >
      {/* Header */}
      <div className="px-3 py-3 border-b border-bc-border/30">
        <h3 className="text-[11px] font-bold text-bc-muted/70 uppercase tracking-[0.12em]">
          Agents
        </h3>
      </div>

      <div className="flex-1 overflow-auto">
        <AnimatePresence>
          {/* Subscribed */}
          {subscribedAgents.length > 0 && (
            <div>
              <div className="px-3 pt-3 pb-1">
                <div className="flex items-center gap-1.5">
                  <span className="w-1 h-1 rounded-full bg-bc-success" />
                  <span className="text-[9px] font-bold text-bc-success/70 uppercase tracking-[0.1em]">
                    Listening ({subscribedAgents.length})
                  </span>
                </div>
              </div>
              {subscribedAgents.map((agent) => (
                <AgentRow
                  key={agent.name}
                  agent={agent}
                  sub={subMap.get(agent.name)}
                  loading={loading}
                  onSubscribe={() => handleSubscribe(agent.name)}
                  onUnsubscribe={() => handleUnsubscribe(agent.name)}
                  onToggleMention={() =>
                    handleToggleMention(
                      agent.name,
                      subMap.get(agent.name)?.mention_only ?? false,
                    )
                  }
                />
              ))}
            </div>
          )}

          {/* Divider */}
          {subscribedAgents.length > 0 && availableAgents.length > 0 && (
            <div className="mx-3 my-2 border-t border-bc-border/15" />
          )}

          {/* Available */}
          {availableAgents.length > 0 && (
            <div>
              <div className="px-3 pt-2 pb-1">
                <span className="text-[9px] font-bold text-bc-muted/30 uppercase tracking-[0.1em]">
                  Available ({availableAgents.length})
                </span>
              </div>
              {availableAgents.map((agent) => (
                <AgentRow
                  key={agent.name}
                  agent={agent}
                  loading={loading}
                  onSubscribe={() => handleSubscribe(agent.name)}
                  onUnsubscribe={() => handleUnsubscribe(agent.name)}
                  onToggleMention={() => {}}
                />
              ))}
            </div>
          )}
        </AnimatePresence>

        {agents.length === 0 && (
          <div className="p-6 text-center text-[11px] text-bc-muted/25">
            No agents
          </div>
        )}
      </div>
    </aside>
  );
}
