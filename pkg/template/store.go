package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope identifies which layer of a LayeredStore a template lives in.
type Scope string

const (
	// ScopeWorkspace is the per-workspace override directory
	// (<ws>/.bc/templates/). Values here win when a name collides with
	// the global scope.
	ScopeWorkspace Scope = "workspace"

	// ScopeGlobal is the user-global directory (~/.bc/templates/). It is
	// the default for writes unless the caller asks for a workspace
	// override explicitly.
	ScopeGlobal Scope = "global"
)

// ErrWrongScope is returned from Delete when the caller is operating in
// a scope that does not own the named template. The command layer can
// match this with errors.Is to produce a friendly "use --global" hint.
var ErrWrongScope = errors.New("template exists only in a different scope")

// Store is a file-based template store. Each template is stored as
// <name>.json (metadata) and an optional <name>.md (system prompt).
//
// When both a workspace dir and a global dir are configured, List/Get
// merge the two with workspace overrides winning on name collision.
// Create defaults to writing the global dir; callers may pass
// ScopeWorkspace to write into the override dir instead. This keeps
// single-layer callers (legacy workspace-only code) working by passing
// only one directory.
type Store struct {
	// dir is the default write layer. For legacy NewStore callers this
	// is the only layer; for LayeredStore it is the global dir.
	dir string
	// overrideDir, when non-empty, is the workspace-scoped override. Reads
	// check it first; writes do not go here unless the caller asks.
	overrideDir string
}

// NewStore creates a single-layer Store rooted at dir. The directory is
// created on first write. This is retained for backwards compatibility
// with callers that operate in a single-tenant model (eg. legacy
// per-workspace usage). Prefer NewLayeredStore for new code.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// NewLayeredStore creates a two-layer Store. globalDir is the default
// write target and the fallback read source; workspaceDir, when
// non-empty, overrides globalDir on reads and is the write target only
// when scope == ScopeWorkspace.
//
// Either argument may be empty: an empty globalDir degrades to a
// workspace-only store, and an empty workspaceDir degrades to a
// global-only store. At least one must be non-empty.
func NewLayeredStore(globalDir, workspaceDir string) *Store {
	return &Store{dir: globalDir, overrideDir: workspaceDir}
}

// WithOverride returns a new Store that inherits the receiver's global
// dir but uses workspaceDir as its override layer. The receiver is
// unchanged so it remains safe for concurrent use with multiple
// workspace scopes.
func (s *Store) WithOverride(workspaceDir string) *Store {
	return &Store{dir: s.dir, overrideDir: workspaceDir}
}

// GlobalDir returns the configured global directory (empty when the
// store is workspace-only).
func (s *Store) GlobalDir() string { return s.dir }

// WorkspaceDir returns the configured workspace override directory
// (empty when the store is single-layer).
func (s *Store) WorkspaceDir() string { return s.overrideDir }

