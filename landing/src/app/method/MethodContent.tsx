"use client";

import { type ReactNode } from "react";
import Link from "next/link";
import { ArrowRight, Github } from "lucide-react";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import { ProductFrame } from "../_components/ProductFrame";
import { FadeUp, ScrollReveal } from "../_components/Motion";

/* ════════════════════════════════════════════════════════════════
   The mycel method — an editorial essay in two movements.

   Movement I  · "The shape of it"  — the real sequence, five steps,
                 threaded on a hyphae spine. Numbers carry order.
   Movement II · "The convictions"  — the manifesto: six things the
                 product believes, each grounded in a feature that
                 exists today.
   ════════════════════════════════════════════════════════════════ */

type Step = {
  number: string;
  title: string;
  body: ReactNode;
  artifact?: ReactNode;
};

const STEPS: Step[] = [
  {
    number: "01",
    title: "Start it",
    body: (
      <>
        One command:{" "}
        <code className="method-cmd">
          <span className="prompt">$</span> mycel up
        </code>
        . The app opens on localhost &mdash; no cloud account, no sign-up,
        nothing leaves your machine. Run it as a desktop app or in your
        browser; it&rsquo;s the same app either way.
      </>
    ),
  },
  {
    number: "02",
    title: "Hire your first agent",
    body: (
      <>
        Point mycel at a repository and create an agent. It gets a name, a
        face, and its own git worktree &mdash; a full working copy of your
        repo on its own branch. Choose the tool it runs (Claude Code, Gemini,
        Codex, Cursor, Aider), give it a job, and it starts working.
      </>
    ),
    artifact: (
      <ProductFrame
        srcDark="/screenshots/agents-dark.png"
        srcLight="/screenshots/agents-light.png"
        alt="The agents roster: each agent with a character face, status, repository, and cost"
        title="Agents"
        width={1440}
        height={700}
      />
    ),
  },
  {
    number: "03",
    title: "Put it in your pocket",
    body: (
      <>
        Connect Slack, WhatsApp, Telegram, Discord &mdash; the places you
        already talk. Agents join as themselves, post progress in the thread
        your team already reads, and answer when you reply from your phone.
        You never open another inbox.
      </>
    ),
  },
  {
    number: "04",
    title: "Watch, then review",
    body: (
      <>
        The Live feed streams every command, file read, and tool call the
        moment it happens, timed to the millisecond. When the work is done,
        open the diff and review it the way you&rsquo;d review a
        colleague&rsquo;s pull request &mdash; the work itself, not a summary
        of it.
      </>
    ),
    artifact: (
      <ProductFrame
        srcDark="/screenshots/live-dark.png"
        alt="The live feed: a stream of an agent's commands, file reads, and tool calls with per-action timing"
        title="Live"
        width={1440}
        height={900}
      />
    ),
  },
  {
    number: "05",
    title: "Read the bill",
    body: (
      <>
        Insights shows spend by agent, by model, and over time &mdash; read
        from each agent&rsquo;s own records, not estimated. Burn rate and
        your biggest cost driver are called out for any range you pick. The
        bill never surprises you, because you watched it happen.
      </>
    ),
  },
];

type Principle = {
  number: string;
  title: string;
  subtitle: string;
  paragraphs: string[];
  practice: ReactNode;
};

