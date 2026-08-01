package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/tool"
)

// DepsInstallHandler runs a single dependency's install command on the host
// and streams its output back to the UI as newline-delimited JSON.
//
// Security: this executes a shell command, so it is loopback-only (same
// gate as the deps start/stop routes) and only ever runs commands sourced
// from a vetted table (installCommand) or the persisted tools registry —
// never a command supplied in the request body.
type DepsInstallHandler struct {
	tools *tool.Store
}

// NewDepsInstallHandler constructs a DepsInstallHandler.
func NewDepsInstallHandler() *DepsInstallHandler { return &DepsInstallHandler{} }

// SetToolStore wires the tools registry so registered CLI tools (gh, aws, …)
// can be installed/updated from their stored install_cmd / upgrade_cmd. Nil
// is fine — resolution then falls back to the vetted table only.
func (h *DepsInstallHandler) SetToolStore(s *tool.Store) *DepsInstallHandler {
	h.tools = s
	return h
}

// Register mounts POST /api/deps/install. It is a more specific pattern than
// the DepsHandler's "/api/deps/" prefix, so ServeMux routes it here.
func (h *DepsInstallHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/deps/install", h.install)
}

// depInstallRequest is the POST body: the readiness item / tool id to act on
// ("git", "tmux", "claude", "gh", …) and the action ("install", "update", or
// "uninstall"; empty means install).
type depInstallRequest struct {
	ID   string `json:"id"`
	Mode string `json:"mode,omitempty"`
}

// installRunner builds the command that runs a resolved install line. It is
// a package var so tests can substitute a harmless command.
var installRunner = func(ctx context.Context, shellCmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", shellCmd) //nolint:gosec // shellCmd comes from the vetted installCommand table
}

// install handles POST /api/deps/install.
func (h *DepsInstallHandler) install(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}
	var req depInstallRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		httpError(w, "id is required", http.StatusBadRequest)
		return
	}

	cmdLine, ok := h.resolveCommand(r.Context(), req.ID, req.Mode)
	if !ok {
		verb := "installer"
		switch req.Mode {
		case "update":
			verb = "updater"
		case "uninstall":
			verb = "uninstaller"
		}
		httpError(w, "no automatic "+verb+" for "+req.ID, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	emit := func(v any) bool {
		payload, mErr := json.Marshal(v)
		if mErr != nil {
			return false
		}
		if _, wErr := w.Write(append(payload, '\n')); wErr != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	emit(map[string]string{"type": "start", "command": cmdLine})
	streamInstall(r.Context(), cmdLine, emit)
}

// streamInstall runs cmdLine, emitting one {"type":"log","line":…} record
// per output line and a final {"type":"done","code":N} (or "error").
func streamInstall(ctx context.Context, cmdLine string, emit func(any) bool) {
	streamCmd(installRunner(ctx, cmdLine), emit)
}

// streamCmd runs an already-built *exec.Cmd, emitting one
// {"type":"log","line":…} record per output line and a final
// {"type":"done","code":N} (or {"type":"error",…}). It is the shared engine
// behind both the vetted-table installer (streamInstall, via `sh -c`) and the
// registry package installer (pkgsearch, via a direct argv), so both surface
// identical NDJSON progress to the UI.
func streamCmd(cmd *exec.Cmd, emit func(any) bool) {
	pr, pw := io.Pipe()
	// Closing the reader on the way out makes any further child writes fail
	// with io.ErrClosedPipe instead of blocking forever — so when a client
	// disconnects mid-stream (emit returns false and we return early), the
	// child's next write errors, cmd.Wait() completes, and the waiter
	// goroutine + process don't leak.
	defer func() { _ = pr.Close() }()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		emit(map[string]string{"type": "error", "error": err.Error()})
		return
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
		_ = pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		if !emit(map[string]string{"type": "log", "line": scanner.Text()}) {
			// Client disconnected — stop reading; the command keeps
			// running to completion in the background.
			return
		}
	}

	err := <-waitErr
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			emit(map[string]any{"type": "done", "code": ee.ExitCode()})
			return
		}
		emit(map[string]string{"type": "error", "error": err.Error()})
		return
	}
	emit(map[string]any{"type": "done", "code": 0})
}

