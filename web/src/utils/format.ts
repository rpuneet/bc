/** Format a token count for compact display (e.g. 1500000 -> "1.5M").
 *
 * Input is the raw token count (int64 from the API, e.g. `provider.total_tokens`),
 * never a pre-scaled millions figure. Callers must not pre-divide — doing so would
 * produce absurd values like "44307.0M" for a 44 B raw count that had been scaled twice.
 */
export function formatTokens(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "0";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
  return String(n);
}

/** Format a USD cost for display (e.g. 1234.5 -> "$1,234.50"). */
export function formatCost(n: number): string {
  if (n === 0) return "$0.00";
  if (Math.abs(n) < 0.01) {
    // Sub-cent values keep 4 decimals and skip group separators.
    return `$${n.toFixed(4)}`;
  }
  return `$${n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

/** How total_cost_usd was produced — mirrors cost.CostBasis on the API. */
export type CostBasis = "priced" | "billed";

/**
 * Dollar-figure label for a cost_basis. Priced dollars are estimates from
 * local model rate tables; billed (reserved) would be invoice figures.
 * Default to priced when the field is missing so older daemons stay honest.
 */
export function costSpendLabel(basis?: CostBasis | string, noun: "spend" | "cost" = "spend"): string {
  if (basis === "billed") return noun === "cost" ? "Billed cost" : "Billed spend";
  return noun === "cost" ? "Estimated cost" : "Estimated spend";
}

/** Short subtitle explaining a priced dollar figure. */
export function costBasisSub(basis?: CostBasis | string): string | undefined {
  if (basis === "billed") return "from provider billing";
  return "model rates · not provider billing";
}
