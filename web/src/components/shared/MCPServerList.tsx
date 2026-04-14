import { useState } from "react";
import { MONO } from "../../utils/typography";

interface MCPServerListProps {
  servers: string[];
  loading?: boolean;
  onAdd?: (name: string) => Promise<void>;
  onRemove?: (name: string) => Promise<void>;
  className?: string;
}

export function MCPServerList({
  servers,
  loading,
  onAdd,
  onRemove,
  className,
}: MCPServerListProps) {
  const [input, setInput] = useState("");
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);

  const handleAdd = () => {
    const name = input.trim();
    if (!name || !onAdd) return;
    setAdding(true);
    onAdd(name)
      .then(() => {
        setInput("");
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        setAdding(false);
      });
  };

  const handleRemove = (name: string) => {
    if (!onRemove) return;
    setRemoving(name);
    onRemove(name)
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        setRemoving(null);
      });
  };

  if (loading) {
    return (
      <p
        className="text-xs text-bc-muted/40 italic pl-1"
        style={{ fontFamily: MONO }}
      >
        Loading...
      </p>
    );
  }

  return (
    <div className={className}>
      {servers.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {servers.map((s) => (
            <span
              key={s}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-bc-border/30 bg-bc-surface/30 text-[11px] text-bc-text/80 font-medium"
              style={{ fontFamily: MONO }}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-bc-accent/60" />
              {s.replace(/^mcp__/, "")}
              {onRemove && (
                <button
                  type="button"
                  onClick={() => { handleRemove(s); }}
                  disabled={removing === s}
                  className="ml-0.5 text-bc-muted/50 hover:text-bc-error transition-colors disabled:opacity-40 leading-none"
                  title={`Remove ${s}`}
                  aria-label={`Remove MCP server ${s}`}
                >
                  ×
                </button>
              )}
            </span>
          ))}
        </div>
      ) : (
        <p className="text-xs text-bc-muted/40 italic pl-1">
          No MCP servers configured.
        </p>
      )}
      {onAdd && (
        <div className="mt-3 flex items-center gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => { setInput(e.target.value); }}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleAdd();
            }}
            placeholder="mcp-server-name"
            disabled={adding}
            className="flex-1 max-w-[240px] rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1 text-[11px] text-bc-text/90 placeholder:text-bc-muted/40 outline-none focus:border-bc-accent/50 transition-colors disabled:opacity-40"
            style={{ fontFamily: MONO }}
          />
          <button
            type="button"
            onClick={handleAdd}
            disabled={adding || !input.trim()}
            className="px-2.5 py-1 rounded border border-bc-accent/30 bg-bc-accent/10 text-[11px] text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
            style={{ fontFamily: MONO }}
          >
            {adding ? "Adding…" : "+ Add MCP"}
          </button>
        </div>
      )}
    </div>
  );
}
