#!/usr/bin/env python3
"""Register the locked ai-native skill for Codex and Claude without networking."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path, PurePosixPath
from urllib.parse import unquote, urlsplit


SKILL = "map-delivery"
SOURCE_IDENTITY = "git:gitee.com/ming-industry/ai-native"
DOCS_IDENTITY = "git:gitee.com/ming-industry/ming-iot-rd-docs"


class SetupError(Exception):
    pass


def _git(repo: Path, *args: str) -> str:
    try:
        result = subprocess.run(
            ["git", "-C", str(repo), *args], check=True, text=True,
            capture_output=True, timeout=10,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise SetupError("目标必须是可读取的 Git 工作树") from exc
    return result.stdout.strip()


def _confined(path: Path, root: Path) -> None:
    if not path.resolve().is_relative_to(root):
        raise SetupError("路径必须位于指定根目录内")


def _no_symlinks(path: Path, root: Path) -> None:
    _confined(path, root)
    cursor = path
    while cursor.is_relative_to(root):
        if cursor.is_symlink():
            raise SetupError("来源、注册父目录或 Git 排除路径不能通过符号链接重定向")
        if cursor == root:
            break
        cursor = cursor.parent


def _identity(repo: Path) -> str:
    if Path(_git(repo, "rev-parse", "--show-toplevel")).resolve() != repo:
        raise SetupError("请传入 Git 工作树根目录")
    value = _git(repo, "remote", "get-url", "origin")
    if value.startswith("git@gitee.com:"):
        remote_path = value.removeprefix("git@gitee.com:")
    else:
        parsed = urlsplit(value)
        if (parsed.scheme not in {"https", "ssh"} or parsed.hostname != "gitee.com"
                or parsed.password is not None or parsed.username not in (None, "git")
                or (parsed.scheme == "https" and parsed.username is not None)
                or parsed.query or parsed.fragment):
            raise SetupError("Git origin 不能确认为已声明的 Gitee 身份")
        remote_path = parsed.path.lstrip("/")
    return "git:gitee.com/" + remote_path.rstrip("/").removesuffix(".git")


def _relative(value: object) -> str:
    if not isinstance(value, str) or not value or "\\" in value or ":" in value:
        raise SetupError("来源契约或锁文件包含非法相对路径")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in {"", ".", ".."} for part in value.split("/")):
        raise SetupError("来源契约或锁文件路径不能逃出根目录")
    return str(path)


def _unique(pairs: list[tuple[str, object]]) -> dict:
    result = {}
    for key, value in pairs:
        if key in result:
            raise SetupError("来源契约或锁文件不能包含重复 JSON 字段")
        result[key] = value
    return result


def _read_json(path: Path) -> dict:
    try:
        result = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_unique)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise SetupError("来源契约或技能锁文件缺失、不可读取或格式错误") from exc
    if not isinstance(result, dict) or type(result.get("schema_version")) is not int or result["schema_version"] != 1:
        raise SetupError("来源契约或技能锁文件 schema_version 必须是 1")
    return result


def _inventory(canonical: Path) -> dict[str, str]:
    if not canonical.is_dir():
        raise SetupError("外部来源缺少 map-delivery 技能目录")
    result = {}
    for path in sorted(canonical.rglob("*")):
        relative = path.relative_to(canonical)
        if "__pycache__" in relative.parts or path.suffix in {".pyc", ".pyo"}:
            continue
        _relative(relative.as_posix())
        _no_symlinks(path, canonical)
        if path.is_dir():
            continue
        if not path.is_file():
            raise SetupError("技能来源包含不可读取的特殊文件")
        result[relative.as_posix()] = hashlib.sha256(path.read_bytes()).hexdigest()
    if "SKILL.md" not in result:
        raise SetupError("外部来源缺少普通 SKILL.md 文件")
    return result


def _check_references(canonical: Path) -> None:
    for document in canonical.rglob("*.md"):
        if "__pycache__" in document.relative_to(canonical).parts:
            continue
        text = document.read_text(encoding="utf-8")
        references = []
        for link in re.findall(r"!?\[[^\]]*\]\(([^\n()]*)\)", text):
            link = link.strip()
            link = link[1:link.index(">")] if link.startswith("<") and ">" in link else link.split(' "', 1)[0].split(" '", 1)[0]
            parsed = urlsplit(link)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            references.append((document.parent, unquote(parsed.path)))
        references.extend((canonical, value) for value in re.findall(
            r"`((?:references|scripts|assets)/[^`\s]+)`", text))
        for parent, relative in references:
            target = parent / relative
            _confined(target, canonical)
            if not target.exists():
                raise SetupError("锁定技能仍包含缺失的本地引用")


def _check_header(text: str) -> None:
    """Enforce the team's name/description single-line-string YAML subset."""
    header = re.match(r"\A---\r?\n(.*?)\r?\n---(?:\r?\n|\Z)", text, re.S)
    if not header:
        raise SetupError("SKILL.md 缺少受支持的 YAML 头部")
    fields = {}
    for line in header.group(1).splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        field = re.fullmatch(r"([a-z][a-z0-9_-]*):[ \t]+(.*)", line)
        if not field:
            raise SetupError("SKILL.md 包含非法或不支持的 YAML 头部行")
        key, raw = field.groups()
        if key not in {"name", "description"} or key in fields:
            raise SetupError("SKILL.md 只接受唯一 name、description 字段，不能重复或增加字段")
        raw = raw.strip()
        if not raw:
            raise SetupError("SKILL.md 头部字段必须是非空单行字符串")
        if raw.startswith("'"):
            if not re.fullmatch(r"'(?:[^']|'')*'", raw):
                raise SetupError("SKILL.md YAML 单引号未闭合或格式错误")
            value = raw[1:-1].replace("''", "'")
        elif raw.startswith('"'):
            try:
                value = json.loads(raw)
            except json.JSONDecodeError as exc:
                raise SetupError("SKILL.md YAML 双引号必须符合 JSON 字符串语法") from exc
        else:
            if (raw[0] in "-?:,[]{}#&*!|>'\"%@`" or re.search(r":(?:\s|$)|(?:^|\s)#", raw)
                    or re.fullmatch(r"(?i:null|true|false|yes|no|on|off|~)", raw)
                    or re.fullmatch(r"[-+]?(?:[0-9][0-9_:.eExX+-]*|\.[0-9]+|\.inf|\.nan)", raw, re.I)):
                raise SetupError("SKILL.md YAML 头部不支持构造、隐式类型或行尾注释")
            value = raw
        if (not isinstance(value, str) or not value.strip()
                or any(ord(char) < 32 and char != "\t" for char in value)
                or any(char in "\x7f\x85\u2028\u2029" for char in value)):
            raise SetupError("SKILL.md 头部字段必须是合法非空单行字符串")
        fields[key] = value
    if set(fields) != {"name", "description"} or fields["name"] != SKILL:
        raise SetupError("SKILL.md 必须声明 map-delivery name 及非空 description")


