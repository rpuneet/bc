/**
 * Code - Top-level VS Code-like tab.
 *
 * Layout:
 *   - Header (shared): title, worktree dropdown, view mode toggle (diff/plain)
 *   - Body: file tree (left) + Monaco viewer (right)
 *
 * Modes:
 *   - view : Monaco read-only viewer (default for main repo)
 *   - diff : Monaco DiffEditor, base = main repo, modified = agent worktree
 *           (default when a worktree other than main is selected)
 *
 * Backend endpoints used (bcd is single-tenant — the handler is
 * anchored at the boot repo root, no repo parameter needed):
 *   GET /api/code/tree?path=&worktree=&show_hidden=
 *   GET /api/code/file?path=&worktree=
 *   GET /api/code/diff?worktree=&path=
 */

import Editor, { DiffEditor } from "@monaco-editor/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { useTheme } from "../context/ThemeContext";
import { languageFromPath } from "../utils/lang";
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

type ViewMode = "diff" | "plain";

interface FileResult {
  content: string;
  binary: boolean;
  ok: boolean;
  notFound: boolean;
}

const EMPTY_FILE: FileResult = {
  content: "",
  binary: false,
  ok: false,
  notFound: false,
};

// Common build / dependency directories that clutter the file tree.
// When `Show hidden` is off, these are filtered out alongside dotfiles.
// Users can still inspect them by toggling `Show hidden`.
const HIDDEN_DIRS = new Set([
  ".git",
  ".bc",
  "node_modules",
  "dist",
  "build",
  ".next",
  ".turbo",
  ".cache",
  ".parcel-cache",
  "__pycache__",
  ".venv",
  "venv",
  "target",
  "vendor",
]);

function isHiddenEntry(name: string): boolean {
  return HIDDEN_DIRS.has(name);
}

async function fetchTree(
  path: string,
  worktree: string,
  showHidden: boolean,
): Promise<FileNode[]> {
  const qs = new URLSearchParams({ path, worktree });
  if (showHidden) qs.set("show_hidden", "1");
  try {
    const r = await fetch(`/api/code/tree?${qs.toString()}`);
    if (!r.ok) return [];
    const data = (await r.json()) as unknown;
    if (!Array.isArray(data)) return [];
    return data.filter((n): n is FileNode => {
      return (
        !!n &&
        typeof n === "object" &&
        typeof (n as Record<string, unknown>).name === "string" &&
        typeof (n as Record<string, unknown>).path === "string" &&
        typeof (n as Record<string, unknown>).is_dir === "boolean"
      );
    });
  } catch {
    return [];
  }
}

async function fetchFile(
  path: string,
  worktree: string,
): Promise<FileResult> {
  const qs = new URLSearchParams({ path, worktree });
  try {
    const r = await fetch(`/api/code/file?${qs.toString()}`);
    if (r.status === 404) {
      return { content: "", binary: false, ok: true, notFound: true };
    }
    if (!r.ok) return EMPTY_FILE;
    const binary = r.headers.get("X-BC-Binary") === "true";
    if (binary) {
      return { content: "", binary: true, ok: true, notFound: false };
    }
    const text = await r.text();
    return { content: text, binary: false, ok: true, notFound: false };
  } catch {
    return EMPTY_FILE;
  }
}

function fileDownloadUrl(path: string, worktree: string): string {
  const qs = new URLSearchParams({ path, worktree });
  return `/api/code/file?${qs.toString()}`;
}

