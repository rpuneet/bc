"use client";

import { useEffect, useState } from "react";

/**
 * Live GitHub signal for rpuneet/mycel — stars, contributors, latest release.
 *
 * The site is a static export, so there is no server to fetch on. We fetch
 * client-side from the public GitHub API and hydrate over sensible cached
 * fallbacks (the real numbers at build time), so the strip is never empty and
 * never flashes zeros. If the API is unavailable or rate-limited, the cached
 * numbers simply stay — graceful degradation, no error UI.
 *
 * Cached fallbacks refreshed 2026-07-31 from the live API.
 */
export const REPO = "rpuneet/mycel";

export type GitHubStats = {
  stars: number;
  contributors: number;
  version: string; // release tag, e.g. "v0.3.13"
  live: boolean; // true once real API data has replaced the fallback
};

export const FALLBACK_STATS: GitHubStats = {
  stars: 4,
  contributors: 6,
  version: "v0.3.13",
  live: false,
};

/** Parse the total contributor count from a paginated Link header. */
function contributorsFromLink(link: string | null): number | null {
  if (!link) return null;
  const last = link.split(",").find((p) => /rel="last"/.test(p));
  const match = last?.match(/[?&]page=(\d+)/);
  return match ? Number(match[1]) : null;
}

/**
 * Fetches live repo stats client-side, falling back to cached values.
 * Runs once on mount; all three requests are best-effort and independent.
 */
export function useGitHubStats(): GitHubStats {
  const [stats, setStats] = useState<GitHubStats>(FALLBACK_STATS);

  useEffect(() => {
    let cancelled = false;
    const api = `https://api.github.com/repos/${REPO}`;

    async function load() {
      const next: GitHubStats = { ...FALLBACK_STATS, live: true };
      try {
        const repo = await fetch(api).then((r) => r.json());
        if (typeof repo.stargazers_count === "number") {
          next.stars = repo.stargazers_count;
        }
      } catch {
        /* keep fallback */
      }
      try {
        // per_page=1 makes the Link header's last page == contributor count
        const res = await fetch(`${api}/contributors?per_page=1&anon=true`);
        const count = contributorsFromLink(res.headers.get("link"));
        if (count) next.contributors = count;
      } catch {
        /* keep fallback */
      }
      try {
        const rel = await fetch(`${api}/releases/latest`).then((r) => r.json());
        if (rel.tag_name) next.version = rel.tag_name;
      } catch {
        /* keep fallback */
      }
      if (!cancelled) setStats(next);
    }

    load();
    return () => {
      cancelled = true;
    };
  }, []);

  return stats;
}

/**
 * Desktop app download URLs, keyed by OS. Built from the release artifact
 * names produced by .github/workflows/release.yml (Wails desktop build):
 *   mycel-desktop_darwin_arm64_<version>.zip
 *   mycel-desktop_linux_amd64_<version>.tar.gz
 *   mycel-desktop_windows_amd64_<version>.zip
 *
 * We use the /releases/latest/download/ redirect so a link resolves to the
 * newest tagged asset. The version is still needed because it is baked into
 * the filename; we take it from the live release tag (stripped of a leading v).
 *
 * NOTE: these 404 until the first desktop release (v0.4.0) is tagged and its
 * assets are uploaded. That is expected — the URLs are correct by construction
 * and will resolve the moment the release exists.
 */
export type DesktopOS = "mac" | "linux" | "windows";

export function desktopDownloadUrl(os: DesktopOS, version: string): string {
  const v = version.replace(/^v/, "");
  const base = `https://github.com/${REPO}/releases/latest/download`;
  switch (os) {
    case "mac":
      return `${base}/mycel-desktop_darwin_arm64_${v}.zip`;
    case "linux":
      return `${base}/mycel-desktop_linux_amd64_${v}.tar.gz`;
    case "windows":
      return `${base}/mycel-desktop_windows_amd64_${v}.zip`;
  }
}

/** Best-effort OS detection for choosing the primary download button. */
export function detectOS(): DesktopOS {
  if (typeof navigator === "undefined") return "mac";
  const ua = `${navigator.userAgent} ${navigator.platform}`.toLowerCase();
  if (ua.includes("win")) return "windows";
  if (ua.includes("linux") && !ua.includes("android")) return "linux";
  return "mac"; // mac + everything else defaults to the flagship build
}
