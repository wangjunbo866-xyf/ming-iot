package thingsstartbaseline

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteBuildManifestRejectsInvalidArtifacts(t *testing.T) {
	t.Run("each required binary missing", func(t *testing.T) {
		for _, binary := range requiredTestBinaries {
			t.Run(binary, func(t *testing.T) {
				binDir := writeStaticArtifacts(t)
				if err := os.Remove(filepath.Join(binDir, binary)); err != nil {
					t.Fatalf("remove %s: %v", binary, err)
				}
				err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, filepath.Join(t.TempDir(), "build-manifest.json"))
				if err == nil || !strings.Contains(err.Error(), binary) {
					t.Fatalf("WriteBuildManifest() error = %v, want missing %s", err, binary)
				}
			})
		}
	})

	t.Run("non ELF binary", func(t *testing.T) {
		binDir := writeStaticArtifacts(t)
		if err := os.WriteFile(filepath.Join(binDir, "syssvr"), []byte("not an ELF"), 0o755); err != nil {
			t.Fatalf("write non-ELF fixture: %v", err)
		}
		err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, filepath.Join(t.TempDir(), "build-manifest.json"))
		if err == nil || !strings.Contains(err.Error(), "syssvr") {
			t.Fatalf("WriteBuildManifest() error = %v, want syssvr rejection", err)
		}
	})

	t.Run("dynamically linked ELF", func(t *testing.T) {
		binDir := writeStaticArtifacts(t)
		copyFile(t, dynamicELFFixture(t), filepath.Join(binDir, "syssvr"))
		err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, filepath.Join(t.TempDir(), "build-manifest.json"))
		if err == nil || !strings.Contains(err.Error(), "syssvr") {
			t.Fatalf("WriteBuildManifest() error = %v, want syssvr rejection", err)
		}
	})
}

func TestOpenArtifactFileRejectsSymbolicLink(t *testing.T) {
	binDir := t.TempDir()
	target := filepath.Join(binDir, "target")
	copyFile(t, staticELFFixture(t), target)
	symlink := filepath.Join(binDir, "syssvr")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symbolic link: %v", err)
	}
	file, err := openArtifactFile(symlink)
	if err == nil {
		file.Close()
		t.Fatal("openArtifactFile() succeeded for symbolic link")
	}
}

func TestWriteBuildManifestRejectsSymbolicLink(t *testing.T) {
	binDir := writeStaticArtifacts(t)
	filename := filepath.Join(binDir, "syssvr")
	if err := os.Remove(filename); err != nil {
		t.Fatalf("remove artifact fixture: %v", err)
	}
	if err := os.Symlink(staticELFFixture(t), filename); err != nil {
		t.Fatalf("create artifact symlink: %v", err)
	}
	err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, filepath.Join(t.TempDir(), "build-manifest.json"))
	if err == nil || !strings.Contains(err.Error(), "syssvr") {
		t.Fatalf("WriteBuildManifest() error = %v, want syssvr symlink rejection", err)
	}
}

