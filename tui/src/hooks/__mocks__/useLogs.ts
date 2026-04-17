/**
 * Manual mock for useLogs hook.
 * Consumed by src/__tests__/components/ActivityFeed.test.tsx via
 * mock.module('../../hooks/useLogs', () => useLogsMock). Must mirror
 * the real module's exports exactly (useLogs + getSeverityColor +
 * getSeverityIcon) so all importers resolve cleanly.
 */

import { vi } from 'bun:test';

const defaultData = [
  { ts: '2026-02-16T10:00:00Z', type: 'message.sent', agent: 'eng-01', message: 'Working on task' },
  { ts: '2026-02-16T10:01:00Z', type: 'agent.error', agent: 'eng-02', message: 'Build failed' },
  { ts: '2026-02-16T10:02:00Z', type: 'agent.stuck', agent: 'eng-03', message: 'Waiting for response' },
];

export const useLogs = vi.fn(() => ({
  data: defaultData,
  loading: false,
  error: null,
  severityFilter: null,
  filterBySeverity: vi.fn(),
  refresh: vi.fn(),
}));

export function getSeverityColor(type: string): string {
  const lower = type.toLowerCase();
  if (lower.includes('error') || lower.includes('fail')) return 'red';
  if (lower.includes('warn') || lower.includes('stuck')) return 'yellow';
  return 'gray';
}

export function getSeverityIcon(type: string): string {
  const lower = type.toLowerCase();
  if (lower.includes('error') || lower.includes('fail')) return '✗';
  if (lower.includes('warn') || lower.includes('stuck')) return '⚠';
  return '·';
}
