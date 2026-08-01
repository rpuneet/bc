import { useEffect, useState } from "react";
import { api, type Agent } from "../../api/client";
import { CreateAgentModal } from "../../components/CreateAgentModal";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 6 — First agent. Opens the real CreateAgentModal so name/repo/
 * provider/model/template all behave exactly as on the Agents page. On
 * success the modal routes to the new agent's detail view. Skippable. */

export function StepAgent({ nav }: StepProps) {
  const [open, setOpen] = useState(false);
  const [agents, setAgents] = useState<Agent[]>([]);

  useEffect(() => {
    api
      .listAgents()
      .then(setAgents)
      .catch(() => setAgents([]));
  }, []);

  const hasAgent = agents.length > 0;

  return (
    <div className="flex flex-col gap-5">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        Time to grow your first agent. Give it a repo and a template — it gets its own git worktree
        and starts working. This is the same New Agent flow you'll use from the dashboard.
      </p>

      {hasAgent ? (
        <div className="rounded-lg border border-mycel-success bg-mycel-success-subtle px-4 py-3 flex items-center gap-2.5">
          <span className="w-2 h-2 rounded-full bg-mycel-success" aria-hidden />
          <span className="text-[13px] text-mycel-text">
            You already have {agents.length} agent{agents.length === 1 ? "" : "s"}. You can add more any time.
          </span>
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-mycel-border bg-mycel-surface px-4 py-8 flex flex-col items-center gap-3 text-center">
          <p className="text-[13px] text-mycel-muted max-w-xs">
            No agents yet. Spawn one to see mycel in action.
          </p>
          <button
            type="button"
            onClick={() => setOpen(true)}
            className="text-[13px] font-medium px-4 py-2 rounded-md bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors"
          >
            Create your first agent
          </button>
        </div>
      )}

      <CreateAgentModal
        open={open}
        onClose={() => setOpen(false)}
        existingNames={agents.map((a) => a.name)}
      />

      <WizardFooter nav={nav} primaryLabel="Continue" skipLabel={hasAgent ? "Skip" : "I'll do this later"} />
    </div>
  );
}
