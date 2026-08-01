import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { HeaderSlotProvider, useHeaderSlotContext } from "../../context/HeaderSlotContext";
import {
  AppsHome,
  buildHomeModel,
  channelLeaf,
  isConnectableApp,
  resolveChannelKind,
  whatsappKindFromId,
} from "../../components/apps/AppsHome";
import { IdentityAvatar, avatarColor, initialsFor } from "../../components/apps/IdentityAvatar";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

function errorResponse(status: number) {
  return Promise.resolve({
    ok: false,
    status,
    statusText: "Not Found",
    json: () => Promise.resolve({ error: "not found" }),
  } as Response);
}

/* ── Fixtures ────────────────────────────────────────────────── */

const NOW = new Date().toISOString();
const GROUP_CH = "whatsapp:12345-67890@g.us";
const PERSON_CH = "whatsapp:919876543210@s.whatsapp.net";
const SLACK_CH = "slack:general";

const overview = {
  apps: [
    { name: "whatsapp", platform: "whatsapp", connected: true, channel_count: 2, last_activity: NOW },
    { name: "slack", platform: "slack", connected: false, disconnect_reason: "Invalid credentials", channel_count: 1 },
  ],
  channels: [
    {
      channel: GROUP_CH,
      platform: "whatsapp",
      display_name: "Family Group",
      kind: "group",
      avatar_url: "/api/apps/avatar?u=Z3JvdXA",
      participant_count: 12,
      subscriber_count: 1,
      message_count: 42,
      last_activity: NOW,
    },
    {
      channel: PERSON_CH,
      platform: "whatsapp",
      display_name: "Puneet Rai",
      kind: "person",
      message_count: 7,
      last_activity: NOW,
    },
    { channel: SLACK_CH, platform: "slack", display_name: "general", message_count: 3, last_activity: NOW },
  ],
};

const sources = [
  { name: GROUP_CH, description: "Gateway channel", members: [], member_count: 0 },
  { name: PERSON_CH, description: "Gateway channel", members: [], member_count: 0 },
  { name: SLACK_CH, description: "Gateway channel", members: [], member_count: 0 },
];

const gateways = [
  { platform: "whatsapp", enabled: true, channels: [] },
  { platform: "slack", enabled: true, channels: [] },
];

const labels = { whatsapp: "WhatsApp", slack: "Slack" };

/** GET /api/apps payload the home composes its model from. */
const appsCatalog = {
  catalog: [
    { id: "whatsapp", label: "WhatsApp", auth: "qr", multi: false, fields: [], docs: [] },
    { id: "slack", label: "Slack", auth: "token", multi: false, fields: [], docs: [] },
  ],
  instances: [
    { name: "whatsapp", app: "whatsapp", enabled: true, connected: true, channels: [] },
    { name: "slack", app: "slack", enabled: true, connected: false, channels: [] },
  ],
};

const subs = [
  { id: 1, channel: GROUP_CH, agent: "zen-zebra", mention_only: false, created_at: NOW },
];

const stats = [
  { name: GROUP_CH, message_count: 42, member_count: 0, last_activity: NOW, top_senders: [] },
];

const groupHistory = [
  { id: 1, sender: "[whatsapp] Mom", content: "Dinner at 8", created_at: NOW },
];

/** Route the fetch mock by URL. `overviewBody: null` simulates a 404 —
 *  the backend half of #3310 not deployed yet. */
