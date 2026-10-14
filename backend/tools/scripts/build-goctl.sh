#!/usr/bin/env bash
# build-goctl.sh — 固定联犀 goctl fork 的可复现构建器。
#
# 用法：
#   bash backend/tools/scripts/build-goctl.sh bootstrap
#   bash backend/tools/scripts/build-goctl.sh build
#
# 两个 mode 都在临时目录克隆 fork、按序应用 patches、运行定向 tests、
# 构建到 $repo_root/backend/bin/goctl，并计算 source archive 与 binary 的 sha256。
#
# 区别：
#   bootstrap  不读取/校验 goctl.lock 里的 checksum，所有进度写 stderr，
#              stdout 只打印固定顺序的 checksum 行，便于两次运行直接 diff。
#   build      读取 goctl.lock；缺失任一 checksum 字段或任一值不匹配即失败。
#
# 所有输出路径都从 ming-iot 根（脚本位置上溯三级）解析，绝不继承 fork cwd。
set -euo pipefail

###############################################################################
# 参数与路径解析
###############################################################################
mode="${1:-}"
if [[ "$mode" != "bootstrap" && "$mode" != "build" ]]; then
  echo "usage: $0 {bootstrap|build}" >&2
  exit 64
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# 脚本位于 <repo_root>/backend/tools/scripts/，需上溯三级到 repo_root。
repo_root="$(cd "$script_dir/../../.." && pwd)"

# 断言 repo_root 确实是 git 顶层目录，防止从软链接或拷贝目录误执行。
git_toplevel="$(git -C "$repo_root" rev-parse --show-toplevel)"
if [[ "$git_toplevel" != "$repo_root" ]]; then
  echo "error: resolved repo_root=$repo_root != git toplevel=$git_toplevel" >&2
  exit 1
fi

lock_file="$repo_root/backend/tools/goctl.lock"
patches_dir="$repo_root/backend/tools/patches/goctl"
bin_dir="$repo_root/backend/bin"
goctl_out="$bin_dir/goctl"
work_root="$repo_root/backend/.goctl-patch-work"

if [[ ! -f "$lock_file" ]]; then
  echo "error: lock file not found: $lock_file" >&2
  exit 1
fi

###############################################################################
# 极简 YAML 读取（只解析本项目固定 schema，避免引入额外依赖）
###############################################################################
# read_lock_field <key>            返回顶层标量值（exit 1 if missing）
# read_lock_target <key>           返回 target.<key> 标量
# read_patch_list                  逐行输出 patch path
read_lock_field() {
  local key="$1"
  # 匹配顶层形如 "key: value" 的行（以两个空格或 tab 开头的属于嵌套 map，跳过）。
  awk -v k="$key" '
    $0 ~ "^"k":[ ]+" {
      # 取冒号后内容并 trim
      val=$0; sub("^"k":[ ]*","",val); sub(/[ \t]+$/,"",val)
      print val
      found=1
    }
    END{ if(!found) exit 1 }
  ' "$lock_file"
}

read_lock_target() {
  local key="$1"
  awk -v k="$key" '
    /^[Tt]arget:[ ]*$/ { intarget=1; next }
    /^[A-Za-z]/ && $0 !~ /^[Tt]arget:/ { if(intarget) intarget=0 }
    intarget && $0 ~ "^  "k":[ ]+" {
      val=$0; sub("^  "k":[ ]*","",val); sub(/[ \t]+$/,"",val)
      print val
      exit 0
    }
  ' "$lock_file"
  return 0
}

# 读取某个 patch path 在 lock 里关联的 sha256（紧跟 path: 的同级 sha256: 行）。
read_patch_sha256() {
  local path="$1"
  awk -v p="$path" '
    $0 ~ "^  - path:[ ]+"p"[ ]*$" { want=1; next }
    want && $0 ~ "^    sha256:[ ]+" {
      val=$0; sub("^    sha256:[ ]*","",val); sub(/[ \t]+$/,"",val)
      print val; exit 0
    }
    want && $0 ~ "^  - path:" { want=0 }
  ' "$lock_file"
}

repository="$(read_lock_field repository)"
commit="$(read_lock_field commit)"
go_version="$(read_lock_field goVersion)"
goos="$(read_lock_target goos)"
goarch="$(read_lock_target goarch)"
cgo_str="$(read_lock_target cgoEnabled)"
if [[ "$cgo_str" == "true" ]]; then cgo_enabled=1; else cgo_enabled=0; fi
lock_source_sha="$(read_lock_field sourceArchiveSha256 || true)"
lock_binary_sha="$(read_lock_field binarySha256 || true)"

