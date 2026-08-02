/**
 * Header.tsx — the full-width top bar rendered above the drawer + main
 * pane. One continuous strip across the whole viewport, single h-12 row.
 *
 * Slots:
 *   left     : the brand / drawer column — mycel mark + wordmark + the
 *              drawer toggle, sized to match the drawer's width and
 *              collapsing with it. Owned by Layout, rendered flush so its
 *              right border lines up with the drawer's. On mobile it is
 *              just the drawer opener while the overlay drawer is closed.
 *   center   : per-view summary / presence line / breadcrumb — NOT a
 *              page title; the drawer's active nav item already names
 *              the section
 *   actions  : per-view controls (search box, filters, primary CTA)
 *
 * Views contribute center/actions through HeaderSlotContext; the left
 * slot is owned by the Layout.
 *
 * `relative z-30` is load-bearing: `backdrop-blur` forces a stacking
 * context, and without an explicit z-index the positioned content
 * wrapper below (`relative`) paints OVER anything overflowing the
 * header — dropdowns anchored in the header rendered behind the main
 * pane and read as broken. z-30 keeps the header (and its popovers)
 * above the content while staying below the mobile drawer overlay
 * (z-40) and the drawer itself (z-50).
 */

import type { ReactNode } from "react";

export interface HeaderProps {
  /** Left slot — owned by Layout. Drawer toggle + brand. */
  left?: ReactNode;
  /** Nav slot — the one back/forward history control. Owned by Layout,
   *  always rendered (not per-view) immediately after the brand column. */
  nav?: ReactNode;
  /** Center slot — per-page summary / breadcrumb / inline status. */
  center?: ReactNode;
  /** Right slot — per-page actions (search, buttons, filters, menus). */
  actions?: ReactNode;
}

export function Header({ left, nav, center, actions }: HeaderProps) {
  return (
    <header className="relative z-30 shrink-0 border-b border-mycel-border bg-[color-mix(in_srgb,var(--mycel-surface)_70%,transparent)] backdrop-blur-sm">
      {/* items-stretch so the brand column's right border spans the full
          row height and continues the drawer's border below it. */}
      <div className="flex items-stretch h-12">
        {/* Left slot — the brand/drawer column. Rendered FLUSH (no row
            padding) and self-contained: it brings its own width, padding
            and right border so it sits directly above the drawer and
            shares its edge. */}
        {left}

        {/* Per-view region — everything the drawer column doesn't own.
            Padded so its content aligns above the main pane. */}
        <div className="flex flex-1 items-center min-w-0 gap-1 sm:gap-2 px-3 sm:px-4">
          {/* Nav slot — back/forward, unconditional across every view. */}
          {nav}

          {/* Center slot — per-view summary / presence. */}
          <div className="flex-1 min-w-0 flex items-center gap-2 text-sm text-mycel-text">
            {center}
          </div>

          {/* Right slot — per-view actions. flex-1 so search inputs inside
              can flex-grow toward their max-w caps; justify-end keeps
              button-only views pinned right. */}
          {actions && (
            <div className="flex flex-1 items-center justify-end gap-2 min-w-0">
              {actions}
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
