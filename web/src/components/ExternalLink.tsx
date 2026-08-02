import type { AnchorHTMLAttributes, MouseEvent, ReactNode } from "react";
import { openExternal } from "../utils/openExternal";

type ExternalLinkProps = {
  href: string;
  children: ReactNode;
} & Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "target" | "rel">;

/**
 * ExternalLink renders an `<a>` that always hands the URL to {@link openExternal}
 * instead of relying on `target="_blank"`. A plain `target="_blank"` is a no-op
 * inside the desktop webview (Wails never injects `window.runtime` on the
 * daemon's http:// origin), so every external link must go through openExternal.
 *
 * The `href` is kept for right-click "copy link", middle-click, and screen
 * readers on the web build; the click is intercepted so the same code path runs
 * on both web (new tab) and desktop (daemon open-url).
 */
export function ExternalLink({ href, children, onClick, ...rest }: ExternalLinkProps) {
  const handleClick = (e: MouseEvent<HTMLAnchorElement>) => {
    // Let modified clicks (open-in-new-tab, etc.) behave natively on web.
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) {
      onClick?.(e);
      return;
    }
    e.preventDefault();
    openExternal(href);
    onClick?.(e);
  };
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" onClick={handleClick} {...rest}>
      {children}
    </a>
  );
}
