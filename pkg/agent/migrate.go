package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// AgentState is the legacy per-agent JSON state format (v1).
// Only used during migration from JSON files to SQLite.
type AgentState struct {
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	Tool      string    `json:"tool,omitempty"`
	Team      string    `json:"team,omitempty"`
	Parent    string    `json:"parent,omitempty"`
	Worktree  string    `json:"worktree,omitempty"`
	Session   string    `json:"session,omitempty"`
	Role      Role      `json:"role"`
	State     State     `json:"state"`
}

// ToAgent converts a legacy AgentState to the current Agent struct.
func (s *AgentState) ToAgent(repo string) *Agent {
	return &Agent{
		Name:        s.Name,
		ID:          s.Name,
		Role:        s.Role,
		Tool:        s.Tool,
		ParentID:    s.Parent,
		State:       s.State,
		WorktreeDir: s.Worktree,
		Session:     s.Session,
		Workspace:   repo,
		StartedAt:   s.StartedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

const rootFileName = "root.json"

// migrateJSONToSQLite migrates agent state from JSON files to SQLite.
// It reads agents.json, root.json, and per-agent JSON files, saves each
// to the SQLite store, then renames the processed files to .migrated.
func migrateJSONToSQLite(store *SQLiteStore, stateDir, repo string) error {
	agentsDir := filepath.Join(stateDir, "agents")

	migrated := false

	// 1. Migrate agents.json (monolithic map[string]*Agent)
	agentsFile := filepath.Join(stateDir, "agents.json")
	if data, readErr := os.ReadFile(agentsFile); readErr == nil { //nolint:gosec // known path
		agents := make(map[string]*Agent)
		if parseErr := json.Unmarshal(data, &agents); parseErr == nil {
			for name, a := range agents {
				a.Name = name
				a.ID = name
				if a.Workspace == "" {
					a.Workspace = repo
				}
				if a.Repo == "" {
					a.Repo = cleanRepoPath(a.Workspace)
				}
				if a.StartedAt.IsZero() {
					a.StartedAt = time.Now()
				}
				if saveErr := store.Save(context.Background(), a); saveErr != nil {
					log.Warn("migrate: failed to save agent from agents.json", "agent", name, "error", saveErr)
				}
			}
			migrated = true
		} else {
			log.Warn("migrate: failed to parse agents.json", "error", parseErr)
		}
	}

	// 2. Migrate root.json (legacy root agent state)
	rootFile := filepath.Join(agentsDir, rootFileName)
	if data, readErr := os.ReadFile(rootFile); readErr == nil { //nolint:gosec // known path
		var state AgentState
		if parseErr := json.Unmarshal(data, &state); parseErr == nil {
			a := state.ToAgent(repo)
			a.IsRoot = true
			if a.Name == "" {
				a.Name = "root"
			}
			a.ID = a.Name
			if saveErr := store.Save(context.Background(), a); saveErr != nil {
				log.Warn("migrate: failed to save root agent", "error", saveErr)
			} else {
				migrated = true
			}
		} else {
			log.Warn("migrate: failed to parse root.json", "error", parseErr)
		}
	}

	// 3. Migrate per-agent JSON files in .bc/agents/*.json
	entries, err := os.ReadDir(agentsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Skip non-json, temp, already-migrated, and root.json
			if filepath.Ext(name) != ".json" || name[0] == '.' || name == rootFileName {
				continue
			}
			agentName := name[:len(name)-5] // strip .json

			path := filepath.Join(agentsDir, name)
			data, readErr := os.ReadFile(path) //nolint:gosec // constructed from known dir
			if readErr != nil {
				continue
			}

			var state AgentState
			if err := json.Unmarshal(data, &state); err != nil {
				log.Warn("migrate: failed to parse per-agent file", "file", name, "error", err)
				continue
			}

			// Only save if not already in DB (agents.json merge took priority)
			existing, _ := store.Load(context.Background(), agentName)
			if existing == nil {
				a := state.ToAgent(repo)
				if err := store.Save(context.Background(), a); err != nil {
					log.Warn("migrate: failed to save per-agent state", "agent", agentName, "error", err)
				} else {
					migrated = true
				}
			}
		}
	}

	// 4. Rename processed files to .migrated
	if migrated {
		renameIfExists(agentsFile, agentsFile+".migrated")
		renameIfExists(rootFile, rootFile+".migrated")

		// Rename per-agent JSONs
		for _, entry := range entries {
			name := entry.Name()
			if filepath.Ext(name) != ".json" || name[0] == '.' || name == rootFileName {
				continue
			}
			src := filepath.Join(agentsDir, name)
			renameIfExists(src, src+".migrated")
		}
	}

	return nil
}

// needsMigration checks whether JSON state files exist that should be migrated.
func needsMigration(stateDir string) bool {
	agentsFile := filepath.Join(stateDir, "agents.json")
	if _, err := os.Stat(agentsFile); err == nil {
		return true
	}

	rootFile := filepath.Join(stateDir, "agents", rootFileName)
	if _, err := os.Stat(rootFile); err == nil {
		return true
	}

	// Check for any per-agent JSONs
	agentsDir := filepath.Join(stateDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && filepath.Ext(name) == ".json" && name[0] != '.' {
			return true
		}
	}
	return false
}

func renameIfExists(src, dst string) {
	if _, err := os.Stat(src); err == nil {
		if err := os.Rename(src, dst); err != nil {
			log.Warn("migrate: failed to rename", "src", src, "dst", dst, "error", err)
		}
	}
}
