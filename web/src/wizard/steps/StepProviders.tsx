import { useEffect, useState } from "react";
import { api, type ModelInfo, type ProviderInfo } from "../../api/client";
import { useReadiness } from "../../hooks/useReadiness";
import { PROVIDER_LABELS, PROVIDER_NAMES } from "../../views/readiness/readiness";
import { InstallButton } from "../InstallButton";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 3 — Providers & sign-in. Pick an agent tool, install it (reusing the
 * streamed installer), optionally store its API key in the vault, and set it
 * as the default provider. Cursor's install is a download page, so it shows
 * guidance instead of a button. */

const CURSOR = "cursor";

// Vault key each provider reads for headless auth. Blank = no standard key
// (the CLI's own login is the path); those still store under <NAME>_API_KEY.
const SECRET_KEY: Record<string, string> = {
  claude: "ANTHROPIC_API_KEY",
  codex: "OPENAI_API_KEY",
};

function secretNameFor(provider: string): string {
  return SECRET_KEY[provider] ?? `${provider.toUpperCase()}_API_KEY`;
}

export function StepProviders({ nav, draft, setDraft, settings, reloadSettings }: StepProps) {
  const { data, refresh } = useReadiness();
  const [selected, setSelected] = useState(draft.provider || settings?.providers?.default || "claude");
  const [models, setModels] = useState<Record<string, ModelInfo[]>>({});
  const [model, setModel] = useState(draft.model);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [keySaved, setKeySaved] = useState(false);

  useEffect(() => {
    api
      .listProviders()
      .then((list: ProviderInfo[]) => {
        const map: Record<string, ModelInfo[]> = {};
        for (const p of list) if (p.models) map[p.name] = p.models;
        setModels(map);
      })
      .catch(() => {
        /* model pickers stay empty; provider default still works */
      });
  }, []);

  const installed = data?.providers ?? {};
  const selModels = models[selected] ?? [];
  const selInstalled = installed[selected];

  const persist = async () => {
    setSaving(true);
    setDraft({ provider: selected, model });
    try {
      if (apiKey.trim()) {
        const name = secretNameFor(selected);
        try {
          await api.createSecret(name, apiKey.trim(), `${PROVIDER_LABELS[selected] ?? selected} API key`);
        } catch {
          // Already exists → update instead.
          await api.updateSecret(name, apiKey.trim());
        }
        setKeySaved(true);
      }
      await api.updateSettings({
        providers: {
          default: selected,
          providers: settings?.providers?.providers ?? {},
        },
      });
      await reloadSettings();
    } catch {
      /* skippable/resumable */
    } finally {
      setSaving(false);
      nav.next();
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        An agent tool is the CLI that actually drives a model (Claude Code, Codex, …). Pick your
        default — you can add more later and switch per agent.
      </p>

      <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
        {PROVIDER_NAMES.map((name) => {
          const active = selected === name;
          const isInstalled = installed[name];
          return (
            <button
              key={name}
              type="button"
              onClick={() => {
                setSelected(name);
                setModel("");
              }}
              aria-pressed={active}
              className={`flex items-center justify-between gap-2 px-3 py-2.5 rounded-lg border text-left transition-colors ${
                active ? "border-mycel-accent bg-mycel-accent-subtle" : "border-mycel-border bg-mycel-surface hover:border-mycel-accent"
              }`}
            >
              <span className="text-[13px] font-medium text-mycel-text truncate">
                {PROVIDER_LABELS[name] ?? name}
              </span>
              <span
                className={`shrink-0 w-1.5 h-1.5 rounded-full ${isInstalled ? "bg-mycel-success" : "bg-mycel-muted"}`}
                title={isInstalled ? "Installed" : "Not installed"}
                aria-hidden
              />
            </button>
          );
        })}
      </div>

      <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 flex flex-col gap-4">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[13px] font-semibold text-mycel-text">{PROVIDER_LABELS[selected] ?? selected}</span>
          <span className={`text-[11px] ${selInstalled ? "text-mycel-success" : "text-mycel-warning"}`}>
            {selInstalled ? "Installed" : "Not installed"}
          </span>
        </div>

        {!selInstalled &&
          (selected === CURSOR ? (
            <a
              href="https://cursor.sh"
              target="_blank"
              rel="noreferrer"
              className="self-start text-[12px] px-2.5 py-1 rounded-md border border-mycel-accent text-mycel-accent hover:bg-mycel-accent-subtle transition-colors"
            >
              Download Cursor ↗
            </a>
          ) : (
            <InstallButton id={selected} label={PROVIDER_LABELS[selected] ?? selected} onDone={() => void refresh()} />
          ))}

        {selModels.length > 0 && (
          <label className="flex flex-col gap-1">
            <span className="text-[12px] text-mycel-text-2">Default model</span>
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              className="w-full max-w-xs bg-mycel-bg border border-mycel-border rounded-md px-2 py-1.5 text-[13px] text-mycel-text outline-none focus:border-mycel-accent"
            >
              <option value="">Provider default</option>
              {selModels.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.id}
                  {m.available ? "" : " (unverified)"}
                </option>
              ))}
            </select>
          </label>
        )}

        <label className="flex flex-col gap-1">
          <span className="text-[12px] text-mycel-text-2">
            API key <span className="text-mycel-muted">(optional — stored encrypted in the vault)</span>
          </span>
          <input
            type="password"
            value={apiKey}
            onChange={(e) => {
              setApiKey(e.target.value);
              setKeySaved(false);
            }}
            placeholder={secretNameFor(selected)}
            className="w-full max-w-sm bg-mycel-bg border border-mycel-border rounded-md px-2.5 py-1.5 text-[13px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
          />
          <span className="text-[11px] text-mycel-muted">
            {keySaved
              ? "Saved to the vault."
              : `Or run \`${selected}\` once in a terminal to sign in with the CLI.`}
          </span>
        </label>
      </div>

      <WizardFooter nav={nav} primaryLabel={saving ? "Saving…" : "Set as default"} onPrimary={persist} primaryDisabled={saving} />
    </div>
  );
}
