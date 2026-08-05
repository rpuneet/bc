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
 * Cached fallbacks refreshed 2026-08-04 from the live API.
 */
export const REPO = "rpuneet/mycel";

export type DesktopOS = "mac" | "linux" | "windows";

export type GitHubStats = {
  stars: number;
  contributors: number;
  version: string; // release tag, e.g. "v0.4.6"
  live: boolean; // true once real API data has replaced the fallback
  /** Resolved desktop asset URLs for the latest release (signed preferred). */
  desktopUrls: Partial<Record<DesktopOS, string>>;
};

/**
 * Candidate artifact names per OS, in preference order. Signed macOS builds
 * omit the suffix; when ALLOW_UNSIGNED_MACOS ships the release, the asset is
 * named `...-UNSIGNED.zip` so Gatekeeper refusal is visible in the filename.
 */
function desktopAssetCandidates(os: DesktopOS, version: string): string[] {
  const v = version.replace(/^v/, "");
  switch (os) {
    case "mac":
      return [
        `mycel-desktop_darwin_arm64_${v}.zip`,
        `mycel-desktop_darwin_arm64_${v}-UNSIGNED.zip`,
      ];
    case "linux":
      return [`mycel-desktop_linux_amd64_${v}.tar.gz`];
    case "windows":
      return [`mycel-desktop_windows_amd64_${v}.zip`];
  }
}

/** Pick the first candidate that appears in the release asset list. */
export function pickDesktopAsset(
  os: DesktopOS,
  version: string,
  assetNames: readonly string[],
): string | undefined {
  const candidates = desktopAssetCandidates(os, version);
  return candidates.find((n) => assetNames.includes(n));
}

export function desktopUrlsFromAssets(
  tag: string,
  assetNames: readonly string[],
): Partial<Record<DesktopOS, string>> {
  const base = `https://github.com/${REPO}/releases/download/${tag}`;
  const out: Partial<Record<DesktopOS, string>> = {};
  for (const os of ["mac", "linux", "windows"] as const) {
    const name = pickDesktopAsset(os, tag, assetNames);
    if (name) out[os] = `${base}/${name}`;
  }
  return out;
}

/**
 * Construct URLs when the asset list is unknown (SSR / offline fallback).
 * For macOS the last candidate is the UNSIGNED name so a release that only
 * shipped unsigned (until Developer ID lands) does not 404 from the site.
 */
export function desktopDownloadUrl(os: DesktopOS, version: string): string {
  const name = desktopAssetCandidates(os, version).at(-1)!;
  return `https://github.com/${REPO}/releases/latest/download/${name}`;
}

export const FALLBACK_STATS: GitHubStats = {
  stars: 4,
  contributors: 6,
  version: "v0.4.6",
  live: false,
  desktopUrls: {
    mac: desktopDownloadUrl("mac", "v0.4.6"),
    linux: desktopDownloadUrl("linux", "v0.4.6"),
    windows: desktopDownloadUrl("windows", "v0.4.6"),
  },
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
        if (rel.tag_name) {
          next.version = rel.tag_name;
          const names = Array.isArray(rel.assets)
            ? rel.assets.map((a: { name?: string }) => a.name).filter(Boolean)
            : [];
          if (names.length > 0) {
            next.desktopUrls = desktopUrlsFromAssets(rel.tag_name, names);
          } else {
            // Live release with no assets — do not invent download hrefs.
            next.desktopUrls = {};
          }
        }
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

/** Best-effort OS detection for choosing the primary download button. */
export function detectOS(): DesktopOS {
  if (typeof navigator === "undefined") return "mac";
  const ua = `${navigator.userAgent} ${navigator.platform}`.toLowerCase();
  if (ua.includes("win")) return "windows";
  if (ua.includes("linux") && !ua.includes("android")) return "linux";
  return "mac"; // mac + everything else defaults to the flagship build
}
