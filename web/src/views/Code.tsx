/**
 * Code - Top-level VS Code-like tab.
 *
 * Layout:
 *   - Header (shared): title, worktree dropdown, view mode toggle (diff/plain)
 *   - Body: file tree (left) + file viewer (right)
 *
 * Modes:
 *   - default: Monaco read-only viewer over the workspace project root
 *              (main repo) or any agent's worktree (diff view)
 *   - code-server: iframe to the locally-running code-server instance
 *                  (only when the optional bc-code-server dep is running)
 *
 * Backend endpoints used (implemented in Phase 3a/3b):
 *   GET /api/workspaces/{ws}/code/tree?path=&worktree=
 *   GET /api/workspaces/{ws}/code/file?path=&worktree=
 *   GET /api/workspaces/{ws}/code/diff?worktree=
 *   GET /api/deps/bc-code-server/status
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { useWorkspace } from "../context/WorkspaceContext";
import { MONO } from "../utils/typography";

interface FileNode {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
}

interface WorktreeOption {
  value: string; // "main" or agent name
  label: string;
  path?: string;
}

async function fetchTree(
  wsId: string,
  path: string,
  worktree: string,
): Promise<FileNode[]> {
  const qs = new URLSearchParams({ path, worktree });
  try {
    const r = await fetch(`/api/workspaces/${encodeURIComponent(wsId)}/code/tree?${qs.toString()}`);
    if (!r.ok) return [];
    const data = (await r.json()) as unknown;
    if (!Array.isArray(data)) return [];
    return data.filter((n): n is FileNode => {
      return (
        !!n &&
        typeof n === "object" &&
        typeof (n as Record<string, unknown>).name === "string" &&
        typeof (n as Record<string, unknown>).path === "string"
      );
    });
  } catch {
    return [];
  }
}

async function fetchFileContent(
  wsId: string,
  path: string,
  worktree: string,
): Promise<string> {
  const qs = new URLSearchParams({ path, worktree });
  try {
    const r = await fetch(`/api/workspaces/${encodeURIComponent(wsId)}/code/file?${qs.toString()}`);
    if (!r.ok) return "";
    return await r.text();
  } catch {
    return "";
  }
}

export function Code() {
  const { workspace } = useWorkspace();
  const [searchParams, setSearchParams] = useSearchParams();
  const worktree = searchParams.get("worktree") ?? "main";
  const path = searchParams.get("path") ?? "";

  const [worktrees, setWorktrees] = useState<WorktreeOption[]>([
    { value: "main", label: "main repo" },
  ]);
  const [tree, setTree] = useState<FileNode[]>([]);
  const [treeLoading, setTreeLoading] = useState(false);
  const [content, setContent] = useState("");
  const [contentLoading, setContentLoading] = useState(false);
  const lastFetchRef = useRef("");

  // Populate worktrees from agent list
  useEffect(() => {
    if (!workspace) return;
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
  }, [workspace]);

  // Fetch tree when worktree or path changes
  useEffect(() => {
    if (!workspace) return;
    setTreeLoading(true);
    const parent = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
    void fetchTree(workspace.id, parent, worktree).then((t) => {
      setTree(t);
      setTreeLoading(false);
    });
  }, [workspace, worktree, path]);

  // Fetch file content when a file is selected
  useEffect(() => {
    if (!workspace || !path) {
      setContent("");
      return;
    }
    const key = `${workspace.id}|${worktree}|${path}`;
    if (lastFetchRef.current === key) return;
    lastFetchRef.current = key;
    setContentLoading(true);
    void fetchFileContent(workspace.id, path, worktree).then((text) => {
      setContent(text);
      setContentLoading(false);
    });
  }, [workspace, path, worktree]);

  const selectFile = useCallback(
    (node: FileNode) => {
      if (node.is_dir) {
        setSearchParams({ worktree, path: node.path });
      } else {
        setSearchParams({ worktree, path: node.path });
      }
    },
    [worktree, setSearchParams],
  );

  const setWorktree = useCallback(
    (wt: string) => {
      setSearchParams({ worktree: wt, path: "" });
    },
    [setSearchParams],
  );

  const breadcrumbs = useMemo(() => {
    if (!path) return [];
    const segments = path.split("/").filter(Boolean);
    return segments.map((seg, idx) => ({
      label: seg,
      path: segments.slice(0, idx + 1).join("/"),
    }));
  }, [path]);

  if (!workspace) {
    return (
      <div className="flex-1 flex items-center justify-center text-bc-muted">
        No workspace selected.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full" style={{ fontFamily: MONO }}>
      {/* Header */}
      <header className="shrink-0 border-b border-bc-border/40 px-6 h-[42px] flex items-center gap-3">
        <span className="text-[11px] font-bold text-bc-text uppercase tracking-[0.2em]">Code</span>

        {/* Worktree dropdown */}
        <select
          value={worktree}
          onChange={(e) => setWorktree(e.target.value)}
          className="rounded border border-bc-border/40 bg-bc-surface/20 text-bc-text/90 text-[11px] px-2 py-1 outline-none focus:border-bc-accent/50"
        >
          {worktrees.map((wt) => (
            <option key={wt.value} value={wt.value}>
              {wt.label}
            </option>
          ))}
        </select>

        {/* Breadcrumb */}
        <div className="flex-1 min-w-0 flex items-center gap-1 text-[11px] text-bc-muted/70 truncate">
          <button
            type="button"
            onClick={() => setSearchParams({ worktree, path: "" })}
            className="hover:text-bc-accent transition-colors shrink-0"
          >
            /
          </button>
          {breadcrumbs.map((b) => (
            <span key={b.path} className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setSearchParams({ worktree, path: b.path })}
                className="hover:text-bc-accent transition-colors truncate max-w-[140px]"
              >
                {b.label}
              </button>
              <span className="text-bc-muted/30">/</span>
            </span>
          ))}
        </div>

        {worktree !== "main" && (
          <span className="text-[9px] text-bc-accent/70 uppercase tracking-wider">
            diff vs main
          </span>
        )}
      </header>

      {/* Body: tree + viewer */}
      <div className="flex-1 min-h-0 flex">
        {/* Tree pane */}
        <aside className="w-64 shrink-0 border-r border-bc-border/40 overflow-y-auto">
          {treeLoading && (
            <div className="px-3 py-2 text-[11px] text-bc-muted/50">loading\u2026</div>
          )}
          {!treeLoading && tree.length === 0 && (
            <div className="px-3 py-2 text-[11px] text-bc-muted/50 italic">
              {worktree === "main"
                ? "No files to display. The Code file tree API (Phase 3a) is required."
                : "This agent's worktree does not exist or has no files."}
            </div>
          )}
          {tree.map((node) => (
            <button
              key={node.path}
              type="button"
              onClick={() => selectFile(node)}
              className={`w-full flex items-center gap-1.5 px-3 py-1 text-left text-[11px] hover:bg-bc-accent/[0.06] transition-colors ${
                path === node.path ? "bg-bc-accent/[0.08] text-bc-accent" : "text-bc-text/80"
              }`}
            >
              <span className="text-bc-muted/40 shrink-0">{node.is_dir ? "\u25B8" : "\u00B7"}</span>
              <span className="truncate">{node.name}</span>
            </button>
          ))}
        </aside>

        {/* Viewer pane */}
        <section className="flex-1 min-w-0 overflow-hidden">
          {!path && (
            <div className="h-full flex items-center justify-center text-[11px] text-bc-muted/50 italic">
              Select a file from the tree
            </div>
          )}
          {path && contentLoading && (
            <div className="h-full flex items-center justify-center text-[11px] text-bc-muted/50">
              loading\u2026
            </div>
          )}
          {path && !contentLoading && (
            <pre className="h-full overflow-auto p-4 text-[11px] text-bc-text/90 leading-relaxed whitespace-pre-wrap">
              {content || "(empty)"}
            </pre>
          )}
        </section>
      </div>
    </div>
  );
}
