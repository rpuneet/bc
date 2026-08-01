import { CopyButton } from "../../components/CopyButton";
import { useReadiness } from "../../hooks/useReadiness";
import type { RStatus, ReadinessItem } from "../../views/readiness/readiness";
import { InstallButton } from "../InstallButton";
import { WizardFooter } from "../WizardFooter";
import type { StepProps } from "../types";

/* Step 1 — System check & install deps. Reuses the readiness engine and
 * adds a streamed installer for each missing, auto-installable item. */

const STATUS_DOT: Record<RStatus, string> = {
  ok: "bg-mycel-success",
  warn: "bg-mycel-warning",
  fail: "bg-mycel-error",
};

// Items the host installer can't safely automate: Docker (platform-specific,
// interactive) and Cursor (install hint is a download page, not a command).
const NON_INSTALLABLE = new Set(["docker", "cursor"]);

function ItemRow({ item, onInstalled }: { item: ReadinessItem; onInstalled: () => void }) {
  const needsWork = item.status !== "ok";
  const canInstall = needsWork && !NON_INSTALLABLE.has(item.key);
  return (
    <div className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-start gap-2.5">
        <span className={`mt-1.5 inline-flex shrink-0 w-2 h-2 rounded-full ${STATUS_DOT[item.status]}`} aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2 flex-wrap">
            <span className="text-[13px] text-mycel-text font-medium">{item.label}</span>
            <span className={`text-[11px] ${item.status === "ok" ? "text-mycel-muted" : "text-mycel-text-2"} truncate`}>
              {item.detail}
            </span>
          </div>
          {item.note && <p className="mt-0.5 text-[11px] text-mycel-muted leading-relaxed">{item.note}</p>}
        </div>
      </div>
      {canInstall && (
        <div className="ml-[18px]">
          <InstallButton id={item.key} label={item.label} onDone={onInstalled} />
        </div>
      )}
      {needsWork && !canInstall && item.fix && (
        <div className="ml-[18px] flex items-center gap-1.5 rounded-md border border-mycel-border bg-mycel-bg pl-2.5 pr-1 py-1">
          <code className="flex-1 min-w-0 font-mono text-[11px] text-mycel-text overflow-x-auto whitespace-nowrap">
            {item.fix}
          </code>
          <CopyButton text={item.fix} />
        </div>
      )}
    </div>
  );
}

export function StepSystem({ nav }: StepProps) {
  const { data, loading, loaded, error, refresh } = useReadiness();

  return (
    <div className="flex flex-col gap-5">
      <p className="text-[14px] leading-relaxed text-mycel-text-2 max-w-prose">
        These are what mycel needs to run agents. Install anything missing right here — output
        streams live, and we re-check when it finishes. Nothing here is destructive.
      </p>

      {error && !data && (
        <div role="alert" className="rounded-md border border-mycel-border bg-mycel-error-subtle px-3 py-2 text-xs text-mycel-error">
          Couldn't reach the daemon: {error}
        </div>
      )}
      {!loaded && !data && !error && (
        <div className="text-sm text-mycel-muted py-6 text-center">Checking your machine…</div>
      )}

      {data && (
        <div className="flex flex-col gap-3">
          {data.groups.map((g) => (
            <section key={g.id} className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden shadow-mycel">
              <header className="flex items-start gap-2.5 px-4 py-2.5 border-b border-mycel-border bg-mycel-bg">
                <span className={`mt-1.5 inline-flex shrink-0 w-2 h-2 rounded-full ${STATUS_DOT[g.status]}`} aria-hidden />
                <div className="min-w-0">
                  <h2 className="text-[13px] font-semibold text-mycel-text">{g.title}</h2>
                  <p className="mt-0.5 text-[11px] text-mycel-muted leading-relaxed">{g.summary}</p>
                </div>
              </header>
              <div className="divide-y divide-mycel-border">
                {g.items.map((it) => (
                  <ItemRow key={it.key} item={it} onInstalled={() => void refresh()} />
                ))}
              </div>
            </section>
          ))}
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={loading}
            className="self-start text-[11px] px-2.5 py-1 rounded border border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-muted hover:text-mycel-text transition-colors disabled:opacity-50"
          >
            {loading ? "Checking…" : "Re-check"}
          </button>
        </div>
      )}

      <WizardFooter nav={nav} primaryLabel="Continue" skipLabel="Skip for now" />
    </div>
  );
}
