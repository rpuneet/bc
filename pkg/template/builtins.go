package template

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Built-in templates ship as a <name>.json / <name>.md pair under builtins/,
// the same layout the store writes, so adding one is adding two files and
// reviewing one is reading the prompt a user would read.
//
// The metadata only sets fields that something actually honors: description,
// mcps, and the two guardrails enforced in server/guardrails.go. Secrets,
// plugins, tool policies and context files are accepted by the API and not yet
// applied to agents, so a built-in that set them would be making the same
// promise the template editor was removed for (#3550).
//
//go:embed builtins/*.json builtins/*.md
var builtinFS embed.FS

const builtinDir = "builtins"

// builtinStateFile records which built-ins have been installed into a templates
// directory. It deliberately does not end in .json: the store lists *.json as
// templates, and this is not one.
const builtinStateFile = ".builtins.state"

type builtinState struct {
	Installed []string `json:"installed"`
}

// BuiltinNames returns the names of the templates that ship with mycel, sorted.
func BuiltinNames() []string {
	entries, err := builtinFS.ReadDir(builtinDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names
}

// Builtin returns the named built-in template and its prompt.
func Builtin(name string) (Template, string, error) {
	if !validName(name) {
		return Template{}, "", fmt.Errorf("invalid template name %q", name)
	}

	raw, err := builtinFS.ReadFile(filepath.Join(builtinDir, name+".json"))
	if err != nil {
		return Template{}, "", fmt.Errorf("no built-in template %q", name)
	}
	var t Template
	if err := json.Unmarshal(raw, &t); err != nil {
		return Template{}, "", fmt.Errorf("parse built-in template %q: %w", name, err)
	}
	t.Name = name

	prompt := ""
	if md, mdErr := builtinFS.ReadFile(filepath.Join(builtinDir, name+".md")); mdErr == nil {
		prompt = string(md)
	}
	return t, prompt, nil
}

// EnsureBuiltins installs any built-in template that has never been installed
// into dir, and returns the names it added.
//
// It is additive and runs on every start, so an upgrade delivers new built-ins
// to an existing workspace rather than only to a fresh one — the previous
// seeding only fired when the directory was completely empty, which meant
// nobody who had ever used mycel would see a template added later.
//
// Two things it will not do. It never overwrites a file that exists, so edits
// to a built-in survive. And it records what it has installed, so a built-in
// someone deleted on purpose stays deleted instead of returning at every
// restart.
func EnsureBuiltins(dir string) ([]string, error) {
	if dir == "" {
		return nil, errors.New("templates dir is empty")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ensure templates dir: %w", err)
	}

	state, err := readBuiltinState(dir)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(state.Installed))
	for _, n := range state.Installed {
		known[n] = true
	}

	s := NewStore(dir)
	var added []string

	for _, name := range BuiltinNames() {
		if known[name] {
			continue // installed before; respect a later deletion
		}

		// A template already on disk was either shipped by an older build or
		// written by hand. Either way it is not ours to replace — record it and
		// leave it be.
		if _, statErr := os.Stat(filepath.Join(dir, name+".json")); statErr == nil {
			state.Installed = append(state.Installed, name)
			continue
		}

		t, prompt, builtinErr := Builtin(name)
		if builtinErr != nil {
			return added, builtinErr
		}
		if createErr := s.Create(t, prompt, ScopeGlobal); createErr != nil {
			return added, fmt.Errorf("install built-in template %q: %w", name, createErr)
		}
		state.Installed = append(state.Installed, name)
		added = append(added, name)
	}

	if err := writeBuiltinState(dir, state); err != nil {
		// The templates are installed; failing to record that only risks
		// reinstalling a deleted one later, which is not worth failing startup.
		return added, fmt.Errorf("record installed built-ins: %w", err)
	}
	return added, nil
}

func readBuiltinState(dir string) (builtinState, error) {
	var st builtinState
	raw, err := os.ReadFile(filepath.Join(dir, builtinStateFile)) //nolint:gosec // fixed name under a known dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return st, nil
		}
		return st, fmt.Errorf("read %s: %w", builtinStateFile, err)
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		// A corrupt state file should not wedge startup. Treating it as empty
		// re-records what is already on disk without overwriting anything.
		return builtinState{}, nil
	}
	return st, nil
}

func writeBuiltinState(dir string, st builtinState) error {
	sort.Strings(st.Installed)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, builtinStateFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
