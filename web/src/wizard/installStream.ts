/* ── installStream ────────────────────────────────────────────────────
 *
 * Client for POST /api/deps/install. The server runs the dependency's
 * install command on the host and streams its output as newline-delimited
 * JSON; this reads that stream and hands each event to the caller so the
 * wizard can show live progress.
 */

export type InstallEvent =
  | { type: "start"; command: string }
  | { type: "log"; line: string }
  | { type: "done"; code: number }
  | { type: "error"; error: string };

/**
 * installDep streams the install (or update) of one readiness item / tool
 * (git, tmux, a provider CLI, a registered CLI tool…). It calls onEvent for
 * every record and resolves with the process exit code (0 = success). Throws
 * on transport errors or an explicit error event.
 *
 * mode "update" runs the tool's upgrade command when the registry defines
 * one; it falls back to the install command otherwise.
 */
export async function installDep(
  id: string,
  onEvent: (ev: InstallEvent) => void,
  opts?: { signal?: AbortSignal; mode?: "install" | "update" | "uninstall" },
): Promise<number> {
  const res = await fetch("/api/deps/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, mode: opts?.mode ?? "install" }),
    signal: opts?.signal,
  });

  return consumeInstallStream(res, onEvent);
}

/**
 * installPackage streams the install of a registry-searched package via an
 * allowlisted manager (brew / npm / cargo) over POST /api/system/package-install
 * — the same NDJSON stream shape as installDep. The server validates the
 * package-name charset and execs the manager directly with an argv slice.
 */
export async function installPackage(
  manager: string,
  pkg: string,
  onEvent: (ev: InstallEvent) => void,
  opts?: { signal?: AbortSignal },
): Promise<number> {
  const res = await fetch("/api/system/package-install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ manager, package: pkg }),
    signal: opts?.signal,
  });
  return consumeInstallStream(res, onEvent);
}

/** Reads an NDJSON install stream, dispatching each event and resolving with
 *  the process exit code. Throws on transport errors or an error event. */
async function consumeInstallStream(
  res: Response,
  onEvent: (ev: InstallEvent) => void,
): Promise<number> {
  if (!res.ok || !res.body) {
    let msg = `Install failed: ${res.status}`;
    try {
      const b = await res.json();
      if (b && typeof b.error === "string") msg = b.error;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(msg);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  let code = -1;

  const handleLine = (raw: string) => {
    const line = raw.trim();
    if (!line) return;
    let ev: InstallEvent;
    try {
      ev = JSON.parse(line) as InstallEvent;
    } catch {
      return;
    }
    onEvent(ev);
    if (ev.type === "done") code = ev.code;
    if (ev.type === "error") throw new Error(ev.error);
  };

  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let nl = buf.indexOf("\n");
    while (nl >= 0) {
      handleLine(buf.slice(0, nl));
      buf = buf.slice(nl + 1);
      nl = buf.indexOf("\n");
    }
  }
  if (buf.trim()) handleLine(buf);
  return code;
}