// List returns all templates visible through the store. When a
// workspace override dir is configured its templates shadow any
// global template with the same name; the Scope is reported on the
// returned Template for the UI / CLI.
func (s *Store) List() ([]Template, error) {
	byName := map[string]Template{}
	order := []string{}

	// Global layer first so workspace entries can overwrite.
	if s.dir != "" {
		if err := collectInto(s.dir, ScopeGlobal, byName, &order); err != nil {
			return nil, err
		}
	}
	if s.overrideDir != "" {
		if err := collectInto(s.overrideDir, ScopeWorkspace, byName, &order); err != nil {
			return nil, err
		}
	}

	out := make([]Template, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// collectInto reads templates from dir and merges them into the result
// map. A template already present in the map is overwritten (workspace
// overrides global), and its name is not re-appended to order.
func collectInto(dir string, scope Scope, byName map[string]Template, order *[]string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read templates dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if !validName(name) {
			continue
		}
		t, _, err := readFrom(dir, name)
		if err != nil {
			continue // skip unreadable files
		}
		t.Scope = scope
		if _, ok := byName[name]; !ok {
			*order = append(*order, name)
		}
		byName[name] = *t
	}
	return nil
}

// validName returns true when name is safe to use as a filesystem path component.
// It rejects empty strings, path separators, and dot-only names to prevent path traversal.
func validName(name string) bool {
	return name != "" &&
		!strings.Contains(name, "/") &&
		!strings.Contains(name, "\\") &&
		!strings.Contains(name, "..") &&
		name != "." &&
		name != ".."
}

// Get returns the template and its system prompt markdown content.
// The workspace override is checked first; if not present there, the
// lookup falls back to the global layer. The system prompt is empty
// string when no .md file exists.
func (s *Store) Get(name string) (*Template, string, error) {
	if !validName(name) {
		return nil, "", fmt.Errorf("template name %q is invalid", name)
	}

	// Workspace override wins.
	if s.overrideDir != "" {
		if t, prompt, err := readFrom(s.overrideDir, name); err == nil {
			t.Scope = ScopeWorkspace
			return t, prompt, nil
		} else if !isNotFound(err) {
			return nil, "", err
		}
	}
	if s.dir != "" {
		if t, prompt, err := readFrom(s.dir, name); err == nil {
			t.Scope = ScopeGlobal
			return t, prompt, nil
		} else if !isNotFound(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("template %q not found", name)
}

// Create writes a new template. If scope is empty it defaults to
// ScopeGlobal when a global dir is configured, otherwise ScopeWorkspace.
// Returns an error if the template already exists in the target scope.
func (s *Store) Create(t Template, systemPrompt string, scope Scope) error {
	if !validName(t.Name) {
		return fmt.Errorf("template name %q is invalid", t.Name)
	}
	dir, err := s.resolveWriteDir(scope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create templates dir %s: %w", dir, err)
	}

	jsonPath := filepath.Join(dir, t.Name+".json")
	if _, err := os.Stat(jsonPath); err == nil {
		return fmt.Errorf("template %q already exists", t.Name)
	}

	// Never persist Scope in the json on disk — it is a runtime-only
	// attribute derived from the file's location.
	onDisk := t
	onDisk.Scope = ""
	if err := writeTemplate(dir, onDisk, systemPrompt); err != nil {
		return err
	}
	return nil
}

// Update overwrites an existing template. The write goes to whichever
// scope currently owns the template — if both layers have it, the
// workspace copy is updated, preserving user-global defaults.
func (s *Store) Update(name string, t Template, systemPrompt string) error {
	if !validName(name) {
		return fmt.Errorf("template name %q is invalid", name)
	}
	dir, err := s.resolveOwningDir(name)
	if err != nil {
		return err
	}
	t.Name = name
	onDisk := t
	onDisk.Scope = ""
	return writeTemplate(dir, onDisk, systemPrompt)
}

// Delete removes both the .json and .md files for the named template.
// If scope is empty the delete targets whichever layer owns the template,
// preferring the workspace override. When the caller asks for
// ScopeWorkspace but the template exists only in the global layer, the
// call fails with ErrWrongScope so the CLI can suggest --global.
func (s *Store) Delete(name string, scope Scope) error {
	if !validName(name) {
		return fmt.Errorf("template name %q is invalid", name)
	}

	switch scope {
	case ScopeWorkspace:
		if s.overrideDir == "" {
			return fmt.Errorf("no workspace override configured for template %q", name)
		}
		if _, err := os.Stat(filepath.Join(s.overrideDir, name+".json")); os.IsNotExist(err) {
			if s.dir != "" {
				if _, gErr := os.Stat(filepath.Join(s.dir, name+".json")); gErr == nil {
					return fmt.Errorf("%w: template %q is user-global (use --global)", ErrWrongScope, name)
				}
			}
			return fmt.Errorf("template %q not found", name)
		}
		return removeTemplate(s.overrideDir, name)
	case ScopeGlobal:
		if s.dir == "" {
			return fmt.Errorf("no global scope configured for template %q", name)
		}
		if _, err := os.Stat(filepath.Join(s.dir, name+".json")); os.IsNotExist(err) {
			return fmt.Errorf("template %q not found", name)
		}
		return removeTemplate(s.dir, name)
	case "":
		// Default: prefer workspace, fall back to global.
		if s.overrideDir != "" {
			if _, err := os.Stat(filepath.Join(s.overrideDir, name+".json")); err == nil {
				return removeTemplate(s.overrideDir, name)
			}
		}
		if s.dir != "" {
			if _, err := os.Stat(filepath.Join(s.dir, name+".json")); err == nil {
				return removeTemplate(s.dir, name)
			}
		}
		return fmt.Errorf("template %q not found", name)
	default:
		return fmt.Errorf("invalid scope %q", scope)
	}
}

// resolveWriteDir returns the directory to write into for the given scope.
func (s *Store) resolveWriteDir(scope Scope) (string, error) {
	switch scope {
	case ScopeWorkspace:
		if s.overrideDir == "" {
			return "", fmt.Errorf("no workspace override configured")
		}
		return s.overrideDir, nil
	case ScopeGlobal:
		if s.dir == "" {
			return "", fmt.Errorf("no global scope configured")
		}
		return s.dir, nil
	case "":
		if s.dir != "" {
			return s.dir, nil
		}
		if s.overrideDir != "" {
			return s.overrideDir, nil
		}
		return "", fmt.Errorf("store has no writable layer")
	default:
		return "", fmt.Errorf("invalid scope %q", scope)
	}
}

// resolveOwningDir returns the dir that owns the named template,
// preferring the workspace override when both layers have it.
func (s *Store) resolveOwningDir(name string) (string, error) {
	if s.overrideDir != "" {
		if _, err := os.Stat(filepath.Join(s.overrideDir, name+".json")); err == nil {
			return s.overrideDir, nil
		}
	}
	if s.dir != "" {
		if _, err := os.Stat(filepath.Join(s.dir, name+".json")); err == nil {
			return s.dir, nil
		}
	}
	return "", fmt.Errorf("template %q not found", name)
}

// SeedDefaults creates the built-in templates when the directory is empty.
// It is a no-op when the directory already contains templates.
func SeedDefaults(dir string) error {
	s := NewStore(dir)
	existing, err := s.List()
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}

	defaults := []Template{
		{
			Name:        "feature-dev",
			Description: "Full-stack feature development",
			MCPs:        []string{"bc", "github"},
		},
		{
			Name:        "reviewer",
			Description: "Code review specialist",
			MCPs:        []string{"bc"},
		},
		{
			Name:        "manager",
			Description: "Task orchestration and delegation",
			MCPs:        []string{"bc"},
		},
		{
			Name:        "blank",
			Description: "Empty starting point",
			MCPs:        []string{"bc"},
		},
	}

	prompts := map[string]string{
		"feature-dev": "You are a feature development agent. Your job is to implement features, fix bugs, and write clean, tested code.\n\n## Guidelines\n- Read existing code before modifying\n- Follow existing patterns and conventions\n- Write meaningful tests\n- Keep changes focused and minimal\n- Commit with clear messages\n",
		"reviewer":    "You are a code review agent. Your job is to review pull requests and provide actionable feedback.\n\n## Review checklist\n- Check for bugs and logic errors\n- Verify error handling\n- Look for security issues\n- Assess test coverage\n- Review code style and consistency\n",
		"manager":     "You are a task orchestration agent. Your job is to break down work, delegate to other agents, and track progress.\n\n## Guidelines\n- Break large tasks into small, independent pieces\n- Assign work based on agent capabilities\n- Monitor progress and unblock stuck agents\n- Summarize status updates clearly\n",
		"blank":       "",
	}

	for _, t := range defaults {
		if err := s.Create(t, prompts[t.Name], ScopeGlobal); err != nil {
			return fmt.Errorf("seed template %q: %w", t.Name, err)
		}
	}
	return nil
}

// --- internal helpers ---

// readFrom reads a single template from the given directory.
func readFrom(dir, name string) (*Template, string, error) {
	jsonPath := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(jsonPath) //nolint:gosec // path built from validated name + known dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("template %q not found", name)
		}
		return nil, "", fmt.Errorf("read template %q: %w", name, err)
	}

	var t Template
	if unmarshalErr := json.Unmarshal(data, &t); unmarshalErr != nil {
		return nil, "", fmt.Errorf("parse template %q: %w", name, unmarshalErr)
	}
	t.Name = name

	prompt := ""
	promptData, err := os.ReadFile(filepath.Join(dir, name+".md")) //nolint:gosec // path built from validated name + known dir
	if err == nil {
		prompt = string(promptData)
	}

	return &t, prompt, nil
}

