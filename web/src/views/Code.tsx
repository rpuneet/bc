/**
 * Code - Top-level VS Code-like tab.
 *
 * Layout:
 *   - Header (shared): title, worktree dropdown, view mode toggle (diff/plain)
 *   - Body: <CodeBrowser> — file tree (left) + Monaco viewer (right)
 *
 * Modes:
 *   - view : Monaco read-only viewer (default for main repo)
 *   - diff : Monaco DiffEditor, base = main repo, modified = agent worktree
 *           (default when a worktree other than main is selected)
 *
 * State (path / view / show_hidden / worktree) lives in URL search params;
 * the browser itself is extracted to components/code/CodeBrowser.tsx so the
 * agent detail Code tab can embed it pinned to one worktree.
 */

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import {
  CodeBreadcrumbs,
  CodeBrowser,
  DownloadPatchButton,
  HiddenToggle,
  ViewModeToggle,
} from "../components/code/CodeBrowser";
import type { CodeBrowserState, ViewMode } from "../components/code/CodeBrowser";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { MONO } from "../utils/typography";

interface WorktreeOption {
  value: string; // "main" or agent name
  label: string;
  path?: string;
}

async function fetchCodeServerStatus(): Promise<{ running: boolean; endpoint: string }> {
  try {
    const r = await fetch("/api/deps/bc-code-server/status");
    if (!r.ok) return { running: false, endpoint: "" };
    const d = (await r.json()) as { state?: string };
    return {
      running: d.state === "running",
      // Hardcoded for now — backend exposes on :8100 per bc_code_server.go
      endpoint: "http://localhost:8100/?folder=/home/coder/workspace",
    };
  } catch {
    return { running: false, endpoint: "" };
  }
}

