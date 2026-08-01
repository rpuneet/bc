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
 * installDep streams the install of one readiness item (git, tmux, a
 * provider CLI…). It calls onEvent for every record and resolves with the
 * process exit code (0 = success). Throws on transport errors or an
 * explicit error event.
 */
export async function installDep(
  id: string,
  onEvent: (ev: InstallEvent) => void,
  signal?: AbortSignal,
): Promise<number> {
  const res = await fetch("/api/deps/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
    signal,
  });

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
