import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { usePolling } from "../hooks/usePolling";
import { api } from "../api/client";
import type { ProviderInfo, ModelInfo } from "../api/client";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { useHeaderSlot } from "../context/HeaderSlotContext";

const modelCount = (models: ModelInfo[] | undefined) => models?.length ?? 0;

function AvailabilityBadge({ available }: { available: boolean }) {
  return available ? (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[10px] font-medium bg-mycel-success-subtle text-mycel-success">
      <span className="w-1.5 h-1.5 rounded-full bg-mycel-success inline-block" />
      live
    </span>
  ) : (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[10px] font-medium bg-mycel-surface border border-mycel-border text-mycel-muted">
      static
    </span>
  );
}

function ModelRow({ model }: { model: ModelInfo }) {
  return (
    <div className="flex items-center justify-between px-3 py-1.5 hover:bg-mycel-surface-hover transition-colors rounded-md">
      <span className="font-mono text-xs text-mycel-text truncate mr-2">{model.id}</span>
      <AvailabilityBadge available={model.available} />
    </div>
  );
}

function ProviderRow({
  provider,
  search,
}: {
  provider: ProviderInfo;
  search: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const models = provider.models ?? [];
  const filtered = search
    ? models.filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
    : models;

  const isInstalled = provider.installed;
  const hasModels = models.length > 0;

  return (
    <motion.div
      layout
      className="rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel overflow-hidden"
    >
      {/* Header row */}
      <button
        type="button"
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-mycel-surface-hover transition-colors"
        onClick={() => hasModels && setExpanded((e) => !e)}
        aria-expanded={expanded}
      >
        {/* Monogram */}
        <div className="w-8 h-8 rounded-full bg-mycel-accent-subtle flex items-center justify-center shrink-0">
          <span className="text-xs font-semibold text-mycel-accent">
            {provider.name.charAt(0).toUpperCase()}
          </span>
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium text-sm text-mycel-text">{provider.name}</span>
            {provider.version && (
              <span className="px-1.5 py-0.5 rounded-md text-xs font-mono bg-mycel-bg border border-mycel-border text-mycel-muted">
                v{provider.version}
              </span>
            )}
            {!isInstalled && (
              <span className="px-1.5 py-0.5 rounded-md text-xs bg-mycel-error-subtle text-mycel-error">
                not installed
              </span>
            )}
          </div>
          <p className="text-xs text-mycel-muted mt-0.5 truncate">{provider.description}</p>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {hasModels && (
            <span className="text-xs tabular-nums text-mycel-muted">
              {models.length} model{models.length !== 1 ? "s" : ""}
            </span>
          )}
          {hasModels && (
            <motion.svg
              animate={{ rotate: expanded ? 90 : 0 }}
              transition={{ type: "spring", stiffness: 300, damping: 20 }}
              className="w-4 h-4 text-mycel-muted"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </motion.svg>
          )}
          {!hasModels && <span className="text-xs text-mycel-muted italic">no models</span>}
        </div>
      </button>

      {/* Model list */}
      <AnimatePresence initial={false}>
        {expanded && hasModels && (
          <motion.div
            key="models"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ type: "spring", stiffness: 300, damping: 30 }}
            className="overflow-hidden border-t border-mycel-border"
          >
            <div className="px-3 py-2 max-h-64 overflow-y-auto">
              {filtered.length === 0 ? (
                <p className="text-xs text-mycel-muted py-2 text-center">
                  No models match &ldquo;{search}&rdquo;
                </p>
              ) : (
                filtered.map((m) => <ModelRow key={m.id} model={m} />)
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  );
}

export function Providers() {
  const [search, setSearch] = useState("");

  const { data: providers, loading, error } = usePolling<ProviderInfo[]>(
    () => api.listProviders(),
    15_000,
  );

  useHeaderSlot({
    actions: (
      <input
        type="search"
        placeholder="Search providers or models…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="w-48 px-2.5 py-1 text-xs rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
      />
    ),
  });

  if (loading && !providers) {
    return (
      <div className="p-6">
        <LoadingSkeleton />
      </div>
    );
  }

  if (error) {
    return (
      <EmptyState icon="!" title="Failed to load providers" description={String(error)} />
    );
  }

  const list = providers ?? [];

  // Filter: include provider if its name matches OR any model ID matches.
  const filtered = search
    ? list.filter(
        (p) =>
          p.name.toLowerCase().includes(search.toLowerCase()) ||
          (p.models ?? []).some((m) => m.id.toLowerCase().includes(search.toLowerCase())),
      )
    : list;

  const liveCount = list.reduce(
    (n, p) => n + (p.models ?? []).filter((m) => m.available).length,
    0,
  );
  const totalModels = list.reduce((n, p) => n + modelCount(p.models), 0);

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Subheader stats */}
      <div className="px-6 py-3 border-b border-mycel-border bg-mycel-surface flex items-center gap-4 text-xs text-mycel-muted shrink-0">
        <span>
          <span className="font-medium text-mycel-text">{list.length}</span> provider
          {list.length !== 1 ? "s" : ""}
        </span>
        <span className="text-mycel-border">|</span>
        <span>
          <span className="font-medium text-mycel-text">{totalModels}</span> models
        </span>
        <span className="text-mycel-border">|</span>
        <span>
          <span className="font-medium text-mycel-success">{liveCount}</span> live
        </span>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        {filtered.length === 0 ? (
          <EmptyState
            icon="*"
            title={search ? "No matches" : "No providers"}
            description={
              search
                ? `No providers or models match "${search}".`
                : "No AI providers are registered."
            }
          />
        ) : (
          <div className="flex flex-col gap-3">
            {filtered.map((p) => (
              <ProviderRow key={p.name} provider={p} search={search} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
