import { useEffect, useRef, useState } from "react";
import type { MCPServer } from "../../api/client";
import { MONO } from "../../utils/typography";

interface MCPServerListProps {
  servers: string[];
  loading?: boolean;
  onAdd?: (name: string) => Promise<void>;
  onRemove?: (name: string) => Promise<void>;
  className?: string;
  /** Real MCP server definitions from the global registry (`GET /api/mcp`).
   *  Drives the autocomplete — only names with an actual command/url can
   *  ever connect, so the picker should never suggest anything else. */
  registry?: MCPServer[];
  /** Surfaces the most recent add failure (e.g. an unknown server name
   *  the backend rejected rather than writing an empty stanza). */
  addError?: string | null;
}

/** One-line summary of how a registry entry connects, shown as the
 *  suggestion subtitle in place of a hardcoded description. */
function summarize(m: MCPServer): string {
  if (m.command) return `stdio · ${m.command}`;
  if (m.url) return `${m.transport || "sse"} · ${m.url}`;
  return m.transport || "";
}

export function MCPServerList({
  servers,
  loading,
  onAdd,
  onRemove,
  className,
  registry = [],
  addError,
}: MCPServerListProps) {
  const [input, setInput] = useState("");
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);
  const [showDropdown, setShowDropdown] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Normalise server names for comparison (strip mcp__ prefix)
  const normalised = servers.map((s) => s.replace(/^mcp__/, ""));

  const suggestions = registry.filter((m) => {
    if (normalised.includes(m.name)) return false;
    if (!input.trim()) return true;
    const q = input.toLowerCase();
    return m.name.toLowerCase().includes(q) || summarize(m).toLowerCase().includes(q);
  });

  const error = addError ?? localError;

  // Close dropdown on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(e.target as Node)
      ) {
        setShowDropdown(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const doAdd = (name: string) => {
    const trimmed = name.trim();
    if (!trimmed || !onAdd) return;
    setAdding(true);
    setShowDropdown(false);
    setLocalError(null);
    onAdd(trimmed)
      .then(() => {
        setInput("");
      })
      .catch((err: unknown) => {
        setLocalError(err instanceof Error ? err.message : "Failed to add MCP server");
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
        className="text-xs text-mycel-muted italic pl-1"
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
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-mycel-border bg-mycel-surface text-[11px] text-mycel-text-2 font-medium"
              style={{ fontFamily: MONO }}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-mycel-accent" />
              {s.replace(/^mcp__/, "")}
              {onRemove && (
                <button
                  type="button"
                  onClick={() => { handleRemove(s); }}
                  disabled={removing === s}
                  className="ml-0.5 text-mycel-muted hover:text-mycel-error transition-colors disabled:opacity-40 leading-none"
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
        <p className="text-xs text-mycel-muted italic pl-1">
          No MCP servers configured.
        </p>
      )}
      {onAdd && (
        <div className="mt-3 relative">
          <div className="flex items-center gap-2">
            <input
              ref={inputRef}
              type="text"
              value={input}
              onChange={(e) => {
                setInput(e.target.value);
                setShowDropdown(true);
                if (localError) setLocalError(null);
              }}
              onFocus={() => setShowDropdown(true)}
              onKeyDown={(e) => {
                if (e.key === "Enter") doAdd(input);
                if (e.key === "Escape") setShowDropdown(false);
              }}
              placeholder="Search or type a server name…"
              disabled={adding}
              aria-invalid={error ? true : undefined}
              className={`flex-1 max-w-[300px] rounded border bg-mycel-bg px-2.5 py-1 text-[11px] text-mycel-text placeholder:text-mycel-muted outline-none transition-colors disabled:opacity-40 ${
                error ? "border-mycel-error focus:border-mycel-error" : "border-mycel-border focus:border-mycel-accent"
              }`}
              style={{ fontFamily: MONO }}
            />
            <button
              type="button"
              onClick={() => doAdd(input)}
              disabled={adding || !input.trim()}
              className="px-2.5 py-1 rounded border border-mycel-accent bg-mycel-accent-subtle text-[11px] text-mycel-accent hover:bg-mycel-accent-subtle transition-colors disabled:opacity-40"
              style={{ fontFamily: MONO }}
            >
              {adding ? "Adding…" : "+ Add MCP"}
            </button>
          </div>

          {/* Suggestions dropdown */}
          {showDropdown && suggestions.length > 0 && (
            <div
              ref={dropdownRef}
              className="absolute z-20 top-full mt-1 w-full max-w-[420px] rounded-md border border-mycel-border bg-mycel-surface shadow-lg overflow-hidden"
            >
              {suggestions.map((m) => (
                <button
                  key={m.name}
                  type="button"
                  onMouseDown={(e) => {
                    // prevent input blur before click registers
                    e.preventDefault();
                    doAdd(m.name);
                  }}
                  className="w-full flex items-start gap-3 px-3 py-2 hover:bg-mycel-accent-subtle transition-colors text-left group"
                >
                  <span
                    className="shrink-0 mt-0.5 w-1.5 h-1.5 rounded-full bg-mycel-accent opacity-50 group-hover:opacity-100 transition-opacity"
                  />
                  <div className="min-w-0">
                    <span
                      className="block text-[11px] font-semibold text-mycel-text group-hover:text-mycel-accent transition-colors"
                      style={{ fontFamily: MONO }}
                    >
                      {m.name}
                    </span>
                    <span
                      className="block text-[10px] text-mycel-muted truncate"
                      style={{ fontFamily: MONO }}
                    >
                      {summarize(m)}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          )}

          {error ? (
            <p className="mt-1.5 text-[10px] text-mycel-error" style={{ fontFamily: MONO }} role="alert">
              {error}
            </p>
          ) : (
            <p className="mt-1.5 text-[10px] text-mycel-muted" style={{ fontFamily: MONO }}>
              Select a server from the registry — only names with a real command or url can connect.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
