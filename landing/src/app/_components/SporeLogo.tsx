"use client";

import { useState } from "react";

/**
 * Animated spore logo — bioluminescent tendrils with glowing nodes.
 * The dots and lines themselves glow (SVG filter blur), not a haze behind.
 * On hover: tendrils speed up, glow intensifies, spore breathes larger.
 */
export function SporeLogo({
  size = 28,
  className = "",
}: {
  size?: number;
  className?: string;
}) {
  const [hovered, setHovered] = useState(false);

  // Use unique filter IDs to avoid collisions when multiple instances render
  const id = `spore-${size}`;

  return (
    <svg
      viewBox="0 0 512 512"
      width={size}
      height={size}
      className={`transition-transform duration-500 ${hovered ? "scale-110" : ""} ${className}`}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      aria-hidden="true"
    >
      <defs>
        {/* Glow filter — makes strokes and fills bloom like neon */}
        <filter id={`${id}-glow`} x="-50%" y="-50%" width="200%" height="200%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="6" result="blur" />
          <feComposite in="SourceGraphic" in2="blur" operator="over" />
        </filter>
        {/* Stronger glow for nodes */}
        <filter id={`${id}-nodeGlow`} x="-100%" y="-100%" width="300%" height="300%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="8" result="blur1" />
          <feGaussianBlur in="SourceGraphic" stdDeviation="3" result="blur2" />
          <feMerge>
            <feMergeNode in="blur1" />
            <feMergeNode in="blur2" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        {/* Core glow — intense center bloom */}
        <filter id={`${id}-coreGlow`} x="-100%" y="-100%" width="300%" height="300%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="12" result="blur1" />
          <feGaussianBlur in="SourceGraphic" stdDeviation="4" result="blur2" />
          <feMerge>
            <feMergeNode in="blur1" />
            <feMergeNode in="blur2" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>

      <style>{`
        @keyframes drift1 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(3px,-2px); } }
        @keyframes drift2 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(-2px,3px); } }
        @keyframes drift3 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(2px,2px); } }
        @keyframes corePulse { 0%,100% { opacity: 1; } 50% { opacity: 0.7; } }
        @keyframes nodePulse { 0%,100% { opacity: 0.9; } 50% { opacity: 0.6; } }
        .t1 { animation: drift1 8s ease-in-out infinite; }
        .t2 { animation: drift2 10s ease-in-out infinite; }
        .t3 { animation: drift3 12s ease-in-out infinite; }
        .t4 { animation: drift1 9s ease-in-out infinite reverse; }
        .t5 { animation: drift2 11s ease-in-out infinite reverse; }
        .t6 { animation: drift3 7s ease-in-out infinite; }
        .t7 { animation: drift1 13s ease-in-out infinite; }
        .spore-core { animation: corePulse 3s ease-in-out infinite; }
        .spore-node { animation: nodePulse 4s ease-in-out infinite; }
        svg:hover .t1, svg:hover .t2, svg:hover .t3,
        svg:hover .t4, svg:hover .t5, svg:hover .t6,
        svg:hover .t7 { animation-duration: 2s; }
        svg:hover .spore-core { animation-duration: 1s; }
        svg:hover .spore-node { animation-duration: 1.5s; }
      `}</style>

      {/* Tendrils — glowing lines */}
      <g stroke="#EA580C" strokeWidth="5" strokeLinecap="round" fill="none" filter={`url(#${id}-glow)`}>
        <g className="t1">
          <path d="M256 256 Q250 200 240 140 Q235 110 220 85" opacity="0.8"/>
          <path d="M240 140 Q260 130 290 100" opacity="0.5"/>
        </g>
        <g className="t2">
          <path d="M256 256 Q300 220 350 180 Q370 165 395 145" opacity="0.8"/>
          <path d="M350 180 Q360 200 390 210" opacity="0.5"/>
        </g>
        <g className="t3">
          <path d="M256 256 Q310 260 360 280 Q390 290 415 310" opacity="0.8"/>
          <path d="M360 280 Q350 310 370 340" opacity="0.5"/>
        </g>
        <g className="t4">
          <path d="M256 256 Q280 310 300 370 Q310 400 320 425" opacity="0.8"/>
          <path d="M300 370 Q330 365 355 385" opacity="0.5"/>
        </g>
        <g className="t5">
          <path d="M256 256 Q240 310 230 370 Q225 400 215 430" opacity="0.8"/>
          <path d="M230 370 Q200 380 185 405" opacity="0.5"/>
        </g>
        <g className="t6">
          <path d="M256 256 Q200 250 150 235 Q120 225 95 210" opacity="0.8"/>
          <path d="M150 235 Q145 260 120 275" opacity="0.5"/>
        </g>
        <g className="t7">
          <path d="M256 256 Q210 220 170 175 Q150 155 130 130" opacity="0.8"/>
          <path d="M170 175 Q145 180 125 170" opacity="0.5"/>
        </g>
      </g>

      {/* Nodes — glowing dots at tips and junctions */}
      <g fill="#FB923C" filter={`url(#${id}-nodeGlow)`} className="spore-node">
        {/* Tips */}
        <circle cx="220" cy="85" r="6" className="t1"/>
        <circle cx="290" cy="100" r="5" className="t1"/>
        <circle cx="395" cy="145" r="6" className="t2"/>
        <circle cx="390" cy="210" r="5" className="t2"/>
        <circle cx="415" cy="310" r="6" className="t3"/>
        <circle cx="370" cy="340" r="5" className="t3"/>
        <circle cx="320" cy="425" r="6" className="t4"/>
        <circle cx="355" cy="385" r="5" className="t4"/>
        <circle cx="215" cy="430" r="6" className="t5"/>
        <circle cx="185" cy="405" r="5" className="t5"/>
        <circle cx="95" cy="210" r="6" className="t6"/>
        <circle cx="120" cy="275" r="5" className="t6"/>
        <circle cx="130" cy="130" r="6" className="t7"/>
        <circle cx="125" cy="170" r="5" className="t7"/>
        {/* Junctions */}
        <circle cx="240" cy="140" r="5" className="t1" opacity="0.7"/>
        <circle cx="350" cy="180" r="5" className="t2" opacity="0.7"/>
        <circle cx="360" cy="280" r="5" className="t3" opacity="0.7"/>
        <circle cx="300" cy="370" r="5" className="t4" opacity="0.7"/>
        <circle cx="230" cy="370" r="5" className="t5" opacity="0.7"/>
        <circle cx="150" cy="235" r="5" className="t6" opacity="0.7"/>
        <circle cx="170" cy="175" r="5" className="t7" opacity="0.7"/>
      </g>

      {/* Center core — brightest glow */}
      <g filter={`url(#${id}-coreGlow)`} className="spore-core">
        <circle cx="256" cy="256" r="18" fill="#EA580C"/>
        <circle cx="256" cy="256" r="10" fill="#FB923C"/>
        <circle cx="256" cy="256" r="4" fill="#FDBA74"/>
      </g>
    </svg>
  );
}
