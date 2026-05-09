package swim

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrLockBusy marks non-blocking flock contention.
var ErrLockBusy = errors.New("lock_busy")

// Lock is an exclusive file lock guard.
type Lock struct {
	f *os.File
}

// AcquireLock acquires an exclusive non-blocking lock at path.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %v", ErrLockBusy, err)
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the lock file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	unErr := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	clErr := l.f.Close()
	l.f = nil
	if unErr != nil {
		return fmt.Errorf("unlock: %w", unErr)
	}
	if clErr != nil {
		return fmt.Errorf("close lock file: %w", clErr)
	}
	return nil
}

// DeriveLockPath derives default lock path from schedule basename.
// Example: docs/plans/x.json -> .waveplan/swim/x/swim.lock
func DeriveLockPath(schedulePath string) string {
	base := filepath.Base(schedulePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = base
	}
	if name == "" {
		name = "default"
	}
	return filepath.Join(".waveplan", "swim", name, "swim.lock")
}
