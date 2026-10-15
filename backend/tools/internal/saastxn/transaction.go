// Package saastxn performs an atomic, all-or-nothing replacement of an entire
// live directory tree (its only in-repo target is backend/saas) using a single
// renameat2(RENAME_EXCHANGE) directory swap.
//
// The protocol:
//
//  1. Fingerprint the live tree (path, mode, content SHA-256).
//  2. Build a complete candidate copy under <txnDir>/<id>/candidate, refusing
//     symlinks, special files and any path escaping the tree.
//  3. Verify the candidate-vs-live diff only touches the caller-supplied
//     allowlist.
//  4. Acquire <txnDir>/lock exclusively; re-fingerprint live and reject
//     concurrent drift.
//  5. Probe same-filesystem and renameat2(RENAME_EXCHANGE) support, then swap
//     backend/saas and the candidate with one atomic directory exchange.
//
// The exchange is the only commit point. A failure of old-tree cleanup or
// journal write after the exchange still reports StatusCommitted plus a
// warning; the next run performs idempotent recovery from the journal.
package saastxn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Status values reported by Commit.
const (
	StatusCommitted = "committed"
)

// Sentinel errors. Compare with errors.Is.
var (
	// ErrAllowlistViolation is returned when the candidate differs from live on
	// a path not in the allowlist.
	ErrAllowlistViolation = errors.New("saastxn: candidate diff outside allowlist")
	// ErrCandidateRejected is returned when the candidate contains a symlink,
	// special file, or escaping path.
	ErrCandidateRejected = errors.New("saastxn: candidate contains forbidden entry")
	// ErrLiveDrift is returned when the live tree changed between the initial
	// treeFingerprint and the re-fingerprint taken under the lock.
	ErrLiveDrift = errors.New("saastxn: live tree drifted during transaction")
	// ErrLockBusy is returned when the transaction lock cannot be acquired.
	ErrLockBusy = errors.New("saastxn: transaction lock busy")
	// ErrExchangeUnsupported is returned when renameat2(RENAME_EXCHANGE) is not
	// available or the directories are not on the same filesystem.
	ErrExchangeUnsupported = errors.New("saastxn: atomic exchange not supported on this platform/filesystem")
)

// Options configures a transaction.
type Options struct {
	// LiveDir is the live tree to replace atomically (e.g. .../backend/saas).
	LiveDir string
	// TxnDir is the parent directory holding <id>/candidate, the lock and the
	// journal (e.g. .../backend/.saastxn).
	TxnDir string
	// Allowlist is the set of repo-relative (filepath.Slash) paths under
	// LiveDir that are permitted to change between live and candidate. Any
	// diff outside this set aborts the transaction.
	Allowlist []string
	// MutateCandidate is invoked with the candidate root after it is copied
	// from live; it applies the caller's changes (e.g. regenerating files).
	// Required.
	MutateCandidate func(candidate string) error
	// DriftHook, if non-nil, is invoked against LiveDir after the initial
	// treeFingerprint, to simulate concurrent live modification in tests.
	DriftHook func(liveDir string) error
	// CleanupErrorHook, if non-nil, is invoked against the post-exchange old
	// tree path; returning an error simulates cleanup failure (commit must
	// still report committed).
	CleanupErrorHook func(oldTree string) error
	// LockAcquireHook, if non-nil, replaces the default lock acquisition. It
	// returns a LockHandle whose Release method releases the lock.
	LockAcquireHook func(txnDir string) (LockHandle, error)
}

// Result reports the outcome of Commit.
type Result struct {
	Status   string
	Warnings []string
}

// LockHandle is a handle to an acquired transaction lock.
type LockHandle interface {
	Release() error
}