export function Code() {
  const [searchParams, setSearchParams] = useSearchParams();

  const worktree = searchParams.get("worktree") ?? "main";
  const path = searchParams.get("path") ?? "";
  const showHidden = searchParams.get("show_hidden") === "1";
  const urlView = searchParams.get("view");
  const vscodeMode = searchParams.get("mode") === "vscode";
  const viewMode: ViewMode =
    urlView === "plain" || urlView === "diff"
      ? (urlView as ViewMode)
      : worktree !== "main"
        ? "diff"
        : "plain";

  // Poll code-server dep status every 5s so the "Edit in VS Code" toggle
  // appears/disappears as the dep starts/stops.
  const [codeServer, setCodeServer] = useState<{ running: boolean; endpoint: string }>({
    running: false,
    endpoint: "",
  });
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      if (typeof document !== "undefined" && document.hidden) return;
      const s = await fetchCodeServerStatus();
      if (!cancelled) setCodeServer(s);
    };
    void tick();
    const id = setInterval(() => void tick(), 10000);
    const onVis = () => {
      if (!document.hidden) void tick();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      cancelled = true;
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, []);

  const [worktrees, setWorktrees] = useState<WorktreeOption[]>([
    { value: "main", label: "main repo" },
  ]);

  // -------- worktree options --------
  useEffect(() => {
    let cancelled = false;
    void api
      .listAgents()
      .then((agents) => {
        if (cancelled) return;
        const opts: WorktreeOption[] = [
          { value: "main", label: "main repo" },
          ...agents.map((a) => ({
            value: a.name,
            label: `${a.name} (worktree)`,
          })),
        ];
        setWorktrees(opts);
      })
      .catch(() => {
        /* best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // -------- URL helpers --------
  const updateParams = useCallback(
    (patch: Partial<{ worktree: string; path: string; view: ViewMode | null; show_hidden: boolean }>) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (patch.worktree !== undefined) next.set("worktree", patch.worktree);
          if (patch.path !== undefined) {
            if (patch.path === "") next.delete("path");
            else next.set("path", patch.path);
          }
          if (patch.view !== undefined) {
            if (patch.view === null) next.delete("view");
            else next.set("view", patch.view);
          }
          if (patch.show_hidden !== undefined) {
            if (patch.show_hidden) next.set("show_hidden", "1");
            else next.delete("show_hidden");
          }
          return next;
        },
        { replace: false },
      );
    },
    [setSearchParams],
  );

  const setWorktree = useCallback(
    (wt: string) => {
      // Reset view when switching worktree, keep path
      updateParams({ worktree: wt, view: null });
    },
    [updateParams],
  );

  const setViewMode = useCallback(
    (v: ViewMode) => {
      updateParams({ view: v });
    },
    [updateParams],
  );

  const toggleHidden = useCallback(() => {
    updateParams({ show_hidden: !showHidden });
  }, [showHidden, updateParams]);

  // Bridge CodeBrowser's state patches back onto the URL params.
  const onBrowserStateChange = useCallback(
    (patch: Partial<CodeBrowserState>) => {
      updateParams({
        ...(patch.path !== undefined ? { path: patch.path } : {}),
        ...(patch.viewMode !== undefined ? { view: patch.viewMode } : {}),
        ...(patch.showHidden !== undefined ? { show_hidden: patch.showHidden } : {}),
      });
    },
    [updateParams],
  );

  // Contribute the CODE band (repo selector + breadcrumb, and the
  // show-hidden / edit-in-vscode controls) to the single shared Header.
  useHeaderSlot({
    title: (
      <>
        {/* Worktree dropdown */}
        <select
          value={worktree}
          onChange={(e) => setWorktree(e.target.value)}
          className="appearance-none rounded-md border border-mycel-border-strong bg-mycel-surface text-mycel-text text-[11px] px-2 py-1 pr-6 outline-none focus:border-mycel-accent bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%2210%22 height=%2210%22 viewBox=%220 0 16 16%22 fill=%22none%22 stroke=%22%237c7b74%22 stroke-width=%222%22%3E%3Cpath d=%22M4 6l4 4 4-4%22/%3E%3C/svg%3E')] bg-no-repeat bg-[right_6px_center]"
        >
          {worktrees.map((wt) => (
            <option key={wt.value} value={wt.value}>
              {wt.label}
            </option>
          ))}
        </select>

        {/* View mode toggle (only when worktree !== main) */}
        {worktree !== "main" && (
          <ViewModeToggle viewMode={viewMode} onChange={setViewMode} />
        )}

        {/* Breadcrumb */}
        <CodeBreadcrumbs path={path} onNavigate={(p) => updateParams({ path: p })} />
      </>
    ),
    actions: (
      <>
        {/* Download patch (diff mode, path set, worktree != main) */}
        {worktree !== "main" && viewMode === "diff" && path && (
          <DownloadPatchButton path={path} worktree={worktree} />
        )}

        {/* Show hidden toggle */}
        <HiddenToggle showHidden={showHidden} onToggle={toggleHidden} />

        {/* Edit in VS Code (only when bc-code-server dep is running) */}
        {codeServer.running && (
          <button
            type="button"
            onClick={() => {
              const next = new URLSearchParams(searchParams);
              if (vscodeMode) next.delete("mode");
              else next.set("mode", "vscode");
              setSearchParams(next);
            }}
            className={`h-8 inline-flex items-center text-[10px] uppercase tracking-wider transition-colors border px-2 rounded ${
              vscodeMode
                ? "bg-mycel-accent-subtle text-mycel-accent border-mycel-accent"
                : "text-mycel-muted border-mycel-border hover:text-mycel-text hover:border-mycel-muted"
            }`}
            title="Open the repo in code-server (VS Code in the browser)"
          >
            {vscodeMode ? "Exit VS Code" : "Edit in VS Code"}
          </button>
        )}
      </>
    ),
  });

  return (
    <div className="flex flex-col h-full" style={{ fontFamily: MONO }}>
      {/* VS Code iframe mode — replaces body when toggled on */}
      {vscodeMode && codeServer.running && (
        <div className="flex-1 min-h-0 flex flex-col">
          <div className="shrink-0 px-6 py-1.5 bg-mycel-warning-subtle border-b border-mycel-warning text-[10px] text-mycel-warning flex items-center gap-2" style={{ fontFamily: MONO }}>
            <span>⚠</span>
            <span>
              VS Code has <strong>write access</strong> to the repo.
              Changes made here are saved directly to disk.
            </span>
          </div>
          <iframe
            title="VS Code (code-server)"
            src={codeServer.endpoint}
            className="flex-1 w-full border-0"
            sandbox="allow-scripts allow-same-origin allow-forms allow-downloads allow-popups allow-modals"
          />
        </div>
      )}

      {/* Body: tree + viewer */}
      {!vscodeMode && (
        <CodeBrowser
          worktree={worktree}
          state={{ path, viewMode, showHidden }}
          onStateChange={onBrowserStateChange}
        />
      )}
    </div>
  );
}
