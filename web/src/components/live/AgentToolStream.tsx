import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import { useAgentActivity } from "../../hooks/useAgentActivity";
import { formatRelative } from "../../utils/time";
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

/**
 * How long a running agent may report nothing before the empty feed stops
 * saying "waiting" and starts saying "something is wrong".
 *
 * Every provider mycel can capture emits something within seconds of starting
 * — a session-start hook, or the first transcript line. Minutes of total
 * silence from a running agent is not a slow start, it is a broken one.
 */
const SILENCE_IS_SUSPICIOUS_AFTER_MS = 2 * 60 * 1000;

/**
 * Re-renders on an interval so a feed that never receives a single event can
 * still notice that time has passed. Without this, the "waiting for the first
 * event" copy would be frozen forever on exactly the agents that most need to
 * be told something is wrong, because no event ever arrives to re-render it.
 */
function useElapsingTime(active: boolean, everyMs = 30_000): void {
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => setTick((n) => n + 1), everyMs);
    return () => clearInterval(id);
  }, [active, everyMs]);
}

/**
 * What to say when there is nothing to show.
 *
 * An empty feed has several very different causes and they are not
 * interchangeable: a provider mycel cannot observe, an agent that is not
 * running, an agent that just started, and an agent that is running but whose
 * CLI is refusing every turn. The last one is the one that gets reported as
 * "live doesn't work" — the events are missing because the agent is dead, and
 * the only place that says so is its terminal.
 */
function EmptyFeed({
  agentName,
  agentTool,
  agentState,
  startedAt,
  reportsActivity,
}: {
  agentName: string;
  agentTool?: string;
  agentState: string;
  startedAt?: string;
  reportsActivity: boolean | undefined;
}) {
  const running = agentState !== "stopped" && agentState !== "error";
  useElapsingTime(running);

  const startedMs = startedAt ? new Date(startedAt).getTime() : NaN;
  const silentTooLong =
    running &&
    Number.isFinite(startedMs) &&
    Date.now() - startedMs > SILENCE_IS_SUSPICIOUS_AFTER_MS;

  let headline: ReactNode;
  let detail: ReactNode;
  let attachLabel: string | null = null;

  if (reportsActivity === false) {
    headline = <>mycel can&rsquo;t read activity from {agentTool} agents</>;
    detail =
      "This provider offers neither hooks nor a transcript to read, so there is nothing to stream. Its terminal is the whole picture.";
    attachLabel = "Watch the terminal";
  } else if (!running) {
    headline = "This agent isn't running";
    detail =
      "Start it from the header and its prompts, tool calls, and results will appear here.";
  } else if (silentTooLong && reportsActivity !== undefined) {
    headline = <>Nothing reported since this agent started {formatRelative(startedAt)}</>;
    detail =
      "An agent that can't reach its model — missing key, spent quota, a model it isn't entitled to — runs but never works, and reports nothing here. Its terminal will name the reason.";
    attachLabel = "See what the terminal says";
  } else {
    headline = "Waiting for the first event";
    detail =
      "Prompts, tool calls, and results appear here as the agent works.";
  }

  return (
    <div className="flex-1 flex items-center justify-center p-6">
      <div className="max-w-sm flex flex-col gap-2 text-center">
        <p className="text-sm font-medium text-mycel-text">{headline}</p>
        <p className="text-[13px] text-mycel-muted leading-relaxed">{detail}</p>
        {attachLabel && (
          <Link
            to={`/agents/${agentName}/attach`}
            className="mt-1 self-center text-[12px] text-mycel-accent rounded px-2 py-1 ring-1 ring-inset ring-mycel-border hover:bg-mycel-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
          >
            {attachLabel}
          </Link>
        )}
      </div>
    </div>
  );
}

export function AgentToolStream({
  agentName,
  agentTool,
  agentState,
  startedAt,
}: AgentToolStreamProps) {
  const { activities, tasks, rawEventsRef } = useAgentActivity(agentName);
  const reportsActivity = useReportsActivity(agentTool);

  const activity = activities.get(agentName);
  const rawEvents = rawEventsRef.current.get(agentName) ?? [];

  if (!activity) {
    return (
      <EmptyFeed
        agentName={agentName}
        agentTool={agentTool}
        agentState={agentState}
        startedAt={startedAt}
        reportsActivity={reportsActivity}
      />
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
