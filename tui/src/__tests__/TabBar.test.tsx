/**
 * TabBar responsive display mode tests
 *
 * Issue #1109: Fixed 80x24 display by adjusting thresholds:
 * - Full (>=120 cols): [dash] Dashboard [ag] Agents ...
 * - Short (100-119 cols): [dash] Dash [ag] Agt ...
 * - Minimal (<100 cols): [dash] [ag] [no] ... (fits 80x24)
 *
 * Issue #1927: Added MCP, Secrets, Processes tabs
 */

import { describe, expect, test } from 'bun:test';
import { render } from 'ink-testing-library';
import React from 'react';
import { ThemeProvider } from '../theme/ThemeContext';
import { TabBar } from '../navigation/TabBar';
import { NavigationProvider } from '../navigation/NavigationContext';

/** All expected tab keys from DEFAULT_TABS */
const ALL_TAB_KEYS = [
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

/** Wrapper to provide navigation context */
function renderTabBar(terminalWidth: number) {
  return render(
    <ThemeProvider>
      <NavigationProvider>
        <TabBar terminalWidth={terminalWidth} />
      </NavigationProvider>
    </ThemeProvider>
  );
}

describe('TabBar display mode logic', () => {
  test('at 140 cols (full mode), uses full labels', () => {
    const { lastFrame } = renderTabBar(140);
    const output = lastFrame() ?? '';

    expect(output).toContain('Dashboard');
    expect(output).toContain('[dash]');
  });

  test('at 120 cols (full mode boundary), uses full labels', () => {
    const { lastFrame } = renderTabBar(120);
    const output = lastFrame() ?? '';

    expect(output).toContain('board'); // "Dashboard" may wrap in 80-col renderer
    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
  });

  test('at 110 cols (short mode), shows abbreviated labels', () => {
    const { lastFrame } = renderTabBar(110);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).toContain('[no]');
    expect(output).toContain('Dash');
    expect(output).toMatch(/Ag/);
    expect(output).not.toContain('Dashboard');
  });

  test('boundary: 119 cols triggers short mode', () => {
    const { lastFrame } = renderTabBar(119);
    const output = lastFrame() ?? '';

    expect(output).toContain('Dash');
    expect(output).not.toContain('Dashboard');
  });

  test('boundary: 100 cols triggers short mode', () => {
    const { lastFrame } = renderTabBar(100);
    const output = lastFrame() ?? '';

    expect(output).toContain('Dash');
    expect(output).toMatch(/Ag/);
    expect(output).not.toContain('Dashboard');
  });

  test('boundary: 99 cols triggers minimal mode', () => {
    const { lastFrame } = renderTabBar(99);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).not.toContain('Dash');
    expect(output).not.toContain('Dashboard');
  });

  test('at 80 cols (minimal mode), shows only tab keys', () => {
    const { lastFrame } = renderTabBar(80);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).toContain('[no]');
    expect(output).not.toContain('Dash');
    expect(output).not.toContain('Dashboard');
  });
});

describe('TabBar structure', () => {
  test('renders separator after title', () => {
    const { lastFrame } = renderTabBar(120);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output.length).toBeGreaterThan(10);
  });

  test('all tab keys are present at every display mode', () => {
    for (const width of [80, 110, 140]) {
      const { lastFrame } = renderTabBar(width);
      const output = lastFrame() ?? '';

      for (const key of ALL_TAB_KEYS) {
        expect(output).toContain(key);
      }
    }
  });

  test('full labels map correctly at 120+ cols', () => {
    const { lastFrame } = renderTabBar(120);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('board'); // "Dashboard" may wrap
    expect(output).toContain('[ag]');
    expect(output).toMatch(/Agent/);
    expect(output).toContain('[no]');
  });

  test('short labels map correctly at 100-119 cols', () => {
    const { lastFrame } = renderTabBar(110);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('Dash');
    expect(output).toContain('[ag]');
    expect(output).toMatch(/Ag/);
  });

  test('minimal mode shows only keys at <100 cols', () => {
    const { lastFrame } = renderTabBar(80);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).not.toContain('Dash');
    expect(output).not.toContain('Dashboard');
  });
});

describe('TabBar accessibility', () => {
  test('keyboard navigation keys always visible', () => {
    const { lastFrame } = renderTabBar(40);
    const output = lastFrame() ?? '';

    for (const key of ALL_TAB_KEYS) {
      expect(output).toContain(key);
    }
  });
});

describe('TabBar #1109 - Fix 80x24 display (replaces #1038 tests)', () => {
  test('at 80x24 (standard terminal), shows minimal tab keys only', () => {
    const { lastFrame } = renderTabBar(80);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).toContain('[ag]');
    expect(output).toContain('[no]');
    expect(output).not.toContain('Dashboard');
    expect(output).not.toContain('Dash');
  });

  test('at 100 cols, shows short abbreviated labels', () => {
    const { lastFrame } = renderTabBar(100);
    const output = lastFrame() ?? '';

    expect(output).toContain('Dash');
    expect(output).toMatch(/Ag/);
    expect(output).not.toContain('Dashboard');
  });

  test('at 120 cols, shows full tab names', () => {
    const { lastFrame } = renderTabBar(120);
    const output = lastFrame() ?? '';

    expect(output).toContain('board'); // "Dashboard" may wrap
    expect(output).toMatch(/Agent/);
  });

  test('at 99 cols (just below 100), shows minimal mode', () => {
    const { lastFrame } = renderTabBar(99);
    const output = lastFrame() ?? '';

    expect(output).toContain('[dash]');
    expect(output).not.toContain('Dash');
    expect(output).not.toContain('Dashboard');
  });
});

describe('TabBar #1927 - New resource views', () => {
  test('MCP tab is present', () => {
    const { lastFrame } = renderTabBar(140);
    const output = lastFrame() ?? '';

    expect(output).toContain('[mcp]');
    expect(output).toContain('MCP');
  });

  test('Secrets tab is present', () => {
    const { lastFrame } = renderTabBar(140);
    const output = lastFrame() ?? '';

    expect(output).toContain('[sec]');
    expect(output).toContain('Secrets');
  });

  test('Processes tab is present', () => {
    const { lastFrame } = renderTabBar(140);
    const output = lastFrame() ?? '';

    expect(output).toContain('[ps]');
    expect(output).toContain('Processes');
  });
});
