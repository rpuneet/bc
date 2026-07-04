/**
 * Header.tsx — the full-width top bar rendered above the drawer + main
 * pane. One continuous strip across the whole viewport.
 *
 * Slots:
 *   left     : the single drawer toggle (owned by Layout)
 *   center   : per-view summary / presence line / breadcrumb — NOT a
 *              page title; the drawer's active nav item already names
 *              the section
 *   actions  : per-view controls — search box, filters, primary CTA
 *
 * Views contribute center/actions through HeaderSlotContext; the left
 * slot is owned by the Layout so the toggle stays consistent.
 */

import type { ReactNode } from "react";

export interface HeaderProps {
  /** Left slot — owned by Layout. The drawer toggle. */
  left?: ReactNode;
  /** Center slot — per-page summary / breadcrumb / inline status. */
  center?: ReactNode;
  /** Right slot — per-page actions (search, buttons, filters, menus). */
  actions?: ReactNode;
}

export function Header({ left, center, actions }: HeaderProps) {
  return (
    <header className="shrink-0 border-b border-mycel-border bg-[color-mix(in_srgb,var(--mycel-surface)_70%,transparent)] backdrop-blur-sm">
      <div className="flex items-center min-w-0 gap-3 px-3 sm:px-4 min-h-[48px]">
        {/* Left slot — the one drawer toggle, far left */}
        {left && <div className="flex items-center shrink-0">{left}</div>}

        {/* Center slot — per-view summary / presence. Grows to fill. */}
        <div className="flex-1 min-w-0 flex items-center gap-2 text-sm text-mycel-text">
          {center}
        </div>

        {/* Right slot — per-view actions */}
        {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
      </div>
    </header>
  );
}
