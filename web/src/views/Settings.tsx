import { useCallback, useState, useEffect, useMemo } from "react";
import { api } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";

import { DependenciesSection } from "../components/settings/Dependencies";
type SaveStatus = "idle" | "saving" | "saved" | "error";

const SECTION_ORDER = ["server", "storage", "runtime", "providers", "logs"];
const RESTART_SECTIONS = new Set(["server", "storage", "runtime"]);

function deepClone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v));
}

function deepEqual(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/* ------------------------------------------------------------------ */
/*  Shared components                                                   */
/* ------------------------------------------------------------------ */

const INPUT_CLS = "w-full px-2 py-0.5 text-xs rounded-md border border-mycel-border bg-mycel-bg text-mycel-text font-mono focus:outline-none focus:ring-1 focus:ring-mycel-accent";

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

/* ------------------------------------------------------------------ */
/*  Section wrapper                                                     */
/* ------------------------------------------------------------------ */

const SECTION_META: Record<string, { icon: React.ReactNode; desc: string }> = {
  server: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />,
    desc: "Host, port, and CORS settings",
  },
  storage: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />,
    desc: "Database backend configuration",
  },
  runtime: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />,
    desc: "Agent execution environment",
  },
  providers: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />,
    desc: "AI provider commands",
  },
  logs: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />,
    desc: "Log file location and rotation",
  },
  dependencies: {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />,
    desc: "Optional supporting services",
  },
  "injected instructions": {
    icon: <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.129.166 2.27.293 3.423.379.35.026.67.21.865.501L12 21l2.755-4.133a1.14 1.14 0 01.865-.501 48.172 48.172 0 003.423-.379c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z" />,
    desc: "Sent to every agent at spawn",
  },
};

function Section({
  title,
  dirty,
  children,
}: {
  title: string;
  dirty: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(true);
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
      {open && <div className="px-3 pb-3 pt-1.5 space-y-1.5 border-t border-mycel-border">{children}</div>}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Per-section renderers                                               */
/* ------------------------------------------------------------------ */

function ServerSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const s = (data.server ?? {}) as Record<string, unknown>;
  const configuredPort = Number(s.port ?? 0);
  // The Settings page renders the *configured* port from settings.json,
  // but the daemon may be running on an override (MYCEL_DAEMON_ADDR, --addr flag,
  // or the legacy 9374 default if neither is set). Surface the actual
  // listen port from the browser's connection so users see the live
  // value alongside the editable config value.
  const actualPort = typeof window !== "undefined" && window.location.port
    ? Number(window.location.port)
    : 0;
  const portDrift = actualPort > 0 && actualPort !== configuredPort;
  return (
    <>
      <Field label="Host"><input className={INPUT_CLS} value={String(s.host ?? "")} onChange={(e) => onChange(["server", "host"], e.target.value)} /></Field>
      <Field
        label="Port"
        suffix={portDrift ? `running on ${actualPort}` : actualPort > 0 ? "live" : undefined}
      >
        <input className={INPUT_CLS} type="number" value={configuredPort} onChange={(e) => onChange(["server", "port"], Number(e.target.value))} />
      </Field>
      <Field label="CORS Origin"><input className={INPUT_CLS} value={String(s.cors_origin ?? "")} onChange={(e) => onChange(["server", "cors_origin"], e.target.value)} /></Field>
    </>
  );
}

function StorageSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const s = (data.storage ?? {}) as Record<string, unknown>;
  const backend = String(s.default ?? "sqlite");
  const ts = (s.timescale ?? {}) as Record<string, unknown>;
  const sq = (s.sqlite ?? {}) as Record<string, unknown>;

  return (
    <>
      <Field label="Backend">
        <select value={backend} onChange={(e) => onChange(["storage", "default"], e.target.value)} className={INPUT_CLS}>
          <option value="timescale">TimescaleDB</option>
          <option value="sqlite">SQLite</option>
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

function RuntimeSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const r = (data.runtime ?? {}) as Record<string, unknown>;
  const mode = String(r.default ?? "docker");
  const docker = (r.docker ?? {}) as Record<string, unknown>;
  const tmux = (r.tmux ?? {}) as Record<string, unknown>;

  return (
    <>
      <Field label="Runtime">
        <select value={mode} onChange={(e) => onChange(["runtime", "default"], e.target.value)} className={INPUT_CLS}>
          <option value="docker">Docker</option>
          <option value="local">Local (tmux)</option>
          <option disabled>Kubernetes (coming soon)</option>
        </select>
      </Field>
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
    </>
  );
}

function ProvidersSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
  const p = (data.providers ?? {}) as Record<string, unknown>;
  const defaultProvider = String(p.default ?? "claude");
  const providers = (p.providers ?? {}) as Record<string, Record<string, unknown>>;
  const providerKeys = Object.keys(providers);

  return (
    <div className="space-y-1.5">
      <Field label="Default">
        <select value={defaultProvider} onChange={(e) => onChange(["providers", "default"], e.target.value)} className={INPUT_CLS}>
          {providerKeys.map((k) => <option key={k} value={k}>{k}</option>)}
        </select>
      </Field>
      {providerKeys.map((k) => (
        <Field key={k} label={k}>
          <input
            className={INPUT_CLS}
            value={String(providers[k]?.command ?? "")}
            title={String(providers[k]?.command ?? "") || "command"}
            onChange={(e) => onChange(["providers", "providers", k, "command"], e.target.value)}
            placeholder="command"
          />
        </Field>
      ))}
    </div>
  );
}

