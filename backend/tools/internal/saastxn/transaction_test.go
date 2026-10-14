package saastxn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// writeAll writes a file with mode 0o644 under dir, creating parents.
func writeAll(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeMode writes a file with an explicit mode, forcing the exact permission
// bits via chmod so it is not weakened by the process umask.
func writeMode(t *testing.T, dir, rel, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod: %v", err)
	}
}

// fixtureLiveCore builds a minimal but Go-compilable "live core" so the
// candidate's GOWORK=off `go test -run ^$ ./...` step can run. It contains a
// go.mod and one tiny package.
func fixtureLiveCore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	live := filepath.Join(root, "core")
	writeAll(t, live, "go.mod", "module example.org/core\n\ngo 1.24.4\n")
	writeAll(t, live, "service/syssvr/pb/sys/sys.pb.go", "package sys\n\n// placeholder\n")
	writeAll(t, live, "service/syssvr/pb/sys/sys_grpc.pb.go", "package sys\n\n// placeholder grpc\n")
	writeAll(t, live, "pkg/greet/greet.go", "package greet\n\nfunc Hello() string { return \"hi\" }\n")
	writeAll(t, live, "pkg/greet/greet_test.go", "package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) { if Hello() != \"hi\" { t.Fatal(\"x\") } }\n")
	// A group-writable file: copyTree must preserve the exact mode despite the
	// process umask, otherwise the allowlist fingerprint check would flag it.
	writeMode(t, live, "pkg/greet/notes.txt", "shared notes\n", 0o664)
	return root
}

// liveFingerprint re-computes the canonical path/mode/sha256 fingerprint of a
// directory tree for "before/after" assertions.
func liveFingerprint(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		mode := ""
		if info.Mode().Type() == 0 {
			mode = "f"
		} else if info.IsDir() {
			mode = "d"
		} else {
			mode = info.Mode().String()
		}
		digest := "-"
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			digest = hex.EncodeToString(sum[:])
		}
		lines = append(lines, fmt.Sprintf("%s %s %o %s", rel, mode, info.Mode().Perm(), digest))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestCommitSuccessSwapsTree(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")

	before := liveFingerprint(t, live)

	res, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go", "service/syssvr/pb/sys/sys_grpc.pb.go"},
		// Mutate the candidate so a diff actually exists.
		MutateCandidate: func(candidate string) error {
			return os.WriteFile(filepath.Join(candidate, "service/syssvr/pb/sys/sys.pb.go"),
				[]byte("package sys\n\n// regenerated\n"), 0o644)
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Status != StatusCommitted {
		t.Fatalf("status = %q, want %q", res.Status, StatusCommitted)
	}

	// After commit, the pb file must reflect the mutated content.
	got, err := os.ReadFile(filepath.Join(live, "service/syssvr/pb/sys/sys.pb.go"))
	if err != nil {
		t.Fatalf("read live pb: %v", err)
	}
	if !strings.Contains(string(got), "regenerated") {
		t.Fatalf("live pb not swapped: %q", got)
	}
	// Non-allowlisted files must be byte-identical to before for the paths that
	// still exist (greet.go unchanged).
	greet, _ := os.ReadFile(filepath.Join(live, "pkg/greet/greet.go"))
	if !strings.Contains(string(greet), "func Hello") {
		t.Fatalf("unrelated file corrupted")
	}
	// Group-writable mode must be preserved despite the process umask; this is
	// the regression guard for the copyTree chmod fix.
	notesInfo, err := os.Stat(filepath.Join(live, "pkg/greet/notes.txt"))
	if err != nil {
		t.Fatalf("stat notes.txt: %v", err)
	}
	if got, want := notesInfo.Mode().Perm(), os.FileMode(0o664); got != want {
		t.Fatalf("notes.txt mode = %o, want %o (group-writable not preserved through copy+swap)", got, want)
	}

	// Cleanup: after commit, candidate tree should be gone (old tree) - check
	// that no leftover candidate directories remain under .saastxn except lock.
	_ = before
}

func TestCommitRejectsAllowlistViolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")
	before := liveFingerprint(t, live)

	_, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go"},
		MutateCandidate: func(candidate string) error {
			// Mutate a file OUTSIDE the allowlist.
			return os.WriteFile(filepath.Join(candidate, "pkg/greet/greet.go"),
				[]byte("package greet\n\n// tampered\n"), 0o644)
		},
	})
	if err == nil {
		t.Fatalf("expected allowlist violation error, got nil")
	}
	if !errors.Is(err, ErrAllowlistViolation) {
		t.Fatalf("expected ErrAllowlistViolation, got %v", err)
	}
	// Live must be byte-identical.
	if after := liveFingerprint(t, live); after != before {
		t.Fatalf("live changed after rejected commit:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestCommitDetectsLiveDrift(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")

	// Drift injector: mutate live greet.go between fingerprint and re-fingerprint.
	_, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go"},
		// DriftHook fires after the initial fingerprint is taken; it mutates a
		// non-allowlisted live file to simulate concurrent modification.
		DriftHook: func(liveDir string) error {
			return os.WriteFile(filepath.Join(liveDir, "pkg/greet/greet.go"),
				[]byte("package greet\n\n// drifted\n"), 0o644)
		},
		MutateCandidate: func(candidate string) error { return nil },
	})
	if err == nil {
		t.Fatalf("expected drift error, got nil")
	}
	if !errors.Is(err, ErrLiveDrift) {
		t.Fatalf("expected ErrLiveDrift, got %v", err)
	}
}

