package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/provider"
)

// providerRunTimeout bounds a single subcommand run so a hung CLI can't wedge
// the request.
const providerRunTimeout = 20 * time.Second

// providerRunOutputCap limits captured output so a chatty command can't
// balloon the response.
const providerRunOutputCap = 64 * 1024

// providerRunResponse is the result of executing one allowlisted, runnable
// provider subcommand.
type providerRunResponse struct { //nolint:govet // field order matches JSON/API contract
	Command   string `json:"command"`
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
}

// providerRunRunner executes bin with args and returns combined output and the
// exit code. It is a package var so tests can substitute a harmless command
// without shelling out to a real provider CLI.
//
// Security: bin and args come verbatim from the provider's curated Commands()
// table (see resolveRunnable) — never from the request body, which only
// selects which allowlisted entry to run. The command is exec'd directly with
// an argument slice (no shell, no `sh -c`), so no request data is ever
// interpreted by a shell. This is the CodeQL-recommended shape for
// go/command-line-injection.
var providerRunRunner = func(ctx context.Context, bin string, args []string) (string, int, error) {
	cctx, cancel := context.WithTimeout(ctx, providerRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...) //nolint:gosec // bin+args are a constant argv from the provider's curated Commands() allowlist, not request input
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			return string(out), -1, err
		}
	}
	return string(out), code, nil
}

// runCommand handles POST /api/providers/{name}/run. It executes a single
// allowlisted, non-interactive, no-argument subcommand from the provider's
// curated Commands() list and returns its output inline.
//
// Guards, in order: loopback-only, provider must exist, the requested command
// must resolve to a curated entry that is Runnable() (not interactive, no
// required args), and the executed argv comes from that entry — never the
// request body. Interactive/arg commands are refused with 400 so the UI shows
// the honest "run in your terminal" path instead.
func (h *ProviderHandler) runCommand(w http.ResponseWriter, r *http.Request, name string) {
	if !checkLoopback(w, r) {
		return
	}

	p, ok := h.registry.Get(name)
	if !ok {
		httpError(w, "unknown provider: "+name, http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		httpError(w, "command is required", http.StatusBadRequest)
		return
	}

	bin, args, ok := resolveRunnableArgv(p, req.Command)
	if !ok {
		httpError(w, "command is not runnable from the UI (unknown, interactive, or requires arguments): "+req.Command, http.StatusBadRequest)
		return
	}

	out, code, runErr := providerRunRunner(r.Context(), bin, args)
	if runErr != nil {
		httpError(w, "failed to run command: "+runErr.Error(), http.StatusBadGateway)
		return
	}

	truncated := false
	if len(out) > providerRunOutputCap {
		out = out[:providerRunOutputCap]
		truncated = true
	}

	writeJSON(w, http.StatusOK, providerRunResponse{
		Command:   strings.Join(append([]string{bin}, args...), " "),
		Output:    out,
		ExitCode:  code,
		Truncated: truncated,
	})
}

// resolveRunnableArgv returns the exact binary + argument slice to exec for the
// curated command whose display Name equals cmdName.
//
// CodeQL barrier (go/command-injection): the executed argv is built ONLY from
// the provider's own curated Commands() literals — constant strings compiled
// into the binary (see commands.go / claude.go). The request-derived cmdName is
// used solely in the `c.Name != cmdName` equality check to *select* an entry;
// it never populates the returned bin/args. The generic default in
// providerCommands (which interpolates the request-derived provider name into a
// command string) is deliberately NOT consulted here — providers without a
// CommandLister expose no runnable commands, so no request-derived string can
// ever reach exec. This makes the taint barrier explicit for the analyzer.
func resolveRunnableArgv(p provider.Provider, cmdName string) (string, []string, bool) {
	cl, ok := p.(provider.CommandLister)
	if !ok {
		// Only curated constant command lists are executable. Providers with
		// only the generic (name-interpolated) default are never run.
		return "", nil, false
	}
	for _, c := range cl.Commands() {
		if c.Name != cmdName {
			continue
		}
		if !c.Runnable() {
			return "", nil, false
		}
		// c.Command is a constant literal from the curated table; splitting it
		// yields a constant argv with no request-derived data.
		fields := strings.Fields(c.Command)
		if len(fields) == 0 {
			return "", nil, false
		}
		return fields[0], fields[1:], true
	}
	return "", nil, false
}
