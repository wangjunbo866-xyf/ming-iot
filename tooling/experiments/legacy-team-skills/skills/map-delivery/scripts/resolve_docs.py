#!/usr/bin/env python3
"""Resolve a local Ming IoT documentation worktree without network access."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import urlsplit


EXPECTED_IDENTITY = "git:gitee.com/ming-industry/ming-iot-rd-docs"
DOCS_DIRECTORY = "ming-iot-rd-docs"
MAX_ANCESTORS = 12
REQUIRED_FILES = ("AGENTS.md", "map/README.md", "tools/map_docs.py")


class ResolutionError(Exception):
    """A local path cannot be selected as the documentation worktree."""


def _git(root: Path, *arguments: str) -> str:
    environment = dict(os.environ, GIT_OPTIONAL_LOCKS="0")
    try:
        result = subprocess.run(
            ["git", "-C", str(root), *arguments],
            check=True, capture_output=True, text=True, timeout=10,
            env=environment,
        )
    except (OSError, subprocess.SubprocessError):
        # Git diagnostics can contain remote URLs. Never forward those details.
        raise ResolutionError("Git 工作树或本地 Git 配置无法读取") from None
    return result.stdout.strip()


def normalize_identity(remote: str) -> str | None:
    """Normalize supported fetch URL forms; never return URL credentials."""
    if "://" in remote:
        try:
            parsed = urlsplit(remote)
            if parsed.scheme not in ("https", "ssh") or parsed.query or parsed.fragment:
                return None
            host = parsed.hostname
            port = parsed.port
            if host is None or port not in (None, 443 if parsed.scheme == "https" else 22):
                return None
            path = parsed.path
        except ValueError:
            return None
    else:
        match = re.fullmatch(r"(?:[^@\s/:]+@)?([^\s/:]+):([^\s]+)", remote)
        if match is None:
            return None
        host, path = match.groups()
    path = path.strip("/")
    if path.endswith(".git"):
        path = path[:-4]
    if not path or any(part in ("", ".", "..") for part in path.split("/")):
        return None
    return f"git:{host.lower()}/{path}"


def _origins_match(root: Path) -> bool:
    try:
        urls = _git(root, "remote", "get-url", "--all", "origin").splitlines()
    except ResolutionError:
        return False
    return bool(urls) and all(normalize_identity(url) == EXPECTED_IDENTITY for url in urls)


def _is_docs_root(root: Path) -> bool:
    if not root.is_dir():
        return False
    try:
        toplevel = Path(_git(root, "rev-parse", "--show-toplevel")).resolve()
    except ResolutionError:
        return False
    return toplevel == root.resolve() and _origins_match(root)


def validate_docs_root(path: Path) -> dict[str, str]:
    try:
        root = path.resolve(strict=True)
    except (OSError, RuntimeError):
        raise ResolutionError("指定文档工作树不存在或无法读取") from None
    if not root.is_dir():
        raise ResolutionError("文档路径必须是 Git 工作树根目录")
    toplevel = Path(_git(root, "rev-parse", "--show-toplevel")).resolve()
    if toplevel != root:
        raise ResolutionError("文档路径必须是 Git 工作树根目录")
    if not _origins_match(root):
        raise ResolutionError("文档仓 origin fetch 身份不符或缺失")
    for relative in REQUIRED_FILES:
        try:
            required = root / relative
            if not required.is_file():
                raise OSError
            with required.open("rb") as stream:
                stream.read(1)
        except OSError:
            raise ResolutionError(f"文档工作树缺少可读文件：{relative}") from None
    head = _git(root, "rev-parse", "--verify", "HEAD^{commit}")
    if re.fullmatch(r"[0-9a-f]{40}", head) is None:
        raise ResolutionError("文档工作树缺少完整 40 位提交 Head")
    return {"docs_root": str(root), "identity": EXPECTED_IDENTITY, "exact_head": head}


def _candidates(directory: Path) -> list[Path]:
    candidates: list[Path] = []
    if _is_docs_root(directory):
        candidates.append(directory)
    named = directory / DOCS_DIRECTORY
    if named.exists() or named.is_symlink():
        candidates.append(named)
    # A symlink and its real directory are one candidate, not an ambiguity.
    distinct: dict[Path, Path] = {}
    for candidate in candidates:
        distinct.setdefault(candidate.resolve(), candidate)
    return list(distinct.values())


def resolve_docs(
    docs_root: Path | None = None,
    workspace_root: Path | None = None,
    start: Path | None = None,
) -> dict[str, str]:
    if docs_root is not None:
        return validate_docs_root(docs_root)
    try:
        search_start = (workspace_root or start or Path.cwd()).resolve(strict=True)
    except (OSError, RuntimeError):
        raise ResolutionError("定位起点不存在或无法读取") from None
    if not search_start.is_dir():
        raise ResolutionError("定位起点必须是目录")
    directories = [search_start] if workspace_root is not None else [
        search_start, *list(search_start.parents)[: MAX_ANCESTORS - 1]
    ]
    for directory in directories:
        candidates = _candidates(directory)
        if len(candidates) > 1:
            raise ResolutionError("同一定位层级存在多个文档工作树，请显式传入 --docs-root")
        if candidates:
            return validate_docs_root(candidates[0])
    raise ResolutionError("未找到文档工作树，请显式传入 --docs-root 或 --workspace-root")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--docs-root", type=Path, help="显式文档工作树根；失败时不回退")
    parser.add_argument("--workspace-root", type=Path, help="只检查此工作区及其直接文档仓入口")
    parser.add_argument("--start", type=Path, default=Path.cwd(), help="向上定位起点，默认当前目录")
    parser.add_argument("--json", action="store_true", help="输出路径、身份和完整本地 Head")
    args = parser.parse_args(argv)
    try:
        result = resolve_docs(args.docs_root, args.workspace_root, args.start)
    except (ResolutionError, OSError, RuntimeError, UnicodeError) as exception:
        message = str(exception) if isinstance(exception, ResolutionError) else "本地路径或配置无法读取"
        print(f"docs root resolution: {message}", file=sys.stderr)
        return 1
    print(json.dumps(result, ensure_ascii=False) if args.json else result["docs_root"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
