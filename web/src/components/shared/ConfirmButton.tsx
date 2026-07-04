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
          className={`px-3 py-1.5 rounded text-xs font-medium shadow-mycel-sm hover:opacity-90 disabled:opacity-50 transition-opacity focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${
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
          className="px-3 py-1.5 rounded border border-mycel-border text-mycel-muted text-xs hover:text-mycel-text transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg disabled:opacity-50"
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
      className={`px-3 py-1.5 rounded border border-mycel-border text-mycel-muted text-xs hover:text-mycel-error hover:border-mycel-error transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${className ?? ""}`}
    >
      {label}
    </button>
  );
}
