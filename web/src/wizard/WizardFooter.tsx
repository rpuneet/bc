import type { WizardNav } from "./types";

/* ── WizardFooter ─────────────────────────────────────────────────────
 *
 * The shared step footer: Back on the left, then Skip and a primary
 * action on the right. Steps supply their own primary label/handler so the
 * verb matches what the step actually does ("Continue", "Finish setup").
 */

export function WizardFooter({
  nav,
  primaryLabel = "Continue",
  onPrimary,
  primaryDisabled = false,
  skipLabel = "Skip",
  hideSkip = false,
}: {
  nav: WizardNav;
  primaryLabel?: string;
  onPrimary?: () => void;
  primaryDisabled?: boolean;
  skipLabel?: string;
  hideSkip?: boolean;
}) {
  return (
    <div className="mt-8 flex items-center justify-between gap-3 border-t border-mycel-border pt-5">
      <button
        type="button"
        onClick={nav.back}
        disabled={nav.isFirst}
        className="text-[13px] px-3 py-2 rounded-md text-mycel-muted hover:text-mycel-text cursor-pointer transition-colors disabled:opacity-0 disabled:pointer-events-none"
      >
        &larr; Back
      </button>
      <div className="flex items-center gap-2">
        {!hideSkip && (
          <button
            type="button"
            onClick={nav.skip}
            className="text-[13px] px-3 py-2 rounded-md text-mycel-muted hover:text-mycel-text cursor-pointer transition-colors"
          >
            {skipLabel}
          </button>
        )}
        <button
          type="button"
          onClick={onPrimary ?? nav.next}
          disabled={primaryDisabled}
          className="text-[13px] font-medium px-4 py-2 rounded-md bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover cursor-pointer shadow-mycel-sm transition-all active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none"
        >
          {primaryLabel}
        </button>
      </div>
    </div>
  );
}
