import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageHeader } from "../PageHeader";

describe("PageHeader", () => {
  it("renders title, subtitle, and actions", () => {
    render(
      <PageHeader
        title="Settings"
        subtitle="Grouped for day-to-day use."
        actions={<button type="button">Go</button>}
      />,
    );
    expect(screen.getByRole("heading", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByText("Grouped for day-to-day use.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Go" })).toBeInTheDocument();
  });
});
