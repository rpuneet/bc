/**
 * WorkspacePicker - Shown at /w when no workspace is selected.
 *
 * - Lists registered workspaces (from useWorkspace)
 * - "Scan local" button - POSTs /api/workspaces/discover/local
 * - "From GitHub" button - GET /api/auth/github -> start OAuth flow
 * - "Add manually" button - opens the manual-add form
 *
 * Clicking a workspace navigates to /w/<id>/live.
 */

import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useWorkspace, type WorkspaceSummary } from "../context/WorkspaceContext";
import { MONO } from "../utils/typography";

export function WorkspacePicker() {
  const { workspaces, loading, refresh } = useWorkspace();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const fromPath = searchParams.get("from");
  const [mode, setMode] = useState<"list" | "scan" | "github" | "manual">("list");

  const open = (ws: WorkspaceSummary) => {
    navigate(`/w/${ws.id}/live`);
  };

  return (
    <div className="flex-1 flex items-start justify-center p-8 overflow-y-auto">
      <div className="w-full max-w-2xl space-y-6" style={{ fontFamily: MONO }}>
        <header className="space-y-1">
          <h1 className="text-xl font-bold text-bc-text">Workspaces</h1>
          <p className="text-[12px] text-bc-muted">
            Pick a workspace to continue, or add a new one.
          </p>
          {fromPath && (
            <p className="text-[11px] text-bc-muted/60 italic">
              The URL {fromPath} is not associated with any known workspace.
            </p>
          )}
        </header>

        {/* Registered workspaces list */}
        <section>
          <div className="text-[10px] text-bc-muted/50 uppercase tracking-[0.2em] mb-2">
            Registered
          </div>
          {loading && <p className="text-[12px] text-bc-muted/50">loading…</p>}
          {!loading && workspaces.length === 0 && (
            <p className="text-[12px] text-bc-muted/60 italic">
              No workspaces registered yet. Add one below.
            </p>
          )}
          {!loading && workspaces.length > 0 && (
            <div className="space-y-1.5">
              {workspaces.map((ws) => (
                <button
                  key={ws.id}
                  type="button"
                  onClick={() => open(ws)}
                  className="w-full flex items-center gap-3 px-3 py-2.5 rounded-md border border-bc-border/40 bg-bc-surface/20 hover:border-bc-accent/50 hover:bg-bc-accent/[0.04] transition-colors text-left"
                >
                  <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${ws.active ? "bg-bc-accent" : "bg-bc-muted/30"}`} />
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-[13px] font-semibold text-bc-text">
                        {ws.name}
                      </span>
                      <span className="text-[9px] text-bc-muted/40 tabular-nums">
                        [{ws.id.slice(0, 6)}]
                      </span>
                      {ws.alias && (
                        <span className="text-[10px] text-bc-muted/60">@{ws.alias}</span>
                      )}
                    </div>
                    <div className="text-[10px] text-bc-muted/50 truncate" title={ws.path}>
                      {ws.path}
                    </div>
                  </div>
                  {ws.github_url && (
                    <span
                      className="text-[9px] text-bc-muted/40 truncate max-w-[180px]"
                      title={ws.github_url}
                    >
                      {ws.github_url.replace(/^https?:\/\/github\.com\//, "gh:")}
                    </span>
                  )}
                </button>
              ))}
            </div>
          )}
        </section>

        {/* Add workspace options */}
        <section className="space-y-3">
          <div className="text-[10px] text-bc-muted/50 uppercase tracking-[0.2em]">
            Add workspace
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <button
              type="button"
              onClick={() => setMode("scan")}
              className={`px-3 py-2.5 rounded-md border text-[11px] transition-colors text-left ${
                mode === "scan"
                  ? "border-bc-accent bg-bc-accent/10 text-bc-text"
                  : "border-bc-border/40 bg-bc-surface/20 text-bc-muted hover:text-bc-text hover:border-bc-border/70"
              }`}
            >
              <div className="font-semibold mb-0.5">Scan local</div>
              <div className="text-[10px] text-bc-muted/60">Find .git repos on disk</div>
            </button>
            <button
              type="button"
              onClick={() => setMode("github")}
              className={`px-3 py-2.5 rounded-md border text-[11px] transition-colors text-left ${
                mode === "github"
                  ? "border-bc-accent bg-bc-accent/10 text-bc-text"
                  : "border-bc-border/40 bg-bc-surface/20 text-bc-muted hover:text-bc-text hover:border-bc-border/70"
              }`}
            >
              <div className="font-semibold mb-0.5">From GitHub</div>
              <div className="text-[10px] text-bc-muted/60">List + clone your repos</div>
            </button>
            <button
              type="button"
              onClick={() => setMode("manual")}
              className={`px-3 py-2.5 rounded-md border text-[11px] transition-colors text-left ${
                mode === "manual"
                  ? "border-bc-accent bg-bc-accent/10 text-bc-text"
                  : "border-bc-border/40 bg-bc-surface/20 text-bc-muted hover:text-bc-text hover:border-bc-border/70"
              }`}
            >
              <div className="font-semibold mb-0.5">Add manually</div>
              <div className="text-[10px] text-bc-muted/60">By absolute path</div>
            </button>
          </div>

          {mode === "scan" && <ScanPane onDone={refresh} />}
          {mode === "github" && <GitHubPane onDone={refresh} />}
          {mode === "manual" && <ManualPane onDone={refresh} />}
        </section>
      </div>
    </div>
  );
}

/* ── ScanPane ─────────────────────────────────────────────────────── */

interface ScanCandidate {
  path: string;
  name: string;
  github_url?: string;
  already_registered?: boolean;
}

function ScanPane({ onDone }: { onDone: () => void }) {
  const [root, setRoot] = useState("~/Projects");
  const [depth, setDepth] = useState(3);
  const [candidates, setCandidates] = useState<ScanCandidate[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [scanning, setScanning] = useState(false);
  const [adding, setAdding] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const scan = async () => {
    setScanning(true);
    setErr(null);
    try {
      const r = await fetch("/api/workspaces/discover/local", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ root, depth }),
      });
      if (!r.ok) {
        setErr(`Scan failed: ${String(r.status)}`);
        setCandidates([]);
        return;
      }
      const data = (await r.json()) as ScanCandidate[];
      setCandidates(data);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "scan failed");
    } finally {
      setScanning(false);
    }
  };

  const addSelected = async () => {
    setAdding(true);
    setErr(null);
    try {
      for (const path of selected) {
        await fetch("/api/workspaces", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ path }),
        });
      }
      setSelected(new Set());
      setCandidates(null);
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "add failed");
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="rounded-md border border-bc-border/40 bg-bc-surface/10 p-3 space-y-2">
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={root}
          onChange={(e) => setRoot(e.target.value)}
          placeholder="~/Projects"
          className="flex-1 rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1 text-[11px] text-bc-text/90 outline-none focus:border-bc-accent/50"
        />
        <input
          type="number"
          value={depth}
          onChange={(e) => setDepth(Number(e.target.value) || 3)}
          min={1}
          max={6}
          className="w-14 rounded border border-bc-border/40 bg-bc-bg px-2 py-1 text-[11px] text-center text-bc-text/90 outline-none focus:border-bc-accent/50"
          title="Scan depth"
        />
        <button
          type="button"
          onClick={() => void scan()}
          disabled={scanning}
          className="px-3 py-1 rounded text-[11px] border border-bc-accent/30 bg-bc-accent/10 text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-50"
        >
          {scanning ? "scanning…" : "Scan"}
        </button>
      </div>

      {err && <p className="text-[11px] text-bc-error">{err}</p>}

      {candidates && candidates.length === 0 && (
        <p className="text-[11px] text-bc-muted/60 italic">No repos found under {root}</p>
      )}

      {candidates && candidates.length > 0 && (
        <>
          <div className="max-h-[260px] overflow-y-auto divide-y divide-bc-border/30 rounded border border-bc-border/40">
            {candidates.map((c) => (
              <label
                key={c.path}
                className="flex items-center gap-2 px-2.5 py-1.5 hover:bg-bc-surface/40 cursor-pointer text-[11px]"
              >
                <input
                  type="checkbox"
                  disabled={c.already_registered}
                  checked={selected.has(c.path)}
                  onChange={(e) => {
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (e.target.checked) next.add(c.path);
                      else next.delete(c.path);
                      return next;
                    });
                  }}
                />
                <span className="font-semibold text-bc-text/90">{c.name}</span>
                <span className="text-bc-muted/50 truncate" title={c.path}>
                  {c.path}
                </span>
                {c.already_registered && (
                  <span className="ml-auto text-[9px] text-bc-muted/40 uppercase">already added</span>
                )}
              </label>
            ))}
          </div>
          <button
            type="button"
            disabled={selected.size === 0 || adding}
            onClick={() => void addSelected()}
            className="px-3 py-1.5 rounded text-[11px] border border-bc-accent/30 bg-bc-accent/10 text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
          >
            {adding ? "adding…" : `Add ${String(selected.size)} workspace${selected.size === 1 ? "" : "s"}`}
          </button>
        </>
      )}
    </div>
  );
}

/* ── GitHubPane ───────────────────────────────────────────────────── */

function GitHubPane({ onDone: _onDone }: { onDone: () => void }) {
  return (
    <div className="rounded-md border border-bc-border/40 bg-bc-surface/10 p-3 text-[11px] text-bc-muted space-y-2">
      <p>
        GitHub integration coming soon. Until then, clone a repo locally and use
        <span className="text-bc-text font-semibold"> Scan local</span> or
        <span className="text-bc-text font-semibold"> Add manually</span>.
      </p>
      <p className="text-[10px] text-bc-muted/50">
        Tracked in: docs/proposals/multi-workspace-and-code-tab.md \u00A7 4.4
      </p>
    </div>
  );
}

/* ── ManualPane ───────────────────────────────────────────────────── */

function ManualPane({ onDone }: { onDone: () => void }) {
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [alias, setAlias] = useState("");
  const [adding, setAdding] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!path.trim()) return;
    setAdding(true);
    setErr(null);
    try {
      const r = await fetch("/api/workspaces", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: path.trim(),
          name: name.trim() || undefined,
          alias: alias.trim() || undefined,
        }),
      });
      if (!r.ok) {
        const msg = (await r.text()) || `status ${String(r.status)}`;
        setErr(msg);
        return;
      }
      setPath("");
      setName("");
      setAlias("");
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "add failed");
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="rounded-md border border-bc-border/40 bg-bc-surface/10 p-3 space-y-2">
      <input
        type="text"
        value={path}
        onChange={(e) => setPath(e.target.value)}
        placeholder="/absolute/path/to/repo"
        className="w-full rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1.5 text-[11px] text-bc-text/90 outline-none focus:border-bc-accent/50"
      />
      <div className="flex gap-2">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="name (optional)"
          className="flex-1 rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1.5 text-[11px] text-bc-text/90 outline-none focus:border-bc-accent/50"
        />
        <input
          type="text"
          value={alias}
          onChange={(e) => setAlias(e.target.value)}
          placeholder="alias (optional)"
          className="w-28 rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1.5 text-[11px] text-bc-text/90 outline-none focus:border-bc-accent/50"
        />
      </div>
      {err && <p className="text-[11px] text-bc-error">{err}</p>}
      <button
        type="button"
        disabled={!path.trim() || adding}
        onClick={() => void submit()}
        className="px-3 py-1.5 rounded text-[11px] border border-bc-accent/30 bg-bc-accent/10 text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
      >
        {adding ? "adding…" : "Add workspace"}
      </button>
    </div>
  );
}
