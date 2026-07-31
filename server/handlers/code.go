// code.go implements the read-only Code tab backend:
//
//	GET /api/code/tree?path=&worktree=[&show_hidden=1]
//	GET /api/code/file?path=&worktree=
//	GET /api/code/diff?worktree=[&path=]
//
// The routes are registered at /api/code/ and dispatched by ServeHTTP
// based on the first path segment. the daemon is single-tenant: the handler is
// anchored at the one bundle repo root supplied at construction time.
//
// Every filesystem read is sandboxed via pkg/files.SafeJoin; .git/
// and .mycel/ subdirs are hidden by default; file reads cap at 2 MiB.
package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/files"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
)

// validWorktreeName allowlists worktree (agent) names: they become path
// components and a `git -C` argument, so only identifier characters are
// accepted — never separators, dots, or shell metacharacters.
var validWorktreeName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// RepoResolver supplies the repo root the Code handler serves. It is
// an interface so tests can supply lightweight implementations.
type RepoResolver interface {
	// ActiveRoot returns the filesystem root of the bundle repo, or ""
	// when the daemon booted without one.
	ActiveRoot() string
}

// CodeHandler serves the read-only Code tab endpoints.
type CodeHandler struct {
	resolver RepoResolver
}

// NewCodeHandler constructs a CodeHandler bound to a repo resolver.
func NewCodeHandler(resolver RepoResolver) *CodeHandler {
	return &CodeHandler{resolver: resolver}
}

// Register mounts the handler on mux at /api/code/.
func (h *CodeHandler) Register(mux *http.ServeMux) {
	mux.Handle("/api/code/", h)
	// /api/code (no trailing slash) is not a real endpoint — respond 404
	// so clients don't get the default mux index HTML.
	mux.HandleFunc("/api/code", func(w http.ResponseWriter, _ *http.Request) {
		httpError(w, "not found", http.StatusNotFound)
	})
}

// ServeHTTP dispatches on the first path segment after /api/code/.
func (h *CodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/code/")
	rest = strings.TrimSuffix(rest, "/")
	head, _, _ := strings.Cut(rest, "/")
	switch head {
	case "tree":
		h.tree(w, r)
	case "file":
		h.file(w, r)
	case "diff":
		h.diff(w, r)
	case "search":
		h.search(w, r)
	default:
		httpError(w, "unknown code endpoint", http.StatusNotFound)
	}
}

// ------------------------------------------------------------------ root resolution

// resolveRepoRoot returns the absolute repo root for this request.
// The root comes from boot configuration, but it is cleaned and traversal
// segments are rejected so nothing built on it can escape sideways.
func (h *CodeHandler) resolveRepoRoot(_ *http.Request) (string, error) {
	if h.resolver == nil {
		return "", errors.New("repo resolver not configured")
	}
	root := h.resolver.ActiveRoot()
	if root == "" {
		return "", errors.New("no repo available")
	}
	root = filepath.Clean(root)
	if strings.Contains(root, "..") {
		return "", errors.New("invalid repo root")
	}
	return root, nil
}

// resolveWorktreeRoot resolves the user-supplied worktree name onto a
// filesystem path. "main" (the default) maps to the repo root; any
// other value is treated as an agent name and maps to
// ~/.mycel/agents/<name>/worktree/ — the same path layout that
// pkg/worktree.Manager uses.
// errNoWorktree signals a valid agent name whose worktree directory
// does not exist yet (the agent hasn't worked on a repo). Callers return
// an empty result with 200 rather than a 400 so the Code tab shows a
// clean empty state instead of a console error.
var errNoWorktree = errors.New("worktree not found")

func (h *CodeHandler) resolveWorktreeRoot(r *http.Request) (repoRoot, wtRoot, worktreeName string, err error) {
	repoRoot, err = h.resolveRepoRoot(r)
	if err != nil {
		return "", "", "", err
	}
	worktreeName = r.URL.Query().Get("worktree")
	if worktreeName == "" {
		worktreeName = "main"
	}
	if worktreeName == "main" {
		return repoRoot, repoRoot, worktreeName, nil
	}
	// Worktree names are agent names — they become path components below
	// and a git -C argument later, so allowlist identifier characters
	// only (no separators, dots, or metacharacters).
	if !validWorktreeName.MatchString(worktreeName) {
		return repoRoot, "", worktreeName, errors.New("invalid worktree name")
	}
	agentsRoot, err := home.AgentsDir()
	if err != nil {
		return repoRoot, "", worktreeName, errNoWorktree
	}
	wtRoot = filepath.Join(agentsRoot, worktreeName, "worktree")
	// Defense in depth: the composed root must stay inside the agents
	// root after cleaning — reject anything else before it reaches git -C.
	wtRoot = filepath.Clean(wtRoot)
	if strings.Contains(wtRoot, "..") || !strings.HasPrefix(wtRoot, agentsRoot+string(filepath.Separator)) {
		return repoRoot, "", worktreeName, errors.New("invalid worktree path")
	}
	info, statErr := os.Stat(wtRoot)
	if statErr != nil || !info.IsDir() {
		return repoRoot, "", worktreeName, errors.New("worktree not found")
	}
	return repoRoot, wtRoot, worktreeName, nil
}

