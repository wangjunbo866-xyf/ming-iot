package thingsbaseline

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSecurityScannerAcceptsGoldenTreeAndDigestIgnoresRootAndMtime(t *testing.T) {
	rootOne, policyOne := scannerFixture(t)
	reportOne, manifestOne, credentialsOne, err := scanTree(rootOne, policyOne)
	if err != nil {
		t.Fatalf("scanTree(first): %v", err)
	}
	if credentialsOne != (CredentialProfileReport{}) || reportOne.RegularFiles != len(policyOne.expectedFiles) {
		t.Fatalf("first scan report=%+v credentials=%+v", reportOne, credentialsOne)
	}

	rootTwo, policyTwo := scannerFixture(t)
	old := time.Unix(946684800, 0)
	if err := filepath.Walk(rootTwo, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		t.Fatal(err)
	}
	_, manifestTwo, _, err := scanTree(rootTwo, policyTwo)
	if err != nil {
		t.Fatalf("scanTree(second): %v", err)
	}
	if !manifestEntriesEqual(manifestOne, manifestTwo) || digestManifest(manifestOne) != digestManifest(manifestTwo) {
		t.Fatalf("manifest/digest depends on root or mtime:\n%+v\n%+v", manifestOne, manifestTwo)
	}
}

func TestSecurityScannerRejectsContentAndPathBypasses(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *securityPolicy)
		want   string
	}{
		{"uppercase private extension", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-secret.PEM", []byte("not even PEM"), "100644")
		}, "forbidden credential extension"},
		{"environment variant", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0/.env.PROD", []byte("TOKEN=value"), "100644")
		}, "forbidden environment filename"},
		{"ELF magic", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-binary", append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fixture")...), "100644")
		}, "ELF magic"},
		{"generic private key marker", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-key.txt", []byte("prefix -----"+"BEGIN ENCRYPTED PRIVATE KEY"+"----- suffix"), "100644")
		}, "private-key marker"},
		{"old Core module", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-core.txt", []byte(testCoreModule), "100644")
		}, "old Core module"},
		{"old Things outside template", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-things.txt", []byte(testThingsModule), "100644")
		}, "old Things module"},
		{"new trailing whitespace", func(t *testing.T, root string, policy *securityPolicy) {
			addExpectedRegular(t, root, policy, "0-trailing.txt", []byte("new trailing space \n"), "100644")
		}, "introduces trailing whitespace"},
		{"wrong credential placeholder", func(t *testing.T, root string, policy *securityPolicy) {
			policy.lock.Transforms.CredentialLineCount = 1
			policy.lock.Transforms.CredentialFileCount = 1
			policy.lock.Transforms.CredentialActiveLineCount = 1
			policy.lock.Transforms.CredentialPaths = CredentialPathCounts{DatabaseDSN: 1}
			addExpectedRegular(t, root, policy, "0-config.yaml", []byte("Database:\n  DSN: ${WRONG_DSN}\n"), "100644")
		}, "placeholder"},
		{"excluded top-level root", func(t *testing.T, root string, policy *securityPolicy) {
			policy.lock.Source.Exclude = append(policy.lock.Source.Exclude, "deploy")
			addExpectedRegular(t, root, policy, "deploy/file.txt", []byte("fixture"), "100644")
		}, "excluded top-level"},
		{"mutated generated README exception", func(t *testing.T, root string, policy *securityPolicy) {
			data := append([]byte(nil), policy.expectedFiles["README.md"].Data...)
			data = append(data, []byte("mutation\n")...)
			replaceExpectedRegular(t, root, policy, "README.md", data, "100644")
		}, "excluded top-level"},
		{"oversized before read", func(t *testing.T, root string, policy *securityPolicy) {
			maximum := maxExpectedSize(policy.expectedFiles)
			policy.lock.Security.MaxFileBytes = maximum
			addExpectedRegular(t, root, policy, "0-large.bin", bytes.Repeat([]byte{'x'}, int(maximum+1)), "100644")
		}, "exceeds maximum before read"},
		{"wrong file mode", func(t *testing.T, root string, policy *securityPolicy) {
			if err := os.Chmod(filepath.Join(root, "service", "safe.txt"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "mode"},
		{"wrong directory mode", func(t *testing.T, root string, policy *securityPolicy) {
			if err := os.Chmod(filepath.Join(root, "service"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, "directory has mode"},
		{"license mismatch", func(t *testing.T, _ string, policy *securityPolicy) {
			policy.expectedLicenseSHA = strings.Repeat("0", 64)
		}, "LICENSE SHA-256"},
		{"unexpected empty directory", func(t *testing.T, root string, _ *securityPolicy) {
			if err := os.Mkdir(filepath.Join(root, "0-empty-dir"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "unexpected directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, policy := scannerFixture(t)
			tt.mutate(t, root, &policy)
			_, _, _, err := scanTree(root, policy)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("scanTree() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSecurityScannerVerifiesNonYAMLCredentialOutputsIndependently(t *testing.T) {
	newFixture := func(t *testing.T) (string, securityPolicy, []byte, []byte) {
		t.Helper()
		root, policy := scannerFixture(t)
		_, cloudOutput := cloudCredentialFixtureForTest()
		_, productOutput := productSecretFixtureForTest()
		policy.lock.Transforms.NonYAMLCredentialValueCount = 5
		addExpectedRegular(t, root, &policy, cloudProtocolCredentialPath, cloudOutput, "100644")
		addExpectedRegular(t, root, &policy, productSecretPath, productOutput, "100644")
		return root, policy, cloudOutput, productOutput
	}

	t.Run("accept exact placeholders", func(t *testing.T) {
		root, policy, _, _ := newFixture(t)
		if _, _, _, err := scanTree(root, policy); err != nil {
			t.Fatalf("scanTree() rejected exact non-YAML credential outputs: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*testing.T, string, *securityPolicy, []byte, []byte)
		want   string
	}{
		{
			name: "cloud raw value",
			mutate: func(t *testing.T, root string, policy *securityPolicy, cloud, _ []byte) {
				bad := bytes.Replace(cloud, []byte(cloudAccessKeyIDPlaceholder), []byte("cloud-raw-redaction-sentinel"), 1)
				replaceExpectedRegular(t, root, policy, cloudProtocolCredentialPath, bad, "100644")
			},
			want: "cloud protocol credential replacement count",
		},
		{
			name: "product wrong placeholder",
			mutate: func(t *testing.T, root string, policy *securityPolicy, _ []byte, product []byte) {
				bad := bytes.Replace(product, []byte(productSecretPlaceholder), []byte("${WRONG_PRODUCT_SECRET}"), 1)
				replaceExpectedRegular(t, root, policy, productSecretPath, bad, "100644")
			},
			want: "product secret replacement count",
		},
		{
			name: "product extra secret path",
			mutate: func(t *testing.T, root string, policy *securityPolicy, _ []byte, product []byte) {
				bad := bytes.Replace(product, []byte(`"Name":"one"`), []byte(`"Name":"one","Secret":"${THINGS_INIT_PRODUCT_SECRET}"`), 1)
				replaceExpectedRegular(t, root, policy, productSecretPath, bad, "100644")
			},
			want: "product secret replacement count",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, policy, cloud, product := newFixture(t)
			tt.mutate(t, root, &policy, cloud, product)
			_, _, _, err := scanTree(root, policy)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("scanTree() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "cloud-raw-redaction-sentinel") {
				t.Fatalf("scanTree() leaked non-YAML credential value: %v", err)
			}
		})
	}
}

func TestSecurityScannerRejectsLinksAndSpecialFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root, policy := scannerFixture(t)
		path := filepath.Join(root, "0-link")
		if err := os.Symlink(filepath.Join(root, "LICENSE"), path); err != nil {
			t.Fatal(err)
		}
		policy.expectedFiles["0-link"] = RenderedFile{Mode: "100644", Data: []byte("fixture")}
		policy.lock.Source.SelectedFiles++
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink error = %v", err)
		}
	})

	t.Run("hardlink", func(t *testing.T) {
		root, policy := scannerFixture(t)
		path := filepath.Join(root, "0-hardlink")
		if err := os.Link(filepath.Join(root, "service", "safe.txt"), path); err != nil {
			t.Fatal(err)
		}
		policy.expectedFiles["0-hardlink"] = RenderedFile{Mode: "100644", Data: []byte("safe\n")}
		policy.lock.Source.SelectedFiles++
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "hardlink") {
			t.Fatalf("hardlink error = %v", err)
		}
	})

	t.Run("FIFO", func(t *testing.T) {
		root, policy := scannerFixture(t)
		path := filepath.Join(root, "0-fifo")
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		policy.expectedFiles["0-fifo"] = RenderedFile{Mode: "100644", Data: nil}
		policy.lock.Source.SelectedFiles++
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("FIFO error = %v", err)
		}
	})

	t.Run("socket", func(t *testing.T) {
		root, policy := scannerFixture(t)
		path := filepath.Join(root, "0-socket")
		fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)
		if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
			t.Fatal(err)
		}
		policy.expectedFiles["0-socket"] = RenderedFile{Mode: "100644", Data: nil}
		policy.lock.Source.SelectedFiles++
		_, _, _, err = scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("socket error = %v", err)
		}
	})

	t.Run("device", func(t *testing.T) {
		root, policy := scannerFixture(t)
		path := filepath.Join(root, "0-device")
		err := unix.Mknod(path, unix.S_IFCHR|0o600, int(unix.Mkdev(1, 3)))
		if err == unix.EPERM || err == unix.EACCES {
			t.Skip("test environment does not allow creating a device node")
		}
		if err != nil {
			t.Fatal(err)
		}
		policy.expectedFiles["0-device"] = RenderedFile{Mode: "100644", Data: nil}
		policy.lock.Source.SelectedFiles++
		_, _, _, err = scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("device error = %v", err)
		}
	})

	t.Run("root symlink", func(t *testing.T) {
		root, policy := scannerFixture(t)
		link := filepath.Join(t.TempDir(), "tree-link")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		_, _, _, err := scanTree(link, policy)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("root symlink error = %v", err)
		}
	})
}

func TestSecurityScannerOnlyAppliesExcludeAtTopLevel(t *testing.T) {
	root, policy := scannerFixture(t)
	policy.lock.Source.Exclude = append(policy.lock.Source.Exclude, "deploy")
	addExpectedRegular(t, root, &policy, "service/deploy/nested.txt", []byte("allowed nested name\n"), "100644")
	if _, _, _, err := scanTree(root, policy); err != nil {
		t.Fatalf("scanTree() rejected nested excluded-name component: %v", err)
	}
}

func TestSecurityScannerDetectsConcurrentReplacementAndDirectoryMutation(t *testing.T) {
	t.Run("regular file mode changes after stat", func(t *testing.T) {
		root, policy := scannerFixture(t)
		mutated := false
		policy.afterPathStat = func(relative string) {
			if mutated || relative != "service/safe.txt" {
				return
			}
			mutated = true
			if err := os.Chmod(filepath.Join(root, "service", "safe.txt"), 0o600); err != nil {
				panic(err)
			}
		}
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "metadata changed") {
			t.Fatalf("file mode mutation error = %v", err)
		}
	})

	t.Run("regular file replacement with identical bytes", func(t *testing.T) {
		root, policy := scannerFixture(t)
		mutated := false
		policy.afterPathStat = func(relative string) {
			if mutated || relative != "service/safe.txt" {
				return
			}
			mutated = true
			path := filepath.Join(root, "service", "safe.txt")
			if err := os.Rename(path, path+".old"); err != nil {
				panic(err)
			}
			if err := os.WriteFile(path, []byte("safe\n"), 0o644); err != nil {
				panic(err)
			}
		}
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "identity changed") {
			t.Fatalf("file replacement error = %v", err)
		}
	})

	t.Run("directory mode changes after stat", func(t *testing.T) {
		root, policy := scannerFixture(t)
		mutated := false
		policy.afterPathStat = func(relative string) {
			if mutated || relative != "service" {
				return
			}
			mutated = true
			if err := os.Chmod(filepath.Join(root, "service"), 0o700); err != nil {
				panic(err)
			}
		}
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "metadata changed") {
			t.Fatalf("directory mode mutation error = %v", err)
		}
	})

	t.Run("directory exchanged for symlink", func(t *testing.T) {
		root, policy := scannerFixture(t)
		escape := t.TempDir()
		mutated := false
		policy.afterPathStat = func(relative string) {
			if mutated || relative != "service" {
				return
			}
			mutated = true
			path := filepath.Join(root, "service")
			if err := os.Rename(path, path+"-old"); err != nil {
				panic(err)
			}
			if err := os.Symlink(escape, path); err != nil {
				panic(err)
			}
		}
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "open scanned directory") {
			t.Fatalf("directory exchange error = %v", err)
		}
	})

	t.Run("directory entries changed", func(t *testing.T) {
		root, policy := scannerFixture(t)
		mutated := false
		policy.afterDirectoryList = func(relative string) {
			if mutated || relative != "" {
				return
			}
			mutated = true
			if err := os.WriteFile(filepath.Join(root, "late-file"), []byte("late"), 0o644); err != nil {
				panic(err)
			}
		}
		_, _, _, err := scanTree(root, policy)
		if err == nil || !strings.Contains(err.Error(), "entries changed") {
			t.Fatalf("directory mutation error = %v", err)
		}
	})
}

