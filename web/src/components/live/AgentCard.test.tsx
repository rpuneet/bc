/**
 * AgentCard.test.tsx — the Live page agent card header toolbar (#3423
 * follow-up).
 *
 * Invariants:
 *   - The running count is shown in exactly ONE place: the pinned
 *     "Running n" section header. The old redundant "n running" chip in
 *     the card header toolbar is gone, so the count never renders twice.
 *   - Cost + tokens render as quiet, tabular-nums secondary metadata.
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentCard } from "./LiveRenderers";
import type { AgentActivity, ToolNode } from "./liveTypes";

function runningNode(overrides: Partial<ToolNode> = {}): ToolNode {
  return {
    id: "r1",
    toolName: "Bash",
    args: "npm run build",
    fullInput: null,
    fullOutput: null,
    status: "running",
    startTime: Date.now() - 2000,
    endTime: undefined,
    children: [],
    ...overrides,
  };
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return {
    name: "eng-01",
    state: "working",
    task: "",
    tool: "claude",
    role: "base",
    tokens: 12000,
    inputTokens: 8000,
    outputTokens: 4000,
    costUsd: 0.42,
    lastEventTime: Date.now(),
    nodes: [runningNode()],
    collapsed: false,
    ...overrides,
  };
}

function renderCard(a: AgentActivity) {
  return render(
    <MemoryRouter>
      <AgentCard
        activity={a}
        onToggle={vi.fn()}
        onDrillDown={vi.fn()}
        isFilterActive={false}
        searchTerm=""
        typeFilter="all"
      />
    </MemoryRouter>,
  );
}

describe("AgentCard header toolbar", () => {
  it("shows the running count exactly once — in the pinned section header, not the toolbar", () => {
    renderCard(activity({ nodes: [runningNode()] }));

    // The pinned "Running" section header labels the running rows once.
    expect(screen.getByText("Running")).toBeTruthy();

    // The old toolbar chip ("1 running") is gone — no element pairs the
    // count with the word "running" anywhere in the card.
    expect(screen.queryByText(/\d+\s+running/i)).toBeNull();
  });

  it("renders cost and tokens as quiet tabular-nums secondary metadata", () => {
    renderCard(activity({ costUsd: 0.42, tokens: 12000 }));

    // Both figures live inside the muted, tabular-nums metadata cluster
    // (font-variant-numeric inherits, so the class sits on the container).
    const cost = screen.getByText("$0.42");
    expect(cost).toBeTruthy();
    expect(cost.closest(".tabular-nums")).not.toBeNull();

    const tokens = screen.getByText("12,000 tok");
    expect(tokens).toBeTruthy();
    expect(tokens.closest(".tabular-nums")).not.toBeNull();
  });

  it("shows a degraded chip when template secrets were missing at create", () => {
    renderCard(activity({ missingSecrets: ["KITE_API_KEY", "TELEGRAM_BOT_TOKEN"] }));
    const chip = screen.getByLabelText(/Degraded — missing secrets: KITE_API_KEY, TELEGRAM_BOT_TOKEN/);
    expect(chip).toBeTruthy();
    expect(chip.textContent).toBe("degraded");
  });
});
