import type { ReactNode } from "react";

interface EmptyStateProps {
  /**
   * Optional iconography. Accepts:
   *   - a ReactNode (preferred — render an SVG / lucide icon directly)
   *   - a short string token: ">" / "*" / "~" / "#" / "!" — mapped to a built-in SVG glyph.
   * String tokens are kept for back-compat; new code should pass an SVG.
   */
  icon?: ReactNode;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
}

/* Inline SVGs replace the previous ASCII placeholders (">" "*" "~"). */
function GlyphIcon({ kind }: { kind: string }) {
  const stroke = "currentColor";
  const common = { width: 32, height: 32, viewBox: "0 0 24 24", fill: "none", stroke, strokeWidth: 1.5, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };
  switch (kind) {
    case ">":
      // Inbox / list — generic empty list
      return (
        <svg {...common} aria-hidden>
          <path d="M22 12h-6l-2 3h-4l-2-3H2" />
          <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11Z" />
        </svg>
      );
    case "*":
      // Sparkle — generic absence-of-results
      return (
        <svg {...common} aria-hidden>
          <path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1" />
        </svg>
      );
    case "~":
      // Pulse / activity
      return (
        <svg {...common} aria-hidden>
          <path d="M3 12h4l2-7 4 14 2-7h6" />
        </svg>
      );
    case "#":
      // Hash / channel
      return (
        <svg {...common} aria-hidden>
          <path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18" />
        </svg>
      );
    case "!":
      // Alert
      return (
        <svg {...common} aria-hidden>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 8v4M12 16h.01" />
        </svg>
      );
    case "clock":
      // Clock face + hour/minute hands — schedule / cron / recurring
      return (
        <svg {...common} aria-hidden>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5l3 2" />
        </svg>
      );
    case "T":
      // Stack of two cards — templates
      return (
        <svg {...common} aria-hidden>
          <rect x="6" y="4" width="14" height="12" rx="1.5" />
          <rect x="4" y="8" width="14" height="12" rx="1.5" />
          <path d="M8 12h6M8 15h4" opacity="0.5" />
        </svg>
      );
    default:
      return null;
  }
}

export function EmptyState({
  icon,
  title,
  description,
  actionLabel,
  onAction,
}: EmptyStateProps) {
  let iconNode: ReactNode = null;
  if (typeof icon === "string") {
    iconNode = <GlyphIcon kind={icon} />;
  } else if (icon) {
    iconNode = icon;
  }
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4 text-center">
      {iconNode && (
        <span className="mb-3 text-mycel-muted" aria-hidden>
          {iconNode}
        </span>
      )}
      <h3 className="text-sm font-medium text-mycel-text">{title}</h3>
      {description && (
        <p className="mt-1 text-xs text-mycel-muted max-w-sm">{description}</p>
      )}
      {actionLabel && onAction && (
        <button
          onClick={onAction}
          className="mt-4 h-9 px-3 inline-flex items-center bg-mycel-accent text-mycel-accent-fg rounded-md text-sm font-medium shadow-mycel-sm hover:bg-mycel-accent-hover transition-colors"
        >
          {actionLabel}
        </button>
      )}
    </div>
  );
}
