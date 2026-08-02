import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import { api } from "../api/client";
import type { Tool } from "../api/client";
import { installDep } from "../wizard/installStream";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { ProvidersTable } from "../components/ProvidersTable";
import { ProviderDefaults } from "../components/ProviderDefaults";
import { PackageManagers } from "../components/PackageManagers";
import { RegistrySearch } from "../components/RegistrySearch";
import { DependenciesSection } from "../components/settings/Dependencies";
import { CopyButton } from "../components/CopyButton";
import { ToastContainer, useToast } from "../components/Toast";
import type { ToastLevel } from "../components/Toast";

/* ── ProvidersToolsSection ─────────────────────────────────────────────
 *
 * The Settings "Providers & Tools" section — list-only. It folds in what
 * used to be the standalone /tools page: fleet defaults, a providers table,
 * and CLI dependency management. There is no card/grid mode anywhere here —
 * table/list only, and a row click drills into /settings/providers/:name for
 * the full per-provider manager (install/update, config, MCP servers,
 * commands, cost breakdown, agents).
 *
 * Both lists (providers, CLI tools) load in parallel via independent
 * usePolling calls; each keeps its last-known-good data on every background
 * refresh (usePolling never clears `data` while re-fetching), so periodic
 * polling never flashes a skeleton over content that's already on screen.
 */

const STATUS_CONFIG: Record<string, { dot: string; label: string; textColor: string }> = {
  connected:     { dot: "bg-mycel-success", label: "Connected",     textColor: "text-mycel-success" },
  configured:    { dot: "bg-mycel-success", label: "Configured",    textColor: "text-mycel-success" },
  installed:     { dot: "bg-mycel-success", label: "Installed",     textColor: "text-mycel-success" },
  disabled:      { dot: "bg-mycel-muted",   label: "Disabled",      textColor: "text-mycel-muted" },
  not_installed: { dot: "bg-mycel-error",   label: "Not installed", textColor: "text-mycel-error" },
  error:         { dot: "bg-mycel-error",   label: "Error",         textColor: "text-mycel-error" },
  unknown:       { dot: "bg-mycel-muted",   label: "Unknown",       textColor: "text-mycel-muted" },
};

const inputCls = "w-full px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent";

function getStatusConfig(s: string) { return STATUS_CONFIG[s] ?? STATUS_CONFIG.unknown!; }

/* ── Animated chevron icon ── */
function ChevronIcon({ expanded }: { expanded: boolean }) {
  return (
    <motion.svg
      animate={{ rotate: expanded ? 90 : 0 }}
      transition={{ type: "spring", stiffness: 300, damping: 20 }}
      className="w-3 h-3 text-mycel-muted"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeWidth={2.5}
    >
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
    </motion.svg>
  );
}

/* ── In-surface streamed command ──────────────────────────────────────
 *
 * Streams a CLI tool's install / update / uninstall command over POST
 * /api/deps/install — the same loopback-guarded NDJSON stream the setup
 * wizard uses — into a live console with an honest running → success/error
 * progression. The stream carries no percentage, so progress is an
 * indeterminate bar while running and a resolved (green/red) bar on
 * completion; the line count is the concrete "how far along" signal. */
type RunState = "idle" | "running" | "ok" | "error";
type RunMode = "install" | "update" | "uninstall";

const RUN_VERBS: Record<RunMode, { idle: string; gerund: string; done: string }> = {
  install: { idle: "Install", gerund: "Installing", done: "Installed" },
  update: { idle: "Update", gerund: "Updating", done: "Updated" },
  uninstall: { idle: "Uninstall", gerund: "Uninstalling", done: "Uninstalled" },
};

/* A single streamed action button + live console. Reused for install,
 * update, and uninstall so all three share one honest state machine. */
