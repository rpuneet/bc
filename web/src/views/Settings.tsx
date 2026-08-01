import { useCallback, useState, useEffect, useMemo } from "react";
import { Link } from "react-router-dom";
import { api, type ProviderInfo, type BudgetStatus, type OnboardingState } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { useTheme } from "../context/ThemeContext";
import { PROVIDER_LABELS } from "./readiness/readiness";
import { WIZARD_STEPS } from "../wizard/types";
import { ThemePicker, RuntimePicker, AdvancedToggle, systemPrefersDark, type ThemeChoice, type RuntimeChoice } from "../settings/controls";

type SaveStatus = "idle" | "saving" | "saved" | "error";

/* ── Settings ──────────────────────────────────────────────────────────
 *
 * The ongoing-control mirror of the first-run wizard: the same options,
 * grouped for day-to-day use. Each section leads with a simple default and
 * tucks power-user knobs behind an Advanced expander (the same "▸ Advanced
 * settings" affordance the wizard uses). Setup lives at the top so the
 * wizard is a first-class part of Settings, not a link buried elsewhere.
 *
 * Settings is deliberately slim: it holds only config that has no other
 * home. Surfaces that already own a top-level page — Providers/Tools
 * (/tools) and Apps + notifications + keys (/apps) — appear as compact link
 * cards that summarize and route out, never as duplicate config UI.
 *
 * Dirty-tracking, per-section PATCH, restart detection, and the floating
 * save bar are preserved: edits to the config-backed sections (Profile,
 * Runtime, Advanced) flow through handleSaveAll. Budgets and Setup manage
 * their own state via their own endpoints. */

// Config top-level keys the save bar tracks and PATCHes. Order drives the
// "Unsaved: …" summary.
const SECTION_ORDER = ["user", "ui", "runtime", "storage", "server", "logs"];
// Sections whose changes require a daemon restart to take effect.
const RESTART_SECTIONS = new Set(["server", "storage", "runtime"]);
// Friendly names for the save bar (several config keys map to one UI section).
const SECTION_LABELS: Record<string, string> = {
  user: "Profile",
  ui: "Profile",
  runtime: "Runtime",
  storage: "Storage",
  server: "Server",
  logs: "Logs",
};

function deepClone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/* ------------------------------------------------------------------ */
/*  Shared primitives                                                   */
/* ------------------------------------------------------------------ */

const INPUT_CLS = "w-full px-2 py-0.5 text-xs rounded-md border border-mycel-border bg-mycel-bg text-mycel-text font-mono focus:outline-none focus:ring-1 focus:ring-mycel-accent";
const LINK_CLS = "inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-surface border border-mycel-border text-mycel-text hover:bg-mycel-surface-hover transition-colors";

function Field({ label, children, suffix }: { label: string; children: React.ReactNode; suffix?: string }) {
  return (
    <div className="flex items-center gap-2 min-h-[28px]">
      <label className="text-xs text-mycel-text-2 w-28 shrink-0 truncate" title={label}>{label}</label>
      <div className="flex-1 flex items-center gap-1.5 min-w-0">
        {children}
        {suffix && <span className="text-[10px] text-mycel-muted shrink-0">{suffix}</span>}
      </div>
    </div>
  );
}

function PasswordField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative w-full">
      <input
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`${INPUT_CLS} pr-8`}
      />
      <button
        type="button"
        onClick={() => setVisible(!visible)}
        className="absolute inset-y-0 right-0 flex items-center px-2 text-mycel-muted hover:text-mycel-text"
        tabIndex={-1}
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          {visible ? (
            <path strokeLinecap="round" strokeLinejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
          ) : (
            <>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
            </>
          )}
        </svg>
      </button>
    </div>
  );
}

/** A collapsed power-user block inside a section — the wizard's advanced card. */
function Advanced({ children, label }: { children: React.ReactNode; label?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="pt-1 space-y-2">
      <AdvancedToggle open={open} onToggle={() => setOpen((v) => !v)} label={label} />
      {open && (
        <div className="rounded-lg border border-mycel-border bg-mycel-bg p-3 space-y-1.5">{children}</div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Section wrapper                                                     */
/* ------------------------------------------------------------------ */

const SECTION_META: Record<string, { icon: React.ReactNode; desc: string }> = {
  setup: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />,
    desc: "First-run wizard",
  },
  profile: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />,
    desc: "Name and theme",
  },
  "providers & tools": {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />,
    desc: "Agent tools and default model",
  },
  runtime: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />,
    desc: "Where agents run",
  },
  apps: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />,
    desc: "Integrations, notifications, keys",
  },
  budgets: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />,
    desc: "Cost caps",
  },
  advanced: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />,
    desc: "Storage, server, logs, instructions",
  },
};

