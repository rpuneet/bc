/**
 * Navigation Context - Global navigation state management
 */

import React, { createContext, useContext, useState, useCallback, useMemo } from 'react';
import type { ReactNode } from 'react';

// View types for navigation - trimmed to 8 core views
export type View =
  | 'dashboard'
  | 'agents'
  | 'notifications'
  | 'costs'
  | 'logs'
  | 'roles'
  | 'worktrees'
  | 'tools'
  | 'mcp'
  | 'secrets'
  | 'processes'
  | 'help';

// Tab configuration
export interface TabConfig {
  key: string;
  view: View;
  label: string;
  shortLabel?: string;
  shortcut?: string;
}

// k9s-style command aliases
export const DEFAULT_TABS: TabConfig[] = [
  { key: 'dash', view: 'dashboard', label: 'Dashboard', shortLabel: 'Dash' },
  { key: 'ag', view: 'agents', label: 'Agents', shortLabel: 'Agt' },
  { key: 'no', view: 'notifications', label: 'Notifications', shortLabel: 'Notif' },
  { key: 'co', view: 'costs', label: 'Costs', shortLabel: 'Cost' },
  { key: 'log', view: 'logs', label: 'Logs', shortLabel: 'Log' },
  { key: 'ro', view: 'roles', label: 'Roles', shortLabel: 'Role' },
  { key: 'wt', view: 'worktrees', label: 'Worktrees', shortLabel: 'Tree' },
  { key: 'tl', view: 'tools', label: 'Tools', shortLabel: 'Tool' },
  { key: 'mcp', view: 'mcp', label: 'MCP', shortLabel: 'MCP' },
  { key: 'sec', view: 'secrets', label: 'Secrets', shortLabel: 'Sec' },
  { key: 'ps', view: 'processes', label: 'Processes', shortLabel: 'Proc' },
  { key: '?', view: 'help', label: 'Help', shortLabel: '?' },
];

// Breadcrumb item for showing navigation path
export interface BreadcrumbItem {
  label: string;
  view?: View;
}

// Navigation state
export interface NavigationState {
  currentView: View;
  previousView: View | null;
  history: View[];
  historyIndex: number;
  breadcrumbs: BreadcrumbItem[];
}

// Navigation context value
export interface NavigationContextValue {
  // State
  currentView: View;
  previousView: View | null;
  tabs: TabConfig[];
  canGoBack: boolean;
  canGoForward: boolean;
  breadcrumbs: BreadcrumbItem[];

  // Actions
  navigate: (view: View) => void;
  goBack: () => void;
  goForward: () => void;
  goHome: () => void;
  nextTab: () => void;
  prevTab: () => void;
  setBreadcrumbs: (items: BreadcrumbItem[]) => void;
  clearBreadcrumbs: () => void;

  // Utilities
  isActive: (view: View) => boolean;
  getTabByKey: (key: string) => TabConfig | undefined;
  getTabByView: (view: View) => TabConfig | undefined;
}

const NavigationContext = createContext<NavigationContextValue | null>(null);

export interface NavigationProviderProps {
  children: ReactNode;
  initialView?: View;
  tabs?: TabConfig[];
}

export function NavigationProvider({
  children,
  initialView = 'dashboard',
  tabs = DEFAULT_TABS,
}: NavigationProviderProps): React.ReactElement {
  const [state, setState] = useState<NavigationState>({
    currentView: initialView,
    previousView: null,
    history: [initialView],
    historyIndex: 0,
    breadcrumbs: [],
  });

  const navigate = useCallback((view: View) => {
    setState((prev) => {
      if (prev.currentView === view) return prev;
      const newHistory = [...prev.history.slice(0, prev.historyIndex + 1), view];
      return {
        currentView: view,
        previousView: prev.currentView,
        history: newHistory,
        historyIndex: newHistory.length - 1,
        breadcrumbs: [],
      };
    });
  }, []);

  const goBack = useCallback(() => {
    setState((prev) => {
      if (prev.historyIndex <= 0) return prev;
      const newIndex = prev.historyIndex - 1;
      return {
        ...prev,
        currentView: prev.history[newIndex],
        previousView: prev.currentView,
        historyIndex: newIndex,
      };
    });
  }, []);

  const goForward = useCallback(() => {
    setState((prev) => {
      if (prev.historyIndex >= prev.history.length - 1) return prev;
      const newIndex = prev.historyIndex + 1;
      return {
        ...prev,
        currentView: prev.history[newIndex],
        previousView: prev.currentView,
        historyIndex: newIndex,
      };
    });
  }, []);

  const goHome = useCallback(() => {
    navigate('dashboard');
  }, [navigate]);

  const mainTabs = useMemo(() => tabs.filter((t) => t.key !== '?'), [tabs]);

  const nextTab = useCallback(() => {
    const currentIndex = mainTabs.findIndex((t) => t.view === state.currentView);
    if (currentIndex === -1) {
      navigate(mainTabs[0]?.view ?? 'dashboard');
    } else {
      const nextIndex = (currentIndex + 1) % mainTabs.length;
      navigate(mainTabs[nextIndex]?.view ?? 'dashboard');
    }
  }, [mainTabs, state.currentView, navigate]);

  const prevTab = useCallback(() => {
    const currentIndex = mainTabs.findIndex((t) => t.view === state.currentView);
    if (currentIndex === -1) {
      navigate(mainTabs[mainTabs.length - 1]?.view ?? 'dashboard');
    } else {
      const prevIndex = (currentIndex - 1 + mainTabs.length) % mainTabs.length;
      navigate(mainTabs[prevIndex]?.view ?? 'dashboard');
    }
  }, [mainTabs, state.currentView, navigate]);

  const setBreadcrumbs = useCallback((items: BreadcrumbItem[]) => {
    setState((prev) => ({ ...prev, breadcrumbs: items }));
  }, []);

  const clearBreadcrumbs = useCallback(() => {
    setState((prev) => ({ ...prev, breadcrumbs: [] }));
  }, []);

  const isActive = useCallback((view: View) => state.currentView === view, [state.currentView]);

  const getTabByKey = useCallback((key: string) => tabs.find((t) => t.key === key), [tabs]);

  const getTabByView = useCallback((view: View) => tabs.find((t) => t.view === view), [tabs]);

  const value = useMemo<NavigationContextValue>(
    () => ({
      currentView: state.currentView,
      previousView: state.previousView,
      tabs,
      canGoBack: state.historyIndex > 0,
      canGoForward: state.historyIndex < state.history.length - 1,
      breadcrumbs: state.breadcrumbs,
      navigate,
      goBack,
      goForward,
      goHome,
      nextTab,
      prevTab,
      setBreadcrumbs,
      clearBreadcrumbs,
      isActive,
      getTabByKey,
      getTabByView,
    }),
    [
      state,
      tabs,
      navigate,
      goBack,
      goForward,
      goHome,
      nextTab,
      prevTab,
      setBreadcrumbs,
      clearBreadcrumbs,
      isActive,
      getTabByKey,
      getTabByView,
    ]
  );

  return <NavigationContext.Provider value={value}>{children}</NavigationContext.Provider>;
}

export function useNavigation(): NavigationContextValue {
  const context = useContext(NavigationContext);
  if (!context) {
    throw new Error('useNavigation must be used within a NavigationProvider');
  }
  return context;
}

export function useIsActiveView(view: View): boolean {
  const { isActive } = useNavigation();
  return isActive(view);
}
