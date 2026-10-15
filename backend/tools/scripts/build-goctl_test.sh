#!/usr/bin/env bash
# build-goctl_test.sh — build-goctl.sh 的契约测试。
#
# 通过在隔离的临时工作区里准备受控的 lock/patches/bin，并把受控 PATH
# 传给被测脚本，逐项注入失败条件，断言：
#   1. 每种失败模式脚本都返回非 0；
#   2. 失败时业务树（fixture 里的 backend/core 等）的路径/模式/内容 SHA-256
#      完全不变；
#   3. 成功时输出只能落在显式 repo root 的 backend/bin/goctl，不能落入
#      fork 临时 cwd。
#
# 本测试不联网：通过 fake git 在临时目录“克隆”一个最小的伪 fork 仓库，
# fake go 产出确定性字节，从而把脚本的每个分支变成纯函数。
#
# 运行：  bash backend/tools/scripts/build-goctl_test.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

###############################################################################
# 颜色 / 断言
###############################################################################
red=$'\033[31m'; green=$'\033[32m'; yellow=$'\033[33m'; reset=$'\033[0m'
pass=0
fail=0
fails=()

ok()   { printf '%sPASS%s %s\n' "$green" "$reset" "$1"; pass=$((pass+1)); }
nok()  { printf '%sFAIL%s %s\n' "$red" "$reset" "$1"; fail=$((fail+1)); fails+=("$1"); }
info() { printf '%s.. %s%s\n' "$yellow" "$reset" "$1"; }

# assert_exit <expected> <desc> <cmd...>
assert_exit() {
  local expect="$1"; shift
  local desc="$1"; shift
  if "$@" >/tmp/bgtest.out 2>/tmp/bgtest.err; then rc=0; else rc=$?; fi
  if [[ "$rc" -eq "$expect" ]]; then ok "$desc (exit=$rc)"; else nok "$desc (expected exit=$expect got=$rc)"; fi
  return 0
}

