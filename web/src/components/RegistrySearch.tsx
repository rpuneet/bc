import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import { api } from "../api/client";
import type { PackageManager, PackageSearchResult } from "../api/client";
import { installPackage } from "../wizard/installStream";
import { CopyButton } from "./CopyButton";

/* ── RegistrySearch ───────────────────────────────────────────────────
 *
 * Seam 1: a guarded browse/search over the host's package registries. Pick a
 * detected, searchable manager, type a package name, and the daemon runs that
 * manager's own `search` subcommand (validated charset, argv-slice exec, no
 * shell, timeout, capped results). Each hit gets an Install action:
 *   - brew / npm / cargo  → streamed install (no sudo, non-interactive)
 *   - everything else      → a copyable command, honestly "run in your terminal"
 *
 * Managers with no vetted search spec are never offered here — the picker only
 * lists searchable ones, and an empty picker says so plainly.
 */

// Client mirror of the server's pkgQueryPattern so we can disable Search and
// explain why before making a doomed request.
const QUERY_RE = /^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$/;

// Copyable install command per manager, for the ones the server won't install
// directly (sudo / non-standard). Honest, terminal-ready lines; `{p}` is
// replaced with the (already validated) name. The trailing default covers any
// manager not listed so a copy never yields a bare package name.
const COPY_INSTALL: Record<string, string> = {
  apt: "sudo apt-get install -y {p}",
  dnf: "sudo dnf install {p}",
  yum: "sudo yum install {p}",
  pacman: "sudo pacman -S {p}",
  zypper: "sudo zypper install {p}",
  winget: "winget install {p}",
};

function copyInstallCmd(manager: string, name: string): string {
  return (COPY_INSTALL[manager] ?? `${manager} install {p}`).replace("{p}", name);
}

type SearchState = "idle" | "searching" | "done" | "error";

export function RegistrySearch() {
  const [managers, setManagers] = useState<PackageManager[]>([]);
  const [loadingManagers, setLoadingManagers] = useState(true);
  const [manager, setManager] = useState("");
  const [query, setQuery] = useState("");
  const [state, setState] = useState<SearchState>("idle");
  const [results, setResults] = useState<PackageSearchResult[]>([]);
  const [note, setNote] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .getPackageManagers()
      .then((res) => {
        if (!alive) return;
        const searchable = (res.managers ?? []).filter((m) => m.searchable);
        setManagers(searchable);
        if (searchable.length > 0 && searchable[0]) setManager(searchable[0].id);
      })
      .catch(() => { /* leave picker empty — the empty-state copy covers it */ })
      .finally(() => { if (alive) setLoadingManagers(false); });
    return () => { alive = false; };
  }, []);

  const directInstall = managers.find((m) => m.id === manager)?.direct_install ?? false;

  const queryValid = QUERY_RE.test(query.trim());

  const runSearch = async () => {
    const q = query.trim();
    if (!manager || !queryValid) return;
    setState("searching");
    setResults([]);
    setNote(null);
    try {
      const res = await api.searchPackages(manager, q);
      setResults(res.results ?? []);
      setNote(res.error ?? null);
      setState("done");
    } catch (e) {
      setNote(e instanceof Error ? e.message : "Search failed.");
      setState("error");
    }
  };

  if (loadingManagers) {
    return (
      <div className="h-8 w-64 animate-pulse rounded-md bg-mycel-surface-hover" aria-busy aria-label="Detecting package managers" />
    );
  }

  if (managers.length === 0) {
    return (
      <p className="text-[11px] text-mycel-muted">
        No searchable package manager detected on this host. Registry search needs one of
        brew, npm, apt, dnf, pacman, cargo… on PATH.
      </p>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <select
          value={manager}
          onChange={(e) => { setManager(e.target.value); setResults([]); setState("idle"); setNote(null); }}
          aria-label="Registry to search"
          className="px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent"
        >
          {managers.map((m) => (
            <option key={m.id} value={m.id}>{m.name}</option>
          ))}
        </select>
        <div className="relative flex-1 min-w-[180px]">
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void runSearch(); }}
            placeholder="Search the registry (e.g. ripgrep)…"
            aria-label="Package search query"
            className="w-full px-2.5 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          />
        </div>
        <button
          type="button"
          onClick={() => void runSearch()}
          disabled={!queryValid || state === "searching"}
          className="inline-flex items-center gap-1.5 h-8 px-3 text-sm font-medium rounded-md bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent"
        >
          {state === "searching" ? "Searching…" : "Search"}
        </button>
      </div>

      {query.trim() !== "" && !queryValid && (
        <p className="text-[11px] text-mycel-warning">
          Enter a single package name — letters, digits and . _ + - only (no spaces or shell characters).
        </p>
      )}

      {note && (
        <p className={`text-[11px] ${state === "error" ? "text-mycel-error" : "text-mycel-muted"}`}>{note}</p>
      )}

      {state === "done" && results.length === 0 && !note && (
        <p className="text-[11px] text-mycel-muted">No packages matched “{query.trim()}”.</p>
      )}

      {results.length > 0 && (
        <ul className="rounded-lg border border-mycel-border divide-y divide-mycel-border overflow-hidden">
          {results.map((r, i) => (
            <ResultRow key={`${manager}:${r.name}:${i}`} manager={manager} direct={directInstall} result={r} />
          ))}
        </ul>
      )}
    </div>
  );
}

