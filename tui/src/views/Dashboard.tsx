import { memo, useCallback } from 'react';
import { Box, Text, useInput, useStdout } from 'ink';
import { useTheme } from '../theme';
import { useIsOverlayActive } from '../navigation/FocusContext';
import { Panel } from '../components/Panel.js';
import { MetricCard } from '../components/MetricCard.js';
import { Footer } from '../components/Footer.js';
import { LoadingIndicator } from '../components/LoadingIndicator.js';
import { ErrorDisplay } from '../components/ErrorDisplay.js';
import { ActivityFeed } from '../components/ActivityFeed.js';
import { useDashboard } from '../hooks/useDashboard.js';
import {
  STATUS_COLORS,
  STATUS_SYMBOLS,
  HEALTH_COLORS,
  getCostIndicator,
  type CostStatus,
} from '../theme/StatusColors.js';

/**
 * Dashboard view - main overview of bc workspace
 * Issues #543 (layout), #544 (stats components), #931 (shortcuts fix)
 */
export function Dashboard() {
  const { stdout } = useStdout();
  const terminalWidth = stdout.columns;
  const isNarrow = terminalWidth < 100;
  const canMultiColumn = terminalWidth >= 120;
  const isMedium = terminalWidth >= 100;
  const isWide = terminalWidth >= 140;

  const {
    summary,
    agents,
    // channels removed from dashboard - use Notifications tab [3]
    agentStats,
    isLoading,
    error,
    refresh,
    lastRefresh,
  } = useDashboard();

  // #1596: Memoize keyboard input handler
  const handleDashboardInput = useCallback(
    (input: string, _key: { ctrl: boolean }) => {
      // Refresh is Dashboard-specific (global Ctrl+R handled elsewhere)
      if (input === 'r') {
        void refresh();
      }
      // Note: q and ESC are handled by global useKeyboardNavigation
    },
    [refresh]
  );

  // Keyboard navigation - Dashboard-specific shortcuts
  // Global shortcuts (1-9, Tab, ESC, q) are handled by useKeyboardNavigation
  // Note: Letter shortcuts (a, c, $) removed to avoid confusion - use numbers [2] [3] [4]
  // See #1327 for comprehensive keybinding system design
  const overlayActive = useIsOverlayActive();
  useInput(handleDashboardInput, { isActive: !overlayActive });

  if (error) {
    return (
      <ErrorDisplay
        error={error.message}
        onRetry={() => {
          void refresh();
        }}
      />
    );
  }

  // Progressive loading: show content structure while data loads (#1614)
  // Only block on initial load when no data exists yet
  const showInitialLoading = isLoading && !agents.data;

  // #1318: Don't set explicit width - parent flexGrow handles it
  // Setting width={terminalWidth} caused overflow when nested inside drawer layout
  return (
    <Box flexDirection="column" padding={1} overflow="hidden">
      {/* Header with activity indicator */}
      <Header
        workspaceName={summary.workspaceName}
        isLoading={isLoading}
        lastRefresh={lastRefresh}
      />

      {/* Summary Cards - Agent counts */}
      <SummaryCards
        total={summary.total}
        active={summary.active}
        working={summary.working}
        idle={summary.idle}
        stuck={summary.stuck}
        errorCount={summary.error}
        isNarrow={isNarrow}
      />

      {/* #1614: Simplified layout - StatsPanels renders once with layout prop */}
      {/* Main Content - Uses responsive layout for flexible column arrangement */}
      <Box marginTop={1} flexDirection={canMultiColumn ? 'row' : 'column'}>
        {/* Stats panels at top when narrow */}
        {!canMultiColumn && (
          <StatsPanels
            summary={summary}
            agentStats={agentStats}
            showInitialLoading={showInitialLoading}
            isNarrow={isNarrow}
          />
        )}

        {/* Activity Feed - primary focus */}
        <Box flexDirection="column" flexGrow={1} marginRight={canMultiColumn ? 1 : 0}>
          {showInitialLoading ? (
            <LoadingIndicator message="Loading activity..." />
          ) : (
            <ActivityFeed
              maxEntries={isMedium || isWide ? 15 : 8}
              compact={!isWide}
              showFilterHints={canMultiColumn}
            />
          )}
        </Box>

        {/* Stats & Health panels - side column when space allows (medium+) */}
        {canMultiColumn && (
          <Box
            flexDirection="column"
            width={Math.min(32, Math.max(26, Math.floor((terminalWidth - 4) * 0.28)))}
          >
            <StatsPanels
              summary={summary}
              agentStats={agentStats}
              showInitialLoading={showInitialLoading}
              isNarrow={isNarrow}
            />
          </Box>
        )}
      </Box>

      {/* Footer with keyboard hints - Issue #1514: use drawer navigation (#1467) */}
      <Footer
        hints={[
          { key: 'Tab', label: 'views' },
          { key: 'j/k', label: 'drawer' },
          { key: 'Enter', label: 'select' },
          { key: 'r', label: 'refresh' },
          { key: 'q', label: 'quit' },
        ]}
      />
    </Box>
  );
}