// writeTemplate writes <name>.json and <name>.md into dir.
func writeTemplate(dir string, t Template, systemPrompt string) error {
	// Template names come from API callers — never let one escape the
	// templates directory.
	if !filepath.IsLocal(t.Name) || strings.ContainsAny(t.Name, `/\`) {
		return fmt.Errorf("invalid template name %q", t.Name)
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal template %q: %w", t.Name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, t.Name+".json"), data, 0640); err != nil { //nolint:gosec // 0640 intentional
		return fmt.Errorf("write template %q: %w", t.Name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, t.Name+".md"), []byte(systemPrompt), 0640); err != nil { //nolint:gosec // 0640 intentional
		return fmt.Errorf("write template prompt %q: %w", t.Name, err)
	}
	return nil
}

// removeTemplate deletes <name>.json + <name>.md from dir. Missing .md
// is not an error.
func removeTemplate(dir, name string) error {
	if !filepath.IsLocal(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid template name %q", name)
	}
	jsonPath := filepath.Join(dir, name+".json")
	if err := os.Remove(jsonPath); err != nil {
		return fmt.Errorf("delete template %q: %w", name, err)
	}
	mdPath := filepath.Join(dir, name+".md")
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete template prompt %q: %w", name, err)
	}
	return nil
}

// isNotFound returns true when err signals a missing template (wrapped
// "not found" error) rather than a real IO error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