function StreamedAction({
  toolName,
  mode,
  canRun,
  disabledHint,
  destructive = false,
  onDone,
}: {
  toolName: string;
  mode: RunMode;
  canRun: boolean;
  disabledHint: string;
  destructive?: boolean;
  onDone: () => void;
}) {
  const reduceMotion = useReducedMotion();
  const [state, setState] = useState<RunState>("idle");
  const [lines, setLines] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const consoleRef = useRef<HTMLDivElement>(null);
  const runningRef = useRef(false);
  const verbs = RUN_VERBS[mode];

  useEffect(() => {
    const el = consoleRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  // Guard against setState after unmount when a slow stream resolves late.
  useEffect(() => () => { runningRef.current = false; }, []);

  const run = async () => {
    setState("running");
    setLines([]);
    setErr(null);
    runningRef.current = true;
    try {
      const code = await installDep(
        toolName,
        (ev) => {
          if (!runningRef.current) return;
          if (ev.type === "start") setLines((l) => [...l, `$ ${ev.command}`]);
          else if (ev.type === "log") setLines((l) => [...l, ev.line]);
        },
        { mode },
      );
      if (!runningRef.current) return;
      if (code === 0) {
        setState("ok");
        onDone();
      } else {
        setState("error");
        setErr(`${verbs.idle} exited with code ${code}.`);
      }
    } catch (e) {
      if (!runningRef.current) return;
      setState("error");
      setErr(e instanceof Error ? e.message : `${mode} failed.`);
    } finally {
      runningRef.current = false;
    }
  };

  const barTone = state === "ok" ? "bg-mycel-success" : state === "error" ? "bg-mycel-error" : destructive ? "bg-mycel-error" : "bg-mycel-accent";
  const btnCls = destructive
    ? "inline-flex items-center gap-1.5 text-[11px] font-medium px-2.5 py-1 rounded-md border border-mycel-error text-mycel-error hover:bg-mycel-error hover:text-white transition-colors disabled:opacity-60 focus-visible:ring-2 focus-visible:ring-mycel-error"
    : "inline-flex items-center gap-1.5 text-[11px] font-medium px-2.5 py-1 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-60 focus-visible:ring-2 focus-visible:ring-mycel-accent";

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        {canRun ? (
          <button type="button" onClick={() => void run()} disabled={state === "running"} className={btnCls}>
            {state === "running" && <Spinner />}
            {state === "running"
              ? `${verbs.gerund}…`
              : state === "ok"
                ? `${verbs.idle} again`
                : state === "error"
                  ? `Retry ${verbs.idle.toLowerCase()}`
                  : verbs.idle}
          </button>
        ) : (
          <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted">
            <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
              <circle cx="7" cy="7" r="5.5" />
              <path d="M7 4.5v.01M6 6.5h1v3h1" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            {disabledHint}
          </span>
        )}
        {state === "ok" && (
          <span className="inline-flex items-center gap-1 text-[11px] text-mycel-success" role="status">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M20 6L9 17l-5-5" />
            </svg>
            {verbs.done}. Re-checking…
          </span>
        )}
        {state === "error" && err && (
          <span className="inline-flex items-center gap-1 text-[11px] text-mycel-error" role="alert">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M12 8v4M12 16h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" />
            </svg>
            <span className="truncate max-w-xs">{err}</span>
          </span>
        )}
      </div>

      {(state === "running" || state === "ok" || state === "error") && (
        <div className="space-y-1.5">
          {/* Progress: indeterminate while running (honest — the stream has no
              percent), resolved to a full green/red bar on completion. */}
          <div className="h-1 rounded-full bg-mycel-border overflow-hidden" role="progressbar" aria-label={`${verbs.idle} progress`} aria-busy={state === "running"}>
            {state === "running" ? (
              <div className={`h-full w-1/3 rounded-full ${barTone} ${reduceMotion ? "" : "animate-indeterminate"}`} />
            ) : (
              <div className={`h-full w-full rounded-full ${barTone}`} />
            )}
          </div>
          {lines.length > 0 && (
            <div
              ref={consoleRef}
              className="max-h-40 overflow-auto rounded-md border border-mycel-border bg-mycel-bg px-2.5 py-1.5 font-mono text-[11px] leading-relaxed text-mycel-text-2 whitespace-pre-wrap"
            >
              {lines.map((l, i) => (
                <div key={i} className={l.startsWith("$ ") ? "text-mycel-accent" : ""}>
                  {l}
                </div>
              ))}
              {state === "running" && <div className="text-mycel-muted">▍</div>}
            </div>
          )}
          <div className="text-[10.5px] text-mycel-muted tabular-nums">
            {`${lines.length} line${lines.length === 1 ? "" : "s"}${state === "running" ? "…" : ""}`}
          </div>
        </div>
      )}
    </div>
  );
}

/* Human name for an inferred package manager id. The API sends the id only —
 * how to render it is presentation, and belongs here. */
const MANAGER_LABEL: Record<string, string> = {
  brew: "Homebrew",
  npm: "npm (global)",
  pnpm: "pnpm (global)",
  yarn: "Yarn (global)",
  cargo: "cargo",
  pipx: "pipx",
  pip: "pip",
  uv: "uv",
  gem: "RubyGems",
  apt: "apt",
  dnf: "dnf",
  yum: "yum",
  pacman: "pacman",
  zypper: "zypper",
  winget: "winget",
  scoop: "Scoop",
  choco: "Chocolatey",
  nvm: "nvm (Node)",
  volta: "Volta",
  pyenv: "pyenv",
  rbenv: "rbenv",
  system: "Your OS",
};

function managerLabel(id: string | undefined): string {
  if (!id) return "Unknown";
  return MANAGER_LABEL[id] ?? id;
}

/* How each manager updates one of its packages. Used to name a command when the
 * tool has no updater configured — more use than telling the user to copy a
 * command that isn't on screen. Shown to copy, never run: the package name is
 * assumed to equal the tool name, which is right often but not always, and
 * that's not a good enough guess to execute on someone's behalf. */
const MANAGER_UPDATE: Record<string, (name: string) => string> = {
  brew: (n) => `brew upgrade ${n}`,
  npm: (n) => `npm install -g ${n}@latest`,
  pnpm: (n) => `pnpm add -g ${n}@latest`,
  yarn: (n) => `yarn global upgrade ${n}`,
  cargo: (n) => `cargo install ${n} --force`,
  pipx: (n) => `pipx upgrade ${n}`,
  pip: (n) => `pip install --upgrade ${n}`,
  uv: (n) => `uv tool upgrade ${n}`,
  gem: (n) => `gem update ${n}`,
  apt: (n) => `sudo apt-get install --only-upgrade ${n}`,
  dnf: (n) => `sudo dnf upgrade ${n}`,
  yum: (n) => `sudo yum update ${n}`,
  pacman: (n) => `sudo pacman -S ${n}`,
  zypper: (n) => `sudo zypper update ${n}`,
  winget: (n) => `winget upgrade ${n}`,
  scoop: (n) => `scoop update ${n}`,
  choco: (n) => `choco upgrade ${n}`,
};

/** The update command to show for a tool: the configured one if there is one,
 *  else the one its owning manager would use. Empty when neither is known. */
function updateCommandFor(tool: Tool): string {
  if (tool.upgrade_cmd) return tool.upgrade_cmd;
  const recipe = tool.manager ? MANAGER_UPDATE[tool.manager] : undefined;
  return recipe ? recipe(tool.name) : "";
}

/** Why the streamed action isn't available, said in terms of this tool. The old
 *  copy pointed at "the command above" even when no command was displayed. */
function actionHint(tool: Tool, mode: RunMode): string {
  if (mode === "install") {
    return "No install command configured — install it with your package manager, then re-check.";
  }
  if (tool.manager === "system") {
    return "Provided by your OS — update it through the system, not from here.";
  }
  return updateCommandFor(tool)
    ? "No automatic updater — copy the Update command below to run it yourself."
    : "No automatic updater, and no package manager owns this binary.";
}

/* Install-or-update for a CLI tool, plus an uninstall path for installed,
 * non-required tools. Mode is chosen from the tool's current status. */
function CLIInstallAction({ tool, onDone }: { tool: Tool; onDone: () => void }) {
  const isInstalled = tool.status !== "not_installed";
  const mode: RunMode = isInstalled ? "update" : "install";
  const canRun = mode === "install"
    ? Boolean(tool.install_cmd)
    : Boolean(tool.upgrade_cmd || tool.install_cmd);

  return (
    <div className="space-y-2">
      <StreamedAction
        toolName={tool.name}
        mode={mode}
        canRun={canRun}
        disabledHint={actionHint(tool, mode)}
        onDone={onDone}
      />
      {/* Uninstall the binary — distinct from "Remove" (which only forgets the
          registry entry). Offered only for installed, non-required tools; the
          backend refuses core system deps and returns an honest error.
          Also withheld for an OS-provided binary: /usr/bin/git is not mycel's
          to delete, so offering the button could only ever produce that error. */}
      {isInstalled && !tool.required && tool.manager !== "system" && (
        <StreamedAction
          toolName={tool.name}
          mode="uninstall"
          canRun
          disabledHint=""
          destructive
          onDone={onDone}
        />
      )}
    </div>
  );
}

/* One derived fact about a tool. Deliberately plain text rather than the
 * bordered box this used to be: these are read-only facts, and the boxed
 * styling made them read as editable fields the user was meant to fill in. */
function Fact({ label, value, copy, mono = true }: {
  label: string;
  value: string;
  copy?: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-2">
      <dt className="text-[11px] text-mycel-muted w-[86px] shrink-0">{label}</dt>
      <dd className={`text-[11px] text-mycel-text-2 min-w-0 truncate ${mono ? "font-mono" : ""}`} title={value}>
        {value}
      </dd>
      {copy && <CopyButton text={copy} />}
    </div>
  );
}

/* What mycel knows about an installed CLI tool, all of it derived — nothing
 * here is typed in. Notably this shows the real resolved path: the previous
 * version labelled the bare command name ("git") as the Path, which told the
 * user nothing they hadn't already typed. */
function ToolFacts({ tool }: { tool: Tool }) {
  const updateCmd = updateCommandFor(tool);

  return (
    <dl className="space-y-1">
      <Fact
        label="Path"
        value={tool.path ?? "Not found on PATH"}
        copy={tool.path}
        mono={Boolean(tool.path)}
      />
      <Fact label="Managed by" value={managerLabel(tool.manager)} mono={false} />
      {tool.version && <Fact label="Version" value={tool.version} />}
      {tool.install_cmd && <Fact label="Install" value={tool.install_cmd} copy={tool.install_cmd} />}
      {updateCmd && <Fact label="Update" value={updateCmd} copy={updateCmd} />}
    </dl>
  );
}

function CLIDepsRow({ tool, onToggle, onRemove, toggling, removing, expanded, onExpand, onChanged }: {
  tool: Tool; onToggle: () => void; onRemove: () => void;
  toggling: boolean; removing: boolean; expanded: boolean; onExpand: () => void;
  onChanged: () => void;
}) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const cfg = getStatusConfig(tool.status);
  const isDisabled = tool.status === "disabled";
  const isNotInstalled = tool.status === "not_installed";

  return (
    <>
      <tr
        className="border-b border-mycel-border hover:bg-mycel-surface-hover cursor-pointer transition-colors"
        onClick={onExpand}
      >
        <td className="px-4 py-2.5 text-sm">
          <div className="flex items-center gap-2">
            <ChevronIcon expanded={expanded} />
            <span className="font-medium">{tool.name}</span>
          </div>
        </td>
        <td className="px-4 py-2.5 text-sm">
          <span className="inline-flex items-center gap-1.5">
            <span className={`w-2 h-2 rounded-full ${cfg.dot}`} />
            {/* The status label, not the version — Version is its own column
                right beside this one. */}
            <span className={`text-xs ${cfg.textColor}`}>{cfg.label}</span>
          </span>
        </td>
        <td className="px-4 py-2.5 text-xs text-mycel-muted font-mono max-w-[180px] truncate" title={tool.version || ""}>{tool.version || "—"}</td>
        <td className="px-4 py-2.5 text-xs">
          {tool.required ? (
            <span className="px-1.5 py-0.5 rounded-md bg-mycel-accent-subtle text-mycel-accent text-[10px] font-medium">Yes</span>
          ) : (
            <span className="text-mycel-muted">No</span>
          )}
        </td>
        <td className="px-4 py-2.5 text-right" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center justify-end gap-1.5">
            <button type="button" onClick={onToggle} disabled={toggling}
              role="switch" aria-checked={!isDisabled && !isNotInstalled}
              aria-label={isNotInstalled ? `Install and enable ${tool.name}` : isDisabled ? `Enable ${tool.name}` : `Disable ${tool.name}`}
              title={isNotInstalled ? "Not installed — installs the binary, then enables it" : undefined}
              className={`text-[11px] px-2 py-0.5 rounded-md transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent disabled:opacity-50 ${isDisabled || isNotInstalled ? "bg-mycel-surface-hover text-mycel-text-2 hover:bg-mycel-border" : "bg-mycel-success-subtle text-mycel-success"}`}>
              {toggling ? "..." : isNotInstalled ? "Install" : isDisabled ? "Enable" : "Disable"}
            </button>
            {!tool.required && (
              confirmRemove ? (
                <span className="inline-flex items-center gap-1">
                  <button type="button" onClick={() => { onRemove(); setConfirmRemove(false); }} disabled={removing}
                    className="text-[11px] px-2 py-0.5 rounded-md bg-mycel-error-subtle text-mycel-error hover:bg-mycel-error hover:text-white disabled:opacity-50" aria-label="Confirm remove">
                    {removing ? "..." : "Yes"}
                  </button>
                  <button type="button" onClick={() => setConfirmRemove(false)} disabled={removing}
                    className="text-[11px] px-2 py-0.5 rounded-md border border-mycel-border text-mycel-muted" aria-label="Cancel remove">No</button>
                </span>
              ) : (
                <button type="button" onClick={() => setConfirmRemove(true)}
                  className="text-[11px] px-2 py-0.5 rounded-md border border-mycel-border text-mycel-error hover:bg-mycel-error-subtle hover:border-mycel-error transition-colors"
                  aria-label={`Remove ${tool.name}`}>Remove</button>
              )
            )}
          </div>
        </td>
      </tr>
      <AnimatePresence>
        {expanded && (
          <tr className="border-b border-mycel-border">
            <td colSpan={5} className="p-0">
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.2, ease: "easeOut" }}
                className="overflow-hidden bg-mycel-surface"
              >
                <div className="px-8 py-3 space-y-3">
                  {/* In-surface install / update — streamed, loopback-guarded. */}
                  <CLIInstallAction tool={tool} onDone={onChanged} />
                  <ToolFacts tool={tool} />
                  {tool.error && (
                    <div className="text-xs text-mycel-error bg-mycel-error-subtle rounded-md px-2 py-1 border border-mycel-error">
                      Error: {tool.error}
                    </div>
                  )}
                </div>
              </motion.div>
            </td>
          </tr>
        )}
      </AnimatePresence>
    </>
  );
}

