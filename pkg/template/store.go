package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store is a file-based template store. Each template is stored as
// <name>.json (metadata) and an optional <name>.md (system prompt).
type Store struct {
	dir string
}

// NewStore creates a Store rooted at dir. The directory is created on first write.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// List returns all templates found in the store directory.
// Returns an empty slice (not an error) when the directory does not exist yet.
func (s *Store) List() ([]Template, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return []Template{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	var templates []Template
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t, _, err := s.Get(name)
		if err != nil {
			continue // skip unreadable files
		}
		templates = append(templates, *t)
	}
	if templates == nil {
		templates = []Template{}
	}
	return templates, nil
}

// Get returns the template and its system prompt markdown content.
// The system prompt is empty string when no .md file exists.
func (s *Store) Get(name string) (*Template, string, error) {
	if name == "" {
		return nil, "", fmt.Errorf("template name is required")
	}

	data, err := os.ReadFile(s.jsonPath(name))
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
	promptData, err := os.ReadFile(s.mdPath(name))
	if err == nil {
		prompt = string(promptData)
	}

	return &t, prompt, nil
}

// Create writes a new template. Returns an error if the template already exists.
func (s *Store) Create(t Template, systemPrompt string) error {
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if err := s.ensureDir(); err != nil {
		return err
	}

	jsonPath := s.jsonPath(t.Name)
	if _, err := os.Stat(jsonPath); err == nil {
		return fmt.Errorf("template %q already exists", t.Name)
	}

	if err := s.writeJSON(t); err != nil {
		return err
	}
	return s.writeMarkdown(t.Name, systemPrompt)
}

// Update overwrites an existing template. Returns an error if it does not exist.
func (s *Store) Update(name string, t Template, systemPrompt string) error {
	if name == "" {
		return fmt.Errorf("template name is required")
	}
	if _, err := os.Stat(s.jsonPath(name)); os.IsNotExist(err) {
		return fmt.Errorf("template %q not found", name)
	}
	t.Name = name
	if err := s.writeJSON(t); err != nil {
		return err
	}
	return s.writeMarkdown(name, systemPrompt)
}

// Delete removes both the .json and .md files for the named template.
func (s *Store) Delete(name string) error {
	if name == "" {
		return fmt.Errorf("template name is required")
	}

	jsonPath := s.jsonPath(name)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return fmt.Errorf("template %q not found", name)
	}

	if err := os.Remove(jsonPath); err != nil {
		return fmt.Errorf("delete template %q: %w", name, err)
	}

	// Remove .md file if present — ignore not-exist errors.
	mdPath := s.mdPath(name)
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete template prompt %q: %w", name, err)
	}

	return nil
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

	for _, t := range defaults {
		if err := s.Create(t, ""); err != nil {
			return fmt.Errorf("seed template %q: %w", t.Name, err)
		}
	}
	return nil
}

// --- internal helpers ---

func (s *Store) jsonPath(name string) string {
	return filepath.Join(s.dir, name+".json")
}

func (s *Store) mdPath(name string) string {
	return filepath.Join(s.dir, name+".md")
}

func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0750); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}
	return nil
}

func (s *Store) writeJSON(t Template) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal template %q: %w", t.Name, err)
	}
	if err := os.WriteFile(s.jsonPath(t.Name), data, 0640); err != nil { //nolint:gosec // 0640 intentional: group-readable config
		return fmt.Errorf("write template %q: %w", t.Name, err)
	}
	return nil
}

func (s *Store) writeMarkdown(name, content string) error {
	if err := os.WriteFile(s.mdPath(name), []byte(content), 0640); err != nil { //nolint:gosec // 0640 intentional
		return fmt.Errorf("write template prompt %q: %w", name, err)
	}
	return nil
}
