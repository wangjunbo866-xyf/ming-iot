//go:build !linux

package saastxn

import "errors"

// probeExchange always fails on non-Linux platforms: renameat2(RENAME_EXCHANGE)
// is a Linux-specific syscall.
func probeExchange(a, b string) error {
	return errors.New("renameat2(RENAME_EXCHANGE) is Linux-only")
}

// exchangeDirectories always fails on non-Linux platforms.
func exchangeDirectories(a, b string) error {
	return errors.New("renameat2(RENAME_EXCHANGE) is Linux-only")
}
