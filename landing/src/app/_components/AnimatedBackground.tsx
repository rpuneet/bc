"use client";

import { useEffect, useRef } from "react";

/**
 * Lightweight canvas particle background.
 * Particles drift very slowly on their own, react to scroll position,
 * and gently gravitate toward the mouse cursor.
 */
export function AnimatedBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let animationId: number;
    let scrollY = 0;
    let prevScrollY = 0;
    let scrollVelocity = 0;
    let mouseX = -1;
    let mouseY = -1;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    // Particle configuration
    const PARTICLE_COUNT = 40;
    const CONNECTION_DISTANCE = 120;
    const GRID_CELL_SIZE = CONNECTION_DISTANCE;
    const MOUSE_RADIUS = 200;
    const MOUSE_STRENGTH = 0.015;
    let idleFrames = 0;
    const IDLE_THRESHOLD = 120; // ~2s at 60fps before throttling

    interface Particle {
      x: number;
      y: number;
      z: number;
      baseX: number;
      baseY: number;
      vx: number;
      vy: number;
      vz: number;
      size: number;
      phase: number;
    }

    let particles: Particle[] = [];
    let width = 0;
    let height = 0;

    function resize() {
      width = window.innerWidth;
      height = window.innerHeight;
      canvas!.width = width * dpr;
      canvas!.height = height * dpr;
      canvas!.style.width = `${width}px`;
      canvas!.style.height = `${height}px`;
      ctx!.scale(dpr, dpr);
    }

    function initParticles() {
      particles = [];
      for (let i = 0; i < PARTICLE_COUNT; i++) {
        const x = Math.random() * width;
        const y = Math.random() * height;
        particles.push({
          x,
          y,
          z: Math.random() * 400 + 100,
          baseX: x,
          baseY: y,
          vx: (Math.random() - 0.5) * 0.5,
          vy: (Math.random() - 0.5) * 0.4,
          vz: (Math.random() - 0.5) * 0.3,
          size: Math.random() * 1.5 + 0.5,
          phase: Math.random() * Math.PI * 2,
        });
      }
    }

    function project(x: number, y: number, z: number) {
      const fov = 600;
      const scale = fov / (fov + z);
      return {
        x: x * scale + width * 0.5 * (1 - scale),
        y: y * scale + height * 0.5 * (1 - scale),
        scale,
      };
    }

    function draw(time: number) {
      ctx!.clearRect(0, 0, width, height);

      const isDark = document.documentElement.classList.contains("dark");
      const particleColor = isDark ? "rgba(234, 88, 12," : "rgba(234, 88, 12,";
      const lineColor = isDark ? "rgba(251, 146, 60," : "rgba(234, 88, 12,";

      // Smooth scroll velocity (decays each frame)
      scrollVelocity = scrollVelocity * 0.92 + (scrollY - prevScrollY) * 0.08;
      prevScrollY = scrollY;

      // Sort by z for depth ordering
      particles.sort((a, b) => b.z - a.z);

      // Update and project particles
      const projected = particles.map((p) => {
        // Medium drift
        p.x += p.vx;
        p.y += p.vy;
        p.z += p.vz + Math.sin(time * 0.0004 + p.phase) * 0.08;

        // Scroll influence — particles shift with scroll velocity
        p.y += scrollVelocity * 0.02;

        // Mouse influence — gentle pull toward cursor
        if (mouseX >= 0 && mouseY >= 0) {
          const dx = mouseX - p.x;
          const dy = mouseY - p.y;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < MOUSE_RADIUS && dist > 1) {
            const force = (1 - dist / MOUSE_RADIUS) * MOUSE_STRENGTH;
            p.x += dx * force;
            p.y += dy * force;
          }
        }

        // Wrap around
        if (p.x < -50) p.x = width + 50;
        if (p.x > width + 50) p.x = -50;
        if (p.y < -50) p.y = height + 50;
        if (p.y > height + 50) p.y = -50;
        if (p.z < 50) p.z = 500;
        if (p.z > 500) p.z = 50;

        return { ...project(p.x, p.y, p.z), particle: p };
      });

      // Draw connections using spatial grid (O(n) average instead of O(n^2))
      const grid = new Map<string, number[]>();
      for (let i = 0; i < projected.length; i++) {
        const cellX = Math.floor(projected[i].x / GRID_CELL_SIZE);
        const cellY = Math.floor(projected[i].y / GRID_CELL_SIZE);
        const key = `${cellX},${cellY}`;
        const cell = grid.get(key);
        if (cell) {
          cell.push(i);
        } else {
          grid.set(key, [i]);
        }
      }

      for (const [key, indices] of grid) {
        const [cx, cy] = key.split(",").map(Number);
        // Check this cell and 4 neighbors (right, below, below-left, below-right) to avoid double-checking
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
              const a = projected[i];
              const b = projected[j];
              const dx = a.x - b.x;
              const dy = a.y - b.y;
              const dist = Math.sqrt(dx * dx + dy * dy);
              if (dist < CONNECTION_DISTANCE) {
                const opacity =
                  (1 - dist / CONNECTION_DISTANCE) *
                  0.12 *
                  Math.min(a.scale, b.scale);
                ctx!.strokeStyle = `${lineColor}${opacity})`;
                ctx!.lineWidth = 0.5;
                ctx!.beginPath();
                ctx!.moveTo(a.x, a.y);
                ctx!.lineTo(b.x, b.y);
                ctx!.stroke();
              }
            }
          }
        }
      }

      // Draw particles
      for (const p of projected) {
        const opacity = 0.35 * p.scale;
        const r = p.particle.size * p.scale * 2;
        ctx!.fillStyle = `${particleColor}${opacity})`;
        ctx!.beginPath();
        ctx!.arc(p.x, p.y, r, 0, Math.PI * 2);
        ctx!.fill();

        // Glow effect for closer particles
        if (p.scale > 0.7) {
          const glow = ctx!.createRadialGradient(p.x, p.y, 0, p.x, p.y, r * 4);
          glow.addColorStop(0, `${particleColor}${opacity * 0.25})`);
          glow.addColorStop(1, `${particleColor}0)`);
          ctx!.fillStyle = glow;
          ctx!.beginPath();
          ctx!.arc(p.x, p.y, r * 4, 0, Math.PI * 2);
          ctx!.fill();
        }
      }

      // Adaptive frame rate: 60fps when active, ~15fps when idle (2s no interaction)
      const hasMotion = Math.abs(scrollVelocity) > 0.1 || mouseX >= 0;
      if (hasMotion) {
        idleFrames = 0;
        animationId = requestAnimationFrame(draw);
      } else {
        idleFrames++;
        const delay = idleFrames > IDLE_THRESHOLD ? 66 : 0; // ~15fps idle, 60fps active
        if (delay > 0) {
          animationId = setTimeout(
            () => requestAnimationFrame(draw),
            delay,
          ) as unknown as number;
        } else {
          animationId = requestAnimationFrame(draw);
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

    // Check for reduced motion preference
    const prefersReducedMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    if (prefersReducedMotion) return;

    resize();
    initParticles();
    animationId = requestAnimationFrame(draw);

    window.addEventListener("resize", resize);
    window.addEventListener("scroll", handleScroll, { passive: true });
    window.addEventListener("mousemove", handleMouseMove, { passive: true });
    document.addEventListener("mouseleave", handleMouseLeave);

    return () => {
      cancelAnimationFrame(animationId);
      clearTimeout(animationId);
      window.removeEventListener("resize", resize);
      window.removeEventListener("scroll", handleScroll);
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
