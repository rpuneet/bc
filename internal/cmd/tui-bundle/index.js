#!/usr/bin/env node
// Stub — real TUI bundle is built by `make build-local-tui-bundle` and
// embedded into the bc binary. In dev checkouts before the first build,
// this stub is present so //go:embed doesn't fail.
// hasEmbeddedTUI() returns false for files smaller than 10KB, so the
// runtime falls back to tui/dist/index.js in dev mode.
console.error("TUI bundle not built. Run: make build-local-tui-bundle");
process.exit(1);
