import { describe, it, expect } from "vitest";
import { resolveMycelColor } from "../mycelColor";

describe("resolveMycelColor", () => {
  it("returns the bare token for solid utilities", () => {
    expect(resolveMycelColor("--mycel-border")).toBe("var(--mycel-border)");
    expect(resolveMycelColor("--mycel-border", undefined)).toBe("var(--mycel-border)");
    expect(resolveMycelColor("--mycel-border", "var(--tw-border-opacity, 1)")).toBe(
      "var(--mycel-border)",
    );
  });

  it("color-mixes numeric /NN modifiers so dark borders stay on-token", () => {
    expect(resolveMycelColor("--mycel-border", "0.7")).toBe(
      "color-mix(in srgb, var(--mycel-border) 70%, transparent)",
    );
    expect(resolveMycelColor("--mycel-accent-subtle", "0.25")).toBe(
      "color-mix(in srgb, var(--mycel-accent-subtle) 25%, transparent)",
    );
  });
});
