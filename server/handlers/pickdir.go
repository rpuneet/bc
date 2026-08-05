package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PickDirectoryHandler exposes POST /api/system/pick-directory — a native
// folder dialog for the desktop/web UI (Finder on macOS).
//
// Same architecture as open-url: the SPA runs on the daemon's http://127.0.0.1
// origin, so Wails never injects window.runtime. The UI asks the daemon to
// show the host dialog instead.
//
// Security: loopback-only. No path is taken from the client; the OS dialog
// returns the chosen directory as a single argv-free stdout string.
type PickDirectoryHandler struct{}

// NewPickDirectoryHandler constructs a PickDirectoryHandler.
func NewPickDirectoryHandler() *PickDirectoryHandler { return &PickDirectoryHandler{} }

// Register mounts POST /api/system/pick-directory.
func (h *PickDirectoryHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/pick-directory", h.pick)
}

// ErrPickCanceled is returned when the user dismisses the folder dialog.
var ErrPickCanceled = errors.New("directory pick canceled")

// realPickDirectory shows a native folder chooser and returns the absolute path.
func realPickDirectory(ctx context.Context) (string, error) {
	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()

	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		// POSIX path of (choose folder …) — trailing slash stripped below.
		name = "osascript"
		args = []string{"-e", `POSIX path of (choose folder with prompt "Choose a folder")`}
	case "windows":
		name = "powershell"
		args = []string{"-NoProfile", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; ` +
				`$d = New-Object System.Windows.Forms.FolderBrowserDialog; ` +
				`$d.Description = 'Choose a folder'; ` +
				`if ($d.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) { exit 1 }; ` +
				`Write-Output $d.SelectedPath`}
	default:
		// Prefer zenity, then kdialog.
		if _, err := exec.LookPath("zenity"); err == nil {
			name, args = "zenity", []string{"--file-selection", "--directory", "--title=Choose a folder"}
		} else if _, err := exec.LookPath("kdialog"); err == nil {
			name, args = "kdialog", []string{"--getexistingdirectory", ".", "--title", "Choose a folder"}
		} else {
			return "", errors.New("no folder dialog available (install zenity or kdialog)")
		}
	}

	// #nosec G204 -- name is a fixed platform opener; args are constants.
	cmd := exec.CommandContext(launchCtx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if err != nil {
		// User cancel / dialog dismiss: empty stdout, non-zero exit.
		if out == "" {
			return "", ErrPickCanceled
		}
		return "", err
	}
	if out == "" {
		return "", ErrPickCanceled
	}
	// osascript returns a trailing slash; normalize.
	out = strings.TrimRight(out, `/\`)
	abs, absErr := filepath.Abs(out)
	if absErr != nil {
		return "", absErr
	}
	return filepath.Clean(abs), nil
}

// pickDirectoryFunc is injectable so tests can stub the OS dialog.
var pickDirectoryFunc = realPickDirectory

// pick handles POST /api/system/pick-directory. Empty body. Returns
// {"path":"/abs/dir"} on success, 204 when the user cancels, 502 on dialog failure.
func (h *PickDirectoryHandler) pick(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}

	path, err := pickDirectoryFunc(r.Context())
	if err != nil {
		if errors.Is(err, ErrPickCanceled) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httpError(w, "failed to open folder dialog: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}
