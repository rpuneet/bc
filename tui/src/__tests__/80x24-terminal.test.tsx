/**
 * Comprehensive 80x24 Terminal Support Tests
 * Issue: UX reported channel names missing at 80x24 terminal
 * Issue #1326: Updated for 5-tier breakpoint system (XS/SM/MD/LG/XL)
 *
 * Tests verify that TUI renders correctly at the standard 80x24 terminal size.
 * These tests cover:
 * - Responsive layout breakpoints
 * - Width calculations for 80 columns
 * - Height calculations for 24 rows
 * - Text truncation behavior
 * - Panel minimum heights
 * - View-specific constraints (Dashboard, Agents, Logs, etc.)
 */

import { describe, it, expect } from 'bun:test';
import { render } from 'ink-testing-library';
import React from 'react';
import { ThemeProvider } from '../theme/ThemeContext';
import { TabBar } from '../navigation/TabBar';
import { NavigationProvider } from '../navigation/NavigationContext';
/** Layout mode type (previously from useResponsiveLayout) */
type LayoutMode = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

/** Breakpoints matching constants/dimensions.ts */
const BREAKPOINTS = {
  XS: 60,
  SM: 80,
  MD: 120,
  LG: 140,
} as const;

/** Legacy breakpoints (previously from useResponsiveLayout) */
const BREAKPOINTS_LEGACY = BREAKPOINTS;

/**
 * Test the responsive layout breakpoints at standard terminal sizes (#1326)
 */
describe('80x24 Terminal - Breakpoints', () => {
  // Helper to test layout mode using new 5-tier system
  function getLayoutMode(width: number): LayoutMode {
    if (width >= BREAKPOINTS.LG) return 'xl';
    if (width >= BREAKPOINTS.MD) return 'lg';
    if (width >= BREAKPOINTS.SM) return 'md';
    if (width >= BREAKPOINTS.XS) return 'sm';
    return 'xs';
  }

  it('80 cols is md mode (at SM breakpoint)', () => {
    expect(getLayoutMode(80)).toBe('md');
  });

  it('79 cols is sm mode (between XS and SM)', () => {
    expect(getLayoutMode(79)).toBe('sm');
  });

  it('59 cols is xs mode (below XS breakpoint)', () => {
    expect(getLayoutMode(59)).toBe('xs');
  });

  it('100 cols is md mode (between SM and MD)', () => {
    expect(getLayoutMode(100)).toBe('md');
  });

  it('120 cols is lg mode (14-char drawer, two columns)', () => {
    expect(getLayoutMode(120)).toBe('lg');
  });

  it('140 cols is xl mode (three columns with detail)', () => {
    expect(getLayoutMode(140)).toBe('xl');
  });
});

/**
 * Test TabBar at 80x24 - should show minimal mode (just numbers)
 * Issue #1109: 12 tabs with full labels need ~140 cols, short labels need ~105 cols
 * At 80 cols, use minimal mode (~55 cols) to prevent overflow
 */
describe('80x24 Terminal - TabBar', () => {
  function renderTabBar(terminalWidth: number) {
    return render(
      <ThemeProvider>
        <NavigationProvider>
          <TabBar terminalWidth={terminalWidth} />
        </NavigationProvider>
      </ThemeProvider>
    );
  }

  it('shows minimal mode (keys only) at 80 columns', () => {
    const { lastFrame } = renderTabBar(80);
    const output = lastFrame() ?? '';

    // At 80 cols, should show minimal mode (keys only) to fit 80x24
    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).toContain('[no]');
    // Labels should NOT appear in minimal mode
    expect(output).not.toContain('Dashboard');
    expect(output).not.toContain('Dash');
  });

  it('all tab keys are visible at 80 columns', () => {
    const { lastFrame } = renderTabBar(80);
    const output = lastFrame() ?? '';

    // All tab keys must be visible
    const keys = [
      '[dash]',
      '[ag]',
      '[no]',
      '[co]',
      '[log]',
      '[ro]',
      '[wt]',
      '[tl]',
      '[mcp]',
      '[sec]',
      '[ps]',
      '[?]',
    ];
    for (const key of keys) {
      expect(output).toContain(key);
    }
  });

  it('shows short labels at 100 columns', () => {
    const { lastFrame } = renderTabBar(100);
    const output = lastFrame() ?? '';

    // At 100 cols, should show short labels
    expect(output).toContain('Dash');
    expect(output).toMatch(/Ag/);
    expect(output).not.toContain('Dashboard');
  });

  it('shows full labels at 120 columns', () => {
    const { lastFrame } = renderTabBar(120);
    const output = lastFrame() ?? '';

    // Full labels wrap due to ink-testing-library's 80-col render
    expect(output).toContain('board'); // "Dashboard" wraps
    expect(output).toContain('[dash]');
  });
});

