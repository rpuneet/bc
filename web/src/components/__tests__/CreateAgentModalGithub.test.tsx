import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { CreateAgentModal } from "../CreateAgentModal";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

const GH_REPOS = [
  {
    full_name: "acme/mycel",
    name: "mycel",
    clone_url: "https://github.com/acme/mycel.git",
    ssh_url: "git@github.com:acme/mycel.git",
    html_url: "https://github.com/acme/mycel",
    default_branch: "main",
    private: false,
  },
  {
    full_name: "acme/private-notes",
    name: "private-notes",
    clone_url: "https://github.com/acme/private-notes.git",
    ssh_url: "git@github.com:acme/private-notes.git",
    html_url: "https://github.com/acme/private-notes",
    default_branch: "main",
    private: true,
  },
];

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response);
}

function errorResponse(message: string, status: number) {
  return Promise.resolve({
    ok: false,
    status,
    statusText: status === 401 ? "Unauthorized" : "Error",
    json: () => Promise.resolve({ error: message }),
  } as Response);
}

function mockApi(opts: {
  connected?: boolean;
  login?: string;
  repos?: typeof GH_REPOS;
  clonePath?: string;
} = {}) {
  const connected = opts.connected ?? false;
  const login = opts.login ?? "puneet";
  const repos = opts.repos ?? GH_REPOS;
  const clonePath = opts.clonePath ?? "/tmp/src/mycel";

  fetchMock.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = String(url);
    const method = init?.method ?? "GET";
    if (u === "/api/auth/github" && method === "GET") {
      return jsonResponse({ connected, login: connected ? login : undefined });
    }
    if (u === "/api/auth/github" && method === "POST") {
      return jsonResponse({ connected: true, login });
    }
    if (u === "/api/auth/github" && method === "DELETE") {
      return Promise.resolve({
        ok: true,
        status: 204,
        json: () => Promise.reject(new Error("no body")),
      } as unknown as Response);
    }
    if (u === "/api/repos/discover/github") {
      if (!connected && method === "POST") {
        // After a successful Connect POST, subsequent list calls should succeed.
        const connectPosted = fetchMock.mock.calls.some(
          (c) => String(c[0]) === "/api/auth/github" && (c[1] as RequestInit | undefined)?.method === "POST",
        );
        if (!connectPosted) return errorResponse("github not authenticated", 401);
      }
      return jsonResponse({ repos });
    }
    if (u === "/api/repos/clone") {
      return jsonResponse({ path: clonePath, name: "mycel" }, 201);
    }
    if (u === "/api/agents" && method === "POST") {
      return jsonResponse({ name: "bold-otter" });
    }
    if (u.includes("/api/repos")) return jsonResponse({ repos: [], default: "/tmp/repo" });
    return jsonResponse([]);
  });
}

function renderModal() {
  return render(
    <MemoryRouter>
      <CreateAgentModal open onClose={() => undefined} existingNames={[]} />
    </MemoryRouter>,
  );
}

async function openGithubPanel() {
  fireEvent.click(screen.getByRole("button", { name: "GitHub" }));
  return waitFor(() => {
    expect(screen.getByTestId("create-agent-github-panel")).toBeInTheDocument();
  });
}

beforeEach(() => {
  fetchMock.mockReset();
  mockApi();
});

