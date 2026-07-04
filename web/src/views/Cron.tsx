import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CronJob, CronLogEntry } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
import { formatRelative } from "../utils/time";
// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type FormStatus =
  | { type: "idle" }
  | { type: "saving" }
  | { type: "success" }
  | { type: "error"; message: string };

function describeCron(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return expr;
  const min = parts[0]!;
  const hour = parts[1]!;
  const dom = parts[2]!;
  const mon = parts[3]!;
  const dow = parts[4]!;

  // Every N minutes
  if (
    min.startsWith("*/") &&
    hour === "*" &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  )
    return `Every ${min.slice(2)} minutes`;

  // Every N hours
  if (
    min === "0" &&
    hour.startsWith("*/") &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  )
    return `Every ${hour.slice(2)} hours`;

  // Daily at HH:00
  if (
    min === "0" &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    const h = parseInt(hour, 10);
    const ampm = h >= 12 ? "PM" : "AM";
    const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
    return `Daily at ${h12}:00 ${ampm}`;
  }

  // Daily at HH:MM
  if (
    /^\d+$/.test(min) &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    const h = parseInt(hour, 10);
    const ampm = h >= 12 ? "PM" : "AM";
    const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
    return `Daily at ${h12}:${min.padStart(2, "0")} ${ampm}`;
  }

  // Weekly on Sunday
  if (min === "0" && hour === "0" && dom === "*" && mon === "*" && dow === "0")
    return "Weekly on Sunday";

  // Weekly on Monday
  if (min === "0" && hour === "0" && dom === "*" && mon === "*" && dow === "1")
    return "Weekly on Monday";

  // Every minute
  if (min === "*" && hour === "*" && dom === "*" && mon === "*" && dow === "*")
    return "Every minute";

  // Hourly
  if (min === "0" && hour === "*" && dom === "*" && mon === "*" && dow === "*")
    return "Hourly";

  return expr;
}

function relativeTime(dateStr: string | null): string {
  return formatRelative(dateStr, {
    emptyLabel: "never",
    allowFuture: true,
    maxDays: Number.POSITIVE_INFINITY,
  });
}

// ---------------------------------------------------------------------------
// Create Job Form
// ---------------------------------------------------------------------------

