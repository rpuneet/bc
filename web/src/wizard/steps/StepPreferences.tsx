import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { PROVIDER_LABELS } from "../../views/readiness/readiness";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 4 — Preferences. Confirm the defaults chosen so far, set an optional
 * monthly budget, and (advanced) tune injected instructions and storage. */

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-4 py-2.5">
      <span className="text-[12px] text-mycel-muted">{label}</span>
      <span className="text-[13px] text-mycel-text font-medium truncate">{value}</span>
    </div>
  );
}

export function StepPreferences({ nav, draft, settings, reloadSettings }: StepProps) {
  const [advanced, setAdvanced] = useState(false);
  const [budget, setBudget] = useState("");
  const [injected, setInjected] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .getInjectedInstructions()
      .then((r) => setInjected(r.injected_instructions))
      .catch(() => {
        /* leave blank */
      });
  }, []);

  const provider = draft.provider || settings?.providers?.default || "claude";
  const runtime = draft.runtime || settings?.runtime?.default || "docker";
  const model = draft.model || "Provider default";
  const storage = settings?.storage?.default || "sqlite";

  const persist = async () => {
    setSaving(true);
    try {
      await api.updateInjectedInstructions(injected);
      const limit = Number(budget);
      if (budget.trim() && limit > 0) {
        await api
          .createCostBudget({ scope: "workspace", period: "monthly", limit_usd: limit, alert_at: 0.8 })
          .catch(() => {
            /* budget is optional — ignore conflicts */
          });
      }
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
        Here's what you've set up. Adjust anything now or later in Settings.
      </p>

      <div className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden divide-y divide-mycel-border shadow-mycel">
        <SummaryRow label="Default agent tool" value={PROVIDER_LABELS[provider] ?? provider} />
        <SummaryRow label="Default model" value={model} />
        <SummaryRow label="Runtime" value={runtime === "docker" ? "Docker (containers)" : "tmux (local)"} />
        <SummaryRow label="Storage" value={storage === "sqlite" ? "SQLite (default)" : storage} />
      </div>

      <label className="flex flex-col gap-1.5 max-w-sm">
        <span className="text-[13px] font-medium text-mycel-text">Monthly budget (optional)</span>
        <div className="flex items-center gap-2">
          <span className="text-[13px] text-mycel-muted">$</span>
          <input
            type="number"
            min={0}
            step={5}
            value={budget}
            onChange={(e) => setBudget(e.target.value)}
            placeholder="No limit"
            className="w-32 bg-mycel-bg border border-mycel-border rounded-md px-2.5 py-1.5 text-[13px] text-mycel-text font-mono outline-none focus:border-mycel-accent"
          />
          <span className="text-[11px] text-mycel-muted">alerts at 80%</span>
        </div>
      </label>

      <button
        type="button"
        onClick={() => setAdvanced((v) => !v)}
        className="self-start text-[12px] text-mycel-muted hover:text-mycel-text transition-colors"
      >
        {advanced ? "▾ Hide advanced" : "▸ Advanced settings"}
      </button>

      {advanced && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-[12px] text-mycel-text-2">Injected instructions</span>
            <textarea
              value={injected}
              onChange={(e) => setInjected(e.target.value)}
              rows={4}
              placeholder="Guidance appended to every agent's prompt (optional)."
              className="w-full bg-mycel-bg border border-mycel-border rounded-md px-2.5 py-2 text-[12px] text-mycel-text font-mono outline-none focus:border-mycel-accent resize-y"
            />
            <span className="text-[11px] text-mycel-muted">Added to every agent's system prompt at spawn. Never include secrets.</span>
          </label>
          <div className="text-[12px] text-mycel-text-2">
            Storage: <span className="font-mono text-mycel-text">SQLite</span> at{" "}
            <span className="font-mono text-mycel-muted">~/.mycel/mycel.db</span>. Switch to TimescaleDB
            later in Settings for time-series metrics.
          </div>
        </div>
      )}

      <WizardFooter nav={nav} primaryLabel={saving ? "Saving…" : "Continue"} onPrimary={persist} primaryDisabled={saving} />
    </div>
  );
}
