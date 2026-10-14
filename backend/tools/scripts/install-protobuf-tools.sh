#!/usr/bin/env bash
# install-protobuf-tools.sh installs a hermetic protobuf toolchain used to
# regenerate the syssvr PB layer from the split proto sources.
#
# Two modes:
#   bootstrap - builds protoc 34.1 + protoc-gen-go v1.31.0 + protoc-gen-go-grpc
#               v1.3.0 into backend/bin/protobuf-tools/ and prints four
#               canonical SHA-256 lines (archive, protoc, protoc-gen-go,
#               protoc-gen-go-grpc) on stdout in that fixed order. All logs go
#               to stderr. After human review, paste the values into
#               protobuf-tools.lock and run "install".
#   install   - verifies the four checksums recorded in protobuf-tools.lock and
#               installs the tools if missing; refuses to silently update or
#               proceed when a checksum is absent/mismatched.
#
# Files installed (all under backend/bin/protobuf-tools/, which is gitignored):
#   bin/protoc                          (from the official archive)
#   bin/protoc-gen-go                   (go install)
#   bin/protoc-gen-go-grpc              (go install)
#   include/google/protobuf/*.proto     (standard protos from the archive)
#
# The script never reuses the system protoc 25.3, protoc-gen-go 1.36.6 or grpc
# plugin 1.6.1; it always builds the pinned versions.

set -euo pipefail

# Locate repo root from script path so the script works from any CWD.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
LOCK_FILE="$REPO_ROOT/backend/tools/protobuf-tools.lock"
DEST_DIR="$REPO_ROOT/backend/bin/protobuf-tools"

ARCHIVE_URL="https://github.com/protocolbuffers/protobuf/releases/download/v34.1/protoc-34.1-linux-x86_64.zip"
PROTOC_VERSION="34.1"
PROTOC_GEN_GO_VERSION="v1.31.0"
PROTOC_GEN_GO_GRPC_VERSION="v1.3.0"
GO_TOOLCHAIN_VERSION="go1.24.4"

log() { printf '[install-protobuf-tools] %s\n' "$*" >&2; }
die() { printf '[install-protobuf-tools] ERROR: %s\n' "$*" >&2; exit 1; }

require() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

# sha256_file prints the SHA-256 of a file in canonical "hash  filename" form
# (64 hex digits, two spaces, basename). This matches `sha256sum`.
sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1"  "$2}'
  else
    shasum -a 256 "$file" | awk '{print $1"  "$2}'
  fi
}

# sha256_data prints the SHA-256 of stdin data followed by two spaces and the
# given label (used for the archive checksum which is computed over bytes).
sha256_data() {
  local label="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk -v l="$label" '{print $1"  "l}'
  else
    shasum -a 256 | awk -v l="$label" '{print $1"  "l}'
  fi
}

