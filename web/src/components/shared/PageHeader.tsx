import type { ReactNode } from "react";

/**
 * Shared page title chrome for titled surfaces (Settings, About, Readiness,
 * Insights, etc.). Header-slot pages (Home, Agents) stay on useHeaderSlot.
 */
export function PageHeader({
  title,
  subtitle,
  actions,
  eyebrow,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  eyebrow?: ReactNode;
}) {
  return (
    <header
      data-testid="page-header"
      className="flex items-end justify-between gap-4 flex-wrap"
    >
      <div className="min-w-0">
        {eyebrow != null && (
          <div className="text-[11px] uppercase tracking-wider text-mycel-muted mb-1">
            {eyebrow}
          </div>
        )}
        <h1 className="font-display text-[26px] leading-none text-mycel-text">{title}</h1>
        {subtitle != null && (
          <p className="mt-2 text-[13px] text-mycel-text-2 max-w-2xl">{subtitle}</p>
        )}
      </div>
      {actions != null && (
        <div className="flex items-center gap-2 shrink-0">{actions}</div>
      )}
    </header>
  );
}
