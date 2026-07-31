import { useCallback, useEffect, useState } from "react";
import { api, type DoctorReport, type HealthReport } from "../api/client";
import { deriveReadiness, type Readiness } from "../views/readiness/readiness";

interface UseReadiness {
  data: Readiness | null;
  loading: boolean;
  loaded: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

/**
 * useReadiness — fetches /api/doctor + /api/health and derives the grouped
 * readiness model. Deps can change while the app is open (the user installs
 * a CLI, starts Docker), so `refresh` re-runs both probes on demand. The
 * health probe is best-effort: doctor alone is enough to render, health
 * only sharpens the Docker verdict.
 *
 * `enabled` gates the automatic fetch (default true). Pass a flag — e.g. a
 * modal's `open` — to defer the probe until the surface is shown; flipping
 * it true re-runs the check so the data is fresh each time.
 */
export function useReadiness(enabled = true): UseReadiness {
  const [data, setData] = useState<Readiness | null>(null);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [report, health] = await Promise.all([
        api.getDoctor() as Promise<DoctorReport>,
        api.getHealth().catch(() => null) as Promise<HealthReport | null>,
      ]);
      setData(deriveReadiness(report, health));
      setLoaded(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to check readiness");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  return { data, loading, loaded, error, refresh };
}
