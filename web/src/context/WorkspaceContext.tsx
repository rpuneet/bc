/**
 * WorkspaceContext - Exposes the list of known workspaces and the currently
 * active one to any consumer that needs it (dropdown, agent filter, etc.).
 *
 * Workspace is a **property on the agent**, not a URL segment. Every route
 * in the app is flat (`/agents`, `/notifications`, `/metrics`, ...). Global
 * pages (costs, tools, secrets, gateways, settings) don't care about the
 * active workspace at all.
 *
 * Switching workspace is a server-side activation call — the URL never
 * changes and no `/w/<id>/` prefix exists anywhere.
 */

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

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
  activate: (id: string) => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceContextValue>({
  workspace: null,
  workspaces: [],
  loading: true,
  refresh: () => undefined,
  activate: async () => undefined,
});

export function useWorkspace(): WorkspaceContextValue {
  return useContext(WorkspaceContext);
}

async function fetchWorkspaces(): Promise<WorkspaceSummary[]> {
  try {
    const r = await fetch("/api/workspaces");
    if (!r.ok) return [];
    const data = (await r.json()) as unknown;
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
  await fetch(`/api/workspaces/${encodeURIComponent(id)}/activate`, { method: "POST" });
}

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
  const activate = useMemo(
    () => async (id: string) => {
      await activateWorkspace(id);
      setVersion((v) => v + 1);
    },
    [],
  );

  const active = workspaces.find((w) => w.active) ?? null;

  const value = useMemo<WorkspaceContextValue>(
    () => ({ workspace: active, workspaces, loading, refresh, activate }),
    [active, workspaces, loading, refresh, activate],
  );

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}
