package template

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
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

// builtinState tracks what EnsureBuiltins has done in a templates directory.
//
// Installed is the set of names that have ever been seeded here — including
// ones the user later deleted — so a deliberate deletion is not undone on
// the next start.
//
// Hashes maps name → sha256 of the JSON+markdown pair EnsureBuiltins last
// wrote. When the on-disk pair still matches that hash, the file is ours to
// upgrade or withdraw; when it does not, the user edited it and we leave it
// alone (#3573).
//
// Withdrawn lists names we used to ship and no longer do. They stay out of
// Installed's reinstall path forever, even after the embed drops them.
type builtinState struct {
	Hashes    map[string]string `json:"hashes,omitempty"`
	Installed []string          `json:"installed"`
	Withdrawn []string          `json:"withdrawn,omitempty"`
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

// builtinContentHash returns a stable hex digest of the JSON+markdown pair.
// Both halves are included so a prompt-only or metadata-only change is seen.
func builtinContentHash(jsonBytes []byte, prompt string) string {
	sum := sha256.New()
	_, _ = sum.Write(jsonBytes)
	_, _ = sum.Write([]byte{0}) // separator so "ab"+"c" ≠ "a"+"bc"
	_, _ = sum.Write([]byte(prompt))
	return hex.EncodeToString(sum.Sum(nil))
}

// shippedBuiltinHash is the content hash of the currently embedded pair.
func shippedBuiltinHash(name string) (string, error) {
	raw, err := builtinFS.ReadFile(filepath.Join(builtinDir, name+".json"))
	if err != nil {
		return "", err
	}
	prompt := ""
	if md, mdErr := builtinFS.ReadFile(filepath.Join(builtinDir, name+".md")); mdErr == nil {
		prompt = string(md)
	}
	return builtinContentHash(raw, prompt), nil
}

// diskBuiltinHash is the content hash of the on-disk pair, or "" when the
// JSON side is missing.
func diskBuiltinHash(dir, name string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".json")) //nolint:gosec // path under templates dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	prompt := ""
	if md, mdErr := os.ReadFile(filepath.Join(dir, name+".md")); mdErr == nil { //nolint:gosec
		prompt = string(md)
	} else if !errors.Is(mdErr, fs.ErrNotExist) {
		return "", mdErr
	}
	return builtinContentHash(raw, prompt), nil
}

// EnsureBuiltins installs, upgrades, or withdraws built-in templates in dir
// and returns the names it newly added (not upgrades).
//
// It runs on every start so an upgrade delivers new built-ins to an existing
// workspace. Behavior for each shipped name:
//
//   - never installed, absent on disk → install and record hash
//   - installed before, absent on disk → leave deleted (user choice)
//   - on disk, hash matches what we last wrote → ours: upgrade if the embed
//     changed, otherwise leave
//   - on disk, hash does not match (or no hash recorded) → user edit or a
//     pre-hash install: leave alone, remember the name so a later delete sticks
//
// Names listed in state.Withdrawn, or present in WithdrawnBuiltins(), are
// never reinstalled; on-disk copies whose hash still matches what we wrote
// are removed so a shipped deletion reaches existing workspaces.
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
	if state.Hashes == nil {
		state.Hashes = map[string]string{}
	}
	known := make(map[string]bool, len(state.Installed))
	for _, n := range state.Installed {
		known[n] = true
	}
	withdrawn := make(map[string]bool, len(state.Withdrawn)+len(WithdrawnBuiltins()))
	for _, n := range state.Withdrawn {
		withdrawn[n] = true
	}
	for _, n := range WithdrawnBuiltins() {
		withdrawn[n] = true
		if !containsString(state.Withdrawn, n) {
			state.Withdrawn = append(state.Withdrawn, n)
		}
	}

	var added []string

	// Drop on-disk copies of withdrawn builtins we still own.
	for name := range withdrawn {
		if err := withdrawOwnedBuiltin(dir, name, &state); err != nil {
			return added, err
		}
		known[name] = true // so a later ship of the same name still respects deletion intent
	}

	for _, name := range BuiltinNames() {
		if withdrawn[name] {
			continue
		}
		shipHash, hashErr := shippedBuiltinHash(name)
		if hashErr != nil {
			return added, hashErr
		}

		diskHash, diskErr := diskBuiltinHash(dir, name)
		if diskErr != nil {
			return added, diskErr
		}

		if diskHash == "" {
			// Absent on disk.
			if known[name] {
				continue // user deleted it
			}
			if installErr := installShippedBuiltin(dir, name, shipHash, &state); installErr != nil {
				return added, installErr
			}
			known[name] = true
			added = append(added, name)
			continue
		}

		// Present on disk.
		recorded := state.Hashes[name]
		if recorded != "" && diskHash == recorded {
			// Still the bytes we wrote — safe to upgrade when the embed moved.
			if diskHash == shipHash {
				if !known[name] {
					state.Installed = append(state.Installed, name)
					known[name] = true
				}
				continue
			}
			if installErr := installShippedBuiltin(dir, name, shipHash, &state); installErr != nil {
				return added, fmt.Errorf("upgrade built-in template %q: %w", name, installErr)
			}
			if !known[name] {
				known[name] = true
			}
			continue
		}

		// Edit, or a pre-hash install we cannot safely overwrite.
		if !known[name] {
			state.Installed = append(state.Installed, name)
			known[name] = true
		}
	}

	if err := writeBuiltinState(dir, state); err != nil {
		// The templates are installed; failing to record that only risks
		// reinstalling a deleted one later, which is not worth failing startup.
		return added, fmt.Errorf("record installed built-ins: %w", err)
	}
	return added, nil
}