describe("CreateAgentModal GitHub discover", () => {
  it("shows a GitHub button next to Browse", () => {
    renderModal();
    expect(screen.getByRole("button", { name: "GitHub" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Browse" })).toBeInTheDocument();
  });

  it("asks for a token when GitHub is not connected", async () => {
    renderModal();
    await openGithubPanel();
    const panel = screen.getByTestId("create-agent-github-panel");
    expect(within(panel).getByLabelText("GitHub token")).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Connect" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Clone" })).not.toBeInTheDocument();
  });

  it("connects a token, lists repos, clones, and binds the path", async () => {
    renderModal();
    await openGithubPanel();
    const panel = screen.getByTestId("create-agent-github-panel");
    fireEvent.change(within(panel).getByLabelText("GitHub token"), {
      target: { value: "ghp_testtoken" },
    });
    fireEvent.click(within(panel).getByRole("button", { name: "Connect" }));

    await waitFor(() => {
      expect(within(panel).getByText("acme/mycel")).toBeInTheDocument();
    });
    const connect = fetchMock.mock.calls.find(
      (c) => String(c[0]) === "/api/auth/github" && (c[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(connect).toBeDefined();
    expect(JSON.parse(String((connect![1] as RequestInit).body))).toEqual({ token: "ghp_testtoken" });

    fireEvent.click(within(panel).getByText("acme/mycel"));
    fireEvent.change(within(panel).getByLabelText("Clone target directory"), {
      target: { value: "/tmp/src" },
    });
    fireEvent.click(within(panel).getByRole("button", { name: "Clone" }));

    await waitFor(() => {
      expect(screen.queryByTestId("create-agent-github-panel")).not.toBeInTheDocument();
    });
    expect((screen.getByPlaceholderText("/absolute/path/to/repo") as HTMLInputElement).value).toBe(
      "/tmp/src/mycel",
    );

    const clone = fetchMock.mock.calls.find(
      (c) => String(c[0]) === "/api/repos/clone" && (c[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(clone).toBeDefined();
    expect(JSON.parse(String((clone![1] as RequestInit).body))).toEqual({
      url: "https://github.com/acme/mycel.git",
      target: "/tmp/src",
      name: "mycel",
    });

    fireEvent.change(screen.getByPlaceholderText("agent-name"), {
      target: { value: "bold-otter" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        (c) => String(c[0]) === "/api/agents" && (c[1] as RequestInit | undefined)?.method === "POST",
      );
      expect(post).toBeDefined();
      const body = JSON.parse(String((post![1] as RequestInit).body)) as Record<string, unknown>;
      expect(body.repo).toBe("/tmp/src/mycel");
      expect(body.name).toBe("bold-otter");
    });
  });

  it("reuses an existing GitHub token and lists repos without a connect form", async () => {
    mockApi({ connected: true, login: "puneet" });
    renderModal();
    await openGithubPanel();
    const panel = screen.getByTestId("create-agent-github-panel");
    await waitFor(() => {
      expect(within(panel).getByText("Connected as puneet")).toBeInTheDocument();
    });
    expect(within(panel).queryByLabelText("GitHub token")).not.toBeInTheDocument();
    expect(within(panel).getByText("acme/mycel")).toBeInTheDocument();
    expect(within(panel).getByText("acme/private-notes")).toBeInTheDocument();
    expect(within(panel).getByText(/· private/)).toBeInTheDocument();
  });

  it("filters the listed GitHub repos by name", async () => {
    mockApi({ connected: true });
    renderModal();
    await openGithubPanel();
    const panel = screen.getByTestId("create-agent-github-panel");
    await waitFor(() => {
      expect(within(panel).getByText("acme/mycel")).toBeInTheDocument();
    });
    fireEvent.change(within(panel).getByLabelText("Filter GitHub repos"), {
      target: { value: "notes" },
    });
    expect(within(panel).queryByText("acme/mycel")).not.toBeInTheDocument();
    expect(within(panel).getByText("acme/private-notes")).toBeInTheDocument();
  });

  it("keeps Clone disabled until a repo is selected", async () => {
    mockApi({ connected: true });
    renderModal();
    await openGithubPanel();
    const panel = screen.getByTestId("create-agent-github-panel");
    await waitFor(() => {
      expect(within(panel).getByText("acme/mycel")).toBeInTheDocument();
    });
    expect(within(panel).getByRole("button", { name: "Clone" })).toBeDisabled();
    fireEvent.click(within(panel).getByText("acme/mycel"));
    expect(within(panel).getByRole("button", { name: "Clone" })).not.toBeDisabled();
  });
});
