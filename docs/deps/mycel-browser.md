# mycel-browser — Deprecated Optional Dependency

> **Status:** Documented / deprioritized &nbsp;|&nbsp; **Updated:** 2026-04-16
>
> Related: [docs/proposals/multi-workspace-and-code-tab.md §7](../proposals/multi-workspace-and-code-tab.md)

## Summary

`mycel-browser` was planned as an optional dependency that runs a headed Playwright
(visible browser) service inside Docker, intended to let agents drive a browser
for web automation tasks. It is **no longer a priority** because:

- Claude Code ships with a built-in **browser plugin** (`/plugin install browser`)
  that covers the same use-case without an extra service.
- The existing headless `playwright` Docker image is already built via
  `make build-docker-playwright` and is used by the Playwright MCP server for
  test automation; that path is still supported.
- Running a headed browser in Docker on macOS requires XQuartz or similar
  X11 forwarding, which is fragile and a poor UX.

## Current state

- The make target `build-docker-playwright` still builds the headless image used
  by the Playwright MCP server; no changes.
- In the Dependencies manager UI (Settings → Dependencies), `mycel-browser` appears
  but is labeled **Deprecated** and its start button is disabled.
- `POST /api/deps/mycel-browser/start` returns `409 Conflict` with the body:
  ```json
  { "error": "mycel-browser is deprecated; use Claude Code's built-in browser plugin instead", "doc": "docs/deps/mycel-browser.md" }
  ```

## If you still need a headed browser

1. Install the Claude Code browser plugin:
   ```
   /plugin install browser
   ```
2. For headless automation, keep using the Playwright MCP server:
   ```
   make build-docker-playwright
   ```
3. For local development with a visible browser, just use your host's browser
   with the `chrome-devtools` MCP or similar — no extra bc service required.

## Revisiting

If a strong use-case emerges (e.g. a remote multi-tenant scenario where agents
truly need a sandboxed headed browser and the Claude Code plugin is unsuitable),
this dependency can be un-deprecated by:

1. Adding a proper implementation to `pkg/deps/bc_browser.go`
2. Wiring the Start/Stop/Status/Logs interface against the
   `docker/playwright-visible/` Dockerfile (when/if it exists and is stable)
3. Removing the `409 Conflict` response in the deps API
4. Updating this doc with a plan-of-record

Until then, this dep is visible-but-inert so users know it was considered.
