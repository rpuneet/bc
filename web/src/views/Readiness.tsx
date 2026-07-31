import { CopyButton } from "../components/CopyButton";
import { useReadiness } from "../hooks/useReadiness";
import type {
  OverallStatus,
  RStatus,
  Readiness,
  ReadinessGroup,
  ReadinessItem,
} from "./readiness/readiness";

/* ── Readiness view ───────────────────────────────────────────────────
 *
 * The in-app "System Readiness" surface. Turns the daemon's doctor report
 * into plain guidance for a desktop-app user who never opens a terminal:
 * what their machine needs to run agents, and the exact command to get it.
 * Grouped into Runtime / Git / Agent tools with per-item pass·warn·fail
 * state, copy-to-clipboard fixes, and an overall verdict up top.
 */

const STATUS_DOT: Record<RStatus, string> = {
  ok: "bg-mycel-success",
  warn: "bg-mycel-warning",
  fail: "bg-mycel-error",
};

const STATUS_TEXT: Record<RStatus, string> = {
  ok: "text-mycel-success",
  warn: "text-mycel-warning",
  fail: "text-mycel-error",
};

const OVERALL_STYLE: Record<OverallStatus, { ring: string; bg: string; dot: string; text: string }> = {
  ready: {
    ring: "ring-mycel-success",
    bg: "bg-mycel-success-subtle",
    dot: "bg-mycel-success",
    text: "text-mycel-success",
  },
  almost: {
    ring: "ring-mycel-warning",
    bg: "bg-mycel-warning-subtle",
    dot: "bg-mycel-warning",
    text: "text-mycel-warning",
  },
  setup: {
    ring: "ring-mycel-error",
    bg: "bg-mycel-error-subtle",
    dot: "bg-mycel-error",
    text: "text-mycel-error",
  },
};

function StatusDot({ status, className = "" }: { status: RStatus; className?: string }) {
  return (
    <span
      className={`inline-flex shrink-0 w-2 h-2 rounded-full ${STATUS_DOT[status]} ${className}`}
      aria-hidden
    />
  );
}

/* Inline-code markers in notes (`claude`) → styled <code>. Keeps the note
   strings readable in source while rendering a real mono chip. */
function renderNote(note: string) {
  const parts = note.split(/`([^`]+)`/g);
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <code
        key={i}
        className="font-mono text-[11px] px-1 py-0.5 rounded bg-mycel-surface-hover text-mycel-text"
      >
        {part}
      </code>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

function ItemRow({ item }: { item: ReadinessItem }) {
  return (
    <div className="flex flex-col gap-1.5 px-4 py-3">
      <div className="flex items-start gap-2.5">
        <StatusDot status={item.status} className="mt-1.5" />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2 flex-wrap">
            <span className="text-[13px] text-mycel-text font-medium">{item.label}</span>
            <span className={`text-[11px] ${item.status === "ok" ? "text-mycel-muted" : STATUS_TEXT[item.status]} truncate`}>
              {item.detail}
            </span>
          </div>
          {item.note && (
            <p className="mt-0.5 text-[11px] text-mycel-muted leading-relaxed">
              {renderNote(item.note)}
            </p>
          )}
        </div>
      </div>
      {item.fix && (
        <div className="ml-[18px] flex items-center gap-1.5 rounded-md border border-mycel-border bg-mycel-bg pl-2.5 pr-1 py-1">
          <code className="flex-1 min-w-0 font-mono text-[11px] text-mycel-text overflow-x-auto whitespace-nowrap">
            {item.fix}
          </code>
          <CopyButton text={item.fix} />
        </div>
      )}
    </div>
  );
}

function GroupCard({ group }: { group: ReadinessGroup }) {
  return (
    <section className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden shadow-mycel">
      <header className="flex items-start gap-2.5 px-4 py-3 border-b border-mycel-border bg-mycel-bg">
        <StatusDot status={group.status} className="mt-1.5" />
        <div className="min-w-0">
          <h2 className="text-[13px] font-semibold text-mycel-text">{group.title}</h2>
          <p className="mt-0.5 text-[11px] text-mycel-muted leading-relaxed">{group.summary}</p>
        </div>
      </header>
      <div className="divide-y divide-mycel-border">
        {group.items.map((it) => (
          <ItemRow key={it.key} item={it} />
        ))}
      </div>
    </section>
  );
}

function OverallBanner({ data }: { data: Readiness }) {
  const s = OVERALL_STYLE[data.overall];
  return (
    <div className={`rounded-lg ${s.bg} ring-1 ring-inset ${s.ring} px-4 py-3.5 flex items-start gap-3`}>
      <span className="relative flex h-2.5 w-2.5 shrink-0 mt-1">
        {data.overall === "ready" && (
          <span className={`absolute inline-flex h-full w-full rounded-full ${s.dot} opacity-50 animate-ping [animation-duration:2.5s]`} />
        )}
        <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${s.dot}`} />
      </span>
      <div className="min-w-0">
        <p className={`text-[15px] font-semibold ${s.text}`}>{data.headline}</p>
        <p className="mt-0.5 text-[12px] text-mycel-text-2 leading-relaxed">{data.subline}</p>
      </div>
    </div>
  );
}

export function Readiness() {
  const { data, loading, loaded, error, refresh } = useReadiness();

  return (
    <div className="p-6 flex flex-col gap-5 max-w-2xl mx-auto w-full">
      <header className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-mycel-text">System readiness</h1>
          <p className="mt-0.5 text-[12px] text-mycel-muted">
            What your machine needs to run agents — and how to get it.
          </p>
        </div>
        <button
          type="button"
          onClick={() => void refresh()}
          disabled={loading}
          className="shrink-0 text-[11px] px-2.5 py-1 rounded border border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-muted hover:text-mycel-text transition-colors disabled:opacity-50"
        >
          {loading ? "Checking…" : "Re-check"}
        </button>
      </header>

      {error && !data && (
        <div
          role="alert"
          className="rounded-md border border-mycel-border bg-mycel-error-subtle px-3 py-2 text-xs text-mycel-error"
        >
          Couldn't reach the daemon to check readiness: {error}
        </div>
      )}

      {!loaded && !data && !error && (
        <div className="text-sm text-mycel-muted py-8 text-center">Checking your machine…</div>
      )}

      {data && (
        <>
          <OverallBanner data={data} />
          <div className="flex flex-col gap-3">
            {data.groups.map((g) => (
              <GroupCard key={g.id} group={g} />
            ))}
          </div>
          <footer className="text-[11px] text-mycel-muted leading-relaxed">
            mycel runs agents in tmux by default; Docker enables isolated containers. You need git
            for worktrees and at least one signed-in agent tool. Re-check after installing anything —
            no restart required.
          </footer>
        </>
      )}
    </div>
  );
}
