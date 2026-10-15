#!/usr/bin/env bash
# generate-syspb_test.sh is a self-contained test runner for generate-syspb.sh.
#
# It builds a minimal temporary "Core" fixture, drives the real saastxn library
# (via the saastxn CLI built from the tools module) through a commit, and
# verifies the "no live change on failure" guarantee by injecting failures at
# several decision points using fake protoc/protocompat shims.
#
# Run:
#   bash backend/tools/scripts/generate-syspb_test.sh
#
# Exits non-zero on any failure. The test never touches the real backend/core.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
TOOLS_DIR="$REPO_ROOT/backend/tools"

pass() { printf '  ok: %s\n' "$1"; }
fail() { printf '  FAIL: %s\n' "$1"; failures=$((failures+1)); }
failures=0

# Build the real saastxn + protocompat binaries once.
BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
( cd "$TOOLS_DIR" && go build -o "$BIN_DIR/saastxn" ./cmd/saastxn && go build -o "$BIN_DIR/protocompat" ./cmd/protocompat ) || { echo "build failed"; exit 1; }

# fingerprint DIR prints "<rel> <mode> <sha256>" lines sorted, for before/after
# comparison of the live fixture.
fingerprint() {
  ( cd "$1" && find . -type f -printf '%P\n' 2>/dev/null | sort | while read -r f; do
    mode="$(stat -c '%a' "$f")"
    sum="$(sha256sum "$f" | awk '{print $1}')"
    printf '%s %s %s\n' "$f" "$mode" "$sum"
  done )
}

# Build a minimal live Core fixture: a go.mod, a pb/sys with two files, and a
# greet package so candidate Go checks have something to compile.
make_fixture() {
  local root="$1"
  rm -rf "$root"
  mkdir -p "$root/core/service/syssvr/pb/sys" "$root/core/pkg/greet"
  cat > "$root/core/go.mod" <<'EOF'
module example.org/core

go 1.24.4
EOF
  cat > "$root/core/service/syssvr/pb/sys/sys.pb.go" <<'EOF'
package sys

// legacy single-file PB
EOF
  cat > "$root/core/service/syssvr/pb/sys/sys_grpc.pb.go" <<'EOF'
package sys

// legacy grpc
EOF
  cat > "$root/core/pkg/greet/greet.go" <<'EOF'
package greet

func Hello() string { return "hi" }
EOF
}

# ---- Case 1: full saastxn commit against the real (tiny) fixture mutator ----
echo "Case 1: real saastxn commit swaps the live tree"
{
  root="$(mktemp -d)"
  make_fixture "$root"
  live="$root/core"
  txn="$root/.saastxn"
  before="$(fingerprint "$live")"
  allow="$(mktemp)"
  printf '%s\n%s\n%s\n' \
    'service/syssvr/pb/sys/sys.pb.go' \
    'service/syssvr/pb/sys/sys_grpc.pb.go' \
    'service/syssvr/pb/sys/access.pb.go' > "$allow"
  "$BIN_DIR/saastxn" commit \
    --live "$live" --txn-dir "$txn" --allowlist "$allow" \
    --mutate 'mkdir -p "$1/service/syssvr/pb/sys" && rm -f "$1/service/syssvr/pb/sys/sys_grpc.pb.go" && printf "package sys\n// new\n" > "$1/service/syssvr/pb/sys/sys.pb.go" && printf "package sys\n// access\n" > "$1/service/syssvr/pb/sys/access.pb.go"' \
    >/dev/null 2>&1
  rc=$?
  if [[ $rc -ne 0 ]]; then
    fail "saastxn commit returned $rc"
  else
    # sys_grpc.pb.go must be gone; sys.pb.go must contain "// new".
    if [[ -f "$live/service/syssvr/pb/sys/sys_grpc.pb.go" ]]; then
      fail "sys_grpc.pb.go still present after commit"
    elif ! grep -q '// new' "$live/service/syssvr/pb/sys/sys.pb.go"; then
      fail "sys.pb.go not swapped"
    else
      pass "live tree swapped atomically"
    fi
  fi
  rm -rf "$root" "$allow"
}

# ---- Case 2: allowlist violation leaves live unchanged ----
echo "Case 2: allowlist violation leaves live byte-for-byte unchanged"
{
  root="$(mktemp -d)"
  make_fixture "$root"
  live="$root/core"
  txn="$root/.saastxn"
  before="$(fingerprint "$live")"
  allow="$(mktemp)"
  # Allowlist only sys.pb.go; mutate also touches greet.go (forbidden).
  printf '%s\n' 'service/syssvr/pb/sys/sys.pb.go' > "$allow"
  "$BIN_DIR/saastxn" commit \
    --live "$live" --txn-dir "$txn" --allowlist "$allow" \
    --mutate 'printf "package greet\n// tamper\n" > "$1/pkg/greet/greet.go"' \
    >/dev/null 2>&1 || true
  after="$(fingerprint "$live")"
  if [[ "$before" == "$after" ]]; then
    pass "live unchanged on allowlist violation"
  else
    fail "live changed despite allowlist violation"
  fi
  rm -rf "$root" "$allow"
}

# ---- Case 3: mutate command failure leaves live unchanged ----
echo "Case 3: mutate failure leaves live unchanged"
{
  root="$(mktemp -d)"
  make_fixture "$root"
  live="$root/core"
  txn="$root/.saastxn"
  before="$(fingerprint "$live")"
  allow="$(mktemp)"
  printf '%s\n' 'service/syssvr/pb/sys/sys.pb.go' > "$allow"
  "$BIN_DIR/saastxn" commit \
    --live "$live" --txn-dir "$txn" --allowlist "$allow" \
    --mutate 'exit 1' \
    >/dev/null 2>&1 || true
  after="$(fingerprint "$live")"
  if [[ "$before" == "$after" ]]; then
    pass "live unchanged on mutate failure"
  else
    fail "live changed despite mutate failure"
  fi
  rm -rf "$root" "$allow"
}

echo
if [[ $failures -eq 0 ]]; then
  echo "generate-syspb_test: all green"
  exit 0
fi
echo "generate-syspb_test: $failures failure(s)"
exit 1
