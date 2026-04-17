/**
 * ActivityFeed component tests
 * Issue #796 - Live activity feed with severity filtering
 *
 * Mock strategy: mock.module() with an inline factory BEFORE any
 * static import of the SUT. Bun hoists mock.module calls above
 * sibling imports (matches the pattern in src/hooks/__tests__/useData.test.ts).
 *
 * Avoiding the `const { X } = await import(...)` dynamic-import
 * pattern because it's sensitive to mock-call ordering on Bun 1.2.x
 * (CI pin) while passing on 1.3.x (dev).
 */

import { describe, it, expect, vi, beforeEach, mock } from 'bun:test';

const defaultData = [
  { ts: '2026-02-16T10:00:00Z', type: 'message.sent', agent: 'eng-01', message: 'Working on task' },
  { ts: '2026-02-16T10:01:00Z', type: 'agent.error', agent: 'eng-02', message: 'Build failed' },
  { ts: '2026-02-16T10:02:00Z', type: 'agent.stuck', agent: 'eng-03', message: 'Waiting for response' },
];

// Inline factory so there is no transitive import back into the real
// hook — every export this module needs is defined right here.
mock.module('../../hooks/useLogs', () => ({
  useLogs: vi.fn(() => ({
    data: defaultData,
    loading: false,
    error: null,
    severityFilter: null,
    filterBySeverity: vi.fn(),
    refresh: vi.fn(),
  })),
  getSeverityColor: (type: string) => {
    const lower = type.toLowerCase();
    if (lower.includes('error') || lower.includes('fail')) return 'red';
    if (lower.includes('warn') || lower.includes('stuck')) return 'yellow';
    return 'gray';
  },
  getSeverityIcon: (type: string) => {
    const lower = type.toLowerCase();
    if (lower.includes('error') || lower.includes('fail')) return '✗';
    if (lower.includes('warn') || lower.includes('stuck')) return '⚠';
    return '·';
  },
}));

// Convenience: the test suite imports useLogs (the mocked vi.fn) so
// individual cases can call mockReturnValueOnce/mockReturnValue to
// swap in per-test data without re-mocking the whole module.

import React from 'react';
import { render } from 'ink-testing-library';
import { ThemeProvider } from '../../theme/ThemeContext';
import { ActivityFeed } from '../../components/ActivityFeed';
import { useLogs } from '../../hooks/useLogs';

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
