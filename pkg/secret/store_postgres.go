package secret

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	bcdb "github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
)

// PostgresStore provides Postgres-backed encrypted secrets storage.
type PostgresStore struct {
	db  *sql.DB
	key []byte // derived AES-256 key
}

// NewPostgresStore creates a PostgresStore from an existing *sql.DB connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// InitSchema creates the secrets tables in Postgres if they don't exist.
func (p *PostgresStore) InitSchema() error {
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS secrets (
			name        TEXT PRIMARY KEY,
			value       TEXT NOT NULL,
			description TEXT,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS secret_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range stmts {
		if _, err := p.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres secrets schema: %w", err)
		}
	}
	return nil
}

// InitKey derives or loads the encryption key from the passphrase.
func (p *PostgresStore) InitKey(passphrase string) error {
	ctx := context.Background()

	var saltB64 string
	err := p.db.QueryRowContext(ctx,
		"SELECT value FROM secret_meta WHERE key = 'salt'",
	).Scan(&saltB64)

	if err == sql.ErrNoRows {
		salt := make([]byte, 16)
		if _, genErr := rand.Read(salt); genErr != nil {
			return fmt.Errorf("generate salt: %w", genErr)
		}
		saltB64 = base64.StdEncoding.EncodeToString(salt)
		if _, execErr := p.db.ExecContext(ctx,
			"INSERT INTO secret_meta (key, value) VALUES ('salt', $1)", saltB64,
		); execErr != nil {
			return fmt.Errorf("store salt: %w", execErr)
		}
		p.key = DeriveKey(passphrase, salt)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load salt: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}
	p.key = DeriveKey(passphrase, salt)
	return nil
}

// Close closes the database connection.
// Close is a no-op — the shared DB is owned by the caller.
func (p *PostgresStore) Close() error {
	return nil
}

// Set creates or updates a secret with an encrypted value.
func (p *PostgresStore) Set(name, value, description string) error {
	if name == "" {
		return fmt.Errorf("secret name is required")
	}

	encrypted, err := Encrypt(p.key, []byte(value))
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	ctx := context.Background()
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO secrets (name, value, description)
		VALUES ($1, $2, $3)
		ON CONFLICT(name) DO UPDATE SET
			value = EXCLUDED.value,
			description = COALESCE(NULLIF(EXCLUDED.description, ''), secrets.description),
			updated_at = NOW()
	`, name, encrypted, description)
	if err != nil {
		return fmt.Errorf("set secret %q: %w", name, err)
	}
	return nil
}

// GetValue retrieves and decrypts a secret value.
func (p *PostgresStore) GetValue(name string) (string, error) {
	ctx := context.Background()
	var encrypted string
	err := p.db.QueryRowContext(ctx,
		"SELECT value FROM secrets WHERE name = $1", name,
	).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("secret %q not found", name)
	}
	if err != nil {
		return "", fmt.Errorf("get secret %q: %w", name, err)
	}

	plaintext, err := Decrypt(p.key, encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt secret %q: %w", name, err)
	}
	return string(plaintext), nil
}

// GetMeta returns metadata for a secret (no value).
func (p *PostgresStore) GetMeta(name string) (*SecretMeta, error) {
	ctx := context.Background()
	row := p.db.QueryRowContext(ctx,
		`SELECT name, description, created_at, updated_at
		 FROM secrets WHERE name = $1`, name,
	)

	var m SecretMeta
	var desc sql.NullString
	var createdAt, updatedAt time.Time

	err := row.Scan(&m.Name, &desc, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan secret: %w", err)
	}

	m.Description = desc.String
	m.CreatedAt = createdAt
	m.UpdatedAt = updatedAt
	return &m, nil
}

// List returns metadata for all secrets (no values).
func (p *PostgresStore) List() ([]*SecretMeta, error) {
	ctx := context.Background()
	rows, err := p.db.QueryContext(ctx,
		`SELECT name, description, created_at, updated_at
		 FROM secrets ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var secrets []*SecretMeta
	for rows.Next() {
		var m SecretMeta
		var desc sql.NullString
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&m.Name, &desc, &createdAt, &updatedAt); scanErr != nil {
			return nil, fmt.Errorf("scan secret: %w", scanErr)
		}
		m.Description = desc.String
		m.CreatedAt = createdAt
		m.UpdatedAt = updatedAt
		secrets = append(secrets, &m)
	}
	return secrets, rows.Err()
}

// Delete removes a secret.
func (p *PostgresStore) Delete(name string) error {
	ctx := context.Background()
	result, err := p.db.ExecContext(ctx, "DELETE FROM secrets WHERE name = $1", name)
	if err != nil {
		return fmt.Errorf("delete secret %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("secret %q not found", name)
	}
	return nil
}

// ResolveEnv resolves ${secret:NAME} references in env vars.
func (p *PostgresStore) ResolveEnv(env map[string]string) map[string]string {
	resolved := make(map[string]string, len(env))
	for k, v := range env {
		resolved[k] = p.resolveValue(v)
	}
	return resolved
}

// resolveValue replaces ${secret:NAME} with the decrypted value.
func (p *PostgresStore) resolveValue(v string) string {
	const prefix = "${secret:"
	const suffix = "}"

	start := 0
	for {
		idx := findIndex(v[start:], prefix)
		if idx < 0 {
			break
		}
		idx += start
		end := findIndex(v[idx+len(prefix):], suffix)
		if end < 0 {
			break
		}
		end += idx + len(prefix)
		secretName := v[idx+len(prefix) : end]
		val, err := p.GetValue(secretName)
		if err != nil {
			start = end + 1
			continue
		}
		v = v[:idx] + val + v[end+1:]
		start = idx + len(val)
	}
	return v
}

// findIndex returns the index of substr in s, or -1 if not found.
func findIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// OpenStore opens the secrets store on the given workspace database.
// The handle is borrowed: callers own its lifecycle. The secret store
// keeps encryption isolation — its own salt and key derivation. In
// SQLite mode it still uses its own file under repoPath.
func OpenStore(d *bcdb.DB, driver, repoPath, passphrase string) (*Store, error) {
	if d != nil && driver == "timescale" {
		pg := NewPostgresStore(d.DB)
		if schemaErr := pg.InitSchema(); schemaErr != nil {
			return nil, fmt.Errorf("secrets store: init timescale schema: %w", schemaErr)
		}
		if keyErr := pg.InitKey(passphrase); keyErr != nil {
			return nil, fmt.Errorf("secrets store: init encryption key: %w", keyErr)
		}
		log.Debug("secrets store: using TimescaleDB backend")
		return &Store{pg: pg}, nil
	}

	// SQLite fallback (still uses its own file for encryption isolation)
	return NewStore(repoPath, passphrase)
}
