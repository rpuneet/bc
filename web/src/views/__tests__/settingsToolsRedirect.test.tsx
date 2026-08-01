/**
 * The Settings "Providers & Tools" drill-down resolves to the flat /tools
 * route. The nested /settings/tools mount remounted in a loop and never
 * settled; every entry point now redirects to the proven /tools view.
 *   /settings/tools            → /tools
 *   /settings/tools/<provider> → /tools/<provider>
 *   /settings/providers        → /tools
 *   /providers                 → /tools
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

describe("Settings tools redirects", () => {
  it("/settings/tools lands on the flat /tools page", async () => {
    renderAt("/settings/tools");
    await waitFor(() => {
      expect(lastLocation).toBe("/tools");
    });
  });

  it("/settings/tools/<provider> preserves the provider in the redirect", async () => {
    renderAt("/settings/tools/claude");
    await waitFor(() => {
      expect(lastLocation).toBe("/tools/claude");
    });
  });

  it("/settings/providers redirects to /tools", async () => {
    renderAt("/settings/providers");
    await waitFor(() => {
      expect(lastLocation).toBe("/tools");
    });
  });

  it("/providers redirects to /tools", async () => {
    renderAt("/providers");
    await waitFor(() => {
      expect(lastLocation).toBe("/tools");
    });
  });
});
