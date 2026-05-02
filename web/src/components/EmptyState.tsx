interface EmptyStateProps {
  icon?: string;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function EmptyState({
  icon,
  title,
  description,
  actionLabel,
  onAction,
}: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4 text-center">
      {icon && <span className="text-3xl mb-3 opacity-60">{icon}</span>}
      <h3 className="text-sm font-medium text-mycel-text">{title}</h3>
      {description && (
        <p className="mt-1 text-sm text-mycel-muted max-w-sm">{description}</p>
      )}
      {actionLabel && onAction && (
        <button
          onClick={onAction}
          className="mt-4 px-4 py-1.5 bg-mycel-accent text-mycel-bg rounded text-sm font-medium hover:opacity-90"
        >
          {actionLabel}
        </button>
      )}
    </div>
  );
}
