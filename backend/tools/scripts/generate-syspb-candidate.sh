#!/usr/bin/env bash
# generate-syspb-candidate.sh is the "mutate" step invoked by saastxn on a full
# copy of backend/saas. It regenerates pb/sys/*.pb.go from the split proto
# sources using the hermetic toolchain, runs the compatibility gate, installs
# exactly the allowlisted files, and runs candidate Go checks with GOWORK=off.
#
# Invocation: generate-syspb-candidate.sh <candidate-dir>
# Environment (set by the parent generate-syspb.sh):
#   PBTOOLS_DIR, TOOLS_DIR, BACKEND_DIR
set -euo pipefail

CANDIDATE="${1:?candidate dir required}"
cd "$CANDIDATE"

PBTOOLS_DIR="${PBTOOLS_DIR:?PBTOOLS_DIR required}"
TOOLS_DIR="${TOOLS_DIR:?TOOLS_DIR required}"
PROTOCOMPAT_BIN="${PROTOCOMPAT_BIN:?PROTOCOMPAT_BIN required}"
PROTO_DIR="$CANDIDATE/service/syssvr/proto"
PB_DIR="$CANDIDATE/service/syssvr/pb/sys"
TESTDATA_DIR="$PROTO_DIR/testdata"

log() { printf '[generate-syspb-candidate] %s\n' "$*" >&2; }
die() { printf '[generate-syspb-candidate] ERROR: %s\n' "$*" >&2; exit 1; }

PROTO_INPUTS=(
  base.proto access.proto app.proto area.proto common.proto data.proto
  department.proto dict.proto log.proto module.proto notify.proto ops.proto
  project.proto role.proto tenant.proto user.proto sys.proto
)

EXPECTED_FILES=(
  base.pb.go
  access.pb.go access_grpc.pb.go
  app.pb.go app_grpc.pb.go
  area.pb.go area_grpc.pb.go
  common.pb.go common_grpc.pb.go
  data.pb.go data_grpc.pb.go
  department.pb.go department_grpc.pb.go
  dict.pb.go dict_grpc.pb.go
  log.pb.go log_grpc.pb.go
  module.pb.go module_grpc.pb.go
  notify.pb.go notify_grpc.pb.go
  ops.pb.go ops_grpc.pb.go
  project.pb.go project_grpc.pb.go
  role.pb.go role_grpc.pb.go
  tenant.pb.go tenant_grpc.pb.go
  user.pb.go user_grpc.pb.go
  sys.pb.go
)

gen_dir="$(mktemp -d)"
trap 'rm -rf "$gen_dir"' EXIT

# Resolve proto inputs to absolute paths under the candidate.
input_paths=()
for p in "${PROTO_INPUTS[@]}"; do
  input_paths+=("$PROTO_DIR/$p")
done

# 1. Generate a descriptor set (with imports) for the compatibility check.
log "generating descriptor set"
"$PBTOOLS_DIR/bin/protoc" \
  -I "$PROTO_DIR" -I "$PBTOOLS_DIR/include" \
  --include_imports \
  --descriptor_set_out="$gen_dir/sys.protoset" \
  "${input_paths[@]}"

# 2. Compatibility gate vs golden. Must report 15 services, 215 methods, 256
#    messages (protocompat prints to stdout and exits 0 on compatible).
log "running compatibility gate"
"$PROTOCOMPAT_BIN" \
  -old "$TESTDATA_DIR/sys-v1.protoset" \
  -new "$gen_dir/sys.protoset" \
  -package sys

# 3. Generate Go PB into the temp dir.
log "generating Go PB"
"$PBTOOLS_DIR/bin/protoc" \
  -I "$PROTO_DIR" -I "$PBTOOLS_DIR/include" \
  --go_out="$gen_dir" --go_opt=paths=source_relative \
  --go-grpc_out="$gen_dir" --go-grpc_opt=paths=source_relative \
  "${input_paths[@]}"

# 4. Collect generated basenames and compare to the expected set.
mapfile -t generated < <(find "$gen_dir" -name '*.pb.go' -printf '%f\n' | sort)
expected_sorted="$(printf '%s\n' "${EXPECTED_FILES[@]}" | sort)"
got_sorted="$(printf '%s\n' "${generated[@]}" | sort)"
if [[ "$got_sorted" != "$expected_sorted" ]]; then
  echo "GENERATED FILE SET MISMATCH" >&2
  diff <(echo "$expected_sorted") <(echo "$got_sorted") >&2 || true
  exit 1
fi

# 5. Install the 32 files into pb/sys; remove old sys_grpc.pb.go. Rebuild the
#    generation dir contents wholesale; only this directory changes.
log "installing generated files into pb/sys"
mkdir -p "$PB_DIR"
rm -f "$PB_DIR/sys_grpc.pb.go"
for f in "${generated[@]}"; do
  cp "$gen_dir/$f" "$PB_DIR/$f"
done

# 6. gofmt the directory.
log "gofmt pb/sys"
gofmt -w "$PB_DIR"

# 7. Candidate Go checks with GOWORK=off so the parent workspace is ignored.
log "candidate go checks"
export GOWORK=off
go test ./service/syssvr/client/... ./service/syssvr/internal/server/... ./service/syssvr/sysdirect
go test -run '^$' ./...
log "candidate checks passed"
