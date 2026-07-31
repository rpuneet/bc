// Repo discovery helpers — see open.go for the main package doc.
//
// These scan for git repos the web UI can offer to adopt. They are
// deliberately separate from the rest of pkg/home to avoid coupling
// config concerns with filesystem scanning or network calls.
//
// Two source types are supported:
//   - Local filesystem: walk a directory, emit every .git repo
//   - GitHub: list the authenticated user's repositories via the REST API
//
// Each helper returns a slice of Candidate with a consistent shape so the
// caller (server/handlers/discovery.go) can serialize to JSON without
// additional shaping.
package home

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// defaultDepth is the WalkDir depth used when the request does not override.
const defaultDepth = 3

// maxDepth caps how far we will recurse so a malicious or oversized request
// cannot force an unbounded walk.
const maxDepth = 8

// skipDirs are directory basenames ignored during local scans — they
// rarely contain a distinct project root and tend to bloat output.
var skipDirs = map[string]struct{}{
	"node_modules": {},
	".git":         {},
	"dist":         {},
	"build":        {},
	"target":       {},
	"vendor":       {},
	"__pycache__":  {},
	".venv":        {},
	"venv":         {},
	"Library":      {}, // macOS
	".Trash":       {},
	"Caches":       {},
	".cache":       {},
	".DS_Store":    {},
	".idea":        {},
	".vscode":      {},
	".next":        {},
	".nuxt":        {},
	"coverage":     {},
	"Pods":         {},
	"DerivedData":  {},
	".npm":         {},
	".yarn":        {},
	".gradle":      {},
	".pnpm-store":  {},
	".docker":      {},
	".terraform":   {},
}

// Candidate describes one discovered repository.
type Candidate struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	GitRemote         string `json:"git_remote,omitempty"`
	GithubURL         string `json:"github_url,omitempty"`
	HasMycel          bool   `json:"has_mycel"`
	AlreadyRegistered bool   `json:"already_registered"`
}

// ScanOptions controls local discovery behavior.
type ScanOptions struct {
	// SkipDirs additional directory basenames to exclude, merged with the
	// default SkipDirs list.
	SkipDirs map[string]struct{}
	// Root is the absolute directory to scan. Required.
	Root string
	// Depth bounds recursion relative to Root. Values <=0 use defaultDepth.
	// Values > maxDepth are clamped.
	Depth int
}

// ScanLocal walks opts.Root looking for Git repositories (directories
// containing a .git entry, file or dir) and returns one Candidate per hit.
// It is safe to call with an unresolvable or inaccessible Root — the error
// is returned without partial results.
func ScanLocal(ctx context.Context, opts ScanOptions) ([]Candidate, error) {
	if opts.Root == "" {
		return nil, fmt.Errorf("root is required")
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("abs: %w", err)
	}
	root = filepath.Clean(root)
	if strings.Contains(root, "..") {
		return nil, fmt.Errorf("invalid scan root %q", opts.Root)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	depth := opts.Depth
	if depth <= 0 {
		depth = defaultDepth
	}
	if depth > maxDepth {
		depth = maxDepth
	}

	skip := make(map[string]struct{}, len(skipDirs)+len(opts.SkipDirs))
	for k := range skipDirs {
		skip[k] = struct{}{}
	}
	for k := range opts.SkipDirs {
		skip[k] = struct{}{}
	}

	var out []Candidate
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			// Tolerate per-entry errors (permission denied, etc.) — just skip.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Enforce depth relative to root.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		level := 0
		if rel != "." {
			level = strings.Count(rel, string(filepath.Separator)) + 1
		}

		if d.IsDir() {
			// Skip ignored basenames (but not the root itself).
			if path != root {
				if _, ok := skip[d.Name()]; ok {
					return fs.SkipDir
				}
			}
			if level > depth {
				return fs.SkipDir
			}
			// Check for .git inside this directory.
			gitPath := filepath.Join(path, ".git")
			if _, statErr := os.Stat(gitPath); statErr == nil {
				c := Candidate{
					Path: path,
					Name: filepath.Base(path),
				}
				// .mycel/ marker left by previously adopted repos.
				if _, bcErr := os.Stat(filepath.Join(path, ".mycel")); bcErr == nil {
					c.HasMycel = true
				}
				c.GitRemote = readGitRemote(ctx, path)
				c.GithubURL = githubURLFromRemote(c.GitRemote)
				out = append(out, c)
				// Don't descend into this repo — we treat the repo as a unit.
				return fs.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readGitRemote returns the `origin` remote URL for the repo at path, or ""
// if none. Best-effort; never blocks indefinitely because we rely on the
// caller-supplied context to short-circuit.
func readGitRemote(ctx context.Context, repoPath string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// githubURLFromRemote normalizes ssh / https git-remote URLs into a plain
// https GitHub URL. Returns "" for non-GitHub remotes.
func githubURLFromRemote(remote string) string {
	if remote == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		repo := strings.TrimPrefix(remote, "git@github.com:")
		repo = strings.TrimSuffix(repo, ".git")
		return "https://github.com/" + repo
	case strings.HasPrefix(remote, "https://github.com/"):
		return strings.TrimSuffix(remote, ".git")
	case strings.HasPrefix(remote, "git://github.com/"):
		repo := strings.TrimPrefix(remote, "git://github.com/")
		return "https://github.com/" + strings.TrimSuffix(repo, ".git")
	}
	return ""
}

// validCloneURL restricts clone sources to the transports discovery
// offers (https, ssh, scp-style git@host:path). Rejects option-shaped
// values (leading "-") and exotic transports like ext:: that let a URL
// execute commands.
func validCloneURL(url string) bool {
	if url == "" || strings.HasPrefix(url, "-") {
		return false
	}
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "ssh://") {
		return true
	}
	return scpLikeURL.MatchString(url)
}

var scpLikeURL = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+:[A-Za-z0-9._~/-]+$`)

// Clone runs `git clone <url> <target>/<name>` to materialize a repository.
// Returns the absolute path of the new checkout on success.
func Clone(ctx context.Context, url, target, name string) (string, error) {
	if !validCloneURL(url) {
		return "", fmt.Errorf("invalid clone url %q", url)
	}
	if target == "" {
		return "", fmt.Errorf("target is required")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("abs target: %w", err)
	}
	absTarget = filepath.Clean(absTarget)
	if strings.Contains(absTarget, "..") {
		return "", fmt.Errorf("invalid clone target %q", target)
	}
	if info, err := os.Stat(absTarget); err != nil {
		if err := os.MkdirAll(absTarget, 0o750); err != nil {
			return "", fmt.Errorf("mkdir target: %w", err)
		}
	} else if !info.IsDir() {
		return "", fmt.Errorf("target %s exists and is not a directory", absTarget)
	}

	if name == "" {
		// Derive from URL basename.
		base := strings.TrimSuffix(filepath.Base(url), ".git")
		base = strings.TrimSuffix(base, ":")
		if base == "" {
			return "", fmt.Errorf("unable to derive name from url %q", url)
		}
		name = base
	}
	// Validate unconditionally — a derived basename can still be ".." or
	// otherwise escape absTarget.
	if !filepath.IsLocal(name) || strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("invalid checkout name %q", name)
	}
	dest := filepath.Join(absTarget, name)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("destination %s already exists", dest)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--", url, dest) //nolint:gosec // url scheme-allowlisted, name IsLocal-checked above
	if out, cErr := cmd.CombinedOutput(); cErr != nil {
		return "", fmt.Errorf("git clone failed: %w: %s", cErr, strings.TrimSpace(string(out)))
	}
	return dest, nil
}
