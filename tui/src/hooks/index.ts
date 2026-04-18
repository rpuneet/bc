/**
 * TUI Hooks - React hooks for bc CLI integration
 */

export {
  useAgents,
  useAgentsByState,
  useAgentsByRole,
  useAgent,
  type UseAgentsOptions,
  type UseAgentsResult,
} from './useAgents';

export {
  useStatus,
  useWorkspaceHealth,
  useUtilization,
  type UseStatusOptions,
  type UseStatusResult,
  type WorkspaceStatus,
} from './useStatus';

export {
  useNotifications,
  useNotifications as useChannels,
  useChannelHistory,
  useUnreadCount,
  useNotificationsWithUnread,
  useNotificationsWithUnread as useChannelsWithUnread,
  type UseNotificationsOptions,
  type UseNotificationsOptions as UseChannelsOptions,
  type UseNotificationsResult,
  type UseNotificationsResult as UseChannelsResult,
  type UseChannelHistoryOptions,
  type UseChannelHistoryResult,
} from './useNotifications';

export { useCosts, type UseCostsOptions, type UseCostsResult } from './useCosts';

export { useDashboard } from './useDashboard';

export {
  useMessagePolling,
  useAgentPolling,
  useCoordinatedPolling,
  type UsePollingOptions,
  type UseMessagePollingOptions,
  type UseMessagePollingResult,
  type UseAgentPollingOptions,
  type UseAgentPollingResult,
  type AgentChange,
  type UseCoordinatedPollingOptions,
} from './usePolling';

export {
  useListNavigation,
  type UseListNavigationOptions,
  type UseListNavigationResult,
} from './useListNavigation';

export { UnreadProvider, useUnread, type UnreadProviderProps } from './UnreadContext';

export {
  useLogs,
  getSeverityColor,
  getSeverityIcon,
  type UseLogsOptions,
  type UseLogsResult,
  type LogSeverity,
} from './useLogs';

export {
  useAdaptivePolling,
  useAdaptiveAgentPolling,
  type PollingMode,
  type AdaptivePollingState,
  type UseAdaptivePollingOptions,
  type UseAdaptivePollingResult,
} from './useAdaptivePolling';

export {
  useKeybindingHints,
  getStatusBarHints,
  formatHintsForStatusBar,
  getViewForKey,
  matchesKey,
  DEFAULT_VIEW_SHORTCUTS,
  DEFAULT_VIEW_NUMBERS,
  type KeyHint,
  type Keybinding,
  type GlobalBindings,
  type ViewBindings,
  type ContextBindings,
  type KeybindingConfig,
} from './useKeybindings';

export {
  HintsProvider,
  useHintsContext,
  useViewHints,
  type HintsProviderProps,
} from './useHintsContext';

export {
  useAgentGroups,
  countAgentStates,
  groupAgentsByRole,
  normalizeTask,
  abbreviateRole,
  type StateCounts,
  type RoleGroup,
  type GroupedItem,
} from './useAgentGroups';

export {
  DisableInputProvider,
  useDisableInput,
  type DisableInputProviderProps,
} from './useDisableInput';

export {
  useDebounce,
  useDebouncedCallback,
  useDebouncedSearch,
  DEFAULT_DEBOUNCE_MS,
  type UseDebouncedCallbackOptions,
  type UseDebouncedCallbackResult,
  type UseDebouncedSearchOptions,
  type UseDebouncedSearchResult,
} from './useDebounce';

export {
  useFocusStateMachine,
  categorizeKey,
  type FocusState,
  type FocusTransition,
  type KeyCategory,
  type FocusStateMachineResult,
} from './useFocusStateMachine';

export { useLoadingTimeout } from './useLoadingTimeout';
