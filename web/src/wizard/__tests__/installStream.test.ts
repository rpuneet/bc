import { beforeEach, describe, expect, it, vi } from "vitest";
import { installDep, type InstallEvent } from "../installStream";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

/** Build a streaming Response whose body yields the given chunks in order. */
function streamResponse(chunks: string[], ok = true, status = 200): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return { ok, status, body } as unknown as Response;
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("installDep", () => {
  it("parses NDJSON events split across chunk boundaries and returns the exit code", async () => {
    // The 'log' record is deliberately split mid-line to exercise buffering.
    fetchMock.mockResolvedValue(
      streamResponse([
        '{"type":"start","command":"brew install git"}\n{"type":"log","li',
        'ne":"downloading"}\n',
        '{"type":"done","code":0}\n',
      ]),
    );

    const events: InstallEvent[] = [];
    const code = await installDep("git", (ev) => events.push(ev));

    expect(code).toBe(0);
    expect(events).toEqual([
      { type: "start", command: "brew install git" },
      { type: "log", line: "downloading" },
      { type: "done", code: 0 },
    ]);
    expect(fetchMock).toHaveBeenCalledWith("/api/deps/install", expect.objectContaining({ method: "POST" }));
  });

  it("throws on an error event", async () => {
    fetchMock.mockResolvedValue(streamResponse(['{"type":"error","error":"boom"}\n']));
    await expect(installDep("git", () => {})).rejects.toThrow("boom");
  });

  it("surfaces a non-OK response as an error", async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      status: 400,
      body: null,
      json: () => Promise.resolve({ error: "no automatic installer for docker" }),
    } as unknown as Response);
    await expect(installDep("docker", () => {})).rejects.toThrow("no automatic installer for docker");
  });
});
