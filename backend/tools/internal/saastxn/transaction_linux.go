//go:build linux

package saastxn

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// probeExchange verifies that two directories live on the same filesystem and
// that renameat2(RENAME_EXCHANGE) is supported. It performs a harmless probe by
// creating two temp dirs at the parent of a and attempting RENAME_EXCHANGE on
// them; on success it cleans up. This is done before any live mutation so an
// unsupported environment fails fast.
func probeExchange(a, b string) error {
	aAbs, err := filepath.Abs(a)
	if err != nil {
		return err
	}
	bAbs, err := filepath.Abs(b)
	if err != nil {
		return err
	}
	// Same-filesystem check via stat.
	var sa, sb unix.Stat_t
	if err := unix.Stat(aAbs, &sa); err != nil {
		return fmt.Errorf("stat %s: %w", aAbs, err)
	}
	if err := unix.Stat(filepath.Dir(bAbs), &sb); err != nil {
		return fmt.Errorf("stat parent of %s: %w", bAbs, err)
	}
	if sa.Dev != sb.Dev {
		return fmt.Errorf("directories not on the same filesystem (dev %d != %d)", sa.Dev, sb.Dev)
	}

	// Functional probe: create two sibling temp dirs and exchange them.
	parent := filepath.Dir(aAbs)
	probeA, err := os.MkdirTemp(parent, ".saastxn-probe-a-*")
	if err != nil {
		return fmt.Errorf("mkdirtemp probe-a: %w", err)
	}
	probeB, err := os.MkdirTemp(parent, ".saastxn-probe-b-*")
	if err != nil {
		_ = os.RemoveAll(probeA)
		return fmt.Errorf("mkdirtemp probe-b: %w", err)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, probeA, unix.AT_FDCWD, probeB, unix.RENAME_EXCHANGE); err != nil {
		_ = os.RemoveAll(probeA)
		_ = os.RemoveAll(probeB)
		// ENOSYS / EINVAL => kernel/filesystem lacks renameat2 RENAME_EXCHANGE.
		if err == unix.ENOSYS || err == unix.EINVAL || err == unix.ENOTSUP {
			return fmt.Errorf("renameat2 RENAME_EXCHANGE not supported: %w", err)
		}
		return fmt.Errorf("renameat2 probe failed: %w", err)
	}
	_ = os.RemoveAll(probeA)
	_ = os.RemoveAll(probeB)
	return nil
}

// exchangeDirectories atomically swaps the two directory trees a and b in
// place using renameat2(RENAME_EXCHANGE). Both must be on the same filesystem
// (callers should probeExchange first).
func exchangeDirectories(a, b string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("renameat2 exchange %s <-> %s: %w", a, b, err)
	}
	return nil
}

// errnoIs reports whether err is the target errno.
func errnoIs(err error, target syscall.Errno) bool {
	if err == nil {
		return false
	}
	return err == target
}
