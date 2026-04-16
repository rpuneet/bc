/**
 * WorkspaceDropdown.test.tsx — UI behavior tests for the workspace
 * switcher.
 *
 * Covers:
 *   - Active workspace name is shown in the trigger button.
 *   - Clicking the trigger opens the menu; the search input becomes
 *     visible and filters results.
 *   - Cmd/Ctrl+K toggles the menu.
 *   - Selecting a workspace calls navigate() to the scoped URL.
 *   - Escape closes the menu.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { WorkspaceDropdown } from "./WorkspaceDropdown";
import { WorkspaceProvider } from "../context/WorkspaceContext";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const ws = [
  { id: "abc123abcabc", name: "alpha", path: "/tmp/alpha", active: true },
  { id: "def456defdef", name: "beta", path: "/tmp/beta" },
  { id: "ghi789ghighi", name: "gamma", path: "/tmp/gamma" },
];

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="probe">{loc.pathname}</div>;
}

function renderDropdown(initialEntries: string[] = ["/w/abc123abcabc/agents"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <WorkspaceProvider>
        <Routes>
          <Route
            path="/w/:wsId/*"
            element={
              <div>
                <WorkspaceDropdown />
                <LocationProbe />
              </div>
            }
          />
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </WorkspaceProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockImplementation((url: string) => {
    if (typeof url === "string" && url.endsWith("/activate")) {
      return jsonResponse({});
    }
    return jsonResponse(ws);
  });
});

describe("WorkspaceDropdown", () => {
  it("shows the active workspace name in the trigger", async () => {
    renderDropdown();
    await waitFor(() => {
      expect(screen.getByText("alpha")).toBeInTheDocument();
    });
  });

  it("opens on click and shows every workspace", async () => {
    renderDropdown();
    await waitFor(() => {
      expect(screen.getByText("alpha")).toBeInTheDocument();
    });

    await act(async () => {
      screen.getByTitle(/switch workspace/i).click();
    });

    // All ws names appear in the menu (alpha also in the trigger button).
    await waitFor(() => {
      expect(screen.getAllByText("alpha").length).toBeGreaterThan(0);
      expect(screen.getByText("beta")).toBeInTheDocument();
      expect(screen.getByText("gamma")).toBeInTheDocument();
    });
  });

  it("filters via the search input", async () => {
    renderDropdown();
    await waitFor(() => {
      expect(screen.getByText("alpha")).toBeInTheDocument();
    });

    await act(async () => {
      screen.getByTitle(/switch workspace/i).click();
    });

    const input = await screen.findByPlaceholderText(/search workspaces/i);
    await act(async () => {
      fireEvent.change(input, { target: { value: "bet" } });
    });

    // Only "beta" should match.
    await waitFor(() => {
      expect(screen.getByText("beta")).toBeInTheDocument();
      expect(screen.queryByText("gamma")).toBeNull();
    });
  });

  it("opens on Cmd+Shift+W and closes on Escape", async () => {
    renderDropdown();
    await waitFor(() => {
      expect(screen.getByText("alpha")).toBeInTheDocument();
    });

    // Cmd+Shift+W opens (see WorkspaceDropdown keydown handler at
    // src/components/WorkspaceDropdown.tsx — title says
    // "Switch workspace (Cmd+Shift+W)"). The test was originally
    // Cmd+K but the component's shortcut changed.
    await act(async () => {
      fireEvent.keyDown(window, { key: "w", metaKey: true, shiftKey: true });
    });
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search workspaces/i)).toBeInTheDocument();
    });

    // Escape closes
    await act(async () => {
      fireEvent.keyDown(window, { key: "Escape" });
    });
    await waitFor(() => {
      expect(screen.queryByPlaceholderText(/search workspaces/i)).toBeNull();
    });
  });

  it("navigates to the target workspace when selected", async () => {
    renderDropdown(["/w/abc123abcabc/agents"]);
    await waitFor(() => {
      expect(screen.getByText("alpha")).toBeInTheDocument();
    });

    await act(async () => {
      screen.getByTitle(/switch workspace/i).click();
    });

    const betaEntry = await screen.findByText("beta");
    await act(async () => {
      betaEntry.click();
    });

    // Path should now include the beta workspace id.
    await waitFor(() => {
      const probe = screen.getByTestId("probe");
      expect(probe.textContent).toContain("def456defdef");
    });
  });
});
