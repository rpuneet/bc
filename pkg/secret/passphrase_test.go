package secret

import (
	"os"
	"path/filepath"
	"testing"
)

// setPassphraseTestHome pins HOME to a temp dir and clears every env
// var that could redirect key resolution.
func setPassphraseTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("MYCEL_HOME", "")
	t.Setenv(PassphraseEnvVar, "")
	return tmp
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
