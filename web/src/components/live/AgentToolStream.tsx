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

// Providers that emit lifecycle/tool hooks the Live feed is built from
// (ActivityMode: hooks). Other providers don't report activity yet, so the
// stream stays empty — say so honestly instead of "waiting for events".
const HOOK_PROVIDERS = new Set(["claude", "agy"]);

export function AgentToolStream({ agentName, agentTool }: AgentToolStreamProps) {
  const { activities, tasks, rawEventsRef } = useAgentActivity(agentName);

  const activity = activities.get(agentName);
  const rawEvents = rawEventsRef.current.get(agentName) ?? [];

  if (!activity) {
    const reportsActivity = !agentTool || HOOK_PROVIDERS.has(agentTool);
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <p className="max-w-sm text-center text-sm text-mycel-muted italic leading-relaxed">
          {reportsActivity ? (
            "No activity yet — waiting for events"
          ) : (
            <>
              Live activity isn&rsquo;t reported by{" "}
              <span className="not-italic font-medium">{agentTool}</span> agents
              yet. Use the <span className="not-italic font-medium">Attach</span>{" "}
              tab to watch this agent&rsquo;s terminal.
            </>
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
