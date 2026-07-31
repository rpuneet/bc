import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentSelect } from "../AgentSelect";

afterEach(cleanup);

const AGENTS = [
  { name: "zen-zebra", state: "working", tool: "claude" },
  { name: "beacon-scout", state: "idle", tool: "claude" },
];

describe("AgentSelect", () => {
  it("opens a listbox of agent chips and selects one", () => {
    const onChange = vi.fn();
    render(<AgentSelect agents={AGENTS} value="" onChange={onChange} allowNone placeholder="— none —" />);

    // Closed initially — no listbox.
    expect(screen.queryByRole("listbox")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Select an agent" }));
    const listbox = screen.getByRole("listbox");
    expect(listbox).toBeInTheDocument();
    // Every agent renders as a chip (its character has an accessible name).
    expect(screen.getByRole("img", { name: /zen-zebra/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("option", { name: /beacon-scout/ }));
    expect(onChange).toHaveBeenCalledWith("beacon-scout");
    // Closes after picking.
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("offers a none row that clears the selection", () => {
    const onChange = vi.fn();
    render(<AgentSelect agents={AGENTS} value="zen-zebra" onChange={onChange} allowNone noneLabel="— none —" />);
    fireEvent.click(screen.getByRole("button", { name: "Select an agent" }));
    fireEvent.click(screen.getByRole("option", { name: "— none —" }));
    expect(onChange).toHaveBeenCalledWith("");
  });
});
