"use client";

import { useEffect, useRef } from "react";

/**
 * Glowing spore network background.
 * 50-60 spores with Brownian motion, proximity connections,
 * mouse attraction, and bloom glow rendering.
 */
export function AnimatedBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    // Check for reduced motion preference
    const prefersReducedMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    if (prefersReducedMotion) return;

    let animationId: number;
    let mouseX = -1;
    let mouseY = -1;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    const SPORE_COUNT = 55;
    const CONNECTION_DISTANCE = 150;
    const MOUSE_RADIUS = 200;
    const MOUSE_ATTRACTION = 0.012;
    const GRID_CELL_SIZE = CONNECTION_DISTANCE;
    let idleFrames = 0;
    const IDLE_THRESHOLD = 120;

    interface Spore {
      x: number;
      y: number;
      vx: number;
      vy: number;
      radius: number;
    }

    let spores: Spore[] = [];
    let width = 0;
    let height = 0;

    function resize() {
      width = window.innerWidth;
      height = window.innerHeight;
      canvas!.width = width * dpr;
      canvas!.height = height * dpr;
      canvas!.style.width = `${width}px`;
      canvas!.style.height = `${height}px`;
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function initSpores() {
      spores = [];
      for (let i = 0; i < SPORE_COUNT; i++) {
        spores.push({
          x: Math.random() * width,
          y: Math.random() * height,
          vx: (Math.random() - 0.5) * 0.4,
          vy: (Math.random() - 0.5) * 0.4,
          radius: 2 + Math.random() * 6,
        });
      }
    }

    function draw() {
      ctx!.clearRect(0, 0, width, height);

      // Update spore positions (Brownian motion)
      for (const s of spores) {
        // Random velocity nudge each frame
        s.vx += (Math.random() - 0.5) * 0.06;
        s.vy += (Math.random() - 0.5) * 0.06;

        // Damping to keep slow
        s.vx *= 0.98;
        s.vy *= 0.98;

        // Clamp velocity
        const maxV = 0.8;
        const speed = Math.sqrt(s.vx * s.vx + s.vy * s.vy);
        if (speed > maxV) {
          s.vx = (s.vx / speed) * maxV;
          s.vy = (s.vy / speed) * maxV;
        }

        // Mouse attraction
        if (mouseX >= 0 && mouseY >= 0) {
          const dx = mouseX - s.x;
          const dy = mouseY - s.y;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < MOUSE_RADIUS && dist > 1) {
            const force = (1 - dist / MOUSE_RADIUS) * MOUSE_ATTRACTION;
            s.vx += dx * force;
            s.vy += dy * force;
          }
        }

        s.x += s.vx;
        s.y += s.vy;

        // Wrap around edges
        if (s.x < -20) s.x = width + 20;
        if (s.x > width + 20) s.x = -20;
        if (s.y < -20) s.y = height + 20;
        if (s.y > height + 20) s.y = -20;
      }

      // Build spatial grid
      const grid = new Map<string, number[]>();
      for (let i = 0; i < spores.length; i++) {
        const cx = Math.floor(spores[i].x / GRID_CELL_SIZE);
        const cy = Math.floor(spores[i].y / GRID_CELL_SIZE);
        const key = `${cx},${cy}`;
        const cell = grid.get(key);
        if (cell) cell.push(i);
        else grid.set(key, [i]);
      }

      // Draw connections between nearby spores
      ctx!.save();
      for (const [key, indices] of grid) {
        const [cx, cy] = key.split(",").map(Number);
        const neighborKeys = [
          key,
          `${cx + 1},${cy}`,
          `${cx},${cy + 1}`,
          `${cx - 1},${cy + 1}`,
          `${cx + 1},${cy + 1}`,
        ];
        for (const nk of neighborKeys) {
          const neighbors = nk === key ? indices : grid.get(nk);
          if (!neighbors) continue;
          for (const i of indices) {
            for (const j of neighbors) {
              if (j <= i) continue;
              const a = spores[i];
              const b = spores[j];
              const dx = a.x - b.x;
              const dy = a.y - b.y;
              const dist = Math.sqrt(dx * dx + dy * dy);
              if (dist < CONNECTION_DISTANCE) {
                const opacity = (1 - dist / CONNECTION_DISTANCE) * 0.25;

                // Bloom pass (thick, faint)
                ctx!.beginPath();
                ctx!.moveTo(a.x, a.y);
                ctx!.lineTo(b.x, b.y);
                ctx!.strokeStyle = `rgba(234, 88, 12, ${opacity * 0.3})`;
                ctx!.lineWidth = 3;
                ctx!.shadowColor = "#EA580C";
                ctx!.shadowBlur = 10;
                ctx!.stroke();

                // Sharp pass (thin, bright)
                ctx!.beginPath();
                ctx!.moveTo(a.x, a.y);
                ctx!.lineTo(b.x, b.y);
                ctx!.strokeStyle = `rgba(234, 88, 12, ${opacity * 0.8})`;
                ctx!.lineWidth = 0.8;
                ctx!.shadowBlur = 0;
                ctx!.stroke();
              }
            }
          }
        }
      }
      ctx!.restore();

      // Draw mouse connections
      if (mouseX >= 0 && mouseY >= 0) {
        for (const s of spores) {
          const dx = mouseX - s.x;
          const dy = mouseY - s.y;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < MOUSE_RADIUS) {
            const opacity = (1 - dist / MOUSE_RADIUS) * 0.2;
            ctx!.beginPath();
            ctx!.moveTo(s.x, s.y);
            ctx!.lineTo(mouseX, mouseY);
            ctx!.strokeStyle = `rgba(253, 186, 116, ${opacity})`;
            ctx!.lineWidth = 0.6;
            ctx!.stroke();
          }
        }
      }

      // Draw spore nodes
      for (const s of spores) {
        // Mouse proximity brightness boost
        let brightnessBoost = 0;
        if (mouseX >= 0 && mouseY >= 0) {
          const dx = mouseX - s.x;
          const dy = mouseY - s.y;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < MOUSE_RADIUS) {
            brightnessBoost = (1 - dist / MOUSE_RADIUS) * 0.5;
          }
        }

        const baseOpacity = 0.5 + brightnessBoost;

        // Glow (radial gradient)
        const glowRadius = s.radius * 4;
        const glow = ctx!.createRadialGradient(
          s.x, s.y, 0,
          s.x, s.y, glowRadius,
        );
        glow.addColorStop(0, `rgba(253, 186, 116, ${(0.15 + brightnessBoost * 0.2)})`);
        glow.addColorStop(1, "rgba(253, 186, 116, 0)");
        ctx!.fillStyle = glow;
        ctx!.beginPath();
        ctx!.arc(s.x, s.y, glowRadius, 0, Math.PI * 2);
        ctx!.fill();

        // Bright core
        ctx!.beginPath();
        ctx!.arc(s.x, s.y, s.radius, 0, Math.PI * 2);
        ctx!.fillStyle = `rgba(251, 146, 60, ${baseOpacity})`;
        ctx!.shadowColor = "#EA580C";
        ctx!.shadowBlur = 8 + brightnessBoost * 16;
        ctx!.fill();
        ctx!.shadowBlur = 0;
      }

      // Adaptive frame rate
      const hasMotion = mouseX >= 0;
      if (hasMotion) {
        idleFrames = 0;
        animationId = requestAnimationFrame(draw);
      } else {
        idleFrames++;
        const delay = idleFrames > IDLE_THRESHOLD ? 66 : 0;
        if (delay > 0) {
          animationId = window.setTimeout(
            () => requestAnimationFrame(draw),
            delay,
          ) as unknown as number;
        } else {
          animationId = requestAnimationFrame(draw);
        }
      }
    }

    function handleMouseMove(e: MouseEvent) {
      mouseX = e.clientX;
      mouseY = e.clientY;
    }

    function handleMouseLeave() {
      mouseX = -1;
      mouseY = -1;
    }

    resize();
    initSpores();
    animationId = requestAnimationFrame(draw);

    window.addEventListener("resize", resize);
    window.addEventListener("mousemove", handleMouseMove, { passive: true });
    document.addEventListener("mouseleave", handleMouseLeave);

    return () => {
      cancelAnimationFrame(animationId);
      clearTimeout(animationId);
      window.removeEventListener("resize", resize);
      window.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseleave", handleMouseLeave);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none fixed inset-0 z-0"
      aria-hidden="true"
    />
  );
}