###############################################################################
# 在临时目录里组装受控的伪 ming-iot 仓库
###############################################################################
setup_fixture() {
  FIX="$(mktemp -d)"
  info "fixture root: $FIX"

  # 伪 ming-iot 结构
  mkdir -p "$FIX/backend/tools/scripts" \
           "$FIX/backend/tools/patches/goctl" \
           "$FIX/backend/bin" \
           "$FIX/backend/core/service"

  # 复制被测脚本
  cp "$HERE/build-goctl.sh" "$FIX/backend/tools/scripts/build-goctl.sh"

  # 业务树 sentinel（任何失败都不得改动）
  echo "core-sentinel-v1" > "$FIX/backend/core/service/sentinel.go"
  echo "go.work-sentinel" > "$FIX/backend/go.work"

  # 伪 fork 源仓库：一个 bare 仓库，含固定 commit + 可被 apply 的目标文件。
  FORK_SRC="$FIX/.fake-upstream.git"
  git init --quiet --bare "$FORK_SRC"

  # 临时 non-bare 仓库造 commit，再 push 到 bare
  local seed="$(mktemp -d)"
  git -C "$seed" init --quiet
  git -C "$seed" config user.email t@t.t
  git -C "$seed" config user.name t
  mkdir -p "$seed/tools/goctl/api/spec"
  cat >"$seed/tools/goctl/api/spec/validate.go" <<'GO'
package spec

// ValidAuthTypes mirrors the fork field for our fake patch target.
var ValidAuthTypes = map[string]bool{
	"platform": true,
	"admin":    true,
	"all":      true,
	"public":   true,
	"device":   true,
}
GO
  mkdir -p "$seed/tools/goctl"
  cat >"$seed/tools/goctl/go.mod" <<'GOMOD'
module gitee.com/unitedrhino/thingsctl-goctl-fake

go 1.24.4
GOMOD
  # 最小 main 包让 fake go build 能命中 .
  cat >"$seed/tools/goctl/main.go" <<'GO'
package main

func main() {}
GO
  git -C "$seed" add -A
  git -C "$seed" commit --quiet -m "seed"
  FAKE_COMMIT="$(git -C "$seed" rev-parse HEAD)"
  git -C "$seed" remote add origin "$FORK_SRC"
  git -C "$seed" push --quiet origin HEAD:master

  rm -rf "$seed"

  # 通过在一份干净 clone 上做真实修改、git diff 生成补丁，保证 hunk header 与
  # 种子内容完全匹配、可干净 apply。
  local patchwork="$FIX/.patch-seed"
  command /usr/bin/git clone --quiet "$FORK_SRC" "$patchwork"
  command /usr/bin/git -C "$patchwork" checkout --quiet --detach "$FAKE_COMMIT"
  command /usr/bin/git -C "$patchwork" config user.email t@t.t
  command /usr/bin/git -C "$patchwork" config user.name t
  # 在 validate.go 末尾大括号前插入一行 supper（用 sed 精确替换）。
  sed -i '/"device":   true,/a\
\t"supper":   true,' "$patchwork/tools/goctl/api/spec/validate.go"
  command /usr/bin/git -C "$patchwork" add -A
  command /usr/bin/git -C "$patchwork" diff --cached > "$FIX/backend/tools/patches/goctl/0001-api-accept-supper.patch"
  rm -rf "$patchwork"

  # 若生成的 patch 为空（sed 没命中），整个测试无意义，立刻失败。
  if [[ ! -s "$FIX/backend/tools/patches/goctl/0001-api-accept-supper.patch" ]]; then
    echo "FATAL: test patch generation produced empty diff" >&2
    exit 99
  fi
  PATCH_SHA="$(sha256sum "$FIX/backend/tools/patches/goctl/0001-api-accept-supper.patch" | awk '{print $1}')"

  # 计算 fake source archive sha（在 build 调用前由脚本计算，测试用同公式独立复核）
  # fake go 产出的 binary 固定内容
  BINARY_CONTENT="FAKE-GOCTL-BINARY-v1"
  BINARY_SHA="$(printf '%s' "$BINARY_CONTENT" | sha256sum | awk '{print $1}')"

  # fake bin 目录里准备一个 sha256 文件表，供 fake go 在 build 时写入
  mkdir -p "$FIX/.fake-runtime"
  printf '%s' "$BINARY_CONTENT" > "$FIX/.fake-runtime/binary.dat"
  echo "$BINARY_SHA" > "$FIX/.fake-runtime/binary.sha256"

  # 默认 lock 骨架（build mode 缺 checksum，应失败）
  write_lock_skeleton "$FIX"
}

write_lock_skeleton() {
  local root="$1"
  cat >"$root/backend/tools/goctl.lock" <<LOCK
repository: $FORK_SRC
commit: $FAKE_COMMIT
goVersion: go1.24.4
target:
  goos: linux
  goarch: amd64
  cgoEnabled: false
patches:
  - path: patches/goctl/0001-api-accept-supper.patch
LOCK
}

# 在 lock 里补上完整 checksum（用于成功 / checksum 校验类用例）
write_lock_full() {
  local root="$1" source_sha="$2" binary_sha="$3"
  cat >"$root/backend/tools/goctl.lock" <<LOCK
repository: $FORK_SRC
commit: $FAKE_COMMIT
goVersion: go1.24.4
target:
  goos: linux
  goarch: amd64
  cgoEnabled: false
patches:
  - path: patches/goctl/0001-api-accept-supper.patch
    sha256: $PATCH_SHA
sourceArchiveSha256: $source_sha
binarySha256: $binary_sha
LOCK
}

