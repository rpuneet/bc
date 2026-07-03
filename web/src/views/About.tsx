import { useCallback, useEffect, useState } from "react";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";

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
  useHeaderSlot({ title: <TabHeaderTitle>About</TabHeaderTitle> });

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

  const channels: ChannelStatus[] = [
    {
      label: "Daemon (bcd)",
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
          <span className="text-[11px] uppercase tracking-wider text-mycel-muted">Installed</span>
          <span className="text-2xl font-semibold text-mycel-text font-mono tabular-nums">
            {health?.version ?? "—"}
          </span>
          {/* Compare like-with-like. `latestTag` is a semver ("0.3.1"). The
              daemon's version is either the same shape (release binaries) or
              a `YYYY.MM.DD.<sha>` dev-build string. Only surface "update
              available" when both look like semver — for dev builds render a
              "dev build" chip instead. Fixes #3212. */}
          {(() => {
            if (!latestTag || !health?.version) return null;
            // Semver = up to 3-digit major.minor.patch, optionally followed
            // by a pre-release or metadata suffix. Rejects the date-hash
            // dev-build format `YYYY.MM.DD.<sha>` since it starts with a
            // 4-digit year.
            const looksLikeSemver = /^\d{1,3}\.\d{1,3}\.\d{1,3}(?:[-+][A-Za-z0-9.]+)?$/.test(health.version);
            if (!looksLikeSemver) {
              return (
                <span className="text-[10px] uppercase tracking-wider rounded px-1.5 py-0.5 bg-mycel-info/15 text-mycel-info ring-1 ring-mycel-info/30">
                  dev build
                </span>
              );
            }
            return health.version !== latestTag ? (
              <span className="text-[10px] uppercase tracking-wider rounded px-1.5 py-0.5 bg-mycel-warning/15 text-mycel-warning ring-1 ring-mycel-warning/30">
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

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-px rounded-md border border-mycel-border bg-mycel-border/40 overflow-hidden">
        {channels.map((c) => (
          <ChannelTile key={c.label} channel={c} />
        ))}
      </div>

      <section className="rounded-md border border-mycel-border/60 bg-mycel-surface px-4 py-3 text-[12px] leading-relaxed text-mycel-muted">
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
  ok: "bg-emerald-400",
  stale: "bg-amber-400",
  loading: "bg-mycel-muted animate-pulse",
  unknown: "bg-mycel-muted/40",
  error: "bg-rose-400",
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
          <p className="mt-0.5 text-[10px] text-mycel-muted/70 truncate" title={channel.detail}>
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
      <a href={channel.href} target="_blank" rel="noreferrer" className="block focus:outline-none focus:ring-1 focus:ring-mycel-accent">
        {body}
      </a>
    );
  }
  return body;
}
