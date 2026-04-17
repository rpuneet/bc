"use client";

import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import {
  ArrowRight,
  CheckCircle2,
  Mail,
  Zap,
  Shield,
  Users,
  Terminal,
  GitBranch,
  Eye,
} from "lucide-react";
import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { RevealSection } from "../_components/TerminalComponents";

const BENEFITS = [
  {
    icon: Terminal,
    title: "Full CLI Access",
    description:
      "Agents, channels, cost controls, roles, cron, secrets, and MCP. Every feature from the command line.",
  },
  {
    icon: Users,
    title: "Multi-Agent Orchestration",
    description:
      "Run multiple agents in parallel. Git worktree isolation prevents conflicts. Channels enable coordination.",
  },
  {
    icon: Shield,
    title: "Cost Control",
    description:
      "Per-agent budgets, real-time token tracking, and automatic hard stops.",
  },
  {
    icon: GitBranch,
    title: "Agent-Agnostic",
    description:
      "Works with Claude Code, Cursor, Codex, Gemini, Aider, and any CLI agent. No vendor lock-in.",
  },
  {
    icon: Eye,
    title: "Real-Time Visibility",
    description:
      "Watch agents coordinate in structured channels. Monitor status and cost across your team.",
  },
  {
    icon: Zap,
    title: "Cron, Secrets & MCP",
    description:
      "Schedule tasks, manage encrypted credentials, and extend agents with MCP servers.",
  },
];

const SCREENSHOTS = [
  {
    src: "/screenshots/dashboard-01-home.png",
    alt: "mycel dashboard showing active agents, channels, total cost, and token usage with agent status table",
    label: "Dashboard Overview",
  },
  {
    src: "/screenshots/dashboard-03-channels.png",
    alt: "bc channels view showing real-time agent-to-agent communication in shared channels",
    label: "Agent Channels",
  },
  {
    src: "/screenshots/dashboard-04-costs.png",
    alt: "bc costs view showing daily cost trends, per-agent cost breakdown, and total token usage",
    label: "Cost Tracking",
  },
];

const GOOGLE_FORM_ACTION =
  "https://docs.google.com/forms/d/e/1FAIpQLSc_aJ3S3nV5EizpkzTZnN7H5UykoANpC8jet2M7J0Qo3rhG8Q/formResponse";
const GOOGLE_FORM_EMAIL_FIELD = "entry.843755864";