interface HeaderProps {
  workspaceName: string;
  isLoading: boolean;
  lastRefresh: Date | null;
}

/**
 * Memoized header - only re-renders when props change
 */
const Header = memo(function Header({ workspaceName, isLoading, lastRefresh }: HeaderProps) {
  const { theme } = useTheme();
  const refreshText = lastRefresh ? `Updated ${formatRelativeTime(lastRefresh)}` : '';

  return (
    <Box marginBottom={1}>
      <Text bold color={theme.colors.primary}>
        bc
      </Text>
      <Text> · </Text>
      <Text>{workspaceName}</Text>
      <Box flexGrow={1} />
      {isLoading ? (
        <Text color={theme.colors.warning}>↻ refreshing...</Text>
      ) : (
        <Text dimColor>{refreshText}</Text>
      )}
    </Box>
  );
});

interface SummaryCardsProps {
  total: number;
  active: number;
  working: number;
  idle: number;
  stuck: number;
  errorCount: number;
  isNarrow: boolean;
}

/**
 * Memoized summary cards - only re-renders when counts change
 * #1352: Uses inline text at <100 cols to prevent border overlap
 */
const SummaryCards = memo(function SummaryCards({
  total,
  active,
  working,
  idle,
  stuck,
  errorCount,
  isNarrow,
}: SummaryCardsProps) {
  const { theme } = useTheme();

  if (isNarrow) {
    return (
      <Box marginBottom={1}>
        <Text>{total} agents</Text>
        <Text> · </Text>
        <Text color={theme.colors.info}>
          {STATUS_SYMBOLS.working} {working} working
        </Text>
        <Text> · </Text>
        <Text color={theme.colors.textMuted}>
          {STATUS_SYMBOLS.idle} {idle} idle
        </Text>
        {stuck > 0 && (
          <>
            <Text> · </Text>
            <Text color={theme.colors.warning}>
              {STATUS_SYMBOLS.warning} {stuck} stuck
            </Text>
          </>
        )}
        {errorCount > 0 && (
          <>
            <Text> · </Text>
            <Text color={theme.colors.error}>
              {STATUS_SYMBOLS.error} {errorCount} error
            </Text>
          </>
        )}
      </Box>
    );
  }

  return (
    <Box flexWrap="wrap" marginBottom={1}>
      <MetricCard value={total} label="Total" />
      <MetricCard value={active} label="Active" color={theme.colors.success} />
      <MetricCard value={working} label="Working" color={theme.colors.info} />
      <MetricCard value={idle} label="Idle" color={theme.colors.textMuted} />
      {stuck > 0 && <MetricCard value={stuck} label="Stuck" color={theme.colors.warning} />}
      {errorCount > 0 && <MetricCard value={errorCount} label="Error" color={theme.colors.error} />}
    </Box>
  );
});

interface SystemHealthPanelProps {
  working: number;
  idle: number;
  stuck: number;
  errorCount: number;
  total: number;
  isNarrow: boolean;
}

/**
 * System Health panel - shows agent state distribution
 * #1352: Uses simple text header at <100 cols to prevent border overlap
 */