function LogsSection({ data, onChange }: { data: Record<string, unknown>; onChange: (path: string[], v: unknown) => void }) {
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

/* ------------------------------------------------------------------ */
/*  Injected instructions — mycel-authored text pushed to every agent   */
/* ------------------------------------------------------------------ */

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
        Appended to every agent&apos;s prompt file at spawn, followed by an
        auto-generated summary of its available MCP servers and credential
        env var names. Secret values are never included.
      </p>
      <textarea
        className={`${INPUT_CLS} h-40 resize-y leading-relaxed`}
        placeholder="e.g. Always report status before and after work. Prefer small PRs."
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      <div className="flex items-center justify-end gap-2">
        {status === "saved" && (
          <span className="text-xs text-mycel-success">Saved</span>
        )}
        {status === "error" && (
          <span className="text-xs text-mycel-error">Save failed</span>
        )}
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

  return (
    <div className="p-4 md:p-6 space-y-3">
      {/* File metadata badge — the source of truth for what these
          settings map to on disk. Small chip with a bordered file
          glyph so it reads as chrome, not as a subtitle competing
          with the page header above. */}
      <div className="flex items-center gap-2">
        <span className="inline-flex items-center gap-1.5 text-[10px] font-mono px-2 py-1 rounded-md border border-mycel-border bg-mycel-surface-hover text-mycel-muted">
          <svg width="10" height="10" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3 1.5h5l3 3v8H3z" />
            <path d="M8 1.5v3h3" />
          </svg>
          preferences.json{typeof version !== "undefined" ? ` · v${version}` : ""}
        </span>
      </div>

      {/* Saved confirmation toast — visible briefly after a successful save
          when no other sections are dirty. */}
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
            <span className="text-mycel-muted">{dirtySections.join(", ")}</span>
            {dirtySections.some((k) => RESTART_SECTIONS.has(k)) && (
              <span className="ml-2 text-mycel-accent">Restart required after save</span>
            )}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              type="button"
              onClick={() => {
                if (original) setEdited(deepClone(original));
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

      {saveStatus === "saved" && dirtySections.length === 0 && !restartWarning && (
        <div className="rounded-md border border-mycel-success bg-mycel-success-subtle px-3 py-1.5 text-xs text-mycel-success">
          Changes saved.
        </div>
      )}

      {restartWarning && (
        <div className="rounded-md border border-mycel-error bg-mycel-error-subtle px-3 py-1.5 text-xs text-mycel-error">
          Changes saved. Restart mycel to apply (<code className="font-mono">mycel down &amp;&amp; mycel up -d</code>)
        </div>
      )}

      {/* Row 1: Server + Storage side by side */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <Section title="server" dirty={dirtySections.includes("server")}>
          <ServerSection data={edited} onChange={handleChange} />
        </Section>
        <Section title="storage" dirty={dirtySections.includes("storage")}>
          <StorageSection data={edited} onChange={handleChange} />
        </Section>
      </div>

      {/* Row 2: Runtime + Providers side by side */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <Section title="runtime" dirty={dirtySections.includes("runtime")}>
          <RuntimeSection data={edited} onChange={handleChange} />
        </Section>
        <Section title="providers" dirty={dirtySections.includes("providers")}>
          <ProvidersSection data={edited} onChange={handleChange} />
        </Section>
      </div>

      {/* Row 3: Logs */}
      <div className="grid grid-cols-1 gap-3">
        <Section title="logs" dirty={dirtySections.includes("logs")}>
          <LogsSection data={edited} onChange={handleChange} />
        </Section>
      </div>

      {/* Row 4: Injected instructions (sent to every agent) */}
      <Section title="injected instructions" dirty={false}>
        <InjectedInstructionsSection />
      </Section>

      {/* Row 5: Optional dependencies (mycel-db, mycel-code-server, mycel-browser) */}
      <Section title="dependencies" dirty={false}>
        <DependenciesSection />
      </Section>
    </div>
  );
}
