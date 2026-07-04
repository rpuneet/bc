/**
 * Header.tsx — Shared top bar rendered above every tab.
 *
 * Slots:
 *   left     : sidebar collapse/expand button (caller passes this)
 *   center   : per-tab title / breadcrumb / status
 *   actions  : per-tab primary CTA(s) — filter pills, "Create" button, etc.
 *
 * Each top-level view calls <Header center={...} actions={...} /> from inside
 * its own render; the left slot is owned by the Layout so it stays consistent.
 *
 * Styling matches the monospace HUD look used by AgentDetail.tsx.
 */

import type { ReactNode } from "react";
import { MONO } from "../utils/typography";

export interface HeaderProps {
  /** Left slot — owned by Layout. Typically the sidebar toggle. */
  left?: ReactNode;
  /** Center slot — per-page title / breadcrumb / inline status. */
  center?: ReactNode;
  /** Right slot — per-page actions (buttons, filters, menus). */
  actions?: ReactNode;
  /** Optional wider min-height if the page needs a taller header (default 42px). */
  compact?: boolean;
}

export function Header({ left, center, actions, compact = true }: HeaderProps) {
  return (
    <header className="shrink-0 border-b border-mycel-border bg-[color-mix(in_srgb,var(--mycel-surface)_70%,transparent)] backdrop-blur-sm">
      <div
        className={`flex items-center min-w-0 px-4 sm:px-6 flex-wrap sm:flex-nowrap py-2 sm:py-0 ${
          compact ? "sm:min-h-[48px]" : "sm:min-h-[56px]"
        }`}
      >
        {/* Left slot — sidebar toggle */}
        {left && (
          <div className="flex items-center gap-2 shrink-0">
            {left}
          </div>
        )}

        {/* Hairline separator between left and center — only when both slots are populated */}
        {left && center && (
          <span className="hidden sm:block mx-3 h-4 w-px bg-mycel-border shrink-0" aria-hidden />
        )}

        {/* Center slot — page title / status. Grows to fill; truncates cleanly. */}
        <div className="flex-1 min-w-0 flex items-center gap-2 text-[13px] text-mycel-text">
          {center}
        </div>

        {/* Right slot — actions. Left separator only when there IS a center. */}
        {actions && (
          <>
            {center && (
              <span className="hidden sm:block mx-3 h-4 w-px bg-mycel-border shrink-0" aria-hidden />
            )}
            <div className="flex items-center gap-2 shrink-0">{actions}</div>
          </>
        )}
      </div>
    </header>
  );
}

/**
 * TabHeaderTitle — standard page title chip in the header center slot.
 *
 * Geist Sans (not mono) at 14px semibold. The status pills on either
 * side already use mono; a mono h1 in the
 * middle blurred the visual hierarchy. Using Sans here gives the page
 * title clear prominence as the heading of the row.
 */
export function TabHeaderTitle({ children }: { children: ReactNode }) {
  // MONO import retained for callers that still reference it via prop.
  void MONO;
  return (
    <span
      className="text-[14px] font-semibold text-mycel-text tracking-tight shrink-0"
    >
      {children}
    </span>
  );
}
