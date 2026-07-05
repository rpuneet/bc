import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Link, Navigate, useParams } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ThemeProvider } from "./context/ThemeContext";

const Live = lazy(() => import("./views/Live").then((m) => ({ default: m.Live })));
const Agents = lazy(() => import("./views/Agents").then((m) => ({ default: m.Agents })));
const AgentDetail = lazy(() => import("./views/AgentDetail").then((m) => ({ default: m.AgentDetail })));
const Notifications = lazy(() => import("./views/Notifications").then((m) => ({ default: m.Notifications })));
const NotificationActivity = lazy(() => import("./views/NotificationActivity").then((m) => ({ default: m.NotificationActivity })));
const Templates = lazy(() => import("./views/Templates").then((m) => ({ default: m.Templates })));
const Tools = lazy(() => import("./views/Tools").then((m) => ({ default: m.Tools })));
const ProviderDetail = lazy(() => import("./views/ProviderDetail").then((m) => ({ default: m.ProviderDetail })));
const Cron = lazy(() => import("./views/Cron").then((m) => ({ default: m.Cron })));
const Secrets = lazy(() => import("./views/Secrets").then((m) => ({ default: m.Secrets })));
const Insights = lazy(() => import("./views/Insights").then((m) => ({ default: m.Insights })));
const Settings = lazy(() => import("./views/Settings").then((m) => ({ default: m.Settings })));
const Code = lazy(() => import("./views/Code").then((m) => ({ default: m.Code })));
const About = lazy(() => import("./views/About").then((m) => ({ default: m.About })));

function Loading() {
  return <div className="p-6 text-mycel-muted">Loading...</div>;
}

function LegacyWorkspaceRedirect() {
  const params = useParams();
  return <Navigate to={`/${params["*"] ?? ""}`} replace />;
}

function NotFound() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6">
      <p className="text-6xl font-bold font-mono text-mycel-muted">404</p>
      <p className="mt-2 text-mycel-muted">Page not found</p>
      <Link to="/" className="mt-4 text-sm text-mycel-accent hover:underline">
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

/**
 * AppRoutes — the route table, split from App so tests can mount it
 * inside a MemoryRouter (App owns the BrowserRouter).
 */
export function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={wrap(<Live />)} />
        <Route path="live" element={wrap(<Live />)} />
        <Route path="agents" element={wrap(<Agents />)} />
        <Route path="agents/:name" element={wrap(<AgentDetail />)} />
        <Route path="agents/:name/*" element={wrap(<AgentDetail />)} />
        <Route path="notifications" element={wrap(<Notifications />)} />
        <Route path="notifications/activity" element={wrap(<NotificationActivity />)} />
        <Route path="notifications/:sourceName" element={wrap(<Notifications />)} />
        <Route path="templates" element={wrap(<Templates />)} />
        <Route path="tools" element={wrap(<Tools />)} />
        <Route path="tools/:provider" element={wrap(<ProviderDetail />)} />
        <Route path="cron" element={wrap(<Cron />)} />
        <Route path="secrets" element={wrap(<Secrets />)} />
        <Route path="insights" element={wrap(<Insights />)} />
        {/* Metrics + Costs merged into the single /insights dashboard —
            old links redirect. */}
        <Route path="stats" element={<Navigate to="/insights" replace />} />
        <Route path="metrics" element={<Navigate to="/insights" replace />} />
        <Route path="costs" element={<Navigate to="/insights" replace />} />
        <Route path="code" element={wrap(<Code />)} />
        <Route path="code/*" element={wrap(<Code />)} />
        <Route path="settings" element={wrap(<Settings />)} />
        <Route path="about" element={wrap(<About />)} />

        {/* Old builds 301'd /<page> → /w/<hash>/<page>; browsers
            cached that redirect, so route it back to flat URLs. */}
        <Route path="w/:ws/*" element={<LegacyWorkspaceRedirect />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}

export function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </ThemeProvider>
    </ErrorBoundary>
  );
}
