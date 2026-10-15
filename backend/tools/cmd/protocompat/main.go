// Command protocompat is a read-only gate that compares two FileDescriptorSet
// (.protoset) files for one protobuf package and reports semantic compatibility.
//
// Usage:
//
//	protocompat -old OLD.protoset -new NEW.protoset -package sys [-summary-json OUT.json]
//
// Exit codes:
//
//	0 - contracts are compatible
//	1 - contracts differ (sorted diffs printed to stderr)
//	2 - input could not be read/parsed, or package is absent
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"ming-iot/backend/tools/internal/protocompat"
)

func main() {
	oldPath := flag.String("old", "", "old .protoset path (required)")
	newPath := flag.String("new", "", "new .protoset path (required)")
	pkg := flag.String("package", "", "protobuf package to compare (required)")
	summaryJSON := flag.String("summary-json", "", "optional path to write a deterministic summary JSON")
	flag.Parse()

	if *oldPath == "" || *newPath == "" || *pkg == "" {
		fmt.Fprintln(os.Stderr, "usage: protocompat -old OLD.protoset -new NEW.protoset -package sys [-summary-json OUT.json]")
		os.Exit(2)
	}

	oldC, err := protocompat.Load(*oldPath, *pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load old: %v\n", err)
		os.Exit(2)
	}
	newC, err := protocompat.Load(*newPath, *pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load new: %v\n", err)
		os.Exit(2)
	}

	if *summaryJSON != "" {
		if err := writeSummary(*summaryJSON, newC); err != nil {
			fmt.Fprintf(os.Stderr, "write summary: %v\n", err)
			os.Exit(2)
		}
	}

	diffs := protocompat.Compare(oldC, newC)
	if len(diffs) == 0 {
		fmt.Fprintf(os.Stdout, "compatible: %d services, %d methods, %d messages\n",
			countServices(newC), countMethods(newC), countTopLevelMessages(newC))
		os.Exit(0)
	}

	for _, d := range diffs {
		if d.Old == "" && d.New == "" {
			fmt.Fprintf(os.Stderr, "%s\n", d.Path)
		} else if d.Old == "" {
			fmt.Fprintf(os.Stderr, "%s -> %s\n", d.Path, d.New)
		} else if d.New == "" {
			fmt.Fprintf(os.Stderr, "%s (was %s)\n", d.Path, d.Old)
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s -> %s\n", d.Path, d.Old, d.New)
		}
	}
	os.Exit(1)
}

// summaryDoc is the deterministic JSON written by -summary-json.
type summaryDoc struct {
	Package      string   `json:"package"`
	Services     int      `json:"services"`
	Methods      int      `json:"methods"`
	Messages     int      `json:"messages"`
	Enums        int      `json:"enums"`
	ServiceNames []string `json:"serviceNames"`
}

func writeSummary(path string, c protocompat.Contract) error {
	doc := summaryDoc{
		Package:      c.Package,
		Services:     countServices(c),
		Methods:      countMethods(c),
		Messages:     countTopLevelMessages(c),
		Enums:        len(c.Enums),
		ServiceNames: sortedServiceNames(c),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func countServices(c protocompat.Contract) int         { return len(c.Services) }
func countTopLevelMessages(c protocompat.Contract) int { return len(c.TopLevelMessages) }

func countMethods(c protocompat.Contract) int {
	n := 0
	for _, s := range c.Services {
		n += len(s.Methods)
	}
	return n
}

func sortedServiceNames(c protocompat.Contract) []string {
	names := make([]string, 0, len(c.Services))
	for k := range c.Services {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
