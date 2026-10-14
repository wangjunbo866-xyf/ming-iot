package rpcgen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttestGoctlUsesPinnedBinaryAndIgnoresPATH(t *testing.T) {
	backend := t.TempDir()
	binary := writeGoctlFixture(t, backend, "#!/bin/sh\necho pinned-goctl\n")
	writeGoctlLockFixture(t, backend, fileSHA256(t, binary), "")

	fakeBin := t.TempDir()
	writeExecutableFixture(t, filepath.Join(fakeBin, "goctl"), "#!/bin/sh\necho PATH-goctl\nexit 99\n")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := AttestGoctl(backend)
	if err != nil {
		t.Fatalf("AttestGoctl() error = %v", err)
	}
	wantPath, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	if got.BinaryPath != wantPath {
		t.Fatalf("BinaryPath = %q, want %q", got.BinaryPath, wantPath)
	}
	if got.VersionText != "pinned-goctl" {
		t.Fatalf("VersionText = %q, want pinned-goctl", got.VersionText)
	}
	if got.Repository != "https://example.com/unitedrhino/go-zero.git" || got.Commit != strings.Repeat("a", 40) {
		t.Fatalf("identity = %#v", got)
	}
	if got.GoVersion != "go1.24.4" || got.GOOS != "linux" || got.GOARCH != "amd64" || got.CGOEnabled {
		t.Fatalf("toolchain target = %#v", got)
	}
}

func TestAttestGoctlRejectsDigestDriftMissingFieldsAndUnsafeBinary(t *testing.T) {
	t.Run("digest drift", func(t *testing.T) {
		backend := t.TempDir()
		writeGoctlFixture(t, backend, "#!/bin/sh\necho pinned-goctl\n")
		writeGoctlLockFixture(t, backend, strings.Repeat("0", 64), "")
		_, err := AttestGoctl(backend)
		if err == nil || !strings.Contains(err.Error(), "binarySha256") {
			t.Fatalf("error = %v, want binarySha256", err)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		backend := t.TempDir()
		writeGoctlLockFixture(t, backend, strings.Repeat("0", 64), "")
		_, err := AttestGoctl(backend)
		if err == nil || !strings.Contains(err.Error(), "bin/goctl") {
			t.Fatalf("error = %v, want bin/goctl", err)
		}
	})

	for _, field := range []string{"repository", "commit", "goVersion", "target", "target.goos", "target.goarch", "target.cgoEnabled", "binarySha256"} {
		field := field
		t.Run("missing "+field, func(t *testing.T) {
			backend := t.TempDir()
			binary := writeGoctlFixture(t, backend, "#!/bin/sh\necho pinned-goctl\n")
			writeGoctlLockFixture(t, backend, fileSHA256(t, binary), field)
			_, err := AttestGoctl(backend)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("error = %v, want %s", err, field)
			}
		})
	}

	t.Run("binary symlink escapes backend", func(t *testing.T) {
		backend := t.TempDir()
		outside := filepath.Join(t.TempDir(), "goctl")
		writeExecutableFixture(t, outside, "#!/bin/sh\necho outside\n")
		if err := os.MkdirAll(filepath.Join(backend, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(backend, "bin", "goctl")); err != nil {
			t.Fatal(err)
		}
		writeGoctlLockFixture(t, backend, fileSHA256(t, outside), "")
		_, err := AttestGoctl(backend)
		if err == nil || !strings.Contains(err.Error(), "escapes backend root") {
			t.Fatalf("error = %v, want escape rejection", err)
		}
	})

	t.Run("lock symlink escapes backend", func(t *testing.T) {
		backend := t.TempDir()
		binary := writeGoctlFixture(t, backend, "#!/bin/sh\necho pinned-goctl\n")
		outside := t.TempDir()
		writeGoctlLockFixture(t, outside, fileSHA256(t, binary), "")
		if err := os.MkdirAll(filepath.Join(backend, "tools"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, "tools", "goctl.lock"), filepath.Join(backend, "tools", "goctl.lock")); err != nil {
			t.Fatal(err)
		}
		_, err := AttestGoctl(backend)
		if err == nil || !strings.Contains(err.Error(), "goctl.lock escapes backend root") {
			t.Fatalf("error = %v, want lock escape rejection", err)
		}
	})
}

func writeGoctlFixture(t *testing.T, backend, body string) string {
	t.Helper()
	path := filepath.Join(backend, "bin", "goctl")
	writeExecutableFixture(t, path, body)
	return path
}

func writeExecutableFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
}

func writeGoctlLockFixture(t *testing.T, backend, digest, omit string) {
	t.Helper()
	values := map[string]string{
		"repository":        "repository: https://example.com/unitedrhino/go-zero.git\n",
		"commit":            "commit: " + strings.Repeat("a", 40) + "\n",
		"goVersion":         "goVersion: go1.24.4\n",
		"target":            "target:\n",
		"target.goos":       "  goos: linux\n",
		"target.goarch":     "  goarch: amd64\n",
		"target.cgoEnabled": "  cgoEnabled: false\n",
		"binarySha256":      "binarySha256: " + digest + "\n",
	}
	var b strings.Builder
	for _, key := range []string{"repository", "commit", "goVersion", "target", "target.goos", "target.goarch", "target.cgoEnabled", "binarySha256"} {
		if key != omit && !(omit == "target" && strings.HasPrefix(key, "target.")) {
			b.WriteString(values[key])
		}
	}
	path := filepath.Join(backend, "tools", "goctl.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