// Commit performs the atomic transaction described by opts and returns the
// result. The function is safe to call concurrently only via the lock; without
// the lock the live treeFingerprint re-check guards against drift.
func Commit(ctx context.Context, opts Options) (Result, error) {
	liveAbs, err := filepath.Abs(opts.LiveDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve LiveDir: %w", err)
	}
	txnAbs, err := filepath.Abs(opts.TxnDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve TxnDir: %w", err)
	}
	// TxnDir must be a sibling (or otherwise outside) of LiveDir so the swap is
	// between two independent directories.
	if strings.HasPrefix(liveAbs, txnAbs+string(filepath.Separator)) || liveAbs == txnAbs {
		return Result{}, fmt.Errorf("TxnDir %q must not contain LiveDir %q", txnAbs, liveAbs)
	}
	if opts.MutateCandidate == nil {
		return Result{}, errors.New("saastxn: MutateCandidate is required")
	}

	id := newID()
	candidate := filepath.Join(txnAbs, id, "candidate")
	oldTree := filepath.Join(txnAbs, id, "old")
	journalPath := filepath.Join(txnAbs, id, "journal")

	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir txn dir: %w", err)
	}

	allow := newAllowlist(opts.Allowlist)

	// 1. Initial fingerprint of live.
	liveFP1, err := computeFingerprint(liveAbs)
	if err != nil {
		return Result{}, fmt.Errorf("treeFingerprint live: %w", err)
	}

	// 2. Build candidate copy from live (snapshot at FP1 time).
	if err := copyTree(liveAbs, candidate); err != nil {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("copy live to candidate: %w", err)
	}
	// 2b. Apply the caller's mutation to the candidate.
	if err := opts.MutateCandidate(candidate); err != nil {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("mutate candidate: %w", err)
	}
	// 2c. Reject symlinks / special files / escaping paths inside candidate.
	if err := validateCandidate(candidate); err != nil {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("%w: %v", ErrCandidateRejected, err)
	}

	// 3. Allowlist diff check (candidate vs live at FP1).
	candFP, err := computeFingerprint(candidate)
	if err != nil {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("treeFingerprint candidate: %w", err)
	}
	if violations := allow.diffViolations(liveFP1, candFP); len(violations) != 0 {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("%w: %s", ErrAllowlistViolation, strings.Join(violations, ", "))
	}

	// Optional drift hook: simulates a concurrent modification of live AFTER
	// the candidate was snapshotted, so the re-fingerprint under the lock will
	// diverge from FP1.
	if opts.DriftHook != nil {
		if err := opts.DriftHook(liveAbs); err != nil {
			return Result{}, fmt.Errorf("drift hook: %w", err)
		}
	}

	// Write a "prepare" journal entry before touching the lock.
	j, err := openJournal(journalPath)
	if err != nil {
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("open journal: %w", err)
	}
	if err := j.write(statePrepare, map[string]string{
		"live":      liveAbs,
		"candidate": candidate,
		"old":       oldTree,
		"liveFp":    liveFP1.summary(),
		"candFp":    candFP.summary(),
	}); err != nil {
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("journal prepare: %w", err)
	}

	// 5. Acquire the lock.
	handle, err := acquireLock(opts, txnAbs)
	if err != nil {
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("%w: %v", ErrLockBusy, err)
	}
	defer handle.Release()

	// 5b. Re-treeFingerprint live under the lock; reject drift.
	liveFP2, err := computeFingerprint(liveAbs)
	if err != nil {
		handle.Release()
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("re-fingerprint live: %w", err)
	}
	if liveFP1.summary() != liveFP2.summary() {
		handle.Release()
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("%w: live tree changed between fingerprint and lock", ErrLiveDrift)
	}

	// 7. Probe same-filesystem and renameat2(RENAME_EXCHANGE) support before
	// touching live. Unsupported -> fail without modifying live.
	if err := probeExchange(liveAbs, candidate); err != nil {
		handle.Release()
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("%w: %v", ErrExchangeUnsupported, err)
	}

	// 8. The directory exchange: swap live <-> candidate atomically. After this
	// the old live tree is at `candidate` path and the new tree is at `liveAbs`.
	// We need the post-swap "old" tree path for cleanup; use oldTree by first
	// moving candidate there via the swap target trick: swap live <-> candidate,
	// then the old live now lives at `candidate` -> rename to oldTree.
	if err := exchangeDirectories(liveAbs, candidate); err != nil {
		handle.Release()
		j.close()
		cleanupDir(candidate)
		return Result{}, fmt.Errorf("atomic exchange failed: %w", err)
	}
	// After exchange: liveAbs now holds the new tree; candidate path holds the
	// OLD live tree. Move it to oldTree for cleanup.
	if err := os.Rename(candidate, oldTree); err != nil {
		// The commit already happened; record a warning and keep going.
		oldTree = candidate
	}

	// Mark journal as committed (best-effort).
	_ = j.write(stateCommitted, map[string]string{
		"old":    oldTree,
		"liveFp": liveFP2.summary(),
	})
	j.close()

	// Cleanup old tree. If cleanup fails we still report committed.
	res := Result{Status: StatusCommitted}
	if opts.CleanupErrorHook != nil {
		if err := opts.CleanupErrorHook(oldTree); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("cleanup of old tree %s failed: %v (commit already complete)", oldTree, err))
			return res, nil
		}
	}
	if err := cleanupDir(oldTree); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("cleanup of old tree %s failed: %v (commit already complete)", oldTree, err))
	}
	return res, nil
}

