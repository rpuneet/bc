/**
 * Route redirects — old bookmarks survive the Apps rename:
 *   /notifications              → /apps
 *   /notifications/activity     → /apps/activity
 *   /notifications/<source>     → /apps/<source>
 *   /secrets                    → /apps#custom-keys
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

describe("Apps redirects", () => {
  it("/notifications redirects to /apps", async () => {
    renderAt("/notifications");
    await waitFor(() => {
      expect(lastLocation).toBe("/apps");
    });
  });

  it("/notifications/activity redirects to /apps/activity", async () => {
    renderAt("/notifications/activity");
    await waitFor(() => {
      expect(lastLocation).toBe("/apps/activity");
    });
  });

  it("/notifications/<source> keeps the channel in the redirect", async () => {
    renderAt("/notifications/slack:general");
    await waitFor(() => {
      expect(lastLocation).toBe("/apps/slack:general");
    });
  });

  it("/secrets redirects to the Apps custom-keys section", async () => {
    renderAt("/secrets");
    await waitFor(() => {
      expect(lastLocation).toBe("/apps#custom-keys");
    });
  });
});