export default function Waitlist() {
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeScreenshot, setActiveScreenshot] = useState(0);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const emailRegex = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
    if (!emailRegex.test(email)) {
      setError("Please enter a valid email address.");
      return;
    }

    setSubmitting(true);

    try {
      await new Promise<void>((resolve, reject) => {
        const iframe = document.createElement("iframe");
        iframe.name = "mycel-waitlist-frame";
        iframe.style.display = "none";
        document.body.appendChild(iframe);

        const form = document.createElement("form");
        form.method = "POST";
        form.action = GOOGLE_FORM_ACTION;
        form.target = "mycel-waitlist-frame";

        const input = document.createElement("input");
        input.type = "hidden";
        input.name = GOOGLE_FORM_EMAIL_FIELD;
        input.value = email;
        form.appendChild(input);

        const cleanup = () => {
          try {
            document.body.removeChild(iframe);
          } catch {
            /* already removed */
          }
          try {
            document.body.removeChild(form);
          } catch {
            /* already removed */
          }
        };

        const timeout = setTimeout(() => {
          cleanup();
          resolve();
        }, 5000);

        iframe.onload = () => {
          clearTimeout(timeout);
          cleanup();
          resolve();
        };
        iframe.onerror = () => {
          clearTimeout(timeout);
          cleanup();
          reject(
            new Error(
              "Network error — please check your connection and try again.",
            ),
          );
        };

        document.body.appendChild(form);
        form.submit();
      });

      setSubmitted(true);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="min-h-screen bg-background selection:bg-primary selection:text-primary-foreground">
      <Nav />
      <section className="relative overflow-hidden">
        <div
          className="absolute inset-0 overflow-hidden opacity-[0.03] pointer-events-none"
          aria-hidden="true"
        >
          {[
            "$ bc up",
            "$ bc agent list --full",
            "$ bc cost usage --monthly",
            "$ bc channel history #eng --since 1h",
          ].map((cmd, row) =>
            Array.from({ length: 5 }).map((_, col) => (
              <span
                key={`${row}-${col}`}
                className="absolute font-mono text-xs text-foreground whitespace-nowrap animate-[slide-x_25s_linear_infinite] will-change-transform"
                style={{
                  top: `${5 + (row * 5 + col * 4) * 5}%`,
                  animationDelay: `${col * 3 + row * 1.2}s`,
                }}
              >
                {cmd}
              </span>
            )),
          )}
        </div>
        <div className="absolute inset-x-0 top-0 h-96 bg-gradient-to-b from-primary/5 to-transparent pointer-events-none" />
        <div className="relative mx-auto max-w-5xl px-6 pt-24 pb-16 lg:pt-32 lg:pb-24">
          <div className="text-center space-y-6 mb-16">
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5 }}
            >
              <span className="inline-flex items-center gap-2 rounded-full border border-primary/20 bg-primary/5 px-4 py-1.5 text-xs font-mono font-bold text-primary">
                <span className="h-1.5 w-1.5 rounded-full bg-primary animate-pulse" />
                EARLY ACCESS
              </span>
            </motion.div>
            <motion.h1
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5, delay: 0.1 }}
              className="text-4xl sm:text-5xl lg:text-6xl font-bold tracking-tight"
            >
              Orchestrate AI agents
              <br />
              <span className="text-primary">from your terminal.</span>
            </motion.h1>
            <motion.p
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5, delay: 0.2 }}
              className="text-lg text-muted-foreground max-w-xl mx-auto leading-relaxed"
            >
              mycel coordinates teams of AI coding agents with isolated worktrees,
              shared channels, and cost controls. Open source and local-first.
              Get notified when we launch.
            </motion.p>
          </div>

          <motion.div
            initial={{ opacity: 0, y: 30 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.3 }}
            className="mx-auto max-w-lg"
          >
            <AnimatePresence mode="wait">
              {!submitted ? (
                <motion.div
                  key="form"
                  exit={{ opacity: 0, scale: 0.95 }}
                  className="relative"
                >
                  <div className="rounded-xl border border-border bg-terminal-bg overflow-hidden shadow-2xl shadow-black/20">
                    <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5">
                      <div className="flex gap-1.5">
                        <div className="h-3 w-3 rounded-full bg-[var(--traffic-red)]" />
                        <div className="h-3 w-3 rounded-full bg-[var(--traffic-yellow)]" />
                        <div className="h-3 w-3 rounded-full bg-[var(--traffic-green)]" />
                      </div>
                      <span className="text-xs font-mono text-white/30 ml-2">
                        mycel waitlist --join
                      </span>
                    </div>
                    <div className="p-6 sm:p-8">
                      <form onSubmit={handleSubmit} className="space-y-5">
                        <div className="space-y-2">
                          <label
                            htmlFor="email"
                            className="text-sm font-mono text-white/50"
                          >
                            email:
                          </label>
                          <div className="relative">
                            <Mail
                              className="absolute left-4 top-1/2 -translate-y-1/2 text-white/30"
                              size={18}
                              aria-hidden="true"
                            />
                            <input
                              id="email"
                              type="email"
                              required
                              value={email}
                              onChange={(e) => {
                                setEmail(e.target.value);
                                setError(null);
                              }}
                              placeholder="you@company.com"
                              pattern="[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}"
                              maxLength={254}
                              className="h-12 w-full rounded-lg border border-white/10 bg-white/5 pl-12 pr-4 text-sm text-white font-mono outline-none transition-all placeholder:text-white/20 focus:border-primary/50 focus:ring-2 focus:ring-primary/10"
                            />
                          </div>
                        </div>
                        <button
                          type="submit"
                          disabled={submitting}
                          className="group h-12 w-full rounded-lg bg-primary px-6 text-sm font-bold font-mono text-primary-foreground transition-all hover:opacity-90 active:scale-[0.98] flex items-center justify-center gap-2 disabled:opacity-50"
                        >
                          {submitting ? (
                            <div className="h-4 w-4 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                          ) : (
                            <>
                              Get Early Access{" "}
                              <ArrowRight
                                size={16}
                                className="transition-transform group-hover:translate-x-1"
                                aria-hidden="true"
                              />
                            </>
                          )}
                        </button>
                        {error && (
                          <p
                            className="text-center text-sm font-mono text-red-400"
                            role="alert"
                          >
                            {error}
                          </p>
                        )}
                        <p className="text-center text-[11px] text-white/25 font-mono">
                          no spam. unsubscribe anytime. we respect your inbox.
                        </p>
                      </form>
                    </div>
                  </div>
                </motion.div>
              ) : (
                <motion.div
                  key="success"
                  initial={{ opacity: 0, scale: 0.9 }}
                  animate={{ opacity: 1, scale: 1 }}
                  className="rounded-xl border border-success/20 bg-terminal-bg overflow-hidden shadow-2xl shadow-black/20"
                >
                  <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5">
                    <div className="flex gap-1.5">
                      <div className="h-3 w-3 rounded-full bg-[var(--traffic-red)]" />
                      <div className="h-3 w-3 rounded-full bg-[var(--traffic-yellow)]" />
                      <div className="h-3 w-3 rounded-full bg-[var(--traffic-green)]" />
                    </div>
                    <span className="text-xs font-mono text-white/30 ml-2">
                      mycel waitlist --status
                    </span>
                  </div>
                  <div className="p-8 sm:p-12 text-center space-y-6">
                    <motion.div
                      initial={{ scale: 0 }}
                      animate={{ scale: 1 }}
                      transition={{
                        type: "spring",
                        stiffness: 200,
                        delay: 0.2,
                      }}
                      className="mx-auto h-16 w-16 rounded-full bg-success/10 border border-success/20 text-success flex items-center justify-center"
                    >
                      <CheckCircle2 size={32} aria-hidden="true" />
                    </motion.div>
                    <div className="space-y-2">
                      <pre className="text-sm font-mono text-success">
                        You&apos;re on the list
                      </pre>
                      <p className="text-white/50 font-mono text-sm">
                        We&apos;ll notify you when mycel is ready for early access.
                        In the meantime, check out the{" "}
                        <a
                          href="https://github.com/rpuneet/bc"
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary hover:underline"
                        >
                          GitHub repo
                        </a>{" "}
                        — mycel is open source.
                      </p>
                    </div>
                    <div className="pt-2">
                      <button
                        onClick={() => {
                          setSubmitted(false);
                          setEmail("");
                        }}
                        className="text-xs font-mono text-primary hover:underline"
                        aria-label="Go back and use a different email address"
                      >
                        use a different email
                      </button>
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </motion.div>

          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.6 }}
            className="flex items-center justify-center gap-8 sm:gap-12 mt-12 text-center"
          >
            <div>
              <div className="text-2xl font-bold font-mono tracking-tighter text-foreground">
                8
              </div>
              <div className="text-[10px] uppercase tracking-widest font-bold text-muted-foreground/50">
                AI Tools
              </div>
            </div>
            <div className="h-8 w-px bg-border" />
            <div>
              <div className="text-2xl font-bold font-mono tracking-tighter text-foreground">
                Open Source
              </div>
              <div className="text-[10px] uppercase tracking-widest font-bold text-muted-foreground/50">
                MIT Licensed
              </div>
            </div>
            <div className="h-8 w-px bg-border" />
            <div>
              <div className="text-2xl font-bold font-mono tracking-tighter text-primary">
                Local-First
              </div>
              <div className="text-[10px] uppercase tracking-widest font-bold text-muted-foreground/50">
                Your Machine
              </div>
            </div>
          </motion.div>
        </div>
      </section>

      {/* Dashboard Preview Section */}
      <section className="py-24 border-t border-border">
        <div className="mx-auto max-w-5xl px-6">
          <RevealSection className="text-center mb-12">
            <h2 className="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
              See what you&apos;ll get
            </h2>
            <p className="text-muted-foreground max-w-lg mx-auto">
              A real-time dashboard ships with mycel. Monitor agents, channels,
              and costs from your browser.
            </p>
          </RevealSection>
          <RevealSection delay={0.2}>
            <div className="rounded-xl border border-border bg-terminal-bg overflow-hidden shadow-2xl shadow-black/20">
              <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5">
                <div className="flex gap-1.5">
                  <div className="h-3 w-3 rounded-full bg-[var(--traffic-red)]" />
                  <div className="h-3 w-3 rounded-full bg-[var(--traffic-yellow)]" />
                  <div className="h-3 w-3 rounded-full bg-[var(--traffic-green)]" />
                </div>
                <span className="text-xs font-mono text-white/30 ml-2">
                  localhost:9374
                </span>
              </div>
              <div className="relative">
                <AnimatePresence mode="wait">
                  <motion.img
                    key={activeScreenshot}
                    src={SCREENSHOTS[activeScreenshot].src}
                    alt={SCREENSHOTS[activeScreenshot].alt}
                    loading="lazy"
                    decoding="async"
                    className="w-full h-auto"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.3 }}
                  />
                </AnimatePresence>
              </div>
            </div>
            <div
              className="flex justify-center gap-3 mt-6"
              role="tablist"
              aria-label="Dashboard screenshots"
            >
              {SCREENSHOTS.map((screenshot, i) => (
                <button
                  key={screenshot.label}
                  onClick={() => setActiveScreenshot(i)}
                  role="tab"
                  aria-selected={activeScreenshot === i}
                  aria-label={screenshot.label}
                  className={`px-4 py-2 rounded-lg text-xs font-mono font-bold transition-all ${
                    activeScreenshot === i
                      ? "bg-primary text-primary-foreground"
                      : "bg-accent/50 text-muted-foreground hover:bg-accent"
                  }`}
                >
                  {screenshot.label}
                </button>
              ))}
            </div>
          </RevealSection>
        </div>
      </section>

      {/* Benefits Section */}
      <section className="py-24 border-t border-border">
        <div className="mx-auto max-w-5xl px-6">
          <RevealSection className="text-center mb-16">
            <h2 className="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
              What you get with early access
            </h2>
            <p className="text-muted-foreground max-w-lg mx-auto">
              Full access to every feature plus direct influence on the roadmap.
            </p>
          </RevealSection>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
            {BENEFITS.map((benefit, i) => (
              <RevealSection key={benefit.title} delay={i * 0.1}>
                <div className="group rounded-xl border border-border bg-card p-6 hover:border-primary/20 hover:bg-primary/[0.02] transition-all h-full">
                  <div className="h-10 w-10 rounded-lg bg-primary/10 text-primary flex items-center justify-center mb-4">
                    <benefit.icon size={20} aria-hidden="true" />
                  </div>
                  <h3 className="font-bold mb-2">{benefit.title}</h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {benefit.description}
                  </p>
                </div>
              </RevealSection>
            ))}
          </div>
        </div>
      </section>

      {/* How It Works Section */}
      <section className="py-24 border-t border-border bg-accent/20">
        <div className="mx-auto max-w-3xl px-6">
          <RevealSection className="text-center mb-16">
            <h2 className="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
              How it works
            </h2>
          </RevealSection>
          <div className="space-y-8">
            {[
              {
                step: "01",
                title: "Sign up for early access",
                description:
                  "Drop your email above. We'll notify you when mycel is ready.",
              },
              {
                step: "02",
                title: "Install locally",
                description:
                  "mycel runs on your machine. One command to install, one to start. No cloud accounts required.",
              },
              {
                step: "03",
                title: "Orchestrate your agents",
                description:
                  "Assign roles to agents and let them work in parallel. Each agent gets its own git worktree.",
              },
              {
                step: "04",
                title: "Shape the product",
                description:
                  "Early access users shape the roadmap. Your feedback drives what we build.",
              },
            ].map((item, i) => (
              <RevealSection key={item.step} delay={i * 0.1}>
                <div className="flex gap-6 items-start">
                  <div className="flex-shrink-0 h-10 w-10 rounded-lg bg-primary/10 text-primary font-mono font-bold text-sm flex items-center justify-center">
                    {item.step}
                  </div>
                  <div>
                    <h3 className="font-bold mb-1">{item.title}</h3>
                    <p className="text-sm text-muted-foreground leading-relaxed">
                      {item.description}
                    </p>
                  </div>
                </div>
              </RevealSection>
            ))}
          </div>
        </div>
      </section>

      {/* FAQ Section */}
      <section className="py-24 border-t border-border">
        <div className="mx-auto max-w-3xl px-6">
          <RevealSection className="text-center mb-16">
            <h2 className="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
              Frequently asked questions
            </h2>
          </RevealSection>
          <div className="space-y-6">
            {[
              {
                q: "What is mycel?",
                a: "mycel coordinates multiple AI coding agents working in parallel on isolated git worktrees. No merge conflicts, no context loss.",
              },
              {
                q: "How is this different from using a single AI agent?",
                a: "A single agent works on one task at a time. mycel runs multiple agents in parallel, each on its own branch, communicating through structured channels. Cost controls and real-time visibility included.",
              },
              {
                q: "Which AI tools does mycel support?",
                a: "Claude Code, Cursor, Codex, Gemini, Aider, OpenCode, OpenClaw, and any CLI agent. Configure providers in a TOML file.",
              },
              {
                q: "Do I need to change how my agents work?",
                a: "No. Your agents keep running the same commands. mycel adds the coordination layer: worktree isolation, channels, persistent memory, and cost tracking.",
              },
              {
                q: "Is mycel open source?",
                a: "Yes. Open source, runs on your machine. Inspect the code, contribute, self-host. Early access includes all features.",
              },
              {
                q: "Does mycel require a cloud account?",
                a: "No. mycel runs on your machine using tmux sessions and local git worktrees. No data leaves your machine unless you configure external AI providers.",
              },
              {
                q: "How does early access work?",
                a: "Sign up with your email. You'll get install instructions, full access, and a direct line to the team.",
              },
            ].map((faq) => (
              <RevealSection key={faq.q}>
                <details className="group rounded-xl border border-border bg-card">
                  <summary className="flex cursor-pointer items-center justify-between p-6 font-semibold text-sm">
                    {faq.q}
                    <span className="ml-4 text-muted-foreground transition-transform group-open:rotate-45">
                      +
                    </span>
                  </summary>
                  <p className="px-6 pb-6 text-sm text-muted-foreground leading-relaxed">
                    {faq.a}
                  </p>
                </details>
              </RevealSection>
            ))}
          </div>
        </div>
      </section>

      <Footer />
    </main>
  );
}
