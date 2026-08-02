import type { RevealState } from "../settings/useProgressiveReveal";

/* ── RevealGroup ──────────────────────────────────────────────────────
 *
 * The New Agent modal's take on the same reveal vocabulary Settings uses
 * (locked/active/complete) — see settings/useProgressiveReveal.ts and
 * Settings.tsx's Section. The modal is one continuous form rather than a
 * set of independently-fetched sections, so groups stay mounted and
 * editable in every state (a fast typer can fill fields out of order);
 * `reveal` only changes the group's chrome — a numbered eyebrow, a dimmed
 * "up next" look while locked, an accent highlight while active, and a
 * quiet checkmark once complete.
 */
export function RevealGroup({
  index,
  label,
  reveal,
  optional = false,
  children,
}: {
  index: number;
  label: string;
  reveal: RevealState;
  optional?: boolean;
  children: React.ReactNode;
}) {
  const locked = reveal === "locked";
  const active = reveal === "active";
  const complete = reveal === "complete";

  return (
    <div
      data-reveal={reveal}
      className={`flex flex-col gap-2 rounded-lg transition-opacity ${locked ? "opacity-50" : "opacity-100"} ${
        active ? "ring-1 ring-mycel-accent ring-offset-2 ring-offset-mycel-surface-2 rounded-lg p-2 -m-2" : ""
      }`}
    >
      <div className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
        <span
          className={`grid place-items-center w-4 h-4 rounded-full text-[9px] font-semibold shrink-0 ${
            complete
              ? "bg-mycel-success-subtle text-mycel-success"
              : active
                ? "bg-mycel-accent-subtle text-mycel-accent"
                : "bg-mycel-bg text-mycel-muted border border-mycel-border"
          }`}
          aria-hidden
        >
          {complete ? (
            <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={4} strokeLinecap="round" strokeLinejoin="round"><path d="M20 6L9 17l-5-5" /></svg>
          ) : (
            index
          )}
        </span>
        <span>{label}</span>
        {optional && <span className="normal-case font-normal text-mycel-muted">(optional)</span>}
      </div>
      {children}
    </div>
  );
}