function mockRoutes({ overviewBody = overview as unknown }: { overviewBody?: unknown } = {}) {
  fetchMock.mockImplementation((url: RequestInfo | URL) => {
    const u = String(url);
    if (u.includes("/api/notifications/overview")) {
      return overviewBody === null ? errorResponse(404) : jsonResponse(overviewBody);
    }
    if (u.includes("/history")) {
      return jsonResponse(u.includes(encodeURIComponent(GROUP_CH)) || u.includes(GROUP_CH) ? groupHistory : []);
    }
    if (/\/api\/apps\/[^/]+\/health/.test(u)) {
      if (u.includes("whatsapp")) {
        return jsonResponse({ platform: "whatsapp", connected: true, status: "connected", last_message_at: NOW });
      }
      return jsonResponse({ platform: "slack", connected: false, status: "error", error: "invalid_auth" });
    }
    if (u.includes("/api/apps/channels")) return jsonResponse(sources);
    if (u.includes("/api/apps")) return jsonResponse(appsCatalog);
    if (u.includes("/notify/subscriptions")) return jsonResponse(subs);
    if (u.includes("/stats/channels")) return jsonResponse(stats);
    if (u.includes("/api/secrets")) return jsonResponse([]);
    return jsonResponse([]);
  });
}

/* ── Harness — renders the header slot like Layout does ──────── */

function HeaderHost() {
  const { slot } = useHeaderSlotContext();
  return (
    <div data-testid="header-host">
      {slot.title}
      {slot.actions}
    </div>
  );
}

function renderHome() {
  return render(
    <MemoryRouter>
      <HeaderSlotProvider>
        <HeaderHost />
        <AppsHome />
      </HeaderSlotProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
});

/* ── Pure helpers ────────────────────────────────────────────── */