func scannerFixture(t *testing.T) (string, securityPolicy) {
	t.Helper()
	_, lock, snapshot, _ := syntheticRenderFixture(t)
	lock.Transforms.CredentialLineCount = 0
	lock.Transforms.CredentialFileCount = 0
	lock.Transforms.CredentialActiveLineCount = 0
	lock.Transforms.CredentialCommentedLineCount = 0
	lock.Transforms.CredentialPaths = CredentialPathCounts{}
	lock.Transforms.NonYAMLCredentialValueCount = 0
	lock.Source.Exclude = []string{"README.md", "Makefile"}
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	templates, err := renderTargetTemplates(&lock, snapshot, TransformReport{CorePrefixes: 6, ThingsPrefixes: 3, CredentialLines: 1, PrivateKeyBlocks: 1, VetFixes: 1, SchemaFixtureIdentifiers: 2})
	if err != nil {
		t.Fatal(err)
	}
	license := []byte("fixture AGPL license\n")
	expected := cloneRenderedMap(templates)
	expected["LICENSE"] = RenderedFile{Mode: "100644", Data: license}
	expected["service/safe.txt"] = RenderedFile{Mode: "100644", Data: []byte("safe\n")}
	for path, file := range expected {
		writeExpectedRegular(t, root, path, file.Data, file.Mode)
	}
	lock.Source.SelectedFiles = len(expected) - 4
	lock.Security.MaxFileBytes = 1 << 20
	licenseSHA := sha256.Sum256(license)
	return root, securityPolicy{
		lock: &lock, expectedFiles: expected, expectedTemplates: templates,
		expectedLicenseSHA:        fmt.Sprintf("%x", licenseSHA),
		allowedTrailingWhitespace: map[trailingWhitespaceFingerprint]struct{}{},
	}
}

