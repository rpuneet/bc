/**
 * ConnectApp tests — the catalog-driven connect flow.
 *
 * The chooser and wizard are driven entirely by GET /api/apps
 * descriptors; these tests mock that endpoint and assert the catalog
 * renders from it, the wizard posts to POST /api/apps/{name} with the
 * entered values (the secret/plain split is server-side), multi apps
 * target labeled instance names, and stored secrets are replace-only.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AppChooser, ConnectWizard, sanitizeInstanceLabel } from "../ConnectApp";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

const catalog = {
  catalog: [
    {
      id: "slack",
      label: "Slack",
      auth: "token",
      multi: false,
      fields: [
        { key: "bot_token", label: "Bot Token", placeholder: "xoxb-...", secret: true, required: true },
        { key: "app_token", label: "App Token", placeholder: "xapp-...", secret: true, required: false },
        { key: "mode", label: "Mode", placeholder: "socket", secret: false, required: false },
      ],
      docs: ["Create a Slack app → https://api.slack.com/apps"],
    },
    {
      id: "telegram",
      label: "Telegram",
      auth: "token",
      multi: true,
      fields: [
        { key: "bot_token", label: "Bot Token", placeholder: "1234567890:AAH...", secret: true, required: true },
      ],
      docs: [],
    },
    { id: "whatsapp", label: "WhatsApp", auth: "qr", multi: false, fields: [], docs: ["Scan the QR"] },
  ],
  instances: [
    { name: "slack", app: "slack", enabled: true, connected: true, config: { has_bot_token: true, has_app_token: false, mode: "socket" }, channels: [] },
  ],
};

function mockCatalog(overrides: { post?: () => Promise<Response> } = {}) {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u === "/api/apps" && (!init?.method || init.method === "GET")) {
      return jsonResponse(catalog);
    }
    if (init?.method === "POST") {
      return overrides.post
        ? overrides.post()
        : jsonResponse({ status: "updated", name: "x", app: "x", enabled: true });
    }
    return jsonResponse([]);
  });
}

beforeEach(() => {
  fetchMock.mockReset();
});

/* ── AppChooser — catalog rendering ──────────────────────────── */

describe("AppChooser", () => {
  it("renders one card per backend descriptor, nothing else", async () => {
    mockCatalog();
    render(<AppChooser onSelect={() => undefined} onClose={() => undefined} />);

    await waitFor(() => {
      expect(screen.getByTestId("app-card-slack")).toBeInTheDocument();
    });
    expect(screen.getByTestId("app-card-telegram")).toBeInTheDocument();
    expect(screen.getByTestId("app-card-whatsapp")).toBeInTheDocument();
    // Apps without a backend descriptor do not render, even if the
    // presentation map knows them (e.g. discord).
    expect(screen.queryByTestId("app-card-discord")).not.toBeInTheDocument();
  });

  it("splits connected instances into the Connected section with auth hints elsewhere", async () => {
    mockCatalog();
    render(<AppChooser onSelect={() => undefined} onClose={() => undefined} />);

    await waitFor(() => {
      expect(screen.getByTestId("app-card-slack")).toBeInTheDocument();
    });
    // slack has an instance → Connected section header + card badge.
    expect(screen.getAllByText("Connected").length).toBeGreaterThanOrEqual(2);
    // qr apps advertise their auth kind.
    expect(screen.getByText("Scan QR to pair")).toBeInTheDocument();
  });

  it("selecting a card hands back the app id", async () => {
    const onSelect = vi.fn();
    mockCatalog();
    render(<AppChooser onSelect={onSelect} onClose={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByTestId("app-card-telegram")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByTestId("app-card-telegram"));
    expect(onSelect).toHaveBeenCalledWith("telegram");
  });
});

/* ── ConnectWizard — descriptor-driven form + POST shape ─────── */

describe("ConnectWizard", () => {
  it("renders fields and docs from the descriptor and posts values to /api/apps/{name}", async () => {
    mockCatalog();
    render(<ConnectWizard appId="telegram" onClose={() => undefined} onConnected={() => undefined} />);

    await waitFor(() => {
      expect(screen.getByText("Connect Telegram")).toBeInTheDocument();
    });

    // Secret fields render as password inputs.
    const token = screen.getByPlaceholderText("1234567890:AAH...");
    expect(token).toHaveAttribute("type", "password");

    fireEvent.change(token, { target: { value: "12345:token-value" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) => String(c[0]) === "/api/apps/telegram" && (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeDefined();
      const body = JSON.parse(String((post![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body.app).toBe("telegram");
      expect(body.enabled).toBe(true);
      // The client posts raw values — the secret/plain split happens
      // server-side against the descriptor.
      expect(body.config).toEqual({ bot_token: "12345:token-value" });
    });
  });

  it("multi apps post to the labeled instance name", async () => {
    mockCatalog();
    render(<ConnectWizard appId="telegram" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByTestId("instance-label-input")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByTestId("instance-label-input"), { target: { value: "Alerts Bot" } });
    fireEvent.change(screen.getByPlaceholderText("1234567890:AAH..."), { target: { value: "t0k3n" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) => String(c[0]) === "/api/apps/telegram%3Aalerts-bot" && (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeDefined();
    });
  });

  it("required check on missing secret blocks the POST", async () => {
    mockCatalog();
    render(<ConnectWizard appId="telegram" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByText("Connect Telegram")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => {
      expect(screen.getByText("Bot Token is required")).toBeInTheDocument();
    });
    const post = fetchMock.mock.calls.find((c) => (c[1] as RequestInit | undefined)?.method === "POST");
    expect(post).toBeUndefined();
  });

  it("connected instances show configured secrets as replace-only and keep them when blank", async () => {
    mockCatalog();
    render(<ConnectWizard appId="slack" onClose={() => undefined} onConnected={() => undefined} />);

    await waitFor(() => {
      expect(screen.getByText("Connect Slack")).toBeInTheDocument();
    });

    // has_bot_token=true → the field reads as configured, never echoing
    // the value, and the placeholder explains replace-only semantics.
    expect(screen.getByText("configured")).toBeInTheDocument();
    const botToken = screen.getByPlaceholderText("•••••• — leave blank to keep");
    expect(botToken).toHaveAttribute("type", "password");
    expect((botToken as HTMLInputElement).value).toBe("");

    // Update: existing instance → "Save & reconnect"; blank required
    // secret passes validation because the vault already holds it.
    fireEvent.change(screen.getByPlaceholderText("socket"), { target: { value: "socket" } });
    fireEvent.click(screen.getByRole("button", { name: "Save & reconnect" }));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) => String(c[0]) === "/api/apps/slack" && (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeDefined();
      const body = JSON.parse(String((post![1] as RequestInit).body)) as { config: Record<string, string> };
      // Blank secrets are omitted — the stored value stays.
      expect(body.config).toEqual({ mode: "socket" });
    });
  });

  it("unknown app ids fall to an explicit error state", async () => {
    mockCatalog();
    render(<ConnectWizard appId="nope" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByText("Unknown app: nope")).toBeInTheDocument();
    });
  });
});

describe("sanitizeInstanceLabel", () => {
  it("normalizes labels to id-safe segments", () => {
    expect(sanitizeInstanceLabel("Alerts Bot")).toBe("alerts-bot");
    expect(sanitizeInstanceLabel("  trade_research  ")).toBe("trade_research");
    expect(sanitizeInstanceLabel("--x--")).toBe("x");
    expect(sanitizeInstanceLabel("")).toBe("");
  });
});
