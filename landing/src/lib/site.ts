/**
 * Canonical site URL, used for metadata, Open Graph, structured data and
 * sitemaps. Single source of truth — change here to repoint the whole site.
 *
 * NOTE: static files that can't import this (public/sitemap.xml,
 * public/robots.txt, public/llms.txt) hard-code the same value; keep them
 * in sync when this changes.
 */
export const SITE_URL = "https://bc-infra.com";

/** Absolute URL for a path on the site, e.g. absoluteUrl("/docs"). */
export function absoluteUrl(path = ""): string {
  return `${SITE_URL}${path.startsWith("/") || path === "" ? path : `/${path}`}`;
}
