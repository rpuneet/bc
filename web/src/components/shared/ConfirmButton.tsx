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
          className={`px-3 py-1.5 rounded text-xs font-medium hover:opacity-90 disabled:opacity-50 transition-opacity focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg ${
            variant === "danger"
              ? "bg-bc-error text-bc-bg"
              : "bg-bc-accent text-white"
          } ${className ?? ""}`}
        >
          {loading ? `${confirmLabel}…` : confirmLabel}
        </button>
        <button
          type="button"
          onClick={() => setConfirming(false)}
          disabled={loading}
          className="px-3 py-1.5 rounded border border-bc-border text-bc-muted text-xs hover:text-bc-text transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg disabled:opacity-50"
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
      className={`px-3 py-1.5 rounded border border-bc-border text-bc-muted text-xs hover:text-bc-error hover:border-bc-error/50 transition-colors focus-visible:ring-2 focus-visible:ring-bc-accent focus-visible:ring-offset-1 focus-visible:ring-offset-bc-bg ${className ?? ""}`}
    >
      {label}
    </button>
  );
}
