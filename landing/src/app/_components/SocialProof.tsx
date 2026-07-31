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
  index = 0,
}: {
  icon: React.ComponentType<{ className?: string }>;
  value: string;
  label: string;
  href: string;
  index?: number;
}) {
  return (
    <Link
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      style={{ animationDelay: `${index * 0.6}s` }}
      className="badge-alive group inline-flex items-center gap-2 rounded-full border border-outline-variant/25 bg-surface-container/60 px-4 py-2 transition-colors hover:border-primary/40 hover:bg-surface-container"
    >
      <Icon className="h-4 w-4 text-primary" aria-hidden="true" />
      <span className="font-headline text-[15px] font-semibold tabular-nums text-on-background">
        {value}
      </span>
      <span className="font-body text-sm text-on-surface-variant">{label}</span>
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
    <div className="flex flex-wrap items-center gap-2.5 sm:gap-3">
      {/* Subtle staggered bob so the row feels alive; motion is disabled by
         the global prefers-reduced-motion rule. */}
      <style>{`
        @keyframes badge-bob {
          0%, 100% { transform: translateY(0); }
          50%      { transform: translateY(-3px); }
        }
        .badge-alive { animation: badge-bob 3.6s ease-in-out infinite; will-change: transform; }
        .badge-alive:hover { animation-play-state: paused; }
      `}</style>
      <Badge icon={Star} value={compact(stars)} label="stars" href={repoUrl} index={0} />
      <Badge
        icon={Users}
        value={`${contributors}`}
        label="contributors"
        href={`${repoUrl}/graphs/contributors`}
        index={1}
      />
      <Badge
        icon={Tag}
        value={version}
        label="latest"
        href={`${repoUrl}/releases/latest`}
        index={2}
      />
      <Badge
        icon={Scale}
        value="MIT"
        label="open source"
        href={`${repoUrl}/blob/main/LICENSE`}
        index={3}
      />
    </div>
  );
}
