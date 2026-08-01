import { useEffect, useRef, useState } from "react";
import { installDep } from "./installStream";

/* ── InstallButton ────────────────────────────────────────────────────
 *
 * Runs one dependency's host install command and streams its output into a
 * small live console. The button click is the consent to run; the resolved
 * command is echoed as the first line so it's clear what executed. On a
 * clean exit it calls onDone so the caller can re-check readiness.
 */

type Status = "idle" | "running" | "ok" | "error";

export function InstallButton({
  id,
  label,
  onDone,
}: {
  id: string;
  label: string;
  onDone?: () => void;
}) {
  const [status, setStatus] = useState<Status>("idle");
  const [lines, setLines] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const consoleRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = consoleRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  const run = async () => {
    setStatus("running");
    setLines([]);
    setErr(null);
    try {
      const code = await installDep(id, (ev) => {
        if (ev.type === "start") setLines((l) => [...l, `$ ${ev.command}`]);
        else if (ev.type === "log") setLines((l) => [...l, ev.line]);
      });
      if (code === 0) {
        setStatus("ok");
        onDone?.();
      } else {
        setStatus("error");
        setErr(`Install exited with code ${code}.`);
      }
    } catch (e) {
      setStatus("error");
      setErr(e instanceof Error ? e.message : "Install failed.");
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={run}
          disabled={status === "running"}
          className="text-[12px] font-medium px-2.5 py-1 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-60"
        >
          {status === "running"
            ? "Installing…"
            : status === "ok"
              ? "Reinstall"
              : status === "error"
                ? "Retry install"
                : `Install ${label}`}
        </button>
        {status === "ok" && (
          <span className="text-[11px] text-mycel-success">Installed. Re-checking…</span>
        )}
        {status === "error" && err && (
          <span className="text-[11px] text-mycel-error truncate">{err}</span>
        )}
      </div>
      {(status === "running" || lines.length > 0) && (
        <div
          ref={consoleRef}
          className="max-h-32 overflow-auto rounded-md border border-mycel-border bg-mycel-bg px-2.5 py-1.5 font-mono text-[11px] leading-relaxed text-mycel-text-2 whitespace-pre-wrap"
        >
          {lines.map((l, i) => (
            <div key={i} className={l.startsWith("$ ") ? "text-mycel-accent" : ""}>
              {l}
            </div>
          ))}
          {status === "running" && <div className="text-mycel-muted">▍</div>}
        </div>
      )}
    </div>
  );
}
