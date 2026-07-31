"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Apple, Monitor, Terminal, Download } from "lucide-react";
import {
  type DesktopOS,
  desktopDownloadUrl,
  detectOS,
  useGitHubStats,
} from "../../lib/github";

const OS_META: Record<
  DesktopOS,
  { label: string; short: string; icon: React.ComponentType<{ className?: string }> }
> = {
  mac: { label: "Download for macOS", short: "macOS", icon: Apple },
  linux: { label: "Download for Linux", short: "Linux", icon: Terminal },
  windows: { label: "Download for Windows", short: "Windows", icon: Monitor },
};

const ORDER: DesktopOS[] = ["mac", "linux", "windows"];

/**
 * Hero download buttons (owner decision #3): the primary conversion path.
 * Detects the visitor's OS and promotes that build as the filled primary
 * button; the other two are quiet secondary links. The curl/brew CLI install
 * lives below the fold in InstallSection as the power-user path.
 *
 * The desktop assets these point at are produced by release.yml's Wails job.
 * They 404 until the first desktop release (v0.4.0) is tagged — expected; the
 * /releases/latest/download/ URLs resolve the moment that release exists.
 */
export function DownloadButtons() {
  const { version } = useGitHubStats();
  // Default to mac for SSR/first paint (matches the flagship build); refine
  // to the real OS after mount so the markup is stable and hydration-safe.
  const [os, setOs] = useState<DesktopOS>("mac");
  useEffect(() => {
    // Defer past the effect body so the React compiler's
    // set-state-in-effect rule is satisfied (same pattern as useMounted).
    const id = setTimeout(() => setOs(detectOS()), 0);
    return () => clearTimeout(id);
  }, []);

  const primary = OS_META[os];
  const PrimaryIcon = primary.icon;
  const others = ORDER.filter((o) => o !== os);

  return (
    <div className="flex flex-col items-center gap-3">
      <Link
        href={desktopDownloadUrl(os, version)}
        className="group inline-flex h-12 items-center gap-2.5 rounded-lg bg-primary px-7 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-[0_0_24px_color-mix(in_srgb,var(--primary)_32%,transparent)] active:scale-[0.97]"
      >
        <PrimaryIcon className="h-[18px] w-[18px]" aria-hidden="true" />
        {primary.label}
        <Download
          className="h-3.5 w-3.5 opacity-70 transition-transform group-hover:translate-y-0.5"
          aria-hidden="true"
        />
      </Link>

      <div className="flex items-center gap-2">
        {others.map((o) => {
          const meta = OS_META[o];
          const Icon = meta.icon;
          return (
            <Link
              key={o}
              href={desktopDownloadUrl(o, version)}
              className="inline-flex items-center gap-1.5 rounded-md border border-outline-variant/20 px-3 py-1.5 font-body text-xs font-medium text-on-surface-variant transition-colors hover:border-primary/30 hover:text-primary active:scale-[0.97]"
            >
              <Icon className="h-3.5 w-3.5" aria-hidden="true" />
              {meta.short}
            </Link>
          );
        })}
      </div>
    </div>
  );
}
