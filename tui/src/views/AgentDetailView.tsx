import React, { useReducer, useEffect, useCallback } from 'react';
import { Box, Text, useInput as inkUseInput } from 'ink';
import { spawnSync } from 'child_process';
import { useTheme } from '../theme';
import type { Agent } from '../types';
import { execBc } from '../services/bc';
import { StatusBadge } from '../components/StatusBadge';
import { useFocus, useIsOverlayActive } from '../navigation/FocusContext';
import { useAgentDetails } from '../hooks/useAgentDetails';
import { Footer } from '../components/Footer';
import { isPeekHeader } from '../utils';
import {
  agentDetailReducer,
  initialState,
  TabButton,
  normalizeTask,
  AgentOutputTab,
  AgentLiveTab,
  AgentDetailsTab,
  AgentMetricsTab,
} from './agent-detail';

// Safe wrapper for useInput that handles test environments
const useSafeInput = (
  handler: Parameters<typeof inkUseInput>[0],
  options?: Parameters<typeof inkUseInput>[1]
) => {
  try {
    inkUseInput(handler, options);
  } catch {
    // Silently fail in test environments
  }
};

interface AgentDetailViewProps {
  agent: Agent;
  onBack?: () => void;
}

export const AgentDetailView: React.FC<AgentDetailViewProps> = ({ agent, onBack }) => {
  const { theme } = useTheme();
  const [state, dispatch] = useReducer(agentDetailReducer, initialState);
  const { setFocus } = useFocus();
  const overlayActive = useIsOverlayActive();

  // Fetch agent-specific details (costs, activity)
  const { cost, activity } = useAgentDetails(agent.name);

  /**
   * Synchronize focus state with input mode
   */
  useEffect(() => {
    if (state.inputMode) {
      setFocus('input');
    } else {
      setFocus('view');
    }
  }, [state.inputMode, setFocus]);

  const fetchAgentOutput = useCallback(async () => {
    try {
      const output = await execBc(['agent', 'peek', agent.name, '--lines', '50']);
      // #1844: Strip peek headers and empty lines from output
      const lines = output.split('\n').filter((line) => line.trim() && !isPeekHeader(line));
      dispatch({ type: 'SET_OUTPUT', lines });
      dispatch({ type: 'SET_ERROR', error: null });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch agent output';
      dispatch({ type: 'SET_ERROR', error: message });
    }
  }, [agent.name]);

  // Live mode fetches more lines and refreshes faster
  const fetchLiveOutput = useCallback(async () => {
    try {
      const output = await execBc(['agent', 'peek', agent.name, '--lines', '200']);
      // #1844: Strip peek headers and empty lines from output
      const lines = output.split('\n').filter((line) => line.trim() && !isPeekHeader(line));
      // Auto-scroll to bottom if following and new content arrived
      const newOffset =
        state.isFollowing && lines.length > state.liveLines.length
          ? Math.max(0, lines.length - 20)
          : undefined;
      dispatch({ type: 'SET_LIVE_LINES', lines, scrollOffset: newOffset });
      dispatch({ type: 'SET_ERROR', error: null });
    } catch {
      // Silently fail for live mode - don't disrupt the experience
    }
  }, [agent.name, state.isFollowing, state.liveLines.length]);

  useEffect(() => {
    dispatch({ type: 'SET_LOADING', loading: true });
    void fetchAgentOutput().finally(() => {
      dispatch({ type: 'SET_LOADING', loading: false });
    });
  }, [fetchAgentOutput]);

  useEffect(() => {
    const interval = setInterval(() => {
      void fetchAgentOutput();
    }, 2000);
    return () => {
      clearInterval(interval);
    };
  }, [fetchAgentOutput]);

  // #1855: Live mode polls at 2.5s only when following; stops when paused.
  useEffect(() => {
    if (state.activeTab !== 'live') return undefined;

    void fetchLiveOutput();

    if (!state.isFollowing) return undefined;

    const interval = setInterval(() => {
      void fetchLiveOutput();
    }, 2500);
    return () => {
      clearInterval(interval);
    };
  }, [state.activeTab, state.isFollowing, fetchLiveOutput]);

  const sendMessage = useCallback(
    async (message: string) => {
      if (!message.trim()) return;
      try {
        dispatch({ type: 'SET_SEND_STATUS', status: `Sending to ${agent.name}...` });
        await execBc(['agent', 'send', agent.name, message]);
        dispatch({ type: 'SET_SEND_STATUS', status: `Sent to ${agent.name}` });
        dispatch({ type: 'SET_MESSAGE_BUFFER', buffer: '' });
        setTimeout(() => {
          dispatch({ type: 'SET_SEND_STATUS', status: null });
        }, 2000);
        await fetchAgentOutput();
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Failed to send';
        dispatch({ type: 'SET_SEND_STATUS', status: `Error: ${errorMsg}` });
        setTimeout(() => {
          dispatch({ type: 'SET_SEND_STATUS', status: null });
        }, 3000);
      }
    },
    [agent.name, fetchAgentOutput]
  );

  useSafeInput(
    (input, key) => {
      if (state.inputMode) {
        if (key.return) {
          void sendMessage(state.messageBuffer);
          dispatch({ type: 'TOGGLE_INPUT_MODE', enabled: false });
        } else if (key.escape) {
          dispatch({ type: 'RESET_INPUT' });
        } else if (key.backspace || key.delete) {
          dispatch({ type: 'SET_MESSAGE_BUFFER', buffer: state.messageBuffer.slice(0, -1) });
        } else if (input && !key.ctrl && !key.meta) {
          dispatch({ type: 'SET_MESSAGE_BUFFER', buffer: state.messageBuffer + input });
        }
      } else {
        if (input === 'i' || input === 'm') {
          dispatch({ type: 'TOGGLE_INPUT_MODE', enabled: true });
        } else if (input === 'a') {
          // #1691: Attach to agent's tmux session directly
          const bcBin = process.env.MYCEL_BIN ?? 'bc';
          spawnSync(bcBin, ['agent', 'attach', agent.name], {
            stdio: 'inherit',
          });
          void fetchAgentOutput();
        } else if (input === 'q' || key.escape) {
          onBack?.();
        } else if (input === 'r') {
          if (state.activeTab === 'live') {
            void fetchLiveOutput();
          } else {
            void fetchAgentOutput();
          }
        } else if (input === '1') {
          dispatch({ type: 'SET_TAB', tab: 'output' });
        } else if (input === '2') {
          dispatch({ type: 'SET_TAB', tab: 'live' });
          dispatch({ type: 'SET_SCROLL_OFFSET', offset: 0 });
        } else if (input === '3') {
          dispatch({ type: 'SET_TAB', tab: 'details' });
        } else if (input === '4') {
          dispatch({ type: 'SET_TAB', tab: 'metrics' });
        } else if (state.activeTab === 'live' && (input === 'j' || key.downArrow)) {
          const maxOffset = Math.max(0, state.liveLines.length - 20);
          const newOffset = Math.min(state.scrollOffset + 1, maxOffset);
          dispatch({ type: 'SET_SCROLL_OFFSET', offset: newOffset });
          if (newOffset >= maxOffset) {
            dispatch({ type: 'SET_IS_FOLLOWING', following: true });
          }
        } else if (state.activeTab === 'live' && (input === 'k' || key.upArrow)) {
          dispatch({ type: 'SET_IS_FOLLOWING', following: false });
          dispatch({ type: 'SET_SCROLL_OFFSET', offset: Math.max(0, state.scrollOffset - 1) });
        } else if (state.activeTab === 'live' && input === 'g') {
          dispatch({ type: 'SET_IS_FOLLOWING', following: false });
          dispatch({ type: 'SET_SCROLL_OFFSET', offset: 0 });
        } else if (state.activeTab === 'live' && input === 'G') {
          dispatch({ type: 'SET_IS_FOLLOWING', following: true });
          dispatch({ type: 'SET_SCROLL_OFFSET', offset: Math.max(0, state.liveLines.length - 20) });
        } else if (state.activeTab === 'live' && input === 'f') {
          const nowFollowing = !state.isFollowing;
          dispatch({ type: 'SET_IS_FOLLOWING', following: nowFollowing });
          if (nowFollowing) {
            dispatch({
              type: 'SET_SCROLL_OFFSET',
              offset: Math.max(0, state.liveLines.length - 20),
            });
          }
        }
      }
    },
    { isActive: !overlayActive }
  );

  // #1161: Use full available height, don't cap artificially
  const outputHeight = 20;

  return (
    <Box flexDirection="column" width="100%" height="100%" overflow="hidden">
      {/* Header */}
      <Box flexDirection="row" marginBottom={1} paddingX={1}>
        <Box flexDirection="column" flexGrow={1}>
          <Box>
            <Text bold color={theme.colors.primary}>
              {agent.name}
            </Text>
            <Text dimColor> | Role: {agent.role}</Text>
          </Box>
          <Box>
            <Text>State: </Text>
            <StatusBadge state={agent.state} />
            <Text dimColor wrap="truncate">
              {' '}
              | Task: {normalizeTask(agent.task)}
            </Text>
          </Box>
        </Box>
      </Box>

      {/* Tab Bar */}
      <Box paddingX={1} marginBottom={1}>
        <TabButton label="Output" tabKey="1" active={state.activeTab === 'output'} />
        <Text> </Text>
        <TabButton label="Live" tabKey="2" active={state.activeTab === 'live'} />
        <Text> </Text>
        <TabButton label="Details" tabKey="3" active={state.activeTab === 'details'} />
        <Text> </Text>
        <TabButton label="Metrics" tabKey="4" active={state.activeTab === 'metrics'} />
      </Box>

      {/* Tab Content */}
      <Box flexDirection="column" flexGrow={1}>
        {state.activeTab === 'output' && (
          <AgentOutputTab
            outputLines={state.outputLines}
            loading={state.loading}
            error={state.error}
            inputMode={state.inputMode}
            messageBuffer={state.messageBuffer}
            sendStatus={state.sendStatus}
            outputHeight={outputHeight}
          />
        )}
        {state.activeTab === 'live' && (
          <AgentLiveTab
            liveLines={state.liveLines}
            scrollOffset={state.scrollOffset}
            outputHeight={outputHeight}
            isFollowing={state.isFollowing}
          />
        )}
        {state.activeTab === 'details' && <AgentDetailsTab agent={agent} />}
        {state.activeTab === 'metrics' && (
          <AgentMetricsTab agent={agent} cost={cost} activity={activity} />
        )}
      </Box>

      {/* Footer with keybindings */}
      {state.inputMode ? (
        <Footer
          hints={[
            { key: 'Enter', label: 'send' },
            { key: 'Esc', label: 'cancel' },
          ]}
        />
      ) : state.activeTab === 'live' ? (
        <Footer
          hints={[
            { key: '1-4', label: 'tabs' },
            { key: 'j/k', label: 'scroll' },
            { key: 'g/G', label: 'top/bottom' },
            { key: 'f', label: 'follow' },
            { key: 'a', label: 'attach' },
            { key: 'q/Esc', label: 'back' },
          ]}
        />
      ) : (
        <Footer
          hints={[
            { key: '1-4', label: 'tabs' },
            { key: 'i', label: 'message' },
            { key: 'a', label: 'attach' },
            { key: 'r', label: 'refresh' },
            { key: 'q/Esc', label: 'back' },
          ]}
        />
      )}
    </Box>
  );
};

export default AgentDetailView;
