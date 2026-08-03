import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { formatRelative } from "../../utils/time";

/**
 * How long a running agent may report nothing before an empty feed stops
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

export interface EmptyFeedProps {
  agentName: string;
  agentTool?: string;
  agentState: string;
  startedAt?: string;
  /**
   * Whether this agent's provider can report activity at all, from the
   * provider's own declared activity_mode. `undefined` means the answer hasn't
   * arrived yet, and no verdict may be rendered until it has.
   */
  reportsActivity: boolean | undefined;
}

/**
 * What to say when an activity feed has nothing to show.
 *
 * An empty feed has four causes that look identical on screen and call for
 * opposite responses from the reader: a provider mycel cannot observe, an agent
 * that is not running, an agent that just started, and an agent that is running
 * but whose CLI is refusing every turn.
 *
 * The last one is what gets reported as "live doesn't work". The events are
 * missing because the agent is dead — no key, no quota, a model it isn't
 * entitled to — and the only place that says so is its terminal, so the reader
 * has to be sent there rather than told to keep waiting. See #3512.
 */
export function EmptyFeed({
  agentName,
  agentTool,
  agentState,
  startedAt,
  reportsActivity,
}: EmptyFeedProps) {
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
    headline = (
      <>
        Nothing reported since this agent started{" "}
        {/* Always a duration, never a date: the sentence is about how long the
            silence has lasted, and formatRelative's default switch to an
            absolute date past 30 days turns it into "started 2 Jul 2026". */}
        {formatRelative(startedAt, { maxDays: Number.MAX_SAFE_INTEGER })}
      </>
    );
    detail =
      "An agent that can't reach its model — no key, spent quota, a model it isn't entitled to — runs but never works. Its terminal will name the reason.";
    attachLabel = "See what the terminal says";
  } else {
    headline = "Waiting for the first event";
    detail = "Prompts, tool calls, and results appear here as the agent works.";
  }

  return (
    // flex-1 centres this when the parent is a flex column (the no-activity-at-all
    // path); min-h gives it the same presence inside the drill-down's plain
    // scroll container, where flex-1 has nothing to stretch against.
    <div className="flex-1 min-h-[16rem] flex items-center justify-center px-6 py-12">
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
