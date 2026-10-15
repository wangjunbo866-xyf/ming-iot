package repositorycontract

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentContractScansFailClosed(t *testing.T) {
	root := findRepositoryRoot(t)
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		first    int
		second   int
		missing  bool
		wantPass bool
		calls    int
	}{
		{name: "both scans have no matches", first: 1, second: 1, wantPass: true, calls: 2},
		{name: "first scan matches", first: 0, second: 1, calls: 1},
		{name: "first scan errors", first: 2, second: 1, calls: 1},
		{name: "first scan cannot execute", first: 127, second: 1, calls: 1},
		{name: "second scan matches", first: 1, second: 0, calls: 2},
		{name: "second scan errors", first: 1, second: 2, calls: 2},
		{name: "second scan cannot execute", first: 1, second: 127, calls: 2},
		{name: "scanner absent from PATH", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			// Exercise the real Make target, with only its external contract and
			// Compose commands stubbed. A bounded PATH makes missing rg deterministic.
			for _, name := range []string{"bash", "mktemp", "cp", "chmod", "rm"} {
				path, err := exec.LookPath(name)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(bin, "scan-calls")
			if !tc.missing {
				script := fmt.Sprintf("#!/bin/sh\nif [ -f \"$SCAN_CALL_LOG\" ]; then status=%d; else status=%d; fi\nprintf 'scan\\n' >> \"$SCAN_CALL_LOG\"\nexit \"$status\"\n", tc.second, tc.first)
				if err := os.WriteFile(filepath.Join(bin, "rg"), []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), verificationCommandTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, makePath, "-C", filepath.Join(root, "deploy"), "things-baseline-contract", "BASELINE=true")
			cmd.Env = append(os.Environ(), "PATH="+bin, "SCAN_CALL_LOG="+log)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("make timed out: %v\n%s", ctx.Err(), output)
			}
			if (err == nil) != tc.wantPass {
				t.Errorf("make error = %v, want pass=%t\n%s", err, tc.wantPass, output)
			}
			calls, readErr := os.ReadFile(log)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if got := strings.Count(string(calls), "scan\n"); got != tc.calls {
				t.Errorf("scan calls = %d, want %d\n%s", got, tc.calls, output)
			}
		})
	}
}
