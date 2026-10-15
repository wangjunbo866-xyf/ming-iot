package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"ming-iot/backend/tools/internal/rpcgen"
)

func TestCLIRequiresExactlyOneModuleRoot(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "neither", args: []string{"--manifest", "rpcgen.yaml"}},
		{name: "both", args: []string{"--manifest", "rpcgen.yaml", "--module-root", "backend/things", "--saas", "backend/saas"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseCLIOptions("check", tc.args)
			if err == nil {
				err = validateCLIOptions(&rpcgen.Manifest{Version: 2}, opts, "")
			}
			if err == nil || !strings.Contains(err.Error(), "exactly one of --module-root and --saas") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCLIRejectsConflictingRootsBeforeReadingManifest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--manifest", filepath.Join(t.TempDir(), "missing.yaml"),
		"--module-root", "backend/things", "--saas", "backend/saas",
	}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "exactly one of --module-root and --saas") {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
}

func TestCLISubcommandHelpReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"check", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rpcgen check") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestCLIGoctlOverridePolicy(t *testing.T) {
	pinned := filepath.Clean("/workspace/backend/bin/goctl")

	v2, err := parseCLIOptions("generate", []string{
		"--manifest", "rpcgen.yaml", "--module-root", "backend/things", "--goctl", pinned,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCLIOptions(&rpcgen.Manifest{Version: 2}, v2, pinned); err == nil || !strings.Contains(err.Error(), "v2 forbids --goctl") {
		t.Fatalf("v2 error = %v", err)
	}

	v1, err := parseCLIOptions("generate", []string{
		"--manifest", "rpcgen.yaml", "--saas", "backend/saas", "--goctl", pinned,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCLIOptions(&rpcgen.Manifest{Version: 1}, v1, pinned); err != nil {
		t.Fatalf("v1 pinned override rejected: %v", err)
	}
	v1.goctl = filepath.Join(filepath.Dir(pinned), "other-goctl")
	if err := validateCLIOptions(&rpcgen.Manifest{Version: 1}, v1, pinned); err == nil || !strings.Contains(err.Error(), "pinned goctl") {
		t.Fatalf("v1 mismatch error = %v", err)
	}
}

func TestCLIDerivesBackendAndToolPaths(t *testing.T) {
	opts, err := parseCLIOptions("generate", []string{
		"--manifest", "rpcgen.yaml", "--module-root", "/workspace/backend/things",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := opts.backendRoot(), "/workspace/backend"; got != want {
		t.Fatalf("backendRoot() = %q, want %q", got, want)
	}
	if got, want := opts.protobufToolsPath(), "/workspace/backend/bin/protobuf-tools"; got != want {
		t.Fatalf("protobufToolsPath() = %q, want %q", got, want)
	}
}
