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
  createdAt?: string;
  startedAt?: string;
  updatedAt?: string;
  stoppedAt?: string;
}

export function AgentToolStream({ agentName }: AgentToolStreamProps) {
  const { activities, tasks, rawEventsRef } = useAgentActivity(agentName);

  const activity = activities.get(agentName);
  const rawEvents = rawEventsRef.current.get(agentName) ?? [];

  if (!activity) {
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <p className="text-sm text-mycel-muted italic">
          No activity yet — waiting for events
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
