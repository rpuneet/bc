/**
 * GatewayFeed's "Go back" control must land on a known page, not rely on
 * history.back(). A channel view (/apps/:sourceName) is reachable via a
 * direct deep link or bookmark, so there is no guaranteed prior in-app
 * history entry — navigate(-1) could pop the user out of the SPA entirely.
 * navigate("/apps") always resolves to the Apps hub.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
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
  it("'Go back' navigates to /apps directly, even with no prior in-app history entry", async () => {
    let lastPathname = "";
    function LocationSpy() {
      lastPathname = useLocation().pathname;
      return null;
    }

    render(
      <MemoryRouter initialEntries={["/apps/slack:general"]}>
        <LocationSpy />
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

    const backBtn = await screen.findByRole("button", { name: "Go back" });
    fireEvent.click(backBtn);

    await waitFor(() => {
      expect(lastPathname).toBe("/apps");
    });
    expect(screen.getByText("Apps Home")).toBeInTheDocument();
  });
});
