import { useHistoryNav } from "../hooks/useHistoryNav";

/**
 * HistoryNavButtons — the one back/forward control in the app, rendered
 * in the header next to the brand column. Mirrors a native browser's
 * back/forward buttons (and the Cmd+ArrowLeft / Cmd+ArrowRight shortcut
 * that already works via the browser itself): chevron-left steps back
 * one history entry, chevron-right steps forward one.
 *
 * react-router v6 doesn't expose canGoBack/canGoForward, so there is no
 * reliable way to grey a button out only when it would be a no-op. Both
 * stay enabled at all times rather than faking a disabled state that
 * could be wrong; they're styled subtly (muted, low-contrast chrome) so
 * an inert click reads as unsurprising rather than as a broken control —
 * the same trade-off real browsers make for tab-restore edge cases.
 */
export function HistoryNavButtons() {
  const { goBack, goForward } = useHistoryNav();
  return (
    <div className="flex items-center shrink-0">
      <button
        type="button"
        onClick={goBack}
        aria-label="Go back"
        title="Go back"
        className="flex items-center justify-center shrink-0 w-11 h-11 text-mycel-muted hover:text-mycel-text focus-visible:text-mycel-text rounded-md outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent transition-colors"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <polyline points="15 18 9 12 15 6" />
        </svg>
      </button>
      <button
        type="button"
        onClick={goForward}
        aria-label="Go forward"
        title="Go forward"
        className="flex items-center justify-center shrink-0 w-11 h-11 text-mycel-muted hover:text-mycel-text focus-visible:text-mycel-text rounded-md outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent transition-colors"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <polyline points="9 18 15 12 9 6" />
        </svg>
      </button>
    </div>
  );
}
