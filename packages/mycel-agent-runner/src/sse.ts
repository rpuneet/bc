import type { Response } from "express";
import type { RunnerEvent } from "./types.js";

// In-memory pub/sub for SSE event streaming. There is exactly one runner per
// process, so a single global hub is fine — no need for routing or topics.
//
// Subscribers are tracked by their Response object so we can write to each
// open connection and prune when they disconnect. There is no replay buffer:
// subscribers only see events that happen after they connect. Use /messages
// for the historical conversation log.

export class SseHub {
  private readonly subscribers = new Set<Response>();

  subscribe(res: Response): void {
    res.setHeader("Content-Type", "text/event-stream");
    res.setHeader("Cache-Control", "no-cache, no-transform");
    res.setHeader("Connection", "keep-alive");
    res.setHeader("X-Accel-Buffering", "no");
    res.flushHeaders?.();

    // Initial comment so clients know the stream is live.
    res.write(": connected\n\n");

    this.subscribers.add(res);
    res.on("close", () => {
      this.subscribers.delete(res);
    });
  }

  publish(event: RunnerEvent): void {
    const payload = `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`;
    for (const res of this.subscribers) {
      try {
        res.write(payload);
      } catch {
        // Best-effort: drop subscribers we can't write to. The 'close'
        // listener will clean up the actual entry.
        this.subscribers.delete(res);
      }
    }
  }

  closeAll(): void {
    for (const res of this.subscribers) {
      try {
        res.end();
      } catch {
        // ignore
      }
    }
    this.subscribers.clear();
  }

  size(): number {
    return this.subscribers.size;
  }
}
