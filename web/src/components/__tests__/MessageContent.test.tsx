import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MessageContent } from "../MessageContent";

/* Cross-surface links: an agent feed message must connect its @mentions
   to the agent detail page and its #channel refs to the Apps page, so
   the surfaces read as one system rather than isolated views. */

afterEach(cleanup);

describe("MessageContent cross-surface links", () => {
  it("links an @mention to the agent detail page", () => {
    render(
      <MessageContent content="ping @zen-zebra when ready" agentNames={new Set(["zen-zebra"])} />,
    );
    const link = screen.getByRole("link", { name: "@zen-zebra" });
    expect(link).toHaveAttribute("href", "/agents/zen-zebra");
  });

  it("links a #channel reference to the Apps page", () => {
    render(<MessageContent content="see #engineering for context" />);
    const link = screen.getByRole("link", { name: "#engineering" });
    expect(link).toHaveAttribute("href", "/apps/engineering");
  });

  it("still renders an unknown @mention as a link, styled as muted", () => {
    render(<MessageContent content="hi @ghost" agentNames={new Set(["zen-zebra"])} />);
    const link = screen.getByRole("link", { name: "@ghost" });
    expect(link).toHaveAttribute("href", "/agents/ghost");
  });
});