###############################################################################
# fake 工具链：放进临时 PATH，让被测脚本不联网即可全流程跑通
###############################################################################
# 环境变量控制 fake 行为：
#   FAKE_UPSTREAM       伪 fork bare 仓库路径
#   FAKE_COMMIT         固定 commit
#   FAKE_GOVERSION      要求的 go 版本（用于版本不匹配用例）
#   FAKE_REQUIRE_GO     若为 1，go 命令强制失败
#   FAKE_TEST_FAIL      若为 1，go test 强制失败
#   FAKE_OUT_DIR        build 输出目录（repo_root/backend/bin）
#   FAKE_BINARY_SRC     fake go build 复制的字节源
#   FAKE_SOURCE_SHA     fake git archive + sha256sum 预期源 archive sha
#   FAKE_BINARY_SHA     fake go build 写出后的预期 binary sha（可被改写制造不一致）
#   FAKE_APPLY_FAIL     若为 1，git apply --check 强制失败
#   FAKE_HEAD_OVERRIDE  git rev-parse HEAD 返回的覆盖值（制造 commit 不匹配）
make_fakes() {
  FAKES="$(mktemp -d)"
  mkdir -p "$FAKES"

  # fake git：把所有真实操作委托给 /usr/bin/git（保留 -C/--no-checkout 等全部
  # 原参数），只在三个注入点上改写行为：
  #   * rev-parse HEAD         可被 FAKE_HEAD_OVERRIDE 覆盖（commit 不匹配用例）
  #   * rev-parse --show-toplevel  返回调用方传入的 REPO_ROOT
  #   * apply --check          可被 FAKE_APPLY_FAIL=1 强制失败
  cat >"$FAKES/git" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

# 把前置的 -C <path> 解析出来（被测脚本多处用 git -C fork ...）。
unset GITCDIR
args=("$@")
i=0
rest=()
while [[ $i -lt ${#args[@]} ]]; do
  a="${args[$i]}"
  if [[ "$a" == "-C" ]]; then
    GITCDIR="${args[$((i+1))]}"
    i=$((i+2))
  else
    rest+=("$a")
    i=$((i+1))
  fi
done

with_cd() {
  if [[ -n "${GITCDIR:-}" ]]; then echo -C "$GITCDIR"; fi
}

# 取首个非 flag 子命令
sub="${rest[0]:-}"

case "$sub" in
  rev-parse)
    # 第二参数（跳过已剥离的 -C）
    op="${rest[1]:-}"
    if [[ "$op" == "--show-toplevel" ]]; then
      echo "${REPO_ROOT:?}"
      exit 0
    elif [[ "$op" == "HEAD" ]]; then
      echo "${FAKE_HEAD_OVERRIDE:-${FAKE_COMMIT:?}}"
      exit 0
    fi
    ;;
  apply)
    if [[ "${rest[1]:-}" == "--check" ]]; then
      if [[ "${FAKE_APPLY_FAIL:-0}" == "1" ]]; then exit 1; fi
    fi
    # --check 通过则落到真 git 做实际校验；apply 也落到真 git
    ;;
esac

command /usr/bin/git $(with_cd) "${rest[@]}"
SH
  chmod +x "$FAKES/git"

  # fake go：实现 build / test，版本检查由 GOTOOLCHAIN 体现
  cat >"$FAKES/go" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
sub="${1:-}"
case "$sub" in
  build)
    if [[ "${FAKE_REQUIRE_GO:-0}" == "1" ]]; then exit 1; fi
    # 解析 -o 输出路径
    out=""
    prev=""
    for a in "$@"; do
      if [[ "$prev" == "-o" ]]; then out="$a"; fi
      prev="$a"
    done
    [[ -n "$out" ]]
    mkdir -p "$(dirname "$out")"
    cp "${FAKE_BINARY_SRC:?}" "$out"
    ;;
  test)
    if [[ "${FAKE_TEST_FAIL:-0}" == "1" ]]; then exit 1; fi
    exit 0
    ;;
  *)
    echo "fake go: unsupported $*" >&2
    exit 1
    ;;
esac
SH
  chmod +x "$FAKES/go"

  # 真 sha256sum 继续用系统的（结果稳定）
}

# 受控环境：跑被测脚本
# 用法： run_script <mode> <env-setting-args...>
run_script() {
  local mode="$1"; shift
  env \
    PATH="$FAKES:/usr/bin:/bin" \
    REPO_ROOT="$FIX" \
    FAKE_UPSTREAM="$FORK_SRC" \
    FAKE_COMMIT="$FAKE_COMMIT" \
    FAKE_HEAD_OVERRIDE="${FAKE_HEAD_OVERRIDE:-}" \
    FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero" \
    FAKE_BINARY_SRC="$FIX/.fake-runtime/binary.dat" \
    FAKE_REQUIRE_GO="${FAKE_REQUIRE_GO:-0}" \
    FAKE_TEST_FAIL="${FAKE_TEST_FAIL:-0}" \
    FAKE_APPLY_FAIL="${FAKE_APPLY_FAIL:-0}" \
    bash "$FIX/backend/tools/scripts/build-goctl.sh" "$mode"
}

