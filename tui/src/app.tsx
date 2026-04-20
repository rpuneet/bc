/**
 * App - Main TUI application component
 * k9s-style navigation with :command bar and /filter bar
 */

import React, { useState, useCallback, useRef, memo } from 'react';
import { Box, Text, useStdout } from 'ink';
import {
  NavigationProvider,
  useNavigation,
  useKeyboardNavigation,
  Breadcrumb,
  FocusProvider,
  type View,
} from './navigation';
import { useTheme } from './theme';
import { useFocus } from './navigation/FocusContext';
import { UnreadProvider, HintsProvider, DisableInputProvider, useDisableInput } from './hooks';
import { useThemeConfig } from './config';
import { RootProvider } from './providers';
import { Dashboard } from './views/Dashboard';
import { AgentsView } from './views/AgentsView';
import { RolesView } from './views/RolesView';
import { NotificationsView } from './views/NotificationsView';
import { CostsView } from './views/CostsView';
import { LogsView } from './views/LogsView';
import { WorktreesView } from './views/WorktreesView';
import { HelpView } from './views/HelpView';
import { ToolsView } from './views/ToolsView';
import { MCPView } from './views/MCPView';
import { SecretsView } from './views/SecretsView';
import { ProcessesView } from './views/ProcessesView';
import { CommandBar } from './components/CommandBar';
import { FilterBar } from './components/FilterBar';
import { FilterProvider } from './hooks/useFilter';
import { ViewErrorBoundary } from './components/ErrorBoundary';

interface AppProps {
  disableInput?: boolean;
  initialView?: View;
}

export function App({
  disableInput = false,
  initialView = 'dashboard',
}: AppProps): React.ReactElement {
  return (
    <RootProvider>
      <AppWithFeatureProviders disableInput={disableInput} initialView={initialView} />
    </RootProvider>
  );
}

interface AppWithFeatureProvidersProps {
  disableInput: boolean;
  initialView: View;
}

function AppWithFeatureProviders({
  disableInput,
  initialView,
}: AppWithFeatureProvidersProps): React.ReactElement {
  const themeConfig = useThemeConfig();

  return (
    <NavigationProvider initialView={initialView}>
      <FocusProvider>
        <UnreadProvider>
          <HintsProvider>
            <DisableInputProvider disabled={disableInput}>
              <FilterProvider>
                <AppContent themeConfig={themeConfig} />
              </FilterProvider>
            </DisableInputProvider>
          </HintsProvider>
        </UnreadProvider>
      </FocusProvider>
    </NavigationProvider>
  );
}

interface AppContentProps {
  themeConfig: ReturnType<typeof useThemeConfig>;
}

function AppContent({ themeConfig }: AppContentProps): React.ReactElement {
  const { currentView, navigate } = useNavigation();
  const { stdout } = useStdout();
  const { setThemeName } = useTheme();
  const { isDisabled: disableInput } = useDisableInput();
  const { setFocus, returnFocus } = useFocus();
  const [showCommandBar, setShowCommandBar] = useState(false);
  const [showFilterBar, setShowFilterBar] = useState(false);

  // #1871: LRU tracking for recently used commands (persists across open/close)
  const MAX_LRU = 10;
  const recentCommandsRef = useRef<string[]>([]);
  const handleCommandUsed = useCallback((command: string) => {
    const lru = recentCommandsRef.current.filter((c) => c !== command);
    lru.unshift(command);
    if (lru.length > MAX_LRU) lru.pop();
    recentCommandsRef.current = lru;
  }, []);

  React.useEffect(() => {
    setThemeName(themeConfig.theme);
  }, [themeConfig.theme, setThemeName]);

  // #1870: Set focus to 'command'/'filter' when overlays open to prevent key leaks
  const openCommandBar = useCallback(() => {
    setShowCommandBar(true);
    setShowFilterBar(false);
    setFocus('command');
  }, [setFocus]);

  const closeCommandBar = useCallback(() => {
    setShowCommandBar(false);
    returnFocus();
  }, [returnFocus]);

  const handleCommandSelect = useCallback(
    (view: View) => {
      navigate(view);
      setShowCommandBar(false);
      returnFocus();
    },
    [navigate, returnFocus]
  );

  const openFilterBar = useCallback(() => {
    setShowFilterBar(true);
    setShowCommandBar(false);
    setFocus('filter');
  }, [setFocus]);

  const closeFilterBar = useCallback(() => {
    setShowFilterBar(false);
    returnFocus();
  }, [returnFocus]);

  useKeyboardNavigation({
    disabled: disableInput || showCommandBar || showFilterBar,
    onCommandBar: openCommandBar,
    onFilterBar: openFilterBar,
  });

  const terminalHeight = stdout.rows;
  const terminalWidth = stdout.columns;

  return (
    <Box
      flexDirection="column"
      padding={1}
      width={terminalWidth}
      height={terminalHeight}
      overflow="hidden"
    >
      {/* Breadcrumb - always shows current view */}
      <Breadcrumb />

      {/* Main content area - full width, no sidebar */}
      <Box flexDirection="column" flexGrow={1} overflow="hidden">
        <ViewErrorBoundary key={currentView} viewName={currentView}>
          <ViewContent view={currentView} />
        </ViewErrorBoundary>
      </Box>

      {/* Command bar overlay (k9s-style :command) */}
      {showCommandBar && (
        <CommandBar
          onSelect={handleCommandSelect}
          onClose={closeCommandBar}
          recentCommands={recentCommandsRef.current}
          onCommandUsed={handleCommandUsed}
        />
      )}

      {/* Filter bar overlay (k9s-style /filter) */}
      {showFilterBar && <FilterBar onClose={closeFilterBar} />}

      {/* Footer with static hints - only when no overlay is active */}
      {!showCommandBar && !showFilterBar && <Footer />}
    </Box>
  );
}

interface ViewContentProps {
  view: View;
}

const ViewContent = memo(function ViewContent({ view }: ViewContentProps): React.ReactElement {
  switch (view) {
    case 'dashboard':
      return <Dashboard />;
    case 'agents':
      return <AgentsView />;
    case 'notifications':
      return <NotificationsView />;
    case 'costs':
      return <CostsView />;
    case 'logs':
      return <LogsView />;
    case 'roles':
      return <RolesView />;
    case 'worktrees':
      return <WorktreesView />;
    case 'tools':
      return <ToolsView />;
    case 'mcp':
      return <MCPView />;
    case 'secrets':
      return <SecretsView />;
    case 'processes':
      return <ProcessesView />;
    case 'help':
      return <HelpView />;
    default:
      return <Text>Unknown view</Text>;
  }
});

/**
 * Footer - single line with static k9s-style hints
 */
const Footer = memo(function Footer(): React.ReactElement {
  return (
    <Box marginTop={1}>
      <Text dimColor>
        [<Text bold>:</Text>] command [<Text bold>/</Text>] filter [<Text bold>1-0</Text>] views [
        <Text bold>?</Text>] help [<Text bold>Tab</Text>] next [<Text bold>q</Text>] quit
      </Text>
    </Box>
  );
});