function AddCLIToolForm({ onClose, onAdded, onToast }: { onClose: () => void; onAdded: () => void; onToast: (level: ToastLevel, text: string) => void }) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [installCmd, setInstallCmd] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await api.createTool({ name: name.trim(), command: command.trim(), install_cmd: installCmd.trim(), enabled: true });
      onToast("info", `Tool '${name.trim()}' added`);
      onAdded();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to add tool";
      setError(msg);
      onToast("error", msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: -8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8 }}
      transition={{ duration: 0.2 }}
      className="rounded-lg border border-mycel-accent bg-mycel-surface p-4 space-y-3"
    >
      <div className="flex items-center justify-between">
        <h3 className="text-base font-semibold">Add CLI Tool</h3>
        <button type="button" onClick={onClose} className="text-xs text-mycel-muted hover:text-mycel-text">Cancel</button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <label className="text-sm text-mycel-text block mb-1">Name</label>
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="gh" className={inputCls} />
        </div>
        <div>
          <label className="text-sm text-mycel-text block mb-1">Command</label>
          <input type="text" value={command} onChange={(e) => setCommand(e.target.value)} placeholder="gh" className={inputCls} />
        </div>
        <div className="md:col-span-2">
          <label className="text-sm text-mycel-text block mb-1">Install Command (optional)</label>
          <input type="text" value={installCmd} onChange={(e) => setInstallCmd(e.target.value)}
            placeholder="apt-get install -y gh" className={inputCls} />
        </div>
      </div>
      {error && <p className="text-xs text-mycel-error">{error}</p>}
      <button type="button" onClick={() => void handleSubmit()} disabled={submitting || !name.trim()}
        className="inline-flex items-center h-9 px-3 text-sm rounded-md bg-mycel-accent text-mycel-accent-fg font-medium hover:bg-mycel-accent-hover shadow-mycel-sm disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent">
        {submitting ? "Adding..." : "Add CLI Tool"}
      </button>
    </motion.div>
  );
}

