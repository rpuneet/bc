import type { DoctorReport, HealthReport } from "../../api/client";

/* ── Readiness model ──────────────────────────────────────────────────
 *
 * Turns the daemon's raw doctor report (+ health probe) into the grouped,
 * human-readable model the Readiness surface renders. Kept as a pure
 * function so the UI stays a thin projection and the logic is unit-tested
 * directly against mocked /api/doctor payloads.
 *
 * The doctor's "Tools" category is the source of truth: it lists tmux,
 * git, every registered provider CLI, and one `image:mycel-agent-<p>`
 * entry per provider (only emitted when the Docker daemon is reachable —
 * absent entirely when Docker is down). The health probe's degraded map
 * carries the "docker runtime unavailable — agents fall back to tmux"
 * reason when the agent manager had to fall back at boot.
 */

export type RStatus = "ok" | "warn" | "fail";
export type OverallStatus = "ready" | "almost" | "setup";

export interface ReadinessItem {
  key: string;
  label: string;
  detail: string;
  status: RStatus;
  /** Shell command that fixes this item — rendered mono with a copy button. */
  fix?: string;
  /** Extra guidance, e.g. an auth step after install. */
  note?: string;
}

export interface ReadinessGroup {
  id: "runtime" | "git" | "reporting" | "providers";
  title: string;
  status: RStatus;
  summary: string;
  items: ReadinessItem[];
}

export interface Readiness {
  overall: OverallStatus;
  headline: string;
  subline: string;
  groups: ReadinessGroup[];
  /** provider name → installed. Consumed by the New Agent flow. */
  providers: Record<string, boolean>;
  tmuxOk: boolean;
  dockerOk: boolean;
  anyRuntime: boolean;
}

/** Registered providers, in the order the New Agent modal lists them. */
export const PROVIDER_LABELS: Record<string, string> = {
  claude: "Claude Code",
  agy: "Antigravity",
  cursor: "Cursor",
  codex: "Codex",
  pi: "Pi",
};

export const PROVIDER_NAMES = Object.keys(PROVIDER_LABELS);

/** The one-time CLI login each tool needs after install. */
const PROVIDER_SIGNIN: Record<string, string> = {
  claude: "claude",
  agy: "agy",
  cursor: "cursor-agent",
  codex: "codex",
  pi: "pi",
};

function labelFor(name: string): string {
  return PROVIDER_LABELS[name] ?? name;
}

/** Worst severity across a set — fail beats warn beats ok. */
function worst(...s: RStatus[]): RStatus {
  if (s.includes("fail")) return "fail";
  if (s.includes("warn")) return "warn";
  return "ok";
}

