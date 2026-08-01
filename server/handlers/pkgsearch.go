package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// PackageSearchHandler exposes a guarded registry search + install for the
// host's package managers, feeding the Tools page's "search the registry"
// surface.
//
// Security model (this shells out, so it is hardened accordingly):
//   - The manager id must resolve to a vetted spec in searchSpecs / installSpecs
//     — never a name from the request drives the binary.
//   - The query / package name must match a strict charset (pkgQueryPattern /
//     pkgNamePattern); anything with shell metacharacters, whitespace, or
//     control bytes is rejected with 400 before any exec.
//   - Every command is exec'd directly with an argument slice (never `sh -c`),
//     so the validated token is passed as one argv element and no request data
//     is ever interpreted by a shell. This is the CodeQL-recommended shape for
//     go/command-line-injection.
//   - Both endpoints are loopback-only and time-bounded; results are capped.
type PackageSearchHandler struct{}

// NewPackageSearchHandler constructs a PackageSearchHandler.
func NewPackageSearchHandler() *PackageSearchHandler { return &PackageSearchHandler{} }

// Register mounts the search + install routes. Both are more specific than any
// prefix route, so ServeMux dispatches them here.
func (h *PackageSearchHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/package-search", h.search)
	mux.HandleFunc("/api/system/package-install", h.install)
}

const (
	// pkgSearchTimeout bounds a registry search (searches hit the network).
	pkgSearchTimeout = 20 * time.Second
	// pkgSearchOutputCap limits bytes read from a search command.
	pkgSearchOutputCap = 1 << 20 // 1 MiB
	// pkgSearchResultCap limits how many parsed results are returned.
	pkgSearchResultCap = 50
)

// pkgQueryPattern is the strict charset a search term must match: a leading
// alphanumeric, then package-name-ish characters only. No whitespace, no shell
// metacharacters, no path traversal. Deliberately conservative — a package
// name, not a free-text query.
var pkgQueryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