func TestInspectArtifactFileRejectsUnsafeELFProperties(t *testing.T) {
	role := RoleContract{ID: "sys", Binary: "syssvr"}
	for _, tt := range []struct {
		name    string
		prepare func(t *testing.T, filename string)
		want    string
	}{
		{
			name: "symbolic link",
			prepare: func(t *testing.T, filename string) {
				target := filepath.Join(filepath.Dir(filename), "target")
				copyFile(t, staticELFFixture(t), target)
				if err := os.Symlink(target, filename); err != nil {
					t.Fatalf("create symbolic link: %v", err)
				}
			},
			want: "open binary",
		},
		{
			name: "no execute bit",
			prepare: func(t *testing.T, filename string) {
				copyFile(t, staticELFFixture(t), filename)
				if err := os.Chmod(filename, 0o644); err != nil {
					t.Fatalf("remove execute bits: %v", err)
				}
			},
			want: "executable",
		},
		{
			name: "wrong ELF type",
			prepare: func(t *testing.T, filename string) {
				copyFile(t, staticELFFixture(t), filename)
				mutateELFHeader(t, filename, func(data []byte) {
					binary.LittleEndian.PutUint16(data[16:], uint16(elf.ET_DYN))
				})
			},
			want: "ET_EXEC",
		},
		{
			name: "wrong ELF machine",
			prepare: func(t *testing.T, filename string) {
				copyFile(t, staticELFFixture(t), filename)
				mutateELFHeader(t, filename, func(data []byte) {
					binary.LittleEndian.PutUint16(data[18:], uint16(elf.EM_AARCH64))
				})
			},
			want: "EM_X86_64",
		},
		{
			name: "PT_INTERP",
			prepare: func(t *testing.T, filename string) {
				copyFile(t, dynamicELFFixture(t), filename)
				mutateELFHeader(t, filename, func(data []byte) {
					binary.LittleEndian.PutUint16(data[16:], uint16(elf.ET_EXEC))
				})
			},
			want: "PT_INTERP",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), role.Binary)
			tt.prepare(t, filename)
			file, err := openArtifactFile(filename)
			if err != nil {
				if tt.name == "symbolic link" {
					return
				}
				t.Fatalf("open artifact file: %v", err)
			}
			defer file.Close()
			_, err = inspectArtifactFile(role, file)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("inspectArtifactFile() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestInspectArtifactFileRecordsOpenedDescriptor(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "syssvr")
	copyFile(t, staticELFFixture(t), filename)
	file, err := openArtifactFile(filename)
	if err != nil {
		t.Fatalf("open artifact file: %v", err)
	}
	defer file.Close()
	artifact, err := inspectArtifactFile(RoleContract{ID: "sys", Binary: "syssvr"}, file)
	if err != nil {
		t.Fatalf("inspectArtifactFile() error = %v", err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("fstat fixture: %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	digest := sha256.Sum256(data)
	if artifact.Size != info.Size() || artifact.SHA256 != hex.EncodeToString(digest[:]) || !artifact.StaticELF {
		t.Fatalf("artifact = %+v, want descriptor size/hash/static ELF", artifact)
	}
}

func TestValidateBuildManifestRejectsEmptySHA256AndWrongArtifactCount(t *testing.T) {
	manifest := validBuildManifest()
	if err := validateBuildManifest(manifest); err != nil {
		t.Fatalf("validateBuildManifest() error = %v", err)
	}

	manifest.Artifacts[0].SHA256 = ""
	if err := validateBuildManifest(manifest); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("validateBuildManifest() error = %v, want empty SHA-256 rejection", err)
	}

	manifest = validBuildManifest()
	manifest.Artifacts = manifest.Artifacts[:4]
	if err := validateBuildManifest(manifest); err == nil || !strings.Contains(err.Error(), "exactly 5") {
		t.Fatalf("validateBuildManifest() error = %v, want artifact count rejection", err)
	}

	manifest = validBuildManifest()
	manifest.Artifacts[0].Role = "api"
	if err := validateBuildManifest(manifest); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("validateBuildManifest() error = %v, want role mapping rejection", err)
	}

	manifest = validBuildManifest()
	manifest.Artifacts[0].SHA256 = strings.Repeat("g", 64)
	if err := validateBuildManifest(manifest); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("validateBuildManifest() error = %v, want malformed SHA-256 rejection", err)
	}

	manifest = validBuildManifest()
	manifest.Artifacts[0].Size = 0
	if err := validateBuildManifest(manifest); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("validateBuildManifest() error = %v, want zero size rejection", err)
	}
}

func TestWriteBuildManifestRecordsExactArtifactsAndReplacesExistingManifest(t *testing.T) {
	binDir := writeStaticArtifacts(t)
	output := filepath.Join(t.TempDir(), "build-manifest.json")
	if err := os.WriteFile(output, []byte("stale manifest"), 0o600); err != nil {
		t.Fatalf("write stale manifest: %v", err)
	}
	if err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, output); err != nil {
		t.Fatalf("WriteBuildManifest() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(data), "stale manifest") {
		t.Fatal("manifest did not replace previous output")
	}
	var manifest BuildManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	wantRevision, err := exec.Command("git", "-C", projectRoot(t), "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("read expected revision: %v", err)
	}
	if manifest.GitRevision != strings.TrimSpace(string(wantRevision)) || len(manifest.Artifacts) != len(baselineTestArtifacts) {
		t.Fatalf("manifest revision/artifact count = %q/%d", manifest.GitRevision, len(manifest.Artifacts))
	}
	for index, want := range baselineTestArtifacts {
		got := manifest.Artifacts[index]
		data, err := os.ReadFile(filepath.Join(binDir, want.Binary))
		if err != nil {
			t.Fatalf("read %s: %v", want.Binary, err)
		}
		info, err := os.Stat(filepath.Join(binDir, want.Binary))
		if err != nil {
			t.Fatalf("stat %s: %v", want.Binary, err)
		}
		digest := sha256.Sum256(data)
		if got.Role != want.Role || got.Binary != want.Binary || got.SHA256 != hex.EncodeToString(digest[:]) || got.Size != info.Size() || !got.StaticELF {
			t.Fatalf("artifact[%d] = %+v, want role/binary/hash/size/static ELF", index, got)
		}
	}
}

