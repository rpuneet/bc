/**
 * pickDirectory — ask the host OS for a folder path.
 *
 * Same desktop constraint as openExternal: the SPA runs on the daemon's
 * http://127.0.0.1 origin, so Wails never injects window.runtime. We POST to
 * /api/system/pick-directory and let the daemon show Finder / zenity /
 * FolderBrowserDialog.
 *
 * Returns the absolute path, or null when the user cancels / dialog fails /
 * the endpoint is unavailable (plain browser without a native dialog).
 */

import { isDesktop } from "./openExternal";

export async function pickDirectory(): Promise<string | null> {
  // Prefer the daemon dialog whenever we can reach it. Desktop always uses
  // it; browser UIs get it too when talking to a local daemon (loopback).
  try {
    const res = await fetch("/api/system/pick-directory", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}",
    });
    if (res.status === 204) return null; // canceled
    if (!res.ok) {
      // Non-desktop browsers may 502 (no zenity) — caller keeps typed path.
      if (!isDesktop()) return null;
      return null;
    }
    const body = (await res.json()) as { path?: string };
    return typeof body.path === "string" && body.path !== "" ? body.path : null;
  } catch {
    return null;
  }
}
