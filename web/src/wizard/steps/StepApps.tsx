import { useEffect, useState } from "react";
import { api, type AppInstance } from "../../api/client";
import { AppChooser, ConnectWizard } from "../../components/apps/ConnectApp";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 5 — Connect an app. Reuses the real Apps connect flow (AppChooser →
 * ConnectWizard) so Slack/Telegram/Discord/WhatsApp/GitHub/Gmail all work
 * exactly as they do on the Apps page. Fully skippable. */

export function StepApps({ nav }: StepProps) {
  const [chooserOpen, setChooserOpen] = useState(false);
  const [connectAppId, setConnectAppId] = useState<string | null>(null);
  const [instances, setInstances] = useState<AppInstance[]>([]);

  const load = () =>
    api
      .getApps()
      .then((res) => setInstances(res.instances ?? []))
      .catch(() => setInstances([]));

  useEffect(() => {
    void load();
  }, []);

  const connected = instances.filter((i) => i.connected);

  return (
    <div className="flex flex-col gap-5">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        Connect a chat app and your agents can talk to you where you already are — Slack, Telegram,
        Discord, WhatsApp, GitHub, or Gmail. Optional; connect one now or later from Apps.
      </p>

      {connected.length > 0 && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface divide-y divide-mycel-border overflow-hidden">
          {connected.map((i) => (
            <div key={i.name} className="flex items-center justify-between gap-3 px-4 py-2.5">
              <span className="text-[13px] text-mycel-text font-medium">{i.name}</span>
              <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-success">
                <span className="w-1.5 h-1.5 rounded-full bg-mycel-success" aria-hidden />
                Connected
              </span>
            </div>
          ))}
        </div>
      )}

      <button
        type="button"
        onClick={() => setChooserOpen(true)}
        className="self-start text-[13px] font-medium px-4 py-2 rounded-md border border-mycel-accent text-mycel-accent bg-mycel-accent-subtle hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors"
      >
        {connected.length > 0 ? "Connect another app" : "Connect an app"}
      </button>

      {chooserOpen && (
        <AppChooser
          onSelect={(key) => {
            setChooserOpen(false);
            setConnectAppId(key);
          }}
          onClose={() => setChooserOpen(false)}
        />
      )}
      {connectAppId && (
        <ConnectWizard
          appId={connectAppId}
          onClose={() => setConnectAppId(null)}
          onConnected={() => {
            void load();
          }}
        />
      )}

      <WizardFooter nav={nav} primaryLabel="Continue" skipLabel="Skip" />
    </div>
  );
}
