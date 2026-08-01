import { useCallback, useEffect, useMemo, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { api } from "../api/client";
import type { ProviderInfo } from "../api/client";
import { PROVIDER_LABELS } from "../views/readiness/readiness";

/* ── ProviderDefaults ────────────────────────────────────────────────
 *
 * The fleet-wide defaults every new agent inherits: which provider it runs
 * on, and which model that provider should use when none is given. Both
 * persist to prefs.providers via PATCH /api/settings and reload live so the
 * panel always reflects what is actually on disk.
 *
 * Model options come from the provider's own curated list (/api/providers →
 * ProviderInfo.models). A model marked `available` is confirmed live from the
 * provider CLI (auth verified); an unavailable one is a static fallback the
 * user can still pick but which we label honestly as unverified.
 *
 * The host line (hostname · os/arch) is folded in here so the Tools &
 * Providers manager answers "what am I running on?" without a separate page.
 */

type SaveState = "idle" | "saving" | "saved" | "error";

/** Strip noisy mDNS suffixes ("Foo.local" → "Foo") for the host readout. */
function prettifyHost(h: string): string {
  return h.replace(/\.(local|lan)$/i, "");
}

const SELECT_CLS =
  "w-full px-2.5 py-1.5 text-[13px] rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:border-mycel-accent focus:ring-1 focus:ring-mycel-accent transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed";

function SavePill({ state, error }: { state: SaveState; error: string | null }) {
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
      <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-error" role="alert">
        <svg className="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M12 8v4M12 16h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" />
        </svg>
        {error ?? "Save failed"}
      </span>
    );
  }
  return null;
}

export function ProviderDefaults({ providers }: { providers: ProviderInfo[] }) {
  const reduceMotion = useReducedMotion();
  const [defaultProvider, setDefaultProvider] = useState<string>("");
  const [defaultModel, setDefaultModel] = useState<string>("");
  const [loaded, setLoaded] = useState(false);
  const [save, setSave] = useState<SaveState>("idle");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [host, setHost] = useState<{ hostname: string; os: string; arch: string } | null>(null);

  // Last-known-good, so a failed PATCH reverts the pickers rather than
  // leaving them showing a choice that never persisted.
  const [committed, setCommitted] = useState<{ provider: string; model: string }>({ provider: "", model: "" });

  useEffect(() => {
    let alive = true;
    api
      .getSettings()
      .then((cfg) => {
        if (!alive) return;
        const p = cfg.providers?.default ?? "";
        const m = cfg.providers?.default_model ?? "";
        setDefaultProvider(p);
        setDefaultModel(m);
        setCommitted({ provider: p, model: m });
        setLoaded(true);
      })
      .catch(() => {
        if (alive) setLoaded(true); // degrade to empty selects rather than a stuck skeleton
      });
    api
      .getSystemInfo()
      .then((info) => { if (alive) setHost(info); })
      .catch(() => { /* host line is optional */ });
    return () => { alive = false; };
  }, []);

  const byName = useMemo(() => {
    const m = new Map<string, ProviderInfo>();
    for (const p of providers) m.set(p.name, p);
    return m;
  }, [providers]);

  const models = useMemo(() => byName.get(defaultProvider)?.models ?? [], [byName, defaultProvider]);

  const persist = useCallback(
    async (next: { provider: string; model: string }) => {
      setSave("saving");
      setSaveError(null);
      try {
        await api.setProviderDefaults({ default: next.provider, default_model: next.model });
        setCommitted(next);
        setSave("saved");
        setTimeout(() => setSave("idle"), 1800);
      } catch (err) {
        // Revert to the last value that actually reached disk.
        setDefaultProvider(committed.provider);
        setDefaultModel(committed.model);
        setSaveError(err instanceof Error ? err.message : "Save failed");
        setSave("error");
      }
    },
    [committed],
  );

  const onProviderChange = (name: string) => {
    // A model only makes sense for its own provider — if the previously
    // selected model isn't offered by the new provider, drop it so we never
    // persist a mismatched (provider, model) pair.
    const nextModels = byName.get(name)?.models ?? [];
    const keepModel = nextModels.some((m) => m.id === defaultModel) ? defaultModel : "";
    setDefaultProvider(name);
    setDefaultModel(keepModel);
    void persist({ provider: name, model: keepModel });
  };

  const onModelChange = (id: string) => {
    setDefaultModel(id);
    void persist({ provider: defaultProvider, model: id });
  };

  return (
    <motion.div
      initial={reduceMotion ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, ease: [0.16, 1, 0.3, 1] }}
      className="rounded-lg border border-mycel-border bg-mycel-surface p-4 mb-4"
    >
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="min-w-0">
          <h3 className="text-[13px] font-semibold text-mycel-text">Fleet defaults</h3>
          <p className="text-[11px] text-mycel-muted mt-0.5">Provider and model new agents inherit when none is set.</p>
        </div>
        <div className="shrink-0 min-h-[16px]">
          <SavePill state={save} error={saveError} />
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">Default provider</span>
          <select
            className={SELECT_CLS}
            value={defaultProvider}
            disabled={!loaded || providers.length === 0}
            onChange={(e) => onProviderChange(e.target.value)}
            aria-label="Default provider"
          >
            {providers.length === 0 && <option value="">No providers</option>}
            {providers.map((p) => (
              <option key={p.name} value={p.name}>
                {PROVIDER_LABELS[p.name] ?? p.name}
                {p.installed ? "" : " (not installed)"}
              </option>
            ))}
          </select>
        </label>

        <label className="block">
          <span className="block text-[11px] text-mycel-text-2 mb-1">Default model</span>
          <select
            className={SELECT_CLS}
            value={defaultModel}
            disabled={!loaded || models.length === 0}
            onChange={(e) => onModelChange(e.target.value)}
            aria-label="Default model"
          >
            <option value="">Provider default</option>
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.id}
                {m.available ? "" : " · unverified"}
              </option>
            ))}
          </select>
          <span className="block text-[10.5px] text-mycel-muted mt-1">
            {models.length === 0
              ? "This provider exposes no model list — its own default is used."
              : "“Unverified” models are a static fallback (provider sign-in not confirmed)."}
          </span>
        </label>
      </div>

      {/* Host machine — folded in so the manager answers "what am I on?". */}
      <div className="flex items-center gap-2 mt-3 pt-3 border-t border-mycel-border text-[11px] text-mycel-muted">
        <svg className="w-3.5 h-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.6} aria-hidden>
          <rect x="3" y="4" width="18" height="12" rx="1.5" /><path strokeLinecap="round" d="M8 20h8M12 16v4" />
        </svg>
        {host?.hostname ? (
          <span className="tabular-nums">
            Running on <span className="text-mycel-text-2 font-medium">{prettifyHost(host.hostname)}</span> · {host.os}/{host.arch}
          </span>
        ) : (
          <span>Resolving host machine…</span>
        )}
      </div>
    </motion.div>
  );
}