// ------------------------------------------------------------------ /tree

type treeEntry struct {
	Size  *int64 `json:"size,omitempty"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

func (h *CodeHandler) tree(w http.ResponseWriter, r *http.Request) {
	_, wtRoot, worktreeName, err := h.resolveWorktreeRoot(r)
	if errors.Is(err, errNoWorktree) {
		writeJSON(w, http.StatusOK, []treeEntry{})
		return
	}
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	relPath := r.URL.Query().Get("path")
	showHidden := r.URL.Query().Get("show_hidden") == "1"

	abs, err := files.SafeJoin(wtRoot, relPath)
	if err != nil {
		httpError(w, "invalid path", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			httpError(w, "path not found", http.StatusNotFound)
			return
		}
		log.Warn("code tree stat", "path", abs, "error", err)
		httpError(w, "stat failed", http.StatusInternalServerError)
		return
	}
	if !info.IsDir() {
		httpError(w, "path is not a directory", http.StatusBadRequest)
		return
	}

	dents, err := os.ReadDir(abs)
	if err != nil {
		log.Warn("code tree readdir", "path", abs, "error", err)
		httpError(w, "read dir failed", http.StatusInternalServerError)
		return
	}

	out := make([]treeEntry, 0, len(dents))
	for _, d := range dents {
		name := d.Name()
		if !showHidden && isHiddenEntry(name, relPath) {
			continue
		}
		entryRel := name
		if relPath != "" {
			entryRel = filepath.ToSlash(filepath.Join(relPath, name))
		}
		e := treeEntry{
			Name:  name,
			Path:  entryRel,
			IsDir: d.IsDir(),
		}
		if !d.IsDir() {
			if fi, statErr := d.Info(); statErr == nil {
				size := fi.Size()
				e.Size = &size
			}
		}
		out = append(out, e)
	}

	// Directories first, then files; both alphabetised.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})

	_ = worktreeName
	writeJSON(w, http.StatusOK, out)
}

// isHiddenEntry reports whether name should be skipped in tree listings.
// We hide .git (always) and .mycel (repo metadata) at the top level.
// A deeper path (e.g. inside a node_modules/.git) is still shown —
// traversal into such dirs is the user's choice.
func isHiddenEntry(name, parentRel string) bool {
	if parentRel != "" && parentRel != "." {
		return false
	}
	return name == ".git" || name == ".mycel"
}

// ------------------------------------------------------------------ /file

// maxFileBytes is the per-request ceiling for /api/code/file responses.
// Matches §6.4.2 (2 MiB) and protects the server from accidental OOMs
// when a client requests a large repo artifact.
const maxFileBytes = 2 * 1024 * 1024

func (h *CodeHandler) file(w http.ResponseWriter, r *http.Request) {
	_, wtRoot, _, err := h.resolveWorktreeRoot(r)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		httpError(w, "path required", http.StatusBadRequest)
		return
	}
	abs, err := files.SafeJoin(wtRoot, relPath)
	if err != nil {
		httpError(w, "invalid path", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			httpError(w, "file not found", http.StatusNotFound)
			return
		}
		log.Warn("code file stat", "path", abs, "error", err)
		httpError(w, "stat failed", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		httpError(w, "path is a directory", http.StatusBadRequest)
		return
	}

	//nolint:gosec // abs has been validated by files.SafeJoin.
	f, err := os.Open(abs)
	if err != nil {
		log.Warn("code file open", "path", abs, "error", err)
		httpError(w, "open failed", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }() //nolint:errcheck

	// Read up to maxFileBytes + 1 so we can flag truncation precisely.
	buf := make([]byte, maxFileBytes+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		log.Warn("code file read", "path", abs, "error", err)
		httpError(w, "read failed", http.StatusInternalServerError)
		return
	}
	data := buf[:n]
	truncated := false
	if n > maxFileBytes {
		data = data[:maxFileBytes]
		truncated = true
	}

	// Binary detection using the leading 512 bytes.
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	contentType := http.DetectContentType(sniff)
	binary := !isTextual(contentType)

	w.Header().Set("X-Mycel-Size", itoa(info.Size()))
	if truncated {
		w.Header().Set("X-Mycel-Truncated", "true")
	}
	if binary {
		w.Header().Set("X-Mycel-Binary", "true")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if _, writeErr := w.Write(data); writeErr != nil {
			log.Debug("code file write", "error", writeErr)
		}
		return
	}
	w.Header().Set("Content-Type", contentTypeForExt(abs, contentType))
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write(data); writeErr != nil {
		log.Debug("code file write", "error", writeErr)
	}
}

// isTextual reports whether a DetectContentType result is safe to
// serve as plain text in the browser.
func isTextual(ct string) bool {
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	switch {
	case strings.HasPrefix(ct, "application/json"):
		return true
	case strings.HasPrefix(ct, "application/xml"):
		return true
	case strings.HasPrefix(ct, "application/javascript"):
		return true
	}
	return false
}

// contentTypeForExt overrides http.DetectContentType for well-known
// source-code extensions so browsers render them as text instead of
// trying to download. The fallback is the detected type.
func contentTypeForExt(path, detected string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".rb", ".rs", ".java", ".kt", ".swift", ".c", ".h",
		".cc", ".cpp", ".hpp", ".m", ".mm", ".sh", ".bash", ".zsh", ".fish",
		".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
		".md", ".markdown", ".txt", ".log", ".conf", ".toml", ".ini",
		".yaml", ".yml", ".sql", ".proto", ".lua", ".pl", ".php", ".r",
		".dart", ".scala", ".clj", ".ex", ".exs", ".erl", ".hs", ".elm",
		".vue", ".svelte", ".tf", ".hcl", ".dockerfile", ".gitignore",
		".env", ".makefile", ".mk":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	}
	return detected
}

// ------------------------------------------------------------------ /diff

// diffTimeout caps git-diff execution per §6.4.3.
const diffTimeout = 10 * time.Second

func (h *CodeHandler) diff(w http.ResponseWriter, r *http.Request) {
	_, wtRoot, worktreeName, err := h.resolveWorktreeRoot(r)
	if errors.Is(err, errNoWorktree) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		return
	}
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// "main" has no diff — it IS the baseline.
	if worktreeName == "main" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return
	}

	relPath := r.URL.Query().Get("path")
	args := []string{"-C", wtRoot, "diff", "main...HEAD"}
	if relPath != "" {
		abs, err := files.SafeJoin(wtRoot, relPath)
		if err != nil {
			httpError(w, "invalid path", http.StatusBadRequest)
			return
		}
		// Feed git a relative path so it's interpreted under wtRoot.
		// SafeJoin guarantees abs is inside wtRoot, so Rel cannot fail;
		// if it somehow does, reject rather than pass raw input to git.
		rel, relErr := filepath.Rel(wtRoot, abs)
		if relErr != nil {
			httpError(w, "invalid path", http.StatusBadRequest)
			return
		}
		args = append(args, "--", rel)
	}

	ctx, cancel := context.WithTimeout(r.Context(), diffTimeout)
	defer cancel()

	//nolint:gosec // wtRoot is regexp-allowlisted + containment-checked in
	// resolveWorktreeRoot; relPath is sanitized via SafeJoin.
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If main...HEAD is an unknown ref (e.g. bare worktree with no
		// local main branch), return an empty diff rather than 500 —
		// this is not a server fault.
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			stderrStr := stderr.String()
			if strings.Contains(stderrStr, "unknown revision") ||
				strings.Contains(stderrStr, "bad revision") {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Mycel-Diff-Empty", "no-main-ref")
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		log.Warn("code diff failed", "worktree", worktreeName, "error", err, "stderr", stderr.String())
		httpError(w, "git diff failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write(stdout.Bytes()); writeErr != nil {
		log.Debug("code diff write", "error", writeErr)
	}
}

// ------------------------------------------------------------------ helpers

// itoa is strconv.FormatInt with a short name; kept here to avoid an
// extra import just for one line.
func itoa(n int64) string {
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// staticRepoResolver is a RepoResolver pinned to one root —
// the single-bundle repo the daemon was booted against.
type staticRepoResolver struct {
	root string
}

// NewStaticRepoResolver builds a resolver pinned to root (may be "").
func NewStaticRepoResolver(root string) RepoResolver {
	return &staticRepoResolver{root: root}
}

// ActiveRoot returns the pinned root or "".
func (r *staticRepoResolver) ActiveRoot() string { return r.root }
