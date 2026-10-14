package repositorycontract

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyGraphRetired(t *testing.T) {
	repoRoot := findRepositoryRoot(t)

	for _, path := range []string{
		".graph",
		"backend/tools/cmd/graphctl",
		"backend/tools/internal/graphctl",
	} {
		t.Run("path_contains_no_files_"+strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			root := filepath.Join(repoRoot, path)
			err := filepath.WalkDir(root, func(entryPath string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() {
					t.Errorf("retired legacy Graph file still exists: %s", entryPath)
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("inspect retired path %s: %v", path, err)
			}
		})
	}

	activeFiles := []string{
		".gitignore",
		"AGENTS.md",
		"backend/tools/README.md",
	}
	markers := []string{
		"graphctl",
		".graph/runtime",
		".graph/registry",
		".graph/loops",
	}
	for _, path := range activeFiles {
		t.Run("active_references_absent_"+strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repoRoot, path))
			if err != nil {
				t.Fatalf("read active repository file %s: %v", path, err)
			}
			for _, marker := range markers {
				if strings.Contains(strings.ToLower(string(content)), marker) {
					t.Errorf("active repository file %s still references retired marker %q", path, marker)
				}
			}
		})
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repository root not found from %s", dir)
		}
		dir = parent
	}
}
