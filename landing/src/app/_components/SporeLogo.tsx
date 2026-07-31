/**
 * mycel brand mark — the mushroom.
 * Same silhouette family as the desktop app icon (desktop/build/appicon.svg):
 * cream cap, rooted stem, fine hyphae threads, drifting spores.
 *
 * Theme-aware: the cap+stem render in the page's ink color (espresso on
 * cream, cream on espresso) while spores and hyphae carry the chanterelle
 * amber accent. Spores drift on a slow loop; motion is disabled globally
 * by the prefers-reduced-motion rule in globals.css.
 */
export function SporeLogo({
  size = 28,
  className = "",
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 512 512"
      width={size}
      height={size}
      className={`spore-mark ${className}`}
      aria-hidden="true"
    >
      <style>{`
        @keyframes spore-drift-a {
          0%, 100% { transform: translate(0, 0); opacity: 0.9; }
          50% { transform: translate(3px, -7px); opacity: 0.55; }
        }
        @keyframes spore-drift-b {
          0%, 100% { transform: translate(0, 0); opacity: 0.7; }
          50% { transform: translate(-4px, -5px); opacity: 1; }
        }
        @keyframes spore-drift-c {
          0%, 100% { transform: translate(0, 0); opacity: 0.6; }
          50% { transform: translate(2px, 6px); opacity: 0.9; }
        }
        .spore-mark .drift-a { animation: spore-drift-a 6s ease-in-out infinite; }
        .spore-mark .drift-b { animation: spore-drift-b 7.5s ease-in-out infinite; }
        .spore-mark .drift-c { animation: spore-drift-c 9s ease-in-out infinite; }
        .spore-mark { transition: transform 400ms cubic-bezier(0.22, 1, 0.36, 1); }
        .spore-mark:hover { transform: scale(1.08) rotate(-2deg); }
      `}</style>

      {/* Hyphae threads reaching down from the stem */}
      <g
        stroke="var(--primary)"
        strokeWidth="12"
        strokeLinecap="round"
        fill="none"
        opacity="0.65"
      >
        <path d="M256 400 C 235 429 200 436 165 451" />
        <path d="M256 400 C 260 435 280 445 305 459" />
        <path d="M256 400 C 280 423 320 426 353 442" />
        <path d="M165 451 C 150 458 140 470 134 480" opacity="0.6" />
        <path d="M353 442 C 370 450 378 463 382 475" opacity="0.6" />
      </g>

      {/* Stem */}
      <path
        d="M224 294 L288 294 L280 382 C 278 403 267 412 256 412 C 245 412 234 403 232 382 Z"
        fill="var(--on-background)"
        opacity="0.82"
      />

      {/* Cap */}
      <path
        d="M99 264
           C 99 165 168 109 256 109
           C 344 109 413 165 413 264
           C 413 286 390 294 358 294
           L 154 294
           C 122 294 99 286 99 264 Z"
        fill="var(--on-background)"
      />

      {/* Spore speckles on the cap — chanterelle amber */}
      <g fill="var(--primary)" opacity="0.9">
        <circle cx="190" cy="200" r="17" />
        <circle cx="274" cy="165" r="14" />
        <circle cx="334" cy="226" r="12" />
      </g>

      {/* Drifting spores */}
      <g fill="var(--primary)">
        <circle className="drift-a" cx="102" cy="371" r="9" />
        <circle className="drift-c" cx="134" cy="410" r="6" opacity="0.75" />
        <circle className="drift-b" cx="411" cy="378" r="9" />
        <circle className="drift-a" cx="432" cy="336" r="6" opacity="0.75" />
        <circle className="drift-c" cx="392" cy="418" r="5" opacity="0.6" />
      </g>
    </svg>
  );
}
