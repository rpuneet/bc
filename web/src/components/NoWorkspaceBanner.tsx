import { Link } from "react-router-dom";
import { useWorkspace } from "../hooks/useWorkspace";

/**
 * Shown when the daemon is up but serving no workspace (#3569 leftover).
 * Without a workspace, new agents have no default repo and the UI used to
 * look like a working install with an empty fleet.
 */
export function NoWorkspaceBanner() {
  const { loaded, hasWorkspace } = useWorkspace();
  if (!loaded || hasWorkspace) return null;

  return (
    <div
      role="status"
      className="flex items-center gap-2 px-3 py-1.5 text-[11px] bg-mycel-warning-subtle border-b border-mycel-warning text-mycel-warning"
    >
      <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="shrink-0" aria-hidden>
        <circle cx="7" cy="7" r="5.5" />
        <path d="M7 4.2v.01M7 6.4v3.2" strokeLinecap="round" />
      </svg>
      <span className="truncate">
        <span className="font-medium">No workspace.</span> This daemon is not
        anchored to a repo — from a project directory run{" "}
        <span className="font-mono">mycel up</span>
        {" "}(or <span className="font-mono">mycel up --workspace /path/to/repo</span>).
      </span>
      <Link
        to="/settings"
        className="ml-auto shrink-0 underline decoration-dotted underline-offset-2 font-medium hover:opacity-80"
      >
        Settings
      </Link>
    </div>
  );
}
