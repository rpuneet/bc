package secret

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/home"
)

// PassphraseEnvVar is the environment variable for the master passphrase.
const PassphraseEnvVar = "MYCEL_SECRET_PASSPHRASE" //nolint:gosec // not a credential, env var name constant

// SecretMeta holds secret metadata (never includes the value).
//
// Scope is runtime-only and reports which layer of a LayeredStore owns
// the secret ("global" or "workspace"). Plain-store lookups leave it
// empty.
type SecretMeta struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Scope       Scope     `json:"scope,omitempty"`
}

// Store provides encrypted secrets storage backed by SQLite or Postgres.
// Values are encrypted with AES-256-GCM; the encryption key is derived
// from a master passphrase via PBKDF2.
type Store struct {
	db  *db.DB
	pg  *PostgresStore // non-nil when using Postgres via OpenStore
	key []byte         // derived AES-256 key
}

// Passphrase returns the passphrase for secret encryption.
// Priority: MYCEL_SECRET_PASSPHRASE env var > key file at <mycel home>/secret-key
// > freshly generated key.
// The key file is created with 0600 permissions on first use.
func Passphrase() (string, error) {
	if p := os.Getenv(PassphraseEnvVar); p != "" {
		return p, nil
	}

	keyDir, err := home.MycelHome()
	if err != nil {
		return "", fmt.Errorf("cannot determine mycel home: %w", err)
	}
	keyPath := filepath.Join(keyDir, "secret-key")

	data, err := os.ReadFile(keyPath) //nolint:gosec // known path under mycel home
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read secret key file: %w", err)
	}

	// Generate a random 32-byte key and write it
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("generate secret key: %w", err)
	}
	key := hex.EncodeToString(keyBytes)

	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write secret key file: %w", err)
	}

	return key, nil
}

// NewStore creates a new secrets store for the given repo path.
// The passphrase is used to derive the encryption key via PBKDF2.
func NewStore(repoPath, passphrase string) (*Store, error) {
	dbPath := filepath.Join(repoPath, ".bc", "secrets.db")
	d, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open secrets database: %w", err)
	}

	s := &Store{db: d}
	if err := s.initSchema(); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init secrets schema: %w", err)
	}

	if err := s.initKey(passphrase); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("init encryption key: %w", err)
	}

	return s, nil
}

// initSchema creates the secrets tables.
func (s *Store) initSchema() error {
	ctx := context.Background()
	schema := `
		CREATE TABLE IF NOT EXISTS secrets (
			name        TEXT PRIMARY KEY,
			value       TEXT NOT NULL,
			description TEXT,
			created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);

		CREATE TABLE IF NOT EXISTS secret_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// initKey derives or loads the encryption key from the passphrase.
// On first run the salt is generated and stored; subsequent calls load it.
func (s *Store) initKey(passphrase string) error {
	ctx := context.Background()

	var saltB64 string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM secret_meta WHERE key = 'salt'",
	).Scan(&saltB64)

	if err == sql.ErrNoRows {
		// First time: generate and store salt
		salt, genErr := GenerateSalt()
		if genErr != nil {
			return genErr
		}
		saltB64 = base64.StdEncoding.EncodeToString(salt)
		if _, execErr := s.db.ExecContext(ctx,
			"INSERT INTO secret_meta (key, value) VALUES ('salt', ?)", saltB64,
		); execErr != nil {
			return fmt.Errorf("store salt: %w", execErr)
		}
		s.key = DeriveKey(passphrase, salt)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load salt: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}
	s.key = DeriveKey(passphrase, salt)
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.pg != nil {
		return s.pg.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Set creates or updates a secret with an encrypted value.
func (s *Store) Set(name, value, description string) error {
	if s.pg != nil {
		return s.pg.Set(name, value, description)
	}
	if name == "" {
		return fmt.Errorf("secret name is required")
	}

	encrypted, err := Encrypt(s.key, []byte(value))
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	ctx := context.Background()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secrets (name, value, description)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			value = excluded.value,
			description = COALESCE(NULLIF(excluded.description, ''), secrets.description),
			updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
	`, name, encrypted, description)
	if err != nil {
		return fmt.Errorf("set secret %q: %w", name, err)
	}
	return nil
}

// GetValue retrieves and decrypts a secret value.
func (s *Store) GetValue(name string) (string, error) {
	if s.pg != nil {
		return s.pg.GetValue(name)
	}
	ctx := context.Background()
	var encrypted string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM secrets WHERE name = ?", name,
	).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("secret %q not found", name)
	}
	if err != nil {
		return "", fmt.Errorf("get secret %q: %w", name, err)
	}

	plaintext, err := Decrypt(s.key, encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt secret %q: %w", name, err)
	}
	return string(plaintext), nil
}

// GetMeta returns metadata for a secret (no value).
func (s *Store) GetMeta(name string) (*SecretMeta, error) {
	if s.pg != nil {
		return s.pg.GetMeta(name)
	}
	ctx := context.Background()
	row := s.db.QueryRowContext(ctx,
		`SELECT name, description, created_at, updated_at
		 FROM secrets WHERE name = ?`, name,
	)
	return s.scanMeta(row)
}

// List returns metadata for all secrets (no values).
func (s *Store) List() ([]*SecretMeta, error) {
	if s.pg != nil {
		return s.pg.List()
	}
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx,
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
		var createdAt, updatedAt string
		if err := rows.Scan(&m.Name, &desc, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		m.Description = desc.String
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			m.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			m.UpdatedAt = t
		}
		secrets = append(secrets, &m)
	}
	return secrets, rows.Err()
}

// Delete removes a secret.
func (s *Store) Delete(name string) error {
	if s.pg != nil {
		return s.pg.Delete(name)
	}
	ctx := context.Background()
	result, err := s.db.ExecContext(ctx, "DELETE FROM secrets WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete secret %q: %w", name, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("secret %q not found", name)
	}
	return nil
}

// ResolveEnv resolves ${secret:NAME} references in env vars, returning
// a new map with secret values substituted. Unresolvable refs are left as-is.
func (s *Store) ResolveEnv(env map[string]string) map[string]string {
	if s.pg != nil {
		return s.pg.ResolveEnv(env)
	}
	resolved := make(map[string]string, len(env))
	for k, v := range env {
		resolved[k] = s.resolveValue(v)
	}
	return resolved
}

// resolveValue replaces ${secret:NAME} with the decrypted value.
func (s *Store) resolveValue(v string) string {
	const prefix = "${secret:"
	const suffix = "}"

	start := 0
	for {
		idx := strings.Index(v[start:], prefix)
		if idx < 0 {
			break
		}
		idx += start
		end := strings.Index(v[idx+len(prefix):], suffix)
		if end < 0 {
			break
		}
		end += idx + len(prefix)
		secretName := v[idx+len(prefix) : end]
		val, err := s.GetValue(secretName)
		if err != nil {
			start = end + 1
			continue
		}
		v = v[:idx] + val + v[end+1:]
		start = idx + len(val)
	}
	return v
}

// scanMeta scans a single row into SecretMeta.
func (s *Store) scanMeta(row *sql.Row) (*SecretMeta, error) {
	var m SecretMeta
	var desc sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(&m.Name, &desc, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan secret: %w", err)
	}

	m.Description = desc.String
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		m.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		m.UpdatedAt = t
	}
	return &m, nil
}
