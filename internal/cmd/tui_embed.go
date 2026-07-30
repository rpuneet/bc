package cmd

import (
	"embed"
)

// tuiBundleFS holds the precompiled TUI bundle (single JS file produced by
// `bun build --minify`). At runtime the TUI is extracted to a temp directory
// and executed via `bun run`.
//
// The bundle is built by `make build-local-tui-bundle` which writes to
// internal/cmd/tui-bundle/index.js. That file is a build artifact and is not
// tracked in git; dev checkouts without it fall back to tui/dist/index.js.
// The directory embed (with a tracked placeholder) keeps `go build` working
// when the bundle is absent.
//
//go:embed tui-bundle
var tuiBundleFS embed.FS

// tuiBundleJS is the embedded bundle contents, or nil in dev checkouts
// where the bundle was not built before compiling.
var tuiBundleJS = func() []byte {
	b, err := tuiBundleFS.ReadFile("tui-bundle/index.js")
	if err != nil {
		return nil
	}
	return b
}()

// hasEmbeddedTUI reports whether a real TUI bundle is embedded.
func hasEmbeddedTUI() bool {
	return len(tuiBundleJS) > 10_000
}
