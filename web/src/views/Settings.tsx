import { useCallback, useState, useEffect, useMemo, useId, Children, isValidElement, cloneElement, type ReactElement } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type AppInstance, type BudgetStatus } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { useTheme } from "../context/ThemeContext";
import { ThemePicker, RuntimePicker, AdvancedToggle, systemPrefersDark, type ThemeChoice, type RuntimeChoice } from "../settings/controls";
import { ProvidersToolsSection } from "../settings/ProvidersToolsSection";
import { useProgressiveReveal, REVEAL_ORDER, type RevealState } from "../settings/useProgressiveReveal";

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

// Sans by default — a settings page is read prose, not a config dump. Taller
// hit target, a border that firms up and an accent ring on focus.
const INPUT_CLS = "w-full px-2.5 py-1.5 text-[13px] rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted outline-none focus-visible:border-mycel-accent focus-visible:ring-2 focus-visible:ring-mycel-accent transition-colors";
// Mono variant — reserved for genuinely technical values (paths, images,
// sockets, shells) where character alignment actually helps.
const MONO_INPUT_CLS = `${INPUT_CLS} font-mono`;

// Form elements a Field can auto-associate its label with via `htmlFor` —
// native controls get the id cloned straight on; PasswordField forwards it
// to its own internal <input> (see below).
const FIELD_CONTROL_TAGS = new Set(["input", "select", "textarea"]);

function Field({ label, children, suffix }: { label: string; children: React.ReactNode; suffix?: string }) {
  const autoId = useId();
  let assigned = false;
  const kids = Children.map(children, (child) => {
    if (assigned || !isValidElement(child)) return child;
    const isNativeControl = typeof child.type === "string" && FIELD_CONTROL_TAGS.has(child.type);
    const isPasswordField = child.type === PasswordField;
    if (!isNativeControl && !isPasswordField) return child;
    assigned = true;
    const props = child.props as { id?: string };
    return cloneElement(child as ReactElement<{ id?: string }>, { id: props.id ?? autoId });
  });
  return (
    <div className="flex items-center gap-3 min-h-[34px]">
      <label htmlFor={autoId} className="text-[12px] text-mycel-text-2 w-28 shrink-0 truncate" title={label}>{label}</label>
      <div className="flex-1 flex items-center gap-1.5 min-w-0">
        {kids}
        {suffix && <span className="text-[10px] text-mycel-muted shrink-0">{suffix}</span>}
      </div>
    </div>
  );
}

function PasswordField({ value, onChange, id }: { value: string; onChange: (v: string) => void; id?: string }) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative w-full">
      <input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`${INPUT_CLS} pr-8`}
      />
      <button
        type="button"
        onClick={() => setVisible(!visible)}
        aria-label={visible ? "Hide password" : "Show password"}
        aria-pressed={visible}
        className="absolute inset-y-0 right-0 flex items-center px-2 text-mycel-muted hover:text-mycel-text outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent rounded-md before:absolute before:-inset-2.5 before:content-['']"
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

/**
 * Section's `reveal` prop drives progressive disclosure. There is no
 * separate "wizard mode" — every Section always renders; `reveal` just
 * says whether *this* first-run install has satisfied every section
 * ahead of it yet:
 *
 *   - "locked":   header only, greyed, no body — never fetches/renders
 *                 children, so a locked provider table etc. never fires
 *                 its request before it's reachable.
 *   - "active":   forced open, a soft entrance, and a "complete this to
 *                 continue" CTA beneath the header.
 *   - "complete": the normal, day-to-day collapsed/expandable section.
 *
 * Omit `reveal` (or pass undefined) for sections that are never gated
 * (Setup, Advanced) — they render exactly as before.
 */