# read_lock_value KEY extracts the quoted value for KEY from protobuf-tools.lock.
# KEY is the dotted "section.field" form, e.g. "protoc.archiveSha256". The lock
# file uses YAML-like nesting: top-level keys at column 0, nested fields
# indented by two spaces.
read_lock_value() {
  local key="$1"
  local section="${key%%.*}"
  local field="${key#*.}"
  awk -v section="$section" -v field="$field" '
    /^[A-Za-z]/ {
      # top-level key line: "name:" at column 0
      cur=$0
      sub(/:.*/, "", cur)
      gsub(/[[:space:]]/, "", cur)
      current=cur
      next
    }
    /^[[:space:]]+[A-Za-z]/ {
      line=$0
      sub(/^[[:space:]]+/, "", line)         # strip indent
      sub(/:.*/, "", line)                   # keep field name only
      val=$0
      sub(/^[^"]*"/, "", val)               # strip up to first quote
      sub(/".*/, "", val)                    # strip trailing quote and rest
      if (current == section && line == field) {
        print val
        found=1
        exit
      }
    }
    END { if (!found) exit 1 }
  ' "$LOCK_FILE"
}

ensure_dirs() {
  mkdir -p "$DEST_DIR/bin" "$DEST_DIR/include"
}

build_protoc() {
  require curl || require wget
  require unzip
  ensure_dirs
  local work
  work="$(mktemp -d)"
  local archive="$work/protoc.zip"
  # Ensure cleanup runs when this function returns; guard against re-entrancy.
  local _cleanup="rm -rf '$work'"
  trap -- "$_cleanup" RETURN
  log "downloading $ARCHIVE_URL"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error --location -o "$archive" "$ARCHIVE_URL"
  else
    wget --quiet -O "$archive" "$ARCHIVE_URL"
  fi
  unzip -o -q "$archive" -d "$work/protoc-extract"
  cp "$work/protoc-extract/bin/protoc" "$DEST_DIR/bin/protoc"
  mkdir -p "$DEST_DIR/include/google/protobuf"
  cp -R "$work/protoc-extract/include/google/protobuf/." "$DEST_DIR/include/google/protobuf/"
  # Print archive checksum to stdout with a fixed label (the basename of the
  # release archive), so two bootstraps produce byte-identical output.
  cat "$archive" | sha256_data "protoc-34.1-linux-x86_64.zip"
}

build_plugin() {
  local module="$1" version="$2" outname="$3"
  ensure_dirs
  log "go install $module@$version -> $outname"
  GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go install -trimpath -ldflags=-buildid= "$module@$version"
  # Locate the built binary in GOBIN / GOPATH(bin) and copy it in.
  local gopath_bin
  gopath_bin="$(GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" GOOS=linux GOARCH=amd64 go env GOPATH)/bin/linux_amd64/$outname"
  if [[ ! -f "$gopath_bin" ]]; then
    gopath_bin="$(GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" go env GOPATH)/bin/$outname"
  fi
  [[ -f "$gopath_bin" ]] || die "built plugin not found: $outname at $gopath_bin"
  cp "$gopath_bin" "$DEST_DIR/bin/$outname"
  sha256_file "$DEST_DIR/bin/$outname"
}

do_bootstrap() {
  log "bootstrap mode: building pinned tools into $DEST_DIR"
  local archive_line protoc_line go_line grpc_line
  archive_line="$(build_protoc)"
  protoc_line="$(sha256_file "$DEST_DIR/bin/protoc")"
  go_line="$(build_plugin google.golang.org/protobuf/cmd/protoc-gen-go "$PROTOC_GEN_GO_VERSION" protoc-gen-go)"
  grpc_line="$(build_plugin google.golang.org/grpc/cmd/protoc-gen-go-grpc "$PROTOC_GEN_GO_GRPC_VERSION" protoc-gen-go-grpc)"
  verify_versions
  # stdout: four canonical lines in fixed order.
  printf '%s\n' "$archive_line"
  printf '%s\n' "$protoc_line"
  printf '%s\n' "$go_line"
  printf '%s\n' "$grpc_line"
}

verify_versions() {
  local got
  got="$("$DEST_DIR/bin/protoc" --version)"
  [[ "$got" == "libprotoc $PROTOC_VERSION" ]] || die "protoc version mismatch: got '$got'"
  log "protoc: $got"
  log "protoc-gen-go: $("$DEST_DIR/bin/protoc-gen-go" --version 2>&1 || true)"
  log "protoc-gen-go-grpc: $("$DEST_DIR/bin/protoc-gen-go-grpc" --version 2>&1 || true)"
}

expect_locked() {
  local key="$1" label="$2"
  local val
  val="$(read_lock_value "$key")" || die "$label checksum missing in $LOCK_FILE (run bootstrap first)"
  [[ -n "$val" ]] || die "$label checksum empty in $LOCK_FILE (run bootstrap first)"
  printf '%s' "$val"
}

