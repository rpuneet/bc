import { useCallback, useMemo, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { useHeaderSlot } from "../context/HeaderSlotContext";

// ─── Types ───────────────────────────────────────────────────────────────────

type ItemType = "mcp" | "skill" | "template";
type ItemSource = "mcp-registry" | "github" | "mycel";

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

// ─── Constants ────────────────────────────────────────────────────────────────

const SOURCE_LABELS: Record<ItemSource, string> = {
  "mcp-registry": "MCP Registry",
  github: "GitHub",
  mycel: "mycel",
};

const SOURCE_COLORS: Record<ItemSource, string> = {
  "mcp-registry": "bg-mycel-accent-subtle text-mycel-accent",
  github: "bg-mycel-success-subtle text-mycel-success",
  mycel: "bg-mycel-error-subtle text-mycel-error",
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
          title="Add to agent"
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

function InstallNotice({
  item,
  onDismiss,
}: {
  item: MarketplaceItem;
  onDismiss: () => void;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 8 }}
      className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 flex items-center gap-3 px-4 py-3 rounded-lg border border-mycel-border bg-mycel-surface shadow-lg text-sm text-mycel-text"
    >
      <span>
        Deep install wiring for{" "}
        <strong className="font-semibold">{item.name}</strong> is coming in a
        follow-up release. For now, add it manually via{" "}
        <code className="font-mono text-xs">bc template edit</code>.
      </span>
      <button
        onClick={onDismiss}
        className="text-mycel-muted hover:text-mycel-text transition-colors ml-1"
        aria-label="Dismiss"
      >
        ✕
      </button>
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

      {/* Install notice toast */}
      <AnimatePresence>
        {pendingInstall && (
          <InstallNotice
            item={pendingInstall}
            onDismiss={() => setPendingInstall(null)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
