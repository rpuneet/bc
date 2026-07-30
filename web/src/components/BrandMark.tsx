/**
 * BrandMark — the mycel logo mark: the mushroom.
 *
 * Same silhouette family as the landing's SporeLogo and the desktop app
 * icon (desktop/build/appicon.svg): domed cap, rooted stem, fine hyphae
 * threads reaching down, and drifting spores. The cap + stem render in
 * the current text color (espresso ink on cream, cream ink on espresso)
 * while spores and hyphae carry the chanterelle amber accent — so the
 * mark works in both themes without any theme-specific styling.
 *
 * Geometry is simplified from the 512-unit landing mark for legibility
 * at drawer sizes (16–24px): fewer hyphae, chunkier spores.
 */

export function BrandMark({ size = 20 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 512 512"
      fill="none"
      aria-hidden="true"
      className="shrink-0"
    >
      {/* Hyphae threads reaching down from the stem */}
      <g
        stroke="var(--mycel-accent)"
        strokeWidth="26"
        strokeLinecap="round"
        fill="none"
        opacity="0.6"
      >
        <path d="M256 400 C 235 429 200 436 165 451" />
        <path d="M256 400 C 260 435 280 445 305 459" />
        <path d="M256 400 C 280 423 320 426 353 442" />
      </g>

      {/* Stem */}
      <path
        d="M224 294 L288 294 L280 382 C 278 403 267 412 256 412 C 245 412 234 403 232 382 Z"
        fill="currentColor"
        opacity="0.8"
      />

      {/* Cap */}
      <path
        d="M99 264
           C 99 165 168 109 256 109
           C 344 109 413 165 413 264
           C 413 286 390 294 358 294
           L 154 294
           C 122 294 99 286 99 264 Z"
        fill="currentColor"
      />

      {/* Spore speckles on the cap — chanterelle amber */}
      <g fill="var(--mycel-accent)" opacity="0.95">
        <circle cx="190" cy="200" r="24" />
        <circle cx="278" cy="163" r="20" />
        <circle cx="336" cy="228" r="17" />
      </g>

      {/* Drifting spores */}
      <g fill="var(--mycel-accent)">
        <circle cx="105" cy="373" r="16" />
        <circle cx="410" cy="378" r="16" />
        <circle cx="430" cy="332" r="10" opacity="0.75" />
      </g>
    </svg>
  );
}
