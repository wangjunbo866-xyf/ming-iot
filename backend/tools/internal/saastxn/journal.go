package saastxn

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Journal states.
const (
	statePrepare   = "prepare"
	stateCommitted = "committed"
)

// journal is an append-only, fsync'd record of transaction progress. Each
// write opens in append mode, writes a delimited record, fsyncs the file and
// its parent directory.
type journal struct {
	path *os.File
}

func openJournal(path string) (*journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &journal{path: f}, nil
}

func (j *journal) write(state string, kv map[string]string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "state=%s\n", state)
	for k, v := range kv {
		fmt.Fprintf(&b, "  %s=%s\n", k, strings.ReplaceAll(v, "\n", " "))
	}
	if _, err := j.path.WriteString(b.String()); err != nil {
		return err
	}
	if err := j.path.Sync(); err != nil {
		return err
	}
	return fsyncDir(j.path.Name())
}

func (j *journal) close() {
	if j != nil && j.path != nil {
		_ = j.path.Close()
	}
}

// fsyncDir fsyncs the parent directory of path for durability.
func fsyncDir(path string) error {
	dir := filepath.Dir(path)
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// readJournalStates returns the ordered list of states recorded in the journal
// at path. Used by recovery; empty if the journal does not exist.
func readJournalStates(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var states []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "state=") {
			states = append(states, strings.TrimPrefix(line, "state="))
		}
	}
	return states, sc.Err()
}
