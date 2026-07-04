import { motion } from "framer-motion";
import type { ProviderInfo } from "../api/client";
import { formatCost, formatTokens } from "../utils/format";

interface ProviderCardProps {
  provider: ProviderInfo;
  onClick: () => void;
}

export function ProviderCard({ provider, onClick }: ProviderCardProps) {
  const letter = provider.name.charAt(0).toUpperCase();
  const isActive = provider.installed && provider.agent_count > 0;
  const isInstalled = provider.installed;

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
          <span className="text-sm font-bold text-mycel-accent">{letter}</span>
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
            <span className="inline-block mt-1 px-1.5 py-0.5 rounded text-xs font-mono bg-mycel-surface border border-mycel-border text-mycel-muted">
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
    </motion.div>
  );
}
