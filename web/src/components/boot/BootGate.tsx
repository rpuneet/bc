import { useState, type ReactNode } from "react";
import { BootSplash, type SplashTimings } from "./BootSplash";
import { markBootSplashDone, shouldSkipBootSplash } from "./desktopBoot";

/**
 * BootGate — desktop-only branded {@link BootSplash} while the daemon is
 * probed, then reveals the app. The browser SPA skips the splash entirely
 * (#3673): open http://127.0.0.1:9374 and you land on the UI immediately.
 *
 * Desktop handoff (`?desktop=1` from `desktop/bootpage.go`) is the only
 * path that shows the animated splash with real readiness/log lines.
 *
 * `skip` forces the gate (tests/embeds). When omitted, skip is derived from
 * {@link shouldSkipBootSplash}.
 */
export function BootGate({
  children,
  skip,
  timings,
}: {
  children: ReactNode;
  /** Force skip (true) or force splash (false). Omit for production default. */
  skip?: boolean;
  timings?: SplashTimings;
}) {
  const [ready, setReady] = useState(() => shouldSkipBootSplash(skip));
  if (!ready) {
    return (
      <BootSplash
        onReady={() => {
          markBootSplashDone();
          setReady(true);
        }}
        timings={timings}
      />
    );
  }
  return <>{children}</>;
}
