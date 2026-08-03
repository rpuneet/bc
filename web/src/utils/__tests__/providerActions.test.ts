import { describe, it, expect } from "vitest";
import { canAutoInstall, canAutoUninstall } from "../providerActions";

/**
 * These predicates exist to keep the UI from offering an action the daemon will
 * refuse. The refusal arrives after the click — and for Remove, after a two-click
 * destructive confirm — so a wrong answer here is not cosmetic (#3475).
 *
 * The hints below are the real ones the providers ship.
 */
describe("canAutoInstall", () => {
  it("accepts a hint that is a command", () => {
    expect(canAutoInstall("npm install -g @anthropic-ai/claude-code")).toBe(true);
    expect(canAutoInstall("brew install codex")).toBe(true);
    expect(canAutoInstall("curl -fsSL https://agy.sh/install | sh")).toBe(true);
  });

  it("refuses a bare download URL", () => {
    // cursor's hint is literally this: a page a person visits, not a command.
    expect(canAutoInstall("https://cursor.sh")).toBe(false);
    expect(canAutoInstall("http://example.com/download")).toBe(false);
  });

  it("refuses nothing at all", () => {
    expect(canAutoInstall("")).toBe(false);
    expect(canAutoInstall("   ")).toBe(false);
    expect(canAutoInstall(null)).toBe(false);
    expect(canAutoInstall(undefined)).toBe(false);
  });
});

describe("canAutoUninstall", () => {
  it("accepts the three forms the daemon can invert", () => {
    expect(canAutoUninstall("npm install -g @anthropic-ai/claude-code")).toBe(true);
    expect(canAutoUninstall("npm i -g mycel-cli")).toBe(true);
    expect(canAutoUninstall("brew install tmux")).toBe(true);
  });

  it("refuses an installer with nothing to invert", () => {
    // agy installs by piping a script into sh; there is no uninstall to derive,
    // and the daemon answers HTTP 400. The button used to be rendered anyway.
    expect(canAutoUninstall("curl -fsSL https://agy.sh/install | sh")).toBe(false);
    expect(canAutoUninstall("https://cursor.sh")).toBe(false);
    expect(canAutoUninstall("go install github.com/foo/bar@latest")).toBe(false);
  });

  it("refuses a package manager prefix with no package after it", () => {
    expect(canAutoUninstall("npm install -g ")).toBe(false);
    expect(canAutoUninstall("brew install")).toBe(false);
  });

  it("refuses nothing at all", () => {
    expect(canAutoUninstall("")).toBe(false);
    expect(canAutoUninstall(null)).toBe(false);
    expect(canAutoUninstall(undefined)).toBe(false);
  });
});
