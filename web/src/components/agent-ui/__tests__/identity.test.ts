import { describe, expect, it } from "vitest";
import {
  deriveIdentity,
  hashName,
  stateAnimClass,
} from "../identity";

describe("deriveIdentity", () => {
  it("is deterministic — same name gives the same features object", () => {
    const a = deriveIdentity("zen-zebra");
    const b = deriveIdentity("zen-zebra");
    expect(a).toEqual(b);
    // Cached instance — cheap under many renders.
    expect(a).toBe(b);
  });

  it("gives different names distinct identities", () => {
    const names = [
      "zen-zebra",
      "lucid-meerkat",
      "amber-falcon",
      "bold-otter",
      "misty-heron",
      "iron-lynx",
      "gold-panda",
      "keen-crow",
    ];
    const ids = names.map((n) => deriveIdentity(n));
    // At least two different body forms and several hues across a small fleet.
    expect(new Set(ids.map((i) => i.form)).size).toBeGreaterThan(1);
    expect(new Set(ids.map((i) => i.hue)).size).toBeGreaterThan(2);
    // No two of these names produce a fully identical identity.
    const keys = ids.map((i) => JSON.stringify(i));
    expect(new Set(keys).size).toBe(names.length);
  });

  it("keeps every derived value inside its family", () => {
    for (const n of ["a", "zz", "very-long-agent-name-with-suffix-42", "日本語"]) {
      const id = deriveIdentity(n);
      expect(["spore", "cap", "sprout"]).toContain(id.form);
      expect(["round", "bead", "oval"]).toContain(id.eyes);
      expect(id.hue).toBeGreaterThanOrEqual(0);
      expect(id.hue).toBeLessThan(360);
      expect(id.marks.length).toBeGreaterThanOrEqual(1);
      expect(id.marks.length).toBeLessThanOrEqual(3);
      expect(Math.abs(id.tilt)).toBeLessThanOrEqual(4);
    }
  });
});

describe("hashName", () => {
  it("is stable and unsigned", () => {
    expect(hashName("zen-zebra")).toBe(hashName("zen-zebra"));
    expect(hashName("zen-zebra")).toBeGreaterThanOrEqual(0);
    expect(hashName("a")).not.toBe(hashName("b"));
  });
});

describe("stateAnimClass", () => {
  it("maps every agent state to its animation mood", () => {
    expect(stateAnimClass("idle")).toBe("agent-anim-idle");
    expect(stateAnimClass("starting")).toBe("agent-anim-starting");
    expect(stateAnimClass("working")).toBe("agent-anim-working");
    expect(stateAnimClass("done")).toBe("agent-anim-done");
    expect(stateAnimClass("stuck")).toBe("agent-anim-stuck");
    expect(stateAnimClass("error")).toBe("agent-anim-error");
    expect(stateAnimClass("waiting")).toBe("agent-anim-waiting");
    expect(stateAnimClass("stopped")).toBe("agent-anim-stopped");
  });

  it("defaults unknown states to idle", () => {
    expect(stateAnimClass("mystery")).toBe("agent-anim-idle");
    expect(stateAnimClass("")).toBe("agent-anim-idle");
  });
});