function CreateJobForm({ onCreated, onCancel }: { onCreated: () => void; onCancel: () => void }) {
  const [name, setName] = useState("");
  const [schedule, setSchedule] = useState("");
  const [command, setCommand] = useState("");
  const [status, setStatus] = useState<FormStatus>({ type: "idle" });

  const reset = () => {
    setName("");
    setSchedule("");
    setCommand("");
    setStatus({ type: "idle" });
  };

  const handleSubmit = async () => {
    if (!name.trim() || !schedule.trim() || !command.trim()) return;
    setStatus({ type: "saving" });
    try {
      await api.createCron({
        name: name.trim(),
        schedule: schedule.trim(),
        command: command.trim(),
      });
      setStatus({ type: "success" });
      reset();
      onCreated();
    } catch (err) {
      setStatus({
        type: "error",
        message: err instanceof Error ? err.message : "Failed to create job",
      });
      setTimeout(() => setStatus({ type: "idle" }), 4000);
    }
  };

  const inputCls =
    "w-full px-3 py-2 rounded border border-mycel-border bg-mycel-bg text-mycel-text text-sm focus:outline-none focus:ring-2 focus:ring-mycel-accent";

  const cronPreview = schedule.trim() ? describeCron(schedule.trim()) : null;
  const isRawCron = cronPreview === schedule.trim();

  return (
    <div className="rounded border border-mycel-border bg-mycel-surface p-5 space-y-4">
      <h2 className="text-sm font-medium text-mycel-muted uppercase tracking-wide">
        New Cron Job
      </h2>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="space-y-1">
          <label className="block text-xs text-mycel-muted uppercase tracking-wide">
            Name
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="deploy-main"
            className={inputCls}
          />
        </div>
        <div className="space-y-1">
          <label className="block text-xs text-mycel-muted uppercase tracking-wide">
            Schedule (cron)
          </label>
          <input
            type="text"
            value={schedule}
            onChange={(e) => setSchedule(e.target.value)}
            placeholder="*/10 * * * *"
            className={inputCls}
          />
          {cronPreview && !isRawCron && (
            <p className="text-xs text-mycel-accent mt-1">{cronPreview}</p>
          )}
        </div>
        <div className="space-y-1">
          <label className="block text-xs text-mycel-muted uppercase tracking-wide">
            Command
          </label>
          <input
            type="text"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder="make check"
            className={inputCls}
          />
        </div>
      </div>
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={handleSubmit}
          disabled={
            status.type === "saving" ||
            !name.trim() ||
            !schedule.trim() ||
            !command.trim()
          }
          className="px-4 py-2 rounded bg-mycel-accent text-mycel-accent-fg text-sm font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          {status.type === "saving" ? "Creating..." : "Create"}
        </button>
        <button
          type="button"
          onClick={() => {
            reset();
            onCancel();
          }}
          className="px-4 py-2 rounded border border-mycel-border text-mycel-muted text-sm hover:text-mycel-text transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          Cancel
        </button>
        {status.type === "error" && (
          <span className="text-xs text-mycel-error">{status.message}</span>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Live Logs (SSE)
// ---------------------------------------------------------------------------

function LiveLogs({ name }: { name: string }) {
  const [output, setOutput] = useState("");
  const preRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    const es = new EventSource(
      `/api/cron/${encodeURIComponent(name)}/logs/live`,
    );
    es.onmessage = (e: MessageEvent) => {
      setOutput((prev) => prev + (e.data as string));
    };
    es.addEventListener("done", () => {
      es.close();
    });
    es.onerror = () => {
      es.close();
    };
    return () => es.close();
  }, [name]);

  useEffect(() => {
    if (preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight;
    }
  }, [output]);

  return (
    <div className="rounded border border-mycel-info bg-mycel-info-subtle p-3">
      <div className="flex items-center gap-2 text-xs text-mycel-info mb-2">
        <span className="inline-block w-2 h-2 rounded-full bg-mycel-info animate-pulse" />
        <span className="font-medium">Live Output</span>
      </div>
      <pre
        ref={preRef}
        className="text-xs text-mycel-text-2 whitespace-pre-wrap max-h-48 overflow-y-auto font-mono bg-mycel-bg rounded p-3"
      >
        {output || "Waiting for output..."}
      </pre>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Run Detail (expanded log entry)
// ---------------------------------------------------------------------------

function RunDetail({ log }: { log: CronLogEntry }) {
  return (
    <div className="mt-2 rounded border border-mycel-border bg-mycel-bg p-3">
      <div className="flex items-center gap-4 text-xs text-mycel-muted mb-2">
        <span>Duration: {log.duration_ms}ms</span>
        <span
          className={`font-medium ${log.status === "success" ? "text-mycel-success" : log.status === "failed" ? "text-mycel-error" : "text-mycel-warning"}`}
        >
          {log.status}
        </span>
      </div>
      <pre className="text-xs text-mycel-text-2 whitespace-pre-wrap max-h-64 overflow-y-auto font-mono bg-mycel-bg rounded p-3">
        {log.output || "(no output)"}
      </pre>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Job Runs (execution history panel)
// ---------------------------------------------------------------------------

function JobRuns({ name, running }: { name: string; running: boolean }) {
  const fetcher = useCallback(() => api.getCronLogs(name), [name]);
  const { data: logs, loading } = usePolling(fetcher, running ? 5000 : 15000);
  const [selectedRun, setSelectedRun] = useState<number | null>(null);

  if (loading && !logs) {
    return (
      <div className="py-3 text-xs text-mycel-muted">Loading runs...</div>
    );
  }

  if (!logs || (logs.length === 0 && !running)) {
    return (
      <div className="py-3 text-xs text-mycel-muted">
        No runs yet. Job will execute on next scheduled tick.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {running && <LiveLogs name={name} />}
      <div className="text-xs font-medium text-mycel-muted uppercase tracking-wide">
        Execution History
      </div>
      <div className="max-h-80 overflow-y-auto space-y-1">
        {(logs ?? []).map((log: CronLogEntry) => {
          const isSelected = selectedRun === log.id;
          return (
            <div key={log.id}>
              <button
                type="button"
                onClick={() =>
                  setSelectedRun(isSelected ? null : log.id)
                }
                className={`w-full flex items-center gap-4 text-xs px-3 py-2 rounded transition-colors text-left ${
                  isSelected
                    ? "bg-mycel-accent-subtle text-mycel-accent"
                    : "hover:bg-mycel-surface-hover"
                }`}
              >
                <span className="text-mycel-muted whitespace-nowrap w-36">
                  {log.run_at
                    ? new Date(log.run_at).toLocaleString()
                    : "\u2014"}
                </span>
                <span
                  className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold uppercase ${
                    log.status === "success"
                      ? "bg-mycel-success-subtle text-mycel-success"
                      : log.status === "failed"
                        ? "bg-mycel-error-subtle text-mycel-error"
                        : "bg-mycel-warning-subtle text-mycel-warning"
                  }`}
                >
                  {log.status}
                </span>
                <span className="text-mycel-muted w-16">
                  {log.duration_ms}ms
                </span>
                <span className="text-mycel-muted truncate flex-1 font-mono">
                  {log.output
                    ? log.output.split("\n")[0]?.slice(0, 80)
                    : "(no output)"}
                </span>
              </button>
              {isSelected && <RunDetail log={log} />}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Inline Delete Confirmation
// ---------------------------------------------------------------------------

function DeleteConfirm({
  jobName,
  onConfirm,
  onCancel,
}: {
  jobName: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="flex items-center gap-2 bg-mycel-error-subtle border border-mycel-error rounded px-3 py-2">
      <span className="text-xs text-mycel-error">
        Delete <strong>{jobName}</strong>?
      </span>
      <button
        type="button"
        onClick={onConfirm}
        className="px-2 py-0.5 rounded text-xs font-medium bg-mycel-error text-white hover:opacity-90 shadow-mycel-sm transition-opacity focus-visible:ring-2 focus-visible:ring-mycel-error"
      >
        Confirm
      </button>
      <button
        type="button"
        onClick={onCancel}
        className="px-2 py-0.5 rounded text-xs font-medium border border-mycel-border text-mycel-muted hover:text-mycel-text transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent"
      >
        Cancel
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Job Card
// ---------------------------------------------------------------------------

function JobCard({
  job,
  expanded,
  onToggleExpand,
  onRefresh,
}: {
  job: CronJob;
  expanded: boolean;
  onToggleExpand: () => void;
  onRefresh: () => void;
}) {
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const act = async (action: string, fn: () => Promise<unknown>) => {
    setBusy(action);
    try {
      await fn();
      onRefresh();
    } catch {
      // Silently fail, user sees stale state until next poll
    } finally {
      setBusy(null);
    }
  };

  // Left bar color
  const barColor = job.running
    ? "bg-mycel-info animate-pulse"
    : job.enabled
      ? "bg-mycel-success"
      : "bg-mycel-border";

  const humanSchedule = describeCron(job.schedule);
  const isHuman = humanSchedule !== job.schedule;

  return (
    <div
      className={`rounded border border-mycel-border bg-mycel-surface shadow-mycel overflow-hidden transition-colors ${
        expanded ? "ring-1 ring-mycel-accent" : ""
      }`}
    >
      {/* Card body — clickable to expand */}
      <button
        type="button"
        onClick={onToggleExpand}
        className="w-full text-left flex"
        aria-label={expanded ? `Collapse ${job.name}` : `Expand ${job.name}`}
      >
        {/* Left colored bar */}
        <div className={`w-1 shrink-0 ${barColor}`} />

        <div className="flex-1 min-w-0 p-4">
          {/* Top row: name + schedule */}
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="text-sm font-bold text-mycel-text truncate">
                {job.name}
              </h3>
              <div className="flex items-center gap-2 mt-0.5">
                {isHuman && (
                  <span className="text-xs text-mycel-accent">
                    {humanSchedule}
                  </span>
                )}
                <code className="text-[11px] text-mycel-muted font-mono">
                  {job.schedule}
                </code>
              </div>
            </div>

            {/* Running indicator in top right */}
            {job.running && (
              <span className="flex items-center gap-1.5 text-xs text-mycel-info shrink-0">
                <span className="inline-block w-1.5 h-1.5 rounded-full bg-mycel-info animate-pulse" />
                Running...
              </span>
            )}
          </div>

          {/* Command preview */}
          <div className="mt-2">
            <code className="text-xs text-mycel-text-2 font-mono bg-mycel-bg rounded px-2 py-1 inline-block max-w-full truncate">
              {job.command}
            </code>
          </div>

          {/* Bottom stats row */}
          <div className="flex items-center gap-4 mt-3 text-[11px] text-mycel-muted">
            <span title={job.next_run ? new Date(job.next_run).toLocaleString() : undefined}>
              Next: {relativeTime(job.next_run)}
            </span>
            <span title={job.last_run ? new Date(job.last_run).toLocaleString() : undefined}>
              Last: {relativeTime(job.last_run)}
            </span>
            <span>
              Runs: {job.run_count}
            </span>
          </div>
        </div>
      </button>

      {/* Action buttons — outside the clickable area */}
      <div className="flex items-center gap-2 px-4 pb-3 pl-5">
        {/* Toggle pill */}
        <button
          type="button"
          onClick={() =>
            act("toggle", () =>
              job.enabled
                ? api.disableCron(job.name)
                : api.enableCron(job.name),
            )
          }
          disabled={busy !== null}
          aria-label={
            job.enabled
              ? `Disable cron job ${job.name}`
              : `Enable cron job ${job.name}`
          }
          className={`px-3 py-1 rounded-full text-xs font-semibold transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${
            job.enabled
              ? "bg-mycel-success-subtle text-mycel-success"
              : "bg-mycel-surface-hover text-mycel-text-2"
          }`}
        >
          {busy === "toggle"
            ? "..."
            : job.enabled
              ? "Enabled"
              : "Disabled"}
        </button>

        {/* Run Now */}
        {job.enabled && (
          <button
            type="button"
            onClick={() => act("run", () => api.runCron(job.name))}
            disabled={busy !== null}
            aria-label={`Run cron job ${job.name} now`}
            className="px-3 py-1 rounded text-xs font-medium bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
          >
            {busy === "run" ? "..." : "Run Now"}
          </button>
        )}

        {/* Delete */}
        {!confirmingDelete ? (
          <button
            type="button"
            onClick={() => setConfirmingDelete(true)}
            disabled={busy !== null}
            aria-label={`Delete cron job ${job.name}`}
            className="px-3 py-1 rounded text-xs font-medium bg-mycel-error-subtle text-mycel-error hover:bg-mycel-error hover:text-white transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
          >
            {busy === "delete" ? "..." : "Delete"}
          </button>
        ) : (
          <DeleteConfirm
            jobName={job.name}
            onConfirm={() => {
              setConfirmingDelete(false);
              act("delete", () => api.deleteCron(job.name));
            }}
            onCancel={() => setConfirmingDelete(false)}
          />
        )}
      </div>

      {/* Expanded panel: execution logs */}
      {expanded && (
        <div className="border-t border-mycel-border bg-mycel-bg px-5 py-4">
          <JobRuns name={job.name} running={job.running} />
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Export
// ---------------------------------------------------------------------------

export function Cron() {
  const fetcher = useCallback(() => api.listCron(), []);
  const anyRunning = (jobs: CronJob[] | null) =>
    jobs?.some((j) => j.running) ?? false;

  const {
    data: jobs,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, anyRunning(null) ? 5000 : 10000);

  const [expandedJob, setExpandedJob] = useState<string | null>(null);
  const [showCreateForm, setShowCreateForm] = useState(false);

  useHeaderSlot({
    title: <TabHeaderTitle>Cron</TabHeaderTitle>,
    actions: (
      <>
        {jobs && (
          <span className="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-mycel-accent-subtle text-mycel-accent text-[11px] font-semibold tabular-nums">
            {jobs.length}
          </span>
        )}
        <button
          type="button"
          onClick={() => setShowCreateForm((v) => !v)}
          className="px-3 py-1 rounded text-[11px] font-medium border border-mycel-accent bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors"
          aria-label="Create new cron job"
        >
          + New Job
        </button>
      </>
    ),
  });

  // Auto-expand running jobs
  useEffect(() => {
    if (jobs) {
      const running = jobs.find((j) => j.running);
      if (running && expandedJob === null) {
        setExpandedJob(running.name);
      }
    }
  }, [jobs, expandedJob]);

  if (loading && !jobs) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-6 w-28 animate-pulse rounded bg-mycel-surface-hover" />
        <LoadingSkeleton variant="table" rows={3} />
      </div>
    );
  }
  if (timedOut && !jobs) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Cron jobs took too long to load"
          description="The server may be unavailable. Check your connection and try again."
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }
  if (error && !jobs) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load cron jobs"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  const jobList = jobs ?? [];

  return (
    <div className="p-6 space-y-5">
      {/* Create form */}
      {showCreateForm && (
        <CreateJobForm
          onCreated={() => {
            refresh();
            setShowCreateForm(false);
          }}
          onCancel={() => setShowCreateForm(false)}
        />
      )}

      {/* Job cards */}
      {jobList.length === 0 ? (
        <EmptyState
          icon="clock"
          title="No cron jobs"
          description="Click '+ New Job' above or use 'mycel cron add <name>' to schedule recurring tasks."
        />
      ) : (
        <div className="grid grid-cols-1 gap-3">
          {jobList.map((j) => (
              <JobCard
                key={j.name}
                job={j}
                expanded={expandedJob === j.name}
                onToggleExpand={() =>
                  setExpandedJob((prev) =>
                    prev === j.name ? null : j.name,
                  )
                }
                onRefresh={refresh}
              />
          ))}
        </div>
      )}
    </div>
  );
}
