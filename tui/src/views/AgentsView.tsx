/**
 * AgentsView - Agent list and management view
 * Refactored from 614 lines to ~250 lines (#1592)
 *
 * Components extracted to ./agents/:
 * - AgentCard, AgentGroupHeader, AgentList
 * - AgentActions, AgentPeekPanel, AgentConfirmDialog
 * - AgentSearchOverlay
 *
 * Logic extracted to hooks/:
 * - useAgentGroups
 */

import React, { useState, useCallback, useEffect, useMemo, useReducer, useRef } from 'react';
import { spawnSync } from 'child_process';
import { Box, Text, useInput } from 'ink';
import { useTheme } from '../theme';
import { isPeekHeader } from '../utils';
import { useAgents, useDebounce, useListNavigation } from '../hooks';
import { useFocus } from '../navigation/FocusContext';
import { useNavigation } from '../navigation/NavigationContext';
import { useAgentGroups } from '../hooks/useAgentGroups';
import { ErrorDisplay } from '../components/ErrorDisplay';
import { AgentDetailView } from './AgentDetailView';
import { execBc } from '../services/bc';
import type { Agent } from '../types';

// Import extracted components
import { Footer } from '../components/Footer';
import {
  AgentList,
  AgentActions,
  AgentPeekPanel,
  AgentConfirmDialog,
  AgentSearchOverlay,
  type AgentAction,
} from './agents';

type AgentsViewProps = Record<string, never>;

/** Action feedback display duration in ms */
const ACTION_FEEDBACK_DURATION = 2500;

interface ActionState {
  action: AgentAction | null;
  target: string | null;
  status: 'pending' | 'success' | 'error';
  message: string;
}

// #1601: Consolidated UI state with useReducer
// #1743: Navigation state moved to useListNavigation
interface UIState {
  showDetail: boolean;
  confirmAction: AgentAction | null;
  peekOutput: string[] | null;
  peekLoading: boolean;
  groupedView: boolean;
  collapsedRoles: Set<string>;
}

type UIAction =
  | { type: 'SHOW_DETAIL' }
  | { type: 'HIDE_DETAIL' }
  | { type: 'SET_CONFIRM_ACTION'; action: AgentAction | null }
  | { type: 'SET_PEEK_OUTPUT'; output: string[] | null }
  | { type: 'SET_PEEK_LOADING'; loading: boolean }
  | { type: 'TOGGLE_GROUPED_VIEW' }
  | { type: 'TOGGLE_ROLE_COLLAPSE'; role: string };

const initialUIState: UIState = {
  showDetail: false,
  confirmAction: null,
  peekOutput: null,
  peekLoading: false,
  groupedView: true,
  collapsedRoles: new Set(),
};

function uiReducer(state: UIState, action: UIAction): UIState {
  switch (action.type) {
    case 'SHOW_DETAIL':
      return { ...state, showDetail: true };
    case 'HIDE_DETAIL':
      return { ...state, showDetail: false };
    case 'SET_CONFIRM_ACTION':
      return { ...state, confirmAction: action.action };
    case 'SET_PEEK_OUTPUT':
      return { ...state, peekOutput: action.output };
    case 'SET_PEEK_LOADING':
      return { ...state, peekLoading: action.loading };
    case 'TOGGLE_GROUPED_VIEW':
      return { ...state, groupedView: !state.groupedView };
    case 'TOGGLE_ROLE_COLLAPSE': {
      const next = new Set(state.collapsedRoles);
      if (next.has(action.role)) {
        next.delete(action.role);
      } else {
        next.add(action.role);
      }
      return { ...state, collapsedRoles: next };
    }
    default:
      return state;
  }
}

