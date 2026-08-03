"use client";

import { motion } from "framer-motion";
import { useEffect, useRef, useState } from "react";

/**
 * Returns true once the component has mounted in the browser.
 *
 * Reveal animations start from `opacity: 0`, so a section stays invisible until
 * its viewport trigger fires. If hydration never runs or the trigger misfires,
 * the content would be permanently blank. Gating the hidden `initial` state on
 * mount keeps the server-rendered markup fully visible and treats the animation
 * as a pure enhancement: content first, motion second.
 */
export function useMounted(): boolean {
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    // Flip on the next tick rather than synchronously in the effect body,
    // which the React compiler's set-state-in-effect rule flags.
    const id = setTimeout(() => setMounted(true), 0);
    return () => clearTimeout(id);
  }, []);
  return mounted;
}

/**
 * Scroll-reveal with a visible-by-default guarantee.
 *
 * Server markup renders fully visible. On the client an effect "arms" the
 * hidden state and an IntersectionObserver reveals the element when it
 * enters the viewport. If the observer is unavailable, reduced motion is
 * requested, or the trigger somehow never fires, a safety timeout unhides
 * the content — it can never be stuck invisible.
 */
export function ScrollReveal({
  children,
  className = "",
  delay = 0,
  from = "up",
  distance = 28,
}: {
  children: React.ReactNode;
  className?: string;
  delay?: number;
  from?: "up" | "left" | "right";
  distance?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [armed, setArmed] = useState(false);
  const [revealed, setRevealed] = useState(false);

  useEffect(() => {
    const el = ref.current;
    const reduced = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    if (!el || reduced || typeof IntersectionObserver === "undefined") {
      // Nothing to do: `hidden` is false until the animation is armed, so the
      // content is already visible. Setting `revealed` here would have been a
      // synchronous state update in an effect — a cascading render — to reach a
      // state that renders identically.
      return;
    }
    // Arm on the next tick rather than synchronously in the effect body, the
    // same way useMounted does and for the same reason. Should the observer win
    // the race — an element already in the viewport at load — `revealed` lands
    // first and the element simply never hides, skipping the animation rather
    // than the content.
    const arm = setTimeout(() => setArmed(true), 0);
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setRevealed(true);
          observer.disconnect();
        }
      },
      { rootMargin: "-60px 0px" },
    );
    observer.observe(el);
    // Safety net: never leave content hidden if the trigger misfires.
    const failsafe = setTimeout(() => setRevealed(true), 3000);
    return () => {
      clearTimeout(arm);
      observer.disconnect();
      clearTimeout(failsafe);
    };
  }, []);

  const hidden = armed && !revealed;
  const offset =
    from === "left"
      ? `translateX(-${distance}px)`
      : from === "right"
        ? `translateX(${distance}px)`
        : `translateY(${distance}px)`;

  return (
    <div
      ref={ref}
      className={className}
      style={{
        opacity: hidden ? 0 : 1,
        transform: hidden ? offset : "none",
        transition: `opacity 0.7s cubic-bezier(0.22, 1, 0.36, 1) ${delay}s, transform 0.7s cubic-bezier(0.22, 1, 0.36, 1) ${delay}s`,
        willChange: hidden ? "transform, opacity" : undefined,
      }}
    >
      {children}
    </div>
  );
}

/** Fade-in on scroll using whileInView */
export function RevealSection({
  children,
  className = "",
  delay = 0,
}: {
  children: React.ReactNode;
  className?: string;
  delay?: number;
}) {
  const mounted = useMounted();
  return (
    <motion.section
      initial={mounted ? { opacity: 0 } : false}
      whileInView={{ opacity: 1 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.6, ease: "easeOut", delay }}
      className={className}
    >
      {children}
    </motion.section>
  );
}

/** Fade + translateY animation */
export function FadeUp({
  children,
  className = "",
  delay = 0,
  distance = 16,
}: {
  children: React.ReactNode;
  className?: string;
  delay?: number;
  distance?: number;
}) {
  const mounted = useMounted();
  return (
    <motion.div
      initial={mounted ? { opacity: 0, y: distance } : false}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.6, ease: "easeOut", delay }}
      className={className}
      style={{ willChange: "transform, opacity" }}
    >
      {children}
    </motion.div>
  );
}

/** Alias — FadeIn reads better at some call sites */
export const FadeIn = FadeUp;

/** Stagger child animations */
export function StaggerChildren({
  children,
  className = "",
  stagger = 0.1,
  delay = 0,
}: {
  children: React.ReactNode;
  className?: string;
  stagger?: number;
  delay?: number;
}) {
  const mounted = useMounted();
  return (
    <motion.div
      initial={mounted ? "hidden" : false}
      whileInView="visible"
      viewport={{ once: true, margin: "-60px" }}
      variants={{
        hidden: {},
        visible: {
          transition: {
            staggerChildren: stagger,
            delayChildren: delay,
          },
        },
      }}
      className={className}
    >
      {children}
    </motion.div>
  );
}

/** Child item for use inside StaggerChildren */
export function StaggerItem({
  children,
  className = "",
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <motion.div
      variants={{
        hidden: { opacity: 0, y: 16 },
        visible: { opacity: 1, y: 0 },
      }}
      transition={{ duration: 0.5, ease: "easeOut" }}
      className={className}
    >
      {children}
    </motion.div>
  );
}

/** Slide in from left or right */
export function SlideIn({
  children,
  className = "",
  direction = "left",
  delay = 0,
  distance = 40,
}: {
  children: React.ReactNode;
  className?: string;
  direction?: "left" | "right";
  delay?: number;
  distance?: number;
}) {
  const x = direction === "left" ? -distance : distance;
  const mounted = useMounted();
  return (
    <motion.div
      initial={mounted ? { opacity: 0, x } : false}
      whileInView={{ opacity: 1, x: 0 }}
      viewport={{ once: true, margin: "-60px" }}
      transition={{ duration: 0.6, ease: "easeOut", delay }}
      className={className}
      style={{ willChange: "transform, opacity" }}
    >
      {children}
    </motion.div>
  );
}

/** Floating animation for decorative elements */
export function Floaty({
  children,
  className = "",
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <motion.div
      animate={{ y: [0, -6, 0] }}
      transition={{ duration: 4.5, repeat: Infinity, ease: "easeInOut" }}
      className={className}
      style={{ willChange: "transform" }}
    >
      {children}
    </motion.div>
  );
}
