package thingsbaseline

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoToolAttestationRejectsNonGoExecutable(t *testing.T) {
	tool, err := openAttestedGoTool(context.Background(), "/usr/bin/dash")
	if err == nil || tool != nil || !strings.Contains(err.Error(), "build info") {
		t.Fatalf("openAttestedGoTool(/usr/bin/dash) = %+v, %v", tool, err)
	}
}

func TestGoToolAttestationAcceptsAndRevalidatesLockedCmdGo(t *testing.T) {
	path := filepath.Join(runtime.GOROOT(), "bin", "go")
	tool, err := openAttestedGoTool(context.Background(), path)
	if err != nil {
		t.Fatalf("openAttestedGoTool(cmd/go): %v", err)
	}
	defer tool.Close()
	if tool.attestedVersion() != lockedGoVersion || tool.buildPath != "cmd/go" || len(tool.sha256) != 64 || tool.inheritedFile() == nil {
		t.Fatalf("Go tool attestation = %+v", tool)
	}
	if err := tool.revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate attested Go tool: %v", err)
	}
}
