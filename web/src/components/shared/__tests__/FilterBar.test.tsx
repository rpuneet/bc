import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FiltersChip, ListSearchInput } from "../FilterBar";

describe("FilterBar", () => {
  it("renders ListSearchInput with accessible name", () => {
    render(<ListSearchInput aria-label="Search agents" placeholder="Search" />);
    expect(screen.getByRole("searchbox", { name: "Search agents" })).toBeInTheDocument();
  });

  it("FiltersChip toggles popover and shows active count", () => {
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <FiltersChip open={false} onOpenChange={onOpenChange} activeCount={2} testId="filters-pop">
        <div>body</div>
      </FiltersChip>,
    );
    expect(screen.getByRole("button", { name: "Filters" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("2")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    expect(onOpenChange).toHaveBeenCalledWith(true);

    rerender(
      <FiltersChip open onOpenChange={onOpenChange} activeCount={2} testId="filters-pop">
        <div>body</div>
      </FiltersChip>,
    );
    expect(screen.getByTestId("filters-pop")).toHaveTextContent("body");
  });
});
