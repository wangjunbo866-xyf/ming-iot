#!/usr/bin/env python3
"""Validate, register, and check required team skills without third-party packages."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tempfile
import tomllib
from urllib.parse import unquote, urlsplit


class SkillError(Exception):
    """An actionable manifest, source, or installation problem."""


NAME = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*\Z")
DIGEST = re.compile(r"[0-9a-f]{64}\Z")
DEFAULT_ROOT = Path(__file__).resolve().parents[1]


def within(path: Path, root: Path) -> bool:
    return path == root or root in path.parents


def relative_path(value: object, label: str) -> PurePosixPath:
    if not isinstance(value, str) or not value or "\\" in value:
        raise SkillError(f"{label}: expected a nonempty relative POSIX path")
    path = PurePosixPath(value)
    if path.is_absolute() or any(p in ("", ".", "..") for p in value.split("/")):
        raise SkillError(f"{label}: path must stay inside its root: {value}")
    return path


def unique_object(pairs: list[tuple[str, object]]) -> dict:
    result = {}
    for key, value in pairs:
        if key in result:
            raise SkillError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def read_json(path: Path) -> dict:
    try:
        result = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique_object)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise SkillError(f"cannot read {path}: {exc}") from exc
    if not isinstance(result, dict):
        raise SkillError(f"{path}: expected a JSON object")
    return result


def required_skills(root: Path) -> dict[str, str]:
    manifest = read_json(root / "manifest.json")
    if type(manifest.get("schema_version")) is not int or manifest["schema_version"] != 1:
        raise SkillError("manifest.json: unsupported schema_version")
    entries = manifest.get("skills")
    if not isinstance(entries, dict) or not entries:
        raise SkillError("manifest.json: skills must be a nonempty object")
    required = {}
    for name, entry in entries.items():
        if not isinstance(name, str) or not NAME.fullmatch(name):
            raise SkillError(f"manifest.json: invalid skill name: {name!r}")
        if not isinstance(entry, dict) or type(entry.get("required")) is not bool:
            raise SkillError(f"manifest.json: {name} requires an explicit boolean required")
        path = relative_path(entry.get("path"), f"manifest.json {name}.path")
        if not within((root / path).resolve(), root):
            raise SkillError(f"manifest.json: {name} source escapes configuration root")
        if entry["required"]:
            required[name] = str(path)
    if not required:
        raise SkillError("manifest.json: at least one skill must be required")
    if len(set(required.values())) != len(required):
        raise SkillError("manifest.json: required skills must have distinct source paths")
    return required


def ignored_cache(path: Path) -> bool:
    return "__pycache__" in path.parts or path.suffix in (".pyc", ".pyo")


def file_hashes(directory: Path) -> dict[str, str]:
    if not directory.is_dir():
        raise SkillError(f"missing skill directory: {directory}")
    base = directory.resolve()
    result = {}
    for item in sorted(directory.rglob("*")):
        relative = item.relative_to(directory)
        if ignored_cache(relative):
            continue
        if item.is_symlink() and not within(item.resolve(), base):
            raise SkillError(f"skill file escapes its directory: {item}")
        if item.is_symlink() and item.is_dir():
            raise SkillError(f"linked subdirectories are not distributable: {item}")
        if item.is_dir():
            continue
        if not item.is_file():
            raise SkillError(f"unreadable or special skill file: {item}")
        result[relative.as_posix()] = hashlib.sha256(item.read_bytes()).hexdigest()
    return result


def check_references(directory: Path) -> None:
    """Validate bundled Markdown links and explicit references/scripts/assets paths."""
    base = directory.resolve()
    for document in directory.rglob("*.md"):
        if ignored_cache(document.relative_to(directory)):
            continue
        text = document.read_text(encoding="utf-8")
        links = re.findall(r"!?\[[^\]]*\]\(([^\n()]*)\)", text)
        paths = []
        for link in links:
            link = link.strip()
            if link.startswith("<") and ">" in link:
                link = link[1:link.index(">")]
            else:
                link = link.split(' "', 1)[0].split(" '", 1)[0]
            parsed = urlsplit(link)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            paths.append((document.parent, unquote(parsed.path)))
        # Files named as inline code are also a common way for skills to load references.
        paths.extend((directory, value) for value in re.findall(
            r"`((?:references|scripts|assets)/[^`\s]+)`", text))
        for parent, reference in paths:
            candidate = parent / reference
            if not within(candidate.resolve(), base):
                raise SkillError(f"{document}: reference escapes skill: {reference}")
            if not candidate.exists():
                raise SkillError(f"{document}: missing reference: {reference}")


def skill_header(text: str, name: str) -> dict[str, str]:
    """Parse this profile's strict YAML subset, not arbitrary YAML.

    The header has exactly one name and one description, each a single-line
    string. It supports plain strings, YAML single quotes, and JSON-style double
    quotes. Nested fields, YAML tags/anchors, multiline values, and inline
    comments are deliberately unsupported. This avoids approving a header that
    the host interprets differently from the required-skill checker.
    """
    frontmatter = re.match(r"\A---\r?\n(.*?)\r?\n---(?:\r?\n|\Z)", text, re.S)
    if not frontmatter:
        raise SkillError(f"{name}: SKILL.md needs YAML frontmatter")
    fields = {}
    for line in frontmatter.group(1).splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        field = re.fullmatch(r"([a-z][a-z0-9_-]*):[ \t]+(.*)", line)
        if not field:
            raise SkillError(f"{name}: malformed or unsupported YAML frontmatter line: {line!r}")
        key, raw = field.groups()
        if key not in ("name", "description"):
            raise SkillError(f"{name}: unsupported YAML frontmatter field: {key}")
        if key in fields:
            raise SkillError(f"{name}: duplicate YAML frontmatter field: {key}")
        raw = raw.strip()
        if not raw:
            raise SkillError(f"{name}: {key} must be a nonempty single-line string")
        if raw.startswith("'"):
            if not re.fullmatch(r"'(?:[^']|'')*'", raw):
                raise SkillError(f"{name}: {key} has malformed YAML single quotes")
            value = raw[1:-1].replace("''", "'")
        elif raw.startswith('"'):
            try:
                value = json.loads(raw)
            except json.JSONDecodeError as exc:
                raise SkillError(f"{name}: {key} has malformed or unsupported double quotes") from exc
        else:
            if (raw[0] in "-?:,[]{}#&*!|>'\"%@`" or re.search(r":(?:\s|$)|(?:^|\s)#", raw)
                    or re.fullmatch(r"(?i:null|true|false|yes|no|on|off|~)", raw)
                    or re.fullmatch(r"[-+]?(?:[0-9][0-9_:.eExX+-]*|\.[0-9]+|\.inf|\.nan)", raw, re.I)):
                raise SkillError(f"{name}: {key} needs quoted text; YAML constructs are unsupported")
            value = raw
        if not isinstance(value, str) or not value.strip() or "\n" in value or "\r" in value:
            raise SkillError(f"{name}: {key} must be a nonempty single-line string")
        fields[key] = value
    if set(fields) != {"name", "description"}:
        raise SkillError(f"{name}: SKILL.md needs exactly name and description frontmatter fields")
    if fields["name"] != name:
        raise SkillError(f"{name}: SKILL.md name does not match manifest")
    return fields


def check_skill(directory: Path, name: str) -> dict[str, str]:
    files = file_hashes(directory)
    if "SKILL.md" not in files:
        raise SkillError(f"{name}: missing SKILL.md")
    text = (directory / "SKILL.md").read_text(encoding="utf-8")
    skill_header(text, name)
    check_references(directory)
    return files


def source_snapshot(root: Path) -> dict:
    root = root.resolve()
    skills = {}
    for name, relative in required_skills(root).items():
        skills[name] = {"path": relative, "files": check_skill(root / relative, name)}
    return {"schema_version": 1, "skills": skills}


def compare_files(actual: dict[str, str], expected: dict[str, str], label: str) -> None:
    missing = sorted(expected.keys() - actual.keys())
    extra = sorted(actual.keys() - expected.keys())
    changed = sorted(name for name in actual.keys() & expected.keys() if actual[name] != expected[name])
    if missing or extra or changed:
        details = []
        if missing:
            details.append("missing=" + ",".join(missing))
        if extra:
            details.append("unexpected=" + ",".join(extra))
        if changed:
            details.append("changed=" + ",".join(changed))
        raise SkillError(f"{label}: lock mismatch ({'; '.join(details)})")


def validate(root: Path) -> dict:
    root = root.resolve()
    current = source_snapshot(root)
    locked = read_json(root / "skills.lock.json")
    if type(locked.get("schema_version")) is not int or locked["schema_version"] != 1:
        raise SkillError("skills.lock.json: unsupported schema_version")
    entries = locked.get("skills")
    if not isinstance(entries, dict) or set(entries) != set(current["skills"]):
        raise SkillError("skills.lock.json: skill set does not match required skills")
    for name, actual in current["skills"].items():
        entry = entries[name]
        if not isinstance(entry, dict):
            raise SkillError(f"skills.lock.json: invalid entry for {name}")
        relative_path(entry.get("path"), f"skills.lock.json {name}.path")
        if entry["path"] != actual["path"]:
            raise SkillError(f"skills.lock.json: source path differs for {name}")
        files = entry.get("files")
        if not isinstance(files, dict) or not files or "SKILL.md" not in files:
            raise SkillError(f"skills.lock.json: missing file inventory for {name}")
        for path, digest in files.items():
            relative_path(path, f"skills.lock.json {name} file")
            if not isinstance(digest, str) or not DIGEST.fullmatch(digest):
                raise SkillError(f"skills.lock.json: invalid sha256 for {name}/{path}")
        compare_files(actual["files"], files, f"source {name}")
    return locked


def write_lock(root: Path) -> dict:
    snapshot = source_snapshot(root)
    path = root / "skills.lock.json"
    content = json.dumps(snapshot, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=root, delete=False) as handle:
            temporary = Path(handle.name)
            handle.write(content)
        os.replace(temporary, path)
    finally:
        if temporary and temporary.exists():
            temporary.unlink()
    return snapshot


def git(root: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", "-C", str(root), *args], text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def install_roots(workspace: Path, projects: list[Path]) -> list[Path]:
    workspace = workspace.resolve()
    if not workspace.is_dir():
        raise SkillError(f"workspace does not exist: {workspace}")
    roots = [workspace]
    for project in projects:
        project = project if project.is_absolute() else workspace / project
        project = project.resolve()
        if not project.is_dir() or not within(project, workspace):
            raise SkillError(f"project must be an existing directory inside workspace: {project}")
        result = git(project, "rev-parse", "--show-toplevel")
        if result.returncode or Path(result.stdout.strip()).resolve() != project:
            raise SkillError(f"project must be a Git repository root: {project}")
        if project not in roots:
            roots.append(project)
    return roots


def safe_target(root: Path, name: str, legacy: bool = False) -> Path:
    target = root / (".codex" if legacy else ".agents") / "skills" / name
    # The skill itself may deliberately link to a distribution outside the workspace.
    # Its parent directories must stay in the selected repository/workspace.
    if not within(target.parent.resolve(), root):
        raise SkillError(f"installation parent escapes selected root: {target}")
    for parent in (target.parent.parent, target.parent):
        if os.path.lexists(parent) and not parent.is_dir():
            raise SkillError(f"installation parent is not a directory: {parent}")
    return target


def verify_installation(target: Path, name: str, entry: dict) -> None:
    if not target.is_dir():
        raise SkillError(f"required skill is missing or broken: {target}")
    actual = check_skill(target, name)
    compare_files(actual, entry["files"], str(target))


def config_paths(roots: list[Path]) -> list[Path]:
    user_root = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))).expanduser()
    paths = [user_root / "config.toml"]
    for root in roots:
        # Project settings from ancestors also apply when Codex opens a nested repository.
        paths.extend(parent / ".codex/config.toml" for parent in reversed((root, *root.parents)))
    return list(dict.fromkeys(paths))


def disabled_skills(roots: list[Path], targets: dict[str, list[Path]]) -> None:
    for config in config_paths(roots):
        if not config.exists():
            continue
        try:
            content = tomllib.loads(config.read_text(encoding="utf-8"))
        except (OSError, UnicodeError, tomllib.TOMLDecodeError) as exc:
            raise SkillError(f"cannot parse Codex configuration {config}: {exc}") from exc
        skills = content.get("skills", {})
        entries = skills.get("config", []) if isinstance(skills, dict) else []
        if not isinstance(entries, list):
            raise SkillError(f"{config}: skills.config must be a list")
        for entry in entries:
            if not isinstance(entry, dict) or entry.get("enabled") is not False:
                continue
            value = entry.get("path")
            if not isinstance(value, str):
                raise SkillError(f"{config}: disabled skill requires a path")
            path = Path(value).expanduser()
            if not path.is_absolute():
                path = config.parent / path
            if path.name == "SKILL.md":
                path = path.parent
            for name, skill_targets in targets.items():
                if any(path.absolute() == target.absolute() or path.resolve() == target.resolve()
                       for target in skill_targets):
                    raise SkillError(f"required skill {name} is disabled in {config}: {value}")


def inspect_destinations(roots: list[Path], locked: dict, source_root: Path,
                         require_present: bool) -> list[tuple[Path, Path]]:
    pending = []
    targets = {name: [] for name in locked["skills"]}
    for root in roots:
        for name, entry in locked["skills"].items():
            target = safe_target(root, name)
            source = source_root / entry["path"]
            targets[name].extend([target, source])
            if os.path.lexists(target):
                if target.is_symlink() and target.resolve() != source.resolve():
                    raise SkillError(f"conflicting skill symlink: {target}")
                verify_installation(target, name, entry)
            elif require_present:
                raise SkillError(f"required skill is missing: {target}")
            else:
                pending.append((target, source))
            legacy = safe_target(root, name, legacy=True)
            if os.path.lexists(legacy):
                try:
                    verify_installation(legacy, name, entry)
                except SkillError as exc:
                    raise SkillError(f"conflicting duplicate skill in .codex/skills: {legacy}: {exc}") from exc
                targets[name].append(legacy)
    user_codex = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))).expanduser()
    global_locations = [Path.home() / ".agents/skills", user_codex / "skills"]
    for name, entry in locked["skills"].items():
        for directory in global_locations:
            duplicate = directory / name
            if os.path.lexists(duplicate):
                try:
                    verify_installation(duplicate, name, entry)
                except SkillError as exc:
                    raise SkillError(f"conflicting duplicate user skill: {duplicate}: {exc}") from exc
                targets[name].append(duplicate)
    disabled_skills(roots, targets)
    return pending


def git_exclusion_plan(roots: list[Path], names: list[str], workspace: Path) -> dict[Path, tuple[bytes | None, bytes]]:
    changes = {}
    workspace = workspace.resolve()
    for root in roots:
        result = git(root, "rev-parse", "--show-toplevel")
        if result.returncode or Path(result.stdout.strip()).resolve() != root:
            continue  # A workspace container need not itself be a repository.
        result = git(root, "rev-parse", "--git-path", "info/exclude")
        if result.returncode:
            raise SkillError(f"cannot locate Git exclude file: {root}")
        exclude = Path(result.stdout.strip())
        if not exclude.is_absolute():
            exclude = root / exclude
        if not within(exclude.resolve(), workspace):
            raise SkillError(f"Git exclude file escapes workspace (external Git directories are unsupported): {exclude}")
        cursor = exclude
        while within(cursor, workspace):
            if cursor.is_symlink():
                raise SkillError(f"Git exclude path contains a symlink: {cursor}")
            if cursor == workspace:
                break
            cursor = cursor.parent
        exclude = exclude.resolve()
        old = changes[exclude][0] if exclude in changes else (exclude.read_bytes() if exclude.exists() else None)
        new = changes[exclude][1] if exclude in changes else (old or b"")
        for name in names:
            relative = f".agents/skills/{name}"
            tracked = git(root, "ls-files", "--", relative)
            if tracked.returncode or tracked.stdout.strip():
                raise SkillError(f"generated skill target is already tracked by Git: {root / relative}")
            ignored = git(root, "check-ignore", "-v", "--no-index", "--", relative)
            if ignored.returncode not in (0, 1):
                raise SkillError(f"cannot inspect Git ignore rules: {root / relative}")
            pattern = ignored.stdout.split("\t", 1)[0].rsplit(":", 1)[-1]
            if ignored.stdout and pattern.startswith("!"):
                raise SkillError(f"Git ignore negation conflicts with generated target: {root / relative}")
            if ignored.stdout:
                continue
            line = ("/" + relative + "\n").encode()
            if line not in new.splitlines(keepends=True):
                if new and not new.endswith(b"\n"):
                    new += b"\n"
                new += line
        if new != (old or b""):
            changes[exclude] = (old, new)
    return changes


def install(source_root: Path, workspace: Path, projects: list[Path]) -> dict:
    source_root = source_root.resolve()
    locked = validate(source_root)
    roots = install_roots(workspace, projects)
    pending = inspect_destinations(roots, locked, source_root, require_present=False)
    excludes = git_exclusion_plan(roots, list(locked["skills"]), workspace)
    # All source, destination, settings, and ignore checks precede any writes.
    created_links, created_dirs, changed_excludes = [], [], []

    def make_parents(path: Path) -> None:
        missing = []
        while not path.exists():
            missing.append(path)
            path = path.parent
        for directory in reversed(missing):
            directory.mkdir()
            created_dirs.append(directory)

    try:
        for exclude, (old, new) in excludes.items():
            make_parents(exclude.parent)
            changed_excludes.append((exclude, old))
            exclude.write_bytes(new)
        for target, source in pending:
            make_parents(target.parent)
            target.symlink_to(os.path.relpath(source, target.parent), target_is_directory=True)
            created_links.append(target)
        # Verify the actual end state before claiming an installation was completed.
        result = check(source_root, workspace, projects)
    except (OSError, SkillError):
        for target in reversed(created_links):
            target.unlink()
        for exclude, old in reversed(changed_excludes):
            if old is None:
                exclude.unlink()
            else:
                exclude.write_bytes(old)
        for directory in reversed(created_dirs):
            directory.rmdir()
        raise
    result["created"] = [str(path) for path in created_links]
    result["git_excludes_updated"] = [str(path) for path in excludes]
    return result


def check(source_root: Path, workspace: Path, projects: list[Path]) -> dict:
    source_root = source_root.resolve()
    locked = validate(source_root)
    roots = install_roots(workspace, projects)
    inspect_destinations(roots, locked, source_root, require_present=True)
    return {"ok": True, "skills": list(locked["skills"]), "roots": [str(path) for path in roots]}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("validate", "lock", "install", "check"):
        command = commands.add_parser(name)
        command.add_argument("--config-root", type=Path, default=DEFAULT_ROOT)
        command.add_argument("--json", action="store_true", help="emit a machine-readable result")
        if name in ("install", "check"):
            command.add_argument("--workspace-root", type=Path, required=True)
            command.add_argument("--project", type=Path, action="append", default=[])
    args = parser.parse_args(argv)
    try:
        if args.command == "validate":
            locked = validate(args.config_root)
            result = {"ok": True, "skills": list(locked["skills"])}
        elif args.command == "lock":
            locked = write_lock(args.config_root)
            result = {"ok": True, "skills": list(locked["skills"]),
                      "lock_file": str(args.config_root.resolve() / "skills.lock.json")}
        else:
            operation = install if args.command == "install" else check
            result = operation(args.config_root, args.workspace_root, args.project)
        result["command"] = args.command
        print(json.dumps(result, ensure_ascii=False) if args.json else f"{args.command}: OK ({', '.join(result['skills'])})")
        return 0
    except (SkillError, OSError, UnicodeError) as exc:
        result = {"ok": False, "command": args.command, "error": str(exc)}
        print(json.dumps(result, ensure_ascii=False) if args.json else f"{args.command}: FAILED: {exc}",
              file=sys.stdout if args.json else sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
