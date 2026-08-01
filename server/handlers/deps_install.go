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
// ("git", "tmux", "claude", "gh", …) and the action ("install" or "update";
// empty means install).
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
		if req.Mode == "update" {
			verb = "updater"
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
	cmd := installRunner(ctx, cmdLine)
	pr, pw := io.Pipe()
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
//   - install (default): prefer the vetted install table, then the registered
//     tool's install_cmd.
//
// Every command originates from a vetted source (the table or the persisted
// tools registry), never from the request body.
func (h *DepsInstallHandler) resolveCommand(ctx context.Context, id, mode string) (string, bool) {
	if mode == "update" {
		if t := h.lookupTool(ctx, id); t != nil {
			if cmd := strings.TrimSpace(t.UpgradeCmd); cmd != "" {
				return cmd, true
			}
			if cmd := strings.TrimSpace(t.InstallCmd); cmd != "" {
				return cmd, true
			}
		}
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
