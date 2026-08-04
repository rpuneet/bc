import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, Link, useLocation, useNavigate } from "react-router-dom";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { api } from "../api/client";
import type { Agent, AgentConfig, MCPServer } from "../api/client";
import { AgentAppsCard } from "../components/apps/AgentAppsCard";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatsTab as StatsTabComponent } from "../components/StatsTab";
import { WebTerminal, type TerminalConnectionState, type TerminalConnectionDetail } from "../components/WebTerminal";
import { LiveAgentCharacter } from "../components/agent-ui";
import { MCPServerList } from "../components/shared/MCPServerList";
import { McpEnvEditor } from "../components/shared/McpEnvEditor";
import { SystemPromptEditor } from "../components/shared/SystemPromptEditor";
import { SectionRule } from "../components/shared";
import { AgentToolStream } from "../components/live/AgentToolStream";
import { CreateAgentModal } from "../components/CreateAgentModal";
import { CodeBrowser } from "../components/code/CodeBrowser";
import { EmptyState } from "../components/EmptyState";
import { SecretValueInput, isValidEnvKey } from "../components/EnvVarsEditor";
import { formatAbsolute, formatRelative as sharedFormatRelative } from "../utils/time";
import { MONO } from "../utils/typography";

const formatTime = (t?: string): string => formatAbsolute(t);
const formatRelative = (t?: string): string => sharedFormatRelative(t, { emptyLabel: "" });

/* ═══════════════════════════════════════════════════════════════════
   Tab types — v3: Live / Attach / Settings / Metrics
   ═══════════════════════════════════════════════════════════════════ */

type Tab = "attach" | "live" | "settings" | "metrics" | "code";

const TABS: { key: Tab; label: string; shortcut: string }[] = [
  { key: "attach", label: "Attach", shortcut: "1" },
  { key: "live", label: "Live", shortcut: "2" },
  { key: "settings", label: "Settings", shortcut: "3" },
  { key: "metrics", label: "Metrics", shortcut: "4" },
  { key: "code", label: "Code", shortcut: "5" },
];

/** Map a URL sub-path segment to a tab.
 *
 *  Two legacy segments are kept alive so existing links and bookmarks do not
 *  break: `config` predates the Settings rename, and `timeline` was a separate
 *  tab that showed the same events as Live. It was removed rather than kept in
 *  parallel — Live already hydrates from the same persisted activity feed
 *  (GET /api/agents/{name}/activity) before it starts appending live events, so
 *  the two tabs were rendering one source of truth twice. */
function tabForSegment(seg: string | undefined): Tab | null {
  if (seg === "config") return "settings";
  if (seg === "timeline") return "live";
  const known: Tab[] = ["attach", "live", "settings", "metrics", "code"];
  return known.includes(seg as Tab) ? (seg as Tab) : null;
}

/* ═══════════════════════════════════════════════════════════════════
   Section chrome
   ═══════════════════════════════════════════════════════════════════ */

