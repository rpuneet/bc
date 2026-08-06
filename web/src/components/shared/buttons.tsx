import type { ButtonHTMLAttributes, ReactNode } from "react";

/** Solid accent CTA — matches Agents `+ New agent` / Templates `+ New template`. */
export const PRIMARY_BTN =
  "inline-flex items-center justify-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg";

/** Bordered secondary / ghost — Health Check, Refresh, Marketplace filters. */
export const SECONDARY_BTN =
  "inline-flex items-center justify-center h-8 px-3 rounded-md text-xs font-medium border border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-text hover:border-mycel-accent transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg";

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
};

export function PrimaryButton({ className = "", children, type = "button", ...rest }: BtnProps) {
  return (
    <button type={type} className={`${PRIMARY_BTN} ${className}`.trim()} {...rest}>
      {children}
    </button>
  );
}

export function SecondaryButton({ className = "", children, type = "button", ...rest }: BtnProps) {
  return (
    <button type={type} className={`${SECONDARY_BTN} ${className}`.trim()} {...rest}>
      {children}
    </button>
  );
}
