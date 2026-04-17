"use client";

import React, { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { ChevronLeft, ChevronRight, Pause, Play } from "lucide-react";

/** ============================================================
 *  Reusable Carousel Component
 *  ============================================================ */

interface CarouselStep {
  id: string;
  title: string;
  render: () => React.ReactNode;
}

interface CarouselProps {
  title: string;
  steps: CarouselStep[];
  intervalMs?: number;
  className?: string;
}

function Carousel({
  title,
  steps,
  intervalMs = 5000,
  className = "",
}: CarouselProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const [isPaused, setIsPaused] = useState(false);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  // Auto-advance carousel
  useEffect(() => {
    if (isPaused || steps.length === 0) return;

    timerRef.current = setInterval(() => {
      setCurrentStep((prev) => (prev + 1) % steps.length);
    }, intervalMs);

    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [isPaused, intervalMs, steps.length]);

  const goToStep = (step: number) => {
    setCurrentStep(step);
    setIsPaused(true);
  };

  const nextStep = () => {
    setCurrentStep((prev) => (prev + 1) % steps.length);
    setIsPaused(true);
  };

  const prevStep = () => {
    setCurrentStep((prev) => (prev - 1 + steps.length) % steps.length);
    setIsPaused(true);
  };

  const togglePlayPause = () => {
    setIsPaused((prev) => !prev);
  };

  const activeStep = steps[currentStep];
  const stepNumber = String(currentStep + 1).padStart(2, "0");

  return (
    <div className={`flex flex-col gap-6 ${className}`}>
      {/* Header with title and controls */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 className="text-2xl font-semibold tracking-tight text-foreground">
            {title}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {activeStep.title} · Step {stepNumber}
          </p>
        </div>

        {/* Controls */}
        <div className="flex items-center gap-2">
          <button
            onClick={prevStep}
            className="inline-flex items-center justify-center h-10 w-10 rounded-lg border border-border bg-background hover:bg-accent transition-colors"
            aria-label="Previous step"
          >
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          </button>

          <button
            onClick={togglePlayPause}
            className="inline-flex items-center justify-center h-10 w-10 rounded-lg border border-border bg-background hover:bg-accent transition-colors"
            aria-label={isPaused ? "Play" : "Pause"}
          >
            {isPaused ? (
              <Play className="h-4 w-4" aria-hidden="true" />
            ) : (
              <Pause className="h-4 w-4" aria-hidden="true" />
            )}
          </button>

          <button
            onClick={nextStep}
            className="inline-flex items-center justify-center h-10 w-10 rounded-lg border border-border bg-background hover:bg-accent transition-colors"
            aria-label="Next step"
          >
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </div>

      {/* Main carousel content */}
      <div
        className="relative rounded-xl md:rounded-2xl border border-border bg-muted/30 p-4 md:p-6 lg:p-8 min-h-[340px] md:min-h-[380px] lg:min-h-[420px] overflow-y-auto"
        onMouseEnter={() => setIsPaused(true)}
        onMouseLeave={() => setIsPaused(false)}
      >
        <AnimatePresence mode="wait">
          <motion.div
            key={currentStep}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -20 }}
            transition={{ duration: 0.4, ease: "easeOut" }}
          >
            {activeStep.render()}
          </motion.div>
        </AnimatePresence>
      </div>

      {/* Step indicators and progress */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-1.5 md:gap-2 lg:gap-3 flex-wrap">
          {steps.map((_, index) => (
            <button
              key={index}
              onClick={() => goToStep(index)}
              className={`h-1.5 md:h-2 rounded-full transition-all duration-300 touch-target-44 ${
                index === currentStep
                  ? "bg-primary w-6 md:w-8"
                  : "bg-muted-foreground/30 w-1.5 md:w-2 hover:bg-muted-foreground/50"
              }`}
              aria-label={`Go to step ${index + 1}`}
              aria-current={index === currentStep ? "step" : undefined}
            />
          ))}
        </div>

        <div className="text-xs md:text-sm font-semibold text-muted-foreground">
          {stepNumber} / {String(steps.length).padStart(2, "0")}
        </div>
      </div>
    </div>
  );
}

/** ============================================================
 *  Demo Content Components
 *  ============================================================ */

// Agents Carousel Demo
const AGENTS_STEPS: CarouselStep[] = [
  {
    id: "agents-1",
    title: "Live Agent Status",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc status
        </div>
        <div className="space-y-3">
          {[
            {
              name: "engineer-01",
              role: "engineer",
              state: "working",
              symbol: "✽",
            },
            {
              name: "tech-lead-01",
              role: "tech-lead",
              state: "tool",
              symbol: "⏺",
            },
            { name: "qa-nova", role: "qa", state: "working", symbol: "✻" },
          ].map((agent) => (
            <motion.div
              key={agent.name}
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.3 }}
              className="flex items-center justify-between p-3 rounded-lg bg-background/50 border border-border/50"
            >
              <div className="flex items-center gap-3">
                <span className="text-primary">{agent.symbol}</span>
                <span className="font-semibold">{agent.name}</span>
              </div>
              <div className="flex items-center gap-4 text-xs text-muted-foreground">
                <span>{agent.role}</span>
                <span
                  className={
                    agent.state === "working"
                      ? "text-success"
                      : "text-[var(--terminal-command)]"
                  }
                >
                  {agent.state}
                </span>
              </div>
            </motion.div>
          ))}
        </div>
      </div>
    ),
  },
  {
    id: "agents-2",
    title: "Agent Context Inspection",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc agent peek engineer-01
        </div>
        <div className="space-y-3 p-4 rounded-lg bg-background/50 border border-border/50">
          <div className="font-semibold text-foreground">engineer-01</div>
          <div className="text-xs text-muted-foreground">
            State:{" "}
            <span className="text-[var(--terminal-command)]">
              tool activity
            </span>
          </div>
          <div className="space-y-2 mt-3">
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.1 }}
              className="text-xs text-muted-foreground border-l-2 border-primary pl-2"
            >
              📍 Loading project memory: &quot;Zod patterns&quot;
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.2 }}
              className="text-xs text-muted-foreground border-l-2 border-primary pl-2"
            >
              ⏺ Writing: validation_test.go
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.3 }}
              className="text-xs text-primary font-semibold border-l-2 border-primary pl-2"
            >
              ⚡ Ready to submit PR
            </motion.div>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: "agents-3",
    title: "Send Instructions",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc agent send engineer-01 &quot;status&quot;
        </div>
        <div className="space-y-3">
          <div className="p-4 rounded-lg bg-primary/10 border border-primary/30">
            <div className="text-xs font-semibold text-primary uppercase">
              YOU
            </div>
            <div className="text-sm text-foreground mt-2">
              @engineer-01 implement validation and report back.
            </div>
          </div>
          <div className="p-4 rounded-lg bg-success/10 border border-success/30">
            <div className="text-xs font-semibold text-success uppercase">
              engineer-01
            </div>
            <div className="text-sm text-foreground mt-2">
              Done! PR #347 ready for review. Tests passing locally.
            </div>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: "agents-4",
    title: "Memory & Context",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Agent Intelligence
        </div>
        <div className="space-y-3">
          <div className="p-4 rounded-lg bg-background/50 border border-border/50">
            <div className="text-xs font-semibold text-foreground">
              Project Context
            </div>
            <ul className="text-xs text-muted-foreground list-disc list-inside mt-2 space-y-1">
              <li>Architecture: Modular microservices</li>
              <li>Stack: Next.js, React, TypeScript</li>
              <li>Testing: Jest + React Testing Library</li>
            </ul>
          </div>
          <div className="p-4 rounded-lg bg-background/50 border border-border/50">
            <div className="text-xs font-semibold text-foreground">
              Recent Experience
            </div>
            <ul className="text-xs text-muted-foreground list-disc list-inside mt-2 space-y-1">
              <li>Fixed 3 similar bugs this week</li>
              <li>Pattern: Zod schema validation</li>
              <li>Success rate: 100% in last 5 tasks</li>
            </ul>
          </div>
        </div>
      </div>
    ),
  },
];