const SystemHealthPanel = memo(function SystemHealthPanel({
  working,
  idle,
  stuck,
  errorCount,
  total,
  isNarrow,
}: SystemHealthPanelProps) {
  const healthyCount = working + idle;
  const unhealthyCount = stuck + errorCount;
  const healthPercent = total > 0 ? Math.round((healthyCount / total) * 100) : 100;
  const healthColor =
    healthPercent >= 80
      ? HEALTH_COLORS.healthy
      : healthPercent >= 50
        ? HEALTH_COLORS.warning
        : HEALTH_COLORS.critical;

  // #1352: Compact borderless layout for narrow terminals
  // #1779: Use full text labels for accessibility (screen reader support)
  if (isNarrow) {
    return (
      <Box flexDirection="column" marginBottom={1}>
        <Box>
          <Text bold dimColor>
            Health:{' '}
          </Text>
          <Text color={healthColor} bold>
            {healthPercent}%
          </Text>
          <Text> · </Text>
          <Text color={STATUS_COLORS.working}>●</Text>
          <Text> {working} working</Text>
          <Text> · </Text>
          <Text color={STATUS_COLORS.idle}>○</Text>
          <Text> {idle} idle</Text>
          {stuck > 0 && (
            <>
              <Text> · </Text>
              <Text color={STATUS_COLORS.warning}>⚠</Text>
              <Text> {stuck} stuck</Text>
            </>
          )}
        </Box>
      </Box>
    );
  }

  // Standard bordered Panel for wider terminals
  return (
    <Panel title="Health">
      <Box flexDirection="column">
        {/* Health percentage - Box layout to prevent truncation garbling */}
        <Box>
          <Text color={healthColor} bold>
            {healthPercent}%
          </Text>
          <Text dimColor> healthy</Text>
        </Box>
        <Box marginTop={1} flexDirection="column">
          {/* Working agents - consistent colors */}
          <Box>
            <Text color={STATUS_COLORS.working}>●</Text>
            <Text> Working: {working}</Text>
          </Box>
          <Box>
            <Text color={STATUS_COLORS.idle}>●</Text>
            <Text> Idle: {idle}</Text>
          </Box>
          {stuck > 0 && (
            <Box>
              <Text color={STATUS_COLORS.warning}>●</Text>
              <Text> Stuck: {stuck}</Text>
            </Box>
          )}
          {errorCount > 0 && (
            <Box>
              <Text color={STATUS_COLORS.error}>●</Text>
              <Text> Error: {errorCount}</Text>
            </Box>
          )}
        </Box>
        {unhealthyCount > 0 && (
          <Text color={STATUS_COLORS.warning} dimColor>
            ⚠ {unhealthyCount} agent{unhealthyCount > 1 ? 's' : ''} need attention
          </Text>
        )}
      </Box>
    </Panel>
  );
});

interface CostPanelProps {
  totalCostUSD: number;
  inputTokens: number;
  outputTokens: number;
  budgetUSD?: number;
  burnRate?: number;
  topAgents?: { name: string; cost: number }[];
  isNarrow: boolean;
}

/**
 * Cost panel with budget progress bar, burn rate, and top agents
 * #1882: Enhanced with ccusage integration fields
 * #1220: Added symbols and text labels for colorblind accessibility
 * #1352: Uses compact inline layout at <100 cols
 */
const CostPanel = memo(function CostPanel({
  totalCostUSD,
  inputTokens: _inputTokens,
  outputTokens: _outputTokens,
  budgetUSD = 10.0,
  burnRate = 0,
  topAgents = [],
  isNarrow,
}: CostPanelProps) {
  const budgetPercent = Math.min(100, Math.round((totalCostUSD / budgetUSD) * 100));
  // Responsive bar width: smaller on narrow terminals
  const barWidth = isNarrow ? 10 : 15;
  const filledWidth = Math.round((budgetPercent / 100) * barWidth);
  const emptyWidth = barWidth - filledWidth;

  // Determine cost status for symbol and label (#1220 colorblind support)
  const costStatus: CostStatus =
    budgetPercent >= 90 ? 'critical' : budgetPercent >= 75 ? 'warning' : 'normal';
  const { color: barColor, symbol: costSymbol } = getCostIndicator(costStatus);

  // #1352: Compact inline layout for narrow terminals
  if (isNarrow) {
    return (
      <Box marginBottom={1}>
        <Text bold dimColor>
          Cost:{' '}
        </Text>
        <Text bold color={barColor}>
          ${totalCostUSD.toFixed(2)}
        </Text>
        <Text dimColor>/${budgetUSD.toFixed(2)} </Text>
        <Text color={barColor}>{'█'.repeat(filledWidth)}</Text>
        <Text dimColor>{'░'.repeat(emptyWidth)}</Text>
        <Text> {budgetPercent}%</Text>
        <Text color={barColor}> {costSymbol}</Text>
      </Box>
    );
  }

  // Standard bordered Panel for wider terminals
  return (
    <Panel title="Cost">
      <Box flexDirection="column">
        {/* Line 1: Total + burn rate (show placeholder when no data yet) */}
        <Box>
          <Text bold color={barColor}>
            {totalCostUSD > 0 ? `$${totalCostUSD.toFixed(2)}` : '$—'}
          </Text>
          {burnRate > 0 && (
            <Text dimColor>
              {' '}
              {costSymbol} ${burnRate.toFixed(2)}/hr
            </Text>
          )}
        </Box>
        {/* Line 2: Budget bar */}
        <Box marginTop={1}>
          <Text color={barColor}>{'█'.repeat(filledWidth)}</Text>
          <Text dimColor>{'░'.repeat(emptyWidth)}</Text>
          <Text> {budgetPercent}%</Text>
        </Box>
        {/* Lines 3-4: Top agents */}
        {topAgents.length > 0 && (
          <Box marginTop={1} flexDirection="column">
            {/* Show top agents 2 per line */}
            <Box>
              {topAgents.slice(0, 2).map((a, i) => (
                <Text key={a.name} dimColor>
                  {i > 0 ? '  ' : ''}
                  {a.name} ${Math.round(a.cost)}
                </Text>
              ))}
            </Box>
            {topAgents.length > 2 && (
              <Box>
                {topAgents.slice(2, 4).map((a, i) => (
                  <Text key={a.name} dimColor>
                    {i > 0 ? '  ' : ''}
                    {a.name} ${Math.round(a.cost)}
                  </Text>
                ))}
                {topAgents.length > 4 && <Text dimColor> +{topAgents.length - 4} more</Text>}
              </Box>
            )}
          </Box>
        )}
      </Box>
    </Panel>
  );
});