describe("AppsHome helpers", () => {
  it("classifies WhatsApp channels by JID shape", () => {
    expect(whatsappKindFromId(GROUP_CH)).toBe("group");
    expect(whatsappKindFromId(PERSON_CH)).toBe("person");
    expect(whatsappKindFromId("whatsapp:status")).toBeNull();
  });

  it("prefers adapter metadata over JID shape", () => {
    expect(resolveChannelKind(PERSON_CH, "whatsapp", "group")).toBe("group");
    expect(resolveChannelKind(GROUP_CH, "whatsapp", undefined)).toBe("group");
    expect(resolveChannelKind(SLACK_CH, "slack", undefined)).toBeNull();
  });

  it("extracts the leaf channel segment", () => {
    expect(channelLeaf("discord:Server:general")).toBe("general");
    expect(channelLeaf(GROUP_CH)).toBe("12345-67890@g.us");
  });

  it("buildHomeModel merges overview metadata over composed endpoints", () => {
    const { apps, channels } = buildHomeModel({
      overview,
      sources,
      gateways,
      labels,
      health: {
        whatsapp: { platform: "whatsapp", connected: true, status: "connected", last_message_at: NOW },
      },
      subs,
      stats,
    });
    const group = channels.find((c) => c.name === GROUP_CH);
    expect(group?.displayName).toBe("Family Group");
    expect(group?.kind).toBe("group");
    // Resolved channel avatar (a proxied path, never a raw CDN URL) flows through.
    expect(group?.avatarUrl).toBe("/api/apps/avatar?u=Z3JvdXA");
    const person = channels.find((c) => c.name === PERSON_CH);
    expect(person?.avatarUrl).toBe(""); // no avatar resolved → initials fallback
    expect(group?.participantCount).toBe(12);
    expect(group?.subscribers).toEqual(["zen-zebra"]);
    expect(group?.messageCount).toBe(42);
    const wa = apps.find((a) => a.key === "whatsapp");
    expect(wa?.status).toBe("connected");
    expect(wa?.channelCount).toBe(2);
  });

  it("excludes internal pseudo-apps from the app pill/filter list", () => {
    // notifications + secrets are page sections, not connectable apps.
    expect(isConnectableApp("notifications")).toBe(false);
    expect(isConnectableApp("secrets")).toBe(false);
    expect(isConnectableApp("internal")).toBe(false);
    expect(isConnectableApp("slack")).toBe(true);
    expect(isConnectableApp("telegram:alerts")).toBe(true);

    // A pseudo-app source with a matching gateway must be excluded from
    // BOTH the app buckets and the channel list.
    const { apps, channels } = buildHomeModel({
      overview: null,
      sources: [
        ...sources,
        { name: "secrets:vault", description: "", members: [], member_count: 0 },
      ],
      gateways: [
        ...gateways,
        { platform: "notifications", enabled: true, channels: [] },
        { platform: "secrets", enabled: true, channels: [] },
      ],
      labels,
      health: {},
      subs,
      stats,
    });
    const keys = apps.map((a) => a.key);
    expect(keys).toContain("slack");
    expect(keys).not.toContain("notifications");
    expect(keys).not.toContain("secrets");
    // The secrets:vault "channel" never enters the channel list.
    expect(channels.some((c) => c.app === "secrets")).toBe(false);
    expect(channels.some((c) => c.name === "secrets:vault")).toBe(false);
  });

  it("derives deterministic, Unicode-safe initials and legible colours", () => {
    expect(initialsFor("Puneet Rai")).toBe("PR");
    expect(initialsFor("zen-zebra")).toBe("ZZ");
    expect(initialsFor("general")).toBe("GE");
    expect(initialsFor("  ")).toBe("?");
    // Multibyte names select whole code points, never half a glyph.
    // 𝐀𝐁𝐂 are astral (surrogate-pair) letters — a raw slice(0,2) would
    // return a broken half-pair; Array.from keeps them intact.
    expect(initialsFor("𝐀𝐁𝐂")).toBe("𝐀𝐁");
    expect(initialsFor("𝐀lpha 𝐁eta")).toBe("𝐀𝐁");
    expect(initialsFor("日本語")).toBe("日本");
    // Stable across calls, differs by name.
    expect(avatarColor("Puneet Rai")).toBe(avatarColor("Puneet Rai"));
    expect(avatarColor("Puneet Rai")).not.toBe(avatarColor("Someone Else"));
    // Lightness stays dark enough for white text — including the yellow
    // hue band that a naive 45%+ lightness would wash out.
    for (const n of ["a", "b", "yellowish", "Puneet Rai", "zzz"]) {
      const m = /hsl\(\d+ \d+% (\d+)%\)/.exec(avatarColor(n));
      expect(m).not.toBeNull();
      expect(Number(m?.[1])).toBeLessThanOrEqual(42);
    }
  });

  it("IdentityAvatar falls back to initials when the image fails to load", () => {
    const { container } = render(<IdentityAvatar name="Puneet Rai" src="https://example.com/broken.png" />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    // Successful load keeps the <img> — only onError swaps to initials.
    expect(screen.queryByText("PR")).not.toBeInTheDocument();
    fireEvent.error(img as HTMLImageElement);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("PR")).toBeInTheDocument();
  });

  it("IdentityAvatar renders initials directly when no src is given", () => {
    render(<IdentityAvatar name="zen-zebra" />);
    expect(screen.getByText("ZZ")).toBeInTheDocument();
  });

  it("buildHomeModel degrades without overview data", () => {
    const { channels } = buildHomeModel({
      overview: null,
      sources,
      gateways,
      labels,
      health: {},
      subs,
      stats,
    });
    const group = channels.find((c) => c.name === GROUP_CH);
    expect(group?.displayName).toBe("12345-67890@g.us");
    expect(group?.kind).toBe("group"); // JID-shape fallback
    expect(group?.messageCount).toBe(42); // from /stats/channels
    const person = channels.find((c) => c.name === PERSON_CH);
    expect(person?.kind).toBe("person");
  });
});

/* ── Page ────────────────────────────────────────────────────── */