export function AgentCarouselDemo() {
  return <Carousel title="Agents" steps={AGENTS_STEPS} intervalMs={5000} />;
}

// Channels Carousel Demo
const CHANNELS_STEPS: CarouselStep[] = [
  {
    id: "channels-1",
    title: "Organic Coordination",
    render: () => (
      <div className="space-y-3 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          #general
        </div>
        <div className="space-y-2">
          {[
            {
              user: "root",
              msg: "3 agents active. QA standup at 11am.",
              tag: "system",
            },
            {
              user: "manager-atlas",
              msg: "Assigning tasks for auth epic to #engineering.",
              tag: "manager",
            },
            {
              user: "qa-nova",
              msg: "Edge-case matrix ready. Ping when merged.",
              tag: "qa",
            },
          ].map((msg, i) => (
            <motion.div
              key={i}
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ delay: i * 0.1 }}
              className="p-3 rounded-lg bg-background/50 border border-border/50"
            >
              <div
                className={`text-xs font-semibold uppercase tracking-wider ${msg.tag === "system" ? "text-primary" : msg.tag === "manager" ? "text-accent" : "text-[var(--terminal-command)]"}`}
              >
                {msg.user}
              </div>
              <div className="text-sm text-foreground mt-1">{msg.msg}</div>
            </motion.div>
          ))}
        </div>
      </div>
    ),
  },
  {
    id: "channels-2",
    title: "Direct Participation",
    render: () => (
      <div className="space-y-3 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          #engineering
        </div>
        <div className="space-y-2">
          <div className="p-3 rounded-lg bg-background/50 border border-border/50">
            <div className="text-xs font-semibold text-muted-foreground uppercase">
              qa-nova
            </div>
            <div className="text-sm text-foreground mt-1">
              Smoke tests passed on #347 🎉
            </div>
          </div>
          <div className="p-3 rounded-lg bg-primary/10 border border-primary/30">
            <div className="text-xs font-semibold text-primary uppercase">
              YOU
            </div>
            <div className="text-sm text-foreground mt-1">
              @tech-lead-01 can we merge #347 now?
            </div>
          </div>
          <div className="p-3 rounded-lg bg-success/10 border border-success/30">
            <div className="text-xs font-semibold text-success uppercase">
              tech-lead-01
            </div>
            <div className="text-sm text-foreground mt-1">
              LGTM! Merging now. Great work team.
            </div>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: "channels-3",
    title: "Full Traceability",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc channel history #general --search &quot;PR #347&quot;
        </div>
        <div className="space-y-2 p-4 rounded-lg bg-background/50 border border-border/50">
          <div className="text-xs text-muted-foreground">5 results found</div>
          <div className="space-y-2 mt-3">
            {[
              { time: "10:32", msg: "PR #347 submitted for review" },
              { time: "10:45", msg: "QA starting test pass" },
              { time: "10:52", msg: "All checks green ✓" },
              { time: "11:00", msg: "Tech lead approval" },
              { time: "11:02", msg: "PR merged to main" },
            ].map((entry, i) => (
              <motion.div
                key={i}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: i * 0.1 }}
                className="flex gap-3 text-xs"
              >
                <span className="text-muted-foreground w-12">{entry.time}</span>
                <span className="text-foreground">{entry.msg}</span>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    ),
  },
];

