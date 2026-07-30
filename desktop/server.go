package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rpuneet/mycel/internal/cmd"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
)

// stopTimeout bounds how long window close waits for the server's
// graceful shutdown (HTTP drain + service closers) before giving up.
const stopTimeout = 15 * time.Second

// Server owns the embedded mycel server: the same boot path as
// `mycel up` (cmd.RunServerCtx), running in-process so the native
// window and the browser URL share one daemon on one localhost port.
type Server struct {
	cancel   context.CancelFunc
	done     chan error
	addr     string
	repoRoot string
	apiKey   string
}

// NewServer resolves listen address and anchor repo the same way
// `mycel up` does: MYCEL_WORKSPACE env or an enclosing adopted repo
// (rare for a GUI launch), preferences for host/port, else the
// default 127.0.0.1:9374. A repo-less boot is fine — the server
// comes up against MycelHome and repos are added from the web UI.
// An optional --addr flag overrides everything (same semantics as
// `mycel up --addr`), handy when another daemon already owns 9374.
func NewServer() *Server {
	repoRoot := resolveRepoRoot()
	addr := resolveListenAddr(repoRoot)
	if override := addrFlag(); override != "" {
		addr = override
	}
	return &Server{
		addr:     addr,
		repoRoot: repoRoot,
		apiKey:   os.Getenv("MYCEL_API_KEY"),
	}
}

// addrFlag parses an optional --addr host:port from the command line.
// GUI launches pass no args; parse errors from launcher-injected args
// are ignored rather than fatal.
func addrFlag() string {
	fs := flag.NewFlagSet("mycel-desktop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", "", "listen address (host:port)")
	_ = fs.Parse(os.Args[1:]) //nolint:errcheck // unknown launcher args are fine
	return *addr
}

// URL returns the browser-reachable base URL of the embedded server.
func (s *Server) URL() string { return "http://" + s.addr }

// Start launches the server goroutine. Errors surface on s.done;
// the boot page keeps polling /api/health, so a failed boot shows
// up as the window never leaving the "starting" state plus a log line.
func (s *Server) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan error, 1)

	publishDaemonAddr(s.URL())

	go func() {
		err := cmd.RunServerCtx(ctx, s.addr, s.repoRoot, "*", s.apiKey)
		if err != nil {
			log.Error("mycel server exited", "error", err)
		}
		s.done <- err
	}()
}

// Stop cancels the server context and waits (bounded) for the
// graceful shutdown to finish. Called from Wails OnShutdown when the
// window closes or the app quits.
func (s *Server) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(stopTimeout):
		log.Warn("server shutdown timed out", "timeout", stopTimeout)
	}
}

// resolveRepoRoot mirrors the CLI's repo resolution: explicit
// MYCEL_WORKSPACE wins, then the enclosing adopted repo of the
// current directory. GUI launches usually have neither — empty means
// a MycelHome-only boot.
func resolveRepoRoot() string {
	if p := os.Getenv("MYCEL_WORKSPACE"); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	h, err := home.Find(cwd)
	if err != nil || h == nil {
		return ""
	}
	return h.RootDir
}

// resolveListenAddr honors the global preferences (server.host /
// server.port) exactly like `mycel up` without --addr, falling back
// to the stock 127.0.0.1:9374.
func resolveListenAddr(repoRoot string) string {
	host, port := "127.0.0.1", 9374
	if repoRoot != "" {
		if h, err := home.Load(repoRoot); err == nil && h.Config != nil {
			if h.Config.Server.Host != "" {
				host = h.Config.Server.Host
			}
			if h.Config.Server.Port > 0 {
				port = h.Config.Server.Port
			}
		}
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// publishDaemonAddr replicates `mycel up`'s discovery side effects:
// MYCEL_DAEMON_ADDR for child processes (agent hooks) and the
// ~/.mycel/daemon.addr file the CLI reads on non-default ports.
// Best-effort with loud warnings, same as up.go.
func publishDaemonAddr(url string) {
	if err := os.Setenv("MYCEL_DAEMON_ADDR", url); err != nil {
		log.Warn("daemon addr: setenv failed", "error", err)
	}
	if _, err := home.EnsureGlobalDir(); err != nil {
		log.Warn("daemon addr: ensure ~/.mycel failed — CLI will fall back to default port", "error", err)
		return
	}
	addrPath, err := home.DaemonAddrPath()
	if err != nil {
		log.Warn("daemon addr: resolve path failed — CLI will fall back to default port", "error", err)
		return
	}
	if err := os.WriteFile(addrPath, []byte(url+"\n"), 0o600); err != nil {
		log.Warn("daemon addr: write failed — CLI will fall back to default port", "path", addrPath, "error", err)
	}
}
