import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import type { ProviderInfo } from "../api/client";
import { formatCost, formatTokens } from "../utils/format";

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

interface ProviderCardProps {
  provider: ProviderInfo;
  onClick: () => void;
}

export function ProviderCard({ provider, onClick }: ProviderCardProps) {
  const [modelsOpen, setModelsOpen] = useState(false);
  const letter = provider.name.charAt(0).toUpperCase();
  const isActive = provider.installed && provider.agent_count > 0;
  const isInstalled = provider.installed;
  const models = provider.models ?? [];

  return (
    <motion.div
      whileHover={{ y: -1 }}
      transition={{ type: "spring", stiffness: 400, damping: 25 }}
      onClick={onClick}
      className="group rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel p-4 cursor-pointer hover:border-mycel-accent hover:bg-mycel-surface-hover transition-colors"
    >
      <div className="flex items-start gap-3">
        {/* Monogram */}
        <div className="flex-shrink-0 w-10 h-10 rounded-full bg-mycel-accent-subtle flex items-center justify-center">
          <span className="text-sm font-semibold text-mycel-accent">{letter}</span>
        </div>

        <div className="flex-1 min-w-0">
          {/* Name + status */}
          <div className="flex items-center gap-2">
            <span className="font-medium text-sm text-mycel-text truncate">
              {provider.name}
            </span>
            <span className="relative flex h-2 w-2 shrink-0">
              {isActive && (
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-mycel-success opacity-75" />
              )}
              <span
                className={`relative inline-flex rounded-full h-2 w-2 ${
                  isActive
                    ? "bg-mycel-success"
                    : isInstalled
                      ? "bg-mycel-muted"
                      : "bg-mycel-error"
                }`}
              />
            </span>
          </div>

          {/* Version badge */}
          {provider.version && (
            <span className="inline-block mt-1 px-1.5 py-0.5 rounded-md text-xs font-mono bg-mycel-surface border border-mycel-border text-mycel-muted">
              v{provider.version}
            </span>
          )}
        </div>

        {/* Arrow */}
        <svg
          className="w-4 h-4 text-mycel-muted opacity-0 group-hover:opacity-100 transition-opacity shrink-0 mt-1"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </div>

      {/* Chips row */}
      <div className="flex items-center gap-2 mt-3 flex-wrap">
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-mycel-accent-subtle text-mycel-accent">
          <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
          </svg>
          {provider.agent_count}
        </span>
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-mycel-info-subtle text-mycel-info tabular-nums">
          {formatTokens(provider.total_tokens)} tok
        </span>
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-mycel-success-subtle text-mycel-success tabular-nums">
          {formatCost(provider.total_cost_usd)}
        </span>
      </div>

      {/* Models affordance — stop propagation so clicking expand doesn't navigate */}
      {models.length > 0 && (
        <div className="mt-2" onClick={(e) => e.stopPropagation()}>
          <button
            type="button"
            onClick={() => setModelsOpen((o) => !o)}
            className="inline-flex items-center gap-1 text-[11px] text-mycel-muted hover:text-mycel-text transition-colors"
            aria-expanded={modelsOpen}
            aria-label={`${modelsOpen ? "Hide" : "Show"} models for ${provider.name}`}
          >
            <motion.svg
              animate={{ rotate: modelsOpen ? 90 : 0 }}
              transition={{ type: "spring", stiffness: 300, damping: 20 }}
              className="w-3 h-3"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2.5}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </motion.svg>
            {models.length} model{models.length !== 1 ? "s" : ""}
          </button>
          <AnimatePresence initial={false}>
            {modelsOpen && (
              <motion.div
                key="models"
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ type: "spring", stiffness: 300, damping: 30 }}
                className="overflow-hidden"
              >
                <div className="mt-1.5 space-y-0.5 max-h-40 overflow-y-auto">
                  {models.map((m) => (
                    <div
                      key={m.id}
                      className="flex items-center justify-between px-1.5 py-1 hover:bg-mycel-surface-hover rounded-md transition-colors"
                    >
                      <span className="font-mono text-[10px] text-mycel-text truncate mr-2">{m.id}</span>
                      <AvailabilityBadge available={m.available} />
                    </div>
                  ))}
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      )}
    </motion.div>
  );
}
