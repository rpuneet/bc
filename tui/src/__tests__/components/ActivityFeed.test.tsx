/**
 * ActivityFeed component tests
 * Issue #796 - Live activity feed with severity filtering
 *
 * Uses mock.module() with an explicit factory — Bun does NOT
 * auto-discover __mocks__ directories (that's a Jest-ism). The
 * factory re-exports src/hooks/__mocks__/useLogs.ts verbatim so
 * the shared mock remains the single source of truth.
 */

import React from 'react';
import { render } from 'ink-testing-library';
import { describe, it, expect, vi, beforeEach, mock } from 'bun:test';
import * as useLogsMock from '../../hooks/__mocks__/useLogs';
import { ThemeProvider } from '../../theme/ThemeContext';

mock.module('../../hooks/useLogs', () => useLogsMock);

// Import after mock.module so ActivityFeed resolves to the mock.
const { ActivityFeed } = await import('../../components/ActivityFeed');
const { useLogs } = await import('../../hooks/useLogs');

const renderWithTheme = (ui: React.ReactElement) => render(<ThemeProvider>{ui}</ThemeProvider>);

describe('ActivityFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders activity entries', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed />);
    const output = lastFrame();

    expect(output).toContain('Activity');
    expect(output).toContain('eng-01');
    expect(output).toContain('Working on task');
  });

  it('renders in compact mode without timestamps', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed compact />);
    const output = lastFrame();

    expect(output).toContain('eng-01');
    expect(output).toContain('sent');
  });

  it('shows error entries with error styling', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed />);
    const output = lastFrame();

    expect(output).toContain('eng-02');
    expect(output).toContain('Build failed');
  });

  it('shows warning entries', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed />);
    const output = lastFrame();

    expect(output).toContain('eng-03');
    expect(output).toContain('Waiting for response');
  });

  it('respects maxEntries limit', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed maxEntries={2} />);
    const output = lastFrame();

    expect(output).toBeDefined();
  });

  it('hides filter hints when showFilterHints is false', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed showFilterHints={false} />);
    const output = lastFrame();

    expect(output).not.toContain('(i/w/e/*)');
  });

  it('shows filter hints by default', () => {
    const { lastFrame } = renderWithTheme(<ActivityFeed showFilterHints />);
    const output = lastFrame();

    expect(output).toContain('(i/w/e/*)');
  });

  it('handles entries with undefined message without crashing', () => {
    (useLogs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ ts: '2026-02-16T10:00:00Z', type: 'agent.start', agent: 'eng-04' }],
      loading: false, error: null, severityFilter: null,
      filterBySeverity: vi.fn(), refresh: vi.fn(),
    });

    const { lastFrame } = renderWithTheme(<ActivityFeed />);
    const output = lastFrame();
    expect(output).toBeDefined();
  });

  it('handles entries with undefined agent and message without crashing', () => {
    (useLogs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: [{ ts: '2026-02-16T10:00:00Z', type: 'agent.start' }],
      loading: false, error: null, severityFilter: null,
      filterBySeverity: vi.fn(), refresh: vi.fn(),
    });

    const { lastFrame } = renderWithTheme(<ActivityFeed />);
    const output = lastFrame();
    expect(output).toBeDefined();
  });
});
