#!/usr/bin/env python3
"""检查固定 Git 候选的 Patch 结构与交接快照；退出 0 仍要求语义评审。"""

import argparse
import fnmatch
import hashlib
import json
import os
import posixpath
import re
import subprocess
from pathlib import Path


def git(*args):
    return subprocess.check_output(["git", *args], stderr=subprocess.PIPE)


def repo_path(path):
    if not isinstance(path, str) or not path or path.startswith("/") or "\\" in path:
        raise ValueError(f"非法仓库相对路径：{path!r}")
    if posixpath.normpath(path) != path or path == ".." or path.startswith("../"):
        raise ValueError(f"非法仓库相对路径：{path!r}")
    return path


def read_at(tree, path):
    return git("show", f"{tree}:{repo_path(path)}").decode("utf-8")


def without_fenced_examples(content):
    """Discard Markdown fenced examples, including longer and indented fences."""
    output, fence = [], None
    for line in content.splitlines(keepends=True):
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line.rstrip("\r\n"))
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
                fence = None
            continue
        if marker:
            fence = marker[1]
        else:
            output.append(line)
    return "".join(output)


def stable_anchors(content):
    # The supported contract uses standalone explicit anchors, never code examples.
    return set(re.findall(r'^ {0,3}<a id="([^"\n]+)"></a>[ \t]*$', without_fenced_examples(content), re.M))


def current_spec_version(content):
    # New specs use frontmatter; legacy v1 fixtures use one top-level version line.
    if content.startswith("---\n"):
        if "\n---\n" not in content[4:]:
            raise ValueError("规格元数据未闭合")
        header = content[4:].split("\n---\n", 1)[0]
    else:
        header = without_fenced_examples(content)
    definitions = [line for line in header.splitlines() if re.match(r"\s*version\s*:", line)]
    value = re.fullmatch(r"version: ([A-Za-z0-9_.-]+)", definitions[0]) if len(definitions) == 1 else None
    if value is None:
        raise ValueError("规格须有且只有一个简单标量 version")
    return value[1]


def reference(tree, ref):
    path, sep, anchor = ref.partition("#")
    if not sep or not anchor:
        raise ValueError(f"缺少稳定锚点：{ref}")
    content = read_at(tree, path)
    if anchor not in stable_anchors(content):
        raise ValueError(f"锚点不存在：{ref}")
    return path, content


def links(path, content):
    result = set()
    for target in re.findall(r"\]\(([^\s)]+)\)", without_fenced_examples(content)):
        if "://" in target:
            continue
        name, sep, anchor = target.partition("#")
        name = posixpath.normpath(posixpath.join(posixpath.dirname(path), name)) if name else path
        result.add(name + ("#" + anchor if sep else ""))
    return result


PATCH_STATES = {"draft", "confirmed", "effective", "absorbed", "withdrawn"}


def patch_metadata(content):
    """只读取正文开头的平面元数据，历史表格和示例不参与判定。"""
    if not content.startswith("---\n") or "\n---\n" not in content[4:]:
        raise ValueError("schema_version=2 的 Change 缺少开头元数据")
    header = content[4:].split("\n---\n", 1)[0]
    result = {}
    for name in ("status", "revision"):
        definitions = [line for line in header.splitlines() if re.match(rf"\s*{name}\s*:", line)]
        value = re.fullmatch(rf"{name}: ([A-Za-z0-9_.-]+)", definitions[0]) if len(definitions) == 1 else None
        if value is None:
            raise ValueError(f"Change 元数据须有且只有一个简单标量 {name}")
        result[name] = value[1]
    if result["status"] not in PATCH_STATES:
        raise ValueError("未知 Patch 状态")
    return result


