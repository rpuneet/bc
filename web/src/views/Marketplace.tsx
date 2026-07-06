import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { useHeaderSlot } from "../context/HeaderSlotContext";

// ─── Types ───────────────────────────────────────────────────────────────────

type ItemType = "mcp" | "skill" | "template";
type ItemSource =
  | "mcp-registry"
  | "github"
  | "mycel"
  | "claude"
  | "openclaw"
  | "gemini";

interface MarketplaceItem {
  id: string;
  name: string;
  description?: string;
  url?: string;
  stars?: number;
  source: ItemSource;
  type: ItemType;
  install_spec?: string;
}

interface Agent {
  name: string;
}

// ─── Constants ────────────────────────────────────────────────────────────────

const SOURCE_LABELS: Record<ItemSource, string> = {
  "mcp-registry": "MCP Registry",
  github: "GitHub",
  mycel: "mycel",
  claude: "Claude skills",
  openclaw: "openclaw",
  gemini: "Google",
};

const SOURCE_COLORS: Record<ItemSource, string> = {
  "mcp-registry": "bg-mycel-accent-subtle text-mycel-accent",
  github: "bg-mycel-success-subtle text-mycel-success",
  mycel: "bg-mycel-error-subtle text-mycel-error",
  claude: "bg-mycel-accent-subtle text-mycel-accent",
  openclaw: "bg-mycel-success-subtle text-mycel-success",
  gemini: "bg-mycel-border text-mycel-muted",
};

const TYPE_LABELS: Record<ItemType, string> = {
  mcp: "MCP Server",
  skill: "Skill",
  template: "Template",
};

const TYPE_COLORS: Record<ItemType, string> = {
  mcp: "bg-mycel-border text-mycel-text",
  skill: "bg-mycel-border text-mycel-text",
  template: "bg-mycel-border text-mycel-muted",
};

const ALL_TYPES: Array<{ value: string; label: string }> = [
  { value: "", label: "All types" },
  { value: "mcp", label: "MCP Servers" },
  { value: "skill", label: "Skills" },
  { value: "template", label: "Templates" },
];

const ALL_SOURCES: Array<{ value: string; label: string }> = [
  { value: "", label: "All sources" },
  { value: "mcp-registry", label: "MCP Registry" },
  { value: "claude", label: "Claude skills" },
  { value: "openclaw", label: "openclaw" },
  { value: "gemini", label: "Google" },
  { value: "github", label: "GitHub" },
  { value: "mycel", label: "mycel" },
];

// ─── API ──────────────────────────────────────────────────────────────────────

async function fetchMarketplace(
  typeFilter: string,
  sourceFilter: string,
  query: string,
): Promise<MarketplaceItem[]> {
  const params = new URLSearchParams();
  if (typeFilter) params.set("type", typeFilter);
  if (sourceFilter) params.set("source", sourceFilter);
  if (query) params.set("q", query);
  const qs = params.toString();
  const res = await fetch(`/api/marketplace${qs ? `?${qs}` : ""}`);
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json() as Promise<MarketplaceItem[]>;
}

async function fetchAgents(): Promise<Agent[]> {
  const res = await fetch("/api/agents");
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  const data = (await res.json()) as { agents?: Agent[] } | Agent[];
  // Handle both {agents: [...]} and [...] shapes.
  if (Array.isArray(data)) return data;
  return (data as { agents?: Agent[] }).agents ?? [];
}

