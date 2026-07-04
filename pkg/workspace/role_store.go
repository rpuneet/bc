package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// RoleStore provides SQL-backed persistence for roles.
// It supports both SQLite and Postgres via the driver field.
type RoleStore struct {
	sqlDB  *sql.DB
	driver string // "sqlite" or "timescale"
	owned  bool   // true if we opened the connection (and should close it)
}

// NewRoleStore opens (or creates) a SQLite database at dbPath and ensures
// the roles table exists. The caller must call Close when done.
func NewRoleStore(dbPath string) (*RoleStore, error) {
	d, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open roles db: %w", err)
	}

	s := &RoleStore{sqlDB: d.DB, driver: "sqlite", owned: true}
	if err := s.InitSchema(); err != nil {
		_ = d.Close()
		return nil, err
	}

	return s, nil
}

// NewRoleStoreFromDB creates a RoleStore from an existing *sql.DB connection.
// The caller retains ownership of the connection — Close on this store is a no-op.
func NewRoleStoreFromDB(sqlDB *sql.DB, driver string) (*RoleStore, error) {
	s := &RoleStore{sqlDB: sqlDB, driver: driver, owned: false}
	if err := s.InitSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// InitSchema creates the roles table if it does not exist.
// Uses driver-appropriate column types.
func (s *RoleStore) InitSchema() error {
	var schema string
	if s.driver == "timescale" {
		schema = `
		CREATE TABLE IF NOT EXISTS roles (
			name          TEXT PRIMARY KEY,
			description   TEXT NOT NULL DEFAULT '',
			prompt        TEXT NOT NULL DEFAULT '',
			mcp_servers   TEXT NOT NULL DEFAULT '[]',
			parent_roles  TEXT NOT NULL DEFAULT '[]',
			secrets       TEXT NOT NULL DEFAULT '[]',
			plugins       TEXT NOT NULL DEFAULT '[]',
			settings      TEXT NOT NULL DEFAULT '{}',
			rules         TEXT NOT NULL DEFAULT '{}',
			agents        TEXT NOT NULL DEFAULT '{}',
			skills        TEXT NOT NULL DEFAULT '{}',
			commands      TEXT NOT NULL DEFAULT '{}',
			prompt_create TEXT NOT NULL DEFAULT '',
			prompt_start  TEXT NOT NULL DEFAULT '',
			prompt_stop   TEXT NOT NULL DEFAULT '',
			prompt_delete TEXT NOT NULL DEFAULT '',
			review        TEXT NOT NULL DEFAULT '',
			cli_tools     TEXT NOT NULL DEFAULT '[]',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`
	} else {
		schema = `
		CREATE TABLE IF NOT EXISTS roles (
			name          TEXT PRIMARY KEY,
			description   TEXT NOT NULL DEFAULT '',
			prompt        TEXT NOT NULL DEFAULT '',
			mcp_servers   TEXT NOT NULL DEFAULT '[]',
			parent_roles  TEXT NOT NULL DEFAULT '[]',
			secrets       TEXT NOT NULL DEFAULT '[]',
			plugins       TEXT NOT NULL DEFAULT '[]',
			settings      TEXT NOT NULL DEFAULT '{}',
			rules         TEXT NOT NULL DEFAULT '{}',
			agents        TEXT NOT NULL DEFAULT '{}',
			skills        TEXT NOT NULL DEFAULT '{}',
			commands      TEXT NOT NULL DEFAULT '{}',
			prompt_create TEXT NOT NULL DEFAULT '',
			prompt_start  TEXT NOT NULL DEFAULT '',
			prompt_stop   TEXT NOT NULL DEFAULT '',
			prompt_delete TEXT NOT NULL DEFAULT '',
			review        TEXT NOT NULL DEFAULT '',
			cli_tools     TEXT NOT NULL DEFAULT '[]',
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL
		);`
	}

	_, err := s.sqlDB.ExecContext(context.Background(), schema)
	if err != nil {
		return fmt.Errorf("create roles table: %w", err)
	}

	// Migration: add cli_tools column if not present (existing databases)
	_, _ = s.sqlDB.ExecContext(context.Background(),
		"ALTER TABLE roles ADD COLUMN cli_tools TEXT NOT NULL DEFAULT '[]'") //nolint:errcheck // ignore if already exists

	return nil
}

// placeholder returns the appropriate placeholder for the given 1-based index.
func (s *RoleStore) placeholder(n int) string {
	if s.driver == "timescale" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Save persists a single role (upsert).
func (s *RoleStore) Save(role *Role) error {
	if role.Metadata.Name == "" {
		return fmt.Errorf("role name is required")
	}

	mcpServers, err := json.Marshal(role.Metadata.MCPServers)
	if err != nil {
		return fmt.Errorf("marshal mcp_servers: %w", err)
	}

	parentRoles, err := json.Marshal(role.Metadata.ParentRoles)
	if err != nil {
		return fmt.Errorf("marshal parent_roles: %w", err)
	}

	secretsJSON, err := json.Marshal(role.Metadata.Secrets)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	pluginsJSON, err := json.Marshal(role.Metadata.Plugins)
	if err != nil {
		return fmt.Errorf("marshal plugins: %w", err)
	}

	settingsJSON, err := json.Marshal(role.Metadata.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	rulesJSON, err := json.Marshal(role.Metadata.Rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}

	agentsJSON, err := json.Marshal(role.Metadata.Agents)
	if err != nil {
		return fmt.Errorf("marshal agents: %w", err)
	}

	skillsJSON, err := json.Marshal(role.Metadata.Skills)
	if err != nil {
		return fmt.Errorf("marshal skills: %w", err)
	}

	commandsJSON, err := json.Marshal(role.Metadata.Commands)
	if err != nil {
		return fmt.Errorf("marshal commands: %w", err)
	}

	cliToolsJSON, err := json.Marshal(role.Metadata.CLITools)
	if err != nil {
		return fmt.Errorf("marshal cli_tools: %w", err)
	}

	now := time.Now().Format(time.RFC3339)

	ctx := context.Background()
	if s.driver == "timescale" {
		_, err = s.sqlDB.ExecContext(ctx, `
		INSERT INTO roles
		(name, description, prompt, mcp_servers, parent_roles, secrets, plugins,
		 settings, rules, agents, skills, commands,
		 prompt_create, prompt_start, prompt_stop, prompt_delete, review, cli_tools,
		 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW(), NOW())
		ON CONFLICT(name) DO UPDATE SET
		 description=$2, prompt=$3, mcp_servers=$4, parent_roles=$5, secrets=$6, plugins=$7,
		 settings=$8, rules=$9, agents=$10, skills=$11, commands=$12,
		 prompt_create=$13, prompt_start=$14, prompt_stop=$15, prompt_delete=$16, review=$17, cli_tools=$18,
		 updated_at=NOW()`,
			role.Metadata.Name, role.Metadata.Description, role.Prompt,
			string(mcpServers), string(parentRoles), string(secretsJSON), string(pluginsJSON),
			string(settingsJSON), string(rulesJSON), string(agentsJSON), string(skillsJSON), string(commandsJSON),
			role.Metadata.PromptCreate, role.Metadata.PromptStart,
			role.Metadata.PromptStop, role.Metadata.PromptDelete, role.Metadata.Review,
			string(cliToolsJSON),
		)
	} else {
		_, err = s.sqlDB.ExecContext(ctx, `
		INSERT OR REPLACE INTO roles
		(name, description, prompt, mcp_servers, parent_roles, secrets, plugins,
		 settings, rules, agents, skills, commands,
		 prompt_create, prompt_start, prompt_stop, prompt_delete, review, cli_tools,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		 COALESCE((SELECT created_at FROM roles WHERE name = ?), ?), ?)`,
			role.Metadata.Name, role.Metadata.Description, role.Prompt,
			string(mcpServers), string(parentRoles), string(secretsJSON), string(pluginsJSON),
			string(settingsJSON), string(rulesJSON), string(agentsJSON), string(skillsJSON), string(commandsJSON),
			role.Metadata.PromptCreate, role.Metadata.PromptStart,
			role.Metadata.PromptStop, role.Metadata.PromptDelete, role.Metadata.Review,
			string(cliToolsJSON),
			role.Metadata.Name, now, now,
		)
	}
	return err
}

// Load reads a single role by name. Returns an error if not found.
func (s *RoleStore) Load(name string) (*Role, error) {
	q := `SELECT name, description, prompt, mcp_servers, parent_roles, secrets, plugins,` + //nolint:gosec // G202: placeholder is "?" or "$1", not user input
		`       settings, rules, agents, skills, commands,
		       prompt_create, prompt_start, prompt_stop, prompt_delete, review, cli_tools,
		       created_at, updated_at
		FROM roles WHERE name = ` + s.placeholder(1)

	row := s.sqlDB.QueryRowContext(context.Background(), q, name)
	return scanRoleRow(row)
}

// LoadAll reads every role into a map keyed by name.
func (s *RoleStore) LoadAll() (map[string]*Role, error) {
	rows, err := s.sqlDB.QueryContext(context.Background(), `
		SELECT name, description, prompt, mcp_servers, parent_roles, secrets, plugins,
		       settings, rules, agents, skills, commands,
		       prompt_create, prompt_start, prompt_stop, prompt_delete, review, cli_tools,
		       created_at, updated_at
		FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	roles := make(map[string]*Role)
	for rows.Next() {
		role, scanErr := scanRoleRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		roles[role.Metadata.Name] = role
	}
	return roles, rows.Err()
}

// Delete removes a single role by name.
func (s *RoleStore) Delete(name string) error {
	q := "DELETE FROM roles WHERE name = " + s.placeholder(1) //nolint:gosec // G202: placeholder is either "?" or "$1", not user input
	res, err := s.sqlDB.ExecContext(context.Background(), q, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("role %q not found", name)
	}
	return nil
}

// Has checks if a role exists in the database.
func (s *RoleStore) Has(name string) bool {
	var count int
	q := "SELECT COUNT(*) FROM roles WHERE name = " + s.placeholder(1)
	err := s.sqlDB.QueryRowContext(context.Background(), q, name).Scan(&count)
	return err == nil && count > 0
}

// Close closes the database if this store owns the connection.
func (s *RoleStore) Close() error {
	if s.owned {
		return s.sqlDB.Close()
	}
	return nil
}

// MigrateFromFiles scans a roles directory for .md files and inserts
// them into the database. Existing roles in the DB are not overwritten.
// The source files are NOT deleted (kept as backup).
func (s *RoleStore) MigrateFromFiles(rolesDir string) (int, error) {
	// The dir is joined with entry names and read below — clean it and
	// reject traversal sequences before touching the filesystem.
	rolesDir = filepath.Clean(rolesDir)
	if strings.Contains(rolesDir, "..") {
		return 0, fmt.Errorf("unsafe roles directory: %s", rolesDir)
	}
	entries, err := os.ReadDir(rolesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read roles directory: %w", err)
	}

	var migrated int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// Entry names become path components — never follow one that
		// could escape the roles directory.
		if !filepath.IsLocal(entry.Name()) || strings.ContainsAny(entry.Name(), `/\`) {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")

		// Skip if already in DB
		if s.Has(name) {
			continue
		}

		filePath := filepath.Join(rolesDir, entry.Name())
		data, readErr := os.ReadFile(filePath) //nolint:gosec // rolesDir cleaned + ".."-rejected, entry name IsLocal-checked above
		if readErr != nil {
			return migrated, fmt.Errorf("read role file %s: %w", entry.Name(), readErr)
		}

		role, parseErr := ParseRoleFile(data)
		if parseErr != nil {
			return migrated, fmt.Errorf("parse role file %s: %w", entry.Name(), parseErr)
		}

		// Ensure name is set
		if role.Metadata.Name == "" {
			role.Metadata.Name = name
		}
		role.FilePath = filePath

		if saveErr := s.Save(role); saveErr != nil {
			return migrated, fmt.Errorf("save role %s: %w", name, saveErr)
		}
		migrated++
	}

	return migrated, nil
}

// MigrateDefaults inserts the built-in default roles (base, root, and
// DefaultRoles map) into the database if they don't already exist.
func (s *RoleStore) MigrateDefaults() error {
	// base role
	if !s.Has("base") {
		role, err := ParseRoleFile([]byte(DefaultBaseRole))
		if err != nil {
			return fmt.Errorf("parse default base role: %w", err)
		}
		if saveErr := s.Save(role); saveErr != nil {
			return fmt.Errorf("save default base role: %w", saveErr)
		}
	}

	// root role
	if !s.Has("root") {
		role, err := ParseRoleFile([]byte(DefaultRootRole))
		if err != nil {
			return fmt.Errorf("parse default root role: %w", err)
		}
		if saveErr := s.Save(role); saveErr != nil {
			return fmt.Errorf("save default root role: %w", saveErr)
		}
	}

	// other default roles
	for name, content := range DefaultRoles {
		if s.Has(name) {
			continue
		}
		role, err := ParseRoleFile([]byte(content))
		if err != nil {
			return fmt.Errorf("parse default role %s: %w", name, err)
		}
		if role.Metadata.Name == "" {
			role.Metadata.Name = name
		}
		if saveErr := s.Save(role); saveErr != nil {
			return fmt.Errorf("save default role %s: %w", name, saveErr)
		}
	}

	return nil
}

// scanRoleRow scans a role from a database row/rows scanner.
func scanRoleRow(scanner interface{ Scan(...any) error }) (*Role, error) {
	var (
		name, description, prompt                           string
		mcpServersJSON, parentRolesJSON                     string
		secretsJSON, pluginsJSON                            string
		settingsJSON, rulesJSON, agentsJSON                 string
		skillsJSON, commandsJSON                            string
		promptCreate, promptStart, promptStop, promptDelete string
		review, cliToolsJSON                                string
		createdAt, updatedAt                                any // any to handle both TEXT (SQLite) and TIMESTAMPTZ (Postgres)
	)

	err := scanner.Scan(
		&name, &description, &prompt,
		&mcpServersJSON, &parentRolesJSON, &secretsJSON, &pluginsJSON,
		&settingsJSON, &rulesJSON, &agentsJSON, &skillsJSON, &commandsJSON,
		&promptCreate, &promptStart, &promptStop, &promptDelete, &review, &cliToolsJSON,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("role not found")
		}
		return nil, err
	}

	role := &Role{
		Prompt: prompt,
		Metadata: RoleMetadata{
			Name:         name,
			Description:  description,
			PromptCreate: promptCreate,
			PromptStart:  promptStart,
			PromptStop:   promptStop,
			PromptDelete: promptDelete,
			Review:       review,
		},
	}

	// Unmarshal JSON array/object fields
	if mcpServersJSON != "" && mcpServersJSON != "[]" {
		_ = json.Unmarshal([]byte(mcpServersJSON), &role.Metadata.MCPServers) //nolint:errcheck // best-effort
	}
	if parentRolesJSON != "" && parentRolesJSON != "[]" {
		_ = json.Unmarshal([]byte(parentRolesJSON), &role.Metadata.ParentRoles) //nolint:errcheck
	}
	if secretsJSON != "" && secretsJSON != "[]" {
		_ = json.Unmarshal([]byte(secretsJSON), &role.Metadata.Secrets) //nolint:errcheck
	}
	if pluginsJSON != "" && pluginsJSON != "[]" {
		_ = json.Unmarshal([]byte(pluginsJSON), &role.Metadata.Plugins) //nolint:errcheck
	}
	if settingsJSON != "" && settingsJSON != "{}" {
		_ = json.Unmarshal([]byte(settingsJSON), &role.Metadata.Settings) //nolint:errcheck
	}
	if rulesJSON != "" && rulesJSON != "{}" {
		_ = json.Unmarshal([]byte(rulesJSON), &role.Metadata.Rules) //nolint:errcheck
	}
	if agentsJSON != "" && agentsJSON != "{}" {
		_ = json.Unmarshal([]byte(agentsJSON), &role.Metadata.Agents) //nolint:errcheck
	}
	if skillsJSON != "" && skillsJSON != "{}" {
		_ = json.Unmarshal([]byte(skillsJSON), &role.Metadata.Skills) //nolint:errcheck
	}
	if commandsJSON != "" && commandsJSON != "{}" {
		_ = json.Unmarshal([]byte(commandsJSON), &role.Metadata.Commands) //nolint:errcheck
	}
	if cliToolsJSON != "" && cliToolsJSON != "[]" {
		_ = json.Unmarshal([]byte(cliToolsJSON), &role.Metadata.CLITools) //nolint:errcheck
	}

	return role, nil
}
