"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Apple, Monitor, Terminal, Download } from "lucide-react";
import {
  type DesktopOS,
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
 * Desktop URLs come from the latest GitHub release asset list (signed zip
 * preferred; falls back to `*-UNSIGNED.zip` when that is what shipped).
 * Missing OS assets are hidden — never linked to a filename that is not
 * on the release (#3619 CodeRabbit).
 */
export function DownloadButtons() {
  const { desktopUrls } = useGitHubStats();
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
  const primaryHref = desktopUrls[os];
  const others = ORDER.filter((o) => o !== os && desktopUrls[o]);
  const unsignedMac = Boolean(desktopUrls.mac && /UNSIGNED/.test(desktopUrls.mac));

  return (
    <div className="flex flex-col items-center gap-3">
      {primaryHref ? (
        <Link
          href={primaryHref}
          className="group inline-flex h-12 items-center gap-2.5 rounded-lg bg-primary px-7 text-sm font-semibold text-primary-foreground shadow-[var(--btn-shadow)] transition-all hover:shadow-[0_0_24px_color-mix(in_srgb,var(--primary)_32%,transparent)] active:scale-[0.97]"
        >
          <PrimaryIcon className="h-[18px] w-[18px]" aria-hidden="true" />
          {primary.label}
          <Download
            className="h-3.5 w-3.5 opacity-70 transition-transform group-hover:translate-y-0.5"
            aria-hidden="true"
          />
        </Link>
      ) : (
        <span
          className="inline-flex h-12 items-center gap-2.5 rounded-lg border border-outline-variant/30 px-7 text-sm font-semibold text-on-surface-variant"
          aria-disabled="true"
        >
          <PrimaryIcon className="h-[18px] w-[18px]" aria-hidden="true" />
          {primary.short} build unavailable
        </span>
      )}

      {others.length > 0 ? (
        <div className="flex items-center gap-2">
          {others.map((o) => {
            const meta = OS_META[o];
            const Icon = meta.icon;
            const href = desktopUrls[o]!;
            return (
              <Link
                key={o}
                href={href}
                className="inline-flex items-center gap-1.5 rounded-md border border-outline-variant/20 px-3 py-1.5 font-body text-xs font-medium text-on-surface-variant transition-colors hover:border-primary/30 hover:text-primary active:scale-[0.97]"
              >
                <Icon className="h-3.5 w-3.5" aria-hidden="true" />
                {meta.short}
              </Link>
            );
          })}
        </div>
      ) : null}

      {unsignedMac && os === "mac" ? (
        <p className="max-w-sm text-center font-body text-xs text-on-surface-variant/80">
          macOS build is unsigned until Developer ID signing lands. After
          download:{" "}
          <code className="rounded bg-surface-container px-1 py-0.5 text-[11px]">
            xattr -dr com.apple.quarantine /path/to/mycel.app
          </code>
        </p>
      ) : null}
    </div>
  );
}