export const AgentsView: React.FC<AgentsViewProps> = () => {
  const { theme } = useTheme();
  const { data: agents, loading, error, refresh } = useAgents();
  const isNarrow = false;

  // #1601: UI state consolidated with useReducer
  // #1743: Navigation and search state moved to useListNavigation
  const [ui, dispatch] = useReducer(uiReducer, initialUIState);
  const { showDetail, confirmAction, peekOutput, peekLoading, groupedView, collapsedRoles } = ui;

  // Action feedback state - kept separate as it's timer-managed
  const [actionState, setActionState] = useState<ActionState | null>(null);
  const feedbackTimeoutRef = useRef<ReturnType<typeof setTimeout>>();

  // #1743: Custom key handlers for agent actions
  const customKeys = useMemo(
    () => ({
      v: () => {
        dispatch({ type: 'TOGGLE_GROUPED_VIEW' });
      },
      r: () => {
        void refresh();
      },
    }),
    [refresh]
  );

  // #1842: Track visibleItems count via ref to break circular dependency
  // (useListNavigation → search.query → useAgentGroups → visibleItems.length → useListNavigation)
  const visibleItemCountRef = useRef<number | undefined>(undefined);

  // #1743: Use useListNavigation for navigation and search
  // #1842: Pass itemCount so navigation clamps to visibleItems.length (includes group headers)
  const {
    selectedIndex,
    search,
    setSelectedIndex: _setSelectedIndex, // Not used directly yet
  } = useListNavigation({
    items: agents ?? [],
    itemCount: visibleItemCountRef.current,
    disabled: showDetail || confirmAction !== null,
    enableSearch: true,
    customKeys,
  });

  // Debounce search query for filtering (issue #1602)
  const debouncedSearchQuery = useDebounce(search.query, 300);

  // Use extracted hook for grouping logic (using debounced query for performance)
  const { agentList, stateCounts, visibleItems } = useAgentGroups(
    agents ?? [],
    debouncedSearchQuery,
    groupedView,
    collapsedRoles
  );

  // #1842: Update ref so useListNavigation clamps to correct boundary on next render
  visibleItemCountRef.current = visibleItems.length;

  // Clamp selectedIndex to visible items (defensive — hook should already clamp via itemCount)
  const validIndex = Math.min(selectedIndex, Math.max(0, visibleItems.length - 1));

  // Get selected agent from visible items
  const selectedAgent = useMemo((): Agent | undefined => {
    if (validIndex < 0 || validIndex >= visibleItems.length) return undefined;
    const item = visibleItems[validIndex];
    if (item.type === 'agent') {
      return item.agent;
    }
    return undefined;
  }, [visibleItems, validIndex]);

  const { setFocus, focusedArea } = useFocus();
  const { setBreadcrumbs, clearBreadcrumbs } = useNavigation();

  // Manage focus state and breadcrumbs for nested view navigation (#1604)
  // When in search mode, set focus='input' to allow typing special chars (#1692)
  useEffect(() => {
    if (showDetail && selectedAgent) {
      setFocus('view');
      setBreadcrumbs([{ label: selectedAgent.name }]);
    } else if (search.isActive) {
      setFocus('input');
      clearBreadcrumbs();
    } else {
      setFocus('main');
      clearBreadcrumbs();
    }
  }, [showDetail, selectedAgent, search.isActive, setFocus, setBreadcrumbs, clearBreadcrumbs]);

  // Clear action feedback after delay
  const showActionFeedback = useCallback(
    (action: AgentAction, target: string, status: 'success' | 'error', message: string) => {
      if (feedbackTimeoutRef.current) clearTimeout(feedbackTimeoutRef.current);
      setActionState({ action, target, status, message });
      feedbackTimeoutRef.current = setTimeout(() => {
        setActionState(null);
      }, ACTION_FEEDBACK_DURATION);
    },
    []
  );

  // Cleanup action feedback timeout on unmount
  useEffect(() => {
    return () => {
      if (feedbackTimeoutRef.current) clearTimeout(feedbackTimeoutRef.current);
    };
  }, []);

  // Execute agent action
  const executeAction = useCallback(
    async (action: AgentAction, agentName: string, role?: string) => {
      try {
        switch (action) {
          case 'start':
            await execBc(['agent', 'start', agentName, '--role', role ?? 'engineer']);
            showActionFeedback(action, agentName, 'success', `Started ${agentName}`);
            break;
          case 'stop':
            await execBc(['agent', 'stop', agentName]);
            showActionFeedback(action, agentName, 'success', `Stopped ${agentName}`);
            break;
          case 'kill':
            await execBc(['agent', 'kill', agentName]);
            showActionFeedback(action, agentName, 'success', `Killed ${agentName}`);
            break;
          case 'attach':
            await execBc(['agent', 'attach', agentName]);
            showActionFeedback(action, agentName, 'success', `Attached to ${agentName}`);
            break;
        }
        void refresh();
      } catch (err) {
        const message = err instanceof Error ? err.message : `Failed to ${action} ${agentName}`;
        showActionFeedback(action, agentName, 'error', message);
      }
    },
    [refresh, showActionFeedback]
  );

  // Peek agent output
  const peekAgent = useCallback(async (agentName: string) => {
    dispatch({ type: 'SET_PEEK_LOADING', loading: true });
    try {
      const output = await execBc(['agent', 'peek', agentName, '--lines', '8']);
      // #1844: Strip peek headers and empty lines using shared util
      const lines = output.split('\n').filter((line: string) => line.trim() && !isPeekHeader(line));
      dispatch({ type: 'SET_PEEK_OUTPUT', output: lines.slice(-6) });
    } catch {
      dispatch({ type: 'SET_PEEK_OUTPUT', output: ['(No output available)'] });
    } finally {
      dispatch({ type: 'SET_PEEK_LOADING', loading: false });
    }
  }, []);

  // #1840: Auto-refresh peek panel every 2s while open
  useEffect(() => {
    if (!peekOutput || !selectedAgent) return undefined;
    const interval = setInterval(() => {
      void peekAgent(selectedAgent.name);
    }, 2000);
    return () => {
      clearInterval(interval);
    };
  }, [peekOutput, selectedAgent, peekAgent]);

  // #1743: Keyboard handling for special keys not covered by useListNavigation
  // The hook handles j/k/g/G navigation, / for search, c to clear search
  useInput((input, key) => {
    // #1870: Don't handle input when CommandBar/FilterBar is open
    if (focusedArea === 'command' || focusedArea === 'filter') return;
    // Let hook handle search mode
    if (search.isActive) return;
    if (showDetail) return;

    // Confirmation mode
    if (confirmAction && selectedAgent) {
      if (input === 'y' || input === 'Y') {
        void executeAction(confirmAction, selectedAgent.name, selectedAgent.role);
        dispatch({ type: 'SET_CONFIRM_ACTION', action: null });
      } else if (input === 'n' || input === 'N' || key.escape) {
        dispatch({ type: 'SET_CONFIRM_ACTION', action: null });
      }
      return;
    }

    // #1837: Enter/a = attach to tmux session (primary action)
    if (key.return || input === 'a') {
      if (validIndex >= 0 && validIndex < visibleItems.length) {
        const item = visibleItems[validIndex];
        if (item.type === 'header') {
          dispatch({ type: 'TOGGLE_ROLE_COLLAPSE', role: item.role });
          return;
        }
      }
      if (selectedAgent) {
        if (selectedAgent.state === 'stopped') {
          showActionFeedback(
            'attach' as AgentAction,
            selectedAgent.name,
            'error',
            `Agent ${selectedAgent.name} is not running`
          );
        } else {
          const bcBin = process.env.MYCEL_BIN ?? 'bc';
          spawnSync(bcBin, ['agent', 'attach', selectedAgent.name], {
            stdio: 'inherit',
          });
          void refresh();
        }
      }
      // d = open detail view (previously Enter behavior)
    } else if (input === 'd' && selectedAgent) {
      dispatch({ type: 'SHOW_DETAIL' });
    } else if (input === 'x' && selectedAgent && selectedAgent.state !== 'stopped') {
      dispatch({ type: 'SET_CONFIRM_ACTION', action: 'stop' });
    } else if (input === 'X' && selectedAgent) {
      dispatch({ type: 'SET_CONFIRM_ACTION', action: 'kill' });
    } else if (input === 'R' && selectedAgent) {
      dispatch({ type: 'SET_CONFIRM_ACTION', action: 'start' });
    } else if (input === 'p' && selectedAgent) {
      if (peekOutput) {
        dispatch({ type: 'SET_PEEK_OUTPUT', output: null });
      } else {
        void peekAgent(selectedAgent.name);
      }
    }
  });

  // Detail view
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- defensive check
  if (showDetail && selectedAgent) {
    return (
      <AgentDetailView
        agent={selectedAgent}
        onBack={() => {
          dispatch({ type: 'HIDE_DETAIL' });
        }}
      />
    );
  }

  // Search mode overlay
  if (search.isActive) {
    return <AgentSearchOverlay searchQuery={search.query} isNarrow={isNarrow} />;
  }

  if (loading && agentList.length === 0) {
    return (
      <Box padding={1}>
        <Text color={theme.colors.primary}>Loading agents...</Text>
      </Box>
    );
  }

  if (error) {
    return (
      <ErrorDisplay
        error={error}
        onRetry={() => {
          void refresh();
        }}
      />
    );
  }

  return (
    <Box flexDirection="column" overflow="hidden">
      {/* Header with state summary */}
      <Box marginBottom={1}>
        <Text bold color={theme.colors.success}>
          Agents ({agentList.length})
        </Text>
        {stateCounts.working > 0 && (
          <Text color={theme.colors.secondary}> ● {stateCounts.working} working</Text>
        )}
        {stateCounts.stuck > 0 && (
          <Text color={theme.colors.warning}> ⚠ {stateCounts.stuck} stuck</Text>
        )}
        {stateCounts.error > 0 && (
          <Text color={theme.colors.error}> ✗ {stateCounts.error} error</Text>
        )}
        {search.query && <Text color={theme.colors.primary}> [/] &quot;{search.query}&quot;</Text>}
        {loading && <Text color={theme.colors.textMuted}> (refreshing...)</Text>}
      </Box>

      {/* Action feedback */}
      {actionState && (
        <Box marginBottom={1}>
          <Text
            color={actionState.status === 'success' ? theme.colors.success : theme.colors.error}
          >
            {actionState.status === 'success' ? '✓' : '✗'} {actionState.message}
          </Text>
        </Box>
      )}

      {/* Peek output panel */}
      {peekOutput && selectedAgent && (
        <AgentPeekPanel
          agent={selectedAgent}
          output={peekOutput}
          loading={peekLoading}
          isNarrow={isNarrow}
        />
      )}

      {/* Confirmation dialog */}
      {confirmAction && selectedAgent && (
        <AgentConfirmDialog action={confirmAction} agent={selectedAgent} isNarrow={isNarrow} />
      )}

      {/* Agent list */}
      <AgentList
        items={visibleItems}
        agents={agentList}
        selectedIndex={validIndex}
        groupedView={groupedView}
        collapsedRoles={collapsedRoles}
      />

      {/* Actions bar */}
      {selectedAgent && !confirmAction && <AgentActions agent={selectedAgent} />}

      {/* Footer */}
      <Footer
        hints={[
          { key: 'j/k', label: 'nav' },
          { key: 'Enter', label: 'attach' },
          { key: 'd', label: 'detail' },
          { key: 'v', label: groupedView ? 'flat' : 'grouped' },
          { key: '/', label: 'search' },
          ...(search.query ? [{ key: 'c', label: 'clear' }] : []),
          { key: 'p', label: 'peek' },
          { key: 'r', label: 'refresh' },
        ]}
      />
    </Box>
  );
};

export default AgentsView;
