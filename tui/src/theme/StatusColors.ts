/**
 * StatusColors - Consistent color scheme for status indicators across all views
 * Issue #1040: Establish consistent color scheme for status indicators
 *
 * Defines status colors used consistently across Dashboard, Agents, Channels, Costs, Logs views.
 * Combines symbols + colors for accessibility (not color-only coding).
 */

/**
 * Status color definitions with semantic meaning
 */
export const STATUS_COLORS = {
  working: 'cyan', // Active/Working agents
  idle: 'gray', // Inactive/Idle agents
  done: 'green', // Completed/Done state
  error: 'red', // Error/Failed state
  warning: 'yellow', // Warning/Attention needed
  info: 'blue', // Information/General
  pending: 'gray', // Pending/Not started
  success: 'green', // Success/OK state
} as const;

export type StatusColor = keyof typeof STATUS_COLORS;

/**
 * Status symbols (for accessibility - symbol + color combination)
 */
export const STATUS_SYMBOLS = {
  working: '●', // Filled circle - actively working
  idle: '○', // Empty circle - not working
  done: '✓', // Checkmark - completed
  error: '✗', // X mark - failed
  warning: '⚠', // Warning sign
  info: 'ℹ', // Info symbol
  pending: '−', // Dash - not started
  success: '✓', // Checkmark - OK
} as const;

/**
 * Get color for a given status
 */
export function getStatusColor(status: StatusColor): string {
  return STATUS_COLORS[status];
}

/**
 * Get symbol for a given status
 */
export function getStatusSymbol(status: StatusColor): string {
  return STATUS_SYMBOLS[status];
}

/**
 * Get both color and symbol for a status (recommended pattern)
 */
export function getStatusIndicator(status: StatusColor): { color: string; symbol: string } {
  return {
    color: getStatusColor(status),
    symbol: getStatusSymbol(status),
  };
}

/**
 * Health status colors (for progress/health indicators)
 */
export const HEALTH_COLORS = {
  healthy: 'green', // 80-100% healthy
  warning: 'yellow', // 50-79% healthy
  critical: 'red', // <50% healthy
} as const;

/**
 * Health status symbols (for accessibility - colorblind support #1220)
 */
export const HEALTH_SYMBOLS = {
  healthy: '●', // Filled circle - all good
  warning: '◐', // Half circle - needs attention
  critical: '○', // Empty circle - critical
} as const;

export type HealthStatus = keyof typeof HEALTH_COLORS;

/**
 * Get health indicator with color and symbol
 */
export function getHealthIndicator(status: HealthStatus): {
  color: string;
  symbol: string;
  label: string;
} {
  const labels: Record<HealthStatus, string> = {
    healthy: 'Healthy',
    warning: 'Warning',
    critical: 'Critical',
  };
  return {
    color: HEALTH_COLORS[status],
    symbol: HEALTH_SYMBOLS[status],
    label: labels[status],
  };
}

/**
 * Cost/budget colors
 */
export const COST_COLORS = {
  normal: 'green', // <75% of budget used
  warning: 'yellow', // 75-90% of budget used
  critical: 'red', // >90% of budget used
} as const;

/**
 * Cost/budget symbols (for accessibility - colorblind support #1220)
 */
export const COST_SYMBOLS = {
  normal: '✓', // Checkmark - within budget
  warning: '⚠', // Warning - approaching limit
  critical: '!', // Exclamation - over/at budget
} as const;

export type CostStatus = keyof typeof COST_COLORS;

/**
 * Get cost indicator with color and symbol
 */
export function getCostIndicator(status: CostStatus): {
  color: string;
  symbol: string;
  label: string;
} {
  const labels: Record<CostStatus, string> = {
    normal: 'OK',
    warning: 'Near Limit',
    critical: 'Over Budget',
  };
  return {
    color: COST_COLORS[status],
    symbol: COST_SYMBOLS[status],
    label: labels[status],
  };
}

/**
 * Check if high contrast mode is enabled
 * Supports: MYCEL_HIGH_CONTRAST env var, config tui.high_contrast
 * #1220: Colorblind-friendly visual cues
 */
export function isHighContrastEnabled(): boolean {
  // Check environment variable
  const envValue = process.env.MYCEL_HIGH_CONTRAST;
  return envValue === '1' || envValue === 'true';
}

/**
 * Agent role colors — use constants/colors.ts as the single source of truth.
 * See Issue #1847 for the design audit that identified this duplication.
 * Import { ROLE_COLORS, getColorForName } from '../constants/colors' instead.
 */

export default {
  STATUS_COLORS,
  STATUS_SYMBOLS,
  HEALTH_COLORS,
  HEALTH_SYMBOLS,
  COST_COLORS,
  COST_SYMBOLS,
  getStatusColor,
  getStatusSymbol,
  getStatusIndicator,
  getHealthIndicator,
  getCostIndicator,
  isHighContrastEnabled,
};