export function ChannelsCarouselDemo() {
  return <Carousel title="Channels" steps={CHANNELS_STEPS} intervalMs={5000} />;
}

// Cron Carousel Demo
const CRON_STEPS: CarouselStep[] = [
  {
    id: "cron-1",
    title: "Scheduled Workflows",
    render: () => (
      <div className="space-y-3 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc cron list
        </div>
        <div className="space-y-2">
          {[
            {
              name: "cron-nightly",
              schedule: "01:00 (7h 32m)",
              state: "scheduled",
              icon: "📅",
            },
            {
              name: "cron-health",
              schedule: "* * * * * (hourly)",
              state: "running",
              icon: "⚙️",
            },
            {
              name: "cron-deps",
              schedule: "09:00 (Mon)",
              state: "idle",
              icon: "📦",
            },
          ].map((job) => (
            <motion.div
              key={job.name}
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.3 }}
              className="p-3 rounded-lg bg-background/50 border border-border/50"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span>{job.icon}</span>
                  <span className="font-semibold">{job.name}</span>
                </div>
                <span
                  className={`text-xs font-semibold ${job.state === "running" ? "text-success" : "text-accent"}`}
                >
                  {job.state}
                </span>
              </div>
              <div className="text-xs text-muted-foreground mt-1">
                {job.schedule}
              </div>
            </motion.div>
          ))}
        </div>
      </div>
    ),
  },
  {
    id: "cron-2",
    title: "Real-time Execution",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          bc cron run cron-health [logs]
        </div>
        <div className="space-y-2 p-4 rounded-lg bg-background/50 border border-border/50">
          <div className="text-xs text-success font-semibold">
            Status: RUNNING
          </div>
          <div className="space-y-1 text-xs text-foreground mt-3">
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0 }}
            >
              ⏺ Smoke test: agent health
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.15 }}
            >
              ✓ Channel send working
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.3 }}
            >
              ✓ Agent health verified
            </motion.div>
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.45 }}
              className="text-success font-semibold"
            >
              ✻ 0 regressions found
            </motion.div>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: "cron-3",
    title: "One-Command Definition",
    render: () => (
      <div className="space-y-4 font-mono text-sm">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Create Cron Job
        </div>
        <div className="p-4 rounded-lg bg-background border border-border">
          <div className="text-foreground text-xs leading-relaxed">
            <span className="text-success">$</span>{" "}
            <span className="text-muted-foreground">bc cron add</span>{" "}
            <span className="text-[var(--terminal-command)]">
              audit-codebase
            </span>{" "}
            <br />
            <span className="ml-4">
              <span className="text-accent">--schedule</span>{" "}
              <span className="text-green-400">&quot;0 2 * * *&quot;</span>{" "}
              <br />
            </span>
            <span className="ml-4">
              <span className="text-accent">--role</span>{" "}
              <span className="text-green-400">&quot;qa&quot;</span> <br />
            </span>
            <span className="ml-4">
              <span className="text-accent">--task</span>{" "}
              <span className="text-green-400">
                &quot;Nightly audit + drift detection&quot;
              </span>
            </span>
          </div>
        </div>
        <div className="text-xs text-success">
          ✓ Cron job scheduled successfully
        </div>
      </div>
    ),
  },
];

export function CronCarouselDemo() {
  return <Carousel title="Cron" steps={CRON_STEPS} intervalMs={5000} />;
}

/** ============================================================
 *  Main Demo Section
 *  ============================================================ */

export default function InteractiveCarouselDemos() {
  return (
    <section className="py-24 border-t border-b border-border">
      <div className="mx-auto max-w-7xl px-6">
        <div className="mb-12">
          <h2 className="text-4xl font-bold tracking-tight sm:text-5xl">
            Product in Motion
          </h2>
          <p className="mt-4 text-lg text-muted-foreground max-w-2xl">
            Explore interactive demos of Agents, Channels, and Cron Jobs. Each
            carousel showcases core capabilities with smooth transitions and
            real-time examples.
          </p>
        </div>

        <div className="space-y-16">
          <AgentCarouselDemo />
          <ChannelsCarouselDemo />
          <CronCarouselDemo />
        </div>
      </div>
    </section>
  );
}
