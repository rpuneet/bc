package secret

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setPassphraseTestHome pins HOME to a temp dir and clears every env
// var that could redirect key resolution.
func setPassphraseTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv("BC_HOME", "")
	t.Setenv(PassphraseEnvVar, "")
	return tmp
}

// TestPassphrase_LegacyKeyMigrated is the "don't brick decryption" test:
// with ~/.mycel present but the key still at the pre-rename
// ~/.bc/secret-key, Passphrase must return the legacy key and copy it
// into ~/.mycel/secret-key — never generate a fresh key.
func TestPassphrase_LegacyKeyMigrated(t *testing.T) {
	tmp := setPassphraseTestHome(t)
	if err := os.MkdirAll(filepath.Join(tmp, ".mycel"), 0o700); err != nil {
		t.Fatalf("mkdir .mycel: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".bc"), 0o700); err != nil {
		t.Fatalf("mkdir .bc: %v", err)
	}
	const legacyKey = "deadbeefcafe0123456789abcdef0123456789abcdef0123456789abcdef0123"
	if err := os.WriteFile(filepath.Join(tmp, ".bc", "secret-key"), []byte(legacyKey+"\n"), 0o600); err != nil {
		t.Fatalf("write legacy key: %v", err)
	}

	got, err := Passphrase()
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if got != legacyKey {
		t.Fatalf("Passphrase = %q, want legacy key", got)
	}

	// The key must now also live at the canonical path.
	canonical := filepath.Join(tmp, ".mycel", "secret-key")
	data, readErr := os.ReadFile(canonical) //nolint:gosec // test path under temp HOME
	if readErr != nil {
		t.Fatalf("canonical key not written: %v", readErr)
	}
	if strings.TrimSpace(string(data)) != legacyKey {
		t.Fatalf("canonical key = %q, want legacy key", strings.TrimSpace(string(data)))
	}

	// Second call reads the canonical copy and returns the same key.
	again, err := Passphrase()
	if err != nil {
		t.Fatalf("Passphrase (second call): %v", err)
	}
	if again != legacyKey {
		t.Fatalf("second Passphrase = %q, want legacy key", again)
	}
}

// TestPassphrase_GeneratesFreshKey: with no key anywhere, a key is
// generated and persisted under the mycel home, and subsequent calls
// return the same key.
func TestPassphrase_GeneratesFreshKey(t *testing.T) {
	tmp := setPassphraseTestHome(t)

	key, err := Passphrase()
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if len(key) != 64 { // 32 random bytes, hex-encoded
		t.Fatalf("generated key length = %d, want 64", len(key))
	}
	// Nothing existed, so MycelHome defaults to ~/.mycel.
	if _, statErr := os.Stat(filepath.Join(tmp, ".mycel", "secret-key")); statErr != nil {
		t.Fatalf("generated key not persisted: %v", statErr)
	}
	again, err := Passphrase()
	if err != nil {
		t.Fatalf("Passphrase (second call): %v", err)
	}
	if again != key {
		t.Fatalf("second Passphrase = %q, want %q", again, key)
	}
}

// TestPassphrase_EnvOverride: the env var beats every file.
func TestPassphrase_EnvOverride(t *testing.T) {
	setPassphraseTestHome(t)
	t.Setenv(PassphraseEnvVar, "from-env")
	got, err := Passphrase()
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	if got != "from-env" {
		t.Fatalf("Passphrase = %q, want env value", got)
	}
}
