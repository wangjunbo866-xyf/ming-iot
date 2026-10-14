// Command clientcontract snapshots or verifies the public Go API surface of a
// client package tree.
//
// Usage:
//
//	clientcontract snapshot --root CLIENT_DIR --out PUBLIC_API_GOLDEN
//	clientcontract verify   --root CLIENT_DIR --golden PUBLIC_API_GOLDEN
//
// snapshot writes the extracted document as pretty JSON to --out (only when
// explicitly invoked). verify is read-only: it re-extracts and compares to the
// golden; any missing/new/changed entry exits 1.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"ming-iot/backend/tools/internal/clientcontract"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "snapshot":
		snapshot(os.Args[2:])
	case "verify":
		verify(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  clientcontract snapshot --root CLIENT_DIR --out PUBLIC_API_GOLDEN
  clientcontract verify   --root CLIENT_DIR --golden PUBLIC_API_GOLDEN
`)
}

func snapshot(args []string) {
	fs := flag.NewFlagSet("clientcontract snapshot", flag.ExitOnError)
	root := fs.String("root", "", "client package root (required)")
	out := fs.String("out", "", "output golden JSON path (required)")
	_ = fs.Parse(args)
	if *root == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "snapshot: --root and --out are required")
		os.Exit(2)
	}
	doc, err := clientcontract.Extract(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: %v\n", err)
		os.Exit(2)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: marshal: %v\n", err)
		os.Exit(2)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: write %s: %v\n", *out, err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "snapshot: wrote %s (%d aliases, %d interfaces, %d functions)\n",
		*out, len(doc.Aliases), len(doc.Interfaces), len(doc.Functions))
}

func verify(args []string) {
	fs := flag.NewFlagSet("clientcontract verify", flag.ExitOnError)
	root := fs.String("root", "", "client package root (required)")
	golden := fs.String("golden", "", "golden JSON path (required)")
	_ = fs.Parse(args)
	if *root == "" || *golden == "" {
		fmt.Fprintln(os.Stderr, "verify: --root and --golden are required")
		os.Exit(2)
	}
	goldData, err := os.ReadFile(*golden)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: read golden %s: %v\n", *golden, err)
		os.Exit(1)
	}
	var gold clientcontract.Document
	if err := json.Unmarshal(goldData, &gold); err != nil {
		fmt.Fprintf(os.Stderr, "verify: parse golden: %v\n", err)
		os.Exit(1)
	}
	got, err := clientcontract.Extract(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(1)
	}
	if !documentsEqual(gold, got) {
		fmt.Fprintln(os.Stderr, "verify: public API surface changed (golden mismatch)")
		fmt.Fprintf(os.Stderr, "  golden: %d aliases, %d interfaces, %d functions\n",
			len(gold.Aliases), len(gold.Interfaces), len(gold.Functions))
		fmt.Fprintf(os.Stderr, "  actual: %d aliases, %d interfaces, %d functions\n",
			len(got.Aliases), len(got.Interfaces), len(got.Functions))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "verify: OK (%d aliases, %d interfaces, %d functions)\n",
		len(got.Aliases), len(got.Interfaces), len(got.Functions))
}

func documentsEqual(a, b clientcontract.Document) bool {
	if len(a.Aliases) != len(b.Aliases) || len(a.Interfaces) != len(b.Interfaces) || len(a.Functions) != len(b.Functions) {
		return false
	}
	for i := range a.Aliases {
		if a.Aliases[i] != b.Aliases[i] {
			return false
		}
	}
	for i := range a.Interfaces {
		if !interfaceEqual(a.Interfaces[i], b.Interfaces[i]) {
			return false
		}
	}
	for i := range a.Functions {
		if a.Functions[i] != b.Functions[i] {
			return false
		}
	}
	return true
}

func interfaceEqual(a, b clientcontract.Interface) bool {
	if a.Package != b.Package || a.Name != b.Name || len(a.Methods) != len(b.Methods) {
		return false
	}
	for i := range a.Methods {
		if a.Methods[i] != b.Methods[i] {
			return false
		}
	}
	return true
}
