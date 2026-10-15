// Command saastxn is a thin CLI wrapper around the saastxn library, used by
// generate-syspb.sh to atomically swap the live backend/saas tree with a
// candidate produced by a shell command.
//
// Usage:
//
//	saastxn commit \
//	  --live PATH \
//	  --txn-dir PATH \
//	  --allowlist FILE \
//	  --mutate 'SHELL COMMAND'
//
// --allowlist points to a newline-delimited file of repo-relative paths that
// are permitted to change. --mutate is a single shell command run via
// "sh -c" with the candidate directory as $1 (and the CWD).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"ming-iot/backend/tools/internal/saastxn"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "commit":
		commit(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: saastxn commit --live PATH --txn-dir PATH --allowlist FILE --mutate 'SHELL COMMAND'

Atomically replace the live tree at --live with a candidate produced by running
--mutate on a full copy. --mutate runs via "sh -c" with the candidate directory
as $1 and as the working directory. Only paths listed in --allowlist (one
repo-relative path per line) may differ.
`)
}

func commit(args []string) {
	fs := flag.NewFlagSet("saastxn commit", flag.ExitOnError)
	live := fs.String("live", "", "live tree to replace (required)")
	txnDir := fs.String("txn-dir", "", "transaction working dir (required)")
	allowlistFile := fs.String("allowlist", "", "newline-delimited allowlist file (required)")
	mutate := fs.String("mutate", "", "shell command run on the candidate ($1 = candidate dir) (required)")
	_ = fs.Parse(args)

	if *live == "" || *txnDir == "" || *allowlistFile == "" || *mutate == "" {
		fmt.Fprintln(os.Stderr, "saastxn: --live, --txn-dir, --allowlist and --mutate are all required")
		os.Exit(2)
	}
	allow, err := readLines(*allowlistFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "saastxn: read allowlist: %v\n", err)
		os.Exit(2)
	}

	res, err := saastxn.Commit(context.Background(), saastxn.Options{
		LiveDir:   *live,
		TxnDir:    *txnDir,
		Allowlist: allow,
		MutateCandidate: func(candidate string) error {
			cmd := exec.Command("sh", "-c", *mutate, "saastxn-mutate", candidate)
			cmd.Dir = candidate
			// GOWORK=off so candidate Go commands never discover the parent
			// backend/go.work (which only includes ./core).
			cmd.Env = append(os.Environ(), "GOWORK=off")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					return fmt.Errorf("mutate command exited %d", ee.ExitCode())
				}
				return err
			}
			return nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "saastxn: %v\n", err)
		os.Exit(1)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "saastxn: warning: %s\n", w)
	}
	fmt.Printf("saastxn: %s\n", res.Status)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}
