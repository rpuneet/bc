//go:build unix

package slackgw

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// socketFileLock holds an exclusive flock on a Socket Mode lock file.
type socketFileLock struct {
	f *os.File
}

func (l *socketFileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}

func tryLockSocketFile(path string) (*socketFileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		holder := readLockHolder(f)
		_ = f.Close()
		if holder != "" {
			return nil, fmt.Errorf("held by pid %s: %w", holder, err)
		}
		return nil, err
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.Seek(0, 0)
		_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
		_ = f.Sync()
	}
	return &socketFileLock{f: f}, nil
}

func readLockHolder(f *os.File) string {
	buf := make([]byte, 64)
	n, err := f.ReadAt(buf, 0)
	if err != nil && n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}