async function sendInstall(
  item: MarketplaceItem,
  agents: string[],
): Promise<{ dispatched: number }> {
  const res = await fetch("/api/marketplace/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      item_id: item.id,
      item_name: item.name,
      item_source_url: item.url ?? "",
      item_type: item.type,
      item_source: item.source,
      agents,
    }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `API error: ${res.status}`);
  }
  return res.json() as Promise<{ dispatched: number }>;
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function Badge({
  label,
  className,
}: {
  label: string;
  className: string;
}) {
  return (
    <span
      className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium leading-tight ${className}`}
    >
      {label}
    </span>
  );
}

function StarCount({ stars }: { stars: number }) {
  const fmt =
    stars >= 1000 ? `${(stars / 1000).toFixed(1)}k` : String(stars);
  return (
    <span className="flex items-center gap-0.5 text-xs text-mycel-muted">
      <svg
        viewBox="0 0 16 16"
        width="11"
        height="11"
        fill="currentColor"
        className="text-mycel-muted"
      >
        <path d="M8 .25a.75.75 0 01.673.418l1.882 3.815 4.21.612a.75.75 0 01.416 1.279l-3.046 2.97.719 4.192a.75.75 0 01-1.088.791L8 12.347l-3.766 1.98a.75.75 0 01-1.088-.79l.72-4.194L.818 6.374a.75.75 0 01.416-1.28l4.21-.611L7.327.668A.75.75 0 018 .25z" />
      </svg>
      {fmt}
    </span>
  );
}

// ─── Agent Picker ─────────────────────────────────────────────────────────────

interface AgentPickerProps {
  item: MarketplaceItem;
  onDismiss: () => void;
}

function AgentPicker({ item, onDismiss }: AgentPickerProps) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    void fetchAgents()
      .then(setAgents)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  // Close on outside click.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (overlayRef.current && e.target === overlayRef.current) {
        onDismiss();
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onDismiss]);

  function toggle(name: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  async function handleSend() {
    if (selected.size === 0) return;
    setSending(true);
    setError(null);
    try {
      const result = await sendInstall(item, Array.from(selected));
      setToast(`Instruction sent to ${result.dispatched} agent${result.dispatched !== 1 ? "s" : ""}`);
      setTimeout(onDismiss, 1800);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setSending(false);
    }
  }

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <motion.div
        initial={{ opacity: 0, scale: 0.96 }}
        animate={{ opacity: 1, scale: 1 }}
        exit={{ opacity: 0, scale: 0.96 }}
        transition={{ duration: 0.1 }}
        className="w-72 rounded-lg border border-mycel-border bg-mycel-surface shadow-xl"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-mycel-border">
          <span className="text-sm font-semibold text-mycel-text truncate">
            Send to agent
          </span>
          <button
            onClick={onDismiss}
            className="text-mycel-muted hover:text-mycel-text transition-colors ml-2 shrink-0"
            aria-label="Close"
          >
            ✕
          </button>
        </div>

        {/* Item summary */}
        <div className="px-4 py-2 border-b border-mycel-border bg-mycel-bg/50">
          <p className="text-xs text-mycel-muted truncate">
            <span className="font-medium text-mycel-text">{item.name}</span>
            {" · "}
            {SOURCE_LABELS[item.source] ?? item.source}
          </p>
        </div>

        {/* Agent list */}
        <div className="px-4 py-3 max-h-56 overflow-y-auto">
          {loading && (
            <p className="text-xs text-mycel-muted">Loading agents…</p>
          )}
          {!loading && agents.length === 0 && !error && (
            <p className="text-xs text-mycel-muted">No agents found.</p>
          )}
          {!loading &&
            agents.map((a) => (
              <label
                key={a.name}
                className="flex items-center gap-2 py-1.5 cursor-pointer group"
              >
                <input
                  type="checkbox"
                  className="accent-mycel-accent"
                  checked={selected.has(a.name)}
                  onChange={() => toggle(a.name)}
                />
                <span className="text-sm text-mycel-text group-hover:text-mycel-accent transition-colors truncate">
                  {a.name}
                </span>
              </label>
            ))}
        </div>

        {/* Footer */}
        <div className="px-4 py-3 border-t border-mycel-border flex flex-col gap-2">
          {error && (
            <p className="text-xs text-mycel-error">{error}</p>
          )}
          {toast && (
            <p className="text-xs text-mycel-success">{toast}</p>
          )}
          <div className="flex gap-2">
            <button
              onClick={onDismiss}
              className="flex-1 px-3 py-1.5 text-xs rounded-md border border-mycel-border text-mycel-muted hover:bg-mycel-bg transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => void handleSend()}
              disabled={selected.size === 0 || sending}
              className="flex-1 px-3 py-1.5 text-xs rounded-md bg-mycel-accent text-mycel-accent-fg hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed transition-opacity"
            >
              {sending ? "Sending…" : `Send${selected.size > 0 ? ` (${selected.size})` : ""}`}
            </button>
          </div>
        </div>
      </motion.div>
    </div>
  );
}

// ─── Item Card ────────────────────────────────────────────────────────────────

function ItemCard({
  item,
  onInstall,
}: {
  item: MarketplaceItem;
  onInstall: (item: MarketplaceItem) => void;
}) {
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={{ duration: 0.12 }}
      className="group flex flex-col gap-2 p-4 rounded-lg border border-mycel-border bg-mycel-surface hover:bg-mycel-surface-hover transition-colors"
    >
      {/* Header row */}
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5 flex-wrap">
            <span className="text-sm font-semibold text-mycel-text truncate">
              {item.name}
            </span>
            <Badge
              label={TYPE_LABELS[item.type] ?? item.type}
              className={TYPE_COLORS[item.type] ?? "bg-mycel-border text-mycel-muted"}
            />
            <Badge
              label={SOURCE_LABELS[item.source] ?? item.source}
              className={SOURCE_COLORS[item.source] ?? "bg-mycel-border text-mycel-muted"}
            />
          </div>
          {item.description && (
            <p className="mt-1 text-xs text-mycel-muted line-clamp-2">
              {item.description}
            </p>
          )}
        </div>
        {/* Add button */}
        <button
          onClick={() => onInstall(item)}
          className="shrink-0 px-2.5 py-1 text-xs rounded-md border border-mycel-border text-mycel-muted hover:bg-mycel-accent hover:text-mycel-accent-fg hover:border-mycel-accent transition-colors"
          title="Send install instruction to an agent"
        >
          Add
        </button>
      </div>

      {/* Footer row */}
      <div className="flex items-center gap-3 mt-auto">
        {typeof item.stars === "number" && item.stars > 0 && (
          <StarCount stars={item.stars} />
        )}
        {item.url && (
          <a
            href={item.url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-mycel-muted hover:text-mycel-accent transition-colors truncate"
            onClick={(e) => e.stopPropagation()}
          >
            {item.url.replace(/^https?:\/\//, "")}
          </a>
        )}
      </div>
    </motion.div>
  );
}

// ─── Main view ────────────────────────────────────────────────────────────────

export function Marketplace() {
  const [typeFilter, setTypeFilter] = useState("");
  const [sourceFilter, setSourceFilter] = useState("");
  const [query, setQuery] = useState("");
  const [pendingInstall, setPendingInstall] = useState<MarketplaceItem | null>(
    null,
  );

  const fetcher = useCallback(
    () => fetchMarketplace(typeFilter, sourceFilter, query),
    [typeFilter, sourceFilter, query],
  );

  const {
    data: items,
    loading,
    error,
  } = usePolling<MarketplaceItem[]>(
    fetcher,
    60_000, // refresh every minute; catalog is cached server-side for 1h
  );

  const totalSources = useMemo(() => {
    if (!items) return null;
    const seen = new Set(items.map((i) => i.source));
    return seen.size;
  }, [items]);

  useHeaderSlot({
    title:
      totalSources != null ? (
        <span className="text-xs text-mycel-muted">
          {totalSources} source{totalSources !== 1 ? "s" : ""} active
        </span>
      ) : undefined,
  });

  const inputCls =
    "px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent";
  const selectCls =
    "px-2 py-1.5 text-sm rounded-md border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent";

  return (
    <div className="flex flex-col gap-4 p-4 max-w-4xl mx-auto w-full">
      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <input
          type="search"
          placeholder="Search catalog…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className={`${inputCls} flex-1 min-w-[180px]`}
        />
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className={selectCls}
        >
          {ALL_TYPES.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <select
          value={sourceFilter}
          onChange={(e) => setSourceFilter(e.target.value)}
          className={selectCls}
        >
          {ALL_SOURCES.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      {/* Content */}
      {loading && !items && (
        <LoadingSkeleton rows={6} variant="cards" />
      )}

      {error && (
        <div className="text-sm text-mycel-error px-1">
          Failed to load catalog: {error}
        </div>
      )}

      {!loading && items && items.length === 0 && (
        <EmptyState
          title="No items found"
          description={
            query || typeFilter || sourceFilter
              ? "Try clearing your filters."
              : "The catalog is empty or all sources are unavailable."
          }
        />
      )}

      {items && items.length > 0 && (
        <>
          <p className="text-xs text-mycel-muted px-0.5">
            {items.length} item{items.length !== 1 ? "s" : ""}
            {typeFilter || sourceFilter || query ? " (filtered)" : ""}
          </p>
          <AnimatePresence mode="popLayout">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {items.map((item) => (
                <ItemCard
                  key={item.id}
                  item={item}
                  onInstall={setPendingInstall}
                />
              ))}
            </div>
          </AnimatePresence>
        </>
      )}

      {/* Agent picker modal */}
      <AnimatePresence>
        {pendingInstall && (
          <AgentPicker
            item={pendingInstall}
            onDismiss={() => setPendingInstall(null)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
