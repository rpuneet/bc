import { useState, type ReactNode } from "react";
import { BootSplash, type SplashTimings } from "./BootSplash";

/**
 * BootGate — shows the branded {@link BootSplash} while the daemon boots,
 * then reveals the app. Wrap the route tree with it: children stay
 * unmounted (so no app-level API calls fire) until the splash hands off,
 * at which point the router's current URL takes over — including the
 * existing first-run gate (`HomeGate` → `/welcome`), which we deliberately
 * do NOT duplicate here.
 *
 * `skip` bypasses the splash entirely (used by tests/embeds that don't
 * want the sequence); `timings` lets tests collapse the phase durations.
 */
export function BootGate({
  children,
  skip = false,
  timings,
}: {
  children: ReactNode;
  skip?: boolean;
  timings?: SplashTimings;
}) {
  const [ready, setReady] = useState(skip);
  if (!ready) {
    return <BootSplash onReady={() => setReady(true)} timings={timings} />;
  }
  return <>{children}</>;
}
