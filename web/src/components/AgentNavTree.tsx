/* ── Agent tree (inline in the drawer nav) ──────────────────────────
   Collapsible child list under the Agents nav item — running agents as
   living AgentChips with their current state, click-through to the
   agent detail page. Pattern-matches NotificationNavTree: indent rail,
   light poll, calm rows (fixed height, no layout jank when states
   change). Stopped agents collapse into a single muted count row. */

import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink } from "react-router-dom";
import { api } from "../api/client";
import type { Agent } from "../api/client";
import { AgentChip, subscribeAgentPulse, ANY_AGENT } from "./agent-ui";

const POLL_MS = 15_000;
/** Debounce for SSE-triggered refreshes so bursts coalesce. */
const REFRESH_DEBOUNCE_MS = 600;

export function AgentNavTree() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();

  const fetchAgents = useCallback(async () => {
    try {
      const list = await api.listAgents();
      setAgents(Array.isArray(list) ? list : []);
    } catch {
      // keep previous state
    }
  }, []);

  useEffect(() => {
    void fetchAgents();
    const interval = setInterval(() => void fetchAgents(), POLL_MS);
    // State-change pulses from the shared SSE bus refresh the list
    // sooner than the poll would; debounced so bursts coalesce.
    const unsub = subscribeAgentPulse(ANY_AGENT, (kind) => {
      if (kind !== "state") return;
      clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => void fetchAgents(), REFRESH_DEBOUNCE_MS);
    });
    return () => {
      clearInterval(interval);
      clearTimeout(debounceRef.current);
      unsub();
    };
  }, [fetchAgents]);

  const running = agents.filter((a) => a.state !== "stopped" && a.state !== "error");
  const stoppedCount = agents.length - running.length;

  return (
    <div
      data-testid="agent-nav-tree"
      style={{
        paddingLeft: 10,
        marginLeft: 25,
        borderLeft: "1px solid var(--mycel-border)",
        marginTop: 2,
        marginBottom: 4,
        // Bound to its own scroll region, capped relative to the viewport
        // so a long fleet scrolls in place rather than pushing the Apps
        // tree and footer off the bottom of the drawer.
        maxHeight: "min(280px, 38vh)",
        overflowY: "auto",
      }}
    >
      {running.length === 0 && (
        <div style={{ padding: "3px 8px 5px", fontSize: 11, color: "var(--mycel-muted)", fontStyle: "italic" }}>
          No running agents
        </div>
      )}
      {running.map((a) => (
        <NavLink
          key={a.name}
          to={`/agents/${encodeURIComponent(a.name)}`}
          title={a.task ? `${a.name} — ${a.task}` : a.name}
          style={({ isActive }: { isActive: boolean }) => ({
            display: "flex",
            alignItems: "center",
            height: 26,
            padding: "0 8px",
            borderRadius: 5,
            fontSize: 12.5,
            color: isActive ? "var(--mycel-text)" : "var(--mycel-text-2)",
            background: isActive
              ? "color-mix(in srgb, var(--mycel-accent) 14%, transparent)"
              : "transparent",
            fontWeight: isActive ? 600 : 500,
            cursor: "pointer",
            marginBottom: 1,
            textDecoration: "none",
            minWidth: 0,
          })}
        >
          <AgentChip name={a.name} state={a.state} size={18} className="w-full" preview previewSeed={a} />
        </NavLink>
      ))}
      {stoppedCount > 0 && (
        <NavLink
          to="/agents"
          style={{
            display: "block",
            padding: "3px 8px 5px",
            fontSize: 11,
            color: "var(--mycel-muted)",
            textDecoration: "none",
          }}
        >
          {stoppedCount} stopped
        </NavLink>
      )}
    </div>
  );
}
