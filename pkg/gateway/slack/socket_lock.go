package slackgw

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// socketLockDirEnv overrides the directory used for Socket Mode app-token
// locks. Tests set this; production leaves it unset so locks live under the
// per-user cache dir (shared across MYCEL_HOME variants on the same machine).
const socketLockDirEnv = "MYCEL_SLACK_SOCKET_LOCK_DIR"

// appTokenLockPath returns the exclusive-lock file path for appToken.
// The path embeds only a hash of the token — never the secret itself.
func appTokenLockPath(appToken string) (string, error) {
	dir, err := resolveSocketLockDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(appToken))
	name := hex.EncodeToString(sum[:8]) + ".lock"
	return filepath.Join(dir, name), nil
}

func resolveSocketLockDir() (string, error) {
	if d := os.Getenv(socketLockDirEnv); d != "" {
		return d, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}
	return filepath.Join(cache, "mycel", "slack-socket-locks"), nil
}

// acquireAppTokenLock refuses a second local Socket Mode client that shares
// the same SLACK_APP_TOKEN. Slack delivers Events API payloads to only one
// Socket Mode connection; two mycel daemons (e.g. a dogfood daemon and a
// test daemon with a different MYCEL_HOME) silently steal the stream.
//
// The lock lives outside MYCEL_HOME so distinct homes on one OS user still
// conflict. Returns a no-op release when appToken is empty or the platform
// cannot flock.
func acquireAppTokenLock(appToken string) (release func(), err error) {
	noop := func() {}
	if appToken == "" {
		return noop, nil
	}
	path, err := appTokenLockPath(appToken)
	if err != nil {
		return noop, fmt.Errorf("slack: socket lock path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return noop, fmt.Errorf("slack: create socket lock dir: %w", err)
	}
	lock, err := tryLockSocketFile(path)
	if err != nil {
		return noop, fmt.Errorf(
			"slack: another local process already holds Socket Mode for this SLACK_APP_TOKEN (lock %s): %w; "+
				"only one Socket Mode client receives Events API messages — stop the other mycel daemon "+
				"or unset Slack tokens in secondary/test environments",
			path, err,
		)
	}
	if lock == nil {
		return noop, nil
	}
	return func() { _ = lock.Release() }, nil
}
