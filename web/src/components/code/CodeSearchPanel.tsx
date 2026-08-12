/**
 * Modest Code-tab search panel. Calls GET /api/code/search and opens
 * matches in the surrounding CodeBrowser.
 */
import { useEffect, useState } from "react";
import { ListSearchInput } from "../shared";
import { MONO } from "../../utils/typography";

export interface CodeSearchMatch {
  path: string;
  text: string;
  line: number;
  col: number;
}

interface SearchResponse {
  matches?: CodeSearchMatch[];
  truncated?: boolean;
  elapsed_ms?: number;
}

async function fetchSearch(
  q: string,
  worktree: string,
): Promise<SearchResponse> {
  const qs = new URLSearchParams({ q, worktree, case: "1" });
  const r = await fetch(`/api/code/search?${qs.toString()}`);
  if (!r.ok) {
    let msg = `search failed (${String(r.status)})`;
    try {
      const body = (await r.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* keep status fallback */
    }
    throw new Error(msg);
  }
  return (await r.json()) as SearchResponse;
}

export function CodeSearchPanel({
  worktree,
  onOpen,
}: {
  worktree: string;
  onOpen: (path: string, line: number) => void;
}) {
  const [q, setQ] = useState("");
  const [matches, setMatches] = useState<CodeSearchMatch[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [truncated, setTruncated] = useState(false);

  useEffect(() => {
    const query = q.trim();
    if (!query) {
      setMatches([]);
      setError(null);
      setTruncated(false);
      setLoading(false);
      return;
    }
    let cancelled = false;
    const t = setTimeout(() => {
      setLoading(true);
      void fetchSearch(query, worktree)
        .then((out) => {
          if (cancelled) return;
          setMatches(out.matches ?? []);
          setTruncated(Boolean(out.truncated));
          setError(null);
        })
        .catch((err: unknown) => {
          if (cancelled) return;
          setMatches([]);
          setTruncated(false);
          setError(err instanceof Error ? err.message : "search failed");
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 280);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [q, worktree]);

  const searching = q.trim().length > 0;

  return (
    <div className="border-b border-mycel-border" style={{ fontFamily: MONO }}>
      <div className="p-2">
        <ListSearchInput
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search code…"
          aria-label="Search code"
          className="w-full max-w-none min-w-0"
          data-testid="code-search-input"
        />
      </div>
      {searching && (
        <div
          className="max-h-56 overflow-y-auto border-t border-mycel-border"
          data-testid="code-search-results"
        >
          {loading && (
            <div className="px-3 py-2 text-[11px] text-mycel-muted">Searching…</div>
          )}
          {!loading && error && (
            <div className="px-3 py-2 text-[11px] text-mycel-error">{error}</div>
          )}
          {!loading && !error && matches.length === 0 && (
            <div className="px-3 py-2 text-[11px] text-mycel-muted italic">No matches</div>
          )}
          {!loading && !error && matches.map((m, i) => (
            <button
              key={`${m.path}:${String(m.line)}:${String(i)}`}
              type="button"
              onClick={() => onOpen(m.path, m.line)}
              className="w-full text-left px-3 py-1.5 hover:bg-mycel-surface-hover border-b border-mycel-border/60 last:border-b-0"
            >
              <div className="text-[11px] text-mycel-accent truncate">
                {m.path}
                <span className="text-mycel-muted">:{String(m.line)}</span>
              </div>
              <div className="text-[11px] text-mycel-text-2 truncate">{m.text}</div>
            </button>
          ))}
          {!loading && truncated && (
            <div className="px-3 py-1.5 text-[10px] text-mycel-muted">Results truncated</div>
          )}
        </div>
      )}
    </div>
  );
}
