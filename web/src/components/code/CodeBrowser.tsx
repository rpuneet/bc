/**
 * CodeBrowser - Reusable read-only code browser: file tree + Monaco
 * viewer/DiffEditor for a single worktree.
 *
 * Used in two places:
 *   - The top-level /code view (views/Code.tsx) — controlled mode. The view
 *     owns path / viewMode / showHidden in the URL and contributes the
 *     breadcrumb + toggles to the shared Header via useHeaderSlot.
 *   - The agent detail "Code" tab — embedded mode. The browser owns its
 *     state internally, renders its own compact header row (view toggle,
 *     breadcrumb, hidden toggle, link to the full /code view) and defaults
 *     to diff view since the worktree is always an agent worktree there.
 *
 * Backend endpoints used (bcd is single-tenant — the handler is
 * anchored at the boot repo root, no repo parameter needed):
 *   GET /api/code/tree?path=&worktree=&show_hidden=
 *   GET /api/code/file?path=&worktree=
 *   GET /api/code/diff?worktree=&path=
 */

import Editor, { DiffEditor } from "@monaco-editor/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useTheme } from "../../context/ThemeContext";
import { languageFromPath } from "../../utils/lang";
import { MONO } from "../../utils/typography";

export interface FileNode {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
}

export type ViewMode = "diff" | "plain";

/** Browser state — path of the selected file, view mode, hidden-entry
 *  visibility. Controlled by the /code view (URL params); internal in
 *  embedded mode. */
export interface CodeBrowserState {
  path: string;
  viewMode: ViewMode;
  showHidden: boolean;
}

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

/* ─── Shared header controls ───────────────────────────────────────────
   Rendered by views/Code.tsx inside the shared Header slot, and by the
   embedded header row below. Single source of truth for the markup. */

export function ViewModeToggle({
  viewMode,
  onChange,
}: {
  viewMode: ViewMode;
  onChange: (v: ViewMode) => void;
}) {
  return (
    <div className="flex items-center rounded border border-mycel-border overflow-hidden text-[10px]">
      <button
        type="button"
        onClick={() => onChange("diff")}
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
        onClick={() => onChange("plain")}
        className={`px-2 py-1 uppercase tracking-wider transition-colors border-l border-mycel-border ${
          viewMode === "plain"
            ? "bg-mycel-accent-subtle text-mycel-accent"
            : "text-mycel-muted hover:text-mycel-text"
        }`}
      >
        Plain
      </button>
    </div>
  );
}

export function CodeBreadcrumbs({
  path,
  onNavigate,
}: {
  path: string;
  onNavigate: (path: string) => void;
}) {
  const breadcrumbs = useMemo(() => {
    if (!path) return [];
    const segments = path.split("/").filter(Boolean);
    return segments.map((seg, idx) => ({
      label: seg,
      path: segments.slice(0, idx + 1).join("/"),
    }));
  }, [path]);

  return (
    <div className="flex-1 min-w-0 flex items-center gap-1 text-[11px] text-mycel-muted truncate">
      <button
        type="button"
        onClick={() => onNavigate("")}
        className="hover:text-mycel-accent transition-colors shrink-0"
      >
        /
      </button>
      {breadcrumbs.map((b) => (
        <span key={b.path} className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => onNavigate(b.path)}
            className="hover:text-mycel-accent transition-colors truncate max-w-[140px]"
          >
            {b.label}
          </button>
          <span className="text-mycel-muted">/</span>
        </span>
      ))}
    </div>
  );
}

export function HiddenToggle({
  showHidden,
  onToggle,
}: {
  showHidden: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={`text-[10px] uppercase tracking-wider transition-colors ${
        showHidden ? "text-mycel-accent" : "text-mycel-muted hover:text-mycel-text"
      }`}
      title="Toggle .git / .bc entries"
    >
      {showHidden ? "Hide hidden" : "Show hidden"}
    </button>
  );
}

export function DownloadPatchButton({
  path,
  worktree,
}: {
  path: string;
  worktree: string;
}) {
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

  return (
    <button
      type="button"
      onClick={downloadPatch}
      className="text-[10px] uppercase tracking-wider text-mycel-muted hover:text-mycel-accent transition-colors"
      title="Download unified diff as .patch"
    >
      Download patch
    </button>
  );
}

