import { useState } from "react";
import { Link } from "react-router-dom";
import { useReadiness } from "../hooks/useReadiness";

/* ── SetupNudge ───────────────────────────────────────────────────────
 *
 * First-run surfacing for the Readiness surface. When the machine is
 * missing essentials to run agents (no runtime, no git, or no agent tool),
 * a calm amber strip invites the user to the setup page. Dismissible for
 * the session, and it stays quiet entirely when everything's ready — no
 * nagging a working install.
 */

const DISMISS_KEY = "mycel-setup-nudge-dismissed";

function wasDismissed(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.sessionStorage.getItem(DISMISS_KEY) === "1";
  } catch {
    return false;
  }
}

export function SetupNudge() {
  const { data, loaded } = useReadiness();
  const [dismissed, setDismissed] = useState(wasDismissed);

  // Only speak up once we know the machine is short of essentials.
  if (!loaded || dismissed || !data || data.overall === "ready") return null;

  const dismiss = () => {
    setDismissed(true);
    try {
      window.sessionStorage.setItem(DISMISS_KEY, "1");
    } catch {
      /* ignore */
    }
  };

  return (
    <div
      role="status"
      className="flex items-center gap-2 px-3 py-1.5 text-[11px] bg-mycel-warning-subtle border-b border-mycel-warning text-mycel-warning"
    >
      <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="shrink-0">
        <circle cx="7" cy="7" r="5.5" />
        <path d="M7 4.2v.01M7 6.4v3.2" strokeLinecap="round" />
      </svg>
      <span className="truncate">
        <span className="font-medium">{data.headline}.</span> {data.subline}
      </span>
      <Link
        to="/settings"
        className="ml-auto shrink-0 underline decoration-dotted underline-offset-2 font-medium hover:opacity-80"
      >
        Resume setup
      </Link>
      <button
        type="button"
        onClick={dismiss}
        className="shrink-0 p-0.5 rounded-md text-mycel-warning hover:opacity-80 transition-colors"
        aria-label="Dismiss setup reminder"
      >
        <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M3 3l8 8M11 3l-8 8" />
        </svg>
      </button>
    </div>
  );
}
