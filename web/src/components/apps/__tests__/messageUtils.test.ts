/**
 * messageUtils — gateway detection covers every built-in app (one
 * gateway package per app, see pkg/app/builtin/builtin.go on the Go
 * side) so a real human sender's channel/message is never mistaken for
 * an internal agent channel and hidden from Apps views.
 */

import { describe, expect, it, afterEach } from "vitest";
import {
  GATEWAY_BASES,
  gatewayPlatform,
  isGatewaySource,
  registerAppBases,
  sourcePlatform,
} from "../messageUtils";

// Kept in lockstep with the imports in pkg/app/builtin/builtin.go — every
// built-in app registers exactly one gateway package with this base name.
const REAL_APP_BASES = [
  "bitbucket", "datadog", "discord", "github", "gitlab", "gmail", "grafana",
  "imessage", "irc", "jira", "line", "linear", "matrix", "mattermost", "mqtt",
  "netlify", "notion", "pagerduty", "reddit", "rss", "sentry", "signal",
  "slack", "stripe", "telegram", "twitch", "vercel", "webhook", "whatsapp",
];

describe("GATEWAY_BASES (bootstrap default)", () => {
  it("covers every built-in app registered in pkg/app/builtin/builtin.go", () => {
    for (const base of REAL_APP_BASES) {
      expect(GATEWAY_BASES).toContain(base);
    }
  });

  it("does not contain dead/unregistered prefixes", () => {
    for (const dead of ["twitter", "nostr", "homeassistant"]) {
      expect(GATEWAY_BASES).not.toContain(dead);
    }
  });
});

describe("sourcePlatform / isGatewaySource / gatewayPlatform (default set)", () => {
  it.each(REAL_APP_BASES)("recognizes %s: as a gateway source", (base) => {
    const name = `${base}:general`;
    expect(isGatewaySource(name)).toBe(true);
    expect(gatewayPlatform(name)).toBe(base);
    expect(sourcePlatform(name)).toBe(base);
  });

  it("buckets internal/agent channel names as internal", () => {
    expect(sourcePlatform("root")).toBe("internal");
    expect(sourcePlatform("engineering")).toBe("internal");
    expect(isGatewaySource("engineering")).toBe(false);
    expect(gatewayPlatform("engineering")).toBeNull();
  });
});

describe("registerAppBases (live /api/apps catalog)", () => {
  afterEach(() => {
    // Restore the default set so other tests aren't affected by ordering.
    registerAppBases(REAL_APP_BASES);
  });

  it("replaces the known set with whatever the catalog reports", () => {
    registerAppBases(["slack", "custom_app"]);
    expect(sourcePlatform("custom_app:channel")).toBe("custom_app");
    expect(sourcePlatform("slack:general")).toBe("slack");
    // No longer recognized once the catalog no longer reports it.
    expect(sourcePlatform("github:repo")).toBe("internal");
  });

  it("ignores an empty catalog rather than wiping known gateways", () => {
    registerAppBases(["slack"]);
    registerAppBases([]);
    expect(sourcePlatform("slack:general")).toBe("slack");
  });
});