const PRINCIPLES: Principle[] = [
  {
    number: "01",
    title: "Isolation",
    subtitle: "Shared state is the enemy of parallel work.",
    paragraphs: [
      "Concurrent agents sharing a branch will destroy each other's work. That is not a tooling problem — it is physics. Parallel writers need separate spaces.",
      "So every mycel agent works in its own git worktree: a full copy of the repository on its own branch. No agent can overwrite another's changes. No merge conflicts from parallel edits. Work arrives as clean, reviewable diffs that merge the first time.",
      "Isolation is the foundation. Without it, every other conviction collapses under merge conflicts and broken builds.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> every agent&rsquo;s worktree is
        browsable in the Code view &mdash; file tree, branch, side-by-side
        diff.
      </>
    ),
  },
  {
    number: "02",
    title: "Communication",
    subtitle: "Coordination through structure, not scrollback.",
    paragraphs: [
      "Isolated agents working in silence produce fragmented results. A real team coordinates in channels — persistent, structured, addressed to people. Agents deserve the same, and you shouldn't have to open another inbox to give it to them.",
      "So mycel puts agents in the channels you already use. They join Slack, WhatsApp, Telegram, and Discord as themselves — named, recognizable — and answer when you reply. The difference between agents and a team of agents is structured communication. Without it, they duplicate effort and contradict each other. With it, they converge.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> the Apps view &mdash; 20+ platforms,
        every connection and every conversation flowing through one screen.
      </>
    ),
  },
  {
    number: "03",
    title: "Visibility",
    subtitle: "What you cannot see, you cannot trust.",
    paragraphs: [
      "Every command, every file, every tool call, every dollar — attributed and observable the moment it happens. When you can see the complete picture, you intervene early. Not after the damage.",
      "Visibility starts before the log lines. Every agent has a face, and the roster tells you who's working, who's idle, and who's stuck before you read a word. From there, the Live feed goes as deep as you want: every action, expandable to exactly what happened. There is no black box.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> the character roster for status
        at a glance; the Live feed for depth on demand.
      </>
    ),
  },
  {
    number: "04",
    title: "Cost",
    subtitle: "An unwatched agent is an unbounded bill.",
    paragraphs: [
      "A single agent left running can quietly consume real money in an hour. Multiply that by a fleet and cost stops being a footnote — it becomes the operating constraint.",
      "So mycel reads usage straight from each agent's own records — no estimates, no bookkeeping. Spend by agent, by model, over time; burn rate and the biggest cost driver called out for any range. The teams that scale agents successfully treat cost as a first-class signal, not an afterthought.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> Insights &mdash; spend, tokens, burn
        rate, broken down per agent and per model.
      </>
    ),
  },
  {
    number: "05",
    title: "Persistence",
    subtitle: "A tool runs once. A teammate finishes the job.",
    paragraphs: [
      "Most agents quit at the first obstacle. They produce a partial answer and call it done. That is not how hard problems get solved. Hard problems yield to repetition — try, fail, learn, try again. Each attempt sharper than the last.",
      "The difference between an assistant and a collaborator is what happens after the first failure. An assistant stops. A collaborator adapts and continues. Persistence is not about running longer. It is about getting closer with every iteration.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> agents keep working while
        you&rsquo;re away; check in from any connected app.
      </>
    ),
  },
  {
    number: "06",
    title: "Openness",
    subtitle: "Knowledge shared compounds. Knowledge hoarded decays.",
    paragraphs: [
      "Every team building with AI agents faces the same hard problems. Progress compounds when solutions are shared; keeping them behind walls is not just slow — it is self-defeating.",
      "Open source is not generosity. It is the only rational approach when the challenge exceeds what any one team can solve alone. The code is open. The ideas are open. The method is open.",
    ],
    practice: (
      <>
        <strong>In practice:</strong> MIT licensed, on GitHub, and it
        runs entirely on your machine.
      </>
    ),
  },
];

function MovementHeader({
  id,
  part,
  title,
  lede,
}: {
  id: string;
  part: string;
  title: string;
  lede: string;
}) {
  return (
    <ScrollReveal distance={24}>
      <span className="deck-eyebrow">{part}</span>
      <h2
        id={id}
        className="mt-4 font-headline text-3xl font-semibold tracking-tight text-on-background sm:text-4xl"
      >
        {title}
      </h2>
      <p className="method-body mt-4 max-w-2xl text-base sm:text-lg">{lede}</p>
    </ScrollReveal>
  );
}

