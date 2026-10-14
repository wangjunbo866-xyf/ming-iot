package thingsstartbaseline

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Artifact describes one verified baseline executable.
type Artifact struct {
	Role      string `json:"role"`
	Binary    string `json:"binary"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	StaticELF bool   `json:"staticElf"`
}

// BuildManifest records the source revision and baseline executables.
type BuildManifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	GitRevision   string     `json:"gitRevision"`
	Artifacts     []Artifact `json:"artifacts"`
}

// WriteBuildManifest verifies the contract binaries and atomically writes their manifest.
func WriteBuildManifest(contractPath, repo, binDir, output string) error {
	contract, err := LoadContract(contractPath)
	if err != nil {
		return err
	}
	revision, err := gitRevision(repo)
	if err != nil {
		return err
	}
	artifacts := make([]Artifact, 0, len(contract.Roles))
	for _, role := range contract.Roles {
		artifact, err := inspectArtifact(role, filepath.Join(binDir, role.Binary))
		if err != nil {
			return err
		}
		artifacts = append(artifacts, artifact)
	}
	manifest := BuildManifest{SchemaVersion: 1, GitRevision: revision, Artifacts: artifacts}
	if err := validateBuildManifest(manifest); err != nil {
		return err
	}
	return writeManifestAtomically(output, manifest)
}

func gitRevision(repo string) (string, error) {
	output, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("read Git revision for %q: %w", repo, err)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", fmt.Errorf("read Git revision for %q: empty revision", repo)
	}
	return revision, nil
}

func openArtifactFile(filename string) (*os.File, error) {
	fd, err := syscall.Open(filename, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filename)
	if file == nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("create file from descriptor")
	}
	return file, nil
}

func inspectArtifact(role RoleContract, filename string) (Artifact, error) {
	file, err := openArtifactFile(filename)
	if err != nil {
		return Artifact{}, fmt.Errorf("open binary %q: %w", role.Binary, err)
	}
	defer file.Close()
	return inspectArtifactFile(role, file)
}

func inspectArtifactFile(role RoleContract, file *os.File) (Artifact, error) {
	info, err := file.Stat()
	if err != nil {
		return Artifact{}, fmt.Errorf("fstat binary %q: %w", role.Binary, err)
	}
	if !info.Mode().IsRegular() {
		return Artifact{}, fmt.Errorf("binary %q must be a regular file", role.Binary)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return Artifact{}, fmt.Errorf("binary %q must be executable", role.Binary)
	}
	elfFile, err := elf.NewFile(file)
	if err != nil {
		return Artifact{}, fmt.Errorf("open binary %q as ELF: %w", role.Binary, err)
	}
	defer elfFile.Close()
	if elfFile.Type != elf.ET_EXEC {
		return Artifact{}, fmt.Errorf("binary %q ELF type must be ET_EXEC", role.Binary)
	}
	if elfFile.Machine != elf.EM_X86_64 {
		return Artifact{}, fmt.Errorf("binary %q ELF machine must be EM_X86_64", role.Binary)
	}
	for _, program := range elfFile.Progs {
		if program.Type == elf.PT_INTERP {
			return Artifact{}, fmt.Errorf("binary %q must not contain PT_INTERP", role.Binary)
		}
	}
	libraries, err := elfFile.ImportedLibraries()
	if err != nil {
		return Artifact{}, fmt.Errorf("inspect binary %q ELF imports: %w", role.Binary, err)
	}
	if len(libraries) != 0 {
		return Artifact{}, fmt.Errorf("binary %q must be a static ELF", role.Binary)
	}
	digest, err := sha256OpenFile(file)
	if err != nil {
		return Artifact{}, fmt.Errorf("hash binary %q: %w", role.Binary, err)
	}
	return Artifact{
		Role:      role.ID,
		Binary:    role.Binary,
		SHA256:    digest,
		Size:      info.Size(),
		StaticELF: true,
	}, nil
}

func sha256OpenFile(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

var requiredManifestArtifacts = []struct {
	role   string
	binary string
}{
	{role: "sys", binary: "syssvr"},
	{role: "timedjob", binary: "timedjobsvr"},
	{role: "dm", binary: "dmsvr"},
	{role: "dg", binary: "dgsvr"},
	{role: "api", binary: "thingsvr"},
}

func validateBuildManifest(manifest BuildManifest) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("schema version must be 1")
	}
	if manifest.GitRevision == "" {
		return fmt.Errorf("Git revision is empty")
	}
	if len(manifest.Artifacts) != len(requiredManifestArtifacts) {
		return fmt.Errorf("manifest must contain exactly 5 artifacts")
	}
	for index, artifact := range manifest.Artifacts {
		required := requiredManifestArtifacts[index]
		if artifact.Role != required.role {
			return fmt.Errorf("artifact %d role must be %s", index, required.role)
		}
		if artifact.Binary != required.binary {
			return fmt.Errorf("artifact %d binary must be %s", index, required.binary)
		}
		if len(artifact.SHA256) != sha256.Size*2 {
			return fmt.Errorf("artifact %d SHA-256 must contain 64 hexadecimal characters", index)
		}
		if _, err := hex.DecodeString(artifact.SHA256); err != nil {
			return fmt.Errorf("artifact %d SHA-256 must be hexadecimal: %w", index, err)
		}
		if artifact.Size <= 0 {
			return fmt.Errorf("artifact %d size must be positive", index)
		}
		if !artifact.StaticELF {
			return fmt.Errorf("artifact %d is not a static ELF", index)
		}
	}
	return nil
}

func writeManifestAtomically(output string, manifest BuildManifest) error {
	outputDir := filepath.Dir(output)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create manifest directory %q: %w", outputDir, err)
	}
	temporary, err := os.CreateTemp(outputDir, "."+filepath.Base(output)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create manifest temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(manifest); err != nil {
		temporary.Close()
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	if err := os.Rename(temporaryName, output); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	if err := syncManifestDirectory(outputDir); err != nil {
		return err
	}
	return nil
}

func syncManifestDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open manifest directory: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync manifest directory: %w", err)
	}
	return nil
}