// pkgNamePattern is the charset an installable package name must match. It
// accepts exactly two documented shapes and nothing else:
//   - a plain package name: leading alphanumeric, then name characters (no '/')
//   - a proper npm scope: "@scope/name", a single '/' separating one anchored
//     scope segment from one anchored name segment
//
// Because each segment must start with an alphanumeric and '/' only appears
// between an anchored scope and name, this rejects leading '-' (argv flag
// injection like "-g"/"--force"), bare or duplicate '/', trailing '/', '..'
// path traversal ("a/../../b"), and absolute/relative paths ("/etc/x", "./x").
var pkgNamePattern = regexp.MustCompile(`^(@[A-Za-z0-9][A-Za-z0-9._-]{0,63}/)?[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// searchResult is one registry hit.
type searchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// searchSpec describes how to search one manager's registry.
type searchSpec struct {
	// args builds the argv for a validated query.
	args func(query string) []string
	// parse extracts results from the command's combined output.
	parse func(raw string) []searchResult
	// bin is the binary to exec (a constant, never request-derived).
	bin string
}

// installSpec describes how to install a validated package via one manager.
// Only managers whose install is non-interactive and needs no sudo are listed
// — the rest surface a copyable command in the UI instead.
type installSpec struct {
	args func(pkg string) []string
	bin  string
}

// searchSpecs is the vetted set of managers whose registry the UI can search.
// A manager absent here is honestly labeled "search unavailable".
var searchSpecs = map[string]searchSpec{
	"brew":   {bin: "brew", args: func(q string) []string { return []string{"search", q} }, parse: parseLineNames},
	"npm":    {bin: "npm", args: func(q string) []string { return []string{"search", "--no-color", "--parseable", q} }, parse: parseNpmParseable},
	"apt":    {bin: "apt-cache", args: func(q string) []string { return []string{"search", q} }, parse: parseColonDesc},
	"dnf":    {bin: "dnf", args: func(q string) []string { return []string{"--quiet", "search", q} }, parse: parseColonDesc},
	"yum":    {bin: "yum", args: func(q string) []string { return []string{"--quiet", "search", q} }, parse: parseColonDesc},
	"pacman": {bin: "pacman", args: func(q string) []string { return []string{"-Ss", q} }, parse: parsePacman},
	"zypper": {bin: "zypper", args: func(q string) []string { return []string{"--quiet", "search", q} }, parse: parseLineNames},
	"cargo":  {bin: "cargo", args: func(q string) []string { return []string{"search", q} }, parse: parseCargo},
	"winget": {bin: "winget", args: func(q string) []string { return []string{"search", q} }, parse: parseLineNames},
}

// installSpecs is the subset of managers whose install we run directly (no
// sudo, non-interactive). Others: the UI shows a copyable command.
var installSpecs = map[string]installSpec{
	"brew":  {bin: "brew", args: func(pkg string) []string { return []string{"install", pkg} }},
	"npm":   {bin: "npm", args: func(pkg string) []string { return []string{"install", "-g", pkg} }},
	"cargo": {bin: "cargo", args: func(pkg string) []string { return []string{"install", pkg} }},
}

// pkgManagerSearchable reports whether the UI can drive a registry search for
// this manager id. Single source of truth for the detect surface's Searchable
// flag, so the picture never drifts from what's actually wired.
func pkgManagerSearchable(id string) bool {
	_, ok := searchSpecs[id]
	return ok
}

// pkgManagerDirectInstall reports whether the server can install a searched
// package via this manager directly (no sudo). Single source of truth for the
// UI's Install affordance, derived from installSpecs so it can't drift.
func pkgManagerDirectInstall(id string) bool {
	_, ok := installSpecs[id]
	return ok
}

// limitedWriter writes at most n bytes to w and silently discards the rest,
// always reporting a full write so the child process is never killed by a
// short-write error. It bounds captured memory *during* the read rather than
// buffering the whole subprocess output and truncating afterwards.
type limitedWriter struct {
	w         io.Writer
	n         int
	truncated bool
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	keep := p
	if len(keep) > l.n {
		keep = keep[:l.n]
		l.truncated = true
	}
	if l.n > 0 {
		if _, err := l.w.Write(keep); err != nil {
			return 0, err
		}
		l.n -= len(keep)
	}
	return len(p), nil
}

// runCapped runs cmd, capturing at most limit bytes of combined stdout+stderr
// via a limitedWriter so a chatty subprocess can't balloon memory, and reports
// whether output was truncated. WaitDelay bounds the wait so a killed child that
// leaked a pipe to a grandchild can't wedge the read indefinitely.
func runCapped(cmd *exec.Cmd, limit int) (out []byte, truncated bool, err error) {
	var buf bytes.Buffer
	lw := &limitedWriter{w: &buf, n: limit}
	cmd.Stdout = lw
	cmd.Stderr = lw
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	return buf.Bytes(), lw.truncated, err
}

// pkgSearchRunner runs a search command and returns bounded combined output.
// Package var so tests can inject a stub host.
var pkgSearchRunner = func(ctx context.Context, bin string, args []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, pkgSearchTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...) //nolint:gosec // bin is a constant from searchSpecs; args carry a charset-validated query as a slice element (no shell)
	out, _, err := runCapped(cmd, pkgSearchOutputCap)
	return out, err
}

// pkgInstallRunner builds the *exec.Cmd for a package install. Package var so
// tests can substitute a harmless command.
var pkgInstallRunner = func(ctx context.Context, bin string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin is a constant from installSpecs; args carry a charset-validated package name as a slice element (no shell)
}

// search handles POST /api/system/package-search.
func (h *PackageSearchHandler) search(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}

	var req struct {
		Manager string `json:"manager"`
		Query   string `json:"query"`
	}
	if !decodePkgBody(w, r, &req) {
		return
	}
	req.Manager = strings.TrimSpace(req.Manager)
	req.Query = strings.TrimSpace(req.Query)

	spec, ok := searchSpecs[req.Manager]
	if !ok {
		httpError(w, "search unavailable for manager: "+req.Manager, http.StatusBadRequest)
		return
	}
	if !pkgQueryPattern.MatchString(req.Query) {
		httpError(w, "invalid query: must be a package name (letters, digits, . _ + -), 1-64 chars", http.StatusBadRequest)
		return
	}

	out, err := pkgSearchRunner(r.Context(), spec.bin, spec.args(req.Query))
	results := spec.parse(string(out))
	if len(results) > pkgSearchResultCap {
		results = results[:pkgSearchResultCap]
	}
	// A non-zero exit with zero results is a real failure (e.g. brew/apt
	// found nothing or the binary errored); surface it honestly. A non-zero
	// exit that still parsed results (some managers exit 1 on "no matches"
	// after printing a header) is treated as an empty/normal result set.
	if err != nil && len(results) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"manager": req.Manager,
			"query":   req.Query,
			"results": []searchResult{},
			"error":   "no results (the search command reported an error or found nothing)",
		})
		return
	}

	if results == nil {
		results = []searchResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"manager": req.Manager,
		"query":   req.Query,
		"results": results,
	})
}

// install handles POST /api/system/package-install: streams the install of a
// validated package via an allowlisted manager as NDJSON, reusing the same
// stream engine as the vetted-table installer.
func (h *PackageSearchHandler) install(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !checkLoopback(w, r) {
		return
	}

	var req struct {
		Manager string `json:"manager"`
		Package string `json:"package"`
	}
	if !decodePkgBody(w, r, &req) {
		return
	}
	req.Manager = strings.TrimSpace(req.Manager)
	req.Package = strings.TrimSpace(req.Package)

	spec, ok := installSpecs[req.Manager]
	if !ok {
		httpError(w, "no direct installer for manager: "+req.Manager+" (copy the command and run it yourself)", http.StatusBadRequest)
		return
	}
	if !pkgNamePattern.MatchString(req.Package) {
		httpError(w, "invalid package name", http.StatusBadRequest)
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

	args := spec.args(req.Package)
	emit(map[string]string{"type": "start", "command": strings.Join(append([]string{spec.bin}, args...), " ")})
	streamCmd(pkgInstallRunner(r.Context(), spec.bin, args), emit)
}

// decodePkgBody reads and JSON-decodes a small request body into dst, writing
// a 400 and returning false on failure.
func decodePkgBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpError(w, "failed to read body", http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		httpError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ── parsers ───────────────────────────────────────────────────────────
// Each converts one manager's search output into name/description pairs.
// They tolerate headers, blank lines, and formatting noise rather than
// assuming a rigid layout.

// parseLineNames treats each non-empty, single-token line as a package name
// (brew/zypper/winget style). Lines with spaces (headers, "==> Formulae") are
// skipped so section banners don't leak in.
func parseLineNames(raw string) []searchResult {
	var out []searchResult
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "==>") {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			continue
		}
		out = append(out, searchResult{Name: line})
	}
	return out
}

// parseNpmParseable parses `npm search --parseable` output: tab-separated
// name, description, author, date, version.
func parseNpmParseable(raw string) []searchResult {
	var out []searchResult
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		name := strings.TrimSpace(cols[0])
		if name == "" {
			continue
		}
		desc := ""
		if len(cols) > 1 {
			desc = strings.TrimSpace(cols[1])
		}
		out = append(out, searchResult{Name: name, Description: desc})
	}
	return out
}

// parseColonDesc parses "name - description" lines (apt-cache / dnf / yum).
func parseColonDesc(raw string) []searchResult {
	var out []searchResult
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "=") {
			continue
		}
		name, desc, found := strings.Cut(line, " - ")
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " \t") {
			// dnf prints "name.arch : desc"; fall back to that shape.
			if n, d, ok := strings.Cut(line, " : "); ok {
				name, desc, found = strings.TrimSpace(n), d, true
			}
		}
		if !found || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		out = append(out, searchResult{Name: name, Description: strings.TrimSpace(desc)})
	}
	return out
}

// parsePacman parses `pacman -Ss` output: a "repo/name version" header line
// followed by an indented description line.
func parsePacman(raw string) []searchResult {
	var out []searchResult
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var pending *searchResult
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if pending != nil {
				pending.Description = strings.TrimSpace(line)
				out = append(out, *pending)
				pending = nil
			}
			continue
		}
		if pending != nil {
			out = append(out, *pending)
			pending = nil
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		pending = &searchResult{Name: name}
	}
	if pending != nil {
		out = append(out, *pending)
	}
	return out
}

// parseCargo parses `cargo search` output: `name = "version"    # description`.
func parseCargo(raw string) []searchResult {
	var out []searchResult
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "...") {
			continue
		}
		name, rest, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		desc := ""
		if _, d, ok := strings.Cut(rest, "#"); ok {
			desc = strings.TrimSpace(d)
		}
		out = append(out, searchResult{Name: name, Description: desc})
	}
	return out
}
