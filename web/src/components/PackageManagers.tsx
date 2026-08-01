import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { PackageManager } from "../api/client";

/* ── PackageManagers ──────────────────────────────────────────────────
 *
 * A compact, honest readout of the host's package managers — what mycel
 * actually detected on PATH, with each manager's reported version. Feeds the
 * Tools page so an install failure ("no brew here") is legible before it
 * happens. Read-only: the data comes from GET /api/system/package-managers,
 * which only ever runs each manager's own --version probe.
 */

type LoadState = "loading" | "ready" | "error";

export function PackageManagers() {
  const [state, setState] = useState<LoadState>("loading");
  const [managers, setManagers] = useState<PackageManager[]>([]);
  const [os, setOs] = useState<string>("");

  useEffect(() => {
    let alive = true;
    api
      .getPackageManagers()
      .then((res) => {
        if (!alive) return;
        setManagers(res.managers ?? []);
        setOs(res.os ?? "");
        setState("ready");
      })
      .catch(() => {
        if (alive) setState("error");
      });
    return () => {
      alive = false;
    };
  }, []);

  if (state === "loading") {
    return (
      <div className="flex flex-wrap gap-2" aria-busy>
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-7 w-28 animate-pulse rounded-md bg-mycel-surface-hover" />
        ))}
      </div>
    );
  }

  if (state === "error") {
    return (
      <p className="text-[11px] text-mycel-muted">
        Could not detect package managers on this host.
      </p>
    );
  }

  if (managers.length === 0) {
    return (
      <p className="text-[11px] text-mycel-muted">
        No supported package managers detected on this {os || "host"}. Install commands that rely
        on one (brew, apt, npm…) will need it on PATH first.
      </p>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {managers.map((m) => (
        <span
          key={m.id}
          className="inline-flex items-center gap-1.5 rounded-md border border-mycel-border bg-mycel-bg px-2 py-1 text-[11px]"
          title={m.searchable ? `${m.name} — registry search supported` : `${m.name} — no registry search`}
        >
          <span className="w-1.5 h-1.5 rounded-full bg-mycel-success shrink-0" aria-hidden />
          <span className="font-medium text-mycel-text">{m.name}</span>
          {m.version && (
            <span className="font-mono text-mycel-muted tabular-nums max-w-[140px] truncate">
              {m.version}
            </span>
          )}
        </span>
      ))}
    </div>
  );
}
