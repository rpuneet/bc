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

// Providers whose activity mycel captures for the Live feed: hook-based
// providers (claude, agy) POST lifecycle events; transcript-based providers
// (pi, codex) have their session log tailed by the daemon. Providers not listed
// here expose no readable activity yet, so the stream stays empty — say so
// honestly instead of pretending events are coming.
//
// cursor is deliberately absent: cursor-agent writes an on-disk transcript, but
// it records user prompts and tool *invocations* only — never tool *results* or
// a reliable turn-completion marker — so it cannot yield paired PreToolUse/
// PostToolUse activity (tool calls would appear stuck "running" forever).
const CAPTURE_PROVIDERS = new Set(["claude", "agy", "pi", "codex"]);

export function AgentToolStream({ agentName, agentTool }: AgentToolStreamProps) {
  const { activities, tasks, rawEventsRef } = useAgentActivity(agentName);

  const activity = activities.get(agentName);
  const rawEvents = rawEventsRef.current.get(agentName) ?? [];

  if (!activity) {
    const reportsActivity = !agentTool || CAPTURE_PROVIDERS.has(agentTool);
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <p className="max-w-sm text-center text-sm text-mycel-muted italic leading-relaxed">
          {reportsActivity ? (
            "No activity yet — waiting for events"
          ) : (
            <>
              Live capture isn&rsquo;t available for{" "}
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
