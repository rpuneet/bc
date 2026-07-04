import { useState } from "react";

/**
 * Two-step confirmation button. First click switches to confirm mode,
 * second click fires onConfirm. Includes a cancel button in confirm mode.
 */
export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  loading,
  variant = "danger",
  className,
}: {
  label: string;
  confirmLabel: string;
  onConfirm: () => void;
  loading?: boolean;
  variant?: "danger" | "default";
  className?: string;
}) {
  const [confirming, setConfirming] = useState(false);

  if (confirming) {
    return (
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => {
            onConfirm();
          }}
          disabled={loading}
          className={`h-8 px-3 inline-flex items-center rounded-md text-xs font-medium shadow-mycel-sm hover:opacity-90 disabled:opacity-50 transition-opacity focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${
            variant === "danger"
              ? "bg-mycel-error text-white"
              : "bg-mycel-accent text-mycel-accent-fg"
          } ${className ?? ""}`}
        >
          {loading ? `${confirmLabel}…` : confirmLabel}
        </button>
        <button
          type="button"
          onClick={() => setConfirming(false)}
          disabled={loading}
          className="h-8 px-3 inline-flex items-center rounded-md border border-mycel-border bg-mycel-surface text-mycel-text-2 text-xs hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg disabled:opacity-50"
        >
          Cancel
        </button>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={() => setConfirming(true)}
      className={`h-8 px-3 inline-flex items-center rounded-md border text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${
        variant === "danger"
          ? "border-mycel-border text-mycel-error hover:bg-mycel-error-subtle hover:border-mycel-error"
          : "border-mycel-border bg-mycel-surface text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover"
      } ${className ?? ""}`}
    >
      {label}
    </button>
  );
}