func addExpectedRegular(t *testing.T, root string, policy *securityPolicy, path string, data []byte, mode string) {
	t.Helper()
	if _, exists := policy.expectedFiles[path]; exists {
		t.Fatalf("duplicate expected path %q", path)
	}
	policy.expectedFiles[path] = RenderedFile{Mode: mode, Data: append([]byte(nil), data...)}
	policy.lock.Source.SelectedFiles++
	writeExpectedRegular(t, root, path, data, mode)
}

func replaceExpectedRegular(t *testing.T, root string, policy *securityPolicy, path string, data []byte, mode string) {
	t.Helper()
	policy.expectedFiles[path] = RenderedFile{Mode: mode, Data: append([]byte(nil), data...)}
	writeExpectedRegular(t, root, path, data, mode)
}

func writeExpectedRegular(t *testing.T, root, path string, data []byte, mode string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	fileMode := os.FileMode(0o644)
	if mode == "100755" {
		fileMode = 0o755
	}
	if err := os.WriteFile(filename, data, fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, fileMode); err != nil {
		t.Fatal(err)
	}
}

func maxExpectedSize(files map[string]RenderedFile) int64 {
	var maximum int64
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if size := int64(len(files[path].Data)); size > maximum {
			maximum = size
		}
	}
	return maximum
}