function Section({
  title,
  dirty,
  children,
  defaultOpen = true,
  index = 0,
  reveal,
  onContinue,
}: {
  title: string;
  dirty: boolean;
  children: React.ReactNode;
  defaultOpen?: boolean;
  index?: number;
  reveal?: RevealState;
  /** Rendered as the CTA button when reveal === "active" and the section
   *  has no real field to force completion (runtime, apps, budgets). */
  onContinue?: () => void;
}) {
  const locked = reveal === "locked";
  const active = reveal === "active";
  const [open, setOpen] = useState(active ? true : defaultOpen);

  // An active section is always open — it's the one thing the user
  // still needs to do.
  useEffect(() => {
    if (active) setOpen(true);
  }, [active]);

  const meta = SECTION_META[title];

  return (
    <div
      className={`rounded-xl border overflow-hidden transition-colors ${
        active ? "animate-reveal border-mycel-accent bg-mycel-surface shadow-mycel-lg" :
        locked ? "border-mycel-border bg-mycel-bg opacity-60" :
        `animate-reveal ${dirty ? "border-mycel-accent" : "border-mycel-border"} bg-mycel-surface shadow-mycel`
      }`}
      style={{ animationDelay: `${index * 45}ms` }}
      data-reveal={reveal}
    >
      <button
        type="button"
        onClick={() => !locked && setOpen(!open)}
        aria-expanded={!locked && open}
        aria-disabled={locked}
        disabled={locked}
        className={`w-full flex items-center gap-3 px-4 py-3 text-left transition-colors ${locked ? "cursor-not-allowed" : "hover:bg-mycel-surface-hover"}`}
      >
        <span className={`grid place-items-center w-8 h-8 rounded-lg shrink-0 transition-colors ${open && !locked ? "bg-mycel-accent-subtle text-mycel-accent" : "bg-mycel-bg text-mycel-muted"}`}>
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.6}>
            {meta?.icon}
          </svg>
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13.5px] font-semibold capitalize tracking-tight text-mycel-text leading-tight">{title}</span>
          {meta?.desc && <span className="block text-[11px] text-mycel-muted truncate">{meta.desc}</span>}
        </span>
        {locked && (
          <svg className="w-3.5 h-3.5 text-mycel-muted shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-label="Locked">
            <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
          </svg>
        )}
        {dirty && !locked && (
          <span className="inline-flex items-center gap-1.5 shrink-0 text-[10px] font-medium text-mycel-accent">
            <span className="w-1.5 h-1.5 rounded-full bg-mycel-accent" />
            Edited
          </span>
        )}
        {!locked && (
          <svg className={`w-4 h-4 text-mycel-muted shrink-0 transition-transform duration-200 ${open ? "rotate-90" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
        )}
      </button>
      {/* Locked sections never mount their body — no fetch, no children
          render — until a prior section completes. */}
      {!locked && open && (
        <div className="px-4 pb-4 pt-3 space-y-2.5 border-t border-mycel-border">
          {children}
          {active && onContinue && (
            <div className="flex items-center justify-between gap-3 rounded-lg border border-mycel-accent bg-mycel-accent-subtle px-3 py-2">
              <span className="text-[11.5px] text-mycel-text-2">Complete this to continue setup.</span>
              <button
                type="button"
                onClick={onContinue}
                className="inline-flex items-center h-7 px-3 rounded-md text-[11.5px] font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover transition-colors shrink-0"
              >
                Continue
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Setup — progress derived from the same reveal states that gate the  */
/*  sections below. Re-running setup lives in the page header (the      */
/*  circular re-run icon) — there is no separate wizard to link to.     */
/* ------------------------------------------------------------------ */

function SetupSection({
  completedCount,
  total,
}: {
  completedCount: number;
  total: number;
}) {
  // Rendered only while setup is unfinished (see the call site), so there is no
  // "complete" state to draw here.
  const pct = Math.round((Math.min(completedCount, total) / total) * 100);

  return (
    <div className="space-y-3">
      <p className="text-xs text-mycel-text-2 max-w-prose">
        Sections below unlock one at a time as you finish each — machine checks, runtime, an agent
        tool, and your first agent. It only writes config; your agents and apps are left untouched.
        When you create an agent, pick its git repo with Browse (native folder picker + scan) —
        the daemon does not need a project workspace.
      </p>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-[11px]">
          <span className="inline-flex items-center gap-1.5 font-medium text-mycel-text-2">
            {`${String(completedCount)} of ${String(total)} sections done`}
          </span>
          <span className="text-mycel-muted tabular-nums">{pct}%</span>
        </div>
        <div className="h-2 rounded-full bg-mycel-bg border border-mycel-border overflow-hidden">
          <div
            className="h-full rounded-full transition-all duration-500 ease-out bg-gradient-to-r from-mycel-accent to-mycel-accent-hover"
            style={{ width: `${Math.max(pct, 4)}%` }}
          />
        </div>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Link cards — surfaces that own a top-level page. Settings summarizes  */
/*  and points; it never duplicates their config UI.                     */
/* ------------------------------------------------------------------ */

/* A quiet status dot + label for the compact summary tables. */
function MiniStatus({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={`inline-flex items-center gap-1.5 text-[11px] ${ok ? "text-mycel-success" : "text-mycel-muted"}`}>
      <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${ok ? "bg-mycel-success" : "bg-mycel-muted"}`} aria-hidden />
      {label}
    </span>
  );
}