export function deriveReadiness(
  report: DoctorReport | null,
  health: HealthReport | null,
): Readiness {
  const tools = report?.categories?.find((c) => c.name === "Tools")?.items ?? [];
  const byName = (n: string) => tools.find((i) => i.name === n);

  // ── Runtime ──────────────────────────────────────────────────────
  const tmuxItem = byName("tmux");
  const tmuxOk = tmuxItem?.severity === "ok";
  // `image:*` items only exist when the Docker daemon answered a list;
  // absent means Docker is unreachable. A degraded.runtime reason means
  // the agent manager already fell back off Docker at boot.
  const imageItems = tools.filter((i) => i.name.startsWith("image:"));
  const runtimeDegraded = health?.degraded?.runtime;
  const dockerOk = imageItems.length > 0 && !runtimeDegraded;
  const anyRuntime = tmuxOk || dockerOk;

  const runtimeItems: ReadinessItem[] = [
    {
      key: "tmux",
      label: "tmux",
      detail: tmuxOk ? (tmuxItem?.message ?? "installed") : "not found",
      status: tmuxOk ? "ok" : "warn",
      fix: tmuxOk ? undefined : (tmuxItem?.fix ?? "brew install tmux  OR  apt install tmux"),
      note: tmuxOk ? undefined : "The default backend — agents run in tmux sessions.",
    },
    {
      key: "docker",
      label: "Docker",
      detail: dockerOk
        ? "running — isolated containers available"
        : runtimeDegraded
          ? runtimeDegraded
          : "not detected",
      status: dockerOk ? "ok" : "warn",
      fix: dockerOk ? undefined : "start Docker Desktop  OR  install docker",
      note: dockerOk
        ? undefined
        : "Optional — enables isolated per-agent containers. Without it agents use tmux.",
    },
  ];

  let runtimeStatus: RStatus;
  let runtimeSummary: string;
  if (!anyRuntime) {
    runtimeStatus = "fail";
    runtimeSummary = "No usable backend — agents cannot start until tmux or Docker is available.";
  } else if (tmuxOk && dockerOk) {
    runtimeStatus = "ok";
    runtimeSummary = "tmux and Docker are both available.";
  } else {
    runtimeStatus = "warn";
    runtimeSummary = tmuxOk
      ? "Running on tmux (the default). Add Docker for isolated containers."
      : "Running on Docker. tmux was not found for the local fallback.";
  }

  const runtime: ReadinessGroup = {
    id: "runtime",
    title: "Runtime",
    status: runtimeStatus,
    summary: runtimeSummary,
    items: runtimeItems,
  };

  // ── git ──────────────────────────────────────────────────────────
  const gitItem = byName("git");
  const gitOk = gitItem?.severity === "ok";
  const git: ReadinessGroup = {
    id: "git",
    title: "Git",
    status: gitOk ? "ok" : "fail",
    summary: gitOk
      ? "git is installed — agent worktrees are ready."
      : "git is required: every agent gets its own git worktree.",
    items: [
      {
        key: "git",
        label: "git",
        detail: gitOk ? (gitItem?.message ?? "installed") : "not found",
        status: gitOk ? "ok" : "fail",
        fix: gitOk ? undefined : (gitItem?.fix ?? "brew install git   OR  apt install git"),
      },
    ],
  };

  // ── Reporting ────────────────────────────────────────────────────
  // jq is how an agent's hooks turn a provider's event into a payload carrying
  // the tool's name, input and result. Without it the reporters fall back to
  // posting the bare event, so agents keep working and the Live feed keeps
  // moving — while showing only that *something* happened, never what. That is
  // a silent partial failure in the one view used to judge whether anything is
  // working, and nothing anywhere said a word about it (#3493).
  // The doctor lists tool-store CLIs under a "cli:" prefix while the runtime
  // and provider checks report bare names; accepting both means this reads the
  // report as it is rather than as one half of it looks.
  const jqItem = byName("cli:jq") ?? byName("jq");
  const jqOk = jqItem?.severity === "ok";
  const reporting: ReadinessGroup = {
    id: "reporting",
    title: "Activity reporting",
    status: jqOk ? "ok" : "warn",
    summary: jqOk
      ? "jq is installed — agents report full tool detail."
      : "Without jq, agents report that events happened but not what they were.",
    items: [
      {
        key: "jq",
        label: "jq",
        detail: jqOk ? (jqItem?.message ?? "installed") : "not found",
        status: jqOk ? "ok" : "warn",
        fix: jqOk ? undefined : (jqItem?.fix ?? "brew install jq   OR  apt install jq"),
        note: jqOk
          ? undefined
          : "Agents still run. Their hooks fall back to posting the bare event, so the Live feed loses tool names, inputs and results.",
      },
    ],
  };

  // ── Providers ────────────────────────────────────────────────────
  const providers: Record<string, boolean> = {};
  const providerItems: ReadinessItem[] = PROVIDER_NAMES.map((name) => {
    const it = byName(name);
    const installed = it?.severity === "ok";
    providers[name] = installed;
    const signin = PROVIDER_SIGNIN[name] ?? name;
    return {
      key: name,
      label: labelFor(name),
      detail: installed ? (it?.message ?? "installed") : "not installed",
      status: installed ? "ok" : "warn",
      fix: installed ? undefined : it?.fix,
      // Only the actionable case carries a note — installed tools stay quiet
      // so the ready state doesn't drown in repeated sign-in reminders.
      note: installed ? undefined : `After installing, run \`${signin}\` once to sign in.`,
    };
  });
  const installedCount = Object.values(providers).filter(Boolean).length;

  const providerGroup: ReadinessGroup = {
    id: "providers",
    title: "Agent tools",
    status: installedCount >= 1 ? "ok" : "fail",
    summary:
      installedCount >= 1
        ? `You have ${installedCount} agent tool${installedCount === 1 ? "" : "s"} ready. Install more to run agents on other models.`
        : "Install at least one agent CLI to get started — Claude Code is the default.",
    items: providerItems,
  };

  // ── Overall ──────────────────────────────────────────────────────
  const essentialsMet = anyRuntime && gitOk && installedCount >= 1;
  let overall: OverallStatus;
  let headline: string;
  let subline: string;

  if (!anyRuntime || !gitOk) {
    overall = "setup";
    headline = "Setup needed";
    subline = !anyRuntime
      ? "No runtime backend is available — install tmux (or start Docker) so agents can run."
      : "git is missing — it's required for agent worktrees.";
  } else if (installedCount === 0) {
    overall = "almost";
    headline = "Almost there — install an agent tool";
    subline = "Your machine can run agents; you just need one agent CLI. Claude Code is the default.";
  } else {
    overall = "ready";
    headline = "Ready to run agents";
    // Honest subline: reflect the worst *item* below, not just group rollups —
    // an optional missing tool (amber row) shouldn't read as "all clear".
    const allItems = [runtime, git, reporting, providerGroup].flatMap((g) => g.items.map((i) => i.status));
    subline = worst(...allItems) === "ok"
      ? "Everything checks out."
      : "Essentials are in place. A few optional extras below could smooth things out.";
  }
  // essentialsMet is implied by the branches above; keep the name meaningful.
  void essentialsMet;

  return {
    overall,
    headline,
    subline,
    groups: [runtime, git, reporting, providerGroup],
    providers,
    tmuxOk,
    dockerOk,
    anyRuntime,
  };
}
