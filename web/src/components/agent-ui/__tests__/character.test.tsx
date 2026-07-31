import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentCharacter } from "../AgentCharacter";
import { AgentChip } from "../AgentChip";
import { handleAgentEvent } from "../agentEventBus";
import { ALL_FORMS, deriveIdentity } from "../identity";
import type { BodyForm } from "../identity";
import { PULSE_MS, useAgentPulse } from "../useAgentPulse";

/** One representative agent name per species, found by probing the
 *  deterministic hash — keeps the suite honest if the mapping changes. */
function speciesRepresentatives(): Map<BodyForm, string> {
  const reps = new Map<BodyForm, string>();
  for (let i = 0; reps.size < ALL_FORMS.length && i < 100000; i++) {
    const name = `probe-${String(i)}`;
    const { form } = deriveIdentity(name);
    if (!reps.has(form)) reps.set(form, name);
  }
  return reps;
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("AgentCharacter", () => {
  it("renders the same silhouette for the same name", () => {
    const { container: a } = render(<AgentCharacter name="zen-zebra" state="idle" size={32} />);
    const { container: b } = render(<AgentCharacter name="zen-zebra" state="idle" size={32} />);
    expect(a.innerHTML).toBe(b.innerHTML);
  });

  it("applies the state animation class and aria label", () => {
    render(<AgentCharacter name="zen-zebra" state="working" size={32} />);
    const el = screen.getByRole("img", { name: "zen-zebra — working" });
    expect(el.className).toContain("agent-anim-working");
    // Working state shows the orbiting tool mote.
    expect(el.querySelector(".agent-orbit")).not.toBeNull();
  });

  it("closes the eyes when stopped", () => {
    const { container } = render(<AgentCharacter name="zen-zebra" state="stopped" size={32} />);
    const eyes = container.querySelector(".agent-eyes");
    expect(eyes).not.toBeNull();
    // Closed eyes are stroked arcs, not filled pupils.
    expect(eyes?.querySelectorAll("path").length).toBe(2);
  });

  it("layers detail with size — marks hidden at 16px, tool chip at 64px", () => {
    const { container: small } = render(
      <AgentCharacter name="zen-zebra" state="idle" size={16} tool="claude" />,
    );
    const { container: large } = render(
      <AgentCharacter name="zen-zebra" state="idle" size={64} tool="claude" />,
    );
    expect(small.querySelectorAll("text").length).toBe(0);
    expect(large.querySelectorAll("text").length).toBe(1); // tool glyph chip
  });

  it("finds a representative name for every one of the ten species", () => {
    expect([...speciesRepresentatives().keys()].sort()).toEqual([...ALL_FORMS].sort());
  });

  for (const size of [16, 28, 96]) {
    it(`renders every species at ${String(size)}px with a face and correct layering`, () => {
      for (const [form, name] of speciesRepresentatives()) {
        const { container, unmount } = render(
          <AgentCharacter name={name} state="idle" size={size} tool="claude" />,
        );
        const root = container.firstChild as HTMLElement;
        expect(root.getAttribute("data-agent-form")).toBe(form);
        // Body silhouette and eyes exist on every species at every size.
        expect(container.querySelector(".agent-body")).not.toBeNull();
        expect(container.querySelector(".agent-eyes")).not.toBeNull();
        // Tool chip (the only <text>) appears at ≥56px only.
        expect(container.querySelectorAll("text").length).toBe(size >= 56 ? 1 : 0);
        unmount();
      }
    });
  }

  it("closes the eyes when stopped, on every species", () => {
    for (const [, name] of speciesRepresentatives()) {
      const { container, unmount } = render(
        <AgentCharacter name={name} state="stopped" size={32} />,
      );
      const eyes = container.querySelector(".agent-eyes");
      expect(eyes?.querySelectorAll("path").length).toBe(2);
      unmount();
    }
  });

  it("applies each state animation class on every species", () => {
    const states = [
      ["idle", "agent-anim-idle"],
      ["working", "agent-anim-working"],
      ["stuck", "agent-anim-stuck"],
      ["error", "agent-anim-error"],
      ["waiting", "agent-anim-waiting"],
      ["stopped", "agent-anim-stopped"],
    ] as const;
    for (const [, name] of speciesRepresentatives()) {
      for (const [state, cls] of states) {
        const { container, unmount } = render(
          <AgentCharacter name={name} state={state} size={32} />,
        );
        expect((container.firstChild as HTMLElement).className).toContain(cls);
        unmount();
      }
    }
  });

  it("shows speech dots on a message pulse", () => {
    const { container } = render(
      <AgentCharacter name="zen-zebra" state="idle" size={32} pulse="message" />,
    );
    expect(container.querySelector(".agent-speech")).not.toBeNull();
    expect((container.firstChild as HTMLElement).className).toContain("agent-pulse-message");
  });
});

describe("AgentChip", () => {
  it("renders character, name and status dot", () => {
    render(<AgentChip name="lucid-meerkat" state="working" />);
    expect(screen.getByText("lucid-meerkat")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "lucid-meerkat — working" })).toBeInTheDocument();
    expect(screen.getByTestId("agent-chip-dot")).toBeInTheDocument();
  });

  it("becomes a button when onClick is provided", () => {
    const onClick = vi.fn();
    render(<AgentChip name="lucid-meerkat" state="idle" onClick={onClick} />);
    screen.getByRole("button", { name: /lucid-meerkat/ }).click();
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});

function PulseProbe({ name }: { name: string }) {
  const pulse = useAgentPulse(name);
  return <output data-testid="pulse">{pulse ?? "none"}</output>;
}

describe("useAgentPulse", () => {
  it("reacts to bus events for its agent only, one pulse at a time", () => {
    vi.useFakeTimers();
    render(<PulseProbe name="zen-zebra" />);
    expect(screen.getByTestId("pulse").textContent).toBe("none");

    act(() => {
      handleAgentEvent({
        type: "agent.hook",
        data: { agent: "zen-zebra" },
        timestamp: "",
      });
    });
    expect(screen.getByTestId("pulse").textContent).toBe("tool");

    // A second event while a pulse is active is queue-dropped.
    act(() => {
      handleAgentEvent({
        type: "channel.message",
        data: { sender: "zen-zebra" },
        timestamp: "",
      });
    });
    expect(screen.getByTestId("pulse").textContent).toBe("tool");

    // Events for other agents never reach this probe.
    act(() => {
      handleAgentEvent({
        type: "agent.hook",
        data: { agent: "someone-else" },
        timestamp: "",
      });
    });
    expect(screen.getByTestId("pulse").textContent).toBe("tool");

    act(() => {
      vi.advanceTimersByTime(PULSE_MS + 10);
    });
    expect(screen.getByTestId("pulse").textContent).toBe("none");
  });

  it("stays silent when the user prefers reduced motion", () => {
    const original = window.matchMedia;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: (q: string) => ({
        matches: q.includes("prefers-reduced-motion"),
        media: q,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        onchange: null,
        dispatchEvent: () => false,
      }),
    });
    try {
      render(<PulseProbe name="quiet-agent" />);
      act(() => {
        handleAgentEvent({
          type: "agent.hook",
          data: { agent: "quiet-agent" },
          timestamp: "",
        });
      });
      expect(screen.getByTestId("pulse").textContent).toBe("none");
    } finally {
      Object.defineProperty(window, "matchMedia", { writable: true, value: original });
    }
  });
});
