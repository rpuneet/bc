"use client";

import { useState, useSyncExternalStore, type ReactNode } from "react";
import Link from "next/link";
import { Copy, Check, ArrowRight, Github } from "lucide-react";
import { Nav } from "./_components/Nav";
import { Footer } from "./_components/Footer";
import { InstallSection } from "./_components/InstallSection";
import {
  TerminalWindow,
  StatusTable,
  ChannelView,
  CostTable,
} from "./_components/TerminalComponents";
import { RevealSection, FadeUp, ScrollReveal } from "./_components/Motion";
import { AnimatedBackground } from "./_components/AnimatedBackground";

/* ── Install commands by platform (hero) ── */
const installCommands = {
  macOS: "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
  Linux: "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
  Homebrew: "brew install rpuneet/mycel/mycel",
  Docker: "docker run -p 9374:9374 ghcr.io/rpuneet/mycel mycel up",
} as const;

type Platform = keyof typeof installCommands;

function detectPlatform(ua: string): Platform {
  if (/Mac/i.test(ua)) return "macOS";
  if (/Linux/i.test(ua)) return "Linux";
  return "macOS";
}

/* ── External-store hook for client-only platform detection.
 *    SSR snapshot is "macOS" to match initial render; the client
 *    swaps in the detected platform after hydration. ── */
const subscribePlatform = () => () => {};
const getPlatformSnapshot = (): Platform => detectPlatform(navigator.userAgent);
const getPlatformServerSnapshot = (): Platform => "macOS";

/* ── Copy button ── */
function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard unavailable — no-op
    }
  };

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="shrink-0 rounded p-1.5 text-on-surface-variant hover:text-on-surface transition-colors"
      aria-label="Copy to clipboard"
    >
      {copied ? <Check className="h-4 w-4 text-terminal-success" /> : <Copy className="h-4 w-4" />}
    </button>
  );
}

/* ── A compact code line block for panel artifacts ── */
function CmdLine({
  cmd,
  out,
}: {
  cmd?: string;
  out?: { text: string; tone?: "muted" | "ok" | "flare" | "text" };
}) {
  const toneClass = {
    muted: "text-terminal-muted",
    ok: "text-terminal-success",
    flare: "text-terminal-command",
    text: "text-terminal-text",
  } as const;
  if (cmd) {
    return (
      <div>
        <span className="text-terminal-prompt">$ </span>
        <span className="text-terminal-text">{cmd}</span>
      </div>
    );
  }
  return (
    <div className={toneClass[out?.tone ?? "muted"]}>{out?.text}</div>
  );
}

/* ── One deck panel: numbered principle → thesis → live artifact ── */
function DeckPanel({
  index,
  eyebrow,
  title,
  body,
  artifact,
  imageFirst = false,
  last = false,
}: {
  index: string;
  eyebrow: string;
  title: ReactNode;
  body: ReactNode;
  artifact: ReactNode;
  imageFirst?: boolean;
  last?: boolean;
}) {
  const text = (
    <ScrollReveal
      from={imageFirst ? "right" : "left"}
      distance={40}
      className={imageFirst ? "lg:order-2" : ""}
    >
      <div className="flex items-baseline gap-4">
        <span className="deck-index text-5xl sm:text-6xl">{index}</span>
        <span className="deck-eyebrow pb-1">{eyebrow}</span>
      </div>
      <h3 className="mt-6 font-headline text-3xl font-semibold leading-[1.12] tracking-tight text-on-background sm:text-4xl lg:text-[2.75rem]">
        {title}
      </h3>
      <p className="mt-5 max-w-xl font-body text-[15px] leading-[1.8] text-on-surface-variant">
        {body}
      </p>
    </ScrollReveal>
  );

  const art = (
    <ScrollReveal
      from="up"
      distance={32}
      delay={0.12}
      className={imageFirst ? "lg:order-1" : ""}
    >
      <div className="deck-artifact rounded-xl">{artifact}</div>
    </ScrollReveal>
  );

  return (
    <div className="relative">
      {/* Ember-rail node marker (desktop) */}
      <span
        aria-hidden="true"
        className="ember-node absolute left-1/2 top-16 hidden h-3 w-3 -translate-x-1/2 rounded-full bg-primary lg:block"
      />
      <div className="grid items-center gap-10 py-16 sm:py-20 lg:grid-cols-2 lg:gap-20">
        {imageFirst ? (
          <>
            {art}
            {text}
          </>
        ) : (
          <>
            {text}
            {art}
          </>
        )}
      </div>
      {!last && (
        <div
          aria-hidden="true"
          className="ember-rail absolute left-1/2 bottom-0 hidden h-16 w-px -translate-x-1/2 lg:block"
        />
      )}
    </div>
  );
}