function MetaCell({
  label,
  children,
  mono,
}: {
  label: string;
  children: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="space-y-1">
      <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
        {label}
      </dt>
      <dd
        className="text-sm text-mycel-text leading-tight break-all"
        style={mono ? { fontFamily: MONO } : undefined}
      >
        {children}
      </dd>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Lifecycle controls — Start / Stop / Restart in the detail header
   (#3283)
   ═══════════════════════════════════════════════════════════════════ */

type LifecycleAction = "start" | "stop" | "restart";

/** Pure disabled-state logic, exported for tests. Start is only
 *  available when the agent session is not alive (stopped/error);
 *  Stop and Restart require a live session. Everything is disabled
 *  while a lifecycle request is in flight. */
export function lifecycleDisabled(
  state: string,
  pending: boolean,
): Record<LifecycleAction, boolean> {
  const stopped = state === "stopped" || state === "error";
  return {
    start: pending || !stopped,
    stop: pending || stopped,
    restart: pending || stopped,
  };
}

const LIFECYCLE_BTN =
  "inline-flex items-center justify-center w-[22px] h-[22px] rounded-md border border-mycel-border " +
  "text-mycel-muted transition-colors disabled:opacity-30 disabled:pointer-events-none " +
  "focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg";

function LifecycleSpinner() {
  return (
    <svg
      className="animate-spin"
      width="11"
      height="11"
      viewBox="0 0 14 14"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden
    >
      <path d="M7 1.5A5.5 5.5 0 1 1 1.5 7" />
    </svg>
  );
}

function LifecycleControls({
  agent,
  onAction,
}: {
  agent: Agent;
  onAction?: () => void;
}) {
  const [pending, setPending] = useState<LifecycleAction | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const disabled = lifecycleDisabled(agent.state, pending !== null);

  const run = useCallback(
    async (action: LifecycleAction) => {
      if (action === "stop" && !window.confirm(`Stop agent ${agent.name}?`)) {
        return;
      }
      setPending(action);
      setActionError(null);
      try {
        if (action === "start") {
          await api.startAgent(agent.name);
        } else if (action === "stop") {
          await api.stopAgent(agent.name);
        } else {
          // No dedicated /restart endpoint — stop, then start. The SSE
          // agent.state_changed stream flips the header state chip.
          await api.stopAgent(agent.name);
          await api.startAgent(agent.name);
        }
        onAction?.();
      } catch (err) {
        setActionError(
          err instanceof Error ? err.message : `Failed to ${action} agent`,
        );
      } finally {
        setPending(null);
      }
    },
    [agent.name, onAction],
  );

  return (
    <span className="inline-flex items-center gap-1 shrink-0">
      <button
        type="button"
        onClick={() => void run("start")}
        disabled={disabled.start}
        title="Start agent"
        aria-label={`Start agent ${agent.name}`}
        className={`${LIFECYCLE_BTN} hover:text-mycel-success hover:border-mycel-success hover:bg-mycel-success-subtle`}
      >
        {pending === "start" ? (
          <LifecycleSpinner />
        ) : (
          <svg width="11" height="11" viewBox="0 0 14 14" fill="currentColor" aria-hidden>
            <path d="M4 2.5l7.5 4.5L4 11.5z" />
          </svg>
        )}
      </button>
      <button
        type="button"
        onClick={() => void run("stop")}
        disabled={disabled.stop}
        title="Stop agent"
        aria-label={`Stop agent ${agent.name}`}
        className={`${LIFECYCLE_BTN} hover:text-mycel-error hover:border-mycel-error hover:bg-mycel-error-subtle`}
      >
        {pending === "stop" ? (
          <LifecycleSpinner />
        ) : (
          <svg width="11" height="11" viewBox="0 0 14 14" fill="currentColor" aria-hidden>
            <rect x="3" y="3" width="8" height="8" rx="1" />
          </svg>
        )}
      </button>
      <button
        type="button"
        onClick={() => void run("restart")}
        disabled={disabled.restart}
        title="Restart agent (stop, then start)"
        aria-label={`Restart agent ${agent.name}`}
        className={`${LIFECYCLE_BTN} hover:text-mycel-info hover:border-mycel-info hover:bg-mycel-info-subtle`}
      >
        {pending === "restart" ? (
          <LifecycleSpinner />
        ) : (
          <svg
            width="11"
            height="11"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden
          >
            <path d="M11 3.5A5 5 0 1 0 12 7" />
            <path d="M8 3.5h3V.5" />
          </svg>
        )}
      </button>
      {actionError && (
        <span
          className="text-xs text-mycel-error truncate max-w-[140px]"
          title={actionError}
        >
          {actionError}
        </span>
      )}
    </span>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 2 — Attach
   Direct terminal access with a status overlay that explains
   connection state: connecting / stopped / error, plus a Start/Retry
   affordance. The overlay sits on top of the xterm surface so the
   terminal instance (and its scrollback) is preserved across retries.
   ═══════════════════════════════════════════════════════════════════ */

type AttachOverlayKind =
  | "hidden"
  | "connecting"
  | "stopped"
  | "error";

function AttachTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";

  // WS lifecycle reported by WebTerminal. Seeded to "connecting" when
  // we are about to (re)open the socket.
  const [wsState, setWsState] = useState<TerminalConnectionState>("connecting");
  const [wsDetail, setWsDetail] = useState<TerminalConnectionDetail | undefined>(undefined);
  // Bumping this counter asks WebTerminal to close the current socket
  // and open a fresh one — without rebuilding the xterm instance.
  const [reconnectToken, setReconnectToken] = useState(0);
  // Tracks an in-flight POST /start so the overlay can show progress.
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);

  const onStateChange = useCallback(
    (state: TerminalConnectionState, detail?: TerminalConnectionDetail) => {
      setWsState(state);
      setWsDetail(detail);
    },
    [],
  );

  const handleRetry = useCallback(() => {
    setWsState("connecting");
    setWsDetail(undefined);
    setReconnectToken((t) => t + 1);
  }, []);

  const handleStart = useCallback(async () => {
    setStarting(true);
    setStartError(null);
    try {
      await api.startAgent(agent.name);
      // Parent AgentDetail polls and will re-render once state flips to
      // "running"; the WS will then connect on its own.
      setWsState("connecting");
    } catch (err) {
      setStartError(err instanceof Error ? err.message : "Failed to start agent");
    } finally {
      setStarting(false);
    }
  }, [agent.name]);

  // Compute which overlay variant (if any) to show.
  const overlay: AttachOverlayKind = (() => {
    if (isStopped) return "stopped";
    if (wsState === "open") return "hidden";
    if (wsState === "connecting") return "connecting";
    // closed or error
    return "error";
  })();

  return (
    <div className="flex-1 min-h-0 relative" title="Click anywhere to focus the terminal">
      {/* When the agent is stopped, we don't mount the terminal at all
          — opening the WS would just 404. Start the agent to connect. */}
      {!isStopped && (
        <WebTerminal
          agentName={agent.name}
          reconnectToken={reconnectToken}
          onConnectionStateChange={onStateChange}
        />
      )}
      {overlay !== "hidden" && (
        <AttachOverlay
          kind={overlay}
          agent={agent}
          detail={wsDetail}
          starting={starting}
          startError={startError}
          onStart={handleStart}
          onRetry={handleRetry}
        />
      )}
    </div>
  );
}

interface AttachOverlayProps {
  kind: Exclude<AttachOverlayKind, "hidden">;
  agent: Agent;
  detail?: TerminalConnectionDetail;
  starting: boolean;
  startError: string | null;
  onStart: () => void;
  onRetry: () => void;
}

function AttachOverlay({
  kind,
  agent,
  detail,
  starting,
  startError,
  onStart,
  onRetry,
}: AttachOverlayProps) {
  // Routes are flat (/agents/<name>/<tab>) — link straight to the
  // sibling "live" tab instead of parsing the current path.
  const livePath = `/agents/${agent.name}/live`;
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="attach-overlay"
      data-state={kind}
      className="absolute inset-0 z-10 flex items-center justify-center bg-mycel-overlay backdrop-blur-sm"
    >
      <div className="w-full max-w-[360px] rounded-lg border border-mycel-border bg-mycel-surface-2 px-5 py-4 shadow-mycel-lg">
        {kind === "connecting" && (
          <div className="flex items-center gap-3">
            <span
              aria-hidden="true"
              data-testid="attach-overlay-spinner"
              className="inline-block h-3 w-3 rounded-full border-2 border-mycel-accent border-t-transparent animate-spin"
            />
            <span className="text-sm text-mycel-text">
              Connecting to {agent.name}
              <span className="text-mycel-muted">…</span>
            </span>
          </div>
        )}

        {kind === "stopped" && (
          <div className="flex flex-col gap-3">
            <div>
              <p className="text-sm font-semibold text-mycel-text">Agent is stopped</p>
              <p className="mt-1 text-xs leading-relaxed text-mycel-muted">
                Start the agent to attach a live terminal.
              </p>
              {startError && (
                <p className="mt-2 text-xs text-mycel-error break-words">{startError}</p>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={onStart}
                disabled={starting}
                className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-xs font-medium text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors disabled:opacity-40"
              >
                {starting ? "Starting…" : "Start agent"}
              </button>
            </div>
          </div>
        )}

        {kind === "error" && (
          <div className="flex flex-col gap-3">
            <div>
              <p className="text-sm font-semibold text-mycel-text">Connection lost</p>
              <p className="mt-1 text-xs leading-relaxed text-mycel-muted">
                The terminal stream dropped.
                {detail?.code ? ` (code ${String(detail.code)})` : ""}
              </p>
              {detail?.reason && (
                <p className="mt-1 text-xs text-mycel-muted break-words">{detail.reason}</p>
              )}
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={onRetry}
                className="inline-flex items-center h-9 px-3 rounded-md bg-mycel-accent text-xs font-medium text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors"
              >
                Retry
              </button>
              <Link
                to={livePath}
                className="text-xs text-mycel-accent hover:text-mycel-accent-hover hover:underline"
              >
                View logs
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* ── shared save-pill for the Settings sub-panels ──────────────────── */

type SaveState = "idle" | "saving" | "saved" | "error";

function SavePill({ state, error }: { state: SaveState; error?: string | null }) {
  if (state === "saving") {
    return (
      <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted" role="status">
        <svg className="w-3 h-3 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden>
          <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="3" opacity="0.25" />
          <path d="M21 12a9 9 0 00-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
        </svg>
        Saving…
      </span>
    );
  }
  if (state === "saved") {
    return (
      <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-success" role="status">
        <svg className="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M20 6L9 17l-5-5" />
        </svg>
        Saved
      </span>
    );
  }
  if (state === "error") {
    return (
      <span className="text-[11px] text-mycel-error" role="alert">
        {error ?? "Save failed"}
      </span>
    );
  }
  return null;
}

const SETTINGS_SELECT_CLS =
  "w-full rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[13px] text-mycel-text outline-none focus:border-mycel-accent transition-colors disabled:opacity-50 disabled:cursor-not-allowed";
const SETTINGS_INPUT_CLS =
  "w-32 rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[13px] tabular-nums text-mycel-text placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors disabled:opacity-50 disabled:cursor-not-allowed";

/* ── Provider & Model override ──────────────────────────────────────
 * The provider (tool) is fixed for an agent's lifetime — switching it
 * would need a fresh container image, so it's shown read-only with an
 * honest note. The model, however, is genuinely re-read on restart, so
 * it is a live picker bound to PATCH /config { model }.
 */
function ProviderModelSection({
  agentName,
  tool,
  model,
  onSaved,
}: {
  agentName: string;
  tool: string;
  model: string;
  onSaved: () => void;
}) {
  const [models, setModels] = useState<Array<{ id: string; available: boolean }>>([]);
  const [selected, setSelected] = useState(model);
  const [save, setSave] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => { setSelected(model); }, [model]);

  useEffect(() => {
    let alive = true;
    api
      .listProviders()
      .then((providers) => {
        if (!alive) return;
        const p = providers.find((x) => x.name === tool);
        setModels((p?.models ?? []).map((m) => ({ id: m.id, available: m.available })));
      })
      .catch(() => { /* degrade to no model list */ });
    return () => { alive = false; };
  }, [tool]);

  const persist = useCallback(
    async (next: string) => {
      setSelected(next);
      setSave("saving");
      setSaveError(null);
      try {
        await api.patchAgentConfig(agentName, { model: next });
        setSave("saved");
        onSaved();
        setTimeout(() => setSave("idle"), 1800);
      } catch (err) {
        setSelected(model);
        setSaveError(err instanceof Error ? err.message : "Save failed");
        setSave("error");
      }
    },
    [agentName, model, onSaved],
  );

  return (
    <section>
      <div className="flex items-center justify-between mb-1">
        <SectionRule>Provider &amp; Model</SectionRule>
        <div className="min-h-[16px]"><SavePill state={save} error={saveError} /></div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">Provider</span>
          <input
            type="text"
            value={tool || "—"}
            disabled
            aria-label="Provider (fixed for this agent)"
            className={SETTINGS_SELECT_CLS}
            style={{ fontFamily: MONO }}
          />
        </label>
        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">Model override</span>
          <select
            className={SETTINGS_SELECT_CLS}
            value={selected}
            disabled={save === "saving" || models.length === 0}
            onChange={(e) => { void persist(e.target.value); }}
            aria-label="Model override"
            style={{ fontFamily: MONO }}
          >
            <option value="">Provider default</option>
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.id}{m.available ? "" : " · unverified"}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="mt-2 text-xs text-mycel-muted leading-relaxed">
        {models.length === 0
          ? "This provider exposes no model list — its own default is used. "
          : "Model applies on the agent’s next restart. "}
        Switching provider isn’t supported in place — clone the agent onto a different
        provider instead.
        {/* follow-up: a real per-agent provider switch needs a container re-image. */}
        {tool && (
          <>
            {" "}
            <Link
              to={`/settings/providers/${encodeURIComponent(tool)}`}
              className="text-mycel-accent hover:underline"
            >
              Manage {tool} (commands, MCP, install) →
            </Link>
          </>
        )}
      </p>
    </section>
  );
}

/* ── Resource limits (CPU / Memory) ─────────────────────────────────
 * Per-agent Docker caps, bound to PATCH /config { cpus, memory_mb }.
 * Blank inputs clear the override (send 0) so the fleet default applies.
 * For tmux agents the limits are stored but not enforced — labelled so.
 */
function AgentGuardrailsSection({ templateName }: { templateName?: string }) {
  const [maxCost, setMaxCost] = useState<number | null>(null);
  const [stuckMin, setStuckMin] = useState<number | null>(null);
  const [loadErr, setLoadErr] = useState(false);

  useEffect(() => {
    if (!templateName) {
      setMaxCost(null);
      setStuckMin(null);
      return;
    }
    let cancelled = false;
    fetch(`/api/templates/${encodeURIComponent(templateName)}`)
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status));
        return r.json() as Promise<{ max_cost_usd?: number; stuck_timeout_min?: number }>;
      })
      .then((t) => {
        if (cancelled) return;
        setMaxCost(t.max_cost_usd && t.max_cost_usd > 0 ? t.max_cost_usd : null);
        setStuckMin(t.stuck_timeout_min && t.stuck_timeout_min > 0 ? t.stuck_timeout_min : null);
        setLoadErr(false);
      })
      .catch(() => {
        if (!cancelled) setLoadErr(true);
      });
    return () => {
      cancelled = true;
    };
  }, [templateName]);

  return (
    <section>
      <SectionRule>Guardrails</SectionRule>
      {!templateName ? (
        <p className="text-xs text-mycel-muted leading-relaxed mt-1">
          No template on this agent — cost and stuck limits are not enforced. Create from a
          template (or set one) to attach guardrails.
        </p>
      ) : loadErr ? (
        <p className="text-xs text-mycel-muted mt-1">Could not load template {templateName}.</p>
      ) : (
        <div className="mt-2 space-y-2">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm" style={{ fontFamily: MONO }}>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-mycel-muted">Max cost</div>
              <div className="text-mycel-text">
                {maxCost != null ? `$${maxCost.toFixed(2)}` : "— (no cap)"}
              </div>
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-mycel-muted">Stuck timeout</div>
              <div className="text-mycel-text">
                {stuckMin != null ? `${stuckMin} min` : "— (no timeout)"}
              </div>
            </div>
          </div>
          <p className="text-xs text-mycel-muted leading-relaxed">
            In effect from template{" "}
            <Link
              to={`/templates`}
              className="text-mycel-accent hover:underline"
              title={templateName}
            >
              {templateName}
            </Link>
            . Edit them on the template — the daemon re-reads on each check.
          </p>
        </div>
      )}
    </section>
  );
}

function ResourceLimitsSection({
  agentName,
  isDocker,
  cpus,
  memoryMB,
  onSaved,
}: {
  agentName: string;
  isDocker: boolean;
  cpus: number;
  memoryMB: number;
  onSaved: () => void;
}) {
  const [cpuInput, setCpuInput] = useState(cpus > 0 ? String(cpus) : "");
  const [memInput, setMemInput] = useState(memoryMB > 0 ? String(memoryMB) : "");
  const [defaults, setDefaults] = useState<{ cpus: number; memory_mb: number } | null>(null);
  const [save, setSave] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => { setCpuInput(cpus > 0 ? String(cpus) : ""); }, [cpus]);
  useEffect(() => { setMemInput(memoryMB > 0 ? String(memoryMB) : ""); }, [memoryMB]);

  useEffect(() => {
    let alive = true;
    api
      .getSettings()
      .then((cfg) => { if (alive) setDefaults({ cpus: cfg.runtime?.docker?.cpus ?? 0, memory_mb: cfg.runtime?.docker?.memory_mb ?? 0 }); })
      .catch(() => { /* placeholder stays generic */ });
    return () => { alive = false; };
  }, []);

  const dirty =
    (cpuInput.trim() === "" ? 0 : Number(cpuInput)) !== cpus ||
    (memInput.trim() === "" ? 0 : Number(memInput)) !== memoryMB;
  const cpuNum = cpuInput.trim() === "" ? 0 : Number(cpuInput);
  const memNum = memInput.trim() === "" ? 0 : Number(memInput);
  const invalid =
    (cpuInput.trim() !== "" && (!Number.isFinite(cpuNum) || cpuNum < 0)) ||
    (memInput.trim() !== "" && (!Number.isFinite(memNum) || memNum < 0 || !Number.isInteger(memNum)));

  const handleSave = useCallback(async () => {
    if (invalid) return;
    setSave("saving");
    setSaveError(null);
    try {
      await api.patchAgentConfig(agentName, { cpus: cpuNum, memory_mb: memNum });
      setSave("saved");
      onSaved();
      setTimeout(() => setSave("idle"), 1800);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Save failed");
      setSave("error");
    }
  }, [agentName, cpuNum, memNum, invalid, onSaved]);

  return (
    <section>
      <div className="flex items-center justify-between mb-1">
        <SectionRule>Resource Limits</SectionRule>
        <div className="min-h-[16px]"><SavePill state={save} error={saveError} /></div>
      </div>
      <div className="flex flex-wrap items-end gap-4">
        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">CPU (cores)</span>
          <input
            type="number"
            min={0}
            step={0.5}
            value={cpuInput}
            onChange={(e) => setCpuInput(e.target.value)}
            placeholder={defaults ? `default ${defaults.cpus}` : "default"}
            aria-label="CPU cores cap"
            className={SETTINGS_INPUT_CLS}
            style={{ fontFamily: MONO }}
          />
        </label>
        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">Memory (MB)</span>
          <input
            type="number"
            min={0}
            step={256}
            value={memInput}
            onChange={(e) => setMemInput(e.target.value)}
            placeholder={defaults ? `default ${defaults.memory_mb}` : "default"}
            aria-label="Memory MB cap"
            className={SETTINGS_INPUT_CLS}
            style={{ fontFamily: MONO }}
          />
        </label>
        <button
          type="button"
          disabled={!dirty || invalid || save === "saving"}
          onClick={() => { void handleSave(); }}
          className="inline-flex items-center h-9 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover active:scale-[0.98] shadow-mycel-sm transition-all disabled:opacity-40 disabled:pointer-events-none"
        >
          {save === "saving" ? "Saving…" : "Save limits"}
        </button>
      </div>
      {invalid && (
        <p className="mt-2 text-xs text-mycel-error">
          CPU must be a non-negative number; memory must be a whole number of MB.
        </p>
      )}
      <p className="mt-2 text-xs text-mycel-muted leading-relaxed">
        {isDocker
          ? "Applied to the container on the agent’s next restart. Leave blank to inherit the fleet default. Caps prevent one agent from starving the host."
          : "This agent runs under tmux on the host, where per-agent CPU/memory caps are not enforced. Limits are saved and will apply if the agent moves to the Docker runtime."}
      </p>
    </section>
  );
}

function SettingsTab({ agent, agentsUrl }: { agent: Agent; agentsUrl: string }) {
  const navigate = useNavigate();
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Clone + Archive state
  const [cloneOpen, setCloneOpen] = useState(false);
  const [allAgents, setAllAgents] = useState<Agent[]>([]);
  const [confirmArchive, setConfirmArchive] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [archiveError, setArchiveError] = useState<string | null>(null);
  const isArchived = Boolean(agent.archived_at);

  // MCP server management state
  const [mcpList, setMcpList] = useState<string[] | null>(null);
  const [mcpLoading, setMcpLoading] = useState(true);
  const [mcpRegistry, setMcpRegistry] = useState<MCPServer[]>([]);
  const [mcpAddError, setMcpAddError] = useState<string | null>(null);

  // Env vars state — persisted via API to .mycel/agents/<name>/env.json
  const [envVars, setEnvVars] = useState<Array<{ key: string; value: string }>>([]);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const [envSaved, setEnvSaved] = useState(false);
  const envSavedTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Template sync state
  const [templates, setTemplates] = useState<string[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [syncDone, setSyncDone] = useState(false);
  const syncDoneTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [hostname, setHostname] = useState("localhost");

  const fetchMcps = useCallback(() => {
    setMcpLoading(true);
    api
      .getAgentMcps(agent.name)
      .then((data) => {
        setMcpList(data.map((m) => m.name));
      })
      .catch(() => {
        setMcpList(null); // fall back to static chips
      })
      .finally(() => {
        setMcpLoading(false);
      });
  }, [agent.name]);

  // Re-fetch the agent config (system prompt, model, resource caps) after a
  // sub-panel persists a change so the surface reflects what's on disk.
  const reloadConfig = useCallback(() => {
    api
      .getAgentConfig(agent.name)
      .then((data) => setConfig(data))
      .catch(() => { /* best-effort */ });
  }, [agent.name]);

  useEffect(() => {
    let cancelled = false;
    setConfigLoading(true);
    api
      .getAgentConfig(agent.name)
      .then((data) => {
        if (!cancelled) {
          setConfig(data);
        }
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        if (!cancelled) setConfigLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  useEffect(() => {
    fetchMcps();
  }, [fetchMcps]);

  // Real MCP server definitions from the global registry — used to power
  // the "Add MCP" suggestions instead of a hardcoded, disconnected list.
  useEffect(() => {
    let cancelled = false;
    api
      .listMCP()
      .then((servers) => {
        if (!cancelled) setMcpRegistry(servers);
      })
      .catch(() => {
        if (!cancelled) setMcpRegistry([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Load templates list on mount.
  useEffect(() => {
    let cancelled = false;
    fetch("/api/templates")
      .then((r) => (r.ok ? (r.json() as Promise<Array<{ name: string }>>) : Promise.resolve([])))
      .then((data) => {
        if (!cancelled) {
          const names = data.map((t) => t.name).filter(Boolean) as string[];
          setTemplates(names);
          if (names.length > 0) {
            const nonBlank = names.filter((t) => t !== "blank");
            setSelectedTemplate(nonBlank[0] ?? names[0] ?? "blank");
          }
        }
      })
      .catch(() => {/* best-effort */});
    return () => { cancelled = true; };
  }, []);

  // Fetch hostname from system info endpoint (uses shared API client for workspace header).
  useEffect(() => {
    api.getSystemInfo()
      .then((data) => { if (data?.hostname) setHostname(data.hostname); })
      .catch(() => {/* best-effort */});
  }, []);

  // Load persisted env vars from API on mount.
  useEffect(() => {
    let cancelled = false;
    api
      .getAgentEnv(agent.name)
      .then((data) => {
        if (!cancelled) setEnvVars(data);
      })
      .catch(() => {
        /* best-effort — fall back to empty */
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  // Persist env vars to API and show "Saved" indicator.
  const saveEnvVars = useCallback((vars: Array<{ key: string; value: string }>) => {
    api
      .putAgentEnv(agent.name, vars)
      .then(() => {
        setEnvSaved(true);
        if (envSavedTimer.current) clearTimeout(envSavedTimer.current);
        envSavedTimer.current = setTimeout(() => setEnvSaved(false), 2000);
      })
      .catch(() => {
        /* best-effort */
      });
  }, [agent.name]);

  const handleSync = useCallback(async () => {
    if (!selectedTemplate) return;
    setSyncing(true);
    try {
      const res = await fetch(`/api/templates/${encodeURIComponent(selectedTemplate)}`);
      if (!res.ok) throw new Error(`Failed to fetch template: ${res.status}`);
      const tmpl = await res.json() as { system_prompt?: string };
      if (tmpl.system_prompt !== undefined) {
        await api.patchAgentConfig(agent.name, { system_prompt: tmpl.system_prompt });
        const updated = await api.getAgentConfig(agent.name);
        setConfig(updated);
      }
      setSyncDone(true);
      if (syncDoneTimer.current) clearTimeout(syncDoneTimer.current);
      syncDoneTimer.current = setTimeout(() => setSyncDone(false), 2000);
    } catch {
      /* best-effort */
    } finally {
      setSyncing(false);
    }
  }, [agent.name, selectedTemplate]);

  // If live fetch succeeded use that list; otherwise fall back to static record
  const mcpServers: string[] =
    mcpList !== null
      ? mcpList
      : config?.mcp_servers && config.mcp_servers.length > 0
        ? config.mcp_servers
        : (agent.mcp_servers ?? []);

  const useLiveMcp = mcpList !== null;

  const handleSystemPromptSave = async (newValue: string) => {
    await api.patchAgentConfig(agent.name, { system_prompt: newValue });
    // Refresh config after save
    const updated = await api.getAgentConfig(agent.name);
    setConfig(updated);
  };

  const handleMcpAdd = async (mcpName: string) => {
    setMcpAddError(null);
    try {
      await api.addAgentMcp(agent.name, mcpName);
      fetchMcps();
    } catch (err) {
      // The backend resolves mcpName against the global MCP registry and
      // rejects names with no known command/url definition rather than
      // writing an empty stanza — surface that (or any other) failure
      // instead of swallowing it.
      setMcpAddError(err instanceof Error ? err.message : "Failed to add MCP server");
      throw err;
    }
  };

  const handleMcpRemove = async (mcpName: string) => {
    await api.removeAgentMcp(agent.name, mcpName);
    fetchMcps();
  };

  const isDocker = (agent.runtime_backend ?? config?.runtime_backend ?? "") === "docker";
  const isTmux = !isDocker;

  return (
    <div className="flex-1 min-h-0 overflow-y-auto p-6">
      <div className="max-w-3xl mx-auto space-y-10">

        {/* ── RUNTIME BANNER ── */}
        <div
          className={`flex items-center gap-2.5 rounded-md px-4 py-2.5 border text-[11px] ${
            isDocker
              ? "border-mycel-border bg-mycel-info-subtle text-mycel-info"
              : "border-mycel-border bg-mycel-accent-subtle text-mycel-accent"
          }`}
          style={{ fontFamily: MONO }}
        >
          {isDocker ? (
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <rect x="1" y="4" width="12" height="8" rx="1" />
              <path d="M4 4V2h6v2" />
              <path d="M5 7h4M5 9.5h4" opacity="0.6" />
            </svg>
          ) : (
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <rect x="1.5" y="1.5" width="11" height="8" rx="1.5" />
              <path d="M7 9.5v2.5M4 12h6" />
            </svg>
          )}
          <span className="font-semibold">
            {isDocker ? "Docker container" : `tmux (${hostname})`}
          </span>
          <span className="text-current/50">·</span>
          {isDocker ? (
            <span className="text-current/60">
              {agent.session ?? config?.session ?? "isolated container"}
            </span>
          ) : (
            <span className="text-current/60">
              session: {agent.session ?? config?.session ?? agent.name}
            </span>
          )}
        </div>

        {/* ── SYSTEM PROMPT ── */}
        <SystemPromptEditor
          value={config?.system_prompt ?? ""}
          loading={configLoading}
          onSave={handleSystemPromptSave}
        />

        {/* ── PROVIDER & MODEL ── */}
        <ProviderModelSection
          agentName={agent.name}
          tool={agent.tool ?? config?.tool ?? ""}
          model={config?.model ?? agent.model ?? ""}
          onSaved={reloadConfig}
        />

        {/* ── RESOURCE LIMITS ── */}
        <ResourceLimitsSection
          agentName={agent.name}
          isDocker={isDocker}
          cpus={config?.cpus ?? agent.cpus ?? 0}
          memoryMB={config?.memory_mb ?? agent.memory_mb ?? 0}
          onSaved={reloadConfig}
        />

        {/* ── GUARDRAILS (from template) ── */}
        <AgentGuardrailsSection templateName={agent.template} />

        {/* ── TEMPLATE ── */}
        {templates.length > 0 && (
          <section>
            <div className="flex items-center justify-between mb-1">
              <SectionRule>Template</SectionRule>
              {syncDone && (
                <span className="text-xs text-mycel-success transition-opacity">
                  Synced
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              <select
                value={selectedTemplate}
                onChange={(e) => setSelectedTemplate(e.target.value)}
                className="flex-1 rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[11px] text-mycel-text outline-none focus:border-mycel-accent transition-colors"
                style={{ fontFamily: MONO }}
              >
                {templates.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
              <button
                type="button"
                disabled={syncing || !selectedTemplate}
                onClick={() => { void handleSync(); }}
                className="inline-flex items-center px-3 py-1.5 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-xs font-medium text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-40"
              >
                {syncing ? "Syncing…" : "Sync"}
              </button>
            </div>
            <p className="text-xs text-mycel-muted mt-1 leading-relaxed">
              Re-apply template system prompt and MCP configuration
            </p>
          </section>
        )}

        {/* ── MCP SERVERS ── */}
        <section>
          <SectionRule>MCP Servers</SectionRule>
          <MCPServerList
            servers={mcpServers}
            loading={mcpLoading}
            onAdd={useLiveMcp ? handleMcpAdd : undefined}
            onRemove={useLiveMcp ? handleMcpRemove : undefined}
            registry={mcpRegistry}
            addError={mcpAddError}
          />
          <p className="mt-2 text-xs text-mycel-muted leading-relaxed">
            {isTmux
              ? "For tmux agents, MCPs are managed via the Claude CLI. Changes here write to the agent\u2019s worktree."
              : "Changes write to .mcp.json in the container."}
          </p>
        </section>

        {/* ── MCP ENVIRONMENT ── */}
        {mcpServers.length > 0 && (
          <section>
            <SectionRule>MCP Environment</SectionRule>
            <McpEnvEditor serverNames={mcpServers} />
          </section>
        )}

        {/* ── APPS ── */}
        <section>
          <SectionRule>Apps</SectionRule>
          <AgentAppsCard agentName={agent.name} />
          <p className="mt-2 text-xs text-mycel-muted leading-relaxed">
            App channels this agent listens to — messages route here from connected
            platforms (Slack, Telegram, WhatsApp, …).
          </p>
        </section>

        {/* ── RUNTIME INFO ── */}
        <section>
          <SectionRule>Runtime</SectionRule>
          <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-4">
            <MetaCell label="Provider" mono>
              {agent.tool || "\u2014"}
              {agent.model && (
                <span className="text-mycel-muted"> \u00b7 {agent.model}</span>
              )}
            </MetaCell>
            <MetaCell label="Backend" mono>
              {isDocker ? "docker" : "tmux"}
            </MetaCell>
            <MetaCell label="Session" mono>
              {/* Session name is the agent name by construction; fall back
                  to it while the agent is running instead of rendering an
                  em-dash next to a live state badge. */}
              {agent.session
                || config?.session
                || (agent.state !== "stopped" && agent.state !== "error" ? agent.name : "")
                || "\u2014"}
            </MetaCell>
            <MetaCell label="Created">
              <span className="tabular-nums">{formatTime(agent.created_at)}</span>
            </MetaCell>
            <MetaCell label="Started">
              <span className="tabular-nums">{formatTime(agent.started_at)}</span>
            </MetaCell>
            {/* Hide a stale stopped_at from a previous run \u2014 rendering
                "Stopped" earlier than "Started" reads as a contradiction. */}
            {agent.stopped_at
              && (!agent.started_at
                || new Date(agent.stopped_at).getTime() >= new Date(agent.started_at).getTime()) && (
              <MetaCell label="Stopped">
                <span className="tabular-nums">{formatTime(agent.stopped_at)}</span>
              </MetaCell>
            )}
          </dl>
        </section>

        {/* ── ENVIRONMENT ── */}
        <section>
          <div className="flex items-center justify-between mb-1">
            <SectionRule>Environment</SectionRule>
            {envSaved && (
              <span className="text-xs text-mycel-success transition-opacity">
                Saved
              </span>
            )}
          </div>

          {/* Placeholder hint when no env vars are set */}
          {envVars.length === 0 && (
            <p className="mb-3 text-xs text-mycel-muted italic" style={{ fontFamily: MONO }}>
              Common: ANTHROPIC_API_KEY, GITHUB_TOKEN, AWS_ACCESS_KEY_ID
            </p>
          )}

          {/* Existing env var rows */}
          {envVars.length > 0 && (
            <div className="mb-3 rounded-md border border-mycel-border overflow-hidden divide-y divide-mycel-border">
              {envVars.map((ev, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 px-3 py-2 bg-mycel-surface group"
                >
                  <span
                    className="flex-1 min-w-0 text-[11px] font-semibold text-mycel-text-2 truncate"
                    style={{ fontFamily: MONO }}
                  >
                    {ev.key}
                  </span>
                  <span className="text-mycel-muted text-[11px]">=</span>
                  <span
                    className="flex-1 min-w-0 text-[11px] text-mycel-muted truncate"
                    style={{ fontFamily: MONO }}
                  >
                    {ev.value}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      const next = envVars.filter((_, idx) => idx !== i);
                      setEnvVars(next);
                      saveEnvVars(next);
                    }}
                    className="shrink-0 text-[11px] text-mycel-muted hover:text-mycel-error transition-colors opacity-0 group-hover:opacity-100"
                    aria-label={`Remove ${ev.key}`}
                    title={`Remove ${ev.key}`}
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Add row — value input supports ${secret:NAME} autocomplete */}
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={newKey}
              onChange={(e) => setNewKey(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && isValidEnvKey(newKey.trim())) {
                  const next = [...envVars, { key: newKey.trim(), value: newValue }];
                  setEnvVars(next);
                  saveEnvVars(next);
                  setNewKey("");
                  setNewValue("");
                }
              }}
              placeholder="KEY"
              aria-invalid={newKey.trim() !== "" && !isValidEnvKey(newKey.trim())}
              className={`w-32 rounded-md border bg-mycel-bg px-2.5 py-1 text-[11px] text-mycel-text placeholder:text-mycel-muted outline-none focus:border-mycel-accent transition-colors ${
                newKey.trim() !== "" && !isValidEnvKey(newKey.trim())
                  ? "border-mycel-error"
                  : "border-mycel-border-strong"
              }`}
              style={{ fontFamily: MONO }}
            />
            <span className="text-mycel-muted text-[11px]" style={{ fontFamily: MONO }}>=</span>
            <SecretValueInput value={newValue} onChange={setNewValue} />
            <button
              type="button"
              disabled={!isValidEnvKey(newKey.trim())}
              onClick={() => {
                if (!isValidEnvKey(newKey.trim())) return;
                const next = [...envVars, { key: newKey.trim(), value: newValue }];
                setEnvVars(next);
                saveEnvVars(next);
                setNewKey("");
                setNewValue("");
              }}
              className="inline-flex items-center px-2.5 py-1 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-xs font-medium text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-40"
            >
              + Add
            </button>
          </div>

          <p className="mt-2 text-xs text-mycel-muted leading-relaxed">
            {isTmux
              ? "Set via provider CLI environment · Env vars are applied on agent restart"
              : "Injected as container environment variables · Env vars are applied on agent restart"}
            {" · "}Values may reference vault secrets as{" "}
            <span style={{ fontFamily: MONO }}>{"${secret:NAME}"}</span> — resolved at spawn,
            shown here as the reference.
          </p>
          {agent.tool === "claude" && (
            <div className="mt-2 text-xs text-mycel-muted">
              <span className="font-medium">Claude requires:</span> ANTHROPIC_API_KEY
            </div>
          )}
          {agent.tool === "agy" && (
            <div className="mt-2 text-xs text-mycel-muted">
              <span className="font-medium">agy requires:</span> a signed-in Google account (run <code>agy</code> once to authenticate)
            </div>
          )}
          {agent.tool === "openai" && (
            <div className="mt-2 text-xs text-mycel-muted">
              <span className="font-medium">OpenAI requires:</span> OPENAI_API_KEY
            </div>
          )}
        </section>

        {/* ── ACTIONS ── */}
        <section>
          <SectionRule>Actions</SectionRule>
          <div className="rounded-md border border-mycel-border bg-mycel-surface px-4 py-3 shadow-mycel">
            <div className="flex flex-wrap gap-2 items-center">
              {/* Clone — opens CreateAgentModal pre-seeded with this agent */}
              <button
                type="button"
                onClick={() => {
                  api
                    .listAgents()
                    .then((list) => setAllAgents(list))
                    .catch(() => setAllAgents([agent]))
                    .finally(() => setCloneOpen(true));
                }}
                className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-border bg-mycel-surface text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              >
                Clone
              </button>

              {/* Archive / Unarchive with confirm flow mirroring Delete */}
              {archiveError && (
                <span className="text-xs text-mycel-error">
                  {archiveError}
                </span>
              )}
              {isArchived ? (
                <button
                  type="button"
                  disabled={archiving}
                  onClick={() => {
                    setArchiving(true);
                    setArchiveError(null);
                    api
                      .unarchiveAgent(agent.name)
                      .then(() => navigate(agentsUrl))
                      .catch((err: unknown) => {
                        setArchiving(false);
                        setArchiveError(
                          err instanceof Error ? err.message : "Failed to unarchive agent",
                        );
                      });
                  }}
                  className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-accent text-mycel-accent hover:bg-mycel-accent-subtle transition-colors disabled:opacity-40"
                >
                  {archiving ? "Unarchiving…" : "Unarchive"}
                </button>
              ) : confirmArchive ? (
                <>
                  <span className="text-xs text-mycel-muted">
                    Archive this agent?
                  </span>
                  <button
                    type="button"
                    disabled={archiving}
                    onClick={() => {
                      setArchiving(true);
                      setArchiveError(null);
                      api
                        .archiveAgent(agent.name)
                        .then(() => navigate(agentsUrl))
                        .catch((err: unknown) => {
                          setArchiving(false);
                          setConfirmArchive(false);
                          setArchiveError(
                            err instanceof Error ? err.message : "Failed to archive agent",
                          );
                        });
                    }}
                    className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-accent bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-40"
                  >
                    {archiving ? "Archiving…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    disabled={archiving}
                    onClick={() => setConfirmArchive(false)}
                    className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-border bg-mycel-surface text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-40"
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmArchive(true)}
                  className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-border bg-mycel-surface text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
                >
                  Archive
                </button>
              )}
            </div>

            <CreateAgentModal
              open={cloneOpen}
              onClose={() => setCloneOpen(false)}
              existingNames={allAgents.map((a) => a.name)}
              existingAgents={allAgents}
              defaultCloneFrom={agent.name}
            />
          </div>
        </section>

        {/* ── DANGER ZONE ── */}
        <section>
          <SectionRule>Danger Zone</SectionRule>
          <div className="rounded-md border border-mycel-error bg-mycel-surface px-4 py-3 shadow-mycel">
            {deleteError && (
              <p className="mb-2 text-xs text-mycel-error">
                {deleteError}
              </p>
            )}
            <div className="flex flex-wrap gap-2 items-center">
              {/* Delete — with confirmation */}
              {confirmDelete ? (
                <>
                  <span className="text-xs text-mycel-error">
                    Are you sure?
                  </span>
                  <button
                    type="button"
                    disabled={deleting}
                    onClick={() => {
                      setDeleting(true);
                      setDeleteError(null);
                      api
                        .deleteAgent(agent.name)
                        .then(() => {
                          navigate(agentsUrl);
                        })
                        .catch((err: unknown) => {
                          setDeleting(false);
                          setConfirmDelete(false);
                          setDeleteError(err instanceof Error ? err.message : "Failed to delete agent");
                        });
                    }}
                    className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-error bg-mycel-error-subtle text-mycel-error hover:bg-mycel-error hover:text-white transition-colors disabled:opacity-40"
                  >
                    {deleting ? "Deleting…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    disabled={deleting}
                    onClick={() => setConfirmDelete(false)}
                    className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-border bg-mycel-surface text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-40"
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmDelete(true)}
                  className="inline-flex items-center px-3 py-1.5 rounded-md text-xs font-medium border border-mycel-border text-mycel-error hover:bg-mycel-error-subtle hover:border-mycel-error transition-colors"
                >
                  Delete
                </button>
              )}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 4 — Stats
   Wraps existing StatsTabComponent, adds stopped-agent context
   ═══════════════════════════════════════════════════════════════════ */

/* ═══════════════════════════════════════════════════════════════════
   Tab 5 — Code
   Embedded CodeBrowser pinned to this agent's worktree. Defaults to
   diff view (agent worktree vs main repo). The compact header carries
   a "Full view" link to the top-level /code view.
   ═══════════════════════════════════════════════════════════════════ */

function CodeTab({ agent }: { agent: Agent }) {
  return (
    <CodeBrowser
      key={agent.name}
      worktree={agent.name}
      embedded
      fullViewHref={`/code?worktree=${encodeURIComponent(agent.name)}`}
      emptyState={
        <EmptyState
          icon={
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="m8 8-4 4 4 4M16 8l4 4-4 4M13 5l-2 14" />
            </svg>
          }
          title="No worktree to browse"
          description={`${agent.name} has no git worktree yet, or it is empty. Once the agent works on a repo, its uncommitted changes show up here as a diff against the main repo.`}
        />
      }
    />
  );
}

function MetricsTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";
  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-4xl mx-auto space-y-4">
        {isStopped && (
          <p className="text-xs text-mycel-muted italic">
            Agent is not running. Stats show last known values.
          </p>
        )}
        <StatsTabComponent agent={agent} />
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Main component
   ═══════════════════════════════════════════════════════════════════ */

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const agentsUrl = "/agents";

  // Derive active tab from URL sub-path: /agents/<name>/<tab>
  // Defaults to "attach" when no sub-path is present.
  const tabFromPath = useMemo<Tab>(() => {
    const segments = location.pathname.split("/");
    const last = segments[segments.length - 1];
    return tabForSegment(last) ?? "attach";
  }, [location.pathname]);
  const [activeTab, setActiveTab] = useState<Tab>(tabFromPath);

  // Keep state in sync when URL changes (browser back/forward, deep link)
  useEffect(() => {
    if (tabFromPath !== activeTab) setActiveTab(tabFromPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabFromPath]);

  // Clicking a tab updates both state and URL. Routes are flat
  // (/agents/<name>/<tab>) — build the path absolutely instead of
  // appending to the current one (the old relative slice logic grew
  // /agents/x/config/metrics/code on every click), and replace history
  // so Back leaves the detail page instead of replaying tab switches.
  const selectTab = useCallback(
    (tab: Tab) => {
      setActiveTab(tab);
      if (!name) return;
      navigate(`/agents/${name}/${tab}`, { replace: true });
    },
    [name, navigate],
  );

  const { subscribe } = useWebSocket();

  const agentFetcher = useCallback(async () => {
    if (!name) throw new Error("No agent name");
    return api.getAgent(name);
  }, [name]);

  const {
    data: agent,
    loading,
    error,
    refresh,
  } = usePolling<Agent>(agentFetcher, 3000);

  // Refresh on agent state changes
  useEffect(() => {
    return subscribe("agent.state_changed", () => void refresh());
  }, [subscribe, refresh]);

  // Keyboard shortcuts: 1-5 for tabs, s for start/stop, Esc for back
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      switch (e.key) {
        case "1":
          selectTab("attach");
          break;
        case "2":
          selectTab("live");
          break;
        case "3":
          selectTab("settings");
          break;
        case "4":
          selectTab("metrics");
          break;
        case "5":
          selectTab("code");
          break;
        case "Escape":
          navigate(agentsUrl);
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
    };
  }, [navigate, selectTab, agentsUrl]);

  // The comprehensive HUD (back link + agent icon + name + state + task)
  // now lives in the shared header's center slot; the tabs live in the
  // right-aligned actions slot. Fresh JSX each render keeps the state
  // chip and active tab live — that's the intended HeaderSlot usage.
  const hudLastSeen =
    agent && (agent.updated_at ?? agent.started_at ?? agent.created_at);
  useHeaderSlot(
    agent
      ? {
          // ── center slot: identity + status ──
          title: (
            <div className="flex items-center gap-3 min-w-0">
              {/* ── Identity ── */}
              <Link
                to={agentsUrl}
                className="text-mycel-muted hover:text-mycel-text transition-colors shrink-0"
                title="Back to agents"
                aria-label="Back to agents"
              >
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M9 3l-4 4 4 4" />
                </svg>
              </Link>
              <LiveAgentCharacter name={agent.name} state={agent.state} size={28} tool={agent.tool} />
              <span className="text-lg font-semibold text-mycel-text tracking-tight shrink-0">
                {agent.name}
              </span>
              {agent.runtime_backend && (
                <span className="shrink-0 text-mycel-muted" title={`Runtime: ${agent.runtime_backend}`}>
                  {agent.runtime_backend === "docker" ? (
                    <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                      <rect x="1" y="4" width="12" height="8" rx="1" />
                      <path d="M4 4V2h6v2" />
                      <path d="M5 7h4M5 9.5h4" opacity="0.5" />
                    </svg>
                  ) : (
                    <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
                      <rect x="1.5" y="1.5" width="11" height="8" rx="1.5" />
                      <path d="M7 9.5v2.5M4 12h6" />
                    </svg>
                  )}
                </span>
              )}

              {/* Hairline separator — identity ↔ status */}
              <span className="hidden sm:block h-4 w-px bg-mycel-border shrink-0" aria-hidden />

              {/* ── Status ── state chip + task line */}
              <span
                className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[10px] font-medium tracking-wide uppercase ring-1 ring-inset ring-mycel-border shrink-0 ${
                  agent.state === "working" ? "bg-mycel-success-subtle text-mycel-success" :
                  agent.state === "idle" ? "bg-mycel-warning-subtle text-mycel-warning" :
                  agent.state === "stuck" ? "bg-mycel-warning-subtle text-mycel-warning" :
                  agent.state === "error" ? "bg-mycel-error-subtle text-mycel-error" :
                  agent.state === "starting" ? "bg-mycel-success-subtle text-mycel-success" :
                  agent.state === "done" ? "bg-mycel-info-subtle text-mycel-info" :
                  "bg-mycel-surface-hover text-mycel-text-2"
                }`}
                title={agent.state}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full ${
                    agent.state === "working" ? "bg-mycel-success animate-pulse" :
                    agent.state === "starting" ? "bg-mycel-success animate-pulse" :
                    agent.state === "stuck" ? "bg-mycel-warning animate-pulse" :
                    agent.state === "error" ? "bg-mycel-error" :
                    "bg-current opacity-60"
                  }`}
                  aria-hidden
                />
                {agent.state}
              </span>

              <LifecycleControls agent={agent} onAction={() => void refresh()} />

              {agent.task && (
                <span
                  className="text-xs text-mycel-text-2 truncate min-w-0 flex-shrink"
                  title={agent.task}
                >
                  {agent.task}
                </span>
              )}

              {hudLastSeen && (
                <span
                  className="text-xs text-mycel-muted tabular-nums shrink-0"
                  title={formatTime(hudLastSeen)}
                >
                  {formatRelative(hudLastSeen)}
                </span>
              )}
            </div>
          ),
          // ── actions slot: tabs ──
          actions: (
            <div className="flex items-center gap-0.5">
              {TABS.map((tab) => {
                const isActive = activeTab === tab.key;
                return (
                  <button
                    key={tab.key}
                    onClick={() => selectTab(tab.key)}
                    className={`relative px-2.5 py-1.5 text-[11px] font-medium tracking-wide uppercase transition-colors shrink-0 ${
                      isActive
                        ? "text-mycel-accent"
                        : "text-mycel-muted hover:text-mycel-text"
                    }`}
                  >
                    <span className="inline-flex items-center gap-1.5">
                      {tab.label}
                      <span
                        className={`inline-flex items-center justify-center min-w-[16px] h-[15px] rounded px-1 text-[9px] leading-none font-mono transition-colors ${
                          isActive
                            ? "bg-mycel-accent-subtle text-mycel-accent"
                            : "bg-mycel-surface-hover text-mycel-muted"
                        }`}
                        aria-hidden
                      >
                        {tab.shortcut}
                      </span>
                    </span>
                    {/* Active indicator — clean underline instead of the
                        old glow-bar; matches the surrounding restraint. */}
                    {isActive && (
                      <span className="absolute -bottom-px left-2 right-2 h-[2px] bg-mycel-accent rounded-full" />
                    )}
                  </button>
                );
              })}
            </div>
          ),
        }
      : { hidden: true },
  );

  /* ─── Loading / Error ─── */

  if (loading && !agent) {
    return (
      <div className="flex items-center justify-center h-full">
        <span className="text-sm text-mycel-muted">
          loading\u2026
        </span>
      </div>
    );
  }
  if (error && !agent) {
    return (
      <div className="p-6 space-y-3">
        <div className="text-sm text-mycel-error">
          error: {error}
        </div>
        <Link
          to={agentsUrl}
          className="text-xs text-mycel-accent hover:underline"
        >
          ← back to agents
        </Link>
      </div>
    );
  }
  if (!agent) return null;

  /* ─── Render ─── */

  return (
    <div className="flex flex-col h-full">
      {/* ═══ TAB CONTENT ═══ */}
      <div className="flex-1 min-h-0 flex flex-col">
        {activeTab === "live" && (
          <AgentToolStream
            agentName={agent.name}
            agentState={agent.state}
            agentTask={agent.task}
            agentTool={agent.tool}
            stoppedAt={agent.stopped_at}
            updatedAt={agent.updated_at}
            startedAt={agent.started_at}
            createdAt={agent.created_at}
          />
        )}
        {activeTab === "attach" && <AttachTab agent={agent} />}
        {activeTab === "settings" && <SettingsTab agent={agent} agentsUrl={agentsUrl} />}
        {activeTab === "metrics" && <MetricsTab agent={agent} />}
        {activeTab === "code" && <CodeTab agent={agent} />}
      </div>

    </div>
  );
}
