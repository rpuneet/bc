/**
 * WorkspaceDropdown.tsx - Workspace switcher shown in the Header left slot.
 *
 * - Lists registered workspaces from GET /api/workspaces
 * - Shows the active workspace name + short id
 * - Clicking an entry navigates to /w/<newId>/<currentTabPath>
 * - Footer has "Add workspace" that opens AddWorkspaceModal
 * - Supports Cmd/Ctrl+K to open, Escape to close
 */

import { useEffect, useRef, useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { MONO } from "../utils/typography";
import { useWorkspace, type WorkspaceSummary } from "../context/WorkspaceContext";

type WorkspaceEntry = WorkspaceSummary;

/** Render a shortened path: strips $HOME prefix and keeps the last 2 segments. */
function shortenPath(full: string): string {
  let p = full;
  p = p.replace(/^\/Users\/[^/]+/, "~");
  p = p.replace(/^\/home\/[^/]+/, "~");
  const parts = p.split("/").filter(Boolean);
  if (parts.length <= 2) return p;
  return (p.startsWith("~") ? "~/…/" : "…/") + parts.slice(-2).join("/");
}

export function WorkspaceDropdown({
  onAddClick,
}: {
  onAddClick?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const navigate = useNavigate();
  const location = useLocation();
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const { workspaces, loading } = useWorkspace();

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [open]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === "w") {
        e.preventDefault();
        setOpen((v) => !v);
      }
      if (e.key === "Escape" && open) setOpen(false);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open]);

  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      setQuery("");
    }
  }, [open]);

  const active = workspaces.find((w) => w.active) ?? workspaces[0];
  const lowerQuery = query.toLowerCase();
  const filtered = workspaces.filter(
    (w) =>
      !lowerQuery ||
      w.name.toLowerCase().includes(lowerQuery) ||
      (w.alias ?? "").toLowerCase().includes(lowerQuery) ||
      w.path.toLowerCase().includes(lowerQuery),
  );

  const handleSelect = (ws: WorkspaceEntry) => {
    setOpen(false);
    // Phase M5: bcd dispatches to any registered workspace per-request.
    // No restart needed — the scoped URL /w/<id>/... routes directly to
    // that workspace's services via the WorkspaceManager.
    const match = /\/w\/[^/]+(\/.*)?$/.exec(location.pathname);
    const rest = match?.[1] ?? "/agents";
    navigate(`/w/${ws.id}${rest}`);
  };

  return (
    <div ref={containerRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={`flex items-center gap-1.5 px-2.5 py-1 rounded border text-[11px] transition-colors ${
          open
            ? "border-mycel-accent/60 bg-mycel-accent/[0.06] text-mycel-text"
            : "border-mycel-border/40 bg-mycel-surface/20 text-mycel-text/80 hover:border-mycel-border/70"
        }`}
        style={{ fontFamily: MONO }}
        title="Switch workspace (Cmd+Shift+W)"
      >
        <span className="font-semibold truncate max-w-[180px]">
          {active ? (active.name || active.path.split("/").pop() || "unnamed") : (loading ? "…" : "no workspace")}
        </span>
        {active?.id && (
          <span className="text-mycel-muted text-[9px] tabular-nums">
            [{active.id.slice(0, 6)}]
          </span>
        )}
        <svg
          width="9"
          height="9"
          viewBox="0 0 9 9"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.4"
          className={`transition-transform ${open ? "rotate-180" : ""}`}
        >
          <path d="M2 3.5l2.5 2.5L7 3.5" strokeLinecap="round" />
        </svg>
      </button>

      {open && (
        <div
          className="absolute left-0 mt-1.5 w-80 rounded-md border border-mycel-border/60 bg-mycel-surface shadow-xl z-50 overflow-hidden"
          role="menu"
        >
          <div className="px-2.5 py-2 border-b border-mycel-border/40">
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search workspaces…"
              className="w-full bg-mycel-bg border border-mycel-border/40 rounded px-2.5 py-1 text-[11px] text-mycel-text/90 placeholder:text-mycel-muted outline-none focus:border-mycel-accent/50"
              style={{ fontFamily: MONO }}
            />
          </div>

          <div className="max-h-[320px] overflow-y-auto py-1">
            {loading && (
              <div className="px-3 py-2 text-[11px] text-mycel-muted" style={{ fontFamily: MONO }}>
                loading…
              </div>
            )}
            {!loading && filtered.length === 0 && (
              <div className="px-3 py-2 text-[11px] text-mycel-muted italic" style={{ fontFamily: MONO }}>
                {workspaces.length === 0 ? "no workspaces registered" : "no matches"}
              </div>
            )}
            {filtered.map((ws) => (
              <button
                key={ws.id}
                type="button"
                onClick={() => handleSelect(ws)}
                className={`w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-mycel-accent/[0.06] transition-colors ${
                  ws.active ? "bg-mycel-accent/[0.04]" : ""
                }`}
                style={{ fontFamily: MONO }}
              >
                <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${ws.active ? "bg-mycel-accent" : "bg-mycel-muted/30"}`} />
                <span className="text-[12px] font-semibold text-mycel-text/90 truncate">
                  {ws.name || ws.path.split("/").pop() || "unnamed"}
                </span>
                <span className="text-[9px] text-mycel-muted tabular-nums">
                  [{ws.id.slice(0, 6)}]
                </span>
                <span className="ml-auto text-[10px] text-mycel-muted truncate max-w-[160px]" title={ws.path}>
                  {shortenPath(ws.path)}
                </span>
              </button>
            ))}
          </div>

          <div className="border-t border-mycel-border/40 px-2.5 py-1.5">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onAddClick?.();
              }}
              className="w-full flex items-center gap-2 px-2.5 py-1.5 text-[11px] text-mycel-accent hover:bg-mycel-accent/[0.08] rounded transition-colors"
              style={{ fontFamily: MONO }}
            >
              <span>+</span>
              <span>Add workspace…</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export function SidebarToggle({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="shrink-0 w-7 h-7 flex items-center justify-center rounded text-mycel-muted/60 hover:text-mycel-text hover:bg-mycel-surface/40 transition-colors"
      title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
    >
      <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
        {collapsed ? (
          <>
            <path d="M3 7h8" />
            <path d="M7 3l4 4-4 4" />
          </>
        ) : (
          <>
            <path d="M11 7H3" />
            <path d="M7 3L3 7l4 4" />
          </>
        )}
      </svg>
    </button>
  );
}
