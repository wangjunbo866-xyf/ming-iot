package saastxn

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// flockHandle is the default LockHandle backed by an flock(LOCK_EX) on
// <txnDir>/lock.
type flockHandle struct {
	f *os.File
}

func (h *flockHandle) Release() error {
	if h == nil || h.f == nil {
		return nil
	}
	_ = unix.Flock(int(h.f.Fd()), unix.LOCK_UN)
	return h.f.Close()
}

// acquireLock obtains the exclusive transaction lock, honoring an injected
// hook (used by tests to simulate contention).
func acquireLock(opts Options, txnDir string) (LockHandle, error) {
	if opts.LockAcquireHook != nil {
		return opts.LockAcquireHook(txnDir)
	}
	lockPath := filepath.Join(txnDir, "lock")
	if err := os.MkdirAll(txnDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir txn dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	// Blocking exclusive flock. A peer holding the lock serializes us.
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return &flockHandle{f: f}, nil
}
