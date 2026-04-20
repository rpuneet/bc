"use client";

import { useEffect, useRef, useCallback } from "react";

/**
 * Canvas-based mycelium network — organic branching filaments that grow,
 * pulse with bioluminescent warmth, and respond to mouse proximity.
 * NOT a particle system. This grows fractal branches from a central spore.
 */
export function MyceliumCanvas({ className = "" }: { className?: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const mouseRef = useRef({ x: -1, y: -1 });
  const animRef = useRef<number>(0);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d")!;
    if (!ctx) return;

    const w = canvas.width;
    const h = canvas.height;
    const cx = w / 2;
    const cy = h * 0.4;
    const time = Date.now() * 0.001;
    const mx = mouseRef.current.x;
    const my = mouseRef.current.y;

    ctx.clearRect(0, 0, w, h);

    // Draw organic mycelium branches using recursive fractal
    function drawBranch(
      x: number, y: number, angle: number, length: number,
      thickness: number, depth: number, maxDepth: number, seed: number
    ) {
      if (depth > maxDepth || length < 2) return;

      // Organic wobble based on time and seed
      const wobble = Math.sin(time * 0.5 + seed * 7.3) * 0.15;
      const adjustedAngle = angle + wobble;

      const endX = x + Math.cos(adjustedAngle) * length;
      const endY = y + Math.sin(adjustedAngle) * length;

      // Mouse influence — branches bend toward cursor
      let bendX = 0, bendY = 0;
      if (mx >= 0 && my >= 0) {
        const dx = mx * (w / canvas!.clientWidth) - (x + endX) / 2;
        const dy = my * (h / canvas!.clientHeight) - (y + endY) / 2;
        const dist = Math.sqrt(dx * dx + dy * dy);
        if (dist < 200) {
          const force = (1 - dist / 200) * 0.15;
          bendX = dx * force;
          bendY = dy * force;
        }
      }

      // Glow intensity based on depth (closer to root = brighter)
      const brightness = 1 - (depth / maxDepth) * 0.6;
      const pulse = 0.7 + 0.3 * Math.sin(time * 1.5 + seed * 3.1);
      const alpha = brightness * pulse * 0.8;

      // Draw the branch as a curved line
      ctx.beginPath();
      ctx.moveTo(x, y);
      ctx.quadraticCurveTo(
        (x + endX) / 2 + bendX,
        (y + endY) / 2 + bendY,
        endX, endY
      );
      ctx.strokeStyle = `rgba(234, 88, 12, ${alpha})`;
      ctx.lineWidth = thickness;
      ctx.lineCap = "round";
      ctx.stroke();

      // Glow layer (thicker, more transparent)
      ctx.beginPath();
      ctx.moveTo(x, y);
      ctx.quadraticCurveTo(
        (x + endX) / 2 + bendX,
        (y + endY) / 2 + bendY,
        endX, endY
      );
      ctx.strokeStyle = `rgba(251, 146, 60, ${alpha * 0.3})`;
      ctx.lineWidth = thickness * 3;
      ctx.stroke();

      // Node at junction
      if (depth > 0 && depth % 2 === 0) {
        const nodeR = thickness * 1.2;
        // Bright center
        ctx.beginPath();
        ctx.arc(endX, endY, nodeR, 0, Math.PI * 2);
        ctx.fillStyle = `rgba(251, 146, 60, ${alpha})`;
        ctx.fill();
        // Glow halo
        const grad = ctx.createRadialGradient(endX, endY, 0, endX, endY, nodeR * 4);
        grad.addColorStop(0, `rgba(253, 186, 116, ${alpha * 0.4})`);
        grad.addColorStop(1, "rgba(253, 186, 116, 0)");
        ctx.beginPath();
        ctx.arc(endX, endY, nodeR * 4, 0, Math.PI * 2);
        ctx.fillStyle = grad;
        ctx.fill();
      }

      // Branch out — fractal subdivision
      const nextLen = length * (0.6 + Math.sin(seed * 13.7) * 0.15);
      const nextThick = thickness * 0.7;
      const branchCount = depth < 3 ? 3 : 2;
      const spread = 0.8 + (depth / maxDepth) * 0.4;

      for (let i = 0; i < branchCount; i++) {
        const branchAngle = adjustedAngle + (i - (branchCount - 1) / 2) * spread;
        const branchSeed = seed * 7.1 + i * 3.3 + depth * 1.7;
        drawBranch(
          endX, endY, branchAngle, nextLen,
          nextThick, depth + 1, maxDepth, branchSeed
        );
      }
    }

    // Draw main network — multiple root branches from center
    const rootCount = 7;
    for (let i = 0; i < rootCount; i++) {
      const angle = (i / rootCount) * Math.PI * 2 - Math.PI / 2;
      const seed = i * 17.3;
      drawBranch(cx, cy, angle, 60, 2.5, 0, 6, seed);
    }

    // Central spore — bright glowing core
    const coreGlow = ctx.createRadialGradient(cx, cy, 0, cx, cy, 40);
    coreGlow.addColorStop(0, "rgba(253, 186, 116, 0.6)");
    coreGlow.addColorStop(0.3, "rgba(251, 146, 60, 0.3)");
    coreGlow.addColorStop(0.6, "rgba(234, 88, 12, 0.1)");
    coreGlow.addColorStop(1, "rgba(234, 88, 12, 0)");
    ctx.beginPath();
    ctx.arc(cx, cy, 40, 0, Math.PI * 2);
    ctx.fillStyle = coreGlow;
    ctx.fill();

    // Bright center dot
    ctx.beginPath();
    ctx.arc(cx, cy, 5, 0, Math.PI * 2);
    ctx.fillStyle = "rgba(253, 186, 116, 0.9)";
    ctx.fill();

    animRef.current = requestAnimationFrame(draw);
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    // Reduced motion check
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    function resize() {
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      const ctx = canvas.getContext("2d");
      if (ctx) ctx.scale(dpr, dpr);
    }

    function onMouseMove(e: MouseEvent) {
      const rect = canvas!.getBoundingClientRect();
      mouseRef.current = {
        x: (e.clientX - rect.left) * (canvas!.width / rect.width),
        y: (e.clientY - rect.top) * (canvas!.height / rect.height),
      };
    }

    function onMouseLeave() {
      mouseRef.current = { x: -1, y: -1 };
    }

    resize();
    animRef.current = requestAnimationFrame(draw);

    window.addEventListener("resize", resize);
    canvas.addEventListener("mousemove", onMouseMove);
    canvas.addEventListener("mouseleave", onMouseLeave);

    return () => {
      cancelAnimationFrame(animRef.current);
      window.removeEventListener("resize", resize);
      canvas.removeEventListener("mousemove", onMouseMove);
      canvas.removeEventListener("mouseleave", onMouseLeave);
    };
  }, [draw]);

  return (
    <canvas
      ref={canvasRef}
      className={`w-full ${className}`}
      style={{ height: "320px" }}
      aria-hidden="true"
    />
  );
}
