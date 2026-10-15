package rpcgen

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GoctlAttestation records the exact generator binary and source/toolchain
// identity declared by backend/tools/goctl.lock.
type GoctlAttestation struct {
	BinaryPath   string
	BinarySHA256 string
	Repository   string
	Commit       string
	GoVersion    string
	GOOS         string
	GOARCH       string
	CGOEnabled   bool
	VersionText  string
}

type goctlLock struct {
	Repository          string           `yaml:"repository"`
	Commit              string           `yaml:"commit"`
	GoVersion           string           `yaml:"goVersion"`
	Target              goctlLockTarget  `yaml:"target"`
	Patches             []goctlLockPatch `yaml:"patches"`
	SourceArchiveSHA256 string           `yaml:"sourceArchiveSha256"`
	BinarySHA256        string           `yaml:"binarySha256"`
}

type goctlLockTarget struct {
	GOOS       string `yaml:"goos"`
	GOARCH     string `yaml:"goarch"`
	CGOEnabled *bool  `yaml:"cgoEnabled"`
}

type goctlLockPatch struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// AttestGoctl validates and executes only backend/bin/goctl under backendRoot.
// It deliberately never resolves goctl through PATH.
func AttestGoctl(backendRoot string) (GoctlAttestation, error) {
	var result GoctlAttestation
	root, err := filepath.Abs(backendRoot)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: resolve backend root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: resolve backend root %s: %w", root, err)
	}

	lockPath := filepath.Join(root, "tools", "goctl.lock")
	realLock, err := filepath.EvalSymlinks(lockPath)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: resolve tools/goctl.lock: %w", err)
	}
	if !pathWithin(realRoot, realLock) {
		return result, fmt.Errorf("goctl attestation: tools/goctl.lock escapes backend root")
	}
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: read tools/goctl.lock: %w", err)
	}
	lock, err := decodeGoctlLock(lockData)
	if err != nil {
		return result, err
	}
	if err := validateGoctlLock(lock); err != nil {
		return result, err
	}

	binaryPath := filepath.Join(root, "bin", "goctl")
	info, err := os.Stat(binaryPath)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: read bin/goctl: %w", err)
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("goctl attestation: bin/goctl is not a regular file")
	}
	if info.Mode().Perm()&0111 == 0 {
		return result, fmt.Errorf("goctl attestation: bin/goctl is not executable")
	}
	realBinary, err := filepath.EvalSymlinks(binaryPath)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: resolve bin/goctl: %w", err)
	}
	if !pathWithin(realRoot, realBinary) {
		return result, fmt.Errorf("goctl attestation: bin/goctl escapes backend root")
	}

	binaryData, err := os.ReadFile(binaryPath)
	if err != nil {
		return result, fmt.Errorf("goctl attestation: read bin/goctl: %w", err)
	}
	actualDigest := fmt.Sprintf("%x", sha256.Sum256(binaryData))
	if !strings.EqualFold(actualDigest, lock.BinarySHA256) {
		return result, fmt.Errorf("goctl attestation: binarySha256 mismatch: lock=%s actual=%s", lock.BinarySHA256, actualDigest)
	}

	command := exec.Command(binaryPath, "--version")
	versionOutput, err := command.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("goctl attestation: execute bin/goctl --version: %w: %s", err, strings.TrimSpace(string(versionOutput)))
	}
	result = GoctlAttestation{
		BinaryPath:   binaryPath,
		BinarySHA256: actualDigest,
		Repository:   lock.Repository,
		Commit:       lock.Commit,
		GoVersion:    lock.GoVersion,
		GOOS:         lock.Target.GOOS,
		GOARCH:       lock.Target.GOARCH,
		CGOEnabled:   *lock.Target.CGOEnabled,
		VersionText:  strings.TrimSpace(string(versionOutput)),
	}
	return result, nil
}

func decodeGoctlLock(data []byte) (goctlLock, error) {
	var lock goctlLock
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&lock); err != nil {
		return lock, fmt.Errorf("goctl attestation: decode tools/goctl.lock: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return lock, fmt.Errorf("goctl attestation: decode trailing lock document: %w", err)
		}
		return lock, fmt.Errorf("goctl attestation: tools/goctl.lock contains multiple YAML documents")
	}
	return lock, nil
}

func validateGoctlLock(lock goctlLock) error {
	required := []struct {
		name  string
		value string
	}{
		{"repository", lock.Repository},
		{"commit", lock.Commit},
		{"goVersion", lock.GoVersion},
		{"target.goos", lock.Target.GOOS},
		{"target.goarch", lock.Target.GOARCH},
		{"binarySha256", lock.BinarySHA256},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("goctl attestation: %s is required", field.name)
		}
	}
	if lock.Target.CGOEnabled == nil {
		return fmt.Errorf("goctl attestation: target.cgoEnabled is required")
	}
	if len(lock.BinarySHA256) != sha256.Size*2 {
		return fmt.Errorf("goctl attestation: binarySha256 must be a SHA-256 digest")
	}
	return nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