verify_against_lock() {
  local archive_sha protoc_sha go_sha grpc_sha
  archive_sha="$(expect_locked protoc.archiveSha256 'protoc archive')"
  protoc_sha="$(expect_locked protoc.binarySha256 'protoc binary')"
  go_sha="$(expect_locked protocGenGo.binarySha256 'protoc-gen-go binary')"
  grpc_sha="$(expect_locked protocGenGoGrpc.binarySha256 'protoc-gen-go-grpc binary')"

  log "verifying against locked checksums"
  # If binaries already present and matching, skip rebuild; otherwise build and
  # verify. We always re-derive the archive checksum from a fresh download to
  # confirm reproducibility.
  local work archive
  work="$(mktemp -d)"; local _cleanup="rm -rf '$work'"; trap -- "$_cleanup" RETURN
  archive="$work/protoc.zip"
  log "re-downloading $ARCHIVE_URL to verify archive checksum"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error --location -o "$archive" "$ARCHIVE_URL"
  else
    wget --quiet -O "$archive" "$ARCHIVE_URL"
  fi
  local got_archive got_protoc
  got_archive="$(sha256_file "$archive" | awk '{print $1}')"
  [[ "$got_archive" == "$archive_sha" ]] || die "protoc archive checksum mismatch: got $got_archive want $archive_sha"

  # protoc binary: build if absent, refuse (no silent update) if present and
  # mismatched, reuse if already correct.
  if [[ -f "$DEST_DIR/bin/protoc" ]]; then
    got_protoc="$(sha256_file "$DEST_DIR/bin/protoc" | awk '{print $1}')"
    [[ "$got_protoc" == "$protoc_sha" ]] || die "protoc binary present but checksum mismatch: got $got_protoc want $protoc_sha (remove $DEST_DIR/bin/protoc and re-run, or bootstrap again)"
  else
    log "installing protoc binary"
    unzip -o -q "$archive" -d "$work/protoc-extract"
    cp "$work/protoc-extract/bin/protoc" "$DEST_DIR/bin/protoc"
    mkdir -p "$DEST_DIR/include/google/protobuf"
    cp -R "$work/protoc-extract/include/google/protobuf/." "$DEST_DIR/include/google/protobuf/"
    got_protoc="$(sha256_file "$DEST_DIR/bin/protoc" | awk '{print $1}')"
    [[ "$got_protoc" == "$protoc_sha" ]] || die "built protoc checksum mismatch: got $got_protoc want $protoc_sha"
  fi

  # Plugins: same policy - build if absent, refuse if present and mismatched,
  # reuse if correct.
  install_plugin google.golang.org/protobuf/cmd/protoc-gen-go "$PROTOC_GEN_GO_VERSION" protoc-gen-go "$go_sha"
  install_plugin google.golang.org/grpc/cmd/protoc-gen-go-grpc "$PROTOC_GEN_GO_GRPC_VERSION" protoc-gen-go-grpc "$grpc_sha"

  verify_versions
  log "install OK: all checksums verified"
}

# install_plugin builds a Go plugin only when absent, refuses (no silent
# update) when present but checksum-mismatched, and reuses when already correct.
install_plugin() {
  local module="$1" version="$2" outname="$3" want="$4"
  local target="$DEST_DIR/bin/$outname"
  if [[ -f "$target" ]]; then
    local got
    got="$(sha256_file "$target" | awk '{print $1}')"
    [[ "$got" == "$want" ]] || die "$outname present but checksum mismatch: got $got want $want (remove $target and re-run, or bootstrap again)"
    return
  fi
  log "installing $outname"
  GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go install -trimpath -ldflags=-buildid= "$module@$version"
  local gopath_bin
  gopath_bin="$(GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" GOOS=linux GOARCH=amd64 go env GOPATH)/bin/linux_amd64/$outname"
  [[ -f "$gopath_bin" ]] || gopath_bin="$(GOTOOLCHAIN="$GO_TOOLCHAIN_VERSION" go env GOPATH)/bin/$outname"
  [[ -f "$gopath_bin" ]] || die "built plugin not found: $outname"
  cp "$gopath_bin" "$target"
  local got
  got="$(sha256_file "$target" | awk '{print $1}')"
  [[ "$got" == "$want" ]] || die "built $outname checksum mismatch: got $got want $want"
}

usage() {
  cat >&2 <<EOF
usage: install-protobuf-tools.sh <bootstrap|install>

  bootstrap - build pinned tools and print 4 SHA-256 lines to stdout for review
  install   - verify locked checksums and ensure tools are installed
EOF
}

main() {
  local mode="${1:-}"
  case "$mode" in
    bootstrap) do_bootstrap ;;
    install) ensure_dirs; verify_against_lock ;;
    ""|*) usage; exit 2 ;;
  esac
}

main "$@"
