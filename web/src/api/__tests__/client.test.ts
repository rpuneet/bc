import { describe, it, expect, vi, beforeEach } from "vitest";
import { api } from "../client";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, status = 200, statusText = "OK") {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText,
    json: () => Promise.resolve(body),
  } as Response);
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("api.request", () => {
  it("sends Content-Type header", async () => {
    fetchMock.mockReturnValue(jsonResponse([]));
    await api.listAgents();
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe(
      "application/json",
    );
  });

  it("throws on non-ok response", async () => {
    fetchMock.mockReturnValue(jsonResponse(null, 500, "Internal Server Error"));
    await expect(api.listAgents()).rejects.toThrow(
      "API error: 500 Internal Server Error",
    );
  });

  it("extracts error message from JSON response body", async () => {
    fetchMock.mockReturnValue(
      jsonResponse({ error: 'tool "wget" already exists' }, 400, "Bad Request"),
    );
    await expect(api.listAgents()).rejects.toThrow(
      'tool "wget" already exists',
    );
  });

  it("formats URL with path", async () => {
    fetchMock.mockReturnValue(jsonResponse({}));
    await api.getAgent("test-agent");
    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toBe("/api/agents/test-agent");
  });

  it("encodes agent name in URL", async () => {
    fetchMock.mockReturnValue(jsonResponse({}));
    await api.getAgent("agent with spaces");
    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toBe("/api/agents/agent%20with%20spaces");
  });

  it("hits the flat /api/repos surface for getRepos", async () => {
    fetchMock.mockReturnValue(
      jsonResponse({ repos: [{ path: "/r/a", name: "a", agent_count: 1 }], default: "/r/a" }),
    );
    const resp = await api.getRepos();
    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toBe("/api/repos");
    expect(resp.default).toBe("/r/a");
    expect(resp.repos[0]?.name).toBe("a");
  });

  it("lists agents globally — no workspace query param", async () => {
    fetchMock.mockReturnValue(jsonResponse([]));
    await api.listAgents();
    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toBe("/api/agents");
  });

  it("passes query params for getLogs", async () => {
    fetchMock.mockReturnValue(jsonResponse([]));
    await api.getLogs(25);
    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toContain("tail=25");
  });
});

describe("shared cache wiring", () => {
  it("listAgents collapses a concurrent mount storm into one fetch", async () => {
    fetchMock.mockReturnValue(jsonResponse([{ name: "a" }]));
    // drawer + page + palette mounting together.
    await Promise.all([api.listAgents(), api.listAgents(), api.listAgents()]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("listAgents serves the cached list on a follow-up call", async () => {
    fetchMock.mockReturnValue(jsonResponse([{ name: "a" }]));
    await api.listAgents();
    await api.listAgents();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("a mutation invalidates the agents cache so the next read refetches", async () => {
    fetchMock.mockReturnValue(jsonResponse([{ name: "a" }]));
    await api.listAgents();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await api.stopAgent("a"); // mutating call -> invalidates "agents"
    await api.listAgents();
    // stopAgent fetch + the post-invalidation listAgents fetch.
    const listCalls = fetchMock.mock.calls.filter(
      ([u]) => (u as string) === "/api/agents",
    );
    expect(listCalls).toHaveLength(2);
  });
});

describe("tools seams API", () => {
  it("searchPackages POSTs manager + query to the guarded endpoint", async () => {
    fetchMock.mockReturnValue(jsonResponse({ manager: "brew", query: "ripgrep", results: [] }));
    await api.searchPackages("brew", "ripgrep");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/system/package-search");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ manager: "brew", query: "ripgrep" });
  });

  it("runProviderCommand POSTs the selected command name to the run endpoint", async () => {
    fetchMock.mockReturnValue(jsonResponse({ command: "claude --version", output: "1.0", exit_code: 0, truncated: false, timed_out: false }));
    const res = await api.runProviderCommand("claude", "version");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/providers/claude/run");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ command: "version" });
    expect(res.exit_code).toBe(0);
  });
});

/** Build a streaming Response whose body yields the given chunks in order —
 *  mirrors src/wizard/__tests__/installStream.test.ts since streamProviderUpdate
 *  parses the same NDJSON event shape over a different endpoint. */
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

describe("api.streamProviderUpdate", () => {
  it("POSTs to the provider's update endpoint and parses the NDJSON stream", async () => {
    fetchMock.mockReturnValue(
      streamResponse([
        '{"type":"start","command":"npm install -g @openai/codex"}\n',
        '{"type":"log","line":"+ codex@1.2.4"}\n',
        '{"type":"done","code":0}\n',
      ]),
    );

    const events: unknown[] = [];
    const code = await api.streamProviderUpdate("codex", (ev) => events.push(ev));

    expect(code).toBe(0);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/providers/codex/update",
      expect.objectContaining({ method: "POST" }),
    );
    expect(events).toEqual([
      { type: "start", command: "npm install -g @openai/codex" },
      { type: "log", line: "+ codex@1.2.4" },
      { type: "done", code: 0 },
    ]);
  });

  it("throws on an error event", async () => {
    fetchMock.mockReturnValue(streamResponse(['{"type":"error","error":"no automatic updater for cursor"}\n']));
    await expect(api.streamProviderUpdate("cursor", () => {})).rejects.toThrow(
      "no automatic updater for cursor",
    );
  });

  it("surfaces a non-OK response as an error", async () => {
    fetchMock.mockReturnValue({
      ok: false,
      status: 400,
      body: null,
      json: () => Promise.resolve({ error: "no automatic updater for cursor" }),
    } as unknown as Response);
    await expect(api.streamProviderUpdate("cursor", () => {})).rejects.toThrow(
      "no automatic updater for cursor",
    );
  });
});