/* ── Artifact: provider / model catalog fetched from source ── */
function ProviderArtifact() {
  return (
    <TerminalWindow title="mycel model list" ariaLabel="Terminal listing providers and models fetched live from each CLI">
      <div className="space-y-1.5 text-[12.5px] leading-6">
        <CmdLine cmd="mycel model list" />
        <div className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
          <span className="text-terminal-command">claude</span>
          <span className="text-terminal-muted">claude-opus · claude-sonnet · claude-haiku</span>
          <span className="text-terminal-command">pi → bedrock</span>
          <span className="text-terminal-muted">kimi-k2 · deepseek-v3 · qwen3-coder</span>
          <span className="text-terminal-command">agy</span>
          <span className="text-terminal-muted">gemini-pro · gemini-flash</span>
          <span className="text-terminal-command">codex</span>
          <span className="text-terminal-muted">gpt-5-codex</span>
        </div>
        <div className="mt-2 text-terminal-comment">
          fetched live from each tool&rsquo;s CLI &middot; no hardcoded catalog
        </div>
      </div>
    </TerminalWindow>
  );
}

/* ── Artifact: marketplace install-by-dispatch ── */
function MarketplaceArtifact() {
  return (
    <TerminalWindow title="mycel skill install" ariaLabel="Terminal installing a skill from a registry by dispatching an instruction to an agent">
      <div className="space-y-1.5 text-[12.5px] leading-6">
        <CmdLine cmd="mycel skill search postgres" />
        <CmdLine out={{ text: "mcp-registry   postgres-mcp        official", tone: "muted" }} />
        <CmdLine out={{ text: "smithery       supabase            verified", tone: "muted" }} />
        <CmdLine out={{ text: "glama          neon-serverless     community", tone: "muted" }} />
        <div className="mt-2">
          <CmdLine cmd='mycel skill install postgres-mcp --agent db-eng' />
        </div>
        <CmdLine out={{ text: "→ dispatched to db-eng · installing", tone: "flare" }} />
        <CmdLine out={{ text: "✓ postgres-mcp available to db-eng", tone: "ok" }} />
      </div>
    </TerminalWindow>
  );
}

/* ── Artifact: secrets → env injection ── */
function SecretsArtifact() {
  return (
    <TerminalWindow title="mycel secret" ariaLabel="Terminal storing a secret and injecting it into an agent's environment">
      <div className="space-y-1.5 text-[12.5px] leading-6">
        <CmdLine cmd="mycel secret set STRIPE_API_KEY" />
        <CmdLine out={{ text: "✓ stored · encrypted at rest", tone: "ok" }} />
        <div className="mt-2">
          <CmdLine cmd="mycel connect github" />
        </div>
        <CmdLine out={{ text: "✓ connected · available to every agent", tone: "ok" }} />
        <div className="mt-2">
          <CmdLine cmd="mycel agent create pay-eng --tool claude" />
        </div>
        <CmdLine out={{ text: "env → STRIPE_API_KEY, GITHUB_TOKEN injected", tone: "flare" }} />
        <CmdLine out={{ text: "agent pay-eng is online", tone: "muted" }} />
      </div>
    </TerminalWindow>
  );
}