if [[ -z "$repository" || -z "$commit" || -z "$go_version" \
      || -z "$goos" || -z "$goarch" ]]; then
  echo "error: lock file missing required top-level/target fields" >&2
  exit 1
fi

mapfile -t patch_paths < <(awk '
  /^patches:[ ]*$/ { inp=1; next }
  inp && /^[A-Za-z]/ { inp=0 }
  inp && $0 ~ /^  - path:[ ]+/ {
    val=$0; sub(/^  - path:[ ]*/,"",val); sub(/[ \t]+$/,"",val); print val
  }
' "$lock_file")

###############################################################################
# build mode 结构性预检：任何 checksum 字段缺失/空，必须在 clone/build 前
# 失败，保证失败 build 不在 repo_root/backend/bin 留下二进制。值的真实匹配
# 在构建完成后做（构建是确定性的）。
###############################################################################
if [[ "$mode" == "build" ]]; then
  precheck_fail=0
  for p in "${patch_paths[@]}"; do
    s="$(read_patch_sha256 "$p")"
    if [[ -z "$s" ]]; then
      echo "error: lock missing sha256 for patch $p" >&2
      precheck_fail=1
    fi
  done
  if [[ -z "$lock_source_sha" ]]; then
    echo "error: lock missing sourceArchiveSha256" >&2
    precheck_fail=1
  fi
  if [[ -z "$lock_binary_sha" ]]; then
    echo "error: lock missing binarySha256" >&2
    precheck_fail=1
  fi
  if [[ "$precheck_fail" -ne 0 ]]; then
    echo "[goctl] build rejected: lock checksums incomplete" >&2
    exit 1
  fi
fi

###############################################################################
# 工具探测
###############################################################################
need_tool() {
  command -v "$1" >/dev/null 2>&1 || { echo "error: required tool missing: $1" >&2; exit 127; }
}
need_tool git
need_tool sha256sum

###############################################################################
# bootstrap_dir：在工作根下建临时 fork clone 目录
###############################################################################
bootstrap_id="build-$$-$(date +%s)-$RANDOM"
fork_dir="$work_root/$bootstrap_id/go-zero"
mkdir -p "$(dirname "$fork_dir")"

cleanup() {
  rm -rf "$work_root/$bootstrap_id" >&2 2>&1 || true
}
trap cleanup EXIT

echo "[goctl] clone fork (mode=$mode)" >&2
git clone --quiet --no-checkout "$repository" "$fork_dir" >&2
git -C "$fork_dir" checkout --quiet --detach "$commit" >&2

actual_head="$(git -C "$fork_dir" rev-parse HEAD)"
if [[ "$actual_head" != "$commit" ]]; then
  echo "error: checked out HEAD $actual_head != lock commit $commit" >&2
  exit 1
fi

###############################################################################
# 按序应用补丁
###############################################################################
echo "[goctl] apply ${#patch_paths[@]} patches" >&2
for p in "${patch_paths[@]}"; do
  abs_patch="$repo_root/backend/tools/$p"
  if [[ ! -f "$abs_patch" ]]; then
    echo "error: patch file missing: $abs_patch" >&2
    exit 1
  fi
  # bootstrap 也计算并打印每个 patch 的 sha256（落到 stdout），用于 review。
  if ! git -C "$fork_dir" apply --check "$abs_patch" >&2 2>&1; then
    echo "error: git apply --check failed for $p" >&2
    git -C "$fork_dir" apply --check "$abs_patch" >&2 || true
    exit 1
  fi
  git -C "$fork_dir" apply --quiet "$abs_patch" >&2
done

###############################################################################
# 校验 Go toolchain 版本
###############################################################################
# GOTOOLCHAIN=go1.24.4 让 Go 自动拉取并使用指定 toolchain；本机 go1.25.4 在
# GOTOOLCHAIN=auto 下也会按 go.mod 声明拉取。这里显式固定。
want_toolchain="$go_version"
echo "[goctl] use toolchain $want_toolchain" >&2

###############################################################################
# 隔离 fork 构建/测试与上层 ming-iot go.work
###############################################################################
# fork clone 落在 backend/.goctl-patch-work/，处于 ming-iot go.work 覆盖范围。
# goctl 在生成代码时会调用 util/ctx.UpdateGoWorkIfExist 执行 `go work use .`
# （rpc/generator、api/gogen 测试都会触发）。若继承上层 GOWORK，该命令会把
# 测试临时目录写进 ming-iot 的 go.work，污染并破坏后续构建。patch 0004 已把
# UpdateGoWorkIfExist 修成 GOWORK=off 时的真正 no-op；这里给 fork 的全部
# test/build 子进程显式 GOWORK=off，使构建完全脱离 workspace、可复现且零污染。
export GOWORK=off

###############################################################################
# 运行定向 tests（bootstrap 与 build 都跑，保证补丁顺序可用）
###############################################################################
echo "[goctl] run focused fork tests" >&2
(
  cd "$fork_dir/tools/goctl"
  GOTOOLCHAIN="$want_toolchain" CGO_ENABLED="$cgo_enabled" \
    go test ./api/spec ./api/gogen ./api/new ./api/apigen \
      ./api/swagger ./api/access ./rpc/generator >&2
)

###############################################################################
# 构建到 repo_root/backend/bin/goctl
###############################################################################
mkdir -p "$bin_dir"
echo "[goctl] build goctl to $goctl_out" >&2
(
  cd "$fork_dir/tools/goctl"
  GOTOOLCHAIN="$want_toolchain" \
    CGO_ENABLED="$cgo_enabled" GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -buildvcs=false -ldflags=-buildid= \
      -o "$goctl_out" .
)

###############################################################################
# 计算 checksum
###############################################################################
# source archive：未压缩 tar，固定 prefix，与 patches 无关。
source_archive_tar="$work_root/$bootstrap_id/source.tar"
git -C "$fork_dir" archive --prefix=go-zero/ "$commit" -o "$source_archive_tar" >&2
source_sha="$(sha256sum "$source_archive_tar" | awk '{print $1}')"
binary_sha="$(sha256sum "$goctl_out" | awk '{print $1}')"

# 收集每个 patch 的 sha256（按 lock 顺序）
patch_shas=()
for p in "${patch_paths[@]}"; do
  abs_patch="$repo_root/backend/tools/$p"
  patch_shas+=("$(sha256sum "$abs_patch" | awk '{print $1}')")
done

###############################################################################
# 输出 / 校验
###############################################################################
print_checksums() {
  # 固定顺序，便于 diff。tag\tvalue 两列。
  for i in "${!patch_paths[@]}"; do
    printf 'patch\t%s\t%s\n' "${patch_paths[$i]}" "${patch_shas[$i]}"
  done
  printf 'sourceArchive\t%s\n' "$source_sha"
  printf 'binary\t%s\n' "$binary_sha"
}

if [[ "$mode" == "bootstrap" ]]; then
  print_checksums
  # 进度：实际 fork HEAD 与构建目标
  echo "[goctl] fork HEAD=$actual_head goctl=$goctl_out" >&2
  exit 0
fi

# build mode：逐项校验
fail=0
for i in "${!patch_paths[@]}"; do
  p="${patch_paths[$i]}"
  actual="${patch_shas[$i]}"
  want="$(read_patch_sha256 "$p")"
  if [[ -z "$want" ]]; then
    echo "error: lock missing sha256 for patch $p" >&2
    fail=1
  elif [[ "$want" != "$actual" ]]; then
    echo "error: patch sha256 mismatch for $p: lock=$want actual=$actual" >&2
    fail=1
  fi
done
if [[ -z "$lock_source_sha" ]]; then
  echo "error: lock missing sourceArchiveSha256" >&2
  fail=1
elif [[ "$lock_source_sha" != "$source_sha" ]]; then
  echo "error: sourceArchiveSha256 mismatch: lock=$lock_source_sha actual=$source_sha" >&2
  fail=1
fi
if [[ -z "$lock_binary_sha" ]]; then
  echo "error: lock missing binarySha256" >&2
  fail=1
elif [[ "$lock_binary_sha" != "$binary_sha" ]]; then
  echo "error: binarySha256 mismatch: lock=$lock_binary_sha actual=$binary_sha" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  echo "[goctl] build verification FAILED" >&2
  exit 1
fi

print_checksums >&2
echo "[goctl] build OK: $goctl_out (sha256=$binary_sha)" >&2
