/**
 * WorkspaceDropdown.tsx - Workspace switcher shown in the Header left slot.
 *
 * - Lists registered workspaces from GET /api/workspaces
 * - Shows the active workspace name + short id
 * - Clicking an entry navigates to /w/<newId>/<currentTabPath>
 * - Footer has "Add workspace" that opens AddWorkspaceModal
 * - Supports Cmd/Ctrl+K to open, Escape to close
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { MONO } from "../utils/typography";

interface WorkspaceEntry {
  id: string;
  name: string;
  path: string;
  alias?: string;
  active?: boolean;
  github_url?: string;
}

async function fetchWorkspaces(): Promise<WorkspaceEntry[]> {
  try {
    const r = await fetch("/api/workspaces");
    if (!r.ok) return [];
    const data = (await r.json()) as unknown;
    if (!Array.isArray(data)) return [];
    return data.filter((w): w is WorkspaceEntry => {
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

export function WorkspaceDropdown({
  onAddClick,
}: {
  onAddClick?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [workspaces, setWorkspaces] = useState<WorkspaceEntry[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const location = useLocation();
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const refresh = useCallback(() => {
    setLoading(true);
    void fetchWorkspaces().then((ws) => {
      setWorkspaces(ws);
      setLoading(false);
    });
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

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
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
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
    const match = /^\/w\/[^/]+(\/.*)?$/.exec(location.pathname);
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
            ? "border-bc-accent/60 bg-bc-accent/[0.06] text-bc-text"
            : "border-bc-border/40 bg-bc-surface/20 text-bc-text/80 hover:border-bc-border/70"
        }`}
        style={{ fontFamily: MONO }}
        title="Switch workspace (Cmd+K)"
      >
        <span className="text-bc-muted/60 text-[9px] uppercase tracking-wider">ws</span>
        <span className="font-semibold truncate max-w-[160px]">
          {active?.name ?? (loading ? "\u2026" : "no workspace")}
        </span>
        {active?.id && (
          <span className="text-bc-muted/40 text-[9px] tabular-nums">
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
          className="absolute left-0 mt-1.5 w-80 rounded-md border border-bc-border/60 bg-bc-surface shadow-xl z-50 overflow-hidden"
          role="menu"
        >
          <div className="px-2.5 py-2 border-b border-bc-border/40">
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search workspaces\u2026"
              className="w-full bg-bc-bg border border-bc-border/40 rounded px-2.5 py-1 text-[11px] text-bc-text/90 placeholder:text-bc-muted/40 outline-none focus:border-bc-accent/50"
              style={{ fontFamily: MONO }}
            />
          </div>

          <div className="max-h-[320px] overflow-y-auto py-1">
            {loading && (
              <div className="px-3 py-2 text-[11px] text-bc-muted/50" style={{ fontFamily: MONO }}>
                loading\u2026
              </div>
            )}
            {!loading && filtered.length === 0 && (
              <div className="px-3 py-2 text-[11px] text-bc-muted/50 italic" style={{ fontFamily: MONO }}>
                {workspaces.length === 0 ? "no workspaces registered" : "no matches"}
              </div>
            )}
            {filtered.map((ws) => (
              <button
                key={ws.id}
                type="button"
                onClick={() => handleSelect(ws)}
                className={`w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-bc-accent/[0.06] transition-colors ${
                  ws.active ? "bg-bc-accent/[0.04]" : ""
                }`}
                style={{ fontFamily: MONO }}
              >
                <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${ws.active ? "bg-bc-accent" : "bg-bc-muted/30"}`} />
                <span className="text-[12px] font-semibold text-bc-text/90 truncate">
                  {ws.name}
                </span>
                <span className="text-[9px] text-bc-muted/40 tabular-nums">
                  [{ws.id.slice(0, 6)}]
                </span>
                <span className="ml-auto text-[10px] text-bc-muted/40 truncate max-w-[140px]" title={ws.path}>
                  {ws.path.replace(/^.*\//, "")}
                </span>
              </button>
            ))}
          </div>

          <div className="border-t border-bc-border/40 px-2.5 py-1.5">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                onAddClick?.();
              }}
              className="w-full flex items-center gap-2 px-2.5 py-1.5 text-[11px] text-bc-accent hover:bg-bc-accent/[0.08] rounded transition-colors"
              style={{ fontFamily: MONO }}
            >
              <span>+</span>
              <span>Add workspace\u2026</span>
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
      className="shrink-0 w-7 h-7 flex items-center justify-center rounded text-bc-muted/60 hover:text-bc-text hover:bg-bc-surface/40 transition-colors"
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
