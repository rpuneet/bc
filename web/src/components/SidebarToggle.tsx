/**
 * SidebarToggle — the single drawer toggle, far left of the full-width
 * header. Panel-left icon (frame + drawer pane); the pane fills when the
 * drawer is open so the button reads as a state, not a back arrow.
 */
export function SidebarToggle({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="shrink-0 w-7 h-7 flex items-center justify-center rounded-md text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
      title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      aria-expanded={!collapsed}
    >
      <svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
        <rect x="1.5" y="2.5" width="13" height="11" rx="2" />
        <path d="M6 2.5v11" />
        {!collapsed && (
          <rect x="2.5" y="3.5" width="2.5" height="9" rx="1" fill="currentColor" stroke="none" opacity="0.45" />
        )}
      </svg>
    </button>
  );
}
