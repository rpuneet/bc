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
    >
      <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
        {collapsed ? (
          <>
            <path d="M3 7h8" />
            <path d="M7 3l4 4-4 4" />
          </>
        ) : (
          <>
            <path d="M11 7H3" />
            <path d="M7 3L3 7l4 4" />
          </>
        )}
      </svg>
    </button>
  );
}