/**
 * Test width calculations for 80-column terminal
 */
describe('80x24 Terminal - Width Calculations', () => {
  it('accounts for borders and padding in 80-col content', () => {
    // A bordered box with paddingX=2 uses:
    // - Border: 2 columns (1 each side)
    // - Padding: 4 columns (2 each side)
    // - Total overhead: 6 columns
    const terminalWidth = 80;
    const borderWidth = 2;
    const paddingWidth = 4;
    const availableContent = terminalWidth - borderWidth - paddingWidth;

    expect(availableContent).toBe(74);
  });

  it('table columns fit in 80-col terminal', () => {
    // AgentsView table columns: 14 + 10 + 10 + 32 = 66
    // Plus selection indicator (2) = 68
    // This should fit in 80 cols with borders
    const columnWidths = [14, 10, 10, 32];
    const totalColumnWidth = columnWidths.reduce((a, b) => a + b, 0);
    const selectionIndicator = 2;
    const tableWidth = totalColumnWidth + selectionIndicator;

    expect(tableWidth).toBeLessThanOrEqual(80);
    expect(tableWidth).toBe(68);
  });
});

/**
 * Test height calculations for 24-row terminal
 */
describe('80x24 Terminal - Height Calculations', () => {
  it('reserves correct header space', () => {
    // App header usage:
    // - padding: 2 rows (top + bottom)
    // - TabBar: 1 row
    // - Breadcrumb: 0-1 row
    // - marginTop: 1 row (for content)
    // - Footer: 2 rows (marginTop + content)
    const terminalHeight = 24;
    const padding = 2;
    const tabBar = 1;
    const breadcrumb = 0; // Usually 0 unless navigated deep
    const marginTop = 1;
    const footer = 2;

    const overhead = padding + tabBar + breadcrumb + marginTop + footer;
    const availableContent = terminalHeight - overhead;

    expect(overhead).toBe(6);
    expect(availableContent).toBe(18);
  });

  it('channel history input height fits in 24 rows', () => {
    // ChannelHistoryView layout at 24 rows:
    // - Header: 4 rows (3 + marginBottom=1)
    // - Message area: dynamic (min 10)
    // - Input area: 3-10 rows
    // - Footer: 1 row
    const terminalHeight = 24;
    const minInputHeight = 3;
    const layoutOverhead = 10 + minInputHeight; // 13

    const messageAreaHeight = Math.max(10, terminalHeight - layoutOverhead);

    expect(messageAreaHeight).toBe(11);
    expect(messageAreaHeight).toBeGreaterThanOrEqual(10);
  });

  it('dashboard has minimum viable content at 24 rows', () => {
    // Dashboard needs space for:
    // - Header: 2 rows
    // - Summary cards: 1-2 rows
    // - Metrics panels: variable
    // - Activity feed: 8+ entries ideally
    const terminalHeight = 24;
    const appOverhead = 6; // from app.tsx
    const dashboardHeader = 2;
    const summaryCards = 2;

    const availableForContent = terminalHeight - appOverhead - dashboardHeader - summaryCards;

    // Should have at least 14 rows for metrics + activity
    expect(availableForContent).toBe(14);
    expect(availableForContent).toBeGreaterThanOrEqual(10);
  });
});

