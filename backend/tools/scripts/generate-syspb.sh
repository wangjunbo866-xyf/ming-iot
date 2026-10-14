#!/usr/bin/env bash
# generate-syspb.sh regenerates the syssvr PB layer (pb/sys/*.pb.go) from the
# split proto sources using the hermetic protobuf toolchain, and atomically
# swaps the new files into the live backend/saas tree via saastxn.
#
# The script never mutates the live tree directly: it delegates the candidate
# build to generate-syspb-candidate.sh, and saastxn performs the allowlist
# re-check + atomic directory exchange. Failures during candidate build or
# compatibility leave the live tree byte-for-byte unchanged.
#
# Prerequisites:
#   - bash backend/tools/scripts/install-protobuf-tools.sh install
#   - the golden descriptor at backend/saas/service/syssvr/proto/testdata/sys-v1.protoset

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
BACKEND_DIR="$REPO_ROOT/backend"
CORE_DIR="$BACKEND_DIR/saas"
TOOLS_DIR="$BACKEND_DIR/tools"
TXN_DIR="$BACKEND_DIR/.saastxn"
PBTOOLS_DIR="$BACKEND_DIR/bin/protobuf-tools"
CANDIDATE_SCRIPT="$SCRIPT_DIR/generate-syspb-candidate.sh"

log() { printf '[generate-syspb] %s\n' "$*" >&2; }
die() { printf '[generate-syspb] ERROR: %s\n' "$*" >&2; exit 1; }

preflight() {
  [[ -x "$PBTOOLS_DIR/bin/protoc" ]] || die "missing hermetic protoc; run: bash backend/tools/scripts/install-protobuf-tools.sh install"
  [[ -x "$PBTOOLS_DIR/bin/protoc-gen-go" ]] || die "missing hermetic protoc-gen-go"
  [[ -x "$PBTOOLS_DIR/bin/protoc-gen-go-grpc" ]] || die "missing hermetic protoc-gen-go-grpc"
  local got
  got="$("$PBTOOLS_DIR/bin/protoc" --version)"
  [[ "$got" == "libprotoc 34.1" ]] || die "hermetic protoc version mismatch: '$got' (want libprotoc 34.1)"
  [[ -x "$CANDIDATE_SCRIPT" ]] || die "missing candidate script: $CANDIDATE_SCRIPT"
}

main() {
  preflight

  # Build the protocompat + saastxn CLIs from the tools module into a scratch
  # .bin dir (gitignored). The candidate script reads PROTOCOMPAT_BIN.
  local bin_dir="$TOOLS_DIR/.bin"
  mkdir -p "$bin_dir"
  local saastxn_bin="$bin_dir/saastxn"
  local protocompat_bin="$bin_dir/protocompat"
  ( cd "$TOOLS_DIR" && go build -o "$saastxn_bin" ./cmd/saastxn && go build -o "$protocompat_bin" ./cmd/protocompat ) || die "failed to build saastxn/protocompat"
  # Expand the path into the trap body now so EXIT cleanup does not depend on
  # the local going out of scope under set -u.
  trap "rm -rf '$bin_dir'" EXIT

  # Allowlist: the 33 pb/sys paths that may change (32 new files + the deleted
  # sys_grpc.pb.go). saastxn aborts if any other live path differs.
  local allowlist
  allowlist="$(mktemp)"
  {
    for f in \
      base.pb.go \
      access.pb.go access_grpc.pb.go \
      app.pb.go app_grpc.pb.go \
      area.pb.go area_grpc.pb.go \
      common.pb.go common_grpc.pb.go \
      data.pb.go data_grpc.pb.go \
      department.pb.go department_grpc.pb.go \
      dict.pb.go dict_grpc.pb.go \
      log.pb.go log_grpc.pb.go \
      module.pb.go module_grpc.pb.go \
      notify.pb.go notify_grpc.pb.go \
      ops.pb.go ops_grpc.pb.go \
      project.pb.go project_grpc.pb.go \
      role.pb.go role_grpc.pb.go \
      tenant.pb.go tenant_grpc.pb.go \
      user.pb.go user_grpc.pb.go \
      sys.pb.go \
      sys_grpc.pb.go ; do
      echo "service/syssvr/pb/sys/$f"
    done
  } > "$allowlist"

  # Export the env vars the candidate script reads.
  export PBTOOLS_DIR TOOLS_DIR BACKEND_DIR
  export PROTOCOMPAT_BIN="$protocompat_bin"

  log "committing via saastxn (live=$CORE_DIR, txn=$TXN_DIR)"
  "$saastxn_bin" commit \
    --live "$CORE_DIR" \
    --txn-dir "$TXN_DIR" \
    --allowlist "$allowlist" \
    --mutate "$CANDIDATE_SCRIPT \"\$1\""

  rm -f "$allowlist"
  log "PB regeneration committed"
}

main "$@"