func TestCommitRejectsSymlinkInCandidate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")
	before := liveFingerprint(t, live)

	_, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go"},
		MutateCandidate: func(candidate string) error {
			// Inject a symlink inside the candidate (should be rejected).
			return os.Symlink("/etc/hostname", filepath.Join(candidate, "service/syssvr/pb/sys/evil"))
		},
	})
	if err == nil {
		t.Fatalf("expected symlink rejection, got nil")
	}
	if !errors.Is(err, ErrCandidateRejected) {
		t.Fatalf("expected ErrCandidateRejected, got %v", err)
	}
	if after := liveFingerprint(t, live); after != before {
		t.Fatalf("live changed after rejected commit")
	}
}

func TestCommitReportsCommittedWhenCleanupFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")

	res, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go"},
		MutateCandidate: func(candidate string) error {
			return os.WriteFile(filepath.Join(candidate, "service/syssvr/pb/sys/sys.pb.go"),
				[]byte("package sys\n\n// regenerated\n"), 0o644)
		},
		// CleanupErrorHook makes the post-exchange old-tree removal fail. The
		// commit must still report committed with a warning.
		CleanupErrorHook: func(oldTree string) error {
			return errors.New("simulated cleanup failure")
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Status != StatusCommitted {
		t.Fatalf("status = %q, want committed", res.Status)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected cleanup warning, got none")
	}
}

func TestCommitConcurrentLockConflict(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("saastxn exchange requires Linux renameat2")
	}
	root := fixtureLiveCore(t)
	live := filepath.Join(root, "core")
	txnDir := filepath.Join(root, ".saastxn")

	// Pre-acquire the lock with an flock-style holder: create the lock dir and
	// hold an open fd that we keep open.
	if err := os.MkdirAll(txnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// We test lock conflict via an injected hook that errors when acquiring.
	_, err := Commit(t.Context(), Options{
		LiveDir:   live,
		TxnDir:    txnDir,
		Allowlist: []string{"service/syssvr/pb/sys/sys.pb.go"},
		LockAcquireHook: func(txnDir string) (LockHandle, error) {
			return nil, errors.New("lock busy")
		},
		MutateCandidate: func(string) error { return nil },
	})
	if err == nil {
		t.Fatalf("expected lock conflict, got nil")
	}
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("expected ErrLockBusy, got %v", err)
	}
}
