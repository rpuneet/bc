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
import { Navigate, useLocation, useParams } from "react-router-dom";

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
    return rawList.filter((w): w is WorkspaceSummary => {
      return (
        !!w &&
        typeof w === "object" &&
        typeof (w as Record<string, unknown>).id === "string" &&
        typeof (w as Record<string, unknown>).name === "string"
      );
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
 * ActiveWorkspaceGuard - Route element that:
 *   1. Reads :wsId from URL params
 *   2. Validates it against the list
 *   3. Activates it server-side (fire-and-forget)
 *   4. Renders children via <Outlet /> (handled by caller)
 *
 * If the wsId is unknown, redirects to /w (the picker).
 */
export function ActiveWorkspaceGuard({ children }: { children: ReactNode }) {
  const { wsId } = useParams<{ wsId: string }>();
  const { workspaces, loading } = useWorkspace();
  const location = useLocation();
  const [activated, setActivated] = useState<string | null>(null);

  useEffect(() => {
    if (!wsId) return;
    if (activated === wsId) return;
    void activateWorkspace(wsId).then(() => setActivated(wsId));
  }, [wsId, activated]);

  if (loading) return null;
  if (!wsId) return <Navigate to="/w" replace />;

  const match = workspaces.find((w) => w.id === wsId || w.alias === wsId);
  if (!match) {
    return <Navigate to={`/w?from=${encodeURIComponent(location.pathname)}`} replace />;
  }

  return <>{children}</>;
}

/**
 * RedirectToActiveWorkspace - Used by legacy top-level routes like /agents.
 * Redirects to /w/<active>/<path> preserving the sub-path.
 */
export function RedirectToActiveWorkspace({ tab }: { tab: string }) {
  const { workspace, loading } = useWorkspace();
  const location = useLocation();

  if (loading) return null;
  if (!workspace) return <Navigate to="/w" replace />;

  // Preserve sub-path: /agents/foo -> /w/<id>/agents/foo
  // Use the caller's `tab` as the base path segment.
  const cleanTab = tab.startsWith("/") ? tab.slice(1) : tab;
  const suffix = location.pathname.replace(/^\/[^/]+/, "");
  const target = `/w/${workspace.id}/${cleanTab}${suffix}${location.search}${location.hash}`;
  return <Navigate to={target} replace />;
}
