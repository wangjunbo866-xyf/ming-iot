// Package thingsbaselinetemplates embeds the four target-owned L0 files.
package thingsbaselinetemplates

import "embed"

// FS is immutable at runtime and makes rendering independent of cwd.
//
//go:embed README.md NOTICE PROVENANCE.yaml Makefile
var FS embed.FS
