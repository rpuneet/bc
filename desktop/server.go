package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
	attached bool
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

// Start attaches to an already-running daemon when one answers at the
// resolved address (or at the address published in run/daemon.addr) —
// the desktop window is then a pure client and closing it leaves the
// daemon alone. Otherwise it launches the in-process server goroutine;
// errors surface on s.done and the boot page keeps polling /api/health,
// so a failed boot shows up as the window never leaving the "starting"
// state plus a log line.
func (s *Server) Start() {
	// Probe a few times before booting: a daemon that is mid-restart
	// (deploys swap the binary with down/up) answers a beat later, and
	// booting into that gap causes a port fight the moment it returns.
	for attempt := 0; attempt < 4; attempt++ {
		if addr, ok := runningDaemonAddr(s.addr); ok {
			s.addr = addr
			s.attached = true
			log.Info("attached to running mycel daemon", "url", s.URL())
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan error, 1)

	publishDaemonAddr(s.URL())

	go func() {
		err := cmd.RunServerCtx(ctx, s.addr, s.repoRoot, cmd.ResolveCORSOrigin(), s.apiKey)
		if err != nil && strings.Contains(err.Error(), "address already in use") {
			// Lost the port race to a daemon that came up between the
			// probe and the bind — become a client of it instead.
			if daemonHealthy(s.URL()) {
				s.attached = true
				log.Info("lost the port race — attached to the running daemon", "url", s.URL())
				s.done <- nil
				return
			}
		}
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
	if s.attached || s.cancel == nil {
		return
	}
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(stopTimeout):
		log.Warn("server shutdown timed out", "timeout", stopTimeout)
	}
	unpublishDaemonAddr(s.URL())
}

// unpublishDaemonAddr removes ~/.mycel/run/daemon.addr on shutdown,
// but only if it still points at this instance — a daemon that took
// over meanwhile owns the file.
func unpublishDaemonAddr(url string) {
	p, err := home.DaemonAddrPath()
	if err != nil {
		return
	}
	// #nosec G304 -- fixed path under ~/.mycel, not user input.
	b, err := os.ReadFile(p)
	if err != nil || strings.TrimSpace(string(b)) != url {
		return
	}
	if err := os.Remove(p); err != nil {
		log.Warn("daemon addr: cleanup failed", "error", err)
	}
}

// resolveRepoRoot mirrors the CLI's repo resolution: explicit
// MYCEL_WORKSPACE wins, then the enclosing adopted repo of the current
// directory, then the workspace the last daemon served.
//
// That last step is not a nicety. Tmux session names carry a hash of the
// workspace so two of them cannot collide, which means a daemon serving a
// different workspace cannot see the sessions of agents that are plainly still
// running. A Finder launch has a working directory of `/`, so the first two
// steps find nothing and the app used to serve no workspace at all — every
// agent listed as running, none of them attachable, and nothing in the UI
// naming the cause (#3569). The workspace the CLI daemon published is the
// closest thing to an answer the app can have without asking.
func resolveRepoRoot() string {
	if p := os.Getenv("MYCEL_WORKSPACE"); p != "" {
		return p
	}
	if cwd, err := os.Getwd(); err == nil {
		if h, err := home.Find(cwd); err == nil && h != nil {
			return h.RootDir
		}
	}
	return home.LastDaemonWorkspace()
}

// resolveListenAddr honors the global preferences (server.host /
// server.port) exactly like `mycel up` without --addr, falling back
// to the stock 127.0.0.1:9374.
func resolveListenAddr(repoRoot string) string {
	host, port := "127.0.0.1", 9374
	if h, err := home.Load(repoRoot); err == nil && h.Config != nil {
		if h.Config.Server.Host != "" {
			host = h.Config.Server.Host
		}
		if h.Config.Server.Port > 0 {
			port = h.Config.Server.Port
		}
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// runningDaemonAddr reports whether a mycel daemon already answers —
// first at the candidate host:port, then at the address published in
// ~/.mycel/run/daemon.addr (covers a daemon started with --addr).
// Returns the reachable host:port.
func runningDaemonAddr(candidate string) (string, bool) {
	if daemonHealthy("http://" + candidate) {
		return candidate, true
	}
	p, err := home.DaemonAddrPath()
	if err != nil {
		return "", false
	}
	// #nosec G304 -- fixed path under ~/.mycel, not user input.
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	url := strings.TrimSpace(string(b))
	if url == "" {
		return "", false
	}
	if !strings.Contains(url, "://") {
		url = "http://" + url
	}
	if daemonHealthy(url) {
		return strings.TrimPrefix(url, "http://"), true
	}
	return "", false
}

// daemonHealthy is a 1s bounded /api/health probe.
func daemonHealthy(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	return resp.StatusCode == http.StatusOK
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
