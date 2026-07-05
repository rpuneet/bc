/**
 * Insights — a single-page analytics dashboard.
 *
 * Metrics and Costs used to live behind a tab bar; they are now one
 * scrollable dashboard rendered by <Stats/>, which owns the KPI strip,
 * the sticky section anchor-nav and every chart panel. The range picker
 * is slotted into the page header via Stats' useHeaderSlot. /stats,
 * /metrics and /costs redirect here (see App.tsx) so old links keep
 * working.
 */

import { Stats } from "./Stats";

export function Insights() {
  return <Stats />;
}
