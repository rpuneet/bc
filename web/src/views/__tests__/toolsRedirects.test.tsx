/**
 * Route redirects — Providers/Tools folded into Settings, the standalone
 * /tools page is gone:
 *   /tools                          → /settings
 *   /tools/:provider                → /settings/providers/:provider
 *   /providers                      → /settings
 *   /settings/tools                 → /settings
 *   /settings/tools/:provider       → /settings/providers/:provider
 *   /settings/providers             → /settings
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AppRoutes } from "../../App";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

let lastLocation = "";
function LocationSpy() {
  const loc = useLocation();
  lastLocation = loc.pathname + loc.hash;
  return null;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LocationSpy />
      <AppRoutes />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockReturnValue(jsonResponse([]));
  lastLocation = "";
});

describe("Tools/Providers redirects", () => {
  it("/tools redirects to /settings", async () => {
    renderAt("/tools");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings");
    });
  });

  it("/tools/:provider redirects to /settings/providers/:provider", async () => {
    renderAt("/tools/claude");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings/providers/claude");
    });
  });

  it("/providers redirects to /settings", async () => {
    renderAt("/providers");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings");
    });
  });

  it("/settings/tools redirects to /settings", async () => {
    renderAt("/settings/tools");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings");
    });
  });

  it("/settings/tools/:provider redirects to /settings/providers/:provider", async () => {
    renderAt("/settings/tools/codex");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings/providers/codex");
    });
  });

  it("/settings/providers redirects to /settings", async () => {
    renderAt("/settings/providers");
    await waitFor(() => {
      expect(lastLocation).toBe("/settings");
    });
  });
});
