import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Navigate, Link } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ThemeProvider } from "./context/ThemeContext";

// Lazy-loaded views — each gets its own chunk
const Live = lazy(() =>
  import("./views/Live").then((m) => ({ default: m.Live })),
);
const Agents = lazy(() =>
  import("./views/Agents").then((m) => ({ default: m.Agents })),
);
const AgentDetail = lazy(() =>
  import("./views/AgentDetail").then((m) => ({ default: m.AgentDetail })),
);
const Channels = lazy(() =>
  import("./views/Channels").then((m) => ({ default: m.Channels })),
);
const Roles = lazy(() =>
  import("./views/Roles").then((m) => ({ default: m.Roles })),
);
const Tools = lazy(() =>
  import("./views/Tools").then((m) => ({ default: m.Tools })),
);
const ProviderDetail = lazy(() =>
  import("./views/ProviderDetail").then((m) => ({ default: m.ProviderDetail })),
);
const Cron = lazy(() =>
  import("./views/Cron").then((m) => ({ default: m.Cron })),
);
const Secrets = lazy(() =>
  import("./views/Secrets").then((m) => ({ default: m.Secrets })),
);
const Stats = lazy(() =>
  import("./views/Stats").then((m) => ({ default: m.Stats })),
);
const Workspace = lazy(() =>
  import("./views/Workspace").then((m) => ({ default: m.Workspace })),
);
const Settings = lazy(() =>
  import("./views/Settings").then((m) => ({ default: m.Settings })),
);

function Loading() {
  return <div className="p-6 text-bc-muted">Loading...</div>;
}

function NotFound() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6">
      <p className="text-6xl font-bold font-mono text-bc-muted">404</p>
      <p className="mt-2 text-bc-muted">Page not found</p>
      <Link to="/live" className="mt-4 text-sm text-bc-accent hover:underline">
        Back to Live
      </Link>
    </div>
  );
}

export function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <BrowserRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<Navigate to="/live" replace />} />
              <Route
                path="live"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Live />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              {/* Backward compat: /logs redirects to /live */}
              <Route path="logs" element={<Navigate to="/live" replace />} />
              <Route
                path="agents"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Agents />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="agents/:name"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <AgentDetail />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="channels/:channelName?"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Channels />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="roles"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Roles />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="tools"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Tools />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="tools/:provider"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <ProviderDetail />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="cron"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Cron />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="secrets"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Secrets />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="stats"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Stats />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="workspace"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Workspace />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route
                path="settings"
                element={
                  <Suspense fallback={<Loading />}>
                    <ErrorBoundary>
                      <Settings />
                    </ErrorBoundary>
                  </Suspense>
                }
              />
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </ThemeProvider>
    </ErrorBoundary>
  );
}
