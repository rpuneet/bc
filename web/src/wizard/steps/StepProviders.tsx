import { useEffect, useState } from "react";
import { api, type ProviderInfo } from "../../api/client";
import { useReadiness } from "../../hooks/useReadiness";
import { PROVIDER_LABELS, PROVIDER_NAMES } from "../../views/readiness/readiness";
import { ProviderDefaults } from "../../components/ProviderDefaults";
import { AdvancedToggle } from "../../settings/controls";
import { InstallButton } from "../InstallButton";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 3 — Providers & sign-in. The fleet default (provider + model) is set
 * through the SAME <ProviderDefaults> control the /tools page ships, so the
 * wizard and the ongoing manager never drift. Everything else — installing
 * other tools, a per-provider command override, and stashing an API key in
 * the vault — lives behind an Advanced expander.
 *
 * Provider OAuth sign-in is intentionally deferred (owner decision pending),
 * so the honest paths are the CLI's own login or an API key in the vault; the
 * model list still labels unverified models as such. */

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
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [advanced, setAdvanced] = useState(false);
  // The provider the Advanced install / config controls target. Seeded from
  // the current default; changing the fleet default re-points it.
  const [focus, setFocus] = useState(draft.provider || settings?.providers?.default || "claude");
  const [command, setCommand] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [keySaved, setKeySaved] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .listProviders()
      .then(setProviders)
      .catch(() => setProviders([]));
  }, []);

  // Seed the command override from prefs whenever the focused provider changes.
  useEffect(() => {
    setCommand(settings?.providers?.providers?.[focus]?.command ?? "");
    setApiKey("");
    setKeySaved(false);
  }, [focus, settings]);

  const installed = data?.providers ?? {};
  const focusInstalled = installed[focus];

  // Persist the focused provider's API key (vault) and command override
  // (prefs) — merged so we never clobber other providers' overrides.
  const persistFocused = async () => {
    if (apiKey.trim()) {
      const name = secretNameFor(focus);
      try {
        await api.createSecret(name, apiKey.trim(), `${PROVIDER_LABELS[focus] ?? focus} API key`);
      } catch {
        await api.updateSecret(name, apiKey.trim());
      }
      setKeySaved(true);
    }
    const existing = settings?.providers?.providers ?? {};
    const current = existing[focus]?.command ?? "";
    if (command.trim() !== current) {
      await api.updateSettings({
        providers: {
          providers: { ...existing, [focus]: { command: command.trim() } },
        },
      });
    }
  };

  const onContinue = async () => {
    setSaving(true);
    try {
      await persistFocused();
      await reloadSettings();
    } catch {
      /* skippable/resumable — the fleet default already persisted live */
    } finally {
      setSaving(false);
      nav.next();
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        An agent tool is the CLI that actually drives a model (Claude Code, Codex, …). Set your
        fleet default below — every new agent inherits it. You can add more and switch per agent.
      </p>

      {/* Fleet default provider + model — the shared control, persisted live
          to prefs.providers via PATCH /api/settings. The host readout is
          hidden here (the System step already covers the machine). */}
      <ProviderDefaults
        providers={providers}
        showHost={false}
        onChange={({ provider, model }) => {
          setDraft({ provider, model });
          setFocus(provider);
        }}
      />

      <div className="flex flex-col gap-3">
        <AdvancedToggle open={advanced} onToggle={() => setAdvanced((v) => !v)} label="More tools & keys" />
        {advanced && (
          <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 flex flex-col gap-4">
            <p className="text-[12px] text-mycel-muted">
              Install another tool, override its command, or stash an API key. Pick the tool to
              configure — this doesn&apos;t change your default.
            </p>

            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
              {PROVIDER_NAMES.map((name) => {
                const active = focus === name;
                const isInstalled = installed[name];
                return (
                  <button
                    key={name}
                    type="button"
                    onClick={() => setFocus(name)}
                    aria-pressed={active}
                    className={`flex items-center justify-between gap-2 px-3 py-2.5 rounded-lg border text-left transition-colors ${
                      active
                        ? "border-mycel-accent bg-mycel-accent-subtle"
                        : "border-mycel-border bg-mycel-bg hover:border-mycel-accent"
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

            <div className="flex items-center justify-between gap-2">
              <span className="text-[13px] font-semibold text-mycel-text">{PROVIDER_LABELS[focus] ?? focus}</span>
              <span className={`inline-flex items-center gap-1.5 text-[11px] ${focusInstalled ? "text-mycel-success" : "text-mycel-warning"}`}>
                <span className={`w-1.5 h-1.5 rounded-full ${focusInstalled ? "bg-mycel-success" : "bg-mycel-warning"}`} aria-hidden />
                {focusInstalled ? "Installed" : "Not installed"}
              </span>
            </div>

            {!focusInstalled &&
              (focus === CURSOR ? (
                <a
                  href="https://cursor.sh"
                  target="_blank"
                  rel="noreferrer"
                  className="self-start text-[12px] px-2.5 py-1 rounded-md border border-mycel-accent text-mycel-accent hover:bg-mycel-accent-subtle transition-colors"
                >
                  Download Cursor ↗
                </a>
              ) : (
                <InstallButton id={focus} label={PROVIDER_LABELS[focus] ?? focus} onDone={() => void refresh()} />
              ))}

            <label className="flex flex-col gap-1">
              <span className="text-[12px] text-mycel-text-2">
                Command override <span className="text-mycel-muted">(optional — defaults to the tool&apos;s own binary)</span>
              </span>
              <input
                type="text"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder={focus}
                className="w-full max-w-sm bg-mycel-bg border border-mycel-border rounded-md px-2.5 py-1.5 text-[13px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
              />
            </label>

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
                placeholder={secretNameFor(focus)}
                className="w-full max-w-sm bg-mycel-bg border border-mycel-border rounded-md px-2.5 py-1.5 text-[13px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
              />
              <span className="text-[11px] text-mycel-muted">
                {keySaved
                  ? "Saved to the vault."
                  : `Or run \`${focus}\` once in a terminal to sign in with the CLI (auth unverified until then).`}
              </span>
            </label>
          </div>
        )}
      </div>

      <WizardFooter nav={nav} primaryLabel={saving ? "Saving…" : "Continue"} onPrimary={onContinue} primaryDisabled={saving} />
    </div>
  );
}