export function MethodContent() {
  return (
    <main className="min-h-screen overflow-x-hidden">
      {/* Quiet warm wash — an essay page reads better without the canvas */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_70%_50%_at_50%_-10%,color-mix(in_srgb,var(--primary)_8%,transparent),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        <article className="mx-auto max-w-3xl px-5 pt-32 pb-24 sm:px-6 sm:pt-40">
          {/* ── Header ── */}
          <header className="mb-16 sm:mb-20">
            <FadeUp>
              <span className="deck-eyebrow">The method</span>
            </FadeUp>
            <FadeUp delay={0.08}>
              <h1 className="mt-5 font-headline text-5xl font-semibold leading-[1.02] tracking-tight text-on-background sm:text-6xl lg:text-7xl">
                How mycel works,{" "}
                <em className="accent-instrument text-primary">and why.</em>
              </h1>
            </FadeUp>
            <FadeUp delay={0.14}>
              <p className="method-body mt-7 max-w-2xl text-lg sm:text-xl">
                Running one AI agent is easy. Running a team of them &mdash;
                on real repositories, in parallel, without chaos &mdash; is a
                discipline. mycel is that discipline built into a product.
                Here is the whole method: the sequence, then the convictions
                underneath it.
              </p>
            </FadeUp>
          </header>

          <FadeUp delay={0.18}>
            <div className="method-divider mb-16 sm:mb-20" />
          </FadeUp>

          {/* ══════════ Movement I — the shape of it ══════════ */}
          <section aria-labelledby="method-shape">
            <MovementHeader
              id="method-shape"
              part="Part one"
              title="The shape of it"
              lede="From an empty terminal to a reviewed diff. Five steps, in the order they happen."
            />

            {/* The hyphae spine threads the steps together */}
            <div className="relative mt-12 sm:mt-16">
              <div
                aria-hidden="true"
                className="method-rail absolute bottom-2 left-[5px] top-2 w-px"
              />
              <ol className="space-y-14 sm:space-y-16">
                {STEPS.map((step, i) => (
                  <li key={step.number} className="relative pl-10 sm:pl-14">
                    <span
                      aria-hidden="true"
                      className="method-node absolute left-0 top-[0.45rem] h-[11px] w-[11px] rounded-full"
                    />
                    <ScrollReveal distance={24} delay={i === 0 ? 0.05 : 0}>
                      <div className="flex items-baseline gap-3">
                        <span className="font-label text-xs font-bold tracking-[0.2em] text-primary-text">
                          {step.number}
                        </span>
                        <h3 className="font-headline text-2xl font-semibold tracking-tight text-on-background sm:text-[1.75rem]">
                          {step.title}
                        </h3>
                      </div>
                      <p className="method-body mt-3 text-[15px] sm:text-base">
                        {step.body}
                      </p>
                      {step.artifact && (
                        <div className="mt-6 lg:-mr-24">{step.artifact}</div>
                      )}
                    </ScrollReveal>
                  </li>
                ))}
              </ol>
            </div>

            <ScrollReveal distance={20} className="mt-14 sm:mt-16">
              <p className="deck-serif text-xl leading-snug text-on-surface-variant sm:text-2xl">
                That&rsquo;s the whole loop &mdash; hire, delegate, watch,
                review, account.{" "}
                <span className="text-primary">Then hire another.</span>
              </p>
            </ScrollReveal>
          </section>

          <div className="method-divider my-16 sm:my-24" />

          {/* ══════════ Movement II — the convictions ══════════ */}
          <section aria-labelledby="method-convictions">
            <MovementHeader
              id="method-convictions"
              part="Part two"
              title="The convictions"
              lede="These are not features. They are design convictions — six of them — and every view, command, and architectural decision exists because it serves one."
            />

            <div className="mt-14 sm:mt-20">
              {PRINCIPLES.map((p, i) => (
                <section key={p.number} className={i > 0 ? "mt-16 sm:mt-20" : ""}>
                  <ScrollReveal distance={26}>
                    <span className="deck-eyebrow inline-flex items-center gap-3">
                      <span
                        aria-hidden="true"
                        className="inline-block h-px w-8 bg-primary/40"
                      />
                      Conviction {p.number}
                    </span>
                    <h3 className="mt-4 font-headline text-3xl font-semibold tracking-tight text-on-background sm:text-4xl">
                      {p.title}
                    </h3>
                    <p className="accent-instrument mt-2 text-lg text-on-surface-variant sm:text-xl">
                      {p.subtitle}
                    </p>
                    <div className="mt-6 space-y-5">
                      {p.paragraphs.map((para, j) => (
                        <p key={j} className="method-body text-[15px] sm:text-base">
                          {para}
                        </p>
                      ))}
                    </div>
                    <p className="method-practice mt-7">{p.practice}</p>
                  </ScrollReveal>
                  {i < PRINCIPLES.length - 1 && (
                    <div className="method-divider-short mt-16 sm:mt-20" />
                  )}
                </section>
              ))}
            </div>
          </section>

          {/* ── Closing ── */}
          <ScrollReveal distance={24} className="mt-20 border-t border-border pt-14 sm:mt-28 sm:pt-16">
            <p className="deck-serif text-2xl leading-snug text-on-surface-variant sm:text-3xl">
              None of this is aspirational. It is how mycel works today
              &mdash; visible, isolated, reachable, accountable,{" "}
              <span className="text-primary">and yours.</span>
            </p>
            <div className="mt-10 flex flex-col items-start gap-4 sm:flex-row sm:items-center">
              <Link
                href="/#install"
                className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-lg active:scale-[0.97]"
              >
                Get mycel
                <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
              </Link>
              <Link
                href="https://github.com/rpuneet/mycel"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 font-body text-sm font-medium text-on-surface-variant transition-colors hover:border-primary/30 hover:bg-surface-container hover:text-primary active:scale-[0.97]"
              >
                <Github className="h-4 w-4" aria-hidden="true" />
                Read the source
              </Link>
            </div>
          </ScrollReveal>
        </article>

        <div className="mx-auto max-w-6xl px-6">
          <div className="footer-separator" />
        </div>
        <Footer />
      </div>
    </main>
  );
}
