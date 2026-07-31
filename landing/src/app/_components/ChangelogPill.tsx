"use client";

import Link from "next/link";
import { ArrowRight, Sparkles } from "lucide-react";
import { REPO, useGitHubStats } from "../../lib/github";

/**
 * "Shipped" signal (teardown #11): a small, live pill that shows the project is
 * alive. It reuses the release tag already fetched for the download buttons and
 * links straight to the GitHub release notes. Deliberately quiet — a single
 * line above the fold, not a banner.
 */
export function ChangelogPill() {
  const { version } = useGitHubStats();

  return (
    <Link
      href={`https://github.com/${REPO}/releases/latest`}
      target="_blank"
      rel="noopener noreferrer"
      className="group inline-flex items-center gap-2 rounded-full border border-outline-variant/20 bg-surface-container/60 py-1 pl-2 pr-3 font-body text-xs text-on-surface-variant transition-colors hover:border-primary/40 hover:text-on-surface"
    >
      <span className="inline-flex items-center gap-1 rounded-full bg-primary/12 px-2 py-0.5 font-label text-[11px] font-semibold tabular-nums text-primary-text">
        <Sparkles className="h-3 w-3" aria-hidden="true" />
        {version}
      </span>
      <span>See what&rsquo;s new</span>
      <ArrowRight
        className="h-3 w-3 transition-transform group-hover:translate-x-0.5"
        aria-hidden="true"
      />
    </Link>
  );
}
