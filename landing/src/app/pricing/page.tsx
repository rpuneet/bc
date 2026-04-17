"use client";

import Link from "next/link";
import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";
import {
  Copy,
  Check,
  ChevronDown,
  Apple,
  Container,
  GitBranch,
  ExternalLink,
} from "lucide-react";
import { TechTags } from "../_components/TechIcon";

const fadeUp = {
  hidden: { opacity: 0, y: 30 },
  visible: (i: number) => ({
    opacity: 1,
    y: 0,
    transition: { delay: i * 0.12, duration: 0.6, ease: "easeOut" as const },
  }),
};

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={handleCopy}
      className="shrink-0 p-1 rounded hover:bg-accent/30 transition-colors"
      aria-label="Copy to clipboard"
    >
      {copied ? (
        <Check className="h-3.5 w-3.5 text-success" />
      ) : (
        <Copy className="h-3.5 w-3.5 text-muted-foreground" />
      )}
    </button>
  );
}

interface InstallRowProps {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  command: string;
}

function InstallRow({ icon: Icon, title, command }: InstallRowProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div
      className="group rounded-md border border-border/40 hover:border-border/70 transition-all cursor-pointer"
      onClick={() => setExpanded(!expanded)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") setExpanded(!expanded);
      }}
      role="button"
      tabIndex={0}
      aria-expanded={expanded}
    >
      <div className="flex items-center gap-2.5 px-3 py-1.5">
        <Icon className="h-3 w-3 text-muted-foreground shrink-0" />
        <span className="text-sm">{title}</span>
        <ChevronDown
          className={`h-2.5 w-2.5 text-muted-foreground/40 ml-auto transition-transform ${expanded ? "rotate-180" : ""}`}
        />
      </div>
      <AnimatePresence>
        {expanded && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden"
          >
            <div className="px-3 pb-2">
              <div className="flex items-center gap-2 rounded bg-background/60 border border-border/30 px-2.5 py-1.5 font-mono text-[11px]">
                <code className="text-muted-foreground flex-1 overflow-x-auto whitespace-nowrap">
                  {command}
                </code>
                <CopyButton text={command} />
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}


interface FAQItemProps {
  question: string;
  answer: string;
}