function ResultRow({ manager, direct, result }: { manager: string; direct: boolean; result: PackageSearchResult }) {
  const copyCmd = copyInstallCmd(manager, result.name);

  return (
    <li className="px-3 py-2 bg-mycel-surface">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <span className="text-sm font-medium text-mycel-text font-mono">{result.name}</span>
          {result.description && (
            <p className="text-[11px] text-mycel-muted truncate max-w-md">{result.description}</p>
          )}
        </div>
        <div className="shrink-0">
          {direct ? (
            <PackageInstallButton manager={manager} pkg={result.name} />
          ) : (
            <div className="flex items-center gap-1" title="Needs sudo / a terminal — run it yourself">
              <code className="text-[11px] font-mono text-mycel-text-2 bg-mycel-bg rounded-md px-2 py-1 border border-mycel-border">{copyCmd}</code>
              <CopyButton text={copyCmd} />
            </div>
          )}
        </div>
      </div>
    </li>
  );
}

type RunState = "idle" | "running" | "ok" | "error";

/* Streamed install of one searched package. Mirrors the deps installer's
 * honest state machine: indeterminate bar + live line count while running,
 * resolved green/red on completion. */
function PackageInstallButton({ manager, pkg }: { manager: string; pkg: string }) {
  const reduceMotion = useReducedMotion();
  const [state, setState] = useState<RunState>("idle");
  const [lines, setLines] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const consoleRef = useRef<HTMLDivElement>(null);
  const runningRef = useRef(false);

  useEffect(() => {
    const el = consoleRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);
  useEffect(() => () => { runningRef.current = false; }, []);

  const run = async () => {
    setState("running");
    setLines([]);
    setErr(null);
    runningRef.current = true;
    try {
      const code = await installPackage(manager, pkg, (ev) => {
        if (!runningRef.current) return;
        if (ev.type === "start") setLines((l) => [...l, `$ ${ev.command}`]);
        else if (ev.type === "log") setLines((l) => [...l, ev.line]);
      });
      if (!runningRef.current) return;
      if (code === 0) setState("ok");
      else { setState("error"); setErr(`Install exited with code ${code}.`); }
    } catch (e) {
      if (!runningRef.current) return;
      setState("error");
      setErr(e instanceof Error ? e.message : "Install failed.");
    } finally {
      runningRef.current = false;
    }
  };

  const barTone = state === "ok" ? "bg-mycel-success" : state === "error" ? "bg-mycel-error" : "bg-mycel-accent";

  return (
    <div className="flex flex-col items-end gap-1.5">
      <button
        type="button"
        onClick={() => void run()}
        disabled={state === "running"}
        className="inline-flex items-center gap-1.5 text-[11px] font-medium px-2.5 py-1 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-60 focus-visible:ring-2 focus-visible:ring-mycel-accent"
      >
        {state === "running" ? "Installing…" : state === "ok" ? "Installed ✓" : state === "error" ? "Retry install" : "Install"}
      </button>
      {(state === "running" || state === "ok" || state === "error") && (
        <div className="w-56 space-y-1.5">
          <div className="h-1 rounded-full bg-mycel-border overflow-hidden" role="progressbar" aria-busy={state === "running"} aria-label="Install progress">
            {state === "running" ? (
              <div className={`h-full w-1/3 rounded-full ${barTone} ${reduceMotion ? "" : "animate-indeterminate"}`} />
            ) : (
              <div className={`h-full w-full rounded-full ${barTone}`} />
            )}
          </div>
          <AnimatePresence>
            {lines.length > 0 && (
              <motion.div
                ref={consoleRef}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                className="max-h-32 overflow-auto rounded-md border border-mycel-border bg-mycel-bg px-2 py-1 font-mono text-[10.5px] leading-relaxed text-mycel-text-2 whitespace-pre-wrap text-left"
              >
                {lines.map((l, i) => (
                  <div key={i} className={l.startsWith("$ ") ? "text-mycel-accent" : ""}>{l}</div>
                ))}
              </motion.div>
            )}
          </AnimatePresence>
          {err && <p className="text-[10.5px] text-mycel-error">{err}</p>}
        </div>
      )}
    </div>
  );
}
