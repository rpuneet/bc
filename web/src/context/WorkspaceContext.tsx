/**
 * WorkspaceContext - Provides the active workspace to all children.
 *
 * Populated by ActiveWorkspaceGuard which extracts :wsId from the URL,
 * validates it against /api/workspaces, and activates it server-side
 * (so legacy /api/* routes route to the correct workspace too).
 *
 * Consumers typically read via `useWorkspace()` or `useWorkspacePath(tab)`.
 */

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Navigate, Outlet, useLocation, useParams } from "react-router-dom";

export interface WorkspaceSummary {
  id: string;
  name: string;
  path: string;
  alias?: string;
  github_url?: string;
  active?: boolean;
}

interface WorkspaceContextValue {
  workspace: WorkspaceSummary | null;
  workspaces: WorkspaceSummary[];
  loading: boolean;
  refresh: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue>({
  workspace: null,
  workspaces: [],
  loading: true,
  refresh: () => undefined,
});

export function useWorkspace(): WorkspaceContextValue {
  return useContext(WorkspaceContext);
}

/** Build a /w/<id>/<tab> path preserving the current workspace. */
export function useWorkspacePath(tab: string): string {
  const { workspace } = useWorkspace();
  if (!workspace) return tab.startsWith("/") ? tab : `/${tab}`;
  const cleanTab = tab.startsWith("/") ? tab.slice(1) : tab;
  return `/w/${workspace.id}/${cleanTab}`;
}

async function fetchWorkspaces(): Promise<WorkspaceSummary[]> {
  try {
    const r = await fetch("/api/workspaces");
    if (!r.ok) return [];
    const data = (await r.json()) as unknown;
    // Backend may return { workspaces: [...], active: "id" } or a bare array
    let rawList: unknown[] = [];
    if (Array.isArray(data)) {
      rawList = data;
    } else if (data && typeof data === "object" && "workspaces" in data) {
      const inner = (data as Record<string, unknown>).workspaces;
      if (Array.isArray(inner)) rawList = inner;
    }
    return rawList
      .filter((w): w is WorkspaceSummary => {
        return (
          !!w &&
          typeof w === "object" &&
          typeof (w as Record<string, unknown>).id === "string" &&
          typeof (w as Record<string, unknown>).name === "string"
        );
      })
      .filter((w) => {
        // Drop transient test-temp workspaces that leak into the registry.
        const p = String(w.path || "");
        if (/\/T\/Test[A-Z][A-Za-z0-9_]+\d/.test(p)) return false;
        if (/\/tmp\/Test[A-Z][A-Za-z0-9_]+\d/.test(p)) return false;
        return true;
      });
  } catch {
    return [];
  }
}

async function activateWorkspace(id: string): Promise<void> {
  try {
    await fetch(`/api/workspaces/${encodeURIComponent(id)}/activate`, { method: "POST" });
  } catch {
    /* best-effort */
  }
}

/**
 * WorkspaceProvider - Loads all workspaces and exposes them via context.
 * Wrap around the router so every page can use `useWorkspace`.
 */
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchWorkspaces().then((ws) => {
      if (!cancelled) {
        setWorkspaces(ws);
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [version]);

  const refresh = useMemo(() => () => setVersion((v) => v + 1), []);

  const active = workspaces.find((w) => w.active) ?? null;

  const value = useMemo<WorkspaceContextValue>(
    () => ({ workspace: active, workspaces, loading, refresh }),
    [active, workspaces, loading, refresh],
  );

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}

/**
 * ActiveWorkspaceGuard - Route element used as a parent <Route element={...}>.
 * Children routes are rendered via <Outlet />.
 *   1. Reads :wsId from URL params
 *   2. Validates it against the workspace list
 *   3. Activates it server-side (fire-and-forget, single trigger per wsId)
 *
 * Unknown wsId redirects to /w (the picker) with a ?from= hint.
 */
export function ActiveWorkspaceGuard() {
  const { wsId } = useParams<{ wsId: string }>();
  const { workspaces, loading, refresh } = useWorkspace();
  const location = useLocation();
  const [activated, setActivated] = useState<string | null>(null);

  useEffect(() => {
    if (!wsId || activated === wsId) return;
    void activateWorkspace(wsId).then(() => {
      setActivated(wsId);
      // Tell WorkspaceContext to re-fetch so header + dropdown reflect the
      // new active workspace immediately (otherwise they show stale data
      // until the next full-page reload).
      refresh();
    });
  }, [wsId, activated, refresh]);

  if (loading) return <div className="p-6 text-mycel-muted">Loading workspace...</div>;
  if (!wsId) return <Navigate to="/w" replace />;

  const match = workspaces.find((w) => w.id === wsId || w.alias === wsId);
  if (!match) {
    return <Navigate to={`/w?from=${encodeURIComponent(location.pathname)}`} replace />;
  }

  return <Outlet />;
}

/**
 * RedirectToActiveWorkspace - Used by legacy top-level routes like /agents.
 * Redirects to /w/<active>/<path> preserving the sub-path.
 */
export function RedirectToActiveWorkspace({ tab }: { tab: string }) {
  const { workspace, loading } = useWorkspace();
  const location = useLocation();

  if (loading) return <div className="p-6 text-mycel-muted">Loading...</div>;
  if (!workspace) return <Navigate to="/w" replace />;

  // Preserve sub-path: /agents/foo -> /w/<id>/agents/foo
  const cleanTab = tab.startsWith("/") ? tab.slice(1) : tab;
  const suffix = location.pathname.replace(/^\/[^/]+/, "");
  const target = `/w/${workspace.id}/${cleanTab}${suffix}${location.search}${location.hash}`;
  return <Navigate to={target} replace />;
}