function FAQItem({ question, answer }: FAQItemProps) {
  const [open, setOpen] = useState(false);

  return (
    <div className="border-b border-border/40">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between py-3 text-left"
        aria-expanded={open}
      >
        <span className="font-semibold text-sm">{question}</span>
        <ChevronDown
          className={`h-4 w-4 text-muted-foreground shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden"
          >
            <p className="pb-3 text-sm text-muted-foreground leading-relaxed">
              {answer}
            </p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

export default function PricingPage() {
  return (
    <main className="min-h-screen selection:bg-primary/20 selection:text-foreground">
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.04),transparent)] dark:bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,rgba(234,88,12,0.08),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        <div className="mx-auto max-w-5xl px-4 sm:px-6 py-16 sm:py-24">
          <motion.div
            initial="hidden"
            animate="visible"
            className="text-center mb-16"
          >
            <motion.h1
              variants={fadeUp}
              custom={1}
              className="text-4xl font-bold tracking-tight sm:text-6xl"
            >
              Pricing
            </motion.h1>
            <motion.p
              variants={fadeUp}
              custom={2}
              className="mt-4 text-lg text-muted-foreground max-w-xl mx-auto"
            >
              No login. No signup. You pay only for AI tokens.
            </motion.p>
          </motion.div>

          <motion.div
            initial="hidden"
            whileInView="visible"
            viewport={{ once: true, margin: "-50px" }}
            className="grid gap-6 lg:grid-cols-3 items-stretch"
          >
            {/* Free Tier */}
            <motion.div
              variants={fadeUp}
              custom={0}
              className="rounded-2xl border border-border/60 border-l-2 border-l-primary bg-gradient-to-b from-[#1A1714] to-[#1C1916] p-8 flex flex-col shadow-[0_0_30px_rgba(234,88,12,0.06)] transition-transform duration-300 hover:-translate-y-1"
            >
              <div className="mb-1">
                <h2 className="text-2xl font-bold">Free</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Runs locally on your machine
                </p>
              </div>

              <div className="h-px bg-border/40 my-6" />

              <div className="space-y-2 mb-6 flex-1">
                <InstallRow
                  icon={Apple}
                  title="macOS / Linux"
                  command="curl -fsSL https://raw.githubusercontent.com/rpuneet/bc/main/scripts/install.sh | bash"
                />
                <InstallRow
                  icon={Container}
                  title="Docker"
                  command="docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/bc mycel up --addr 0.0.0.0:9374"
                />
                <InstallRow
                  icon={GitBranch}
                  title="From source"
                  command="go install github.com/rpuneet/bc/cmd/mycel@latest"
                />
              </div>

              <TechTags tags={["docker", "tmux", "postgresql", "sqlite"]} />

              <div className="mt-6">
                <Link
                  href="https://github.com/rpuneet/bc"
                  className="flex items-center justify-center gap-2 w-full rounded-lg bg-primary py-2.5 text-center text-sm font-semibold text-primary-foreground transition-all hover:opacity-90 active:scale-[0.98]"
                >
                  Get Started
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                </Link>
              </div>
            </motion.div>

            {/* Cloud Tier */}
            <motion.div
              variants={fadeUp}
              custom={1}
              className="rounded-2xl border border-border/60 hover:border-blue-500/20 bg-gradient-to-b from-[#1A1714] to-[#1C1916] p-8 flex flex-col transition-all duration-300 hover:-translate-y-1"
            >
              <div className="mb-1">
                <h2 className="text-2xl font-bold">Cloud</h2>
                <div className="mt-1 flex items-baseline gap-1">
                  <span className="text-2xl font-bold tracking-tight">
                    &#8377;1,000
                  </span>
                  <span className="text-muted-foreground text-sm">
                    / month
                  </span>
                </div>
                <p className="mt-1 text-sm text-muted-foreground">
                  Access your workspace from anywhere
                </p>
              </div>

              <div className="h-px bg-border/40 my-6" />

              <ul className="space-y-3 mb-6 flex-1">
                {[
                  "SSH access to your workspace",
                  "Remote agent management",
                  "Everything in Free",
                ].map((f) => (
                  <li
                    key={f}
                    className="flex items-center gap-2.5 text-sm text-muted-foreground"
                  >
                    <span className="h-1 w-1 rounded-full bg-muted-foreground/50 shrink-0" />
                    {f}
                  </li>
                ))}
              </ul>

              <TechTags
                tags={["kubernetes", "aws", "gcp", "ssh", "mcp"]}
              />

              <div className="mt-6">
                <Link
                  href="/waitlist"
                  className="block w-full rounded-lg border border-border/60 py-2.5 text-center text-sm font-semibold transition-colors hover:bg-accent/20 active:scale-[0.98]"
                >
                  Join Waitlist
                </Link>
              </div>
            </motion.div>

            {/* Enterprise Tier */}
            <motion.div
              variants={fadeUp}
              custom={2}
              className="rounded-2xl border border-border/60 bg-gradient-to-b from-[#1A1714] to-[#1C1916] p-8 flex flex-col relative overflow-hidden transition-transform duration-300 hover:-translate-y-1"
            >
              <div className="flex flex-col flex-1">
                <div className="mb-1">
                  <h2 className="text-2xl font-bold">Enterprise</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    For teams at scale
                  </p>
                </div>

                <div className="h-px bg-border/40 my-6" />

                <ul className="space-y-3 mb-6 flex-1">
                  {[
                    "SSO / SAML authentication",
                    "Audit logs and compliance",
                    "Dedicated support",
                    "Custom SLA",
                  ].map((f) => (
                    <li
                      key={f}
                      className="flex items-center gap-2.5 text-sm text-muted-foreground"
                    >
                      <span className="h-1 w-1 rounded-full bg-muted-foreground/50 shrink-0" />
                      {f}
                    </li>
                  ))}
                </ul>

                <TechTags
                  tags={["sso", "saml", "audit", "sla"]}
                />
              </div>

              {/* Coming soon overlay */}
              <div className="absolute inset-0 z-10 bg-gradient-to-b from-transparent via-[#1A1714]/60 to-[#1A1714]/95" />
              <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3">
                <span className="px-4 py-1.5 rounded-full border border-border/80 bg-card/90 text-sm font-semibold text-muted-foreground shadow-[0_0_20px_rgba(255,255,255,0.04)]">
                  Coming soon
                </span>
                <a
                  href="mailto:hello@mycel.dev"
                  className="text-xs text-muted-foreground/60 hover:text-primary transition-colors"
                >
                  Talk to us &rarr; hello@mycel.dev
                </a>
              </div>
            </motion.div>
          </motion.div>

          {/* FAQ Section */}
          <motion.div
            initial="hidden"
            whileInView="visible"
            viewport={{ once: true, margin: "-50px" }}
            className="mt-10 max-w-2xl mx-auto"
          >
            <motion.h2
              variants={fadeUp}
              custom={0}
              className="text-2xl font-bold tracking-tight text-center mb-6"
            >
              Frequently asked questions
            </motion.h2>
            <motion.div variants={fadeUp} custom={1}>
              <FAQItem
                question="Is mycel really free?"
                answer="Yes. mycel is open source and runs locally on your machine. All features are included with no restrictions. You only pay for AI API tokens from your chosen providers (Claude, Gemini, etc.)."
              />
              <FAQItem
                question="Do I need an account?"
                answer="No for the Free tier. Install mycel, run mycel init, and start orchestrating. No signup, no login, no telemetry. The Cloud tier requires a signup for remote access features."
              />
              <FAQItem
                question="What's included in Cloud?"
                answer="SSH access to your workspace, remote agent chat, hosted dashboard, team management features, and cloud-synced memory. Everything in the Free tier is included plus remote access capabilities."
              />
              <FAQItem
                question="What AI providers work with mycel?"
                answer="mycel supports Claude Code, Cursor, Gemini, Codex, Aider, OpenCode, and OpenClaw. You bring your own API keys and mycel orchestrates agents across any combination of providers."
              />
            </motion.div>
          </motion.div>
        </div>

        <Footer />
      </div>
    </main>
  );
}
