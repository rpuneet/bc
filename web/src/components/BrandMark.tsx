/**
 * BrandMark — the mycel logo mark.
 *
 * mycel = mycelium: a small geometric network of four nodes joined by
 * thin branching lines (hyphae). The hub node is accent-filled; the
 * satellite nodes and strokes render in the current text color at
 * reduced opacity, so the mark works in both themes without any
 * theme-specific styling.
 */

export function BrandMark({ size = 20 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 20 20"
      fill="none"
      aria-hidden="true"
      className="shrink-0"
    >
      {/* Hyphae — thin branching strokes from the hub */}
      <g stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" opacity="0.4">
        <path d="M8.5 11.5L4.5 15.5" />
        <path d="M9.5 9.5L15 4.5" />
        <path d="M11 11l4.5 2.5" />
      </g>
      {/* Satellite nodes */}
      <circle cx="4.5" cy="15.5" r="1.7" fill="currentColor" opacity="0.55" />
      <circle cx="15" cy="4.5" r="1.5" fill="currentColor" opacity="0.55" />
      <circle cx="15.5" cy="13.5" r="1.5" fill="currentColor" opacity="0.55" />
      {/* Hub node — the accent */}
      <circle cx="10" cy="10.5" r="2.4" fill="var(--mycel-accent)" />
    </svg>
  );
}
