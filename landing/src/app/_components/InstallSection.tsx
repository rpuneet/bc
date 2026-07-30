"use client";

import { useState, useEffect } from "react";
import { motion } from "framer-motion";
import { Copy, Check, Terminal, Beer, Code2, Container, Package, GitBranch } from "lucide-react";

function useLatestVersion() {
  const [version, setVersion] = useState("latest");
  useEffect(() => {
    fetch("https://api.github.com/repos/rpuneet/mycel/releases/latest")
      .then((r) => r.json())
      .then((data) => {
        if (data.tag_name) setVersion(data.tag_name);
      })
      .catch(() => {});
  }, []);
  return version;
}

type InstallOption = {
  id: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  platforms: string;
  commands: string[];
};

function getInstallOptions(version: string): InstallOption[] {
  return [
    {
      id: "curl",
      label: "curl",
      icon: Terminal,
      platforms: "macOS · Linux",
      commands: [
        "curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash",
      ],
    },
    {
      id: "brew",
      label: "Homebrew",
      icon: Beer,
      platforms: "macOS",
      commands: [
        "brew install rpuneet/mycel/mycel",
      ],
    },
    {
      id: "npm",
      label: "npm / bun",
      icon: Package,
      platforms: "Linux · macOS",
      commands: [
        "npm install -g mycel-cli",
        "# or",
        "bunx mycel-cli",
      ],
    },
    {
      id: "go",
      label: "Go",
      icon: Code2,
      platforms: "All platforms · requires Go 1.25+",
      commands: [
        `go install github.com/rpuneet/mycel/cmd/mycel@latest`,
      ],
    },
    {
      id: "docker",
      label: "Docker",
      icon: Container,
      platforms: "Stable + main branch",
      commands: [
        `docker pull ghcr.io/rpuneet/mycel:${version}`,
        `docker run -p 9374:9374 -v $(pwd):/workspace ghcr.io/rpuneet/mycel:${version} mycel up --addr 0.0.0.0:9374`,
        `# Bleeding edge from main:`,
        `docker pull ghcr.io/rpuneet/mycel:main`,
      ],
    },
    {
      id: "source",
      label: "From source",
      icon: GitBranch,
      platforms: "Requires Go 1.25+, Bun, tmux",
      commands: [
        "git clone https://github.com/rpuneet/mycel",
        "cd mycel",
        "make install-local-mycel",
      ],
    },
  ];
}

function CodeBlock({ lines, id }: { lines: string[]; id: string }) {
  const [copied, setCopied] = useState(false);
  const copyText = lines.filter((l) => !l.startsWith("#")).join("\n");

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(copyText);
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    } catch {
      // clipboard api unavailable — silent no-op
    }
  };

  return (
    <div className="group terminal-glow relative rounded-lg border border-[var(--terminal-header-border)] bg-terminal-bg p-4 pr-14 font-mono text-sm">
      <button
        type="button"
        onClick={handleCopy}
        aria-label="Copy command"
        className="absolute right-3 top-3 rounded-md border border-[var(--terminal-header-border)] bg-terminal-header p-2 text-terminal-muted transition-colors hover:border-terminal-prompt hover:text-terminal-prompt"
      >
        {copied ? (
          <Check className="h-4 w-4 text-terminal-success" />
        ) : (
          <Copy className="h-4 w-4" />
        )}
      </button>
      <pre className="overflow-x-auto text-[13px] leading-relaxed text-terminal-text">
        {lines.map((line, i) => (
          <div
            key={`${id}-${i}`}
            className={`w-max min-w-full whitespace-pre ${line.startsWith("#") ? "text-terminal-comment" : ""}`}
          >
            {line.startsWith("#") ? line : <><span className="text-terminal-prompt">$</span>{" "}{line}</>}
          </div>
        ))}
      </pre>
    </div>
  );
}

export function InstallSection() {
  const version = useLatestVersion();
  const options = getInstallOptions(version);
  const [active, setActive] = useState("curl");
  const current = options.find((m) => m.id === active) ?? options[0];

  return (
    <section
      id="install"
      className="scroll-mt-24 py-16 sm:py-24 lg:py-32"
    >
      <div className="mx-auto max-w-5xl px-4 sm:px-6">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, margin: "-100px" }}
          transition={{ duration: 0.6 }}
        >
          <span className="deck-eyebrow">
            Install
          </span>
          <div className="mt-3 flex flex-col items-start gap-3 sm:flex-row sm:items-end sm:justify-between">
            <h2 className="text-3xl font-semibold tracking-tight sm:text-5xl">
              Install in 30 seconds.
            </h2>
            <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card/80 px-3 py-1 font-mono text-xs text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-success" />
              Latest: {version}
            </span>
          </div>
          <p className="mt-4 max-w-2xl text-muted-foreground">
            One binary. No login. No config. Pick your platform and copy the command.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, margin: "-100px" }}
          transition={{ duration: 0.6, delay: 0.1 }}
          className="mt-8"
        >
          {/* Mobile select */}
          <label htmlFor="install-option-select" className="sr-only">
            Install option
          </label>
          <select
            id="install-option-select"
            value={active}
            onChange={(e) => setActive(e.target.value)}
            className="block w-full rounded-lg border border-border bg-card px-4 py-3 font-mono text-sm text-foreground sm:hidden"
          >
            {options.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label}
              </option>
            ))}
          </select>

          {/* Desktop tabs */}
          <div className="hidden flex-wrap gap-2 sm:flex">
            {options.map((m) => {
              const Icon = m.icon;
              const isActive = m.id === active;
              return (
                <button
                  key={m.id}
                  type="button"
                  onClick={() => setActive(m.id)}
                  className={`inline-flex items-center gap-2 rounded-lg border px-4 py-2 font-mono text-sm transition-colors ${
                    isActive
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border bg-card/60 text-muted-foreground hover:border-primary/50 hover:text-foreground"
                  }`}
                >
                  <Icon className="h-4 w-4" />
                  {m.label}
                </button>
              );
            })}
          </div>

          <div className="mt-4 flex items-center gap-2 font-mono text-xs text-muted-foreground">
            <span>{current.platforms}</span>
          </div>

          <div className="mt-3">
            <CodeBlock lines={current.commands} id={current.id} />
          </div>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, margin: "-100px" }}
          transition={{ duration: 0.6, delay: 0.2 }}
          className="mt-8"
        >
          <div className="font-mono text-xs uppercase tracking-[0.15em] text-muted-foreground">
            Then run
          </div>
          <div className="mt-3">
            <CodeBlock
              id="after"
              lines={[
                "mycel up            # Start server + web UI on localhost:9374",
                "mycel agent create  # Spawn an AI agent",
              ]}
            />
          </div>
        </motion.div>
      </div>
    </section>
  );
}