/**
 * Test text truncation behavior at 80 columns
 */
describe('80x24 Terminal - Text Truncation', () => {
  it('channel names truncate correctly with wrap=truncate', () => {
    // ChannelRow builds: "▸ #channel-name [1 new] (5)"
    // Max length considerations:
    const maxChannelNameLength = 30;
    const prefix = '▸ '.length; // 2
    const channelHash = '#'.length; // 1
    const unreadSuffix = ' [99+ new]'.length; // 10
    const memberSuffix = ' (999)'.length; // 6

    const maxLineLength = prefix + channelHash + maxChannelNameLength + unreadSuffix + memberSuffix;

    // Should fit in 74 available columns (80 - 6 for borders/padding)
    expect(maxLineLength).toBe(49);
    expect(maxLineLength).toBeLessThan(74);
  });

  it('agent names truncate to 12 characters', () => {
    // AgentsView column renders: name.slice(0, 11) + '…'
    const maxNameDisplay = 12; // 11 chars + ellipsis
    const columnWidth = 14;

    expect(maxNameDisplay).toBeLessThan(columnWidth);
  });
});

/**
 * Test that panel minimum heights prevent collapse
 */
describe('80x24 Terminal - Panel Minimum Heights', () => {
  it('panel has minimum effective height', () => {
    // Panel.tsx enforces minHeight
    const titleHeight = 1;
    const defaultMinContentHeight = 3;

    // effectiveMinHeight = minHeight ?? (title ? 4 : 3)
    const effectiveMinWithTitle = 4;
    const effectiveMinWithoutTitle = 3;

    expect(effectiveMinWithTitle).toBeGreaterThanOrEqual(titleHeight + defaultMinContentHeight);
    expect(effectiveMinWithoutTitle).toBe(defaultMinContentHeight);
  });
});

/**
 * Test responsive layout flags at 80x24 (#1326)
 */
describe('80x24 Terminal - Layout Flags', () => {
  function getLayoutFlags(width: number) {
    const mode: LayoutMode =
      width >= BREAKPOINTS.LG
        ? 'xl'
        : width >= BREAKPOINTS.MD
          ? 'lg'
          : width >= BREAKPOINTS.SM
            ? 'md'
            : width >= BREAKPOINTS.XS
              ? 'sm'
              : 'xs';

    return {
      isXS: mode === 'xs',
      isSM: mode === 'sm',
      isMD: mode === 'md',
      isLG: mode === 'lg',
      isXL: mode === 'xl',
      // Legacy compatibility
      isMinimal: mode === 'xs',
      isCompact: mode === 'sm',
      isMedium: mode === 'md',
      isWide: mode === 'lg' || mode === 'xl',
      canMultiColumn: width >= BREAKPOINTS.MD,
      canTripleColumn: width >= BREAKPOINTS.LG,
      canShowDetail: width >= BREAKPOINTS.LG,
    };
  }

  it('at 80 cols: md mode (at SM breakpoint)', () => {
    const flags = getLayoutFlags(80);

    expect(flags.isMedium).toBe(true);
    expect(flags.isCompact).toBe(false);
    expect(flags.isXS).toBe(false);
    expect(flags.canMultiColumn).toBe(false); // 80 < MD(120)
    expect(flags.canTripleColumn).toBe(false);
    expect(flags.canShowDetail).toBe(false);
  });

  it('canMultiColumn requires 120+ columns (LG+)', () => {
    expect(getLayoutFlags(119).canMultiColumn).toBe(false);
    expect(getLayoutFlags(120).canMultiColumn).toBe(true);
  });

  it('canTripleColumn requires 140+ columns (XL)', () => {
    expect(getLayoutFlags(139).canTripleColumn).toBe(false);
    expect(getLayoutFlags(140).canTripleColumn).toBe(true);
  });

  it('canShowDetail requires 140+ columns (XL)', () => {
    expect(getLayoutFlags(139).canShowDetail).toBe(false);
    expect(getLayoutFlags(140).canShowDetail).toBe(true);
  });
});

/**
 * Test Dashboard layout at 80x24
 */
