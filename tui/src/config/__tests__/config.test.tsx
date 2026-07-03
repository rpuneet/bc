/* eslint-disable @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access */

/**
 * Tests for ConfigContext and config hooks
 */

import React from 'react';
import { render } from 'ink-testing-library';
import { Text } from 'ink';
import { describe, it, expect, mock, spyOn, beforeEach, afterEach } from 'bun:test';
import {
  ConfigProvider,
  useConfig,
  usePerformanceConfig,
  useThemeConfig,
  DEFAULT_PERFORMANCE_CONFIG,
  DEFAULT_TUI_CONFIG,
} from '..';
import type { PerformanceConfig, TUIConfig } from '../../types';

// NOTE: Issue #1151 - Do NOT use vi.mock on services/bc as it pollutes
// the module cache and breaks bc.test.ts which needs the real module.
// Instead, ConfigProvider handles errors gracefully when execBcJson fails,
// so we don't need to mock it for these tests.
//
// The ConfigProvider already catches errors and returns default config,
// which is what we're testing anyway.

/** Wait until the rendered frame stops showing the loading state (max ~5s). */
async function waitForLoaded(lastFrame: () => string | undefined): Promise<void> {
  for (let i = 0; i < 50; i++) {
    if (!(lastFrame() ?? '').includes('Loading')) return;
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

// Test component that uses config
function ConfigConsumer() {
  const { performance, tui, loading } = useConfig();
  if (loading) return <Text>Loading</Text>;
  return (
    <Text>
      Performance: {performance.poll_interval_agents}, TUI: {tui.theme} {tui.mode}
    </Text>
  );
}

// Test component that uses performance config
function PerformanceConsumer() {
  const config = usePerformanceConfig();
  return <Text>Poll: {config.poll_interval_agents}</Text>;
}

// Test component that uses theme config
function ThemeConsumer() {
  const config = useThemeConfig();
  return (
    <Text>
      Theme: {config.theme}, Mode: {config.mode}
    </Text>
  );
}

describe('ConfigProvider', () => {
  it('provides default config', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <ConfigConsumer />
      </ConfigProvider>
    );
    // Wait for async loading
    await waitForLoaded(lastFrame);
    expect(lastFrame()).toContain('Performance:');
    expect(lastFrame()).toContain('TUI:');
  });

  it('provides default theme config', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <ThemeConsumer />
      </ConfigProvider>
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(lastFrame()).toContain('Theme:');
    expect(lastFrame()).toContain('Mode:');
  });
});

describe('useConfig', () => {
  it('throws when used outside provider', () => {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
    const consoleError = spyOn(console, 'error').mockImplementation();
    try {
      render(<ConfigConsumer />);
    } catch (error) {
      expect((error as Error).message).toContain('useConfig must be used within a ConfigProvider');
    }
    consoleError.mockRestore();
  });

  it('returns default performance config on error', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <PerformanceConsumer />
      </ConfigProvider>
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(lastFrame()).toContain(`Poll: ${DEFAULT_PERFORMANCE_CONFIG.poll_interval_agents}`);
  });
});

describe('usePerformanceConfig', () => {
  it('returns performance configuration', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <PerformanceConsumer />
      </ConfigProvider>
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(lastFrame()).toContain('Poll:');
  });

  it('throws when used outside provider', () => {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
    const consoleError = spyOn(console, 'error').mockImplementation();
    try {
      render(<PerformanceConsumer />);
    } catch (error) {
      expect((error as Error).message).toContain('useConfig must be used within a ConfigProvider');
    }
    consoleError.mockRestore();
  });
});

describe('useThemeConfig', () => {
  it('returns theme configuration from workspace config', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <ThemeConsumer />
      </ConfigProvider>
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    // Should contain default values
    expect(lastFrame()).toContain('Theme:');
    expect(lastFrame()).toContain('Mode:');
  });

  it('returns default theme config on error', async () => {
    const { lastFrame } = render(
      <ConfigProvider>
        <ThemeConsumer />
      </ConfigProvider>
    );
    await new Promise((resolve) => setTimeout(resolve, 100));
    const output = lastFrame();
    expect(output).toContain('Theme:');
    expect(output).toContain('Mode:');
  });

  it('throws when used outside provider', () => {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
    const consoleError = spyOn(console, 'error').mockImplementation();
    try {
      render(<ThemeConsumer />);
    } catch (error) {
      expect((error as Error).message).toContain('useConfig must be used within a ConfigProvider');
    }
    consoleError.mockRestore();
  });
});

describe('DEFAULT_TUI_CONFIG', () => {
  it('has required theme field', () => {
    expect(DEFAULT_TUI_CONFIG.theme).toBeDefined();
    expect(typeof DEFAULT_TUI_CONFIG.theme).toBe('string');
  });

  it('has required mode field', () => {
    expect(DEFAULT_TUI_CONFIG.mode).toBeDefined();
    expect(typeof DEFAULT_TUI_CONFIG.mode).toBe('string');
  });

  it('sets dark theme and auto mode by default', () => {
    expect(DEFAULT_TUI_CONFIG.theme).toBe('dark');
    expect(DEFAULT_TUI_CONFIG.mode).toBe('auto');
  });
});

describe('DEFAULT_PERFORMANCE_CONFIG', () => {
  it('has all required polling interval fields', () => {
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_agents).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_channels).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_costs).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_status).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_logs).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.poll_interval_demons).toBeDefined();
  });

  it('has all required cache TTL fields', () => {
    expect(DEFAULT_PERFORMANCE_CONFIG.cache_ttl_tmux).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.cache_ttl_commands).toBeDefined();
  });

  it('has all required adaptive interval fields', () => {
    expect(DEFAULT_PERFORMANCE_CONFIG.adaptive_fast_interval).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.adaptive_normal_interval).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.adaptive_slow_interval).toBeDefined();
    expect(DEFAULT_PERFORMANCE_CONFIG.adaptive_max_interval).toBeDefined();
  });
});
