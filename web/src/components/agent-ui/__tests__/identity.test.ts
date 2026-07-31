import { describe, expect, it } from "vitest";
import {
  ALL_FORMS,
  deriveIdentity,
  hashName,
  stateAnimClass,
} from "../identity";
import type { BodyForm } from "../identity";

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

  it("reaches all ten species with a roughly uniform distribution", () => {
    const counts = new Map<BodyForm, number>();
    const total = 2000;
    for (let i = 0; i < total; i++) {
      const { form } = deriveIdentity(`agent-${String(i)}`);
      counts.set(form, (counts.get(form) ?? 0) + 1);
    }
    expect(counts.size).toBe(ALL_FORMS.length);
    for (const form of ALL_FORMS) {
      const share = (counts.get(form) ?? 0) / total;
      // Each species owns ~10% of the namespace; allow generous slack.
      expect(share).toBeGreaterThan(0.05);
      expect(share).toBeLessThan(0.16);
    }
  });

  it("preserves the pre-expansion species when the roll lands in the legacy band", () => {
    // Contract: the original 3-species release assigned FORMS[h % 3].
    // After the ten-species expansion, any name whose separate species
    // roll (top hash byte % 10) falls below 3 must keep that assignment.
    const legacy: BodyForm[] = ["spore", "cap", "sprout"];
    let checked = 0;
    for (let i = 0; i < 500; i++) {
      const name = `agent-${String(i)}`;
      const h = hashName(name);
      if ((h >>> 24) % 10 < 3) {
        expect(deriveIdentity(name).form).toBe(legacy[h % 3]);
        checked++;
      }
    }
    expect(checked).toBeGreaterThan(50); // the legacy band is ~30% of names
  });

  it("pins known names to their species (regression guard)", () => {
    // These exact assignments are user-visible identity; changing any of
    // them silently reshuffles live fleets. Update only intentionally.
    const pinned: Record<string, BodyForm> = {
      "zen-zebra": "chanterelle",
      "lucid-meerkat": "cap", // kept from the 3-species era
      "amber-falcon": "spore", // kept from the 3-species era
      "bold-otter": "bracket",
      "gold-panda": "coral",
      "keen-crow": "spore", // kept from the 3-species era
    };
    for (const [name, form] of Object.entries(pinned)) {
      expect(deriveIdentity(name).form).toBe(form);
    }
  });

  it("keeps every derived value inside its family", () => {
    for (const n of ["a", "zz", "very-long-agent-name-with-suffix-42", "日本語"]) {
      const id = deriveIdentity(n);
      expect(ALL_FORMS).toContain(id.form);
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
