/**
 * GatewayFeed used to grow its own "Go back" control because a channel
 * view (/apps/:sourceName) is reachable via a direct deep link or
 * bookmark with no guaranteed prior in-app history entry. That's now the
 * header's job (HistoryNavButtons, one control for the whole app) — this
 * view must not resurrect a page-level back button.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { HeaderSlotProvider, useHeaderSlotContext } from "../../../context/HeaderSlotContext";
import { GatewayFeed } from "../GatewayFeed";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function HeaderHost() {
  const { slot } = useHeaderSlotContext();
  return (
    <div data-testid="header-host">
      {slot.title}
      {slot.actions}
    </div>
  );
}

// jsdom has no IntersectionObserver; GatewayFeed's infinite-scroll sentinel
// only needs a no-op stand-in for this test.
class FakeIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockReturnValue(jsonResponse([]));
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
});

describe("GatewayFeed", () => {
  it("no longer renders its own 'Go back' button — the header owns back/forward", async () => {
    render(
      <MemoryRouter initialEntries={["/apps/slack:general"]}>
        <HeaderSlotProvider>
          <HeaderHost />
          <Routes>
            <Route path="/apps" element={<div>Apps Home</div>} />
            <Route
              path="/apps/:sourceName"
              element={<GatewayFeed channelName="slack:general" onPeekAgent={() => {}} />}
            />
          </Routes>
        </HeaderSlotProvider>
      </MemoryRouter>,
    );

    await screen.findByTestId("header-host");
    expect(screen.queryByRole("button", { name: "Go back" })).not.toBeInTheDocument();
  });
});
