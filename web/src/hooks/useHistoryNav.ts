import { useCallback } from "react";
import { useNavigate } from "react-router-dom";

/**
 * useHistoryNav — the single source of truth for "go back / go forward"
 * inside the app. Backed by react-router's own history stack via
 * `navigate(-1)` / `navigate(1)`, which is exactly what the browser's
 * native Cmd+ArrowLeft / Cmd+ArrowRight (and the OS/browser chrome back
 * and forward buttons) already drive — there is no separate in-app
 * keydown handler to keep in sync with, because none exists: the
 * shortcut is native browser behavior, not app code. This hook exists so
 * every clickable "back"/"forward" affordance (currently just the header
 * buttons) calls the exact same thing the native shortcut calls, instead
 * of each view growing its own bespoke back button.
 *
 * react-router v6 does not expose whether there is a prior/next entry to
 * go back/forward to (no `canGoBack`/`canGoForward`), so callers cannot
 * reliably disable either button. Consumers should keep both enabled and
 * style them as subordinate/subtle chrome rather than fake-disabling
 * them — see HistoryNavButtons.
 */
export function useHistoryNav() {
  const navigate = useNavigate();
  const goBack = useCallback(() => navigate(-1), [navigate]);
  const goForward = useCallback(() => navigate(1), [navigate]);
  return { goBack, goForward };
}
