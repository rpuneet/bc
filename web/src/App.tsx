import { lazy, Suspense, useEffect, useState } from "react";
import { BrowserRouter, Routes, Route, Link, Navigate, useParams } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { BootGate } from "./components/boot/BootGate";
import { ThemeProvider } from "./context/ThemeContext";
import { api } from "./api/client";

const Home = lazy(() => import("./views/Home").then((m) => ({ default: m.Home })));
const Agents = lazy(() => import("./views/Agents").then((m) => ({ default: m.Agents })));
const AgentDetail = lazy(() => import("./views/AgentDetail").then((m) => ({ default: m.AgentDetail })));
const Apps = lazy(() => import("./views/Apps").then((m) => ({ default: m.Apps })));
const AppsActivity = lazy(() => import("./views/AppsActivity").then((m) => ({ default: m.AppsActivity })));
const Templates = lazy(() => import("./views/Templates").then((m) => ({ default: m.Templates })));
const Marketplace = lazy(() => import("./views/Marketplace").then((m) => ({ default: m.Marketplace })));
const ProviderDetail = lazy(() => import("./views/ProviderDetail").then((m) => ({ default: m.ProviderDetail })));
const Insights = lazy(() => import("./views/Insights").then((m) => ({ default: m.Insights })));
const Settings = lazy(() => import("./views/Settings").then((m) => ({ default: m.Settings })));
const Code = lazy(() => import("./views/Code").then((m) => ({ default: m.Code })));
const About = lazy(() => import("./views/About").then((m) => ({ default: m.About })));
const Readiness = lazy(() => import("./views/Readiness").then((m) => ({ default: m.Readiness })));
const Welcome = lazy(() => import("./wizard/Welcome").then((m) => ({ default: m.Welcome })));

function Loading() {
  return <div className="p-6 text-mycel-muted">Loading...</div>;
}

/**
 * HomeGate — routes a fresh install to the setup wizard. While the
 * onboarding probe is in flight it renders a blank/skeleton frame — not
 * the real Home — so a first run never flashes the empty dashboard before
 * bouncing to /welcome. Any probe failure (or a positive "not first run")
 * falls through to Home, so a daemon hiccup never traps the user in
 * onboarding.
 */
export function HomeGate() {
  const [status, setStatus] = useState<"pending" | "home" | "welcome">("pending");
  useEffect(() => {
    let cancelled = false;
    api
      .getOnboardingState()
      .then((s) => {
        if (!cancelled) setStatus(s.firstRun ? "welcome" : "home");
      })
      .catch(() => {
        if (!cancelled) setStatus("home");
      });
    return () => {
      cancelled = true;
    };
  }, []);
  if (status === "pending") return <Loading />;
  if (status === "welcome") return <Navigate to="/welcome" replace />;
  return <Home />;
}

function LegacyWorkspaceRedirect() {
  const params = useParams();
  return <Navigate to={`/${params["*"] ?? ""}`} replace />;
}

/** /notifications/<source> bookmarks survive the Apps rename. */
function LegacyNotificationsRedirect() {
  const params = useParams();
  const source = params.sourceName ?? "";
  return <Navigate to={source ? `/apps/${source}` : "/apps"} replace />;
}

/**
 * Providers/Tools folded into Settings as a list-only section (no more
 * standalone /tools page). Old /tools and /tools/:provider bookmarks — and
 * the earlier /settings/tools(/:provider) mounts — all resolve to the new
 * home: /settings for the list, /settings/providers/:name for detail. These
 * are permanent redirects so old links and bookmarks keep working.
 */
function ToolsProviderRedirect() {
  const { provider } = useParams();
  return <Navigate to={provider ? `/settings/providers/${provider}` : "/settings"} replace />;
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
      {/* Full-screen first-run wizard — lives outside the app chrome. */}
      <Route path="welcome" element={wrap(<Welcome />)} />
      <Route element={<Layout />}>
        <Route index element={wrap(<HomeGate />)} />
        <Route path="home" element={wrap(<HomeGate />)} />
        {/* Live view became the Home page — old links/bookmarks redirect. */}
        <Route path="live" element={<Navigate to="/" replace />} />
        <Route path="agents" element={wrap(<Agents />)} />
        <Route path="agents/:name" element={wrap(<AgentDetail />)} />
        <Route path="agents/:name/*" element={wrap(<AgentDetail />)} />
        <Route path="apps" element={wrap(<Apps />)} />
        <Route path="apps/activity" element={wrap(<AppsActivity />)} />
        <Route path="apps/:sourceName" element={wrap(<Apps />)} />
        {/* Notifications became Apps — old bookmarks redirect. */}
        <Route path="notifications" element={<Navigate to="/apps" replace />} />
        <Route path="notifications/activity" element={<Navigate to="/apps/activity" replace />} />
        <Route path="notifications/:sourceName" element={<LegacyNotificationsRedirect />} />
        <Route path="templates" element={wrap(<Templates />)} />
        <Route path="marketplace" element={wrap(<Marketplace />)} />
        {/* Providers/Tools folded into Settings — no standalone page.
            Bookmarked /tools links get a permanent redirect; the real
            per-provider detail view now mounts under /settings/providers. */}
        <Route path="tools" element={<Navigate to="/settings" replace />} />
        <Route path="tools/:provider" element={<ToolsProviderRedirect />} />
        <Route path="settings/tools" element={<Navigate to="/settings" replace />} />
        <Route path="settings/tools/:provider" element={<ToolsProviderRedirect />} />
        <Route path="settings/providers" element={<Navigate to="/settings" replace />} />
        <Route path="settings/providers/:provider" element={wrap(<ProviderDetail />)} />
        <Route path="providers" element={<Navigate to="/settings" replace />} />
        {/* Secrets became the Custom Keys section on the Apps home. */}
        <Route path="secrets" element={<Navigate to="/apps#custom-keys" replace />} />
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
        <Route path="readiness" element={wrap(<Readiness />)} />
        {/* "setup" is the friendlier alias surfaced in nudges/CTAs. */}
        <Route path="setup" element={<Navigate to="/readiness" replace />} />

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
          <BootGate>
            <AppRoutes />
          </BootGate>
        </BrowserRouter>
      </ThemeProvider>
    </ErrorBoundary>
  );
}
