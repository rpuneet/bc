import { useCallback, useEffect, useState } from "react";
import { ExternalLink } from "../components/ExternalLink";
import { desktopAppVersion } from "../utils/desktopApp";

/* ── Types ──────────────────────────────────────────────────────────── */

interface Health {
  status: string;
  db: string;
  version: string;
}

interface GhRelease {
  tag_name: string;
  html_url: string;
  published_at: string;
}

interface NpmInfo {
  versions: string[];
  latest: string;
}

interface ChannelStatus {
  label: string;
  href?: string;
  version?: string;
  detail?: string;
  state: "ok" | "stale" | "loading" | "unknown" | "error";
}

/* ── Helpers ────────────────────────────────────────────────────────── */

const GITHUB_REPO = "rpuneet/mycel";

/**
 * isReleaseVersion — whether a version string names a published release, which
 * is the only case where being "behind" the latest tag is meaningful.
 *
 * A release build's version is exactly X.Y.Z. Everything else is a source build:
 * `dev` when no tag was reachable, otherwise the `0.4.5-dev.12.g1a2b3c4` form
 * from scripts/version.sh. Testing for the absence of a prerelease suffix rather
 * than for a particular dev-build spelling is deliberate — the previous check
 * tested for a date-prefixed format and would have started calling source builds
 * releases the moment that spelling changed (#3212).
 */
export function isReleaseVersion(v: string): boolean {
  return /^\d+\.\d+\.\d+$/.test(v);
}

/** withTimeout — caps any one channel-check fetch at `ms` so a hung
 *  request (DNS timeout, GitHub API rate-limit holding the socket open,
 *  slow mirror) can't pin the page in the loading state. Resolves the
 *  supplied fallback instead of rejecting. */
function withTimeout<T>(p: Promise<T>, fallback: T, ms = 8000): Promise<T> {
  return Promise.race([
    p,
    new Promise<T>((resolve) => setTimeout(() => resolve(fallback), ms)),
  ]);
}

async function fetchHealth(): Promise<Health | null> {
  try {
    const r = await fetch("/api/health");
    if (!r.ok) return null;
    return (await r.json()) as Health;
  } catch {
    return null;
  }
}

async function fetchLatestGhRelease(): Promise<GhRelease | null> {
  try {
    const r = await fetch(`https://api.github.com/repos/${GITHUB_REPO}/releases/latest`);
    if (!r.ok) return null;
    const j = (await r.json()) as GhRelease;
    return j;
  } catch {
    return null;
  }
}

async function fetchNpmLatest(): Promise<NpmInfo | null> {
  try {
    const r = await fetch(`https://registry.npmjs.org/mycel-cli`);
    if (!r.ok) return null;
    const j = (await r.json()) as { "dist-tags"?: { latest?: string }; versions?: Record<string, unknown> };
    const versions = Object.keys(j.versions ?? {});
    const latest = j["dist-tags"]?.latest ?? versions[versions.length - 1] ?? "?";
    return { versions, latest };
  } catch {
    return null;
  }
}

async function fetchBrewVersion(): Promise<string | null> {
  try {
    // The Formula file lives on the homebrew-mycel tap repo at a stable path.
    const r = await fetch(`https://raw.githubusercontent.com/${GITHUB_REPO.split("/")[0]}/homebrew-mycel/main/Formula/mycel.rb`);
    if (!r.ok) return null;
    const text = await r.text();
    const m = text.match(/version\s+"([^"]+)"/);
    return m?.[1] ?? null;
  } catch {
    return null;
  }
}

async function pingPages(): Promise<boolean> {
  try {
    const r = await fetch(`https://${GITHUB_REPO.split("/")[0]}.github.io/${GITHUB_REPO.split("/")[1]}/`, { method: "HEAD", mode: "no-cors" });
    // no-cors fetches return opaque responses; presence of a non-throwing fetch is the strongest signal we can get.
    return r.type === "opaque" || r.ok;
  } catch {
    return false;
  }
}

/* ── View ──────────────────────────────────────────────────────────── */

