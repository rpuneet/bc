import { Link } from "react-router-dom";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 7 — Done. Marks onboarding complete and points at where to go next. */

function NextCard({ to, title, body }: { to: string; title: string; body: string }) {
  return (
    <Link
      to={to}
      className="flex-1 rounded-lg border border-mycel-border bg-mycel-surface p-4 hover:border-mycel-accent transition-colors group"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[13px] font-semibold text-mycel-text">{title}</span>
        <span className="text-mycel-muted group-hover:text-mycel-accent transition-colors">→</span>
      </div>
      <p className="mt-1 text-[12px] text-mycel-text-2 leading-relaxed">{body}</p>
    </Link>
  );
}

export function StepDone({ nav }: StepProps) {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <span className="relative flex h-3 w-3">
          <span className="absolute inline-flex h-full w-full rounded-full bg-mycel-success opacity-50 animate-ping [animation-duration:2.5s] motion-reduce:hidden" />
          <span className="relative inline-flex h-3 w-3 rounded-full bg-mycel-success" />
        </span>
        <p className="text-[14px] text-mycel-text-2">
          Your machine is ready and your defaults are set. mycel grows from here.
        </p>
      </div>

      <div className="flex flex-col sm:flex-row gap-3">
        <NextCard to="/marketplace" title="Marketplace" body="Browse skills, MCP servers, and agent templates to install." />
        <NextCard to="/templates" title="Templates" body="Reusable agent presets — role, prompt, and tools in one." />
      </div>

      <WizardFooter nav={nav} primaryLabel="Finish setup" onPrimary={nav.finish} hideSkip />
    </div>
  );
}