/* The compact table shell used by every drilldown summary: a bordered,
 * rounded table whose header sits on the surface tint. */
function SummaryTable({ head, children }: { head: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-mycel-border overflow-hidden">
      <table className="w-full text-left">
        <thead>
          <tr className="bg-mycel-bg border-b border-mycel-border text-[10px] text-mycel-muted uppercase tracking-[0.08em]">
            {head}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

/* The "Open the full manager →" footer link every summary table drills into. */
function DrilldownFooter({ to, label }: { to: string; label: string }) {
  return (
    <Link
      to={to}
      className="group flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-[11.5px] text-mycel-text-2 hover:text-mycel-accent hover:bg-mycel-surface-hover transition-colors"
    >
      <span>{label}</span>
      <span className="text-mycel-muted group-hover:text-mycel-accent group-hover:translate-x-0.5 transition-all" aria-hidden>→</span>
    </Link>
  );
}

/* ------------------------------------------------------------------ */
/*  Runtime                                                             */
/* ------------------------------------------------------------------ */

/** Repeatable host:container[:ro] bind-mount list for the Docker runtime.
 *  Each row is a free-form mount spec (docker -v syntax); rows can be added,
 *  edited in place, or removed. Empty rows are dropped on blur. */
function ExtraMountsField({ mounts, onChange }: { mounts: string[]; onChange: (next: string[]) => void }) {
  const setRow = (i: number, v: string) => {
    const next = [...mounts];
    next[i] = v;
    onChange(next);
  };
  const removeRow = (i: number) => onChange(mounts.filter((_, idx) => idx !== i));
  const addRow = () => onChange([...mounts, ""]);

  return (
    <div className="space-y-1.5">
      <label className="text-[12px] text-mycel-text-2 block">Extra Mounts</label>
      <div className="space-y-1.5">
        {mounts.map((m, i) => (
          <div key={i} className="flex items-center gap-1.5">
            <input
              className={MONO_INPUT_CLS}
              value={m}
              placeholder="/host/path:/container/path[:ro]"
              onChange={(e) => setRow(i, e.target.value)}
            />
            <button
              type="button"
              onClick={() => removeRow(i)}
              aria-label={`Remove mount ${i + 1}`}
              className="shrink-0 text-mycel-muted hover:text-mycel-error text-sm px-1.5 py-1 rounded-md border border-mycel-border hover:border-mycel-error transition-colors"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={addRow}
        className="text-[11.5px] text-mycel-accent hover:underline"
      >
        + Add mount
      </button>
    </div>
  );
}

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
            <Field label="Image"><input className={MONO_INPUT_CLS} value={String(docker.image ?? "")} onChange={(e) => onChange(["runtime", "docker", "image"], e.target.value)} /></Field>
            <Field label="Network"><input className={MONO_INPUT_CLS} value={String(docker.network ?? "")} onChange={(e) => onChange(["runtime", "docker", "network"], e.target.value)} /></Field>
            <Field label="Docker Socket"><input className={MONO_INPUT_CLS} value={String(docker.docker_socket_path ?? "")} onChange={(e) => onChange(["runtime", "docker", "docker_socket_path"], e.target.value)} /></Field>
            <Field label="CPUs"><input className={INPUT_CLS} type="number" value={Number(docker.cpus ?? 2)} onChange={(e) => onChange(["runtime", "docker", "cpus"], Number(e.target.value))} /></Field>
            <Field label="Memory" suffix="MB"><input className={INPUT_CLS} type="number" value={Number(docker.memory_mb ?? 4096)} onChange={(e) => onChange(["runtime", "docker", "memory_mb"], Number(e.target.value))} /></Field>
            <ExtraMountsField
              mounts={Array.isArray(docker.extra_mounts) ? (docker.extra_mounts as string[]) : []}
              onChange={(next) => onChange(["runtime", "docker", "extra_mounts"], next)}
            />
          </>
        ) : (
          <>
            <Field label="Session Prefix"><input className={MONO_INPUT_CLS} value={String(tmux.session_prefix ?? "")} onChange={(e) => onChange(["runtime", "tmux", "session_prefix"], e.target.value)} /></Field>
            <Field label="History Limit"><input className={INPUT_CLS} type="number" value={Number(tmux.history_limit ?? 10000)} onChange={(e) => onChange(["runtime", "tmux", "history_limit"], Number(e.target.value))} /></Field>
            <Field label="Default Shell"><input className={MONO_INPUT_CLS} value={String(tmux.default_shell ?? "")} onChange={(e) => onChange(["runtime", "tmux", "default_shell"], e.target.value)} /></Field>
          </>
        )}
      </Advanced>
    </div>
  );
}

/* Apps drilldown: a compact table of connected integrations (platform ·
 * status), summarizing the /apps manager. Rows and footer drill into /apps. */
function AppsCard() {
  const navigate = useNavigate();
  const [instances, setInstances] = useState<AppInstance[] | null>(null);

  useEffect(() => {
    let alive = true;
    api.getApps()
      .then((res) => { if (alive) setInstances(res.instances ?? []); })
      .catch(() => { if (alive) setInstances([]); });
    return () => { alive = false; };
  }, []);

  return (
    <div className="space-y-2.5">
      {instances === null ? (
        <div className="h-16 animate-pulse rounded-lg bg-mycel-surface-hover" />
      ) : instances.length === 0 ? (
        <p className="text-[11.5px] text-mycel-muted px-1">No apps connected yet.</p>
      ) : (
        <SummaryTable
          head={<>
            <th className="px-3 py-1.5 font-medium">App</th>
            <th className="px-3 py-1.5 font-medium">Platform</th>
            <th className="px-3 py-1.5 font-medium">Status</th>
          </>}
        >
          {instances.map((inst) => (
            <tr
              key={inst.name}
              onClick={() => navigate("/apps")}
              className="border-b border-mycel-border last:border-0 hover:bg-mycel-surface-hover cursor-pointer transition-colors"
            >
              <td className="px-3 py-1.5 text-[12px] font-medium text-mycel-text">{inst.name}</td>
              <td className="px-3 py-1.5 text-[11px] text-mycel-muted">{inst.app}</td>
              <td className="px-3 py-1.5">
                <MiniStatus ok={inst.connected} label={inst.connected ? "Connected" : inst.error ? "Error" : "Disconnected"} />
              </td>
            </tr>
          ))}
        </SummaryTable>
      )}
      <DrilldownFooter to="/apps" label="Integrations, notification delivery, and API keys" />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Budgets — cost caps as a small list of scope→limit rows. Same        */
/*  endpoints Insights' BudgetPanel uses (GET/POST/DELETE                */
/*  /api/costs/budgets); pkg/cost.BudgetConfig is {period, limit_usd,    */
/*  alert_at, hard_stop}. Entirely optional — never gates reveal.       */
/* ------------------------------------------------------------------ */

function BudgetsSection() {
  const fetcher = useCallback(() => api.getCostBudgets(), []);
  const { data, refresh } = usePolling<BudgetStatus[]>(fetcher, 30000);
  const budgets = Array.isArray(data) ? data : [];
  const workspace = budgets.find((b) => b.scope === "workspace");
  const perAgent = budgets.filter((b) => b.scope.startsWith("agent:"));

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

  return (
    <div className="space-y-2.5">
      <Field label="Monthly cap" suffix="alerts at 80%">
        <span className="text-mycel-muted">$</span>
        <input
          type="number"
          min={0}
          step={5}
          value={limit}
          placeholder="No limit"
          onChange={(e) => setLimit(e.target.value)}
          aria-label="Monthly cost cap in dollars"
          className={INPUT_CLS}
        />
        <button
          type="button"
          onClick={() => { void save(); }}
          disabled={status === "saving"}
          className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover active:scale-[0.98] shadow-mycel-sm transition-all disabled:opacity-60 shrink-0"
        >
          {status === "saving" ? "Saving…" : "Save"}
        </button>
      </Field>
      {status === "saved" && <p aria-live="polite" className="text-[11px] text-mycel-success">Saved</p>}
      {status === "error" && <p role="alert" className="text-[11px] text-mycel-error">Couldn&apos;t save the cap.</p>}

      {perAgent.length > 0 && (
        <div className="rounded-lg border border-mycel-border bg-mycel-bg divide-y divide-mycel-border overflow-hidden">
          {perAgent.map((b) => (
            <div key={b.scope} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
              <span className="text-mycel-text truncate">{b.scope.replace(/^agent:/, "")}</span>
              <span className="text-mycel-muted shrink-0 tabular-nums">${b.limit_usd}/{b.period}</span>
              <button
                type="button"
                onClick={() => void api.deleteCostBudget(b.scope).then(refresh)}
                className="text-mycel-muted hover:text-mycel-error shrink-0 cursor-pointer transition-colors"
                title="Remove cap"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
      <p className="text-[11px] text-mycel-muted">
        Per-agent caps are set from an agent&apos;s own page and listed here read-only. Caps compare against spend computed from provider usage — no separate cost ledger.
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
        <Field label="Path"><input className={MONO_INPUT_CLS} value={String(sq.path ?? "")} onChange={(e) => onChange(["storage", "sqlite", "path"], e.target.value)} /></Field>
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
      <Field label="Host"><input className={MONO_INPUT_CLS} value={String(s.host ?? "")} onChange={(e) => onChange(["server", "host"], e.target.value)} /></Field>
      <Field label="Port" suffix={portDrift ? `running on ${actualPort}` : actualPort > 0 ? "live" : undefined}>
        <input className={INPUT_CLS} type="number" value={configuredPort} onChange={(e) => onChange(["server", "port"], Number(e.target.value))} />
      </Field>
      <Field label="CORS Origin"><input className={MONO_INPUT_CLS} value={String(s.cors_origin ?? "")} onChange={(e) => onChange(["server", "cors_origin"], e.target.value)} /></Field>
    </>
  );
}

function LogsFields({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const l = (data.logs ?? {}) as Record<string, unknown>;
  const maxBytes = Number(l.max_bytes ?? 0);
  const maxMB = Math.round(maxBytes / 1048576);
  return (
    <>
      <Field label="Path"><input className={MONO_INPUT_CLS} value={String(l.path ?? "")} onChange={(e) => onChange(["logs", "path"], e.target.value)} /></Field>
      <Field label="Max Size" suffix="MB"><input className={INPUT_CLS} type="number" value={maxMB} onChange={(e) => onChange(["logs", "max_bytes"], Number(e.target.value) * 1048576)} /></Field>
    </>
  );
}

/** MycelHome (install root) + remembered projects scan root for Create Agent Browse. */
function MycelHomeFields() {
  const [mycelHome, setMycelHome] = useState("");
  const [scanRoot, setScanRoot] = useState(() => {
    try {
      return localStorage.getItem("mycel.projects_scan_root") ?? "";
    } catch {
      return "";
    }
  });
  const [picking, setPicking] = useState(false);
  const [hint, setHint] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api.getSystemInfo()
      .then((info) => {
        if (!alive) return;
        setMycelHome(info.mycel_home ?? "");
      })
      .catch(() => {
        /* best-effort */
      });
    return () => { alive = false; };
  }, []);

  const persistScanRoot = (path: string) => {
    setScanRoot(path);
    try {
      if (path) localStorage.setItem("mycel.projects_scan_root", path);
      else localStorage.removeItem("mycel.projects_scan_root");
    } catch {
      /* ignore */
    }
  };

  const chooseScanRoot = async () => {
    setPicking(true);
    setHint(null);
    try {
      const path = await api.pickDirectory();
      if (path) {
        persistScanRoot(path);
        setHint("Saved — Create Agent → Browse will scan here first.");
      }
    } catch {
      setHint("Folder dialog unavailable — type a path below, or use Browse in Create Agent.");
    } finally {
      setPicking(false);
    }
  };

  return (
    <>
      <p className="text-[11px] text-mycel-muted">
        MycelHome is the install root (agents, prefs, vault). Override with the{" "}
        <code className="text-mycel-text">MYCEL_HOME</code> env var and restart the daemon —
        relocating from the UI is not supported yet. Repos are chosen per agent, not as a
        required daemon workspace.
      </p>
      <Field label="MycelHome">
        <input className={MONO_INPUT_CLS} value={mycelHome} readOnly spellCheck={false} />
      </Field>
      <Field label="Projects scan root">
        <div className="flex items-center gap-2">
          <input
            className={MONO_INPUT_CLS}
            value={scanRoot}
            onChange={(e) => persistScanRoot(e.target.value)}
            placeholder="~/Projects"
            spellCheck={false}
          />
          <button
            type="button"
            onClick={() => { void chooseScanRoot(); }}
            disabled={picking}
            className="shrink-0 inline-flex items-center px-3 h-8 rounded-md border border-mycel-border bg-mycel-bg text-xs font-medium text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors disabled:opacity-50"
          >
            {picking ? "…" : "Choose…"}
          </button>
        </div>
      </Field>
      {hint && <p className="text-[11px] text-mycel-muted">{hint}</p>}
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
        {status === "saved" && <span aria-live="polite" className="text-xs text-mycel-success">Saved</span>}
        {status === "error" && <span role="alert" className="text-xs text-mycel-error">Save failed</span>}
        <button
          type="button"
          onClick={handleSave}
          disabled={status === "saving" || !dirty}
          className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover active:scale-[0.98] shadow-mycel-sm transition-all disabled:opacity-50"
        >
          {status === "saving" ? "Saving…" : "Save"}
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
  const themeLabelId = useId();
  const reveal = useProgressiveReveal(config, refresh);

  // An install that already runs agents has finished setup by definition, so
  // it counts as complete however the individual sections read.
  const setupComplete = reveal.allComplete || reveal.hasAgents;

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

  const isDirty = (...keys: string[]) => keys.some((k) => dirtySections.includes(k));
  const userName = String(((edited.user ?? {}) as Record<string, unknown>).name ?? "");

  return (
    <div className="p-4 md:p-6 space-y-4 max-w-3xl mx-auto">
      <header className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          <h1 className="font-display text-[26px] leading-none text-mycel-text">Settings</h1>
          <p className="mt-2 text-[13px] text-mycel-text-2">The config that has no other home — grouped for day-to-day use.</p>
        </div>
        <button
          type="button"
          onClick={() => { void reveal.replay(); }}
          title="Re-run setup — replays the guided reveal below without blanking anything you've already set"
          aria-label="Re-run setup"
          className="relative shrink-0 grid place-items-center w-9 h-9 rounded-full border border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-accent hover:border-mycel-accent transition-colors active:scale-95 outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent before:absolute before:-inset-1.5 before:content-['']"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
          </svg>
        </button>
      </header>

      {saveStatus === "saved" && dirtySections.length === 0 && (
        <div aria-live="polite" className="fixed bottom-4 right-4 z-30 rounded-lg border border-mycel-success bg-mycel-success-subtle px-3 py-2 text-xs text-mycel-success shadow-mycel-lg">
          Saved
        </div>
      )}

      {/* Floating save bar */}
      {dirtySections.length > 0 && (
        <div className="animate-fade-in sticky top-0 z-20 rounded-lg border border-mycel-accent bg-mycel-accent-subtle backdrop-blur px-3 py-2 flex items-center justify-between gap-3 shadow-mycel-lg">
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
              className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover active:scale-[0.98] transition-all disabled:opacity-50"
              title="Discard unsaved changes"
            >
              Discard
            </button>
            <button
              onClick={handleSaveAll}
              disabled={saveStatus === "saving"}
              className={`inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs font-medium transition-all active:scale-[0.98] disabled:opacity-60 ${
                saveStatus === "error"
                  ? "bg-mycel-error text-white hover:opacity-90 shadow-mycel-sm"
                  : "bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm"
              }`}
            >
              {saveStatus === "saving" && (
                <svg className="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden>
                  <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="3" opacity="0.25" />
                  <path d="M21 12a9 9 0 00-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
                </svg>
              )}
              {saveStatus === "saving" ? "Saving…" : saveStatus === "error" ? "Retry" : "Save"}
            </button>
          </div>
        </div>
      )}

      {restartWarning && (
        <div role="alert" className="rounded-md border border-mycel-error bg-mycel-error-subtle px-3 py-1.5 text-xs text-mycel-error">
          Changes saved. Restart mycel to apply (<code className="font-mono">mycel down &amp;&amp; mycel up -d</code>)
        </div>
      )}

      {/* Only while setup is unfinished. Once it's done this card said nothing
          except "it's done — use the re-run icon", which the icon in the header
          already offers: a permanent 100% bar at the top of Settings is noise
          on every visit. Gated on `loaded` so an established install never
          flashes it before the onboarding state arrives. */}
      {reveal.loaded && !setupComplete && (
        <Section title="setup" dirty={false} index={0}>
          <SetupSection
            completedCount={reveal.completedCount}
            total={REVEAL_ORDER.length}
          />
        </Section>
      )}

      <Section title="profile" dirty={isDirty("user", "ui")} index={1} reveal={reveal.states.profile}>
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
          <span id={themeLabelId} className="text-xs text-mycel-text-2 w-28 shrink-0">Theme</span>
          <div role="group" aria-labelledby={themeLabelId}>
            <ThemePicker value={currentThemeChoice} onChange={handleThemeChange} />
          </div>
        </div>
        <Field label="Default view">
          <select
            className={INPUT_CLS}
            value={String(((edited.ui ?? {}) as Record<string, unknown>).default_view ?? "home")}
            onChange={(e) => handleChange(["ui", "default_view"], e.target.value)}
          >
            <option value="home">Home</option>
            <option value="agents">Agents</option>
            <option value="insights">Insights</option>
          </select>
        </Field>
      </Section>

      {/* Providers/Tools folded into Settings as a list-only section — no
          more standalone /tools page. Per-provider drill-down still lives at
          /settings/providers/:name. Self-completes once a provider is
          installed — no CTA needed. */}
      <Section title="providers & tools" dirty={false} index={2} reveal={reveal.states["providers & tools"]}>
        <ProvidersToolsSection />
      </Section>

      <Section
        title="runtime"
        dirty={isDirty("runtime")}
        index={3}
        reveal={reveal.states.runtime}
        onContinue={reveal.states.runtime === "active" ? () => { void reveal.markTouched("runtime"); } : undefined}
      >
        <RuntimeSection data={edited} onChange={handleChange} />
      </Section>

      {/* Apps owns integrations, notification delivery, and API keys on the
          /apps page; Settings only summarizes and links out. The
          prefs.notifications{} controls surface there (follow-up PR).
          Optional/skippable — the Continue CTA acknowledges without
          requiring a connected app. */}
      <Section
        title="apps"
        dirty={false}
        index={4}
        reveal={reveal.states.apps}
        onContinue={reveal.states.apps === "active" ? () => { void reveal.markTouched("apps"); } : undefined}
      >
        <AppsCard />
      </Section>

      {/* Cost caps — entirely optional, so it self-completes (see
          useProgressiveReveal) and never shows a Continue CTA. */}
      <Section title="budgets" dirty={false} index={5} reveal={reveal.states.budgets}>
        <BudgetsSection />
      </Section>

      <Section title="advanced" dirty={isDirty("storage", "server", "logs")} defaultOpen={false} index={6}>
        <Advanced label="MycelHome">
          <MycelHomeFields />
        </Advanced>
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
