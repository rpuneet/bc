/**
 * WorkspaceContext.test.tsx — unit tests for the workspace provider +
 * ActiveWorkspaceGuard.
 *
 * Covers:
 *   - fetchWorkspaces handles both response shapes:
 *       { workspaces: [...], active: "id" }  (object wrapper)
 *       [ ... ]                               (bare array)
 *   - refresh() triggers a re-fetch
 *   - ActiveWorkspaceGuard activates a valid :wsId + renders children
 *   - ActiveWorkspaceGuard redirects to /w when wsId is unknown
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import {
  WorkspaceProvider,
  ActiveWorkspaceGuard,
  useWorkspace,
} from "./WorkspaceContext";
import type { WorkspaceSummary } from "./WorkspaceContext";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const sampleWorkspaces: WorkspaceSummary[] = [
  { id: "abc123", name: "alpha", path: "/tmp/a", active: true },
  { id: "def456", name: "beta", path: "/tmp/b" },
];

// Tiny child component that reads the context.
function CtxReader() {
  const { workspaces, workspace, loading, refresh } = useWorkspace();
  return (
    <div>
      <div data-testid="loading">{loading ? "loading" : "ready"}</div>
      <div data-testid="count">{workspaces.length}</div>
      <div data-testid="active">{workspace?.name ?? "none"}</div>
      <button type="button" onClick={refresh} data-testid="refresh">
        refresh
      </button>
    </div>
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("WorkspaceProvider", () => {
  it("parses the bare-array response shape", async () => {
    fetchMock.mockImplementation(() => jsonResponse(sampleWorkspaces));

    render(
      <MemoryRouter>
        <WorkspaceProvider>
          <CtxReader />
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("ready");
    });
    expect(screen.getByTestId("count")).toHaveTextContent("2");
    expect(screen.getByTestId("active")).toHaveTextContent("alpha");
  });

  it("parses the object-wrapper response shape", async () => {
    fetchMock.mockImplementation(() =>
      jsonResponse({ workspaces: sampleWorkspaces, active: "abc123" }),
    );

    render(
      <MemoryRouter>
        <WorkspaceProvider>
          <CtxReader />
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("ready");
    });
    expect(screen.getByTestId("count")).toHaveTextContent("2");
  });

  it("filters out malformed entries (missing id/name)", async () => {
    fetchMock.mockImplementation(() =>
      jsonResponse([
        { id: "good", name: "keep", path: "/p" },
        { id: 123, name: "bad-id", path: "/p" }, // wrong id type
        { name: "no-id", path: "/p" },
        null,
      ]),
    );

    render(
      <MemoryRouter>
        <WorkspaceProvider>
          <CtxReader />
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("count")).toHaveTextContent("1");
    });
  });

  it("handles a non-OK response by yielding an empty list", async () => {
    fetchMock.mockImplementation(() => jsonResponse({}, 500));

    render(
      <MemoryRouter>
        <WorkspaceProvider>
          <CtxReader />
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("ready");
    });
    expect(screen.getByTestId("count")).toHaveTextContent("0");
  });

  it("refresh() triggers a second fetch", async () => {
    fetchMock.mockImplementation(() => jsonResponse(sampleWorkspaces));

    render(
      <MemoryRouter>
        <WorkspaceProvider>
          <CtxReader />
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("ready");
    });

    const before = fetchMock.mock.calls.length;
    await act(async () => {
      screen.getByTestId("refresh").click();
    });
    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
    });
  });
});

describe("ActiveWorkspaceGuard", () => {
  // The guard is a Route element; it reads :wsId from URL + looks up
  // the workspace list. It calls fetch() to /api/workspaces and then to
  // /api/workspaces/{id}/activate.
  it("renders children when wsId is valid", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (typeof url === "string" && url.endsWith("/activate")) {
        return jsonResponse({});
      }
      return jsonResponse(sampleWorkspaces);
    });

    render(
      <MemoryRouter initialEntries={["/w/abc123/agents"]}>
        <WorkspaceProvider>
          <Routes>
            <Route path="/w/:wsId" element={<ActiveWorkspaceGuard />}>
              <Route path="agents" element={<div>agents-content</div>} />
            </Route>
            <Route path="/w" element={<div>picker</div>} />
          </Routes>
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("agents-content")).toBeInTheDocument();
    });

    // Activation POST must have fired.
    const activateCalls = fetchMock.mock.calls.filter((c) =>
      typeof c[0] === "string" && c[0].includes("/activate"),
    );
    expect(activateCalls.length).toBeGreaterThan(0);
  });

  it("redirects to /w when wsId is unknown", async () => {
    fetchMock.mockImplementation(() => jsonResponse(sampleWorkspaces));

    render(
      <MemoryRouter initialEntries={["/w/not-a-real-id/agents"]}>
        <WorkspaceProvider>
          <Routes>
            <Route path="/w/:wsId" element={<ActiveWorkspaceGuard />}>
              <Route path="agents" element={<div>agents-content</div>} />
            </Route>
            <Route path="/w" element={<div data-testid="picker">picker</div>} />
          </Routes>
        </WorkspaceProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("picker")).toBeInTheDocument();
    });
    expect(screen.queryByText("agents-content")).toBeNull();
  });
});