def _source(docs: Path, workspace: Path, skills_repo: Path | None) -> tuple[Path, dict[str, str]]:
    contract_path = docs / "meta/SKILLS_SOURCE.json"
    _no_symlinks(contract_path, docs)
    contract = _read_json(contract_path)
    if (contract.get("repository") != SOURCE_IDENTITY
            or contract.get("skills") != {SKILL: "skills/map-delivery"}):
        raise SetupError("技能来源契约必须声明唯一 ai-native map-delivery 来源")
    directory = _relative(contract.get("directory"))
    if len(PurePosixPath(directory).parts) != 1:
        raise SetupError("默认技能仓目录必须是工作区内的单个目录名")
    selected = skills_repo or workspace / directory
    if not selected.is_absolute():
        selected = workspace / selected
    _no_symlinks(selected, workspace)
    source = selected.resolve(strict=True)
    if _identity(source) != contract["repository"]:
        raise SetupError("技能来源 Git identity 与契约不符")
    canonical = source / contract["skills"][SKILL]
    _no_symlinks(canonical, source)
    lock_path = source / _relative(contract.get("lock_file"))
    _no_symlinks(lock_path, source)
    locked = _read_json(lock_path)
    entries = locked.get("skills")
    entry = entries.get(SKILL) if isinstance(entries, dict) else None
    if not isinstance(entry, dict) or entry.get("path") != contract["skills"][SKILL]:
        raise SetupError("技能锁文件的来源路径与契约不符")
    expected = entry.get("files")
    if not isinstance(expected, dict) or not expected or "SKILL.md" not in expected:
        raise SetupError("技能锁文件缺少完整文件库存")
    for relative, digest in expected.items():
        _relative(relative)
        if not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise SetupError("技能锁文件包含非法 SHA-256")
    if _inventory(canonical) != expected:
        raise SetupError("技能来源与锁定完整文件库存或 SHA-256 不符")
    _check_header((canonical / "SKILL.md").read_text(encoding="utf-8"))
    _check_references(canonical)
    return canonical, expected