function patchDownloadUrl(path: string, worktree: string): string {
  const qs = new URLSearchParams({ worktree, path });
  return `/api/code/diff?${qs.toString()}`;
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
  const { mode: themeMode } = useTheme();
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
  const [treeCache, setTreeCache] = useState<Record<string, FileNode[]>>({});
  const [treeLoading, setTreeLoading] = useState<Record<string, boolean>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [rootLoading, setRootLoading] = useState(false);
  const [rootError, setRootError] = useState<string | null>(null);

  const [fileContent, setFileContent] = useState<FileResult>(EMPTY_FILE);
  const [baseContent, setBaseContent] = useState<FileResult>(EMPTY_FILE);
  const [contentLoading, setContentLoading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);
  const lastFetchRef = useRef("");

  const isDark = themeMode === "dark";
  const monacoTheme = isDark ? "vs-dark" : "vs-light";

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

  // -------- tree fetching --------
  const loadDir = useCallback(
    async (dirPath: string) => {
      const key = `${worktree}|${dirPath}|${showHidden ? "1" : "0"}`;
      setTreeLoading((prev) => ({ ...prev, [key]: true }));
      const entries = await fetchTree(dirPath, worktree, showHidden);
      const visible = showHidden ? entries : entries.filter((n) => !isHiddenEntry(n.name));
      setTreeCache((prev) => ({ ...prev, [key]: visible }));
      setTreeLoading((prev) => ({ ...prev, [key]: false }));
    },
    [worktree, showHidden],
  );

  // Load root whenever worktree/showHidden changes
  useEffect(() => {
    let cancelled = false;
    setRootLoading(true);
    setRootError(null);
    const key = `${worktree}||${showHidden ? "1" : "0"}`;
    void fetchTree("", worktree, showHidden).then((entries) => {
      if (cancelled) return;
      const visible = showHidden ? entries : entries.filter((n) => !isHiddenEntry(n.name));
      setTreeCache((prev) => ({ ...prev, [key]: visible }));
      setRootLoading(false);
      if (visible.length === 0) setRootError(null);
    });
    return () => {
      cancelled = true;
    };
  }, [worktree, showHidden]);

  // Reset expanded when worktree changes
  useEffect(() => {
    setExpanded(new Set());
  }, [worktree]);

  const toggleExpand = useCallback(
    (dirPath: string) => {
      setExpanded((prev) => {
        const next = new Set(prev);
        if (next.has(dirPath)) {
          next.delete(dirPath);
        } else {
          next.add(dirPath);
          void loadDir(dirPath);
        }
        return next;
      });
    },
    [loadDir],
  );

  // -------- file content fetching --------
  useEffect(() => {
    if (!path) {
      setFileContent(EMPTY_FILE);
      setBaseContent(EMPTY_FILE);
      return;
    }
    const key = `${worktree}|${path}|${viewMode}`;
    if (lastFetchRef.current === key) return;
    lastFetchRef.current = key;
    let cancelled = false;
    setContentLoading(true);
    setFileError(null);

    const headPromise = fetchFile(path, worktree);
    const basePromise =
      viewMode === "diff" && worktree !== "main"
        ? fetchFile(path, "main")
        : Promise.resolve(EMPTY_FILE);

    void Promise.all([headPromise, basePromise]).then(([head, base]) => {
      if (cancelled) return;
      setFileContent(head);
      setBaseContent(base);
      setContentLoading(false);
      if (!head.ok) {
        setFileError("Failed to load file");
      }
    });

    return () => {
      cancelled = true;
    };
  }, [path, worktree, viewMode]);

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

  const selectFile = useCallback(
    (node: FileNode) => {
      if (node.is_dir) {
        toggleExpand(node.path);
      } else {
        updateParams({ path: node.path });
      }
    },
    [toggleExpand, updateParams],
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

  const downloadPatch = useCallback(() => {
    if (!path) return;
    const url = patchDownloadUrl(path, worktree);
    const link = document.createElement("a");
    link.href = url;
    const filename = path.replace(/[\\/]/g, "_");
    link.download = `${filename}.patch`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }, [path, worktree]);

  // -------- breadcrumb --------
  const breadcrumbs = useMemo(() => {
    if (!path) return [];
    const segments = path.split("/").filter(Boolean);
    return segments.map((seg, idx) => ({
      label: seg,
      path: segments.slice(0, idx + 1).join("/"),
    }));
  }, [path]);

  const rootKey = `${worktree}||${showHidden ? "1" : "0"}`;
  const rootEntries = treeCache[rootKey] ?? [];
  const language = languageFromPath(path);

  return (
    <div className="flex flex-col h-full" style={{ fontFamily: MONO }}>
      {/* Header */}
      <header className="shrink-0 border-b border-mycel-border px-6 h-[42px] flex items-center gap-3">
        <span className="text-[11px] font-bold text-mycel-text uppercase tracking-[0.2em]">
          Code
        </span>

        {/* Worktree dropdown */}
        <select
          value={worktree}
          onChange={(e) => setWorktree(e.target.value)}
          className="rounded border border-mycel-border-strong bg-mycel-surface text-mycel-text text-[11px] px-2 py-1 outline-none focus:border-mycel-accent"
        >
          {worktrees.map((wt) => (
            <option key={wt.value} value={wt.value}>
              {wt.label}
            </option>
          ))}
        </select>

        {/* View mode toggle (only when worktree !== main) */}
        {worktree !== "main" && (
          <div className="flex items-center rounded border border-mycel-border overflow-hidden text-[10px]">
            <button
              type="button"
              onClick={() => setViewMode("diff")}
              className={`px-2 py-1 uppercase tracking-wider transition-colors ${
                viewMode === "diff"
                  ? "bg-mycel-accent-subtle text-mycel-accent"
                  : "text-mycel-muted hover:text-mycel-text"
              }`}
            >
              Diff
            </button>
            <button
              type="button"
              onClick={() => setViewMode("plain")}
              className={`px-2 py-1 uppercase tracking-wider transition-colors border-l border-mycel-border ${
                viewMode === "plain"
                  ? "bg-mycel-accent-subtle text-mycel-accent"
                  : "text-mycel-muted hover:text-mycel-text"
              }`}
            >
              Plain
            </button>
          </div>
        )}

        {/* Breadcrumb */}
        <div className="flex-1 min-w-0 flex items-center gap-1 text-[11px] text-mycel-muted truncate">
          <button
            type="button"
            onClick={() => updateParams({ path: "" })}
            className="hover:text-mycel-accent transition-colors shrink-0"
          >
            /
          </button>
          {breadcrumbs.map((b) => (
            <span key={b.path} className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => updateParams({ path: b.path })}
                className="hover:text-mycel-accent transition-colors truncate max-w-[140px]"
              >
                {b.label}
              </button>
              <span className="text-mycel-muted">/</span>
            </span>
          ))}
        </div>

        {/* Download patch (diff mode, path set, worktree != main) */}
        {worktree !== "main" && viewMode === "diff" && path && (
          <button
            type="button"
            onClick={downloadPatch}
            className="text-[10px] uppercase tracking-wider text-mycel-muted hover:text-mycel-accent transition-colors"
            title="Download unified diff as .patch"
          >
            Download patch
          </button>
        )}

        {/* Show hidden toggle */}
        <button
          type="button"
          onClick={toggleHidden}
          className={`text-[10px] uppercase tracking-wider transition-colors ${
            showHidden ? "text-mycel-accent" : "text-mycel-muted hover:text-mycel-text"
          }`}
          title="Toggle .git / .bc entries"
        >
          {showHidden ? "Hide hidden" : "Show hidden"}
        </button>

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
            className={`text-[10px] uppercase tracking-wider transition-colors border px-2 py-1 rounded ${
              vscodeMode
                ? "bg-mycel-accent-subtle text-mycel-accent border-mycel-accent"
                : "text-mycel-muted border-mycel-border hover:text-mycel-text hover:border-mycel-muted"
            }`}
            title="Open the repo in code-server (VS Code in the browser)"
          >
            {vscodeMode ? "Exit VS Code" : "Edit in VS Code"}
          </button>
        )}
      </header>

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
      {!vscodeMode && <div className="flex-1 min-h-0 flex">
        {/* Tree pane */}
        <aside className="w-64 shrink-0 border-r border-mycel-border overflow-y-auto">
          {rootLoading && <TreeSkeleton />}
          {!rootLoading && rootError && (
            <div className="px-3 py-2 text-[11px] text-mycel-error">{rootError}</div>
          )}
          {!rootLoading && !rootError && rootEntries.length === 0 && (
            <div className="px-3 py-2 text-[11px] text-mycel-muted italic">
              {worktree === "main"
                ? "No files to display."
                : "This agent's worktree does not exist or has no files."}
            </div>
          )}
          {!rootLoading && rootEntries.length > 0 && (
            <TreeList
              entries={rootEntries}
              depth={0}
              selectedPath={path}
              expanded={expanded}
              treeCache={treeCache}
              treeLoading={treeLoading}
              worktree={worktree}
              showHidden={showHidden}
              onSelect={selectFile}
            />
          )}
        </aside>

        {/* Viewer pane */}
        <section className="flex-1 min-w-0 overflow-hidden relative">
          {!path && (
            <div className="h-full flex items-center justify-center text-[11px] text-mycel-muted italic">
              Select a file from the tree
            </div>
          )}
          {path && contentLoading && <EditorShimmer />}
          {path && !contentLoading && fileError && (
            <div className="h-full flex items-center justify-center text-[11px] text-mycel-error">
              {fileError}
            </div>
          )}
          {path && !contentLoading && !fileError && fileContent.binary && (
            <div className="h-full flex flex-col items-center justify-center gap-2 text-[11px] text-mycel-muted">
              <div>Binary file</div>
              <a
                href={fileDownloadUrl(path, worktree)}
                download
                className="text-mycel-accent hover:underline"
              >
                Download
              </a>
            </div>
          )}
          {path &&
            !contentLoading &&
            !fileError &&
            !fileContent.binary &&
            worktree !== "main" &&
            viewMode === "diff" && (
              <DiffEditor
                theme={monacoTheme}
                language={language}
                original={baseContent.content}
                modified={fileContent.content}
                options={{
                  readOnly: true,
                  renderSideBySide: true,
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                  fontSize: 12,
                  fontFamily: MONO,
                  automaticLayout: true,
                }}
              />
            )}
          {path &&
            !contentLoading &&
            !fileError &&
            !fileContent.binary &&
            (worktree === "main" || viewMode === "plain") && (
              <Editor
                theme={monacoTheme}
                language={language}
                value={fileContent.content}
                options={{
                  readOnly: true,
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                  fontSize: 12,
                  fontFamily: MONO,
                  automaticLayout: true,
                  wordWrap: "off",
                }}
              />
            )}
        </section>
      </div>}
    </div>
  );
}