export function About() {
  const [health, setHealth] = useState<Health | null>(null);
  const [latest, setLatest] = useState<GhRelease | null>(null);
  const [npm, setNpm] = useState<NpmInfo | null>(null);
  const [brew, setBrew] = useState<string | null>(null);
  const [pagesOk, setPagesOk] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [h, l, n, b, p] = await Promise.all([
        withTimeout(fetchHealth(), null),
        withTimeout(fetchLatestGhRelease(), null),
        withTimeout(fetchNpmLatest(), null),
        withTimeout(fetchBrewVersion(), null),
        withTimeout(pingPages(), false),
      ]);
      setHealth(h);
      setLatest(l);
      setNpm(n);
      setBrew(b);
      setPagesOk(p);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const latestTag = latest?.tag_name?.replace(/^v/, "") ?? null;

  const appVersion = desktopAppVersion();
  // The desktop app attaches to an already-running daemon rather than starting
  // its own when it finds one, so these two genuinely can differ — and when they
  // do, every version on this page comes from the daemon while the user is
  // looking at a newer app. Surfacing it is the difference between "my update
  // didn't work" and "my daemon is stale".
  const appDiffersFromDaemon = Boolean(appVersion && health?.version && appVersion !== health.version);

  const channels: ChannelStatus[] = [
    ...(appVersion
      ? [
          {
            label: "Desktop app",
            version: appVersion,
            detail: appDiffersFromDaemon ? "newer than the daemon below" : "matches the daemon",
            state: (appDiffersFromDaemon ? "stale" : "ok") as ChannelStatus["state"],
          },
        ]
      : []),
    {
      label: "Daemon",
      version: health?.version ?? undefined,
      detail: health ? `db: ${health.db}` : "unavailable",
      state: health ? "ok" : "error",
    },
    {
      label: "GitHub release",
      href: latest?.html_url,
      version: latestTag ?? undefined,
      detail: latest?.published_at
        ? new Date(latest.published_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }) +
          " · " +
          new Date(latest.published_at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
        : undefined,
      state: latest ? "ok" : "unknown",
    },
    {
      label: "npm (mycel-cli)",
      href: "https://www.npmjs.com/package/mycel-cli",
      version: npm?.latest,
      detail: npm ? `${npm.versions.length} versions` : undefined,
      // Without a latest GitHub tag we have no baseline to call this
      // "ok" or "stale" against — degrade to "unknown" instead of
      // showing a false green dot when the GitHub API is rate-limited.
      state: npm
        ? latestTag
          ? npm.latest !== latestTag
            ? "stale"
            : "ok"
          : "unknown"
        : "unknown",
    },
    {
      label: "Homebrew tap",
      href: `https://github.com/${GITHUB_REPO.split("/")[0]}/homebrew-mycel`,
      version: brew ?? undefined,
      detail: "brew tap rpuneet/mycel",
      // Same reasoning as the npm tile — without latestTag we can't
      // declare healthy or stale, so fall back to "unknown".
      state: brew
        ? latestTag
          ? brew !== latestTag
            ? "stale"
            : "ok"
          : "unknown"
        : "unknown",
    },
    {
      label: "Docs site",
      href: `https://${GITHUB_REPO.split("/")[0]}.github.io/${GITHUB_REPO.split("/")[1]}/`,
      detail: "GitHub Pages",
      state: pagesOk == null ? "loading" : pagesOk ? "ok" : "error",
    },
  ];

  return (
    <div className="p-6 flex flex-col gap-6 max-w-3xl mx-auto">
      <header className="flex items-baseline justify-between gap-3">
        <div className="flex items-baseline gap-3">
          {/* Named for what it actually is. This number always comes from the
              daemon's /api/health; calling it "Installed" inside the desktop app
              invited reading it as the app's own version, which is a different
              number whenever the app attached to a daemon it did not start. */}
          <span className="text-[11px] uppercase tracking-wider text-mycel-muted">
            {appVersion ? "Daemon" : "Installed"}
          </span>
          <span className="text-2xl font-semibold text-mycel-text font-mono tabular-nums">
            {health?.version ?? "—"}
          </span>
          {appDiffersFromDaemon && (
            <span
              className="text-[10px] uppercase tracking-wider rounded px-1.5 py-0.5 bg-mycel-warning-subtle text-mycel-warning ring-1 ring-inset ring-mycel-border"
              title={`This desktop app is ${appVersion}, but it is using a separately running daemon on ${health?.version}. Restart the daemon from the newer build to match.`}
            >
              app is {appVersion}
            </span>
          )}
          {/* Compare like-with-like. `latestTag` is a released version
              ("0.3.1"), so only a build that claims to *be* a release can
              meaningfully be behind one. Fixes #3212. */}
          {(() => {
            if (!latestTag || !health?.version) return null;
            if (!isReleaseVersion(health.version)) {
              return (
                <span className="text-[10px] uppercase tracking-wider rounded px-1.5 py-0.5 bg-mycel-info-subtle text-mycel-info ring-1 ring-inset ring-mycel-border">
                  dev build
                </span>
              );
            }
            return health.version !== latestTag ? (
              <span className="text-[10px] uppercase tracking-wider rounded px-1.5 py-0.5 bg-mycel-warning-subtle text-mycel-warning ring-1 ring-inset ring-mycel-border">
                update available
              </span>
            ) : null;
          })()}
        </div>
        <button
          type="button"
          onClick={() => void refresh()}
          disabled={loading}
          className="text-[11px] px-2.5 py-1 rounded border border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-muted hover:text-mycel-text transition-colors disabled:opacity-50"
        >
          {loading ? "Refreshing…" : "Refresh"}
        </button>
      </header>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-px rounded-md border border-mycel-border bg-mycel-border overflow-hidden shadow-mycel [&>*:last-child:nth-child(odd)]:sm:col-span-2">
        {channels.map((c) => (
          <ChannelTile key={c.label} channel={c} />
        ))}
      </div>

      <section className="rounded-md border border-mycel-border bg-mycel-surface px-4 py-3 text-[12px] leading-relaxed text-mycel-muted shadow-mycel">
        <p className="text-mycel-text mb-1.5 font-medium">Install or upgrade</p>
        <pre className="font-mono text-[11px] text-mycel-text whitespace-pre-wrap">
{`brew tap rpuneet/mycel && brew install mycel    # macOS / Linux
npm i -g mycel-cli                              # any platform with node
go install github.com/rpuneet/mycel/cmd/mycel@latest    # from source`}
        </pre>
      </section>

      <footer className="text-[10px] text-mycel-muted font-mono">
        Built from commit <span className="text-mycel-text">{health?.version ?? "?"}</span>.
        Live distribution checks hit github.com / registry.npmjs.org / raw.githubusercontent.com directly.
      </footer>
    </div>
  );
}

