import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { useAgentActivity } from "../../hooks/useAgentActivity";
import { AgentDrillDown } from "./LiveRenderers";

/* ── AgentToolStream ────────────────────────────────────────────────
   Renders a live tool-call stream for a single agent.
   Used in the Activity tab of AgentDetail.
─────────────────────────────────────────────────────────────────── */

interface AgentToolStreamProps {
  agentName: string;
  agentState: string;
  agentTask?: string;
  agentTool?: string;
  createdAt?: string;
  startedAt?: string;
  updatedAt?: string;
  stoppedAt?: string;
}

/**
 * Whether this agent's provider can report activity at all, resolved from the
 * provider's own declared activity_mode rather than a list kept here.
 *
 * A hardcoded list is what made this wrong before: it claimed cursor could never
 * be captured long after that stopped being true, so cursor agents were told to
 * go use the terminal while their events were being ingested. The provider is
 * the only thing that knows how it is observed, so ask it.
 *
 * `undefined` means "not known yet" — the caller must not render a verdict until
 * the answer arrives, or it will flash the wrong empty state.
 */
function useReportsActivity(agentTool?: string): boolean | undefined {
  const [modes, setModes] = useState<Record<string, string> | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .listProviders()
      .then((providers) => {
        if (cancelled) return;
        setModes(
          Object.fromEntries(
            providers.map((p) => [p.name, p.activity_mode ?? "none"]),
          ),
        );
      })
      // A failed lookup must not claim capture is unavailable — an empty map
      // leaves the tool unknown, which reads as "waiting for events".
      .catch(() => {
        if (!cancelled) setModes({});
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!agentTool) return true;
  if (modes === null) return undefined;
  const mode = modes[agentTool];
  // An unrecognized tool is given the benefit of the doubt: it may be a custom
  // claude-compatible wrapper, which mycel does capture.
  return mode === undefined || mode !== "none";
}

export function AgentToolStream({ agentName, agentTool }: AgentToolStreamProps) {
  const { activities, tasks, rawEventsRef } = useAgentActivity(agentName);
  const reportsActivity = useReportsActivity(agentTool);

  const activity = activities.get(agentName);
  const rawEvents = rawEventsRef.current.get(agentName) ?? [];

  if (!activity) {
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <p className="max-w-sm text-center text-sm text-mycel-muted italic leading-relaxed">
          {reportsActivity === false ? (
            <>
              Live capture isn&rsquo;t available for{" "}
              <span className="not-italic font-medium">{agentTool}</span> agents.
              Use the <span className="not-italic font-medium">Attach</span> tab
              to watch this agent&rsquo;s terminal.
            </>
          ) : (
            "No activity yet — waiting for events"
          )}
        </p>
      </div>
    );
  }

  return (
    <div className="p-6 flex flex-col h-full">
      <AgentDrillDown
        activity={activity}
        rawEvents={rawEvents}
        tasks={tasks}
        hideRawStream
      />
    </div>
  );
}