interface AgentStatsPanelProps {
  stats: {
    byState: Record<string, number>;
    byRole: Record<string, number>;
  };
  isNarrow: boolean;
}

/**
 * Memoized agent stats panel - only re-renders when stats change
 * Fixed: Use proper Box layout to prevent text overlap (#1065)
 * #1181 fix: Use Box for inline layout instead of nested Text with wrap="truncate"
 * #1352: Uses compact inline layout at <100 cols
 */
const AgentStatsPanel = memo(function AgentStatsPanel({ stats, isNarrow }: AgentStatsPanelProps) {
  const hasRoles = Object.keys(stats.byRole).length > 0;

  if (!hasRoles) return null;

  const roleEntries = Object.entries(stats.byRole);

  // #1338: Truncate role names at narrow widths to prevent text corruption
  const MAX_ROLE_LEN = isNarrow ? 8 : 12;

  // #1352: Compact inline layout for narrow terminals
  if (isNarrow) {
    // Show top 3 roles inline: "eng: 5 · mgr: 2 · ux: 1"
    const topRoles = roleEntries.slice(0, 3);
    return (
      <Box marginBottom={1}>
        <Text bold dimColor>
          Roles:{' '}
        </Text>
        {topRoles.map(([role, count], idx) => {
          const displayRole =
            role.length > MAX_ROLE_LEN ? role.slice(0, MAX_ROLE_LEN - 1) + '…' : role;
          return (
            <Text key={role}>
              {idx > 0 && ' · '}
              {displayRole}: {count}
            </Text>
          );
        })}
        {roleEntries.length > 3 && <Text dimColor> +{roleEntries.length - 3}</Text>}
      </Box>
    );
  }

  // Standard bordered Panel for wider terminals
  return (
    <Panel title="Roles">
      <Box flexDirection="column">
        {roleEntries.map(([role, count]) => {
          // Truncate long role names to prevent overflow at narrow widths
          const displayRole =
            role.length > MAX_ROLE_LEN ? role.slice(0, MAX_ROLE_LEN - 1) + '…' : role;
          return (
            <Text key={role} wrap="truncate">
              {displayRole}: {count}
            </Text>
          );
        })}
      </Box>
    </Panel>
  );
});

interface StatsPanelsProps {
  summary: {
    working: number;
    idle: number;
    stuck: number;
    error: number;
    total: number;
    totalCostUSD: number;
    inputTokens: number;
    outputTokens: number;
    burnRate?: number;
    topAgents?: { name: string; cost: number }[];
  };
  agentStats: {
    byState: Record<string, number>;
    byRole: Record<string, number>;
  };
  showInitialLoading: boolean;
  isNarrow: boolean;
}

/**
 * StatsPanels - Consolidated stats display (#1614)
 * Reduces layout complexity by rendering panels once
 */
const StatsPanels = memo(function StatsPanels({
  summary,
  agentStats,
  showInitialLoading,
  isNarrow,
}: StatsPanelsProps) {
  if (showInitialLoading) {
    return <LoadingIndicator message="Loading stats..." />;
  }

  return (
    <>
      <SystemHealthPanel
        working={summary.working}
        idle={summary.idle}
        stuck={summary.stuck}
        errorCount={summary.error}
        total={summary.total}
        isNarrow={isNarrow}
      />
      <CostPanel
        totalCostUSD={summary.totalCostUSD}
        inputTokens={summary.inputTokens}
        outputTokens={summary.outputTokens}
        burnRate={summary.burnRate}
        topAgents={summary.topAgents}
        isNarrow={isNarrow}
      />
      <AgentStatsPanel stats={agentStats} isNarrow={isNarrow} />
    </>
  );
});

/**
 * Format date to relative time string
 */
function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSecs = Math.floor(diffMs / 1000);

  if (diffSecs < 5) return 'just now';
  if (diffSecs < 60) return `${String(diffSecs)}s ago`;

  const diffMins = Math.floor(diffSecs / 60);
  if (diffMins < 60) return `${String(diffMins)}m ago`;

  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default Dashboard;
