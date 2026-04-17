"use client";

import { motion } from "framer-motion";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";

const PRINCIPLES = [
  {
    number: "01",
    title: "Isolation",
    subtitle: "Shared state is the enemy of parallel work.",
    content: `Concurrent agents sharing a branch will destroy each other's work. This is not a tooling problem. It is a physics problem. Parallel writers need separate spaces.

Isolation is the foundation. Without it, every other principle collapses under merge conflicts and broken builds.`,
  },
  {
    number: "02",
    title: "Communication",
    subtitle: "Agents coordinate through structure, not chaos.",
    content: `Isolated agents working in silence produce fragmented results. Agents need persistent, structured channels — not ad-hoc messages lost to scrollback.

The difference between agents and a team of agents is structured coordination. Without it, they duplicate effort and contradict each other. With it, they converge.`,
  },
  {
    number: "03",
    title: "Visibility",
    subtitle: "What you cannot see, you cannot trust.",
    content: `Every token, every cost, every tool call, every decision — attributed and observable in real time. When you see the complete picture, you intervene early. Not after the damage.

Agents left unchecked burn through budgets silently. Visibility is not a convenience. It is a safety mechanism.`,
  },
  {
    number: "04",
    title: "Persistence",
    subtitle: "A tool runs once. A teammate finishes the job.",
    content: `Most agents quit at the first obstacle. They produce a partial answer and call it done. That is not how hard problems get solved. Hard problems yield to repetition — try, fail, learn, try again. Each attempt sharper than the last.

The difference between an assistant and a collaborator is what happens after the first failure. An assistant stops. A collaborator adapts and continues. Persistence is not about running longer. It is about getting closer with every iteration.`,
  },
  {
    number: "05",
    title: "Surface",
    subtitle:
      "An agent's usefulness is bounded by what it can reach.",
    content: `Real development is not just editing files. It is filing issues, reviewing pull requests, testing in browsers, querying databases, deploying services. An agent confined to a text editor solves text-editor problems.

Every system a developer touches should be reachable by an agent. The surface expands. The boundaries hold.`,
  },
  {
    number: "06",
    title: "Openness",
    subtitle: "Knowledge shared compounds. Knowledge hoarded decays.",
    content: `We are not building for Mars. We are aiming for Andromeda. The distance ahead is so vast that keeping solutions behind walls is not just slow — it is self-defeating. The hard problems are universal. Progress compounds when solutions are shared.

Open source is not generosity. It is the only rational approach when the challenge exceeds what any one team can solve alone. The code is open. The ideas are open. The method is open. That is how you build something that outlasts you.`,
  },
];

const fadeUp = {
  hidden: { opacity: 0, y: 24 },
  visible: { opacity: 1, y: 0 },
};

export function MethodContent() {
  return (
    <main className="method-page min-h-screen selection:bg-primary/20 selection:text-foreground">
      <Nav />

      <article className="mx-auto max-w-4xl px-6 sm:px-8 py-20 sm:py-32">
        {/* Header */}
        <motion.header
          className="mb-24"
          initial="hidden"
          animate="visible"
          variants={fadeUp}
          transition={{ duration: 0.8, ease: [0.25, 0.1, 0.25, 1] }}
        >
          <span className="font-mono text-[11px] font-medium uppercase tracking-[0.35em] text-muted-foreground/50">
            Philosophy
          </span>
          <h1
            className="mt-5 text-5xl font-bold tracking-tight sm:text-7xl lg:text-8xl"
            style={{ fontFamily: "Georgia, 'Times New Roman', serif" }}
          >
            The mycel Method
          </h1>
          <p className="mt-8 text-xl sm:text-2xl text-[var(--method-muted)] leading-relaxed max-w-2xl">
            Practices for orchestrating AI agent teams.
          </p>
        </motion.header>

        {/* Introduction */}
        <motion.div
          className="mb-24"
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-80px" }}
          variants={fadeUp}
          transition={{ duration: 0.7, ease: [0.25, 0.1, 0.25, 1] }}
        >
          <p className="text-lg sm:text-xl method-body-text text-[var(--method-muted)]">
            There is a growing art to coordinating AI coding agents
            effectively. Running a single agent is straightforward. Running
            multiple agents on the same codebase, in parallel, without
            chaos — that requires structure.
          </p>
          <p className="mt-8 text-lg sm:text-xl method-body-text text-[var(--method-muted)]">
            These are not features. They are design convictions that shape
            everything we build.
          </p>
        </motion.div>

        {/* Decorative divider */}
        <div className="mb-24 flex items-center justify-center">
          <div className="method-divider-gradient h-px w-full" />
        </div>

        {/* Principles */}
        {PRINCIPLES.map((p, i) => (
          <motion.section
            key={p.number}
            className={i > 0 ? "mt-24" : ""}
            initial="hidden"
            whileInView="visible"
            viewport={{ once: true, margin: "-60px" }}
            variants={fadeUp}
            transition={{
              duration: 0.7,
              delay: 0.1,
              ease: [0.25, 0.1, 0.25, 1],
            }}
          >
            <div className="mb-8">
              <span className="method-principle-label inline-flex items-center gap-3 font-mono text-xs font-bold uppercase tracking-[0.3em] text-primary/60">
                <span className="inline-block w-8 h-px bg-primary/40" />
                Principle {p.number}
              </span>
            </div>
            <h2
              className="text-4xl font-bold tracking-tight sm:text-5xl"
              style={{
                fontFamily:
                  "'Instrument Serif', Georgia, 'Times New Roman', serif",
              }}
            >
              {p.title}
            </h2>
            <p
              className="mt-3 text-lg sm:text-xl text-[var(--method-muted)] italic"
              style={{
                fontFamily:
                  "'Instrument Serif', Georgia, 'Times New Roman', serif",
              }}
            >
              {p.subtitle}
            </p>
            <div className="mt-10 space-y-8">
              {p.content.split("\n\n").map((paragraph, j) => (
                <p
                  key={j}
                  className="text-base sm:text-lg method-body-text text-[var(--method-muted)]"
                >
                  {paragraph}
                </p>
              ))}
            </div>
            {i < PRINCIPLES.length - 1 && (
              <div className="mt-24 method-divider-gradient h-px w-24" />
            )}
          </motion.section>
        ))}

        {/* Closing */}
        <motion.div
          className="mt-32 pt-20 border-t border-border"
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-60px" }}
          variants={fadeUp}
          transition={{ duration: 0.7, ease: [0.25, 0.1, 0.25, 1] }}
        >
          <p className="text-xl sm:text-2xl method-body-text text-[var(--method-muted)]">
            These are not features. They are design convictions. Every
            command, every view, every architectural decision exists because
            it serves one of these principles.
          </p>
        </motion.div>
      </article>

      <Footer />
    </main>
  );
}