func TestWriteBuildManifestCleansTemporaryFileWhenRenameFails(t *testing.T) {
	binDir := writeStaticArtifacts(t)
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "manifest-target")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatalf("create rename target directory: %v", err)
	}
	if err := WriteBuildManifest(baselineContractPath(t), projectRoot(t), binDir, output); err == nil {
		t.Fatal("WriteBuildManifest() succeeded with directory output target")
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("read output directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-target.tmp-") {
			t.Fatalf("temporary manifest %q was not cleaned up", entry.Name())
		}
	}
}

func TestBaselineMakefilesDefineStaticBinaryTargets(t *testing.T) {
	const baselineBuild = "BASELINE_GO_BUILD := env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags no_k8s -trimpath"
	for _, tt := range []struct {
		name     string
		filename string
		want     []string
	}{
		{
			name:     "saas",
			filename: filepath.Join(projectRoot(t), "backend/saas/Makefile"),
			want: []string{
				"GO ?= go",
				baselineBuild,
				"build.baseline: build.baseline.sys build.baseline.timedjob",
				"build.baseline.sys: | $(BIN_DIR)\n\trm -f $(BIN_DIR)/syssvr\n\tumask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/syssvr ./service/syssvr",
				"build.baseline.timedjob: | $(BIN_DIR)\n\trm -f $(BIN_DIR)/timedjobsvr\n\tumask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/timedjobsvr ./service/timed/timedjobsvr",
			},
		},
		{
			name:     "things",
			filename: filepath.Join(projectRoot(t), "backend/things/Makefile"),
			want: []string{
				baselineBuild,
				"build.baseline: build.baseline.api build.baseline.dg build.baseline.dm",
				"build.baseline.api: | $(BIN_DIR)\n\trm -f $(BIN_DIR)/thingsvr\n\tumask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr",
				"build.baseline.dg: | $(BIN_DIR)\n\trm -f $(BIN_DIR)/dgsvr\n\tumask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr",
				"build.baseline.dm: | $(BIN_DIR)\n\trm -f $(BIN_DIR)/dmsvr\n\tumask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.filename)
			if err != nil {
				t.Fatalf("read Makefile: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(data), want) {
					t.Errorf("Makefile missing %q", want)
				}
			}
			if strings.Contains(string(data), "GOWORK=off") {
				t.Errorf("Makefile disables the repository workspace required by the local Share module")
			}
		})
	}
}

func TestBaselineTargetsRemoveStaleArtifactsBeforeBuildFailure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		module string
		target string
		binary string
	}{
		{name: "saas sys", module: "saas", target: "build.baseline.sys", binary: "syssvr"},
		{name: "things api", module: "things", target: "build.baseline.api", binary: "thingsvr"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			stale := filepath.Join(binDir, tt.binary)
			if err := os.WriteFile(stale, []byte("stale artifact"), 0o755); err != nil {
				t.Fatalf("write stale artifact: %v", err)
			}
			command := exec.Command(
				"make", "-C", filepath.Join(projectRoot(t), "backend", tt.module), tt.target,
				"BIN_DIR="+binDir, "GO=false",
			)
			if output, err := command.CombinedOutput(); err == nil {
				t.Fatalf("make unexpectedly succeeded: %s", output)
			}
			if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("stale artifact stat error = %v, want not exist", err)
			}
		})
	}
}

