import { query, type Options, type Query, type SDKMessage } from "@anthropic-ai/claude-agent-sdk";

import type {
  AgentState,
  MessageLogEntry,
  QueryRequest,
  RunnerEvent,
  StatusResponse,
} from "./types.js";
import type { SseHub } from "./sse.js";

// AgentRunner wraps a single Claude agent. Only one query can be in flight at
// a time — if a new /query arrives while one is already running, the caller
// gets a 409. /stop interrupts the active query so a new one can start.
//
// We don't try to support multiple concurrent sessions in a single runner:
// bcd spawns one runner process per agent, so concurrency is achieved at the
// process level, not inside the runner.

export interface AgentRunnerConfig {
  agentName: string;
  workingDir: string;
  defaultSystemPrompt?: string;
  defaultAllowedTools?: string[];
  defaultMaxBudgetUsd?: number;
  defaultMaxTurns?: number;
  // MCP server configs forwarded directly to the SDK Options.mcpServers field.
  // Shape matches @anthropic-ai/claude-agent-sdk's McpServerConfig.
  mcpServers?: Record<string, unknown>;
}

export class AgentRunner {
  private readonly cfg: AgentRunnerConfig;
  private readonly hub: SseHub;
  private readonly messages: MessageLogEntry[] = [];

  private state: AgentState = "idle";
  private currentSessionId: string | null = null;
  private currentQuery: Query | null = null;
  private currentTurn = 0;
  private tokensUsed = 0;
  private costUsd = 0;
  private startedAt: string | null = null;
  private lastActivityAt: string | null = null;
  private lastError: string | null = null;

  constructor(cfg: AgentRunnerConfig, hub: SseHub) {
    this.cfg = cfg;
    this.hub = hub;
  }

  status(): StatusResponse {
    return {
      agent_name: this.cfg.agentName,
      state: this.state,
      session_id: this.currentSessionId,
      current_turn: this.currentTurn,
      tokens_used: this.tokensUsed,
      cost_usd: this.costUsd,
      started_at: this.startedAt,
      last_activity_at: this.lastActivityAt,
      last_error: this.lastError,
      working_dir: this.cfg.workingDir,
    };
  }

  getMessages(): MessageLogEntry[] {
    return this.messages.slice();
  }

  isBusy(): boolean {
    return this.state === "working" || this.state === "waiting";
  }

  // Start a new query. Returns the seed session id (null until the SDK emits
  // its first init message — bcd should poll /status or watch /events for it).
  // The actual conversation runs asynchronously; this method does not block
  // until completion.
  async startQuery(req: QueryRequest): Promise<{ session_id: string | null }> {
    if (this.isBusy()) {
      throw new Error("agent is busy; call /stop before starting a new query");
    }

    const options = this.buildOptions(req);

    this.state = "working";
    this.currentTurn = 0;
    this.startedAt = new Date().toISOString();
    this.lastError = null;

    const q = query({ prompt: req.prompt, options });
    this.currentQuery = q;

    // Drive the async iterator on its own — we don't await completion here.
    // Errors are captured and surfaced via state + /events.
    void this.consume(q).catch((err: unknown) => {
      this.lastError = err instanceof Error ? err.message : String(err);
      this.state = "error";
      this.publish({
        ts: new Date().toISOString(),
        type: "error",
        session_id: this.currentSessionId,
        data: { message: this.lastError },
      });
    });

    return { session_id: this.currentSessionId };
  }

  // Interrupt the active query. Safe to call when idle.
  async stop(): Promise<void> {
    const q = this.currentQuery;
    if (!q) {
      this.state = "idle";
      return;
    }
    try {
      await q.interrupt();
    } catch {
      // The SDK throws if there's nothing to interrupt — ignore.
    }
    this.state = "stopped";
    this.publish({
      ts: new Date().toISOString(),
      type: "stop",
      session_id: this.currentSessionId,
      data: { reason: "stop_requested" },
    });
  }

  shutdown(): void {
    void this.stop();
    this.currentQuery = null;
  }

  // ---- internals ----

