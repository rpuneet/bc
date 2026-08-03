/**
 * providerActions — whether the daemon can actually carry out an install or
 * uninstall for a given install hint.
 *
 * The daemon derives both commands from a provider's install hint, and it
 * refuses the cases it cannot derive. The UI has to know the same rules, because
 * offering an action the server will refuse is worse than not offering it: the
 * refusal arrives after the click, and for Remove that is after a two-click
 * destructive confirm (#3475).
 *
 * These predicates therefore mirror the server deliberately, and are kept in one
 * place so a table and a detail page cannot drift into disagreeing about what is
 * possible — which is exactly how the table came to offer Install for a provider
 * whose detail page knew better.
 */

/**
 * canAutoInstall mirrors the daemon's providerInstallCmd/runnableInstallHint
 * predicate: a hint is executable unless it is empty or a bare download URL.
 *
 * Cursor's hint is literally "https://cursor.sh" — a page a human visits, not a
 * command. Offering Install for it led to a screen showing the URL as copyable
 * text, which is a fine thing to show and a poor thing to promise.
 */
export function canAutoInstall(hint: string | undefined | null): boolean {
  if (!hint) return false;
  const h = hint.trim();
  return h !== "" && !h.startsWith("http://") && !h.startsWith("https://");
}

/**
 * canAutoUninstall mirrors the daemon's deriveUninstall: only a global npm
 * install or a brew install can be turned into an uninstall command, and only
 * when a package name follows.
 *
 * Everything else — agy's `curl -fsSL … | sh`, cursor's URL — leaves nothing to
 * derive, and the daemon answers HTTP 400. The Remove button was styled
 * destructively and rendered anyway, so the only way to learn this was to
 * confirm twice and read an error.
 */
export function canAutoUninstall(hint: string | undefined | null): boolean {
  if (!hint) return false;
  const cmd = hint.trim();
  for (const prefix of ["npm install -g ", "npm i -g ", "brew install "]) {
    if (cmd.startsWith(prefix)) {
      return cmd.slice(prefix.length).trim() !== "";
    }
  }
  return false;
}