# snapshot：对业务树核心文件取 sha + mode，返回文本
snapshot_business_tree() {
  (
    cd "$FIX/backend"
    for f in core/service/sentinel.go go.work; do
      if [[ -e "$f" ]]; then
        printf '%s\t%o\t%s\n' "$(sha256sum "$f" | awk '{print $1}')" "$(stat -c %a "$f")" "$f"
      fi
    done
    # bin 目录下除 goctl 外不得有其它东西；goctl 内容随用例变化
    if [[ -d bin ]]; then
      while IFS= read -r -d '' p; do
        rel="${p#$FIX/backend/}"
        printf '%s\t%o\t%s\n' "$(sha256sum "$p" | awk '{print $1}')" "$(stat -c %a "$p")" "$rel"
      done < <(find bin -type f -print0)
    fi
  )
}

###############################################################################
# 用例
###############################################################################
main() {
  info "setting up fixture"
  setup_fixture
  make_fakes

  # 默认 lock 是 skeleton（无 checksum）。先验证 build mode 在缺少 checksum 时失败。
  info "CASE: build mode must fail when checksums missing"
  before="$(snapshot_business_tree)"
  assert_exit 1 "build fails without checksums" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on build-missing-checksum"; else nok "business tree changed on build-missing-checksum"; fi

  # 未知 mode
  info "CASE: unknown mode exits 64"
  assert_exit 64 "unknown mode rejected" env PATH="$FAKES:/usr/bin:/bin" bash "$FIX/backend/tools/scripts/build-goctl.sh" nope

  # bootstrap 成功路径：先跑一次拿到 source/binary sha
  info "CASE: bootstrap success prints checksums, writes backend/bin/goctl"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  # 真实 clone + checkout 模拟：fake git clone 用了真 git clone，但 checkout/apply 依赖 FAKE_WORKTREE
  # 为让 fake apply 工作，先在 FAKE_WORKTREE 做一次真 clone + checkout，保持和 fake clone 一致
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  boot_out="$(run_script bootstrap 2>/tmp/boot.err || true)"
  echo "$boot_out" | sed 's/^/  bootstrap> /' >&2
  # 应该看到 patch / sourceArchive / binary 三类输出
  if echo "$boot_out" | grep -q '^patch	'; then ok "bootstrap prints patch checksum"; else nok "bootstrap missing patch checksum"; fi
  if echo "$boot_out" | grep -q '^sourceArchive	'; then ok "bootstrap prints sourceArchive checksum"; else nok "bootstrap missing sourceArchive checksum"; fi
  if echo "$boot_out" | grep -q '^binary	'; then ok "bootstrap prints binary checksum"; else nok "bootstrap missing binary checksum"; fi
  SOURCE_SHA="$(echo "$boot_out" | awk -F'\t' '$1=="sourceArchive"{print $2}')"
  BIN_SHA="$(echo "$boot_out" | awk -F'\t' '$1=="binary"{print $2}')"
  [[ -n "$SOURCE_SHA" ]] || { nok "bootstrap empty sourceArchive sha"; SOURCE_SHA="<empty>"; }
  [[ -n "$BIN_SHA" ]]    || { nok "bootstrap empty binary sha"; BIN_SHA="<empty>"; }

  # goctl 必须落到 repo_root/backend/bin/goctl，而不是 fork cwd
  if [[ -f "$FIX/backend/bin/goctl" ]]; then ok "goctl written to repo_root/backend/bin/goctl"; else nok "goctl not in repo_root/backend/bin"; fi

  # bootstrap 确定性：再跑一次，stdout diff 必须为空
  info "CASE: bootstrap deterministic (two runs identical stdout)"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  boot_out2="$(run_script bootstrap 2>/dev/null || true)"
  if [[ "$boot_out" == "$boot_out2" ]]; then ok "bootstrap two-run diff empty"; else nok "bootstrap two-run diff differs"; fi

  # 写完整 lock，build mode 应成功
  info "CASE: build success with full lock"
  write_lock_full "$FIX" "$SOURCE_SHA" "$BIN_SHA"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  assert_exit 0 "build succeeds with correct checksums" run_script build

  # commit 不匹配：让 fake git rev-parse HEAD 返回别的值
  info "CASE: commit mismatch fails"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  FAKE_HEAD_OVERRIDE="0123456789abcdef0123456789abcdef01234567" \
    assert_exit 1 "commit mismatch rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on commit mismatch"; else nok "business tree changed on commit mismatch"; fi

  # patch 文件缺失
  info "CASE: missing patch file fails"
  write_lock_full "$FIX" "$SOURCE_SHA" "$BIN_SHA"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  mv "$FIX/backend/tools/patches/goctl/0001-api-accept-supper.patch" /tmp/bgtest.patch.bak
  before="$(snapshot_business_tree)"
  assert_exit 1 "missing patch rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on missing patch"; else nok "business tree changed on missing patch"; fi
  mv /tmp/bgtest.patch.bak "$FIX/backend/tools/patches/goctl/0001-api-accept-supper.patch"

  # git apply --check 失败
  info "CASE: git apply --check failure rejected"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  FAKE_APPLY_FAIL=1 assert_exit 1 "git apply --check failure rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on apply-check fail"; else nok "business tree changed on apply-check fail"; fi

  # fork tests 失败
  info "CASE: fork test failure rejected"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  FAKE_TEST_FAIL=1 assert_exit 1 "fork test failure rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on fork test fail"; else nok "business tree changed on fork test fail"; fi

  # Go build 失败
  info "CASE: go build failure rejected"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  FAKE_REQUIRE_GO=1 assert_exit 1 "go build failure rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on go build fail"; else nok "business tree changed on go build fail"; fi

  # source archive checksum 不符
  info "CASE: sourceArchiveSha256 mismatch rejected"
  write_lock_full "$FIX" "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef" "$BIN_SHA"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  assert_exit 1 "sourceArchiveSha256 mismatch rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on sourceArchive mismatch"; else nok "business tree changed on sourceArchive mismatch"; fi

  # binary checksum 不符
  info "CASE: binarySha256 mismatch rejected"
  write_lock_full "$FIX" "$SOURCE_SHA" "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  assert_exit 1 "binarySha256 mismatch rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on binarySha mismatch"; else nok "business tree changed on binarySha mismatch"; fi

  # patch sha256 不符
  info "CASE: patch sha256 mismatch rejected"
  cat >"$FIX/backend/tools/goctl.lock" <<LOCK
repository: $FORK_SRC
commit: $FAKE_COMMIT
goVersion: go1.24.4
target:
  goos: linux
  goarch: amd64
  cgoEnabled: false
patches:
  - path: patches/goctl/0001-api-accept-supper.patch
    sha256: deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
sourceArchiveSha256: $SOURCE_SHA
binarySha256: $BIN_SHA
LOCK
  FAKE_WORKTREE="$FIX/backend/.goctl-patch-work/current/go-zero"
  rm -rf "$FAKE_WORKTREE"; mkdir -p "$(dirname "$FAKE_WORKTREE")"
  command /usr/bin/git clone --quiet --no-checkout "$FORK_SRC" "$FAKE_WORKTREE"
  command /usr/bin/git -C "$FAKE_WORKTREE" checkout --quiet --detach "$FAKE_COMMIT"
  before="$(snapshot_business_tree)"
  assert_exit 1 "patch sha256 mismatch rejected" run_script build
  after="$(snapshot_business_tree)"
  if [[ "$before" == "$after" ]]; then ok "business tree unchanged on patch sha mismatch"; else nok "business tree changed on patch sha mismatch"; fi

  ###############################################################################
  echo
  printf 'result: %s pass, %s fail\n' "$pass" "$fail"
  if [[ "$fail" -gt 0 ]]; then
    printf '%s\n' "${fails[@]}"
    exit 1
  fi
  exit 0
}

main "$@"