def _lexical_link(target: Path) -> Path:
    value = Path(os.readlink(target))
    return Path(os.path.abspath(value if value.is_absolute() else target.parent / value))


def _preflight_target(target: Path, root: Path, canonical: Path, legacy: Path | None) -> str:
    _no_symlinks(target.parent, root)
    for parent in target.parents:
        if parent == root:
            break
        if parent.exists() and not parent.is_dir():
            raise SetupError("注册目录被普通文件占用")
    if target.is_symlink():
        destination = _lexical_link(target)
        if destination == canonical and target.resolve() == canonical:
            return "existing"
        if legacy is not None and destination == legacy:
            return "migrated"
        raise SetupError("已有注册指向另一来源；默认不覆盖，迁移仅接受已核实的旧文档仓来源")
    if os.path.lexists(target):
        raise SetupError("注册目标已被非符号链接内容占用")
    return "created"


def _exclude_plan(repo: Path, workspace: Path) -> tuple[Path, bytes | None, bytes]:
    path = Path(_git(repo, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude"))
    _no_symlinks(path, workspace)
    old = path.read_bytes() if path.exists() else None
    content = (old or b"").decode("utf-8")
    additions = [entry for entry in ("/.agents/skills/map-delivery", "/.claude/skills/map-delivery")
                 if entry not in content.splitlines()]
    if additions:
        content += ("\n" if content and not content.endswith("\n") else "")
        content += "\n".join(additions) + "\n"
    return path, old, content.encode("utf-8")


def _check_local_ignore(repo: Path, relative: str) -> None:
    if _git(repo, "ls-files", "--", relative):
        raise SetupError("注册入口已被 Git 跟踪，不能作为本地链接修改")
    result = subprocess.run(
        ["git", "-C", str(repo), "check-ignore", "--no-index", "-v", "-z", "--stdin"],
        input=relative + "\0", capture_output=True, text=True, timeout=10,
    )
    if result.returncode not in (0, 1):
        raise SetupError("无法验证 Git 本地排除规则")
    fields = result.stdout.split("\0")
    if len(fields) >= 3 and fields[2].startswith("!"):
        raise SetupError("已有反向忽略规则会暴露本地注册；请先人工确认规则，脚本不改写 .gitignore")


def _make_parents(path: Path, created: list[Path]) -> None:
    missing = []
    while not path.exists():
        missing.append(path)
        path = path.parent
    for directory in reversed(missing):
        directory.mkdir()
        created.append(directory)


def _atomic_bytes(path: Path, content: bytes) -> None:
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=path.parent, prefix=".skills-exclude-", delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        if path.exists():
            temporary.chmod(path.stat().st_mode & 0o777)
        os.replace(temporary, path)
    finally:
        if temporary is not None and temporary.exists():
            temporary.unlink()


def _replace_link(target: Path, expected_old: str, destination: str) -> None:
    descriptor, value = tempfile.mkstemp(dir=target.parent, prefix=".skill-link-")
    os.close(descriptor)
    temporary = Path(value)
    temporary.unlink()
    try:
        temporary.symlink_to(destination, target_is_directory=True)
        if not target.is_symlink() or os.readlink(target) != expected_old:
            raise SetupError("注册目标在预检后发生变化，已停止迁移")
        os.replace(temporary, target)
    finally:
        if os.path.lexists(temporary):
            temporary.unlink()


def setup(workspace_root: Path, product_repo: Path, skills_repo: Path | None = None,
          migrate_known_source: bool = False, legacy_docs_root: Path | None = None) -> list[tuple[Path, str]]:
    workspace = workspace_root.resolve(strict=True)
    product = product_repo.resolve(strict=True)
    docs = Path(__file__).resolve().parents[1]
    for repo in (product, docs):
        _confined(repo, workspace)
        if Path(_git(repo, "rev-parse", "--show-toplevel")).resolve() != repo:
            raise SetupError("请传入 Git 工作树根目录")
    if product == docs or product == workspace or docs == workspace:
        raise SetupError("产品仓和文档仓应为工作区内的独立工作树")
    canonical, expected_files = _source(docs, workspace, skills_repo)
    legacy = None
    if migrate_known_source:
        selected = legacy_docs_root or workspace / "ming-iot-rd-docs"
        if not selected.is_absolute():
            selected = workspace / selected
        _no_symlinks(selected, workspace)
        selected = selected.resolve(strict=True)
        if _identity(selected) != DOCS_IDENTITY:
            raise SetupError("旧来源必须具有研发文档仓 Git identity")
        legacy = selected / ".agents/skills/map-delivery"
        _no_symlinks(legacy.parent, selected)
        if (legacy.is_symlink()
                and (_lexical_link(legacy) != canonical or legacy.resolve() != canonical)):
            raise SetupError("旧来源 canonical 路径已被符号链接改写，不能作为已核实的迁移来源")
    targets = [root / directory / "skills/map-delivery"
               for root in (workspace, product, docs) for directory in (".agents", ".claude")]
    actions = [(target, _preflight_target(target, workspace, canonical, legacy)) for target in targets]
    excludes = {}
    for repo in (product, docs):
        path, old, new = _exclude_plan(repo, workspace)
        excludes[path] = (old, new)
    for target in targets:
        for repo in (product, docs):
            if target.is_relative_to(repo):
                _check_local_ignore(repo, target.relative_to(repo).as_posix())
    # All source, destination, and Git checks precede writes. Roll back ordinary
    # runtime failures; this is not a transaction across process/power failures.
    created_dirs, changed_links, changed_excludes = [], [], []
    try:
        for target, status in actions:
            if status == "existing":
                continue
            _make_parents(target.parent, created_dirs)
            new = os.path.relpath(canonical, target.parent)
            old = os.readlink(target) if status == "migrated" else None
            if old is None:
                target.symlink_to(new, target_is_directory=True)
            else:
                _replace_link(target, old, new)
            changed_links.append((target, old, new))
        for path, (old, new) in excludes.items():
            if new != (old or b""):
                _make_parents(path.parent, created_dirs)
                _atomic_bytes(path, new)
                changed_excludes.append((path, old))
        if _inventory(canonical) != expected_files or any(target.resolve() != canonical for target in targets):
            raise SetupError("安装后来源内容或注册目标变化，已回滚")
    except (OSError, SetupError):
        for path, old in reversed(changed_excludes):
            if old is None:
                path.unlink()
            else:
                _atomic_bytes(path, old)
        for target, old, new in reversed(changed_links):
            if old is None:
                target.unlink()
            else:
                _replace_link(target, new, old)
        for directory in reversed(created_dirs):
            directory.rmdir()
        raise
    return actions


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace-root", type=Path, required=True)
    parser.add_argument("--product-repo", type=Path, required=True)
    parser.add_argument("--skills-repo", type=Path, help="已核实的 ai-native Git 工作树；默认工作区/ai-native")
    parser.add_argument("--migrate-known-source", action="store_true", help="只迁移指向已核实旧文档仓路径的符号链接")
    parser.add_argument("--legacy-docs-root", type=Path, help="旧文档仓工作树；默认工作区/ming-iot-rd-docs")
    args = parser.parse_args(argv)
    try:
        if args.legacy_docs_root and not args.migrate_known_source:
            raise SetupError("--legacy-docs-root 只能与 --migrate-known-source 一起使用")
        actions = setup(args.workspace_root, args.product_repo, args.skills_repo,
                        args.migrate_known_source, args.legacy_docs_root)
    except (SetupError, OSError, UnicodeError, subprocess.SubprocessError, ValueError) as exc:
        message = str(exc) if isinstance(exc, SetupError) else "工作区路径或本地配置无法读写"
        print(f"skill registration: {message}; 未自动复制规则，Windows 可使用 TeamAI 分发后独立检查", file=sys.stderr)
        return 1
    root = args.workspace_root.resolve()
    for target, status in actions:
        print(f"{status} {target.relative_to(root).as_posix()}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
