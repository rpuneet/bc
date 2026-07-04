"use client";

import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import {
  StaggerChildren,
  StaggerItem,
  FadeUp,
} from "../_components/Motion";

const PRINCIPLES = [
  {
    number: "01",
    title: "Isolation",
    description:
      "Without isolation, two agents editing the same file will overwrite each other. Each agent gets its own git worktree, so 10 agents can work on the same codebase with zero merge conflicts.",
    terminal: "$ mycel agent create eng-01 --role engineer",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" /><rect x="14" y="14" width="7" height="7" /><rect x="3" y="14" width="7" height="7" /></svg>
    ),
  },
  {
    number: "02",
    title: "Communication",
    description:
      "Agents need to coordinate without you as the bottleneck. Persistent channels with @mentions let agents hand off work, request reviews, and converge — all logged and searchable.",
    terminal: '$ mycel channel send eng "starting tests"',
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>
    ),
  },
  {
    number: "03",
    title: "Visibility",
    description:
      "You can't trust what you can't see. The web dashboard at localhost:9374 shows every agent's state, output, costs, and channel messages in real time.",
    terminal: "$ mycel status",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg>
    ),
  },
  {
    number: "04",
    title: "Control",
    description:
      "AI tokens add up fast. Per-agent budgets with hard stops prevent surprise bills. Role permissions scope what each agent can do, so you delegate without losing control.",
    terminal: "$ mycel cost budget set eng-01 --limit $5.00",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>
    ),
  },
  {
    number: "05",
    title: "Persistence",
    description:
      "Agents crash. Machines restart. All state lives in .mycel/ — worktrees, channel history, cost records, and memory survive restarts and are backed by git.",
    terminal: "$ mycel agent start eng-01 --resume",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56" /><polyline points="21 3 21 9 15 9" /></svg>
    ),
  },
  {
    number: "06",
    title: "Simplicity",
    description:
      "No runtime dependencies. No cloud accounts. No YAML config files. One Go binary, two commands, and your agent team is running.",
    terminal: "$ mycel up",
    icon: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" /></svg>
    ),
  },
];

export default function MethodPage() {
  return (
    <main className="min-h-screen bg-background">
      <Nav />
      {/* Hero */}
      <section className="pt-24 pb-8">
        <div className="mx-auto max-w-3xl px-4 text-center">
          <FadeUp>
            <span className="mb-6 inline-block rounded-sm bg-surface-container-highest px-3 py-1 font-label text-xs uppercase tracking-widest text-primary">
              CONCEPT
            </span>
          </FadeUp>
          <FadeUp delay={0.1}>
            <h1 className="font-headline text-5xl font-bold text-on-background">
              The mycel Method
            </h1>
          </FadeUp>
          <FadeUp delay={0.2}>
            <p className="mt-4 text-lg text-on-surface-variant">
              Practices for orchestrating AI agent teams.
            </p>
          </FadeUp>
        </div>
      </section>

      {/* Principle Cards */}
      <section className="py-12">
        <StaggerChildren className="mx-auto max-w-3xl space-y-8 px-4" stagger={0.12}>
          {PRINCIPLES.map((p) => (
            <StaggerItem key={p.number}>
              <div className="group relative overflow-hidden rounded-sm bg-surface-container p-8 transition-all hover:border-l-2 hover:border-l-primary">
                {/* Watermark number */}
                <span className="pointer-events-none absolute right-6 top-4 select-none font-headline text-6xl font-bold text-primary/20">
                  {p.number}
                </span>

                {/* Icon */}
                <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-full bg-surface-container-highest text-primary">
                  {p.icon}
                </div>

                {/* Title */}
                <h2 className="font-headline text-xl font-bold text-on-background">
                  {p.title}
                </h2>

                {/* Description */}
                <p className="mt-3 text-on-surface-variant leading-relaxed">
                  {p.description}
                </p>

                {/* Terminal snippet */}
                <div className="mt-4 inline-block rounded-sm bg-surface-container-highest px-3 py-2 font-label text-xs text-on-surface-variant">
                  {p.terminal}
                </div>
              </div>
            </StaggerItem>
          ))}
        </StaggerChildren>
      </section>
      <Footer />
    </main>
  );
}