// resolveCommand picks the shell command to run for id + mode.
//
//   - update: prefer the registered tool's upgrade_cmd, then its install_cmd,
//     then the vetted install table.
//   - uninstall: derive a remove command from the vetted install source
//     (npm/brew only); core system deps (git/tmux/docker) are never
//     auto-uninstalled.
//   - install (default): prefer the vetted install table, then the registered
//     tool's install_cmd.
//
// Every command originates from a vetted source (the table or the persisted
// tools registry), never from the request body.
func (h *DepsInstallHandler) resolveCommand(ctx context.Context, id, mode string) (string, bool) {
	switch mode {
	case "update":
		if t := h.lookupTool(ctx, id); t != nil {
			if cmd := strings.TrimSpace(t.UpgradeCmd); cmd != "" {
				return cmd, true
			}
			if cmd := strings.TrimSpace(t.InstallCmd); cmd != "" {
				return cmd, true
			}
		}
	case "uninstall":
		return h.resolveUninstall(ctx, id)
	}
	if cmd, ok := installCommand(id, runtime.GOOS); ok {
		return cmd, true
	}
	if t := h.lookupTool(ctx, id); t != nil {
		if cmd := strings.TrimSpace(t.InstallCmd); cmd != "" {
			return cmd, true
		}
	}
	return "", false
}

// coreSystemDeps are host tools mycel itself relies on. Auto-uninstalling them
// would break the daemon, so uninstall is refused for these ids.
var coreSystemDeps = map[string]bool{"git": true, "tmux": true, "docker": true}

// resolveUninstall derives a remove command for id from its vetted install
// source. Only npm-global and Homebrew installs map cleanly to an uninstall;
// curl-piped and apt installers return false so the UI shows an honest
// "no automatic uninstaller" rather than guessing a destructive command.
func (h *DepsInstallHandler) resolveUninstall(ctx context.Context, id string) (string, bool) {
	if coreSystemDeps[id] {
		return "", false
	}
	// Registered tool first (gh, aws, wrangler, …), then a provider CLI.
	if t := h.lookupTool(ctx, id); t != nil {
		if cmd, ok := deriveUninstall(t.InstallCmd); ok {
			return cmd, true
		}
	}
	if hint, ok := providerInstallCmd(id); ok {
		if cmd, ok := deriveUninstall(hint); ok {
			return cmd, true
		}
	}
	return "", false
}

// deriveUninstall converts a vetted install command into its uninstall
// counterpart for the two package managers whose grammar is unambiguous:
//
//	npm install -g <pkg>   → npm uninstall -g <pkg>
//	brew install <pkg>     → brew uninstall <pkg>
//
// Anything else (curl | sh, apt-get, pip, …) returns false — we never invent a
// remove command we can't be sure is safe.
func deriveUninstall(installCmd string) (string, bool) {
	cmd := strings.TrimSpace(installCmd)
	if cmd == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(cmd, "npm install -g "):
		pkg := strings.TrimSpace(strings.TrimPrefix(cmd, "npm install -g "))
		if pkg == "" {
			return "", false
		}
		return "npm uninstall -g " + pkg, true
	case strings.HasPrefix(cmd, "npm i -g "):
		pkg := strings.TrimSpace(strings.TrimPrefix(cmd, "npm i -g "))
		if pkg == "" {
			return "", false
		}
		return "npm uninstall -g " + pkg, true
	case strings.HasPrefix(cmd, "brew install "):
		pkg := strings.TrimSpace(strings.TrimPrefix(cmd, "brew install "))
		if pkg == "" {
			return "", false
		}
		return "brew uninstall " + pkg, true
	}
	return "", false
}

// lookupTool returns the registered tool named id, or nil when the store is
// absent or the tool is unknown.
func (h *DepsInstallHandler) lookupTool(ctx context.Context, id string) *tool.Tool {
	if h.tools == nil {
		return nil
	}
	t, err := h.tools.Get(ctx, id)
	if err != nil {
		return nil
	}
	return t
}

// installCommand resolves a readiness item id to a runnable install command
// for the given OS (runtime.GOOS). It returns ok=false for items with no
// safe automatic installer (e.g. Docker, or a provider whose install hint
// is just a URL) so the UI can fall back to guidance.
func installCommand(id, goos string) (string, bool) {
	switch id {
	case "git":
		if goos == "darwin" {
			return "brew install git", true
		}
		return "sudo apt-get update && sudo apt-get install -y git", true
	case "tmux":
		if goos == "darwin" {
			return "brew install tmux", true
		}
		return "sudo apt-get update && sudo apt-get install -y tmux", true
	case "docker":
		// Docker Desktop / engine installation is platform-specific and
		// interactive; the wizard links out rather than auto-installing.
		return "", false
	}
	return providerInstallCmd(id)
}

// providerInstallCmd returns a registered provider's install hint when it is
// a runnable command (npm/npx/curl/…). Hints that are bare URLs are treated
// as non-installable.
func providerInstallCmd(name string) (string, bool) {
	for _, p := range provider.ListProviders() {
		if p.Name() != name {
			continue
		}
		hint := strings.TrimSpace(p.InstallHint())
		if hint == "" || strings.HasPrefix(hint, "http://") || strings.HasPrefix(hint, "https://") {
			return "", false
		}
		return hint, true
	}
	return "", false
}
