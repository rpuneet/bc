import { describe, it, expect } from "vitest";
import { isReleaseVersion, sameBuild } from "../About";

/**
 * The About page shows "update available" only when the running build is a
 * release older than the latest tag. Getting this wrong is user-visible in both
 * directions: a false positive nags every developer running from source (#3212),
 * and a false negative hides a real update from someone on an old release.
 */
describe("isReleaseVersion", () => {
  it("accepts a plain release version", () => {
    expect(isReleaseVersion("0.4.4")).toBe(true);
    expect(isReleaseVersion("1.0.0")).toBe(true);
    // Multi-digit components must not be mistaken for something else — 0.3.13
    // is a real tag in this repo's history.
    expect(isReleaseVersion("0.3.13")).toBe(true);
    expect(isReleaseVersion("12.34.567")).toBe(true);
  });

  it("rejects the source-build version from scripts/version.sh", () => {
    expect(isReleaseVersion("0.4.5-dev.12.g1a2b3c4")).toBe(false);
    expect(isReleaseVersion("0.4.5-dev.0.g1a2b3c4.dirty")).toBe(false);
  });

  it("rejects the tagless sentinel and a bare commit hash", () => {
    // /api/health substitutes the commit when the build has no version at all.
    expect(isReleaseVersion("dev")).toBe(false);
    expect(isReleaseVersion("12029d9f")).toBe(false);
    expect(isReleaseVersion("")).toBe(false);
  });

  it("rejects the retired YYYY.MM.DD.<sha> format", () => {
    // Source builds no longer produce this, but binaries built before the
    // formats were unified are still installed on people's machines and must
    // keep reading as dev builds rather than as a release from the year 2026.
    expect(isReleaseVersion("2026.08.02.48266874")).toBe(false);
  });

  it("rejects a prerelease tag, which is not the latest release", () => {
    expect(isReleaseVersion("0.5.0-rc1")).toBe(false);
    expect(isReleaseVersion("0.1.0-alpha")).toBe(false);
  });
});

/**
 * A build's version can present itself two ways at once: the daemon appends
 * `.dirty` when its tree had uncommitted changes, and the desktop app's version
 * is stamped separately. Comparing the raw strings warned of a mismatch between
 * an app and a daemon built together, from the same tree, in the same minute —
 * and a warning that cannot be acted on teaches people to ignore the one case
 * it exists for.
 */
describe("sameBuild", () => {
  it("treats a dirty daemon and its own app as the same build", () => {
    expect(sameBuild("0.4.5-dev.35.gfbc9a1c", "0.4.5-dev.35.gfbc9a1c.dirty")).toBe(true);
  });

  it("is not fooled by which side is dirty", () => {
    expect(sameBuild("0.4.5-dev.35.gfbc9a1c.dirty", "0.4.5-dev.35.gfbc9a1c")).toBe(true);
  });

  it("still reports the mismatch worth warning about", () => {
    // A new app talking to a daemon left running from an older build: the whole
    // reason the comparison exists.
    expect(sameBuild("0.4.5-dev.35.gfbc9a1c", "0.4.4")).toBe(false);
    expect(sameBuild("0.4.5-dev.35.gfbc9a1c", "0.4.5-dev.12.g1a2b3c4")).toBe(false);
  });

  it("holds for identical versions, dirty or not", () => {
    expect(sameBuild("0.4.4", "0.4.4")).toBe(true);
    expect(sameBuild("0.4.5-dev.35.gfbc9a1c.dirty", "0.4.5-dev.35.gfbc9a1c.dirty")).toBe(true);
  });
});