/* ── Spinner icon ── */
function Spinner() {
  return (
    <svg className="animate-spin w-4 h-4 text-mycel-muted" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  );
}

export function ProvidersToolsSection() {
  const providerFetcher = useCallback(() => api.listProviders(), []);
  const { data: providers, loading: providersLoading } = usePolling(providerFetcher, 10000);

  const fetcher = useCallback(() => api.listTools(), []);
  const { data: tools, loading: toolsLoading, error, refresh, timedOut } = usePolling(fetcher, 10000);
  const [showAddForm, setShowAddForm] = useState(false);
  const [checking, setChecking] = useState(false);
  const [checkedTools, setCheckedTools] = useState<Tool[] | null>(null);
  const [optimisticToggles, setOptimisticToggles] = useState<Map<string, string>>(new Map());
  const [togglingSet, setTogglingSet] = useState<Set<string>>(new Set());
  const [removingSet, setRemovingSet] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const { toasts, addToast, dismiss } = useToast();

  const handleCheck = async () => {
    setChecking(true);
    try {
      const checked = await api.checkTools();
      const checkMap = new Map(checked.map((t) => [t.name, t]));
      setCheckedTools((tools ?? []).map((t) => {
        const c = checkMap.get(t.name);
        if (c) {
          return {
            ...t,
            status: c.status,
            version: c.version ?? t.version,
            command: c.command ?? t.command,
            // A re-check re-resolves both of these — a tool just installed or
            // removed changes path and owner, so take the fresh answer.
            path: c.path ?? t.path,
            manager: c.manager ?? t.manager,
            error: c.error ?? t.error,
          };
        }
        return t;
      }));
      addToast("success", "Health check complete");
    } catch {
      addToast("error", "Health check failed");
    } finally {
      setChecking(false);
    }
  };

  const allTools = useMemo(() => {
    const source = checkedTools ?? tools ?? [];
    const seen = new Set<string>();
    const deduped: Tool[] = [];
    for (const t of source) {
      if (seen.has(t.name)) continue;
      seen.add(t.name);
      const optimistic = optimisticToggles.get(t.name);
      deduped.push(optimistic ? { ...t, status: optimistic } : t);
    }
    return deduped;
  }, [checkedTools, tools, optimisticToggles]);

  const searchLower = useMemo(() => search.toLowerCase().trim(), [search]);

  const { cliTools, filteredCli } = useMemo(() => {
    const matchesSearch = (t: Tool) => !searchLower || t.name.toLowerCase().includes(searchLower);
    const cli = allTools.filter((t) => t.type !== "provider" && t.type !== "mcp");
    return {
      cliTools: cli,
      filteredCli: cli.filter(matchesSearch),
    };
  }, [allTools, searchLower]);

  const providerList = providers ?? [];

  const handleToggle = async (tool: Tool) => {
    // A not-installed tool has nothing to "enable" — flipping the DB flag
    // would just lie about its state. Route through the real streamed
    // installer first (same mechanism CLIInstallAction uses), then enable
    // once the binary actually lands.
    if (tool.status === "not_installed") {
      setTogglingSet((prev) => new Set(prev).add(tool.name));
      try {
        const code = await installDep(tool.name, () => {}, { mode: "install" });
        if (code !== 0) {
          addToast("error", `Install failed (exit ${code}) — ${tool.name} was not enabled`);
          return;
        }
        await api.enableTool(tool.name);
        addToast("success", `${tool.name} installed and enabled`);
        setCheckedTools(null);
        refresh();
      } catch (err) {
        const msg = err instanceof Error ? err.message : `Failed to install ${tool.name}`;
        addToast("error", msg);
      } finally {
        setTogglingSet((prev) => { const next = new Set(prev); next.delete(tool.name); return next; });
      }
      return;
    }

    const wasDisabled = tool.status === "disabled";
    const newStatus = "installed";
    const oldStatus = tool.status;

    setOptimisticToggles((prev) => new Map(prev).set(tool.name, newStatus));
    setTogglingSet((prev) => new Set(prev).add(tool.name));

    try {
      if (wasDisabled) {
        await api.enableTool(tool.name);
      } else {
        await api.disableTool(tool.name);
      }
      addToast("success", `${tool.name} ${wasDisabled ? "enabled" : "disabled"}`);
      setCheckedTools(null);
      refresh();
    } catch (err) {
      setOptimisticToggles((prev) => new Map(prev).set(tool.name, oldStatus));
      const msg = err instanceof Error ? err.message : `Failed to toggle ${tool.name}`;
      addToast("error", msg);
    } finally {
      setTogglingSet((prev) => { const next = new Set(prev); next.delete(tool.name); return next; });
      setTimeout(() => {
        setOptimisticToggles((prev) => {
          const next = new Map(prev);
          next.delete(tool.name);
          return next;
        });
      }, 1500);
    }
  };

  const handleRemove = async (tool: Tool) => {
    setRemovingSet((prev) => new Set(prev).add(tool.name));
    try {
      await api.deleteTool(tool.name);
      addToast("success", `${tool.name} removed`);
      setCheckedTools(null);
      refresh();
    } catch (err) {
      const msg = err instanceof Error ? err.message : `Failed to remove ${tool.name}`;
      addToast("error", msg);
    } finally {
      setRemovingSet((prev) => { const next = new Set(prev); next.delete(tool.name); return next; });
    }
  };

  // Both lists load in parallel (two independent usePolling calls above);
  // only show the full-section skeleton before either has ever landed.
  const initialLoading = providersLoading && providers == null && toolsLoading && tools == null;

  if (initialLoading) {
    return (
      <div className="space-y-6">
        <div className="h-6 w-32 animate-pulse rounded-md bg-mycel-surface-hover" />
        <LoadingSkeleton variant="cards" rows={4} />
      </div>
    );
  }
  if (timedOut && !tools) {
    return <EmptyState icon="!" title="Tools timed out" actionLabel="Retry" onAction={refresh} />;
  }
  if (error && !tools) {
    return <EmptyState icon="!" title="Failed to load tools" description={error} actionLabel="Retry" onAction={refresh} />;
  }

  return (
    <div className="space-y-8">
      {/* Search + health check + add-tool controls */}
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="relative">
          <svg
            className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-mycel-muted pointer-events-none"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <circle cx="11" cy="11" r="8" />
            <path strokeLinecap="round" d="M21 21l-4.35-4.35" />
          </svg>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search providers & tools..."
            className="w-48 sm:w-64 pl-7 pr-7 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch("")}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 text-mycel-muted hover:text-mycel-text text-sm leading-none px-1"
              aria-label="Clear search"
            >
              &times;
            </button>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => void handleCheck()} disabled={checking}
            className="inline-flex items-center gap-1.5 h-8 px-3 text-sm rounded-md bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent">
            {checking ? <Spinner /> : null}
            {checking ? "Checking..." : "Health Check"}
          </button>
          <button type="button" onClick={() => setShowAddForm(!showAddForm)}
            className="inline-flex items-center h-8 px-3 text-sm font-medium rounded-md bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent">
            + CLI Tool
          </button>
        </div>
      </div>

      <AnimatePresence>
        {showAddForm && <AddCLIToolForm onClose={() => setShowAddForm(false)} onAdded={() => { setCheckedTools(null); refresh(); }} onToast={addToast} />}
      </AnimatePresence>

      <section>
        <div className="flex items-baseline gap-2 mb-3">
          <h2 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
            AI Model Providers
          </h2>
          {providers !== null && providers !== undefined && (
            <span className="text-[11px] text-mycel-muted tabular-nums">
              {providerList.length}
            </span>
          )}
          <span className="flex-1 h-px bg-mycel-border self-center" aria-hidden />
        </div>
        {providersLoading && providers == null ? (
          <LoadingSkeleton variant="cards" rows={2} />
        ) : providerList.length === 0 && !search ? (
          <EmptyState
            icon="*"
            title="No AI providers configured"
            description="Connect Claude, Gemini, Cursor or another provider to start spinning up agents."
          />
        ) : (
          <>
            {/* Fleet-wide default provider + model — persists to
                prefs.providers and reloads live. Rendered only once
                provider data has arrived (never over a skeleton). */}
            {providerList.length > 0 && <ProviderDefaults providers={providerList} />}
            <ProvidersTable providers={providerList} search={search} />
          </>
        )}
      </section>

      <section>
        <div className="flex items-baseline gap-2 mb-3">
          <h2 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
            CLI Dependencies
          </h2>
          <span className="text-[11px] text-mycel-muted tabular-nums">
            {filteredCli.length}{searchLower ? `/${cliTools.length}` : ""}
          </span>
          <span className="flex-1 h-px bg-mycel-border self-center" aria-hidden />
        </div>
        {/* Detected host package managers — the honest picture of what
            install/update/uninstall commands can actually use. */}
        <div className="mb-3">
          <p className="text-[10.5px] text-mycel-muted uppercase tracking-[0.08em] mb-1.5">Detected package managers</p>
          <PackageManagers />
        </div>
        {/* Guarded registry search — browse a manager's registry and install a
            hit. brew/npm/cargo install inline; the rest give a copyable
            terminal command (honest about sudo). */}
        <div className="mb-4">
          <p className="text-[10.5px] text-mycel-muted uppercase tracking-[0.08em] mb-1.5">Search a registry</p>
          <RegistrySearch />
        </div>
        {toolsLoading && tools == null ? (
          <LoadingSkeleton variant="cards" rows={2} />
        ) : filteredCli.length === 0 ? (
          searchLower ? (
            <EmptyState icon=">" title="No matching CLI tools" description="Try a different search term." />
          ) : (
            <EmptyState
              icon=">"
              title="No CLI dependencies tracked"
              description='Run "mycel tool add <name>" or use the "+ CLI Tool" button above to register tools like gh, aws, or wrangler.'
            />
          )
        ) : (
          <div className="rounded-lg border border-mycel-border overflow-hidden">
            <table className="w-full text-left">
              <thead>
                <tr className="bg-mycel-surface border-b border-mycel-border text-[11px] text-mycel-muted uppercase tracking-[0.08em]">
                  <th className="px-4 py-2.5 font-medium">Tool</th>
                  <th className="px-4 py-2.5 font-medium">Status</th>
                  <th className="px-4 py-2.5 font-medium">Version</th>
                  <th className="px-4 py-2.5 font-medium">Required</th>
                  <th className="px-4 py-2.5 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredCli.map((t) => (
                  <CLIDepsRow
                    key={t.name}
                    tool={t}
                    expanded={expandedRow === t.name}
                    onExpand={() => setExpandedRow(expandedRow === t.name ? null : t.name)}
                    onToggle={() => void handleToggle(t)}
                    onRemove={() => void handleRemove(t)}
                    toggling={togglingSet.has(t.name)}
                    removing={removingSet.has(t.name)}
                    onChanged={() => { setCheckedTools(null); refresh(); }}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Optional services (pkg/deps): containers the user can start from here,
          such as mycel-code-server. This is the only UI for the /api/deps
          lifecycle — without it the Code tab's "Edit in VS Code" button can
          never appear, since it renders only while the container is running. */}
      <section>
        <div className="flex items-baseline gap-2 mb-3">
          <h2 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
            Optional Services
          </h2>
          <span className="flex-1 h-px bg-mycel-border self-center" aria-hidden />
        </div>
        <p className="text-[10.5px] text-mycel-muted mb-2">
          Background containers the daemon can start for you. Starting
          mycel-code-server enables "Edit in VS Code" on the Code tab.
        </p>
        <DependenciesSection />
      </section>

      <ToastContainer toasts={toasts} onDismiss={dismiss} />
    </div>
  );
}