def check_structure(manifest, tree):
    """Validate one Patch projection at a Git tree, without claiming diff coverage."""
    raw = read_at(tree, manifest)
    record = json.loads(raw)
    if not isinstance(record, dict):
        raise ValueError("认领投影必须是 JSON 对象")
    for key in ("module", "task", "claim", "confirmation_source", "spec", "spec_version", "backlink"):
        if not isinstance(record.get(key), str) or not record[key].strip():
            raise ValueError(f"认领缺少 {key}")
    schema = record.get("schema_version")
    if type(schema) is not int or schema not in (1, 2) or record.get("type") != "patch":
        raise ValueError("本试跑检查器只支持 schema_version=1/2 的 patch")
    scopes = record.get("write_scope")
    if not isinstance(scopes, list) or not scopes:
        raise ValueError("认领缺少 write_scope")
    for scope in scopes:
        repo_path(scope)
    claim_path = repo_path(record["claim"])
    claim = read_at(tree, claim_path)
    if schema == 2:
        if "status" in record or "revision" in record:
            raise ValueError("status/revision 只在 Change 正文维护，check.json 不复制")
        metadata = patch_metadata(claim)
    else:
        if record.get("status") not in PATCH_STATES:
            raise ValueError("未知 Patch 状态")
        metadata = {"status": record["status"], "revision": None}
    spec_path, spec = reference(tree, record["spec"])
    back_path, back = reference(tree, record["backlink"])
    if record["spec"] not in links(claim_path, claim):
        raise ValueError("Patch 缺少指向主规格条款的正向指针")
    if record["backlink"] not in links(claim_path, claim):
        raise ValueError("Patch 缺少指向反向入口的导航")
    if claim_path not in links(back_path, back):
        raise ValueError("模块入口缺少指向 Patch 的反向指针")
    if current_spec_version(spec) != record["spec_version"]:
        raise ValueError("规格版本与认领声明不一致")
    return {
        "result": "STRUCTURE_ONLY_REVIEW_REQUIRED",
        "candidate_tree": tree,
        "module": record["module"],
        "task": record["task"],
        "manifest_schema": schema,
        "change": manifest,
        "change_revision": metadata["revision"],
        "change_status": metadata["status"],
        "write_scope": scopes,
        "handoff_check": "NOT_RUN",
        "inputs_sha256": {p: hashlib.sha256(c.encode()).hexdigest() for p, c in
                          ((manifest, raw), (claim_path, claim), (spec_path, spec), (back_path, back))},
        "semantic_review": "NOT_RUN",
        "authorization": "NOT_VERIFIED",
        "hook_ci": "NOT_VERIFIED",
    }


def check(manifest, base, tree):
    report = check_structure(manifest, tree)
    scopes = report["write_scope"]
    changed = git("diff", "--no-renames", "--name-only", "-z", base, tree, "--").decode().split("\0")
    changed = [p for p in changed if p]
    outside = [p for p in changed if not any(fnmatch.fnmatchcase(p, s) for s in scopes)]
    if outside:
        raise ValueError("认领范围外的候选文件：" + ", ".join(outside))
    # 原始对象差异绑定 mode/blob/path；不把本机显示前缀、颜色、上下文或排序写进摘要。
    diff = git("diff-tree", "--raw", "--no-abbrev", "--no-commit-id", "-r", "-z",
               "--no-renames", "--no-ext-diff", "--no-textconv", "--no-relative",
               "--ignore-submodules=none", "-O", os.devnull, base, tree, "--")
    report.update(result="STRUCTURE_OK_REVIEW_REQUIRED", base_commit=base,
                  changed_files=changed, diff_sha256=hashlib.sha256(diff).hexdigest())
    return report


def handoff_packet(report):
    if report["manifest_schema"] != 2:
        raise ValueError("交接快照需要 schema_version=2；旧记录仍可做结构检查")
    if not report["candidate_commit"]:
        raise ValueError("交接必须固定 commit；先提交候选，再用 --head 生成快照")
    if report["change_status"] not in ("confirmed", "effective"):
        raise ValueError("实施交接只接受 confirmed/effective Patch；其他状态仍可做结构检查")
    fields = ("change", "module", "task", "base_commit", "candidate_commit", "candidate_tree",
              "change_revision", "change_status", "write_scope", "inputs_sha256", "diff_sha256")
    return {"schema_version": 1, "kind": "ming_specs_handoff", **{k: report[k] for k in fields}}


