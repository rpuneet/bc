"use client";

import Link from "next/link";
import { Star, Users, Tag, Scale } from "lucide-react";
import { REPO, useGitHubStats } from "../../lib/github";

/** Compact 1.2k-style formatting so big star counts stay legible. */
function compact(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}k`;
  return `${n}`;
}

function Badge({
  icon: Icon,
  value,
  label,
  href,
}: {
  icon: React.ComponentType<{ className?: string }>;
  value: string;
  label: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="group inline-flex items-center gap-2 rounded-full border border-outline-variant/20 bg-surface-container/60 px-3.5 py-1.5 transition-colors hover:border-primary/40 hover:bg-surface-container"
    >
      <Icon className="h-3.5 w-3.5 text-primary" aria-hidden="true" />
      <span className="font-headline text-sm font-semibold tabular-nums text-on-background">
        {value}
      </span>
      <span className="font-body text-xs text-on-surface-variant">{label}</span>
    </Link>
  );
}

/**
 * Social-proof strip (owner decision #2): live GitHub numbers rendered as
 * brand badges — not shields.io images. Stars, contributors, and the latest
 * release tag are fetched at runtime with cached fallbacks, so the row is
 * always populated even if the API is unavailable. MIT / open-source is the
 * fourth signal.
 */
export function SocialProof() {
  const { stars, contributors, version } = useGitHubStats();
  const repoUrl = `https://github.com/${REPO}`;

  return (
    <div className="flex flex-wrap items-center justify-center gap-2.5 sm:gap-3">
      <Badge
        icon={Star}
        value={compact(stars)}
        label="stars"
        href={repoUrl}
      />
      <Badge
        icon={Users}
        value={`${contributors}`}
        label="contributors"
        href={`${repoUrl}/graphs/contributors`}
      />
      <Badge
        icon={Tag}
        value={version}
        label="latest"
        href={`${repoUrl}/releases/latest`}
      />
      <Badge
        icon={Scale}
        value="MIT"
        label="open source"
        href={`${repoUrl}/blob/main/LICENSE`}
      />
    </div>
  );
}
