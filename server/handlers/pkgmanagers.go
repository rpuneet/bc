package handlers

import (
	"bufio"
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// PackageManagersHandler serves a read-only autodetect of the host OS and the
// package managers available on PATH. It shells out only to run each manager's
// own `--version` probe (never a user-supplied command), so it is safe to
// expose without the loopback gate the install stream requires.
//
// The result feeds the Tools page ("detected managers") and gives install/
// search flows an honest picture of what the host can actually do.
type PackageManagersHandler struct {
	cached time.Time
	cache  []PackageManager
	mu     sync.Mutex
}

// NewPackageManagersHandler constructs a PackageManagersHandler.
func NewPackageManagersHandler() *PackageManagersHandler { return &PackageManagersHandler{} }

// Register mounts GET /api/system/package-managers.
func (h *PackageManagersHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/package-managers", h.list)
}

// PackageManager is one detected (or candidate) package manager.
type PackageManager struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// Available is true when the binary resolves on PATH.
	Available bool `json:"available"`
	// Searchable is true when this manager exposes a registry search the
	// UI can drive (e.g. `brew search`, `npm search`). Honest metadata:
	// managers without a usable search are labeled so rather than faked.
	Searchable bool `json:"searchable"`
	// DirectInstall is true when the server can install a searched package
	// via this manager directly (no sudo, non-interactive — brew/npm/cargo).
	// The UI reads this instead of hard-coding the set, so its Install
	// affordance never drifts from the backend installSpecs.
	DirectInstall bool `json:"direct_install"`
}

// pmCandidate describes a manager to probe: its binary and the args that print
// a version. Whether it has a searchable registry is not stored here — it is
// derived from pkgManagerSearchable (searchSpecs) so the "searchable" bit can
// never drift from what the search endpoint actually wires.
type pmCandidate struct {
	id, name, binary, versionArg string
	// oses restricts a candidate to specific runtime.GOOS values; empty
	// means every OS.
	oses []string
}

// pmCandidates is the vetted set of managers we probe. Cross-platform tools
// (npm, pipx, cargo) are probed everywhere; OS-native managers are gated to
// the platforms they ship on so a Linux box isn't asked about winget.
var pmCandidates = []pmCandidate{
	{id: "brew", name: "Homebrew", binary: "brew", versionArg: "--version", oses: []string{"darwin", "linux"}},
	{id: "apt", name: "APT", binary: "apt-get", versionArg: "--version", oses: []string{"linux"}},
	{id: "dnf", name: "DNF", binary: "dnf", versionArg: "--version", oses: []string{"linux"}},
	{id: "yum", name: "YUM", binary: "yum", versionArg: "--version", oses: []string{"linux"}},
	{id: "pacman", name: "pacman", binary: "pacman", versionArg: "--version", oses: []string{"linux"}},
	{id: "zypper", name: "Zypper", binary: "zypper", versionArg: "--version", oses: []string{"linux"}},
	{id: "winget", name: "winget", binary: "winget", versionArg: "--version", oses: []string{"windows"}},
	{id: "npm", name: "npm", binary: "npm", versionArg: "--version"},
	{id: "pipx", name: "pipx", binary: "pipx", versionArg: "--version"},
	{id: "cargo", name: "Cargo", binary: "cargo", versionArg: "--version"},
}

// pmLookPath and pmRunVersion are package vars so tests can inject a stub host
// without shelling out to real binaries.
var (
	pmLookPath   = exec.LookPath
	pmRunVersion = func(ctx context.Context, binary, arg string) (string, bool) {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cctx, binary, arg).CombinedOutput() //nolint:gosec // binary+arg come from the vetted pmCandidates table
		if err != nil {
			return "", false
		}
		return firstLine(string(out)), true
	}
)

// firstLine returns the first non-empty, trimmed line of s.
func firstLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			return line
		}
	}
	return ""
}

// detect probes every OS-relevant candidate and returns those found on PATH,
// each with its reported version. Results are cached briefly so the polling UI
// doesn't re-shell every few seconds.
func (h *PackageManagersHandler) detect(ctx context.Context) []PackageManager {
	const ttl = 30 * time.Second
	h.mu.Lock()
	if h.cache != nil && time.Since(h.cached) < ttl {
		out := h.cache
		h.mu.Unlock()
		return out
	}
	h.mu.Unlock()

	goos := runtime.GOOS
	found := make([]PackageManager, 0, len(pmCandidates))
	for _, c := range pmCandidates {
		if len(c.oses) > 0 && !pmOSMatch(c.oses, goos) {
			continue
		}
		if _, err := pmLookPath(c.binary); err != nil {
			continue
		}
		version := ""
		if v, ok := pmRunVersion(ctx, c.binary, c.versionArg); ok {
			version = v
		}
		found = append(found, PackageManager{
			ID:            c.id,
			Name:          c.name,
			Version:       version,
			Available:     true,
			Searchable:    pkgManagerSearchable(c.id),
			DirectInstall: pkgManagerDirectInstall(c.id),
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })

	h.mu.Lock()
	h.cache = found
	h.cached = time.Now()
	h.mu.Unlock()
	return found
}

func pmOSMatch(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// list handles GET /api/system/package-managers.
func (h *PackageManagersHandler) list(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	managers := h.detect(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"managers": managers,
	})
}