def verify_handoff(packet, expected):
    if (not isinstance(packet, dict) or type(packet.get("schema_version")) is not int
            or packet.get("schema_version") != 1 or packet.get("kind") != "ming_specs_handoff"):
        raise ValueError("不支持的交接快照格式")
    # 逐项核对，接收时不重写旧包以掩盖失配。
    for key, value in expected.items():
        if packet.get(key) != value:
            raise ValueError(f"交接快照失配：{key}；退回并核对正文、候选与范围后重新交出")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--change", required=True, help="仓库相对 check.json 路径")
    parser.add_argument("--base", required=True, help="明确的比较基线 commit")
    candidate = parser.add_mutually_exclusive_group(required=True)
    candidate.add_argument("--staged", action="store_true")
    candidate.add_argument("--head", help="分支候选 commit")
    handoff = parser.add_mutually_exclusive_group()
    handoff.add_argument("--emit-handoff", action="store_true", help="输出机器生成的实施交接 JSON；要求 --head")
    handoff.add_argument("--handoff", help="读取交出方提供的 JSON 快照，与本次明确候选逐项核对")
    args = parser.parse_args()
    try:
        # 在仓库根运行，避免相对路径随当前目录变化。
        if Path.cwd().resolve() != Path(git("rev-parse", "--show-toplevel").decode().strip()).resolve():
            raise ValueError("请从目标仓库根运行")
        base = git("rev-parse", "--verify", "--end-of-options", f"{args.base}^{{commit}}").decode().strip()
        head = None if args.staged else git("rev-parse", "--verify", "--end-of-options", f"{args.head}^{{commit}}").decode().strip()
        tree = (git("write-tree") if args.staged else git("rev-parse", f"{head}^{{tree}}")).decode().strip()
        report = check(repo_path(args.change), base, tree)
        report["candidate_kind"] = "staged" if args.staged else "commit"
        report["candidate_commit"] = head
        current_head = git("rev-parse", "HEAD").decode().strip()
        index_tree = git("write-tree").decode().strip()
        report["current_head_commit"] = current_head
        report["current_index_tree"] = index_tree
        # --head 可能检查历史提交；只列 index→worktree 会漏掉已经暂存，
        # 甚至已经提交的新版本。以下差异并不属于本次固定候选的检查结论。
        for label, target in (("candidate_vs_index_files", index_tree),
                              ("candidate_vs_current_head_files", current_head),
                              ("candidate_vs_worktree_files", None)):
            args_diff = ["diff", "--no-renames", "--name-only", "-z", tree]
            if target:
                args_diff.append(target)
            report[label] = [p for p in git(*args_diff, "--").decode().split("\0") if p]
        report["checker_sha256"] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        report["unstaged_files_not_checked"] = [p for p in git("diff", "--name-only", "-z").decode().split("\0") if p]
        report["untracked_files_not_checked"] = [p for p in git("ls-files", "--others", "--exclude-standard", "-z").decode().split("\0") if p]
        if args.emit_handoff:
            print(json.dumps(handoff_packet(report), ensure_ascii=False, indent=2))
            return 0
        if args.handoff:
            packet_bytes = Path(args.handoff).read_bytes()
            packet = json.loads(packet_bytes.decode("utf-8"))
            verify_handoff(packet, handoff_packet(report))
            if current_head != head or any(report[key] for key in (
                    "candidate_vs_index_files", "candidate_vs_worktree_files", "candidate_vs_current_head_files")):
                raise ValueError("接收工作树、索引或 HEAD 与交接候选不同；先保存现有改动，再切换并核对")
            report["handoff_check"] = "MATCHED_REVIEW_REQUIRED"
            report["handoff_sha256"] = hashlib.sha256(packet_bytes).hexdigest()
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, UnicodeError) as exc:
        print(json.dumps({"result": "BLOCKED", "reason": str(exc)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
