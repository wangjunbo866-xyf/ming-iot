package repositorycontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type historicalReferences struct {
	SchemaVersion int                   `json:"schema_version"`
	Purpose       string                `json:"purpose"`
	Files         []historicalReference `json:"files"`
}
type historicalReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
}

// Historical records are explicit immutable files, never a directory-wide skip.
func localShareHistoricalReferencePaths(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "tooling/policy/HISTORICAL_REFERENCES.json"))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest historicalReferences
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("historical reference manifest has trailing data")
	}
	if manifest.SchemaVersion != 1 || len(manifest.Files) == 0 {
		return nil, fmt.Errorf("invalid historical reference manifest")
	}
	var paths []string
	seen := map[string]bool{}
	for _, item := range manifest.Files {
		if err := validateHistoricalReference(root, item); err != nil {
			return nil, err
		}
		if seen[item.Path] {
			return nil, fmt.Errorf("duplicate historical reference: %s", item.Path)
		}
		seen[item.Path] = true
		paths = append(paths, item.Path)
	}
	return paths, nil
}
func validateHistoricalReference(root string, item historicalReference) error {
	p := item.Path
	if p == "" || strings.Contains(p, "\\") || filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p || strings.HasPrefix(p, "../") {
		return fmt.Errorf("invalid historical path: %s", p)
	}
	allowed := strings.HasPrefix(p, "docs/wiki/legacy/")
	ext := filepath.Ext(p)
	if !allowed || (ext != ".md" && ext != ".json" && ext != ".txt") || strings.TrimSpace(item.Reason) == "" {
		return fmt.Errorf("not an explicit historical document: %s", p)
	}
	full := filepath.Join(root, filepath.FromSlash(p))
	// Do not bless links into active code or content outside this checkout.
	for current := full; current != root; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("historical reference is a symlink: %s", p)
		}
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != item.SHA256 {
		return fmt.Errorf("historical document changed; review its source and digest: %s", p)
	}
	if !bytes.Contains(data, []byte(legacyShareModulePath)) {
		return fmt.Errorf("stale historical reference: %s", p)
	}
	return nil
}
func TestHistoricalReferencesRejectActiveOrChangedFiles(t *testing.T) {
	root := t.TempDir()
	historical := "docs/wiki/legacy/rd/evidence/frozen.md"
	full := filepath.Join(root, historical)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte("Historical dependency: " + legacyShareModulePath)
	if err := os.WriteFile(full, body, 0644); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	good := historicalReference{Path: historical, SHA256: digest, Reason: "frozen source"}
	if err := validateHistoricalReference(root, good); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"backend/share/live.go", "docs/wiki/legacy/rd/live.go", "docs/specs/current.md", "../escape.md", "docs/wiki/legacy/rd/../live.md"} {
		t.Run(path, func(t *testing.T) {
			bad := good
			bad.Path = path
			if validateHistoricalReference(root, bad) == nil {
				t.Fatal("accepted active or unsafe path")
			}
		})
	}
	if err := os.WriteFile(full, append(body, []byte(" changed")...), 0644); err != nil {
		t.Fatal(err)
	}
	if validateHistoricalReference(root, good) == nil {
		t.Fatal("accepted changed history without digest review")
	}
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "active.md"), full); err != nil {
		t.Fatal(err)
	}
	if validateHistoricalReference(root, good) == nil {
		t.Fatal("accepted symlink")
	}
}
func TestHistoricalClassificationStillFindsNewActiveReferences(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, path := range []string{"docs/wiki/legacy/rd/unregistered.md", "backend/live.go", "tooling/live.py"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(legacyShareModulePath), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got := localShareGitGrepFiles(t, root, legacyShareModulePath)
	if len(got) != 3 {
		t.Fatalf("new active/untracked references lost: %v", got)
	}
	_, unexpected := localSharePathDifference(got, nil)
	if len(unexpected) != 3 {
		t.Fatalf("new references incorrectly exempted: %v", unexpected)
	}
}
