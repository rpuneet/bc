"use client";

import Link from "next/link";
import Image from "next/image";
import { motion } from "framer-motion";
import { ArrowRight } from "lucide-react";

const fadeUp = {
  hidden: { opacity: 0, y: 30 },
  visible: (i: number) => ({
    opacity: 1,
    y: 0,
    transition: { delay: i * 0.1, duration: 0.6, ease: "easeOut" as const },
  }),
};

const stagger = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.08 } },
};

export function HeroSection() {
  return (
    <div className="relative mx-auto max-w-7xl px-4 sm:px-6 pb-0 pt-8 lg:pt-20">
      <motion.section
        initial="hidden"
        animate="visible"
        variants={stagger}
        className="grid items-center gap-8 lg:grid-cols-[1fr_1.1fr] lg:gap-12"
      >
        <div className="flex flex-col items-start">
          <motion.div
            variants={fadeUp}
            custom={0}
            className="mb-6 inline-flex items-center gap-2 rounded-full border border-border bg-accent/10 px-4 py-1.5 font-mono text-xs text-muted-foreground backdrop-blur-sm"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
            CLI-first &middot; Agent-agnostic &middot; Open source
          </motion.div>

          <motion.h1
            variants={fadeUp}
            custom={1}
            className="text-balance text-[2.25rem] font-bold leading-[1.05] tracking-tight sm:text-5xl lg:text-6xl"
          >
            The Underground Network.
            <br />
            <span className="text-muted-foreground/40">
              Let them cook.
            </span>
          </motion.h1>

          <motion.p
            variants={fadeUp}
            custom={2}
            className="mt-4 max-w-[520px] text-base leading-relaxed text-muted-foreground sm:text-lg"
          >
            Coordinate teams of Claude, Gemini, Cursor, and other AI agents
            with isolated worktrees, shared channels, and cost controls.
          </motion.p>

          <motion.div
            variants={fadeUp}
            custom={3}
            className="mt-6 flex flex-wrap items-center gap-3"
          >
            <Link
              href="https://github.com/rpuneet/bc"
              className="cta-glow group inline-flex h-10 sm:h-11 items-center gap-2 rounded-lg bg-primary px-6 sm:px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-xl hover:shadow-primary/20 active:scale-[0.97]"
              aria-label="Get started with mycel on GitHub"
            >
              Get Started
              <ArrowRight
                className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </Link>
            <Link
              href="/docs"
              className="inline-flex h-10 sm:h-11 items-center gap-2 rounded-lg border border-border px-6 sm:px-8 text-sm font-medium transition-colors hover:bg-accent/20 active:scale-[0.97]"
              aria-label="Read the mycel documentation"
            >
              View Docs
            </Link>
          </motion.div>

        </div>

        {/* Hero dashboard screenshot */}
        <motion.div variants={fadeUp} custom={2} className="relative">
          <div className="absolute -inset-8 rounded-3xl bg-gradient-to-tr from-primary/5 via-transparent to-secondary/10 blur-3xl hero-glow" />
          <div className="relative overflow-hidden rounded-xl border border-border shadow-2xl">
            <Image
              src="/screenshots/dashboard-01-home.png"
              alt="mycel dashboard showing active agents, channels, cost tracking, and system overview"
              width={1200}
              height={750}
              className="w-full h-auto"
              priority
            />
          </div>
        </motion.div>
      </motion.section>
    </div>
  );
}
