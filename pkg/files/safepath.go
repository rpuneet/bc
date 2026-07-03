// Package files exposes shared filesystem helpers. The Code tab HTTP
// handlers (server/handlers/code.go) resolve every user-supplied path
// through SafeJoin so traversal attempts (../, absolute paths, NUL
// bytes, escaping symlinks) cannot read files outside of the workspace
// or worktree root.
package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscape is returned when a user-supplied path attempts to escape
// the provided root (via .., absolute path, or a symlink pointing outside).
var ErrPathEscape = errors.New("path escapes root")

// ErrInvalidPath is returned when the user-supplied path contains
// characters that are never acceptable (currently: NUL bytes).
var ErrInvalidPath = errors.New("invalid path")

// SafeJoin joins userPath onto root and returns the absolute, cleaned
// filesystem path.
//
// It guarantees that the returned path lives inside root. SafeJoin rejects:
//   - absolute userPaths (userPath must be relative)
//   - paths containing NUL bytes
//   - paths that, after cleaning, escape root via "../" segments
//   - paths whose symlink target (or any ancestor's symlink target)
//     escapes root
//
// Root must be an absolute, existing directory; if root itself does not
// exist SafeJoin still returns an answer (no symlink check can run) but
// callers typically os.Stat the result immediately and handle ENOENT.
//
// SafeJoin accepts an empty userPath (returns root).
func SafeJoin(root, userPath string) (string, error) {
	if root == "" {
		return "", errors.New("safepath: root is empty")
	}
	if strings.ContainsRune(userPath, 0) {
		return "", fmt.Errorf("%w: NUL byte", ErrInvalidPath)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("safepath: abs root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	// Reject absolute user paths up-front. We purposely do not "rebase"
	// an absolute path into root because that would silently ignore the
	// caller's intent and mask bugs in the frontend.
	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("%w: absolute path %q", ErrPathEscape, userPath)
	}

	// filepath.Clean maps both "" and "." to "." — the caller wants root
	// itself, which is trivially contained.
	cleanUser := filepath.Clean(userPath)
	if cleanUser == "." {
		return absRoot, nil
	}

	// Lexical containment check. cleanUser is already cleaned, so any
	// remaining ".." segments are leading ones that would escape root.
	// filepath.IsLocal rejects those (and absolute paths, and Windows
	// reserved names) up-front.
	if !filepath.IsLocal(cleanUser) {
		return "", fmt.Errorf("%w: %q escapes root", ErrPathEscape, userPath)
	}

	joined := filepath.Join(absRoot, cleanUser)
	joined = filepath.Clean(joined)

	if !isWithin(joined, absRoot) {
		return "", fmt.Errorf("%w: %q outside %q", ErrPathEscape, joined, absRoot)
	}

	// Symlink check — EvalSymlinks walks every component, including
	// intermediate directories. If the final path does not exist yet we
	// walk up to the nearest existing ancestor and check that.
	resolved, err := evalExistingSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("safepath: eval symlinks: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		// Root may legitimately not exist in some tests; if it's gone,
		// fall back to the lexical root for the containment check.
		if os.IsNotExist(err) {
			resolvedRoot = absRoot
		} else {
			return "", fmt.Errorf("safepath: eval root: %w", err)
		}
	}
	if !isWithin(resolved, resolvedRoot) {
		return "", fmt.Errorf("%w: symlink target %q outside %q",
			ErrPathEscape, resolved, resolvedRoot)
	}

	return joined, nil
}

// isWithin reports whether path is equal to or nested under root.
// Both arguments must already be cleaned + absolute.
func isWithin(path, root string) bool {
	if path == root {
		return true
	}
	// Ensure we compare full path components — a prefix match alone
	// would accept "/rootXYZ" as inside "/root".
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// evalExistingSymlinks resolves symlinks for path. If path itself does
// not exist, it walks up to the nearest ancestor that does and returns
// its resolved form joined with the remaining (unresolved) tail. This
// lets SafeJoin answer about files that the caller is about to create
// or that simply don't exist (tree listing of a missing subdir).
func evalExistingSymlinks(path string) (string, error) {
	current := path
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			out := resolved
			for i := len(tail) - 1; i >= 0; i-- {
				out = filepath.Join(out, tail[i])
			}
			return filepath.Clean(out), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root; nothing resolves. Return the
			// original lexical path — the outer containment check on
			// absRoot (also lexical) still applies.
			return filepath.Clean(path), nil
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}