func TestRuntimeDockerfilesAreOneRoleOneBinary(t *testing.T) {
	for _, role := range requiredRoles {
		t.Run(role.ID, func(t *testing.T) {
			filename := filepath.Join(projectRoot(t), role.Dockerfile)
			data, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read Dockerfile %s: %v", role.Dockerfile, err)
			}

			want := strings.Join([]string{
				"FROM alpine:3.20",
				"RUN apk add --no-cache ca-certificates tzdata",
				"ARG VCS_REF",
				"ARG BINARY_SHA256",
				"LABEL org.opencontainers.image.revision=$VCS_REF \\",
				"      io.ming-iot.binary.sha256=$BINARY_SHA256 \\",
				"      io.ming-iot.role=" + role.ID,
				"ENV TZ=Asia/Shanghai",
				"WORKDIR /app",
				"COPY bin/" + role.Binary + " /app/" + role.Binary,
				"COPY etc /app/etc",
				"RUN mkdir -p /app/oss && chmod 0755 /app/" + role.Binary,
				"ENTRYPOINT [\"/app/" + role.Binary + "\"]",
			}, "\n") + "\n"
			if got := string(data); got != want {
				t.Errorf("Dockerfile %s is not the required one-role/one-binary runtime image:\nwant:\n%s\ngot:\n%s", role.Dockerfile, want, got)
			}
		})
	}
}

var baselineTestArtifacts = []Artifact{
	{Role: "sys", Binary: "syssvr"},
	{Role: "timedjob", Binary: "timedjobsvr"},
	{Role: "dm", Binary: "dmsvr"},
	{Role: "dg", Binary: "dgsvr"},
	{Role: "api", Binary: "thingsvr"},
}

var requiredTestBinaries = []string{"syssvr", "timedjobsvr", "dmsvr", "dgsvr", "thingsvr"}

func validBuildManifest() BuildManifest {
	artifacts := make([]Artifact, 0, len(requiredTestBinaries))
	for _, artifact := range baselineTestArtifacts {
		artifacts = append(artifacts, Artifact{
			Role:      artifact.Role,
			Binary:    artifact.Binary,
			SHA256:    strings.Repeat("a", 64),
			Size:      1,
			StaticELF: true,
		})
	}
	return BuildManifest{SchemaVersion: 1, GitRevision: "revision", Artifacts: artifacts}
}

func writeStaticArtifacts(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	source := staticELFFixture(t)
	for _, binary := range requiredTestBinaries {
		copyFile(t, source, filepath.Join(binDir, binary))
	}
	return binDir
}

func staticELFFixture(t *testing.T) string {
	t.Helper()
	filename, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	file, err := elf.Open(filename)
	if err != nil {
		t.Skipf("test executable is not ELF: %v", err)
	}
	defer file.Close()
	libraries, err := file.ImportedLibraries()
	if err != nil {
		t.Fatalf("inspect test executable imports: %v", err)
	}
	if len(libraries) != 0 {
		t.Skipf("test executable is not statically linked: %v", libraries)
	}
	return filename
}

func dynamicELFFixture(t *testing.T) string {
	t.Helper()
	file, err := elf.Open("/bin/sh")
	if err != nil {
		t.Skipf("/bin/sh is not ELF: %v", err)
	}
	defer file.Close()
	libraries, err := file.ImportedLibraries()
	if err != nil {
		t.Fatalf("inspect /bin/sh imports: %v", err)
	}
	if len(libraries) == 0 {
		t.Fatalf("/bin/sh is not dynamically linked")
	}
	return "/bin/sh"
}

func baselineContractPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../../deploy/profiles/things-baseline/contract.yaml"))
}

func projectRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(baselineContractPath(t)))))
}

func copyFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read fixture %s: %v", source, err)
	}
	if err := os.WriteFile(destination, data, 0o755); err != nil {
		t.Fatalf("write fixture %s: %v", destination, err)
	}
}

func mutateELFHeader(t *testing.T, filename string, mutate func([]byte)) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read ELF fixture: %v", err)
	}
	if len(data) < 20 || string(data[:4]) != "\x7fELF" {
		t.Fatal("fixture is not a complete ELF header")
	}
	mutate(data)
	if err := os.WriteFile(filename, data, 0o755); err != nil {
		t.Fatalf("write ELF fixture: %v", err)
	}
}
