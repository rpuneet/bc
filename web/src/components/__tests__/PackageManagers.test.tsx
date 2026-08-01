/**
 * PackageManagers.test.tsx — the Tools-page package-manager readout must show
 * exactly the managers the backend reports (name + version) and degrade to an
 * honest "none detected" line when the host has none. No fabricated managers.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { PackageManagers } from "../PackageManagers";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) } as Response);
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("PackageManagers", () => {
  it("renders detected managers with versions", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      if (String(url).endsWith("/api/system/package-managers")) {
        return jsonResponse({
          os: "darwin",
          arch: "arm64",
          managers: [
            { id: "brew", name: "Homebrew", version: "Homebrew 4.2.0", available: true, searchable: true },
            { id: "npm", name: "npm", version: "10.2.4", available: true, searchable: true },
          ],
        });
      }
      return jsonResponse({});
    });

    render(<PackageManagers />);
    await waitFor(() => expect(screen.getByText("Homebrew")).toBeTruthy());
    expect(screen.getByText("npm")).toBeTruthy();
    expect(screen.getByText("10.2.4")).toBeTruthy();
  });

  it("shows an honest empty state when no managers are detected", async () => {
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      if (String(url).endsWith("/api/system/package-managers")) {
        return jsonResponse({ os: "linux", arch: "amd64", managers: [] });
      }
      return jsonResponse({});
    });

    render(<PackageManagers />);
    await waitFor(() => expect(screen.getByText(/No supported package managers detected/)).toBeTruthy());
  });
});
