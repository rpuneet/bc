import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { useReadiness } from "../../hooks/useReadiness";
import { AdvancedToggle, RuntimePicker, type RuntimeChoice as RT } from "../../settings/controls";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 2 — Runtime. Docker (isolated containers) or tmux (local sessions),
 * with an advanced expander. The runtime picker + advanced toggle are shared
 * with Settings → Runtime (web/src/settings/controls). Writes prefs.runtime. */

const NUM = "w-24 bg-mycel-bg border border-mycel-border rounded-md px-2 py-1 text-[12px] text-mycel-text font-mono outline-none focus:border-mycel-accent";
const TXT = "w-full bg-mycel-bg border border-mycel-border rounded-md px-2 py-1 text-[12px] text-mycel-text font-mono outline-none focus:border-mycel-accent";

export function StepRuntime({ nav, draft, setDraft, settings, reloadSettings }: StepProps) {
  const { data } = useReadiness();
  const [choice, setChoice] = useState<RT>(draft.runtime);
  const [advanced, setAdvanced] = useState(false);
  const [saving, setSaving] = useState(false);

  // Docker knobs
  const [image, setImage] = useState("mycel-agent-claude:latest");
  const [cpus, setCpus] = useState(2);
  const [memoryMB, setMemoryMB] = useState(4096);
  // tmux knobs
  const [shell, setShell] = useState("/bin/bash");
  const [history, setHistory] = useState(10000);

  useEffect(() => {
    const r = settings?.runtime;
    if (!r) return;
    if (r.default === "docker" || r.default === "tmux") setChoice(r.default);
    if (r.docker) {
      setImage(r.docker.image || "mycel-agent-claude:latest");
      setCpus(r.docker.cpus || 2);
      setMemoryMB(r.docker.memory_mb || 4096);
    }
    if (r.tmux) {
      setShell(r.tmux.default_shell || "/bin/bash");
      setHistory(r.tmux.history_limit || 10000);
    }
  }, [settings]);

  const dockerDown = data && !data.dockerOk;

  const persist = async () => {
    setSaving(true);
    setDraft({ runtime: choice });
    try {
      const base = settings?.runtime;
      await api.updateSettings({
        runtime: {
          default: choice,
          docker: {
            image,
            network: base?.docker?.network || "mycel-net",
            docker_socket_path: base?.docker?.docker_socket_path || "/var/run/docker.sock",
            extra_mounts: base?.docker?.extra_mounts || [],
            cpus,
            memory_mb: memoryMB,
          },
          tmux: {
            session_prefix: base?.tmux?.session_prefix || "mycel",
            history_limit: history,
            default_shell: shell,
          },
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
        Agents can run in isolated Docker containers or in local tmux sessions. You can change this
        any time in Settings.
      </p>

      <RuntimePicker value={choice} onChange={setChoice} />

      {choice === "docker" && dockerDown && (
        <div className="rounded-md border border-mycel-warning bg-mycel-warning-subtle px-3 py-2 text-[12px] text-mycel-warning">
          Docker isn't running. Start Docker Desktop (or the engine) before spawning agents, or pick
          tmux instead.
        </div>
      )}

      <AdvancedToggle open={advanced} onToggle={() => setAdvanced((v) => !v)} />

      {advanced && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 flex flex-col gap-4">
          {choice === "docker" ? (
            <>
              <label className="flex flex-col gap-1">
                <span className="text-[12px] text-mycel-text-2">Agent image</span>
                <input value={image} onChange={(e) => setImage(e.target.value)} className={TXT} />
              </label>
              <div className="flex gap-4 flex-wrap">
                <label className="flex flex-col gap-1">
                  <span className="text-[12px] text-mycel-text-2">CPUs</span>
                  <input type="number" min={0.5} step={0.5} value={cpus} onChange={(e) => setCpus(Number(e.target.value))} className={NUM} />
                </label>
                <label className="flex flex-col gap-1">
                  <span className="text-[12px] text-mycel-text-2">Memory (MB)</span>
                  <input type="number" min={512} step={512} value={memoryMB} onChange={(e) => setMemoryMB(Number(e.target.value))} className={NUM} />
                </label>
              </div>
            </>
          ) : (
            <div className="flex gap-4 flex-wrap">
              <label className="flex flex-col gap-1 flex-1 min-w-[12rem]">
                <span className="text-[12px] text-mycel-text-2">Default shell</span>
                <input value={shell} onChange={(e) => setShell(e.target.value)} className={TXT} />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[12px] text-mycel-text-2">History limit</span>
                <input type="number" min={1000} step={1000} value={history} onChange={(e) => setHistory(Number(e.target.value))} className={NUM} />
              </label>
            </div>
          )}
        </div>
      )}

      <WizardFooter nav={nav} primaryLabel={saving ? "Saving…" : "Continue"} onPrimary={persist} primaryDisabled={saving} />
    </div>
  );
}
