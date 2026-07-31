"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { SporeLogo } from "./SporeLogo";

/**
 * The hero mushroom that travels into the header as you scroll.
 *
 * An invisible in-flow anchor reserves the large mark's box in the hero so
 * layout never shifts. The visible mark is portalled to <body> — escaping the
 * framer-motion transforms in the hero, which would otherwise become the
 * containing block for a `fixed` child and throw off viewport positioning —
 * and interpolates from the anchor's live viewport position up to the nav
 * slot. The hero and nav share the same centered container, so the mark's
 * left edge already lines up with the header slot it docks into; only size
 * and vertical position animate. Fully responsive-safe.
 */

const HERO_SIZE = 84;
const NAV_SIZE = 26; // fallback if the nav slot can't be measured
const NAV_TOP = 15; // fallback dock y
const TRAVEL = 200; // px of scroll over which it fully docks

export function HeroLogo() {
  const anchorRef = useRef<HTMLSpanElement>(null);
  const [mounted, setMounted] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number; size: number } | null>(
    null,
  );

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    let raf = 0;
    function update() {
      const el = anchorRef.current;
      if (!el) return;
      const r = el.getBoundingClientRect(); // viewport coords, tracks scroll
      // Measure the nav's reserved slot live so the dock target follows the
      // header exactly across breakpoints and the scrolled/unscrolled states —
      // no duplicated layout constants.
      const slotEl = document.querySelector<HTMLElement>("[data-dock-slot]");
      const slot = slotEl?.getBoundingClientRect();
      const dockLeft = slot ? slot.left : r.left;
      const dockTop = slot ? slot.top : NAV_TOP;
      const dockSize = slot ? slot.width : NAV_SIZE;
      const p = Math.min(1, Math.max(0, window.scrollY / TRAVEL));
      const size = HERO_SIZE + (dockSize - HERO_SIZE) * p;
      const left = r.left + (dockLeft - r.left) * p;
      const top = r.top + (dockTop - r.top) * p;
      setPos({ left, top, size });
    }
    function onScroll() {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(update);
    }
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      cancelAnimationFrame(raf);
    };
  }, [mounted]);

  return (
    <>
      {/* In-flow anchor: reserves the large mark's box, invisible. */}
      <span
        ref={anchorRef}
        aria-hidden="true"
        className="block"
        style={{ width: HERO_SIZE, height: HERO_SIZE }}
      />
      {mounted &&
        pos &&
        createPortal(
          <div
            className="pointer-events-none fixed z-[60]"
            style={{ left: pos.left, top: pos.top, width: pos.size, height: pos.size }}
          >
            <SporeLogo size={pos.size} alive className="drop-shadow-sm" />
          </div>,
          document.body,
        )}
    </>
  );
}
