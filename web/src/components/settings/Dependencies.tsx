import { useCallback, useEffect, useRef, useState } from "react";
import { usePolling } from "../../hooks/usePolling";

export interface DepView {
  id: string;
  name: string;
  description: string;
  state: string;
  deprecated: boolean;
  error?: string;
}

interface DepsListResponse {
  deps: DepView[];
}

async function fetchDeps(): Promise<DepView[]> {
  const res = await fetch("/api/deps");
  if (!res.ok) throw new Error(`deps list: ${res.status}`);
  const body = (await res.json()) as DepsListResponse;
  return body.deps ?? [];
}

async function postAction(id: string, action: "start" | "stop"): Promise<void> {
  const res = await fetch(`/api/deps/${encodeURIComponent(id)}/${action}`, {
    method: "POST",
  });
  if (!res.ok) {
    let msg = `${action} failed: ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) msg = body.error;
    } catch {
      // fall through with default message
    }
    throw new Error(msg);
  }
}

function stateColor(state: string, deprecated: boolean): string {
  if (deprecated) return "bg-mycel-muted";
  switch (state) {
    case "running":
      return "bg-mycel-success";
    case "starting":
      return "bg-mycel-warning";
    case "stopping":
      return "bg-mycel-warning";
    case "error":
      return "bg-mycel-error";
    case "stopped":
      return "bg-mycel-muted";
    default:
      return "bg-mycel-muted";
  }
}

function DepIcon({ id }: { id: string }) {
  // Simple SVGs per dep; generic box as fallback.
  const common = "w-5 h-5 text-mycel-muted";
  if (id === "mycel-db") {
    return (
      <svg className={common} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
      </svg>
    );
  }
  if (id === "mycel-code-server") {
    return (
      <svg className={common} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
      </svg>
    );
  }
  if (id === "mycel-browser") {
    return (
      <svg className={common} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <circle cx="12" cy="12" r="9" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 12h18M12 3a15 15 0 010 18M12 3a15 15 0 000 18" />
      </svg>
    );
  }
  return (
    <svg className={common} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
    </svg>
  );
}

interface LogsPanelProps {
  id: string;
}

function LogsPanel({ id }: LogsPanelProps) {
  const [lines, setLines] = useState<string[]>([]);
  const [streamError, setStreamError] = useState<string | null>(null);
  const containerRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    const src = new EventSource(`/api/deps/${encodeURIComponent(id)}/logs?tail=200`);
    src.addEventListener("log", (ev) => {
      try {
        const payload = JSON.parse((ev as MessageEvent).data) as { line?: string };
        if (typeof payload.line === "string") {
          setLines((prev) => {
            const next = prev.concat(payload.line as string);
            return next.length > 500 ? next.slice(next.length - 500) : next;
          });
        }
      } catch {
        // ignore malformed event
      }
    });
    src.addEventListener("error", (ev) => {
      try {
        const payload = JSON.parse((ev as MessageEvent).data ?? "{}") as { error?: string };
        if (payload?.error) setStreamError(payload.error);
      } catch {
        // native EventSource error — state will be resumed
      }
    });
    src.onerror = () => {
      // Browser automatically retries; no action needed here.
    };
    return () => {
      src.close();
    };
  }, [id]);

  useEffect(() => {
    const el = containerRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [lines]);

  return (
    <div className="mt-2">
      {streamError && (
        <div className="text-[11px] text-mycel-error mb-1">{streamError}</div>
      )}
      <pre
        ref={containerRef}
        className="h-40 overflow-auto rounded border border-mycel-border bg-mycel-bg p-2 text-[11px] font-mono text-mycel-text whitespace-pre-wrap"
      >
        {lines.length === 0 ? (
          <span className="text-mycel-muted">(no log lines yet)</span>
        ) : (
          lines.join("\n")
        )}
      </pre>
    </div>
  );
}

function DepCard({ dep, onRefresh }: { dep: DepView; onRefresh: () => void }) {
  const [showLogs, setShowLogs] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const act = useCallback(
    async (action: "start" | "stop") => {
      setBusy(true);
      setActionError(null);
      try {
        await postAction(dep.id, action);
        onRefresh();
      } catch (err) {
        setActionError(err instanceof Error ? err.message : "action failed");
      } finally {
        setBusy(false);
      }
    },
    [dep.id, onRefresh],
  );

  const running = dep.state === "running";
  const starting = dep.state === "starting" || dep.state === "stopping";

  return (
    <div className="rounded border border-mycel-border bg-mycel-surface p-3">
      <div className="flex items-start gap-3">
        <DepIcon id={dep.id} />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-semibold text-mycel-text truncate">{dep.name}</span>
            {dep.deprecated && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-mycel-surface-hover text-mycel-muted uppercase tracking-wide">
                Deprecated
              </span>
            )}
            <span
              className={`inline-block w-2 h-2 rounded-full ${stateColor(dep.state, dep.deprecated)}`}
              title={dep.state}
            />
            <span className="text-[10px] text-mycel-muted capitalize">{dep.state}</span>
          </div>
          <p className="text-[11px] text-mycel-muted mt-1">{dep.description}</p>
          {dep.error && (
            <p className="text-[11px] text-mycel-error mt-1">{dep.error}</p>
          )}
          {actionError && (
            <p className="text-[11px] text-mycel-error mt-1">{actionError}</p>
          )}
          <div className="flex items-center gap-2 mt-2">
            {dep.deprecated ? (
              <button
                type="button"
                disabled
                className="text-[11px] px-2 py-0.5 rounded border border-mycel-border bg-mycel-bg text-mycel-muted cursor-not-allowed"
              >
                Deprecated
              </button>
            ) : running ? (
              <button
                type="button"
                onClick={() => void act("stop")}
                disabled={busy || starting}
                className="text-[11px] px-2 py-0.5 rounded border border-mycel-border bg-mycel-bg text-mycel-text hover:border-mycel-error hover:text-mycel-error disabled:opacity-50"
              >
                {busy ? "Stopping..." : "Stop"}
              </button>
            ) : (
              <button
                type="button"
                onClick={() => void act("start")}
                disabled={busy || starting}
                className="text-[11px] px-2 py-0.5 rounded border border-mycel-border bg-mycel-bg text-mycel-text hover:border-mycel-accent hover:text-mycel-accent disabled:opacity-50"
              >
                {busy ? "Starting..." : "Start"}
              </button>
            )}
            <button
              type="button"
              onClick={() => setShowLogs((v) => !v)}
              className="text-[11px] px-2 py-0.5 rounded border border-mycel-border bg-mycel-bg text-mycel-muted hover:text-mycel-text"
            >
              {showLogs ? "Hide logs" : "Logs"}
            </button>
          </div>
          {showLogs && <LogsPanel id={dep.id} />}
        </div>
      </div>
    </div>
  );
}

export function DependenciesSection() {
  const { data, loading, error, refresh } = usePolling<DepView[]>(fetchDeps, 3000);

  if (loading && !data) {
    return (
      <div className="text-[11px] text-mycel-muted">Loading dependencies...</div>
    );
  }
  if (error && !data) {
    return (
      <div className="text-[11px] text-mycel-error">
        Failed to load dependencies: {error}
      </div>
    );
  }

  const deps = data ?? [];
  if (deps.length === 0) {
    return (
      <div className="text-[11px] text-mycel-muted">
        No optional dependencies configured.
      </div>
    );
  }

  const active = deps.filter((d) => !d.deprecated);
  const deprecated = deps.filter((d) => d.deprecated);

  return (
    <div className="space-y-2">
      {active.map((d) => (
        <DepCard key={d.id} dep={d} onRefresh={refresh} />
      ))}
      {deprecated.length > 0 && (
        <DeprecatedSection deps={deprecated} onRefresh={refresh} />
      )}
    </div>
  );
}

function DeprecatedSection({ deps, onRefresh }: { deps: DepView[]; onRefresh: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="rounded border border-mycel-border bg-mycel-bg">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-2 px-3 py-1.5 text-[11px] text-mycel-muted hover:text-mycel-text transition-colors"
      >
        <svg className={`w-3 h-3 transition-transform ${open ? "rotate-90" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <span className="uppercase tracking-wide">Deprecated</span>
        <span className="text-mycel-muted">({deps.length})</span>
      </button>
      {open && (
        <div className="px-2 pb-2 space-y-2">
          {deps.map((d) => (
            <DepCard key={d.id} dep={d} onRefresh={onRefresh} />
          ))}
        </div>
      )}
    </div>
  );
}
