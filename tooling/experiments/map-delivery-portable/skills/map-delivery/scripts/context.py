#!/usr/bin/env python3
"""Locate a project's business map without network access or file writes."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path, PureWindowsPath


class ContextError(ValueError):
    pass


def relative_path(value: object, field: str, *, confined: bool = False) -> str:
    if not isinstance(value, str) or not value.strip() or "\x00" in value:
        raise ContextError(f"{field} 必须是非空相对路径")
    path = Path(value)
    if path.is_absolute() or PureWindowsPath(value).drive or "\\" in value:
        raise ContextError(f"{field} 使用以 / 分隔的相对路径")
    if confined and ".." in path.parts:
        raise ContextError(f"{field} 必须位于文档目录内")
    return value


def git_observation(directory: Path) -> dict:
    """A local commit observation is not a cleanliness or remote-state check."""
    def run(*args: str) -> str:
        env = dict(os.environ, GIT_OPTIONAL_LOCKS="0")
        return subprocess.run(
            ["git", "-C", str(directory), *args], check=True,
            capture_output=True, text=True, timeout=5, env=env,
        ).stdout.strip()

    try:
        root = run("rev-parse", "--show-toplevel")
        head = run("rev-parse", "--verify", "HEAD^{commit}")
    except (OSError, subprocess.SubprocessError):
        return {"status": "unavailable", "working_tree": "not_checked"}
    return {
        "status": "observed", "repository_root": root, "head": head,
        "working_tree": "not_checked",
    }


def resolve(project_root: Path, docs_override: Path | None = None) -> dict:
    project = project_root.resolve(strict=True)
    if not project.is_dir():
        raise ContextError("项目根必须是已有目录")
    config_path = project / ".map-delivery.json"
    config = {}
    if config_path.is_symlink() or config_path.exists():
        if not config_path.resolve(strict=True).is_relative_to(project):
            raise ContextError("项目配置必须位于项目目录内")
        if not config_path.is_file():
            raise ContextError("项目配置必须是文件")
        if config_path.stat().st_size > 65536:
            raise ContextError("项目配置过大")

        def unique_object(pairs):
            value = {}
            for key, item in pairs:
                if key in value:
                    raise ContextError("项目配置有重复字段")
                value[key] = item
            return value

        try:
            config = json.loads(config_path.read_text(encoding="utf-8"), object_pairs_hook=unique_object)
        except (json.JSONDecodeError, UnicodeError):
            raise ContextError("项目配置必须是有效 UTF-8 JSON") from None
        if not isinstance(config, dict):
            raise ContextError("项目配置必须是 JSON 对象")
        allowed = {"schema_version", "docs_root", "map_root", "language"}
        if set(config) - allowed:
            raise ContextError("项目配置含未支持字段，请查接入说明")
        if type(config.get("schema_version")) is not int or config["schema_version"] != 1:
            raise ContextError("项目配置需要 schema_version: 1")
        if "language" in config and (not isinstance(config["language"], str) or not config["language"].strip()):
            raise ContextError("language 必须是非空字符串")

    docs_setting = relative_path(config.get("docs_root", "."), "docs_root")
    map_setting = relative_path(config.get("map_root", "map"), "map_root", confined=True)
    # A selected candidate worktree wins over the project pointer. Configuration
    # still must be valid: an invalid policy location is not a fallback signal.
    docs = (docs_override if docs_override is not None else project / docs_setting).resolve(strict=True)
    if not docs.is_dir():
        raise ContextError("文档根必须是已有目录")
    map_root = (docs / map_setting).resolve()
    if not map_root.is_relative_to(docs):
        raise ContextError("地图路径通过符号链接离开了文档目录")
    if map_root.exists() and not map_root.is_dir():
        raise ContextError("地图路径必须是目录")
    map_index = map_root / "README.md"
    if map_index.is_symlink() and not map_index.resolve().is_relative_to(docs):
        raise ContextError("地图入口通过符号链接离开了文档目录")
    return {
        "project_root": str(project),
        "config_path": str(config_path) if config_path.exists() else None,
        "docs_root": str(docs), "map_root": str(map_root),
        "selection": "explicit_docs_root" if docs_override is not None else "project_config" if config else "default",
        "map_exists": map_root.is_dir(), "map_index_exists": map_index.is_file(),
        "language": config.get("language"), "docs_revision": git_observation(docs),
        "scope": "local_location_only",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", type=Path, default=Path.cwd(), help="项目根目录；默认当前目录")
    parser.add_argument("--docs-root", type=Path, help="显式选择文档目录或候选 worktree")
    parser.add_argument("--json", action="store_true", help="输出结构化定位结果")
    args = parser.parse_args()
    try:
        result = resolve(args.project_root, args.docs_root)
    except ContextError as exc:
        print(f"map context: {exc}", file=sys.stderr)
        return 1
    except (OSError, RuntimeError):
        print("map context: 路径缺失、不可访问或含无效符号链接；请核对配置和显式目录", file=sys.stderr)
        return 1
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(f"文档目录：{result['docs_root']}\n地图目录：{result['map_root']}")
        print(f"地图入口：{'存在' if result['map_index_exists'] else '尚未建立'}")
        print("以上仅为本地定位结果；生效状态、授权、工作树改动和远端事实另行核实。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