export default function Home() {
  const detected = useSyncExternalStore(
    subscribePlatform,
    getPlatformSnapshot,
    getPlatformServerSnapshot,
  );
  const [override, setPlatform] = useState<Platform | null>(null);
  const platform = override ?? detected;

  const tabs: Platform[] = ["macOS", "Linux", "Homebrew", "Docker"];

  return (
    <main className="min-h-screen overflow-x-hidden">
      {/* Drifting spore field — fixed, covers the page */}
      <AnimatedBackground />
      {/* Warm radial wash above the fold */}
      <div className="pointer-events-none fixed inset-0 z-[1] bg-[radial-gradient(ellipse_80%_60%_at_50%_-20%,color-mix(in_srgb,var(--primary)_10%,transparent),transparent)]" />

      <div className="relative z-[2]">
        <Nav />

        {/* ════════════════════════════════════════
           Hero — the thesis
           ════════════════════════════════════════ */}
        <section className="pt-28 pb-14 sm:pt-36 sm:pb-20">
          <div className="mx-auto max-w-4xl px-4 text-center sm:px-6">
            <FadeUp>
              <span className="deck-eyebrow">
                CLI-first &middot; Any agent &middot; Open source
              </span>
            </FadeUp>

            <FadeUp delay={0.1}>
              <h1 className="mt-6 font-headline text-4xl font-semibold leading-[1.08] tracking-tight text-on-background md:text-6xl lg:text-[4.25rem]">
                Run a team of AI agents
                <br className="hidden sm:block" />{" "}
                like you run a{" "}
                <span className="text-primary">codebase.</span>
              </h1>
            </FadeUp>

            <FadeUp delay={0.15}>
              <p className="mx-auto mt-6 max-w-2xl font-body text-lg leading-relaxed text-on-surface-variant md:text-xl">
                mycel orchestrates Claude Code, pi, Cursor, Gemini, and Codex
                agents in parallel &mdash; each in its own git worktree and
                runtime. One binary. One command. Your terminal stays the
                control plane.
              </p>
            </FadeUp>

            {/* Hero install */}
            <FadeUp delay={0.2}>
              <div className="mx-auto mt-10 max-w-xl">
                <div className="mb-3 flex items-center justify-center gap-1">
                  {tabs.map((t) => (
                    <button
                      key={t}
                      type="button"
                      onClick={() => setPlatform(t)}
                      className={`rounded px-3 py-1.5 font-label text-xs font-medium transition-colors ${
                        platform === t
                          ? "bg-surface-container-high text-on-surface"
                          : "text-on-surface-variant hover:text-on-surface"
                      }`}
                    >
                      {t}
                    </button>
                  ))}
                </div>

                <div className="flex items-center gap-2 rounded-lg border border-outline-variant/30 bg-surface-container px-4 py-3 shadow-[0_0_60px_color-mix(in_srgb,var(--primary)_8%,transparent),0_0_20px_color-mix(in_srgb,var(--primary)_4%,transparent)]">
                  <span className="select-none font-label text-on-surface-variant">$</span>
                  <code className="scrollbar-none block min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-label text-sm text-on-surface">
                    {installCommands[platform]}
                  </code>
                  <CopyButton text={installCommands[platform]} />
                </div>

                <p className="mt-3 font-body text-sm text-on-surface-variant">
                  Then run:{" "}
                  <code className="font-label text-primary">mycel up</code>
                </p>
              </div>
            </FadeUp>

            {/* CTAs */}
            <FadeUp delay={0.25}>
              <div className="mt-8 flex flex-col items-center gap-5">
                <div className="flex items-center gap-4">
                  <Link
                    href="/docs"
                    className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-lg active:scale-[0.97]"
                  >
                    Read the docs
                    <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
                  </Link>
                  <Link
                    href="https://github.com/rpuneet/mycel"
                    className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-6 font-body text-sm font-medium text-on-surface-variant transition-colors hover:border-primary/30 hover:bg-surface-container hover:text-primary active:scale-[0.97]"
                  >
                    <Github className="h-4 w-4" aria-hidden="true" />
                    GitHub
                  </Link>
                </div>
                <div className="flex flex-wrap items-center justify-center gap-2 opacity-70 transition-opacity hover:opacity-100">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/stars/rpuneet/mycel?style=flat-square&color=a35d0a&labelColor=2a2118" alt="GitHub stars" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/license/rpuneet/mycel?style=flat-square&color=a35d0a&labelColor=2a2118" alt="License" className="h-5" />
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src="https://img.shields.io/github/last-commit/rpuneet/mycel?style=flat-square&color=a35d0a&labelColor=2a2118" alt="Last commit" className="h-5" />
                </div>
              </div>
            </FadeUp>
          </div>
        </section>

        {/* ════════════════════════════════════════
           Live terminal — proof it's real
           ════════════════════════════════════════ */}
        <RevealSection className="pb-8 sm:pb-12">
          <div className="mx-auto max-w-3xl px-4 sm:px-6">
            <TerminalWindow title="terminal" className="terminal-glow">
              <div className="space-y-3 text-[13px] leading-7">
                <div>
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel up</span>
                </div>
                <div className="text-terminal-success">&#10003; Ready &middot; console at <span className="text-primary">http://localhost:9374</span></div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel agent create eng-01 --role engineer --tool claude</span>
                </div>
                <div className="text-terminal-muted">Worktree checked out at .mycel/agents/eng-01</div>
                <div className="text-terminal-muted">
                  Agent <span className="text-primary">eng-01</span> is online.
                </div>

                <div className="mt-2">
                  <span className="text-terminal-prompt">~ $ </span>
                  <span className="text-terminal-text">mycel status</span>
                </div>
                <div className="mt-1 font-label text-terminal-comment">
                  <div className="text-primary/70">AGENT     ROLE       STATE     UPTIME</div>
                  <div className="text-terminal-text">eng-01    engineer   <span className="text-terminal-success">working</span>   2m</div>
                  <div className="text-terminal-text">eng-02    engineer   <span className="text-terminal-success">working</span>   1m</div>
                </div>
                <div className="inline-block h-[18px] w-2 animate-pulse bg-primary/80" />
              </div>
            </TerminalWindow>
          </div>
        </RevealSection>

        {/* Section separator */}
        <div className="mx-auto max-w-5xl px-6"><div className="section-separator" /></div>

        {/* ════════════════════════════════════════
           The deck — one capability per panel
           ════════════════════════════════════════ */}
        <section id="product" className="deck-veil scroll-mt-24 py-14 sm:py-20">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <FadeUp className="mb-6 text-center">
              <span className="deck-eyebrow">What it does</span>
              <h2 className="mx-auto mt-4 max-w-3xl font-headline text-3xl font-semibold tracking-tight text-on-background md:text-5xl">
                A control plane for AI agents,
                <br className="hidden sm:block" />{" "}
                built for people who live in the terminal.
              </h2>
              <p className="mx-auto mt-5 max-w-2xl font-body text-lg text-on-surface-variant">
                Six capabilities, one binary. Everything below runs on your
                machine today.
              </p>
            </FadeUp>

            <div className="relative">
              {/* 01 — Multi-agent orchestration */}
              <DeckPanel
                index="01"
                eyebrow="Orchestration"
                title="Ten agents, one codebase, zero collisions."
                body={
                  <>
                    Spawn as many agents as the work needs. Each gets its own
                    git worktree and its own tmux or Docker runtime, so they
                    build in parallel without stepping on each other. Watch
                    every one&rsquo;s state, task, and output from a single
                    view.
                  </>
                }
                artifact={
                  <TerminalWindow title="mycel status" ariaLabel="Live agent roster showing parallel agents and their states">
                    <StatusTable
                      agents={[
                        { name: "api-eng", role: "engineer", state: "working", detail: "Wiring the billing webhook" },
                        { name: "web-eng", role: "engineer", state: "working", detail: "Refactoring the dashboard" },
                        { name: "qa-01", role: "qa", state: "tool", detail: "Running the e2e suite" },
                        { name: "reviewer", role: "manager", state: "idle", detail: "Waiting on PR #214" },
                        { name: "db-eng", role: "engineer", state: "done", detail: "Schema change merged" },
                      ]}
                    />
                  </TerminalWindow>
                }
              />

              {/* 02 — Any model, from source */}
              <DeckPanel
                index="02"
                eyebrow="Providers"
                title="Any model, pulled straight from the tool."
                body={
                  <>
                    mycel reads providers and models live from each CLI it
                    drives &mdash; Claude Code, pi reaching AWS Bedrock for
                    Kimi, DeepSeek and Qwen, Gemini, Codex. Nothing is
                    hardcoded, so the catalog is whatever your tools expose
                    right now. Mix models across agents on the same project.
                  </>
                }
                artifact={<ProviderArtifact />}
                imageFirst
              />

              {/* 03 — Skills & MCP marketplace */}
              <DeckPanel
                index="03"
                eyebrow="Marketplace"
                title="Install skills and MCP servers by name."
                body={
                  <>
                    Search the official MCP registry, Glama, and Smithery
                    alongside vendor skill repos from Anthropic, Google, and
                    openclaw &mdash; or your own templates. Pick one and mycel
                    dispatches the install straight to the agent that needs it.
                  </>
                }
                artifact={<MarketplaceArtifact />}
              />

              {/* 04 — Secrets → env */}
              <DeckPanel
                index="04"
                eyebrow="Secrets"
                title="Store a key once. Every agent gets it."
                body={
                  <>
                    Keys live in an encrypted vault and land in each
                    agent&rsquo;s environment as variables the moment it
                    spawns. Connect an app once &mdash; GitHub, Stripe, your
                    own API &mdash; and it&rsquo;s wired everywhere, no copying
                    tokens between sessions.
                  </>
                }
                artifact={<SecretsArtifact />}
                imageFirst
              />

              {/* 05 — Notifications across channels */}
              <DeckPanel
                index="05"
                eyebrow="Channels"
                title="Your agents reach you where you already are."
                body={
                  <>
                    Bridge WhatsApp, Slack, Telegram, and Discord. Agents post
                    updates, hand work to each other with @mentions, and answer
                    when you reply &mdash; from your phone, in the thread you
                    were already in.
                  </>
                }
                artifact={
                  <TerminalWindow title="#engineering" ariaLabel="A channel view showing agents coordinating with mentions across bridged apps">
                    <ChannelView
                      name="engineering"
                      members={5}
                      messages={[
                        { time: "09:14", agent: "api-eng", role: "engineer", message: "Billing webhook is green. Opening PR #214." },
                        { time: "09:15", agent: "reviewer", role: "manager", message: "@qa-01 run the payment path before I merge." },
                        { time: "09:16", agent: "qa-01", role: "qa", message: "On it — e2e suite running now." },
                        { time: "09:19", agent: "you", role: "you", message: "Ship it once QA is green. (via Slack)" },
                      ]}
                    />
                  </TerminalWindow>
                }
              />

              {/* 06 — Cost visibility */}
              <DeckPanel
                index="06"
                eyebrow="Cost"
                title="See the bill before it surprises you."
                body={
                  <>
                    Every token is tracked per agent, per model, and per day.
                    Read live spend and set budgets with hard stops &mdash; an
                    agent pauses itself the moment it hits the limit you gave
                    it.
                  </>
                }
                artifact={
                  <TerminalWindow title="mycel cost show" ariaLabel="A cost table breaking down spend and budget per agent">
                    <CostTable
                      rows={[
                        { agent: "api-eng", tokensIn: "1.2M", tokensOut: "312K", cost: "$4.18", budget: "$8.00", percent: 52 },
                        { agent: "web-eng", tokensIn: "880K", tokensOut: "205K", cost: "$2.94", budget: "$8.00", percent: 37 },
                        { agent: "qa-01", tokensIn: "410K", tokensOut: "96K", cost: "$1.31", budget: "$4.00", percent: 33 },
                        { agent: "db-eng", tokensIn: "1.6M", tokensOut: "298K", cost: "$5.02", budget: "$6.00", percent: 84 },
                      ]}
                      total={{ cost: "$13.45", budget: "$26.00" }}
                    />
                  </TerminalWindow>
                }
                imageFirst
                last
              />
            </div>

            {/* Connective serif accent */}
            <FadeUp className="mx-auto mt-8 max-w-2xl text-center">
              <p className="deck-serif text-2xl leading-snug text-on-surface-variant sm:text-3xl">
                One control plane. Every model, secret, and channel your team
                already uses &mdash;{" "}
                <span className="text-primary">wired in once.</span>
              </p>
            </FadeUp>
          </div>
        </section>

        {/* Section separator */}
        <div className="mx-auto max-w-5xl px-6"><div className="section-separator" /></div>

        {/* ════════════════════════════════════════
           Install
           ════════════════════════════════════════ */}
        <InstallSection />

        {/* ════════════════════════════════════════
           Open-source CTA
           ════════════════════════════════════════ */}
        <RevealSection className="py-14 sm:py-20">
          <div className="mx-auto max-w-3xl px-4 text-center sm:px-6">
            <h2 className="font-headline text-2xl font-semibold tracking-tight text-on-background md:text-4xl">
              Free, open source, and yours to run.
            </h2>
            <p className="mt-3 font-body text-lg text-on-surface-variant">
              MIT licensed. No cloud account. It runs on your machine.
            </p>
            <div className="mt-8 flex flex-col items-center justify-center gap-4 sm:flex-row">
              <Link
                href="https://github.com/rpuneet/mycel"
                className="inline-flex h-11 items-center gap-2 rounded-lg bg-primary px-8 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-[0_0_20px_color-mix(in_srgb,var(--primary)_30%,transparent)] active:scale-[0.97]"
              >
                <Github className="h-5 w-5" aria-hidden="true" />
                View on GitHub
              </Link>
              <Link
                href="/docs"
                className="inline-flex h-11 items-center gap-2 rounded-lg border border-outline-variant/20 px-8 font-body text-sm font-medium text-on-surface-variant transition-colors hover:bg-surface-container hover:text-on-surface active:scale-[0.97]"
              >
                Browse the CLI reference
                <ArrowRight className="h-4 w-4" aria-hidden="true" />
              </Link>
            </div>
          </div>
        </RevealSection>

        {/* Footer gradient separator */}
        <div className="mx-auto max-w-6xl px-6"><div className="footer-separator" /></div>

        <Footer />
      </div>
    </main>
  );
}
