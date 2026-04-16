import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Navigate, Link } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ThemeProvider } from "./context/ThemeContext";
import {
  WorkspaceProvider,
  ActiveWorkspaceGuard,
  RedirectToActiveWorkspace,
} from "./context/WorkspaceContext";

const Live = lazy(() => import("./views/Live").then((m) => ({ default: m.Live })));
const Agents = lazy(() => import("./views/Agents").then((m) => ({ default: m.Agents })));
const AgentDetail = lazy(() => import("./views/AgentDetail").then((m) => ({ default: m.AgentDetail })));
const Channels = lazy(() => import("./views/Channels").then((m) => ({ default: m.Channels })));
const Templates = lazy(() => import("./views/Templates").then((m) => ({ default: m.Templates })));
const Tools = lazy(() => import("./views/Tools").then((m) => ({ default: m.Tools })));
const ProviderDetail = lazy(() => import("./views/ProviderDetail").then((m) => ({ default: m.ProviderDetail })));
const Cron = lazy(() => import("./views/Cron").then((m) => ({ default: m.Cron })));
const Secrets = lazy(() => import("./views/Secrets").then((m) => ({ default: m.Secrets })));
const Stats = lazy(() => import("./views/Stats").then((m) => ({ default: m.Stats })));
const Settings = lazy(() => import("./views/Settings").then((m) => ({ default: m.Settings })));
const WorkspacePicker = lazy(() => import("./views/WorkspacePicker").then((m) => ({ default: m.WorkspacePicker })));
const Code = lazy(() => import("./views/Code").then((m) => ({ default: m.Code })));

function Loading() {
  return <div className="p-6 text-bc-muted">Loading...</div>;
}

function NotFound() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6">
      <p className="text-6xl font-bold font-mono text-bc-muted">404</p>
      <p className="mt-2 text-bc-muted">Page not found</p>
      <Link to="/" className="mt-4 text-sm text-bc-accent hover:underline">
        Go home
      </Link>
    </div>
  );
}

const wrap = (node: React.ReactNode) => (
  <Suspense fallback={<Loading />}>
    <ErrorBoundary>{node}</ErrorBoundary>
  </Suspense>
);

export function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <BrowserRouter>
          <WorkspaceProvider>
            <Routes>
              <Route element={<Layout />}>
                {/* Root - redirect to active workspace */}
                <Route index element={<RedirectToActiveWorkspace tab="live" />} />

                {/* Workspace picker (no workspace context required) */}
                <Route path="w" element={wrap(<WorkspacePicker />)} />

                {/* Workspace-scoped routes (guard renders <Outlet /> for children) */}
                <Route path="w/:wsId" element={<ActiveWorkspaceGuard />}>
                  <Route index element={<Navigate to="live" replace />} />
                  <Route path="live" element={wrap(<Live />)} />
                  <Route path="agents" element={wrap(<Agents />)} />
                  <Route path="agents/:name" element={wrap(<AgentDetail />)} />
                  <Route path="agents/:name/*" element={wrap(<AgentDetail />)} />
                  <Route path="channels" element={wrap(<Channels />)} />
                  <Route path="channels/:channelName" element={wrap(<Channels />)} />
                  <Route path="templates" element={wrap(<Templates />)} />
                  <Route path="tools" element={wrap(<Tools />)} />
                  <Route path="tools/:provider" element={wrap(<ProviderDetail />)} />
                  <Route path="cron" element={wrap(<Cron />)} />
                  <Route path="secrets" element={wrap(<Secrets />)} />
                  <Route path="stats" element={wrap(<Stats />)} />
                  <Route path="metrics" element={wrap(<Stats />)} />
                  <Route path="settings" element={wrap(<Settings />)} />
                  <Route path="code" element={wrap(<Code />)} />
                  <Route path="code/*" element={wrap(<Code />)} />
                </Route>

                {/* Legacy redirects - preserve old bookmarks by bouncing to /w/<active>/<tab> */}
                <Route path="live" element={<RedirectToActiveWorkspace tab="live" />} />
                <Route path="logs" element={<RedirectToActiveWorkspace tab="live" />} />
                <Route path="agents" element={<RedirectToActiveWorkspace tab="agents" />} />
                <Route path="agents/*" element={<RedirectToActiveWorkspace tab="agents" />} />
                <Route path="channels" element={<RedirectToActiveWorkspace tab="channels" />} />
                <Route path="channels/*" element={<RedirectToActiveWorkspace tab="channels" />} />
                <Route path="templates" element={<RedirectToActiveWorkspace tab="templates" />} />
                <Route path="tools" element={<RedirectToActiveWorkspace tab="tools" />} />
                <Route path="tools/*" element={<RedirectToActiveWorkspace tab="tools" />} />
                <Route path="cron" element={<RedirectToActiveWorkspace tab="cron" />} />
                <Route path="secrets" element={<RedirectToActiveWorkspace tab="secrets" />} />
                <Route path="stats" element={<RedirectToActiveWorkspace tab="stats" />} />
                <Route path="metrics" element={<RedirectToActiveWorkspace tab="stats" />} />
                <Route path="settings" element={<RedirectToActiveWorkspace tab="settings" />} />
                <Route path="workspace" element={<RedirectToActiveWorkspace tab="settings" />} />
                <Route path="roles" element={<RedirectToActiveWorkspace tab="templates" />} />

                <Route path="*" element={<NotFound />} />
              </Route>
            </Routes>
          </WorkspaceProvider>
        </BrowserRouter>
      </ThemeProvider>
    </ErrorBoundary>
  );
}
