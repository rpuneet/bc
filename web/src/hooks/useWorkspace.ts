import { useEffect, useState } from "react";
import { api } from "../api/client";

export type WorkspaceInfo = {
  workspace: string;
  hasWorkspace: boolean;
  loaded: boolean;
};

/**
 * Whether the daemon has a default repo for new agents.
 * Empty is normal — mycel up boots against MycelHome alone; agents bind
 * their own repo paths. The quiet banner only hints at Create Agent.
 */
export function useWorkspace(): WorkspaceInfo {
  const [info, setInfo] = useState<WorkspaceInfo>({
    workspace: "",
    hasWorkspace: true, // optimistic until probed — avoid a flash of the empty banner
    loaded: false,
  });

  useEffect(() => {
    let cancelled = false;
    api
      .getSystemInfo()
      .then((data) => {
        if (cancelled) return;
        setInfo({
          workspace: data.workspace ?? "",
          hasWorkspace: data.has_workspace === true,
          loaded: true,
        });
      })
      .catch(() => {
        // A probe failure must not claim "no default repo" — that would lie about
        // a working daemon that simply could not answer this call.
        if (!cancelled) {
          setInfo({ workspace: "", hasWorkspace: true, loaded: true });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return info;
}