  private buildOptions(req: QueryRequest): Options {
    const systemPrompt = req.system_prompt ?? this.cfg.defaultSystemPrompt;
    const allowedTools = req.allowed_tools ?? this.cfg.defaultAllowedTools;
    const maxTurns = req.max_turns ?? this.cfg.defaultMaxTurns;
    const opts: Options = {
      cwd: this.cfg.workingDir,
      // permissionMode "bypassPermissions" matches the current
      // --dangerously-skip-permissions behavior of the CLI agents bc spawns.
      permissionMode: req.permission_mode ?? "bypassPermissions",
    };
    if (systemPrompt) opts.systemPrompt = systemPrompt;
    if (allowedTools && allowedTools.length > 0) opts.allowedTools = allowedTools;
    if (maxTurns) opts.maxTurns = maxTurns;
    if (req.resume_session) opts.resume = req.resume_session;
    if (this.cfg.mcpServers && Object.keys(this.cfg.mcpServers).length > 0) {
      // The SDK's McpServerConfig type is a discriminated union we don't want
      // to import (it can change shape between versions). The runner trusts
      // bcd to pass a valid config and forwards it as-is.
      opts.mcpServers = this.cfg.mcpServers as Options["mcpServers"];
    }
    return opts;
  }

  private async consume(q: Query): Promise<void> {
    for await (const msg of q) {
      this.handleMessage(msg);
    }
    if (this.state === "working") {
      this.state = "idle";
    }
    this.currentQuery = null;
  }

  private handleMessage(msg: SDKMessage): void {
    this.lastActivityAt = new Date().toISOString();
    this.messages.push({
      ts: this.lastActivityAt,
      type: msg.type,
      session_id: this.currentSessionId,
      raw: msg,
    });

    switch (msg.type) {
      case "system": {
        // The init system message carries the session id.
        if ("session_id" in msg && typeof msg.session_id === "string") {
          this.currentSessionId = msg.session_id;
          this.publish({
            ts: this.lastActivityAt,
            type: "session_start",
            session_id: this.currentSessionId,
            data: { subtype: "subtype" in msg ? msg.subtype : undefined },
          });
        }
        return;
      }
      case "assistant": {
        // Each assistant turn = +1 turn count. Tool use blocks live inside
        // the assistant message content; we surface them as separate events
        // so the dashboard can render activity timelines.
        this.currentTurn++;
        this.publish({
          ts: this.lastActivityAt,
          type: "assistant_message",
          session_id: this.currentSessionId,
          data: msg,
        });
        const content = (msg as { message?: { content?: unknown[] } }).message?.content;
        if (Array.isArray(content)) {
          for (const block of content) {
            if (
              block &&
              typeof block === "object" &&
              "type" in block &&
              (block as { type: string }).type === "tool_use"
            ) {
              this.publish({
                ts: this.lastActivityAt,
                type: "tool_use",
                session_id: this.currentSessionId,
                data: block,
              });
            }
          }
        }
        return;
      }
      case "user": {
        // User messages in the SDK stream carry tool_result blocks from the
        // previous assistant turn — surface them so dashboards can show what
        // each tool actually returned.
        const content = (msg as { message?: { content?: unknown[] } }).message?.content;
        if (Array.isArray(content)) {
          for (const block of content) {
            if (
              block &&
              typeof block === "object" &&
              "type" in block &&
              (block as { type: string }).type === "tool_result"
            ) {
              this.publish({
                ts: this.lastActivityAt!,
                type: "tool_result",
                session_id: this.currentSessionId,
                data: block,
              });
            }
          }
        }
        return;
      }
      case "result": {
        // Final message of a query. Update accumulated cost/tokens and emit
        // a result event so subscribers can mark the query as done.
        const r = msg as {
          subtype?: string;
          total_cost_usd?: number;
          usage?: { input_tokens?: number; output_tokens?: number };
        };
        if (typeof r.total_cost_usd === "number") {
          this.costUsd += r.total_cost_usd;
        }
        if (r.usage) {
          this.tokensUsed +=
            (r.usage.input_tokens ?? 0) + (r.usage.output_tokens ?? 0);
        }
        if (r.subtype && r.subtype !== "success") {
          this.state = "error";
          this.lastError = `query ended with subtype=${r.subtype}`;
        }
        this.publish({
          ts: this.lastActivityAt,
          type: "result",
          session_id: this.currentSessionId,
          data: msg,
        });
        return;
      }
      default:
        // stream_event, partial messages, compact boundaries, etc. are
        // captured in the message log but don't get their own typed event.
        return;
    }
  }

  private publish(event: RunnerEvent): void {
    this.hub.publish(event);
  }
}
