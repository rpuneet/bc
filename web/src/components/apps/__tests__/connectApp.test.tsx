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
    {
      id: "github",
      label: "GitHub",
      auth: "webhook-secret",
      multi: true,
      oauth_available: true,
      fields: [
        { key: "secret", label: "Webhook Secret", placeholder: "your-webhook-secret", secret: true, required: false },
        { key: "oauth_client_id", label: "OAuth Client ID", placeholder: "Ov23li...", secret: false, required: false },
        { key: "api_token", label: "API Token", placeholder: "ghp_...", secret: true, required: false },
      ],
      docs: [],
    },
    {
      id: "gmail",
      label: "Gmail",
      auth: "token",
      multi: false,
      oauth_available: true,
      fields: [
        { key: "client_id", label: "OAuth Client ID", placeholder: "xxxx.apps.googleusercontent.com", secret: true, required: true },
        { key: "client_secret", label: "OAuth Client Secret", placeholder: "GOCSPX-...", secret: true, required: true },
        { key: "refresh_token", label: "OAuth Refresh Token", placeholder: "1//0g...", secret: true, required: true },
      ],
      docs: [],
    },
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

/* ── ConnectWizard — browser sign-in (OAuth device flow) ─────── */

/** Mocks the catalog plus the OAuth begin/poll endpoints for github. */
function mockOAuthEndpoints(opts: {
  begin?: () => Promise<Response>;
  status?: () => Promise<Response>;
} = {}) {
  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    if (u === "/api/apps" && (!init?.method || init.method === "GET")) {
      return jsonResponse(catalog);
    }
    if (u === "/api/apps/github/auth" && init?.method === "POST") {
      return opts.begin
        ? opts.begin()
        : jsonResponse({
            id: "sess-1",
            kind: "device",
            state: "pending",
            verification_url: "https://github.com/login/device",
            user_code: "ABCD-1234",
            interval_seconds: 5,
          });
    }
    if (u.startsWith("/api/apps/github/auth/status?session=")) {
      return opts.status ? opts.status() : jsonResponse({ state: "pending" });
    }
    return jsonResponse([]);
  });
}

describe("ConnectWizard OAuth", () => {
  it("runs the device flow: sign in → user code → poll → agents step", async () => {
    const onConnected = vi.fn();
    mockOAuthEndpoints({ status: () => jsonResponse({ state: "complete" }) });
    render(<ConnectWizard appId="github" onClose={() => undefined} onConnected={onConnected} />);

    await waitFor(() => {
      expect(screen.getByText("Connect GitHub")).toBeInTheDocument();
    });
    // Manual fields remain available, collapsed under an Advanced disclosure.
    expect(screen.getByTestId("manual-config")).toBeInTheDocument();
    expect(screen.getByText("Advanced — configure manually or paste a token")).toBeInTheDocument();

    // Typed plain fields ride along with the begin request.
    fireEvent.change(screen.getByPlaceholderText("Ov23li..."), { target: { value: "Ov23liTEST" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in with GitHub" }));

    // Device-flow UX: the user code plus the verification link.
    await waitFor(() => {
      expect(screen.getByTestId("oauth-user-code")).toHaveTextContent("ABCD-1234");
    });
    const link = screen.getByRole("link", { name: /github.com\/login\/device/ });
    expect(link).toHaveAttribute("href", "https://github.com/login/device");
    expect(screen.getByText("Waiting for authorization...")).toBeInTheDocument();

    const begin = fetchMock.mock.calls.find(
      (c) => String(c[0]) === "/api/apps/github/auth" && (c[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(begin).toBeDefined();
    const body = JSON.parse(String((begin![1] as RequestInit).body)) as { config: Record<string, string> };
    expect(body.config).toEqual({ oauth_client_id: "Ov23liTEST" });

    // First poll (2s) reports complete → wizard advances to agents.
    await waitFor(() => {
      expect(screen.getByText("Add agents to GitHub")).toBeInTheDocument();
    }, { timeout: 4000 });
    expect(onConnected).toHaveBeenCalled();
  }, 10000);

  it("surfaces begin failures with a retry", async () => {
    mockOAuthEndpoints({
      begin: () =>
        Promise.resolve({
          ok: false,
          status: 400,
          statusText: "Bad Request",
          json: () => Promise.resolve({ error: "begin auth: github: oauth_client_id is not set" }),
        } as Response),
    });
    render(<ConnectWizard appId="github" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByText("Connect GitHub")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Sign in with GitHub" }));
    await waitFor(() => {
      expect(screen.getByText(/oauth_client_id is not set/)).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Retry sign in with GitHub" })).toBeInTheDocument();
  });

  it("surfaces poll errors (denied / expired codes)", async () => {
    mockOAuthEndpoints({
      status: () => jsonResponse({ state: "error", error: "access_denied" }),
    });
    render(<ConnectWizard appId="github" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByText("Connect GitHub")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Sign in with GitHub" }));
    await waitFor(() => {
      expect(screen.getByTestId("oauth-user-code")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText("access_denied")).toBeInTheDocument();
    }, { timeout: 4000 });
    expect(screen.getByRole("button", { name: "Retry sign in with GitHub" })).toBeInTheDocument();
  }, 10000);

  it("runs the loopback callback flow: sign in → open consent link → poll → agents", async () => {
    const onConnected = vi.fn();
    fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = String(url);
      if (u === "/api/apps" && (!init?.method || init.method === "GET")) return jsonResponse(catalog);
      if (u === "/api/apps/gmail/auth" && init?.method === "POST") {
        return jsonResponse({
          id: "sess-cb",
          kind: "callback",
          state: "pending",
          auth_url: "https://accounts.google.com/o/oauth2/auth?client_id=x&redirect_uri=http%3A%2F%2F127.0.0.1%3A54321%2Foauth%2Fcallback",
          interval_seconds: 2,
        });
      }
      if (u.startsWith("/api/apps/gmail/auth/status?session=")) return jsonResponse({ state: "complete" });
      return jsonResponse([]);
    });
    render(<ConnectWizard appId="gmail" onClose={() => undefined} onConnected={onConnected} />);

    await waitFor(() => {
      expect(screen.getByText("Connect Gmail")).toBeInTheDocument();
    });
    // Callback flow has no user code — manual fields stay collapsed.
    expect(screen.getByTestId("manual-config")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Sign in with Gmail" }));

    // Callback-flow UX: a link to open the Google consent page (no code).
    await waitFor(() => {
      expect(screen.getByRole("link", { name: /accounts.google.com/ })).toBeInTheDocument();
    });
    expect(screen.queryByTestId("oauth-user-code")).not.toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("Add agents to Gmail")).toBeInTheDocument();
    }, { timeout: 4000 });
    expect(onConnected).toHaveBeenCalled();
  }, 10000);

  it("keeps token apps without oauth_available on the manual path only", async () => {
    mockCatalog();
    render(<ConnectWizard appId="telegram" onClose={() => undefined} onConnected={() => undefined} />);
    await waitFor(() => {
      expect(screen.getByText("Connect Telegram")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("oauth-panel")).not.toBeInTheDocument();
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