// --------- Subcomponents ---------

interface TreeListProps {
  entries: FileNode[];
  depth: number;
  selectedPath: string;
  expanded: Set<string>;
  treeCache: Record<string, FileNode[]>;
  treeLoading: Record<string, boolean>;
  worktree: string;
  showHidden: boolean;
  onSelect: (node: FileNode) => void;
}

function TreeList({
  entries,
  depth,
  selectedPath,
  expanded,
  treeCache,
  treeLoading,
  worktree,
  showHidden,
  onSelect,
}: TreeListProps) {
  return (
    <ul className="py-1">
      {entries.map((node) => {
        const isExpanded = node.is_dir && expanded.has(node.path);
        const childKey = `${worktree}|${node.path}|${showHidden ? "1" : "0"}`;
        const childEntries = treeCache[childKey];
        const childLoading = treeLoading[childKey];
        const icon = node.is_dir ? (isExpanded ? "\u25BE" : "\u25B8") : "\u00B7";
        return (
          <li key={node.path}>
            <button
              type="button"
              onClick={() => onSelect(node)}
              style={{ paddingLeft: 12 + depth * 12 }}
              className={`w-full flex items-center gap-1.5 pr-3 py-1 text-left text-[11px] hover:bg-mycel-surface-hover transition-colors ${
                selectedPath === node.path
                  ? "bg-mycel-accent-subtle text-mycel-accent"
                  : "text-mycel-text-2"
              }`}
            >
              <span className="text-mycel-muted shrink-0 w-3 text-center">{icon}</span>
              <span className="truncate">{node.name}</span>
            </button>
            {isExpanded && (
              <>
                {childLoading && !childEntries && (
                  <div
                    className="text-[10px] text-mycel-muted italic py-0.5"
                    style={{ paddingLeft: 12 + (depth + 1) * 12 }}
                  >
                    loading…
                  </div>
                )}
                {childEntries && childEntries.length > 0 && (
                  <TreeList
                    entries={childEntries}
                    depth={depth + 1}
                    selectedPath={selectedPath}
                    expanded={expanded}
                    treeCache={treeCache}
                    treeLoading={treeLoading}
                    worktree={worktree}
                    showHidden={showHidden}
                    onSelect={onSelect}
                  />
                )}
                {childEntries && childEntries.length === 0 && !childLoading && (
                  <div
                    className="text-[10px] text-mycel-muted italic py-0.5"
                    style={{ paddingLeft: 12 + (depth + 1) * 12 }}
                  >
                    (empty)
                  </div>
                )}
              </>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function TreeSkeleton() {
  return (
    <div className="py-2 px-3 space-y-1.5">
      {Array.from({ length: 8 }).map((_, i) => (
        <div
          key={i}
          className="h-3 animate-pulse rounded bg-mycel-surface-hover"
          style={{ width: `${50 + ((i * 13) % 40)}%` }}
        />
      ))}
    </div>
  );
}

function EditorShimmer() {
  return (
    <div className="absolute inset-0 p-4 space-y-2 pointer-events-none">
      {Array.from({ length: 14 }).map((_, i) => (
        <div
          key={i}
          className="h-3 animate-pulse rounded bg-mycel-surface-hover"
          style={{ width: `${40 + ((i * 17) % 55)}%` }}
        />
      ))}
    </div>
  );
}
