import { Link } from "react-router-dom";
import { useWorkspace } from "../hooks/useWorkspace";

/**
 * Quiet note when the daemon has no default repo for new agents.
 *
 * mycel up does not require a project directory — MycelHome (~/.mycel) is
 * enough. A missing daemon "workspace" only means Create Agent will not
 * pre-fill a repo; the agent still gets an explicit repo path. Do not tell
 * people to re-run mycel up from a git repo (#3560 product model).
 */
export function NoWorkspaceBanner() {
  const { loaded, hasWorkspace } = useWorkspace();
  if (!loaded || hasWorkspace) return null;

  return (
    <div
      role="status"
      className="flex items-center gap-2 px-3 py-1.5 text-[11px] bg-mycel-surface-2 border-b border-mycel-border text-mycel-muted"
    >
      <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="shrink-0" aria-hidden>
        <circle cx="7" cy="7" r="5.5" />
        <path d="M7 4.2v.01M7 6.4v3.2" strokeLinecap="round" />
      </svg>
      <span className="truncate">
        <span className="font-medium text-mycel-text-2">No default repo.</span>{" "}
        The daemon is fine — new agents need a repository path when you create
        them (Browse on Create Agent).
      </span>
      <Link
        to="/agents"
        className="ml-auto shrink-0 underline decoration-dotted underline-offset-2 font-medium text-mycel-text-2 hover:text-mycel-text"
      >
        Agents
      </Link>
    </div>
  );
}
