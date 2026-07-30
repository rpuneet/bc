"use client";

import { useEffect, useRef } from "react";

/**
 * Drifting spore field — the living background of the page.
 *
 * Small soft-glow particles wander on eased sine paths (two incommensurate
 * frequencies per axis, so the drift never visibly repeats). Occasionally a
 * spore "germinates": a fine hyphae thread grows along a curve toward a
 * nearby spore, holds for a breath, then fades away.
 *
 * Performance: capped particle count, spatial work is O(n), adaptive frame
 * rate (~15fps when idle), rendering pauses entirely when the tab is hidden.
 * prefers-reduced-motion renders a single static faint spore field.
 */
export function AnimatedBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let animationId = 0;
    let scrollY = 0;
    let prevScrollY = 0;
    let scrollVelocity = 0;
    let mouseX = -1;
    let mouseY = -1;
    let hidden = document.hidden;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    const SPORE_COUNT = 42;
    const LAVENDER_EVERY = 7; // every 7th spore whispers lavender
    const MOUSE_RADIUS = 200;
    const MOUSE_STRENGTH = 0.012;
    let idleFrames = 0;
    const IDLE_THRESHOLD = 120; // ~2s at 60fps before throttling

    /* Germination — a hyphae thread grows toward a neighbour, then fades */
    const GERMINATE_MIN_GAP = 4000; // ms between germinations
    const GERMINATE_MAX_GAP = 8000;
    const THREAD_GROW_MS = 1700;
    const THREAD_HOLD_MS = 600;
    const THREAD_FADE_MS = 1100;
    const THREAD_REACH = 260;

    interface Spore {
      x: number; // anchor position
      y: number;
      z: number; // depth 0..1 (1 = closest)
      r: number; // base radius
      // organic wander: two sine frequencies + phases per axis
      ax1: number;
      ax2: number;
      ay1: number;
      ay2: number;
      px1: number;
      px2: number;
      py1: number;
      py2: number;
      fx1: number;
      fx2: number;
      fy1: number;
      fy2: number;
      ox: number; // accumulated offset from mouse/scroll influence
      oy: number;
      // current drawn position (anchor + wander + offset)
      dx: number;
      dy: number;
      twinklePhase: number;
    }

    interface Thread {
      from: number;
      to: number;
      start: number; // timestamp
      // fixed control point so the curve doesn't swim while growing
      cx: number;
      cy: number;
    }

    let spores: Spore[] = [];
    let threads: Thread[] = [];
    let nextGerminate = performance.now() + 2500;
    let width = 0;
    let height = 0;

    const rand = (a: number, b: number) => a + Math.random() * (b - a);

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
        const z = Math.random();
        spores.push({
          x: Math.random() * width,
          y: Math.random() * height,
          z,
          r: rand(0.8, 2.1) * (0.6 + z * 0.7),
          ax1: rand(10, 26),
          ax2: rand(4, 10),
          ay1: rand(8, 22),
          ay2: rand(3, 9),
          px1: rand(0, Math.PI * 2),
          px2: rand(0, Math.PI * 2),
          py1: rand(0, Math.PI * 2),
          py2: rand(0, Math.PI * 2),
          fx1: rand(0.04, 0.09),
          fx2: rand(0.11, 0.19),
          fy1: rand(0.03, 0.08),
          fy2: rand(0.1, 0.17),
          ox: 0,
          oy: 0,
          dx: 0,
          dy: 0,
          twinklePhase: rand(0, Math.PI * 2),
        });
      }
      threads = [];
    }

    /* Palette per theme. Light mode uses the deep chanterelle cut so faint
     * spores stay visible on cream; dark mode glows bright amber. */
    function palette() {
      const isDark = document.documentElement.classList.contains("dark");
      return isDark
        ? {
            amber: "232, 163, 61",
            lavender: "169, 151, 189",
            glowPeak: 0.09,
            sporeAlpha: 0.5,
            threadAlpha: 0.3,
          }
        : {
            amber: "163, 93, 10",
            lavender: "141, 122, 158",
            glowPeak: 0.06,
            sporeAlpha: 0.42,
            threadAlpha: 0.26,
          };
    }

    function sporeColor(i: number, colors: ReturnType<typeof palette>) {
      return i % LAVENDER_EVERY === LAVENDER_EVERY - 1
        ? colors.lavender
        : colors.amber;
    }

    function maybeGerminate(now: number) {
      if (now < nextGerminate || threads.length >= 2) return;
      // pick a random spore and its nearest neighbour within reach
      const from = Math.floor(Math.random() * spores.length);
      let best = -1;
      let bestDist = THREAD_REACH;
      for (let i = 0; i < spores.length; i++) {
        if (i === from) continue;
        const d = Math.hypot(
          spores[i].dx - spores[from].dx,
          spores[i].dy - spores[from].dy,
        );
        if (d > 24 && d < bestDist) {
          bestDist = d;
          best = i;
        }
      }
      nextGerminate = now + rand(GERMINATE_MIN_GAP, GERMINATE_MAX_GAP);
      if (best < 0) return;
      const a = spores[from];
      const b = spores[best];
      const mx = (a.dx + b.dx) / 2;
      const my = (a.dy + b.dy) / 2;
      const nx = -(b.dy - a.dy);
      const ny = b.dx - a.dx;
      const bend = rand(-0.22, 0.22);
      threads.push({
        from,
        to: best,
        start: now,
        cx: mx + nx * bend,
        cy: my + ny * bend,
      });
    }

    /* Ease-out cubic — organic growth, quick start then settling */
    const easeOut = (t: number) => 1 - Math.pow(1 - t, 3);

    function quadPoint(
      x0: number,
      y0: number,
      cx: number,
      cy: number,
      x1: number,
      y1: number,
      t: number,
    ) {
      const u = 1 - t;
      return {
        x: u * u * x0 + 2 * u * t * cx + t * t * x1,
        y: u * u * y0 + 2 * u * t * cy + t * t * y1,
      };
    }

    function drawThreads(now: number, colors: ReturnType<typeof palette>) {
      threads = threads.filter(
        (t) => now - t.start < THREAD_GROW_MS + THREAD_HOLD_MS + THREAD_FADE_MS,
      );
      for (const t of threads) {
        const age = now - t.start;
        const a = spores[t.from];
        const b = spores[t.to];
        let alpha = colors.threadAlpha;
        let growth = 1;
        if (age < THREAD_GROW_MS) {
          growth = easeOut(age / THREAD_GROW_MS);
        } else if (age > THREAD_GROW_MS + THREAD_HOLD_MS) {
          const fade =
            (age - THREAD_GROW_MS - THREAD_HOLD_MS) / THREAD_FADE_MS;
          alpha *= 1 - fade;
        }
        ctx!.strokeStyle = `rgba(${colors.amber}, ${alpha})`;
        ctx!.lineWidth = 0.6;
        ctx!.beginPath();
        ctx!.moveTo(a.dx, a.dy);
        // grow along the curve by sampling to the current growth fraction
        const steps = 16;
        for (let s = 1; s <= steps; s++) {
          const tt = (s / steps) * growth;
          const p = quadPoint(a.dx, a.dy, t.cx, t.cy, b.dx, b.dy, tt);
          ctx!.lineTo(p.x, p.y);
        }
        ctx!.stroke();
        // growing tip — a tiny bright bud
        if (age < THREAD_GROW_MS) {
          const tip = quadPoint(a.dx, a.dy, t.cx, t.cy, b.dx, b.dy, growth);
          ctx!.fillStyle = `rgba(${colors.amber}, ${alpha * 2})`;
          ctx!.beginPath();
          ctx!.arc(tip.x, tip.y, 1.1, 0, Math.PI * 2);
          ctx!.fill();
        }
      }
    }

    function drawField(time: number, staticFrame = false) {
      ctx!.clearRect(0, 0, width, height);
      const colors = palette();
      const now = performance.now();

      // Warm ambient glow centred above the fold
      {
        const gx = width * 0.5;
        const gy = height * 0.38;
        const gr = Math.max(width, height) * 0.62;
        const glow = ctx!.createRadialGradient(gx, gy, 0, gx, gy, gr);
        glow.addColorStop(0, `rgba(${colors.amber}, ${colors.glowPeak})`);
        glow.addColorStop(
          0.45,
          `rgba(${colors.amber}, ${colors.glowPeak * 0.4})`,
        );
        glow.addColorStop(1, `rgba(${colors.amber}, 0)`);
        ctx!.fillStyle = glow;
        ctx!.fillRect(0, 0, width, height);
      }

      // Smooth scroll velocity (decays each frame)
      scrollVelocity = scrollVelocity * 0.92 + (scrollY - prevScrollY) * 0.08;
      prevScrollY = scrollY;

      const tSec = time * 0.001;

      for (let i = 0; i < spores.length; i++) {
        const p = spores[i];

        if (!staticFrame) {
          // slow constant fall + lateral creep, scaled by depth
          p.x += 0.02 * (0.4 + p.z);
          p.y -= 0.015 * (0.4 + p.z); // spores rise, like dust in light
          // scroll influence
          p.oy += scrollVelocity * 0.015 * p.z;
          // mouse influence — gentle pull
          if (mouseX >= 0 && mouseY >= 0) {
            const ddx = mouseX - p.dx;
            const ddy = mouseY - p.dy;
            const dist = Math.hypot(ddx, ddy);
            if (dist < MOUSE_RADIUS && dist > 1) {
              const force = (1 - dist / MOUSE_RADIUS) * MOUSE_STRENGTH * p.z;
              p.ox += ddx * force;
              p.oy += ddy * force;
            }
          }
          // offsets relax back toward zero
          p.ox *= 0.985;
          p.oy *= 0.985;
        }

        // organic wander — two-frequency eased sine per axis
        const wx =
          Math.sin(tSec * p.fx1 * Math.PI * 2 + p.px1) * p.ax1 +
          Math.sin(tSec * p.fx2 * Math.PI * 2 + p.px2) * p.ax2;
        const wy =
          Math.sin(tSec * p.fy1 * Math.PI * 2 + p.py1) * p.ay1 +
          Math.sin(tSec * p.fy2 * Math.PI * 2 + p.py2) * p.ay2;

        // wrap anchors around the viewport
        if (p.x < -60) p.x = width + 60;
        if (p.x > width + 60) p.x = -60;
        if (p.y < -60) p.y = height + 60;
        if (p.y > height + 60) p.y = -60;

        p.dx = p.x + wx + p.ox;
        p.dy = p.y + wy + p.oy;

        const color = sporeColor(i, colors);
        const twinkle =
          0.75 + 0.25 * Math.sin(tSec * 0.6 * Math.PI + p.twinklePhase);
        const alpha = colors.sporeAlpha * (0.35 + p.z * 0.65) * twinkle;
        const r = p.r;

        // soft glow halo
        const halo = ctx!.createRadialGradient(
          p.dx,
          p.dy,
          0,
          p.dx,
          p.dy,
          r * 5,
        );
        halo.addColorStop(0, `rgba(${color}, ${alpha * 0.35})`);
        halo.addColorStop(1, `rgba(${color}, 0)`);
        ctx!.fillStyle = halo;
        ctx!.beginPath();
        ctx!.arc(p.dx, p.dy, r * 5, 0, Math.PI * 2);
        ctx!.fill();

        // spore body
        ctx!.fillStyle = `rgba(${color}, ${alpha})`;
        ctx!.beginPath();
        ctx!.arc(p.dx, p.dy, r, 0, Math.PI * 2);
        ctx!.fill();
      }

      if (!staticFrame) {
        maybeGerminate(now);
        drawThreads(now, colors);
      }
    }

    function frame(time: number) {
      if (hidden) return; // resumes via visibilitychange
      drawField(time);

      // Adaptive frame rate: 60fps when active, ~15fps when idle
      const hasMotion =
        Math.abs(scrollVelocity) > 0.1 || mouseX >= 0 || threads.length > 0;
      if (hasMotion) {
        idleFrames = 0;
        animationId = requestAnimationFrame(frame);
      } else {
        idleFrames++;
        const delay = idleFrames > IDLE_THRESHOLD ? 66 : 0;
        if (delay > 0) {
          animationId = setTimeout(
            () => requestAnimationFrame(frame),
            delay,
          ) as unknown as number;
        } else {
          animationId = requestAnimationFrame(frame);
        }
      }
    }

    function handleScroll() {
      scrollY = window.scrollY;
    }

    function handleMouseMove(e: MouseEvent) {
      mouseX = e.clientX;
      mouseY = e.clientY;
    }

    function handleMouseLeave() {
      mouseX = -1;
      mouseY = -1;
    }

    function handleVisibility() {
      hidden = document.hidden;
      if (!hidden) {
        cancelAnimationFrame(animationId);
        clearTimeout(animationId);
        animationId = requestAnimationFrame(frame);
      }
    }

    const prefersReducedMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;

    resize();
    initSpores();

    if (prefersReducedMotion) {
      // Static faint spore field — no animation loop
      drawField(0, true);
      const staticResize = () => {
        resize();
        drawField(0, true);
      };
      window.addEventListener("resize", staticResize);
      return () => window.removeEventListener("resize", staticResize);
    }

    animationId = requestAnimationFrame(frame);

    const handleResize = () => {
      resize();
    };
    window.addEventListener("resize", handleResize);
    window.addEventListener("scroll", handleScroll, { passive: true });
    window.addEventListener("mousemove", handleMouseMove, { passive: true });
    document.addEventListener("mouseleave", handleMouseLeave);
    document.addEventListener("visibilitychange", handleVisibility);

    return () => {
      cancelAnimationFrame(animationId);
      clearTimeout(animationId);
      window.removeEventListener("resize", handleResize);
      window.removeEventListener("scroll", handleScroll);
      window.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseleave", handleMouseLeave);
      document.removeEventListener("visibilitychange", handleVisibility);
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
