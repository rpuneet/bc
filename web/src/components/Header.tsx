/**
 * Header.tsx — Shared top bar rendered above every tab.
 *
 * Slots:
 *   left     : sidebar collapse/expand button + WorkspaceDropdown (caller passes these)
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
  /** Left slot — owned by Layout. Typically sidebar toggle + WorkspaceDropdown. */
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
    <header
      className="shrink-0 border-b border-bc-border/40"
      style={{ fontFamily: MONO }}
    >
      <div
        className={`flex items-center gap-2.5 min-w-0 px-6 ${
          compact ? "h-[42px]" : "h-[52px]"
        }`}
      >
        {/* Left slot */}
        {left && <div className="flex items-center gap-2 shrink-0">{left}</div>}

        {/* Center slot — takes remaining space, truncates on overflow */}
        <div className="flex-1 min-w-0 flex items-center gap-2 text-[12px] text-bc-text/90">
          {center}
        </div>

        {/* Right slot — actions, always visible */}
        {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
      </div>
    </header>
  );
}

/**
 * TabHeaderTitle — standard title chip used by most tabs.
 * Use when the page doesn't need anything fancy in the center slot.
 */
export function TabHeaderTitle({ children }: { children: ReactNode }) {
  return (
    <span
      className="text-[13px] font-bold text-bc-text tracking-tight shrink-0 uppercase"
      style={{ fontFamily: MONO }}
    >
      {children}
    </span>
  );
}
