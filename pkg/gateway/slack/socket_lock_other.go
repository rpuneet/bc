//go:build !unix

package slackgw

// socketFileLock is a no-op holder on platforms without flock.
type socketFileLock struct{}

func (l *socketFileLock) Release() error { return nil }

// tryLockSocketFile skips exclusive locking on non-unix platforms. Flap
// detection and docs still cover dual-client conflicts.
func tryLockSocketFile(path string) (*socketFileLock, error) {
	_ = path
	return &socketFileLock{}, nil
}
