import express, { type Express, type Request, type Response } from "express";

import { AgentRunner } from "./agent.js";
import { SseHub } from "./sse.js";
import type {
  HealthResponse,
  QueryRequest,
  QueryResponse,
  StopResponse,
} from "./types.js";

export interface ServerConfig {
  agentName: string;
  sdkVersion: string;
  startedAt: Date;
}

export function buildServer(
  runner: AgentRunner,
  hub: SseHub,
  cfg: ServerConfig,
): Express {
  const app = express();
  app.use(express.json({ limit: "8mb" }));

  app.get("/health", (_req: Request, res: Response) => {
    const body: HealthResponse = {
      ok: true,
      agent_name: cfg.agentName,
      uptime_seconds: Math.floor((Date.now() - cfg.startedAt.getTime()) / 1000),
      sdk_version: cfg.sdkVersion,
    };
    res.json(body);
  });

  app.get("/status", (_req: Request, res: Response) => {
    res.json(runner.status());
  });

  app.post("/query", async (req: Request, res: Response) => {
    const body = req.body as Partial<QueryRequest>;
    if (!body || typeof body.prompt !== "string" || body.prompt.length === 0) {
      res.status(400).json({ error: "prompt is required" });
      return;
    }
    try {
      const { session_id } = await runner.startQuery(body as QueryRequest);
      const response: QueryResponse = {
        session_id,
        state: runner.status().state,
        started_at: runner.status().started_at ?? new Date().toISOString(),
      };
      res.status(202).json(response);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      // Busy is the only expected error path here.
      const status = message.includes("busy") ? 409 : 500;
      res.status(status).json({ error: message });
    }
  });

  app.post("/stop", async (_req: Request, res: Response) => {
    await runner.stop();
    const body: StopResponse = {
      state: runner.status().state,
      session_id: runner.status().session_id,
    };
    res.json(body);
  });

  app.get("/messages", (_req: Request, res: Response) => {
    res.json({ messages: runner.getMessages() });
  });

  app.get("/events", (_req: Request, res: Response) => {
    hub.subscribe(res);
    // Do not call res.end() — the SseHub manages the connection lifecycle.
  });

  app.use((_req: Request, res: Response) => {
    res.status(404).json({ error: "not found" });
  });

  return app;
}
