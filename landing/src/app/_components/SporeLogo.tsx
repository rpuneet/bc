"use client";

import { useState } from "react";

/**
 * Animated spore logo — tendrils drift slowly like a fractal,
 * on hover the spore grows outward.
 */
export function SporeLogo({
  size = 28,
  className = "",
}: {
  size?: number;
  className?: string;
}) {
  const [hovered, setHovered] = useState(false);

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
      <style>{`
        @keyframes drift1 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(3px,-2px); } }
        @keyframes drift2 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(-2px,3px); } }
        @keyframes drift3 { 0%,100% { transform: translate(0,0); } 50% { transform: translate(2px,2px); } }
        @keyframes pulse { 0%,100% { r: 16; opacity: 1; } 50% { r: 19; opacity: 0.85; } }
        @keyframes glowPulse { 0%,100% { r: 32; opacity: 0.08; } 50% { r: 40; opacity: 0.12; } }
        .tendril-1 { animation: drift1 8s ease-in-out infinite; }
        .tendril-2 { animation: drift2 10s ease-in-out infinite; }
        .tendril-3 { animation: drift3 12s ease-in-out infinite; }
        .tendril-4 { animation: drift1 9s ease-in-out infinite reverse; }
        .tendril-5 { animation: drift2 11s ease-in-out infinite reverse; }
        .tendril-6 { animation: drift3 7s ease-in-out infinite; }
        .tendril-7 { animation: drift1 13s ease-in-out infinite; }
        .core { animation: pulse 4s ease-in-out infinite; }
        .core-glow { animation: glowPulse 4s ease-in-out infinite; }
        svg:hover .tendril-1,
        svg:hover .tendril-2,
        svg:hover .tendril-3,
        svg:hover .tendril-4,
        svg:hover .tendril-5,
        svg:hover .tendril-6,
        svg:hover .tendril-7 { animation-duration: 2s; }
        svg:hover .core { animation: pulse 1.5s ease-in-out infinite; }
        svg:hover .core-glow { r: 56; opacity: 0.15; transition: all 0.5s; }
      `}</style>

      {/* Tendrils — each group drifts independently */}
      <g stroke="#EA580C" strokeWidth="4" strokeLinecap="round" fill="none">
        <g className="tendril-1">
          <path d="M256 256 Q250 200 240 140 Q235 110 220 85" opacity="0.6"/>
          <path d="M240 140 Q260 130 290 100" opacity="0.35"/>
        </g>
        <g className="tendril-2">
          <path d="M256 256 Q300 220 350 180 Q370 165 395 145" opacity="0.6"/>
          <path d="M350 180 Q360 200 390 210" opacity="0.35"/>
        </g>
        <g className="tendril-3">
          <path d="M256 256 Q310 260 360 280 Q390 290 415 310" opacity="0.6"/>
          <path d="M360 280 Q350 310 370 340" opacity="0.35"/>
        </g>
        <g className="tendril-4">
          <path d="M256 256 Q280 310 300 370 Q310 400 320 425" opacity="0.6"/>
          <path d="M300 370 Q330 365 355 385" opacity="0.35"/>
        </g>
        <g className="tendril-5">
          <path d="M256 256 Q240 310 230 370 Q225 400 215 430" opacity="0.6"/>
          <path d="M230 370 Q200 380 185 405" opacity="0.35"/>
        </g>
        <g className="tendril-6">
          <path d="M256 256 Q200 250 150 235 Q120 225 95 210" opacity="0.6"/>
          <path d="M150 235 Q145 260 120 275" opacity="0.35"/>
        </g>
        <g className="tendril-7">
          <path d="M256 256 Q210 220 170 175 Q150 155 130 130" opacity="0.6"/>
          <path d="M170 175 Q145 180 125 170" opacity="0.35"/>
        </g>
      </g>

      {/* Nodes at tips */}
      <g fill="#EA580C">
        <circle cx="220" cy="85" r="5" className="tendril-1"/>
        <circle cx="290" cy="100" r="4" className="tendril-1"/>
        <circle cx="395" cy="145" r="5" className="tendril-2"/>
        <circle cx="390" cy="210" r="4" className="tendril-2"/>
        <circle cx="415" cy="310" r="5" className="tendril-3"/>
        <circle cx="370" cy="340" r="4" className="tendril-3"/>
        <circle cx="320" cy="425" r="5" className="tendril-4"/>
        <circle cx="355" cy="385" r="4" className="tendril-4"/>
        <circle cx="215" cy="430" r="5" className="tendril-5"/>
        <circle cx="185" cy="405" r="4" className="tendril-5"/>
        <circle cx="95" cy="210" r="5" className="tendril-6"/>
        <circle cx="120" cy="275" r="4" className="tendril-6"/>
        <circle cx="130" cy="130" r="5" className="tendril-7"/>
        <circle cx="125" cy="170" r="4" className="tendril-7"/>
      </g>

      {/* Center spore — pulses */}
      <circle cx="256" cy="256" r="56" fill="#EA580C" opacity="0.04" className="core-glow"/>
      <circle cx="256" cy="256" r="32" fill="#EA580C" opacity="0.08" className="core-glow"/>
      <circle cx="256" cy="256" r="16" fill="#EA580C" className="core"/>
    </svg>
  );
}