function Section({
  title,
  dirty,
  children,
  defaultOpen = true,
}: {
  title: string;
  dirty: boolean;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const meta = SECTION_META[title];

  return (
    <div className={`rounded-lg border ${dirty ? "border-mycel-accent" : "border-mycel-border"} bg-mycel-surface shadow-mycel`}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="w-full flex items-center gap-2 px-3 py-2 hover:bg-mycel-surface-hover transition-colors"
      >
        <svg className="w-3.5 h-3.5 text-mycel-muted shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          {meta?.icon}
        </svg>
        <span className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-text">{title}</span>
        {meta?.desc && <span className="text-[10px] text-mycel-muted ml-auto mr-2 hidden sm:inline">{meta.desc}</span>}
        {dirty && <span className="w-1.5 h-1.5 rounded-full bg-mycel-accent" />}
        <svg className={`w-3 h-3 text-mycel-muted transition-transform ${open ? "rotate-90" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
      {open && <div className="px-3 pb-3 pt-2 space-y-2 border-t border-mycel-border">{children}</div>}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Setup — onboarding progress + resume/re-run                          */
/* ------------------------------------------------------------------ */

function SetupSection() {
  const [state, setState] = useState<OnboardingState | null>(null);

  useEffect(() => {
    let alive = true;
    api.getOnboardingState()
      .then((s) => { if (alive) setState(s); })
      .catch(() => { /* Setup still offers Re-run without live state. */ });
    return () => { alive = false; };
  }, []);

  const total = WIZARD_STEPS.length;
  const done = (state?.completed ?? []).includes("done");
  const finished = (state?.completed ?? []).filter((s) => s !== "done").length;
  const pct = done ? 100 : Math.round((Math.min(finished, total) / total) * 100);
  const complete = done || pct >= 100;

  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-3">
        <p className="text-xs text-mycel-text-2 max-w-prose">
          {complete
            ? "Setup is complete. Re-run it any time to reconfigure — it only rewrites config and never touches your agents or connected apps."
            : "Finish first-run setup — machine checks, runtime, an agent tool, and your first agent. It only writes config; your agents and apps are left untouched."}
        </p>
        <Link to="/welcome" className={`${LINK_CLS} shrink-0`}>
          {complete ? "Re-run setup" : "Resume setup"}
        </Link>
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-[10px] text-mycel-muted">
          <span>{complete ? "Complete" : `Step ${Math.min(finished + 1, total)} of ${total}`}</span>
          <span>{pct}%</span>
        </div>
        <div className="h-1.5 rounded-full bg-mycel-bg border border-mycel-border overflow-hidden">
          <div className="h-full bg-mycel-accent transition-all" style={{ width: `${pct}%` }} />
        </div>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Link cards — surfaces that own a top-level page. Settings summarizes  */
/*  and points; it never duplicates their config UI.                     */
/* ------------------------------------------------------------------ */

function ProvidersToolsCard({ data }: { data: Record<string, unknown> }) {
  const p = (data.providers ?? {}) as Record<string, unknown>;
  const defaultProvider = String(p.default ?? "claude");
  const [installed, setInstalled] = useState<number | null>(null);
  const [total, setTotal] = useState<number | null>(null);

  useEffect(() => {
    let alive = true;
    api.listProviders()
      .then((list: ProviderInfo[]) => {
        if (!alive) return;
        setTotal(list.length);
        setInstalled(list.filter((pi) => pi.installed).length);
      })
      .catch(() => { /* summary degrades to the default provider alone */ });
    return () => { alive = false; };
  }, []);

  const counts = installed !== null && total !== null ? ` · ${installed}/${total} installed` : "";

  return (
    <div className="flex items-center justify-between gap-3 min-h-[28px]">
      <p className="text-xs text-mycel-text-2">
        <span className="text-mycel-text font-medium">Default: {PROVIDER_LABELS[defaultProvider] ?? defaultProvider}</span>
        {counts}. Install tools, sign in, and set the default model on the Tools page.
      </p>
      <Link to="/tools" className={`${LINK_CLS} shrink-0`}>Open Tools &amp; Providers →</Link>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Runtime                                                             */
/* ------------------------------------------------------------------ */

function RuntimeSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const r = (data.runtime ?? {}) as Record<string, unknown>;
  const mode = (String(r.default ?? "docker") === "docker" ? "docker" : "tmux") as RuntimeChoice;
  const docker = (r.docker ?? {}) as Record<string, unknown>;
  const tmux = (r.tmux ?? {}) as Record<string, unknown>;

  // The runtime pref stores "local" for tmux historically; map picker → pref.
  const setRuntime = (choice: RuntimeChoice) =>
    onChange(["runtime", "default"], choice === "docker" ? "docker" : "tmux");

  return (
    <div className="space-y-3">
      <RuntimePicker value={mode} onChange={setRuntime} />
      <Advanced label={mode === "docker" ? "Docker tuning" : "tmux tuning"}>
        {mode === "docker" ? (
          <>
            <Field label="Image"><input className={INPUT_CLS} value={String(docker.image ?? "")} onChange={(e) => onChange(["runtime", "docker", "image"], e.target.value)} /></Field>
            <Field label="Network"><input className={INPUT_CLS} value={String(docker.network ?? "")} onChange={(e) => onChange(["runtime", "docker", "network"], e.target.value)} /></Field>
            <Field label="Docker Socket"><input className={INPUT_CLS} value={String(docker.docker_socket_path ?? "")} onChange={(e) => onChange(["runtime", "docker", "docker_socket_path"], e.target.value)} /></Field>
            <Field label="CPUs"><input className={INPUT_CLS} type="number" value={Number(docker.cpus ?? 2)} onChange={(e) => onChange(["runtime", "docker", "cpus"], Number(e.target.value))} /></Field>
            <Field label="Memory" suffix="MB"><input className={INPUT_CLS} type="number" value={Number(docker.memory_mb ?? 4096)} onChange={(e) => onChange(["runtime", "docker", "memory_mb"], Number(e.target.value))} /></Field>
          </>
        ) : (
          <>
            <Field label="Session Prefix"><input className={INPUT_CLS} value={String(tmux.session_prefix ?? "")} onChange={(e) => onChange(["runtime", "tmux", "session_prefix"], e.target.value)} /></Field>
            <Field label="History Limit"><input className={INPUT_CLS} type="number" value={Number(tmux.history_limit ?? 10000)} onChange={(e) => onChange(["runtime", "tmux", "history_limit"], Number(e.target.value))} /></Field>
            <Field label="Default Shell"><input className={INPUT_CLS} value={String(tmux.default_shell ?? "")} onChange={(e) => onChange(["runtime", "tmux", "default_shell"], e.target.value)} /></Field>
          </>
        )}
      </Advanced>
    </div>
  );
}

function AppsCard() {
  const [count, setCount] = useState<number | null>(null);

  useEffect(() => {
    let alive = true;
    api.getApps()
      .then((res) => { if (alive) setCount((res.instances ?? []).filter((i) => i.connected).length); })
      .catch(() => { if (alive) setCount(0); });
    return () => { alive = false; };
  }, []);

  const summary =
    count === null ? "Loading…"
      : count === 0 ? "No apps connected"
        : `${count} app${count === 1 ? "" : "s"} connected`;

  return (
    <div className="flex items-center justify-between gap-3 min-h-[28px]">
      <p className="text-xs text-mycel-text-2">
        <span className="text-mycel-text font-medium">{summary}.</span>{" "}
        Manage integrations, notification delivery, and API keys on the Apps page.
      </p>
      <Link to="/apps" className={`${LINK_CLS} shrink-0`}>Manage apps →</Link>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Budgets — cost caps (own endpoints, not the settings PATCH)         */
/* ------------------------------------------------------------------ */

function BudgetsSection() {
  const fetcher = useCallback(() => api.getCostBudgets(), []);
  const { data, refresh } = usePolling<BudgetStatus[]>(fetcher, 30000);
  const budgets = data ?? [];
  const workspace = budgets.find((b) => b.scope === "workspace");

  const [limit, setLimit] = useState("");
  const [status, setStatus] = useState<SaveStatus>("idle");

  const wsLimit = workspace?.limit_usd;
  useEffect(() => {
    if (wsLimit !== undefined) setLimit(String(wsLimit));
  }, [wsLimit]);

  const save = async () => {
    const value = Number(limit);
    setStatus("saving");
    try {
      if (!limit.trim() || value <= 0) {
        await api.deleteCostBudget("workspace").catch(() => { /* nothing to remove */ });
      } else {
        await api.createCostBudget({ scope: "workspace", period: "monthly", limit_usd: value, alert_at: 0.8 });
      }
      refresh();
      setStatus("saved");
      setTimeout(() => setStatus("idle"), 2000);
    } catch {
      setStatus("error");
    }
  };

  const perAgent = budgets.filter((b) => b.scope.startsWith("agent:"));

  return (
    <div className="space-y-3">
      <Field label="Monthly cap" suffix="alerts at 80%">
        <div className="flex items-center gap-1.5 flex-1">
          <span className="text-xs text-mycel-muted">$</span>
          <input
            type="number"
            min={0}
            step={5}
            value={limit}
            placeholder="No limit"
            onChange={(e) => setLimit(e.target.value)}
            className={INPUT_CLS}
          />
          <button
            type="button"
            onClick={save}
            disabled={status === "saving"}
            className="shrink-0 inline-flex items-center h-7 px-2.5 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-all disabled:opacity-50"
          >
            {status === "saving" ? "Saving…" : "Save"}
          </button>
        </div>
      </Field>
      {status === "saved" && <p className="text-[11px] text-mycel-success">Budget saved.</p>}
      {status === "error" && <p className="text-[11px] text-mycel-error">Couldn&apos;t save the budget.</p>}

      {perAgent.length > 0 && (
        <div className="rounded-lg border border-mycel-border bg-mycel-bg divide-y divide-mycel-border overflow-hidden">
          {perAgent.map((b) => (
            <div key={b.scope} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
              <span className="text-mycel-text truncate">{b.scope.replace(/^agent:/, "")}</span>
              <span className="text-mycel-muted shrink-0">${b.limit_usd}/{b.period}</span>
              <button
                type="button"
                onClick={() => void api.deleteCostBudget(b.scope).then(refresh)}
                className="text-mycel-muted hover:text-mycel-error shrink-0"
                title="Remove cap"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
      <p className="text-[11px] text-mycel-muted">
        Set per-agent caps from an agent&apos;s page. Caps compare against spend computed from provider
        usage.
      </p>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Advanced — storage, server, logs, injected instructions             */
/* ------------------------------------------------------------------ */

function StorageFields({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const s = (data.storage ?? {}) as Record<string, unknown>;
  const backend = String(s.default ?? "sqlite");
  const ts = (s.timescale ?? {}) as Record<string, unknown>;
  const sq = (s.sqlite ?? {}) as Record<string, unknown>;

  return (
    <>
      <Field label="Backend">
        <select value={backend} onChange={(e) => onChange(["storage", "default"], e.target.value)} className={INPUT_CLS}>
          <option value="sqlite">SQLite (default)</option>
          <option value="timescale">TimescaleDB</option>
        </select>
      </Field>
      {backend === "timescale" ? (
        <>
          <Field label="Host"><input className={INPUT_CLS} value={String(ts.host ?? "")} onChange={(e) => onChange(["storage", "timescale", "host"], e.target.value)} /></Field>
          <Field label="Port"><input className={INPUT_CLS} type="number" value={Number(ts.port ?? 5432)} onChange={(e) => onChange(["storage", "timescale", "port"], Number(e.target.value))} /></Field>
          <Field label="User"><input className={INPUT_CLS} value={String(ts.user ?? "")} onChange={(e) => onChange(["storage", "timescale", "user"], e.target.value)} /></Field>
          <Field label="Password"><PasswordField value={String(ts.password ?? "")} onChange={(v) => onChange(["storage", "timescale", "password"], v)} /></Field>
          <Field label="Database"><input className={INPUT_CLS} value={String(ts.database ?? "")} onChange={(e) => onChange(["storage", "timescale", "database"], e.target.value)} /></Field>
        </>
      ) : (
        <Field label="Path"><input className={INPUT_CLS} value={String(sq.path ?? "")} onChange={(e) => onChange(["storage", "sqlite", "path"], e.target.value)} /></Field>
      )}
    </>
  );
}

function ServerFields({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const s = (data.server ?? {}) as Record<string, unknown>;
  const configuredPort = Number(s.port ?? 0);
  const actualPort = typeof window !== "undefined" && window.location.port ? Number(window.location.port) : 0;
  const portDrift = actualPort > 0 && actualPort !== configuredPort;
  return (
    <>
      <p className="text-[11px] text-mycel-muted">
        The daemon binds loopback (127.0.0.1) by default and the desktop app fixes it there. Only
        change these if you run mycel as a shared server.
      </p>
      <Field label="Host"><input className={INPUT_CLS} value={String(s.host ?? "")} onChange={(e) => onChange(["server", "host"], e.target.value)} /></Field>
      <Field label="Port" suffix={portDrift ? `running on ${actualPort}` : actualPort > 0 ? "live" : undefined}>
        <input className={INPUT_CLS} type="number" value={configuredPort} onChange={(e) => onChange(["server", "port"], Number(e.target.value))} />
      </Field>
      <Field label="CORS Origin"><input className={INPUT_CLS} value={String(s.cors_origin ?? "")} onChange={(e) => onChange(["server", "cors_origin"], e.target.value)} /></Field>
    </>
  );
}

function LogsFields({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const l = (data.logs ?? {}) as Record<string, unknown>;
  const maxBytes = Number(l.max_bytes ?? 0);
  const maxMB = Math.round(maxBytes / 1048576);
  return (
    <>
      <Field label="Path"><input className={INPUT_CLS} value={String(l.path ?? "")} onChange={(e) => onChange(["logs", "path"], e.target.value)} /></Field>
      <Field label="Max Size" suffix="MB"><input className={INPUT_CLS} type="number" value={maxMB} onChange={(e) => onChange(["logs", "max_bytes"], Number(e.target.value) * 1048576)} /></Field>
    </>
  );
}

function InjectedInstructionsSection() {
  const [text, setText] = useState("");
  const [original, setOriginal] = useState("");
  const [status, setStatus] = useState<SaveStatus>("idle");

  useEffect(() => {
    let alive = true;
    api.getInjectedInstructions()
      .then((res) => {
        if (!alive) return;
        setText(res.injected_instructions ?? "");
        setOriginal(res.injected_instructions ?? "");
      })
      .catch(() => { /* leave empty on load failure */ });
    return () => { alive = false; };
  }, []);

  const dirty = text !== original;

  const handleSave = async () => {
    setStatus("saving");
    try {
      const res = await api.updateInjectedInstructions(text);
      const saved = res.injected_instructions ?? "";
      setText(saved);
      setOriginal(saved);
      setStatus("saved");
      setTimeout(() => setStatus("idle"), 2000);
    } catch {
      setStatus("error");
    }
  };

  return (
    <div className="space-y-2">
      <p className="text-[11px] text-mycel-muted">
        Appended to every agent&apos;s prompt file at spawn, followed by an auto-generated summary of
        its MCP servers and credential env var names. Secret values are never included.
      </p>
      <textarea
        className={`${INPUT_CLS} h-32 resize-y leading-relaxed`}
        placeholder="e.g. Always report status before and after work. Prefer small PRs."
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      <div className="flex items-center justify-end gap-2">
        {status === "saved" && <span className="text-xs text-mycel-success">Saved</span>}
        {status === "error" && <span className="text-xs text-mycel-error">Save failed</span>}
        <button
          type="button"
          onClick={handleSave}
          disabled={status === "saving" || !dirty}
          className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-all disabled:opacity-50"
        >
          {status === "saving" ? "Saving..." : "Save"}
        </button>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main Settings page                                                  */
/* ------------------------------------------------------------------ */

export function Settings() {
  const fetcher = useCallback(() => api.getSettings(), []);
  const { data: config, loading, error, refresh, timedOut } = usePolling(fetcher, 30000);
  const { setTheme } = useTheme();

  const [edited, setEdited] = useState<Record<string, unknown> | null>(null);
  const [original, setOriginal] = useState<Record<string, unknown> | null>(null);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const [restartWarning, setRestartWarning] = useState(false);

  useEffect(() => {
    if (config) {
      const raw = config as unknown as Record<string, unknown>;
      setOriginal(deepClone(raw));
      setEdited((prev) => (prev === null ? deepClone(raw) : prev));
    }
  }, [config]);

  const dirtySections = useMemo(() => {
    if (!edited || !original) return [];
    return SECTION_ORDER.filter(
      (key) => key in edited && !deepEqual(original[key], edited[key])
    );
  }, [edited, original]);

  // De-duplicated friendly labels for the save bar (user+ui → one "Profile").
  const dirtyLabels = useMemo(
    () => Array.from(new Set(dirtySections.map((k) => SECTION_LABELS[k] ?? k))),
    [dirtySections],
  );

  const handleChange = (path: string[], newValue: unknown) => {
    if (!edited || path.length === 0) return;
    const next = deepClone(edited);
    let cursor: Record<string, unknown> = next;
    for (let i = 0; i < path.length - 1; i++) {
      const k = path[i]!;
      if (typeof cursor[k] !== "object" || cursor[k] === null) {
        cursor[k] = {};
      }
      cursor = cursor[k] as Record<string, unknown>;
    }
    cursor[path[path.length - 1]!] = newValue;
    setEdited(next);
  };

  // Theme lives in two coupled fields (ui.theme + ui.mode). Set both in one
  // clone and preview live so the page reflects the choice immediately.
  const handleThemeChange = (choice: ThemeChoice) => {
    if (!edited) return;
    const resolved = choice === "system" ? (systemPrefersDark() ? "dark" : "light") : choice;
    setTheme(resolved);
    const next = deepClone(edited);
    const ui = (typeof next.ui === "object" && next.ui !== null ? next.ui : {}) as Record<string, unknown>;
    next.ui = { ...ui, theme: resolved, mode: choice === "system" ? "auto" : resolved };
    setEdited(next);
  };

  const currentThemeChoice = ((): ThemeChoice => {
    const ui = (edited?.ui ?? {}) as Record<string, unknown>;
    if (String(ui.mode ?? "") === "auto") return "system";
    const t = String(ui.theme ?? "dark");
    return t === "light" ? "light" : "dark";
  })();

  const revertTheme = () => {
    const ui = (original?.ui ?? {}) as Record<string, unknown>;
    const t = String(ui.theme ?? "dark");
    setTheme(t === "light" ? "light" : "dark");
  };

  const handleSaveAll = async () => {
    if (!edited || dirtySections.length === 0) return;
    setSaveStatus("saving");
    try {
      const patch: Record<string, unknown> = {};
      for (const key of dirtySections) {
        patch[key] = edited[key];
      }
      await api.updateSettings(patch);
      const needsRestart = dirtySections.some((k) => RESTART_SECTIONS.has(k));
      refresh();
      setOriginal((prev) => {
        if (!prev) return prev;
        const next = deepClone(prev);
        for (const key of dirtySections) {
          next[key] = deepClone(edited[key]);
        }
        return next;
      });
      setSaveStatus("saved");
      if (needsRestart) setRestartWarning(true);
      setTimeout(() => setSaveStatus("idle"), 2000);
    } catch {
      setSaveStatus("error");
    }
  };

  if (loading && !config)
    return <div className="p-6"><LoadingSkeleton variant="cards" rows={4} /></div>;
  if (timedOut && !config)
    return <div className="p-6"><EmptyState icon="!" title="Settings timed out" actionLabel="Retry" onAction={refresh} /></div>;
  if (error && !config)
    return <div className="p-6"><EmptyState icon="!" title="Failed to load settings" description={error} actionLabel="Retry" onAction={refresh} /></div>;
  if (!config || !edited || !original) return null;

  const version = edited.version;
  const isDirty = (...keys: string[]) => keys.some((k) => dirtySections.includes(k));
  const userName = String(((edited.user ?? {}) as Record<string, unknown>).name ?? "");

  return (
    <div className="p-4 md:p-6 space-y-3 max-w-3xl mx-auto">
      <div className="flex items-center gap-2">
        <span className="inline-flex items-center gap-1.5 text-[10px] font-mono px-2 py-1 rounded-md border border-mycel-border bg-mycel-surface-hover text-mycel-muted">
          <svg width="10" height="10" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3 1.5h5l3 3v8H3z" />
            <path d="M8 1.5v3h3" />
          </svg>
          preferences.json{typeof version !== "undefined" ? ` · v${version}` : ""}
        </span>
      </div>

      {saveStatus === "saved" && dirtySections.length === 0 && (
        <div className="fixed bottom-4 right-4 z-30 rounded-lg border border-mycel-success bg-mycel-success-subtle px-3 py-2 text-xs text-mycel-success shadow-mycel-lg">
          Saved
        </div>
      )}

      {/* Floating save bar */}
      {dirtySections.length > 0 && (
        <div className="sticky top-0 z-20 rounded-lg border border-mycel-accent bg-mycel-accent-subtle backdrop-blur px-3 py-2 flex items-center justify-between gap-3 shadow-mycel">
          <div className="text-xs text-mycel-text min-w-0">
            <span className="font-medium">Unsaved:</span>{" "}
            <span className="text-mycel-muted">{dirtyLabels.join(", ")}</span>
            {dirtySections.some((k) => RESTART_SECTIONS.has(k)) && (
              <span className="ml-2 text-mycel-accent">Restart required after save</span>
            )}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              type="button"
              onClick={() => {
                if (original) setEdited(deepClone(original));
                revertTheme();
                setSaveStatus("idle");
              }}
              disabled={saveStatus === "saving"}
              className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-50"
              title="Discard unsaved changes"
            >
              Discard
            </button>
            <button
              onClick={handleSaveAll}
              disabled={saveStatus === "saving"}
              className={`inline-flex items-center h-8 px-3 rounded-md text-xs font-medium transition-all disabled:opacity-50 ${
                saveStatus === "error"
                  ? "bg-mycel-error text-white hover:opacity-90 shadow-mycel-sm"
                  : "bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm"
              }`}
            >
              {saveStatus === "saving" ? "Saving..." : saveStatus === "error" ? "Retry" : "Save"}
            </button>
          </div>
        </div>
      )}

      {restartWarning && (
        <div className="rounded-md border border-mycel-error bg-mycel-error-subtle px-3 py-1.5 text-xs text-mycel-error">
          Changes saved. Restart mycel to apply (<code className="font-mono">mycel down &amp;&amp; mycel up -d</code>)
        </div>
      )}

      <Section title="setup" dirty={false}>
        <SetupSection />
      </Section>

      <Section title="profile" dirty={isDirty("user", "ui")}>
        <Field label="Name">
          <input
            className={INPUT_CLS}
            value={userName}
            maxLength={30}
            placeholder="Your name"
            onChange={(e) => handleChange(["user", "name"], e.target.value)}
          />
        </Field>
        <div className="flex items-start gap-2 min-h-[28px] pt-1">
          <label className="text-xs text-mycel-text-2 w-28 shrink-0">Theme</label>
          <ThemePicker value={currentThemeChoice} onChange={handleThemeChange} />
        </div>
      </Section>

      {/* Providers/Tools own the /tools page; Settings only summarizes. The
          default-model + per-provider controls that write prefs.providers
          live there (follow-up PR). */}
      <Section title="providers & tools" dirty={false}>
        <ProvidersToolsCard data={edited} />
      </Section>

      <Section title="runtime" dirty={isDirty("runtime")}>
        <RuntimeSection data={edited} onChange={handleChange} />
      </Section>

      <Section title="budgets" dirty={false}>
        <BudgetsSection />
      </Section>

      {/* Apps owns integrations, notification delivery, and API keys on the
          /apps page; Settings only summarizes and links out. The
          prefs.notifications{} controls surface there (follow-up PR). */}
      <Section title="apps" dirty={false}>
        <AppsCard />
      </Section>

      <Section title="advanced" dirty={isDirty("storage", "server", "logs")} defaultOpen={false}>
        <Advanced label="Storage">
          <StorageFields data={edited} onChange={handleChange} />
        </Advanced>
        <Advanced label="Server">
          <ServerFields data={edited} onChange={handleChange} />
        </Advanced>
        <Advanced label="Logs">
          <LogsFields data={edited} onChange={handleChange} />
        </Advanced>
        <Advanced label="Injected instructions">
          <InjectedInstructionsSection />
        </Advanced>
      </Section>
    </div>
  );
}