/* ─── CodeBrowser ────────────────────────────────────────────────────── */

interface CodeBrowserProps {
  /** "main" or an agent name (its worktree). */
  worktree: string;
  /** Controlled state (the /code view maps this to URL params). When
   *  omitted the browser owns its state internally — embedded mode
   *  defaults to diff view. */
  state?: CodeBrowserState;
  /** Required when `state` is provided. Receives partial patches. */
  onStateChange?: (patch: Partial<CodeBrowserState>) => void;
  /** Render a compact local header row (view toggle, breadcrumb, hidden
   *  toggle, full-view link) instead of relying on the shared Header. */
  embedded?: boolean;
  /** Link target for the "Full view" affordance in the embedded header. */
  fullViewHref?: string;
  /** Rendered in place of tree + viewer when the worktree root is empty
   *  or missing (embedded mode only). */
  emptyState?: ReactNode;
}

export function CodeBrowser({
  worktree,
  state: controlledState,
  onStateChange,
  embedded = false,
  fullViewHref,
  emptyState,
}: CodeBrowserProps) {
  const { mode: themeMode } = useTheme();

  const [internalState, setInternalState] = useState<CodeBrowserState>({
    path: "",
    viewMode: "diff",
    showHidden: false,
  });
  const state = controlledState ?? internalState;
  const update = useCallback(
    (patch: Partial<CodeBrowserState>) => {
      if (onStateChange) onStateChange(patch);
      else setInternalState((prev) => ({ ...prev, ...patch }));
    },
    [onStateChange],
  );
  const { path, viewMode, showHidden } = state;

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

  const selectFile = useCallback(
    (node: FileNode) => {
      if (node.is_dir) {
        toggleExpand(node.path);
      } else {
        update({ path: node.path });
      }
    },
    [toggleExpand, update],
  );

  const rootKey = `${worktree}||${showHidden ? "1" : "0"}`;
  const rootEntries = treeCache[rootKey] ?? [];
  const language = languageFromPath(path);

  const rootEmpty = !rootLoading && !rootError && rootEntries.length === 0;

  return (
    <div className="flex-1 min-h-0 flex flex-col" style={{ fontFamily: MONO }}>
      {/* Embedded header: view toggle + breadcrumb + hidden toggle + full view */}
      {embedded && (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 border-b border-mycel-border bg-mycel-surface">
          {worktree !== "main" && (
            <ViewModeToggle
              viewMode={viewMode}
              onChange={(v) => update({ viewMode: v })}
            />
          )}
          <CodeBreadcrumbs path={path} onNavigate={(p) => update({ path: p })} />
          {worktree !== "main" && viewMode === "diff" && path && (
            <DownloadPatchButton path={path} worktree={worktree} />
          )}
          <HiddenToggle
            showHidden={showHidden}
            onToggle={() => update({ showHidden: !showHidden })}
          />
          {fullViewHref && (
            <Link
              to={fullViewHref}
              className="inline-flex items-center gap-1 text-[10px] uppercase tracking-wider text-mycel-muted hover:text-mycel-accent transition-colors shrink-0"
              title="Open in the full Code view"
            >
              Full view
              <svg width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
                <path d="M3 9l6-6M5 3h4v4" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </Link>
          )}
        </div>
      )}

      {/* Empty worktree (embedded): dedicated empty state replaces the split */}
      {embedded && rootEmpty && emptyState ? (
        <div className="flex-1 min-h-0 flex items-center justify-center">
          {emptyState}
        </div>
      ) : (
        <div className="flex-1 min-h-0 flex">
          {/* Tree pane */}
          <aside className="w-64 shrink-0 border-r border-mycel-border overflow-y-auto">
            {rootLoading && <TreeSkeleton />}
            {!rootLoading && rootError && (
              <div className="px-3 py-2 text-[11px] text-mycel-error">{rootError}</div>
            )}
            {rootEmpty && (
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
        </div>
      )}
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
        const icon = node.is_dir ? (isExpanded ? "▾" : "▸") : "·";
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
