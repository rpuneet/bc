"use client";

import Link from "next/link";
import Image from "next/image";
import { motion } from "framer-motion";
import { ArrowRight, Code2 } from "lucide-react";

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
          {/* Fix 8: More visible badge pills */}
          <motion.div
            variants={fadeUp}
            custom={0}
            className="mb-6 inline-flex items-center gap-2 rounded-full border border-primary/20 bg-primary/10 px-4 py-1.5 font-mono text-xs text-primary backdrop-blur-sm"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
            CLI-first <span className="text-primary/60">&middot;</span> Agent-agnostic <span className="text-primary/60">&middot;</span> Open source
          </motion.div>

          <motion.h1
            variants={fadeUp}
            custom={1}
            className="text-balance text-[2.25rem] font-bold leading-[1.05] tracking-tight sm:text-5xl lg:text-6xl"
          >
            AI in a Box.
          </motion.h1>

          {/* Fix 5: Clearer value proposition descriptor */}
          <motion.p
            variants={fadeUp}
            custom={1}
            className="mt-2 font-mono text-sm tracking-wide text-primary/80"
          >
            The open-source CLI for multi-agent orchestration.
          </motion.p>

          {/* Fix 1: Better subtitle contrast + Fix 5: Larger subtitle */}
          <motion.p
            variants={fadeUp}
            custom={1}
            className="mt-3 text-balance text-xl font-medium leading-snug tracking-tight text-stone-300 sm:text-2xl"
          >
            Orchestrate AI agents from your terminal.
          </motion.p>

          <motion.p
            variants={fadeUp}
            custom={2}
            className="mt-4 max-w-[520px] text-base leading-relaxed text-muted-foreground sm:text-lg"
          >
            Coordinate teams of Claude, Gemini, and Cursor agents on a single
            codebase. Isolated worktrees. Shared channels. Cost controls. One binary.
          </motion.p>

          {/* Fix 4: Get Started CTA + Fix 7: GitHub button styling + Fix 9: Stars */}
          <motion.div
            variants={fadeUp}
            custom={3}
            className="mt-6 flex flex-wrap items-center gap-3"
          >
            <Link
              href="/#install"
              className="cta-glow group inline-flex h-10 sm:h-11 items-center gap-2 rounded-lg bg-primary px-6 sm:px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-xl hover:shadow-primary/20 active:scale-[0.97]"
              aria-label="Get started with mycel"
            >
              Get Started
              <ArrowRight
                className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </Link>
            <Link
              href="https://github.com/rpuneet/bc"
              className="inline-flex h-10 sm:h-11 items-center gap-2 rounded-lg border border-outline-variant/50 px-6 sm:px-8 text-sm font-medium transition-colors hover:bg-accent/20 active:scale-[0.97]"
              aria-label="View mycel on GitHub"
            >
              <Code2 className="h-4 w-4" aria-hidden="true" />
              View on GitHub
            </Link>
            <span className="font-mono text-sm text-muted-foreground">
              150+ stars on GitHub
            </span>
          </motion.div>

        </div>

        {/* Hero dashboard screenshot — Fix 3: Better border/glow */}
        <motion.div variants={fadeUp} custom={2} className="relative">
          <div className="absolute -inset-8 rounded-3xl bg-gradient-to-tr from-primary/8 via-transparent to-secondary/15 blur-3xl hero-glow" />
          <div className="relative overflow-hidden rounded-xl border border-outline-variant/30 shadow-2xl shadow-primary/10">
            <Image
              src="/screenshots/dashboard-01-home.png"
              alt="mycel dashboard showing active agents, channels, cost tracking, and system overview"
              width={1200}
              height={750}
              className="w-full h-auto brightness-110"
              priority
            />
          </div>
        </motion.div>
      </motion.section>
    </div>
  );
}