describe("AppsHome", () => {
  it("renders apps strip and channels grouped by app from overview data", async () => {
    mockRoutes();
    renderHome();

    await waitFor(() => {
      expect(screen.getByText("Family Group")).toBeInTheDocument();
    });

    // Apps strip: one compact pill per app. "Connected"/"N channels" text is
    // dropped — the status dot + aria-label carry it. Broken apps show their
    // reason inline and click straight through to reconnect.
    const waPill = screen.getByTestId("app-pill-whatsapp");
    expect(waPill).toHaveAttribute("aria-label", expect.stringContaining("connected"));
    expect(within(waPill).queryByText("Connected")).not.toBeInTheDocument();
    const slackPill = screen.getByTestId("app-pill-slack");
    expect(within(slackPill).getByText("Invalid credentials")).toBeInTheDocument();
    expect(slackPill).toHaveAttribute("aria-label", expect.stringContaining("reconnect"));

    // Connect pill reuses the existing setup flow.
    expect(screen.getByRole("button", { name: /Connect an app/ })).toBeInTheDocument();

    // Channel rows show resolved display names and counts.
    expect(screen.getByText("Puneet Rai")).toBeInTheDocument();
    expect(screen.getByText("general")).toBeInTheDocument();
    expect(screen.getByText("12 members")).toBeInTheDocument();
    expect(screen.getByText("42 msgs")).toBeInTheDocument();
  });

  it("splits WhatsApp channels into Groups and People sections", async () => {
    mockRoutes();
    renderHome();

    await waitFor(() => {
      expect(screen.getByText("Group chats")).toBeInTheDocument();
    });
    expect(screen.getByText("People")).toBeInTheDocument();

    const waSection = screen.getByRole("region", { name: "WhatsApp channels" });
    expect(within(waSection).getByText("Family Group")).toBeInTheDocument();
    expect(within(waSection).getByText("Puneet Rai")).toBeInTheDocument();
    // Slack channels get no Group chats/People split.
    const slackSection = screen.getByRole("region", { name: "Slack channels" });
    expect(within(slackSection).queryByText("Group chats")).not.toBeInTheDocument();
  });

  it("degrades gracefully when the overview endpoint is missing", async () => {
    mockRoutes({ overviewBody: null });
    renderHome();

    // Raw channel ids stand in for display names… (the id also shows in
    // the recent-activity channel chip, hence getAllByText)
    await waitFor(() => {
      expect(screen.getAllByText("12345-67890@g.us").length).toBeGreaterThan(0);
    });
    // …but the Group chats/People split still works via JID shape,
    expect(screen.getByText("Group chats")).toBeInTheDocument();
    expect(screen.getByText("People")).toBeInTheDocument();
    // and message counts still come from the stats endpoint.
    expect(screen.getByText("42 msgs")).toBeInTheDocument();
  });

  it("search in the header filters channels by name", async () => {
    mockRoutes();
    renderHome();
    await waitFor(() => {
      expect(screen.getByText("Family Group")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Search channels"), { target: { value: "family" } });

    expect(screen.getByText("Family Group")).toBeInTheDocument();
    expect(screen.queryByText("Puneet Rai")).not.toBeInTheDocument();
    expect(screen.getByText("1 of 3 channels")).toBeInTheDocument();
  });

  it("Filters chip narrows to channels with subscribers", async () => {
    mockRoutes();
    renderHome();
    await waitFor(() => {
      expect(screen.getByText("Family Group")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    fireEvent.click(screen.getByLabelText("Has subscribers"));

    expect(screen.getByText("Family Group")).toBeInTheDocument();
    expect(screen.queryByText("Puneet Rai")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText("general")).not.toBeInTheDocument();
    });
  });

  it("renders recent activity across channels with click-through chips", async () => {
    mockRoutes();
    renderHome();

    await waitFor(() => {
      expect(screen.getByText(/Dinner at 8/)).toBeInTheDocument();
    });
    // Sender is cleaned of its platform prefix.
    expect(screen.getByText("Mom")).toBeInTheDocument();
  });

  it("links the Notifications column out to the full notifications page", async () => {
    mockRoutes();
    renderHome();
    await waitFor(() => {
      expect(screen.getByText("Family Group")).toBeInTheDocument();
    });
    // The primary column is labelled "Notifications".
    expect(screen.getByRole("complementary", { name: "Notifications" })).toBeInTheDocument();
    const viewAll = screen.getByRole("link", { name: /View all/i });
    expect(viewAll).toHaveAttribute("href", "/apps/activity");
  });
});
