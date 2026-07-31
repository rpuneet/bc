/**
 * A lightweight, styled message-thread mock (teardown #14) that makes the
 * "reachable where you already talk" claim tangible. No screenshot, no images —
 * just brand-styled markup: a human ping to an agent and its reply, in the shape
 * of a Slack/WhatsApp thread. Purely decorative, so it's aria-hidden.
 */

function Avatar({ label, agent = false }: { label: string; agent?: boolean }) {
  return (
    <span
      className={`flex h-7 w-7 shrink-0 select-none items-center justify-center rounded-full text-[11px] font-semibold ${
        agent
          ? "bg-primary/15 text-primary-text"
          : "bg-surface-container-highest text-on-surface-variant"
      }`}
    >
      {label}
    </span>
  );
}

export function ChatThread() {
  return (
    <div
      data-testid="apps-thread"
      aria-hidden="true"
      className="rounded-xl border border-outline-variant/20 bg-surface-container/40 p-4 shadow-[0_20px_50px_-30px_rgba(23,17,12,0.6)] backdrop-blur-sm sm:p-5"
    >
      {/* Channel header — reads like Slack */}
      <div className="flex items-center gap-2 border-b border-outline-variant/15 pb-3">
        <span className="font-label text-[13px] font-semibold text-on-surface">
          #ship-room
        </span>
        <span className="rounded-full bg-surface-container-highest/60 px-1.5 py-0.5 font-label text-[10px] text-on-surface-variant">
          Slack
        </span>
        <span className="ml-auto flex items-center gap-1.5 font-label text-[10px] text-on-surface-variant">
          <span className="h-1.5 w-1.5 rounded-full bg-success" />
          connected
        </span>
      </div>

      {/* Human ping */}
      <div className="mt-4 flex items-start gap-2.5">
        <Avatar label="PR" />
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <span className="font-label text-[12px] font-semibold text-on-surface">
              Puneet
            </span>
            <span className="font-label text-[10px] text-on-surface-variant">
              9:41
            </span>
          </div>
          <p className="mt-1 font-body text-[13px] leading-relaxed text-on-surface-variant">
            <span className="font-medium text-primary-text">@sage</span> the
            checkout flow is throwing on empty carts — can you take a look?
          </p>
        </div>
      </div>

      {/* Agent reply */}
      <div className="mt-4 flex items-start gap-2.5">
        <Avatar label="🍄" agent />
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <span className="font-label text-[12px] font-semibold text-on-surface">
              sage
            </span>
            <span className="rounded bg-primary/12 px-1 py-px font-label text-[9px] font-semibold uppercase tracking-wide text-primary-text">
              agent
            </span>
            <span className="font-label text-[10px] text-on-surface-variant">
              9:41
            </span>
          </div>
          <p className="mt-1 font-body text-[13px] leading-relaxed text-on-surface-variant">
            On it. Reproduced the crash — a missing guard in{" "}
            <code className="rounded bg-surface-container-highest/60 px-1 py-px font-label text-[11px] text-on-surface">
              cart.total()
            </code>
            . Fix is up on a branch, opening a PR now. 🌱
          </p>
          {/* Typing / working indicator */}
          <div className="mt-2 inline-flex items-center gap-1.5 rounded-full bg-surface-container-highest/50 px-2 py-1">
            <span className="flex items-center gap-0.5">
              <span className="h-1 w-1 animate-pulse rounded-full bg-primary/70" />
              <span className="h-1 w-1 animate-pulse rounded-full bg-primary/70 [animation-delay:150ms]" />
              <span className="h-1 w-1 animate-pulse rounded-full bg-primary/70 [animation-delay:300ms]" />
            </span>
            <span className="font-label text-[10px] text-on-surface-variant">
              sage is running tests
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
