"use client";

import { useEffect, useRef } from "react";

/**
 * Mycelial network field — the *quiet* living background of the page.
 *
 * Soft-glow spore nodes wander on eased sine paths (two incommensurate
 * frequencies per axis, so the drift never visibly repeats). When two spores
 * drift within reach of each other, a fine hyphae thread fades in between
 * them — a gently curved filament whose opacity tracks proximity — and fades
 * back out as they part. The whole field reads as one slow, breathing
 * network: mycelium, not confetti.
 *
 * Motion budget (owner decision #4): the background is deliberately *calm* so
 * it never competes with the animated product hero. Far fewer nodes, slower
 * drift, lower opacity, gentler glow — an ambient accent, not a spectacle.
 * The real motion budget lives in the hero tabs and the streaming Live panel.
 *
 * Performance: capped particle count, pairwise link pass is O(n²) with a
 * small n, adaptive frame rate when idle, rendering pauses entirely when the
 * tab is hidden. prefers-reduced-motion renders a single static frame of the
 * network with its links.
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
    let lastMouseMove = -Infinity;
    let hidden = document.hidden;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    // Calmed field (owner decision #4): roughly half the nodes, a shorter
    // reach so the network stays sparse, and a weaker mouse pull.
    const SPORE_COUNT = 22;
    const LAVENDER_EVERY = 7; // every 7th spore whispers lavender
    const MOUSE_RADIUS = 170;
    const MOUSE_STRENGTH = 0.007;
    let idleFrames = 0;
    const IDLE_THRESHOLD = 90; // ~1.5s at 60fps before throttling down

    /* Proximity links — hyphae threads between spores that drift close.
     * Opacity follows distance, so threads fade in as spores approach and
     * fade out as they part; no timers, the network is continuous. */
    const LINK_REACH = 130; // px — threads appear inside this distance
    const LINK_MIN = 18; // px — too close reads as a blob; keep a gap

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

    let spores: Spore[] = [];
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
    }

    /* Palette per theme. Light mode uses the deep chanterelle cut so faint
     * spores stay visible on cream; dark mode glows bright amber. */
    function palette() {
      const isDark = document.documentElement.classList.contains("dark");
      // Lower opacity across the board so the field recedes behind the
      // product — a whisper, not a wash.
      return isDark
        ? {
            amber: "232, 163, 61",
            lavender: "169, 151, 189",
            glowPeak: 0.055,
            sporeAlpha: 0.3,
            threadAlpha: 0.22,
          }
        : {
            amber: "163, 93, 10",
            lavender: "141, 122, 158",
            glowPeak: 0.038,
            sporeAlpha: 0.26,
            threadAlpha: 0.2,
          };
    }

    function sporeColor(i: number, colors: ReturnType<typeof palette>) {
      return i % LAVENDER_EVERY === LAVENDER_EVERY - 1
        ? colors.lavender
        : colors.amber;
    }

    /* Deterministic per-pair bend so each hyphae filament keeps its own
     * gentle curve as the spores drift — organic, but stable. */
    function pairBend(i: number, j: number) {
      const seed = Math.sin(i * 127.1 + j * 311.7) * 43758.5453;
      return ((seed - Math.floor(seed)) - 0.5) * 0.36;
    }

    /* Hyphae links — one pass over spore pairs. Thread opacity tracks
     * proximity, so filaments breathe in and out as the network drifts. */
    function drawLinks(colors: ReturnType<typeof palette>) {
      ctx!.lineWidth = 0.6;
      for (let i = 0; i < spores.length; i++) {
        const a = spores[i];
        for (let j = i + 1; j < spores.length; j++) {
          const b = spores[j];
          const ddx = b.dx - a.dx;
          const ddy = b.dy - a.dy;
          if (Math.abs(ddx) > LINK_REACH || Math.abs(ddy) > LINK_REACH) {
            continue;
          }
          const d = Math.hypot(ddx, ddy);
          if (d >= LINK_REACH || d < LINK_MIN) continue;
          // smooth fade: 0 at reach, full at half-reach
          const near = 1 - d / LINK_REACH;
          const alpha =
            colors.threadAlpha * near * near * (0.5 + (a.z + b.z) * 0.25);
          if (alpha < 0.008) continue;
          const bend = pairBend(i, j);
          const cx = (a.dx + b.dx) / 2 - ddy * bend;
          const cy = (a.dy + b.dy) / 2 + ddx * bend;
          ctx!.strokeStyle = `rgba(${colors.amber}, ${alpha})`;
          ctx!.beginPath();
          ctx!.moveTo(a.dx, a.dy);
          ctx!.quadraticCurveTo(cx, cy, b.dx, b.dy);
          ctx!.stroke();
        }
      }
    }

    function drawField(time: number, staticFrame = false) {
      ctx!.clearRect(0, 0, width, height);
      const colors = palette();

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
          // very slow constant fall + lateral creep, scaled by depth —
          // halved from the old field so the drift barely registers
          p.x += 0.01 * (0.4 + p.z);
          p.y -= 0.008 * (0.4 + p.z); // spores rise, like dust in light
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
          0.82 + 0.18 * Math.sin(tSec * 0.4 * Math.PI + p.twinklePhase);
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

      // The network itself — hyphae threads between near neighbours.
      // Drawn in the static frame too: the resting state is still a network.
      drawLinks(colors);
    }

    function frame(time: number) {
      if (hidden) return; // resumes via visibilitychange
      drawField(time);

      // Adaptive frame rate: full rate while the visitor scrolls or moves
      // the mouse, ~30fps once everything rests — a cursor merely parked
      // over the page counts as idle, so the loop always winds down.
      const mouseActive = time - lastMouseMove < 1500;
      const hasMotion = Math.abs(scrollVelocity) > 0.1 || mouseActive;
      if (hasMotion) {
        idleFrames = 0;
        animationId = requestAnimationFrame(frame);
      } else {
        idleFrames++;
        const delay = idleFrames > IDLE_THRESHOLD ? 33 : 0;
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
      lastMouseMove = performance.now();
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
