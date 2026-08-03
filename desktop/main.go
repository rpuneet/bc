// Package main is the mycel desktop app: a native window around the
// mycel server, in the spirit of Docker Desktop. The server runs
// in-process on its normal localhost port (default 127.0.0.1:9374),
// so http://127.0.0.1:9374 keeps working in any browser while the
// window is open; closing the window shuts the server down cleanly.
package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/rpuneet/mycel/internal/cmd"
	"github.com/rpuneet/mycel/pkg/log"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version information set by ldflags during build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)

	srv := NewServer()

	err := wails.Run(&options.App{
		Title:     "mycel",
		Width:     1280,
		Height:    820,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: bootMiddleware(srv.URL(), version),
		},
		OnStartup:  func(context.Context) { srv.Start() },
		OnShutdown: func(context.Context) { srv.Stop() },
	})
	if err != nil {
		log.Error("desktop app failed", "error", err)
		os.Exit(1)
	}
}
