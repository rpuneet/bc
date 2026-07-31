package secret

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/db"
)

// Scope distinguishes which layer of a layered secret store holds a
// value. It is a runtime attribute reported in SecretMeta.Scope.
type Scope string

const (
	// ScopeGlobal is the user-global vault (~/.mycel/secrets.vault).
	ScopeGlobal Scope = "global"
	// ScopeWorkspace is the repo-scoped override
	// (<h>/.mycel/secrets.db).
	ScopeWorkspace Scope = "workspace"
)

// OpenVaultFile opens a Store using an explicit SQLite path instead of
// the conventional "<repo>/.mycel/secrets.db". Used for the
// user-global vault at ~/.mycel/secrets.vault where there is no
// "workspace" to anchor against. Directory must exist.
func OpenVaultFile(path, passphrase string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("vault path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, fmt.Errorf("create vault parent: %w", err)
	}

	d, err := db.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vault: %w", err)
	}
	s := &Store{db: d}
	if err := s.initSchema(); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init vault schema: %w", err)
	}
	if err := s.initKey(passphrase); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init vault key: %w", err)
	}
	return s, nil
}

// LayeredStore composes a user-global vault with an optional
// repo-local Store. Reads check the repo layer first, falling back to
// global. Writes default to the global vault; callers can target the
// repo layer with the *Workspace methods or SetWithScope.
//
// Each underlying Store keeps its own encryption key (derived from the
// same passphrase + a per-vault salt) so the two files stay
// independently decryptable. Callers close a LayeredStore with
// Close() which closes both underlying Stores; ownership flows through
// the wrapper.
type LayeredStore struct {
	global *Store
	repo   *Store // may be nil when no repo-scoped override is configured
}

// NewLayeredStore wraps a global vault and an optional repo-scoped Store.
// Either argument may be nil, but at least one must be non-nil or every
// call will error.
func NewLayeredStore(global, repo *Store) *LayeredStore {
	return &LayeredStore{global: global, repo: repo}
}

// Global returns the underlying user-global Store (never nil when the
// LayeredStore was constructed correctly).
func (l *LayeredStore) Global() *Store { return l.global }

// Workspace returns the optional repo-scoped override Store.
func (l *LayeredStore) Workspace() *Store { return l.repo }

// Close closes both underlying stores, returning the first error.
func (l *LayeredStore) Close() error {
	var firstErr error
	if l.repo != nil {
		if err := l.repo.Close(); err != nil {
			firstErr = err
		}
	}
	if l.global != nil {
		if err := l.global.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// GetValue returns the decrypted value, preferring the repo scope.
func (l *LayeredStore) GetValue(name string) (string, error) {
	if l.repo != nil {
		if v, err := l.repo.GetValue(name); err == nil {
			return v, nil
		}
	}
	if l.global == nil {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return l.global.GetValue(name)
}

// GetMeta returns the metadata from whichever layer owns the name,
// with the repo-scoped override winning. The returned meta's Scope field
// is populated to reflect the owning layer.
func (l *LayeredStore) GetMeta(name string) (*SecretMeta, error) {
	if l.repo != nil {
		if m, err := l.repo.GetMeta(name); err == nil && m != nil {
			m.Scope = ScopeWorkspace
			return m, nil
		}
	}
	if l.global == nil {
		return nil, nil
	}
	m, err := l.global.GetMeta(name)
	if err == nil && m != nil {
		m.Scope = ScopeGlobal
	}
	return m, err
}

// List returns metadata for every secret in both layers. When a name
// exists in both, the repo entry wins and scope is reported as
// "workspace".
func (l *LayeredStore) List() ([]*SecretMeta, error) {
	byName := map[string]*SecretMeta{}
	order := []string{}

	if l.global != nil {
		gs, err := l.global.List()
		if err != nil {
			return nil, err
		}
		for _, m := range gs {
			m.Scope = ScopeGlobal
			byName[m.Name] = m
			order = append(order, m.Name)
		}
	}
	if l.repo != nil {
		h, err := l.repo.List()
		if err != nil {
			return nil, err
		}
		for _, m := range h {
			m.Scope = ScopeWorkspace
			if _, seen := byName[m.Name]; !seen {
				order = append(order, m.Name)
			}
			byName[m.Name] = m
		}
	}

	out := make([]*SecretMeta, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// Set writes to the user-global vault (the default for mycel secret add
// KEY=VAL). Use SetWorkspace for repo-scoped overrides.
func (l *LayeredStore) Set(name, value, description string) error {
	if l.global == nil {
		return fmt.Errorf("no global vault configured")
	}
	return l.global.Set(name, value, description)
}

// SetWorkspace writes to the repo-scoped override store.
func (l *LayeredStore) SetWorkspace(name, value, description string) error {
	if l.repo == nil {
		return fmt.Errorf("no repo-scoped override store configured")
	}
	return l.repo.Set(name, value, description)
}

// SetWithScope chooses a layer explicitly; empty scope defaults to
// global when it exists, else the repo layer.
func (l *LayeredStore) SetWithScope(scope Scope, name, value, description string) error {
	switch scope {
	case ScopeWorkspace:
		return l.SetWorkspace(name, value, description)
	case ScopeGlobal:
		return l.Set(name, value, description)
	case "":
		if l.global != nil {
			return l.Set(name, value, description)
		}
		return l.SetWorkspace(name, value, description)
	default:
		return fmt.Errorf("invalid scope %q", scope)
	}
}

// Delete removes from whichever layer owns the name, preferring
// the repo layer. Returns an error when the secret exists in neither.
func (l *LayeredStore) Delete(name string) error {
	if l.repo != nil {
		if err := l.repo.Delete(name); err == nil {
			return nil
		}
	}
	if l.global != nil {
		return l.global.Delete(name)
	}
	return fmt.Errorf("secret %q not found", name)
}

// DeleteScoped removes from a specific scope. Useful for "mycel secret
// delete NAME --global" / "--workspace".
func (l *LayeredStore) DeleteScoped(scope Scope, name string) error {
	switch scope {
	case ScopeWorkspace:
		if l.repo == nil {
			return fmt.Errorf("no repo-scoped override configured")
		}
		return l.repo.Delete(name)
	case ScopeGlobal:
		if l.global == nil {
			return fmt.Errorf("no global vault configured")
		}
		return l.global.Delete(name)
	case "":
		return l.Delete(name)
	default:
		return fmt.Errorf("invalid scope %q", scope)
	}
}

// ResolveEnv substitutes ${secret:NAME} in each value. Names are
// resolved with repo-over-global precedence.
func (l *LayeredStore) ResolveEnv(env map[string]string) map[string]string {
	resolved := make(map[string]string, len(env))
	for k, v := range env {
		resolved[k] = resolveValueLayered(l, v)
	}
	return resolved
}

// resolveValueLayered mirrors Store.resolveValue but uses the layered
// GetValue so config overrides take precedence.
func resolveValueLayered(l *LayeredStore, v string) string {
	const prefix = "${secret:"
	const suffix = "}"

	start := 0
	for {
		idx := indexFrom(v, prefix, start)
		if idx < 0 {
			break
		}
		end := indexFrom(v, suffix, idx+len(prefix))
		if end < 0 {
			break
		}
		secretName := v[idx+len(prefix) : end]
		val, err := l.GetValue(secretName)
		if err != nil {
			start = end + 1
			continue
		}
		v = v[:idx] + val + v[end+1:]
		start = idx + len(val)
	}
	return v
}

func indexFrom(s, sub string, from int) int {
	if from >= len(s) {
		return -1
	}
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