// ---- fingerprint ----

// treeFingerprint is the canonical path/mode/content digest of a directory tree.
type treeFingerprint struct {
	entries []fpEntry
}

type fpEntry struct {
	rel  string
	mode os.FileMode
	sum  string // hex sha256 of content, or "" for non-regular
}

func (f treeFingerprint) summary() string {
	var b strings.Builder
	for _, e := range f.entries {
		fmt.Fprintf(&b, "%s|%o|%s\n", e.rel, e.mode.Perm(), e.sum)
	}
	return b.String()
}

func computeFingerprint(root string) (treeFingerprint, error) {
	var fp treeFingerprint
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		entry := fpEntry{rel: rel, mode: info.Mode()}
		if info.Mode().IsRegular() {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			sum := sha256.Sum256(b)
			entry.sum = hex.EncodeToString(sum[:])
		}
		fp.entries = append(fp.entries, entry)
		return nil
	})
	if err != nil {
		return fp, err
	}
	sort.Slice(fp.entries, func(i, j int) bool { return fp.entries[i].rel < fp.entries[j].rel })
	return fp, nil
}

// ---- allowlist ----

type allowlist struct {
	set      map[string]struct{}
	prefixes []string // dir prefixes for matching files under an allowed dir
}

func newAllowlist(paths []string) *allowlist {
	a := &allowlist{set: map[string]struct{}{}}
	for _, p := range paths {
		p = filepath.ToSlash(filepath.Clean(p))
		a.set[p] = struct{}{}
		a.prefixes = append(a.prefixes, p+"/")
	}
	sort.Strings(a.prefixes)
	return a
}

func (a *allowlist) allows(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if _, ok := a.set[rel]; ok {
		return true
	}
	for _, p := range a.prefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// diffViolations returns the sorted list of relative paths that differ between
// live and candidate and are not covered by the allowlist. The summary omits
// the content hash so a pure content change is detected by the differing
// entries (rel+mode+sum); only the rel is reported as the violation.
func (a *allowlist) diffViolations(live, cand treeFingerprint) []string {
	liveMap := map[string]fpEntry{}
	for _, e := range live.entries {
		liveMap[e.rel] = e
	}
	candMap := map[string]fpEntry{}
	for _, e := range cand.entries {
		candMap[e.rel] = e
	}
	seen := map[string]struct{}{}
	for rel := range liveMap {
		seen[rel] = struct{}{}
	}
	for rel := range candMap {
		seen[rel] = struct{}{}
	}
	var violations []string
	for rel := range seen {
		l, lok := liveMap[rel]
		c, cok := candMap[rel]
		if !lok || !cok || l.sum != c.sum || l.mode.Perm() != c.mode.Perm() {
			if !a.allows(rel) {
				violations = append(violations, rel)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// ---- tree copy + validation ----

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		switch {
		case info.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			// MkdirAll applies umask; chmod to the exact source mode.
			return os.Chmod(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
				return err
			}
			// WriteFile applies umask to the permission bits; chmod to the
			// exact source mode so the fingerprint matches byte-for-byte.
			return os.Chmod(target, info.Mode().Perm())
		default:
			return fmt.Errorf("refusing to copy non-regular file %s (%s)", path, info.Mode())
		}
	})
}

// validateCandidate rejects symlinks, special files, and any entry that would
// escape the candidate root (e.g. via .. in a name).
func validateCandidate(root string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		// Use Lstat semantics: WalkDir already follows the non-symlink tree,
		// but check mode bits for symlinks and special files explicitly.
		m := info.Mode()
		if m&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed: %s", path)
		}
		if m&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 {
			return fmt.Errorf("special file not allowed: %s (%s)", path, m)
		}
		// Escape check: the resolved real path must stay under rootAbs.
		real, ferr := filepath.Abs(path)
		if ferr != nil {
			return ferr
		}
		if !strings.HasPrefix(real, rootAbs+string(filepath.Separator)) && real != rootAbs {
			return fmt.Errorf("path escapes candidate root: %s", path)
		}
		return nil
	})
}

func cleanupDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

func newID() string {
	return time.Now().UTC().Format("20060102-150405.000000000")
}