// WithdrawnBuiltins returns names that used to ship and must not return.
// The 35 task-prompt templates from v0.4.5 are withdrawn in #3552; only
// blank remains embedded as a thin single-agent starting point. Names stay
// listed here so EnsureBuiltins can remove unedited copies from existing
// workspaces after the embed files are gone.
//
//nolint:misspell // "archaeologist" is the historical shipped template name
func WithdrawnBuiltins() []string {
	return []string{
		"accessibility-audit",
		"api-designer",
		"backend-service",
		"bug-fix",
		"changelog",
		"ci-fixer",
		"containerize",
		"cost-optimizer",
		"data-pipeline",
		"db-migration",
		"dependency-upgrade",
		"devops-infra",
		"docs-writer",
		"feature-dev",
		"frontend-ui",
		"i18n",
		"integration-builder",
		"issue-triage",
		"legacy-archaeologist",
		"manager",
		"ml-experiment",
		"observability",
		"oncall-responder",
		"perf-optimizer",
		"pr-shepherd",
		"refactor",
		"release-manager",
		"researcher",
		"reviewer",
		"scraper",
		"security-audit",
		"spec-writer",
		"sql-optimizer",
		"test-writer",
		"type-tightener",
	}
}

func installShippedBuiltin(dir, name, shipHash string, state *builtinState) error {
	jsonBytes, err := builtinFS.ReadFile(filepath.Join(builtinDir, name+".json"))
	if err != nil {
		return err
	}
	prompt := []byte(nil)
	if md, mdErr := builtinFS.ReadFile(filepath.Join(builtinDir, name+".md")); mdErr == nil {
		prompt = md
	} else if !errors.Is(mdErr, fs.ErrNotExist) {
		return mdErr
	}
	if writeErr := writeRawBuiltinPair(dir, name, jsonBytes, prompt); writeErr != nil {
		return fmt.Errorf("install built-in template %q: %w", name, writeErr)
	}
	if !containsString(state.Installed, name) {
		state.Installed = append(state.Installed, name)
	}
	state.Hashes[name] = shipHash
	return nil
}

func withdrawOwnedBuiltin(dir, name string, state *builtinState) error {
	recorded := state.Hashes[name]
	if recorded == "" {
		return nil // never ours under the hash scheme — leave edits alone
	}
	diskHash, err := diskBuiltinHash(dir, name)
	if err != nil || diskHash == "" {
		delete(state.Hashes, name)
		return err
	}
	if diskHash != recorded {
		return nil // user edited after we wrote it
	}
	jsonPath := filepath.Join(dir, name+".json")
	mdPath := filepath.Join(dir, name+".md")
	if rmErr := os.Remove(jsonPath); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return fmt.Errorf("withdraw %q json: %w", name, rmErr)
	}
	if rmErr := os.Remove(mdPath); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		return fmt.Errorf("withdraw %q markdown: %w", name, rmErr)
	}
	delete(state.Hashes, name)
	return nil
}

// writeRawBuiltinPair writes the embedded bytes as-is so the on-disk hash
// matches shippedBuiltinHash (re-marshaling through Template would drift).
func writeRawBuiltinPair(dir, name string, jsonBytes, prompt []byte) error {
	if !validName(name) {
		return fmt.Errorf("invalid template name %q", name)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	jsonPath := filepath.Join(dir, name+".json")
	mdPath := filepath.Join(dir, name+".md")
	tmpJSON := jsonPath + ".tmp"
	tmpMD := mdPath + ".tmp"
	if err := os.WriteFile(tmpJSON, jsonBytes, 0o640); err != nil { //nolint:gosec
		return err
	}
	if err := os.WriteFile(tmpMD, prompt, 0o640); err != nil { //nolint:gosec
		_ = os.Remove(tmpJSON)
		return err
	}
	if err := os.Rename(tmpJSON, jsonPath); err != nil {
		_ = os.Remove(tmpJSON)
		_ = os.Remove(tmpMD)
		return err
	}
	if err := os.Rename(tmpMD, mdPath); err != nil {
		_ = os.Remove(tmpMD)
		return err
	}
	return nil
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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
	sort.Strings(st.Withdrawn)
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