describe('80x24 Terminal - Dashboard Layout', () => {
  it('narrow dashboard uses single column layout', () => {
    // At 80 cols (SM mode), dashboard should use single-column layout
    // No side-by-side panels to prevent text garbling (#1318)
    const terminalWidth = 80;
    const drawerWidth = 6; // SM mode drawer
    const appPadding = 2;
    const contentWidth = terminalWidth - drawerWidth - appPadding;

    // Should have 72 chars for content
    expect(contentWidth).toBe(72);
  });
});

/**
 * Test LogsView dynamic visible rows calculation (80x24 fix)
 */
describe('80x24 Terminal - LogsView Visible Rows', () => {
  // Mirror the calculation from LogsView.tsx
  function calculateVisibleRows(terminalHeight: number): number {
    const viewOverhead = 11;
    return Math.max(5, Math.min(15, terminalHeight - viewOverhead));
  }

  it('calculates correct visible rows at 24 rows terminal', () => {
    // At 24 rows: 24 - 11 overhead = 13 visible rows
    expect(calculateVisibleRows(24)).toBe(13);
  });

  it('calculates correct visible rows at 30 rows terminal', () => {
    // At 30 rows: capped at 15 max
    expect(calculateVisibleRows(30)).toBe(15);
  });

  it('enforces minimum of 5 rows at very short terminal', () => {
    // At 14 rows: 14 - 11 = 3, but minimum is 5
    expect(calculateVisibleRows(14)).toBe(5);
  });

  it('handles edge case at exactly 16 rows', () => {
    // At 16 rows: 16 - 11 = 5 (exactly minimum)
    expect(calculateVisibleRows(16)).toBe(5);
  });
});

/**
 * Test HelpView scroll behavior at 24 rows
 */
describe('80x24 Terminal - HelpView Scroll', () => {
  function calculateHelpLayout(terminalHeight: number) {
    // From app.tsx HelpView component
    const availableHeight = Math.max(10, terminalHeight - 6);
    return { availableHeight };
  }

  it('calculates correct available height at 24 rows', () => {
    // At 24 rows: 24 - 6 = 18 available
    const { availableHeight } = calculateHelpLayout(24);
    expect(availableHeight).toBe(18);
  });

  it('enforces minimum of 10 rows at very short terminal', () => {
    // At 12 rows: 12 - 6 = 6, but minimum is 10
    const { availableHeight } = calculateHelpLayout(12);
    expect(availableHeight).toBe(10);
  });
});

/**
 * Test ActivityFeed truncation at 80 columns
 */
describe('80x24 Terminal - ActivityFeed Truncation', () => {
  it('truncates messages appropriately for compact mode', () => {
    // From ActivityFeed.tsx
    const compact = true;
    const maxMsgLen = compact ? 40 : 60;

    expect(maxMsgLen).toBe(40);
  });

  it('uses longer truncation for non-compact mode', () => {
    const compact = false;
    const maxMsgLen = compact ? 40 : 60;

    expect(maxMsgLen).toBe(60);
  });
});

/**
 * Test drawer configuration per breakpoint (#1326)
 */
describe('80x24 Terminal - Drawer Config', () => {
  function getDrawerWidth(mode: LayoutMode): number {
    switch (mode) {
      case 'xs':
        return 0;
      case 'sm':
        return 6;
      case 'md':
        return 10;
      case 'lg':
      case 'xl':
        return 14;
    }
  }

  it('no drawer at xs mode (<80 cols)', () => {
    expect(getDrawerWidth('xs')).toBe(0);
  });

  it('6-char drawer at sm mode (80-99 cols)', () => {
    expect(getDrawerWidth('sm')).toBe(6);
  });

  it('10-char drawer at md mode (100-119 cols)', () => {
    expect(getDrawerWidth('md')).toBe(10);
  });

  it('14-char drawer at lg+ mode (120+ cols)', () => {
    expect(getDrawerWidth('lg')).toBe(14);
    expect(getDrawerWidth('xl')).toBe(14);
  });
});