/* ── Channel tile ──────────────────────────────────────────────────── */

const STATE_DOT: Record<ChannelStatus["state"], string> = {
  ok: "bg-mycel-success",
  stale: "bg-mycel-warning",
  loading: "bg-mycel-muted animate-pulse",
  unknown: "bg-mycel-muted",
  error: "bg-mycel-error",
};

function ChannelTile({ channel }: { channel: ChannelStatus }) {
  const body = (
    <div className="bg-mycel-surface px-4 py-3 flex items-start gap-3 h-full hover:bg-mycel-surface-hover transition-colors">
      <span className={`mt-1.5 shrink-0 inline-flex w-2 h-2 rounded-full ${STATE_DOT[channel.state]}`} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-[12px] text-mycel-text font-medium">{channel.label}</span>
          {channel.version && (
            <span className="text-[11px] font-mono tabular-nums text-mycel-muted">v{channel.version}</span>
          )}
        </div>
        {channel.detail && (
          <p className="mt-0.5 text-[10px] text-mycel-muted truncate" title={channel.detail}>
            {channel.detail}
          </p>
        )}
      </div>
      {channel.href && (
        <span className="text-[10px] text-mycel-muted shrink-0" aria-hidden>↗</span>
      )}
    </div>
  );
  if (channel.href) {
    return (
      <ExternalLink href={channel.href} className="block focus:outline-none focus:ring-1 focus:ring-mycel-accent">
        {body}
      </ExternalLink>
    );
  }
  return body;
}
