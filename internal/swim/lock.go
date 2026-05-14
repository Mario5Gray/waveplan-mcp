package swim

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLockBusy marks non-blocking flock contention.
var ErrLockBusy = errors.New("lock_busy")

// Lock is an exclusive file lock guard.
type Lock struct {
	f *os.File
}

type lockHolder struct {
	Pid       int    `json:"pid"`
	StartedAt string `json:"started_at"`
	Hostname  string `json:"hostname"`
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

// WriteLockHolder records operator-facing holder metadata into the lock file.
func WriteLockHolder(f *os.File, pid int, startedAt string) error {
	if f == nil {
		return fmt.Errorf("nil lock file")
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	body, err := json.Marshal(lockHolder{
		Pid:       pid,
		StartedAt: startedAt,
		Hostname:  hostname,
	})
	if err != nil {
		return fmt.Errorf("marshal lock holder: %w", err)
	}
	body = append(body, '\n')
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("truncate lock file: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek lock file: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		return fmt.Errorf("write lock holder: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync lock file: %w", err)
	}
	return nil
}

// ReadLockHolder reads the lock-holder metadata from the lock file.
func ReadLockHolder(path string) (int, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, "", fmt.Errorf("read lock holder: %w", err)
	}
	var holder lockHolder
	if err := json.Unmarshal(body, &holder); err != nil {
		return 0, "", fmt.Errorf("decode lock holder: %w", err)
	}
	return holder.Pid, holder.StartedAt, nil
}
