/**
 * BootMark — the boot sequence's mushroom mark.
 *
 * Same silhouette and geometry as {@link BrandMark} (the drawer/header
 * logo), so the rise-into-header hand-off reads as one continuous element.
 * The difference is purely presentational: each part carries a `boot-*`
 * class that boot.css uses to draw the mark in — hyphae trace, cap + stem
 * grow up from the stem base, spores pop — all disabled under
 * `prefers-reduced-motion`, where the mark just appears.
 */

export function BootMark() {
  return (
    <svg viewBox="0 0 512 512" fill="none" aria-hidden="true">
      {/* Hyphae threads reaching down from the stem */}
      <g
        className="boot-hyphae"
        stroke="var(--mycel-accent)"
        strokeWidth="34"
        strokeLinecap="round"
        fill="none"
        opacity="0.7"
      >
        <path d="M256 400 C 235 429 200 436 165 451" />
        <path d="M256 400 C 260 435 280 445 305 459" />
        <path d="M256 400 C 280 423 320 426 353 442" />
      </g>

      {/* Stem */}
      <path
        className="boot-stem"
        d="M224 294 L288 294 L280 382 C 278 403 267 412 256 412 C 245 412 234 403 232 382 Z"
        fill="currentColor"
        opacity="0.8"
      />

      {/* Cap */}
      <path
        className="boot-cap"
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
        <circle className="boot-spore" cx="190" cy="200" r="30" />
        <circle className="boot-spore" cx="280" cy="162" r="25" />
        <circle className="boot-spore" cx="338" cy="230" r="21" />
      </g>

      {/* Drifting spores */}
      <g fill="var(--mycel-accent)">
        <circle className="boot-spore" cx="103" cy="373" r="20" />
        <circle className="boot-spore" cx="412" cy="378" r="20" />
        <circle className="boot-spore" cx="434" cy="330" r="13" opacity="0.75" />
      </g>
    </svg>
  );
}
