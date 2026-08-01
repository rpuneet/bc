import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { BootSplash, type SplashTimings } from "../BootSplash";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

// Collapse the draw/rise/fade phases and use a tiny stall window so the
// fallback state appears almost immediately in tests, while still probing
// on a real (short) interval.
const STALL_FAST: SplashTimings = {
  drawMs: 0,
  minStreamMs: 0,
  riseMs: 0,
  fadeMs: 0,
  boot: { pollMs: 5, paceMs: 0, stallMs: 20 },
};

beforeEach(() => {
  fetchMock.mockReset();
});

describe("BootSplash stall fallback", () => {
  it("surfaces a stall state with retry/continue instead of spinning forever", async () => {
    // The daemon never answers.
    fetchMock.mockImplementation(() => Promise.reject(new Error("connection refused")));

    const onReady = vi.fn();
    render(<BootSplash onReady={onReady} timings={STALL_FAST} />);

    // Initially just the "connecting" state — no stall UI yet.
    expect(screen.queryByTestId("boot-stall")).not.toBeInTheDocument();

    await waitFor(() => expect(screen.getByTestId("boot-stall")).toBeInTheDocument());
    expect(
      screen.getByText(/still trying to reach the daemon/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();

    const continueBtn = screen.getByRole("button", { name: "Continue anyway" });
    continueBtn.click();
    expect(onReady).toHaveBeenCalled();
  });

  it("clears the stall state once a retry succeeds", async () => {
    let fail = true;
    fetchMock.mockImplementation((url: RequestInfo | URL) => {
      const u = String(url);
      if (fail) return Promise.reject(new Error("connection refused"));
      if (u.includes("/api/health"))
        return Promise.resolve({
          ok: true,
          status: 200,
          statusText: "OK",
          json: () => Promise.resolve({ status: "ok" }),
        } as Response);
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: () => Promise.resolve([]),
      } as Response);
    });

    render(<BootSplash onReady={vi.fn()} timings={STALL_FAST} />);

    await waitFor(() => expect(screen.getByTestId("boot-stall")).toBeInTheDocument());

    fail = false;
    screen.getByRole("button", { name: "Retry" }).click();

    await waitFor(() => expect(screen.queryByTestId("boot-stall")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("daemon online")).toBeInTheDocument());
  });
});
