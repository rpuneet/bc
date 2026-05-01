// bc-agent-runner entry point.
//
// One process per agent. Reads its identity, working directory, and policy
// from environment variables (set by bcd when it spawns the runner) and
// exposes the agent over an HTTP API on BC_AGENT_RUNNER_PORT (default 8080).

import { createRequire } from "node:module";

import { AgentRunner } from "./agent.js";
import { SseHub } from "./sse.js";
import { buildServer } from "./server.js";

function requireEnv(name: string): string {
  const v = process.env[name];
  if (!v || v.length === 0) {
    throw new Error(`${name} is required`);
  }
  return v;
}

function optionalEnv(name: string): string | undefined {
  const v = process.env[name];
  return v && v.length > 0 ? v : undefined;
}

function parseJsonEnv<T>(name: string, fallback: T): T {
  const raw = optionalEnv(name);
  if (!raw) return fallback;
  try {
    return JSON.parse(raw) as T;
  } catch (err) {
    throw new Error(
      `${name} must be valid JSON: ${err instanceof Error ? err.message : String(err)}`,
    );
  }
}

function parseFloatEnv(name: string): number | undefined {
  const raw = optionalEnv(name);
  if (!raw) return undefined;
  const n = Number(raw);
  if (!Number.isFinite(n)) {
    throw new Error(`${name} must be a number, got: ${raw}`);
  }
  return n;
}

function parseIntEnv(name: string): number | undefined {
  const raw = optionalEnv(name);
  if (!raw) return undefined;
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n)) {
    throw new Error(`${name} must be an integer, got: ${raw}`);
  }
  return n;
}

function readSdkVersion(): string {
  // The SDK exposes its version through its package.json. We read it via
  // createRequire so this works under both ESM and bundlers.
  try {
    const req = createRequire(import.meta.url);
    const pkg = req("@anthropic-ai/claude-agent-sdk/package.json") as {
      version?: string;
    };
    return pkg.version ?? "unknown";
  } catch {
    return "unknown";
  }
}

async function main(): Promise<void> {
  const agentName = requireEnv("BC_AGENT_NAME");
  const workingDir = optionalEnv("BC_AGENT_WORKING_DIR") ?? process.cwd();
  const port = parseIntEnv("BC_AGENT_RUNNER_PORT") ?? 8080;
  const host = optionalEnv("BC_AGENT_RUNNER_HOST") ?? "0.0.0.0";

  const hub = new SseHub();
  const runner = new AgentRunner(
    {
      agentName,
      workingDir,
      defaultSystemPrompt: optionalEnv("BC_ROLE_PROMPT"),
      defaultAllowedTools: parseJsonEnv<string[] | undefined>(
        "BC_ALLOWED_TOOLS",
        undefined,
      ),
      defaultMaxTurns: parseIntEnv("BC_MAX_TURNS"),
      defaultMaxBudgetUsd: parseFloatEnv("BC_MAX_BUDGET_USD"),
      mcpServers: parseJsonEnv<Record<string, unknown> | undefined>(
        "BC_MCP_SERVERS",
        undefined,
      ),
    },
    hub,
  );

  const app = buildServer(runner, hub, {
    agentName,
    sdkVersion: readSdkVersion(),
    startedAt: new Date(),
  });

  const server = app.listen(port, host, () => {
    // eslint-disable-next-line no-console
    console.log(
      `[bc-agent-runner] agent=${agentName} listening on http://${host}:${port}`,
    );
  });

  const shutdown = (signal: NodeJS.Signals) => {
    // eslint-disable-next-line no-console
    console.log(`[bc-agent-runner] received ${signal}, shutting down`);
    runner.shutdown();
    hub.closeAll();
    server.close(() => process.exit(0));
    // Force-exit if graceful shutdown stalls.
    setTimeout(() => process.exit(1), 5_000).unref();
  };
  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

main().catch((err: unknown) => {
  // eslint-disable-next-line no-console
  console.error(
    `[bc-agent-runner] fatal: ${err instanceof Error ? err.stack ?? err.message : String(err)}`,
  );
  process.exit(1);
});
