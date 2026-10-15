from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath


class AuthorityCatalogError(RuntimeError):
    pass


@dataclass(frozen=True, order=True)
class Issue:
    code: str
    topic: str = ""
    source_id: str = ""
    path: str = ""

    def render(self) -> str:
        fields = [self.code]
        if self.topic and _is_safe_identifier(self.topic):
            fields.append(f"topic={self.topic}")
        if self.source_id and _is_safe_identifier(self.source_id):
            fields.append(f"source_id={self.source_id}")
        if self.path and _is_safe_relative_path(self.path):
            fields.append(f"path={self.path}")
        return " ".join(fields)


@dataclass(frozen=True)
class Catalog:
    default_entry_docs: tuple[str, ...]
    topic_authority: dict[str, str]


def _is_safe_relative_path(path: str) -> bool:
    if "\\" in path or re.match(r"^[A-Za-z][A-Za-z0-9+.-]*:", path):
        return False
    parts = path.split("/")
    if not parts or any(part in {"", ".", ".."} for part in parts):
        return False
    return not PurePosixPath(path).is_absolute()


def _is_safe_identifier(value: str) -> bool:
    return bool(re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:-]*", value))


def _scalar(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
        value = value[1:-1]
    if not value:
        raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
    return value


def read_catalog(path: Path) -> Catalog:
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeDecodeError) as error:
        raise AuthorityCatalogError("CATALOG_UNREADABLE") from error
    defaults: list[str] = []
    topics: dict[str, str] = {}
    section: str | None = None
    seen_sections: set[str] = set()
    for raw in lines:
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not raw.startswith((" ", "\t")):
            key, separator, remainder = stripped.partition(":")
            if key in {"default_entry_docs", "topic_authority"}:
                if not separator or remainder.strip() or key in seen_sections:
                    raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
                seen_sections.add(key)
                section = key
            else:
                section = None
            continue
        if section is not None and ("\t" in raw or not raw.startswith("  ") or raw.startswith("   ")):
            raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
        if section == "default_entry_docs":
            if not stripped.startswith("- "):
                raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
            defaults.append(_scalar(stripped[2:]))
        elif section == "topic_authority":
            topic, separator, value = stripped.partition(":")
            topic = topic.strip()
            if not separator or not topic or topic in topics:
                raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
            topics[topic] = _scalar(value)
    if not defaults or not topics:
        raise AuthorityCatalogError("CATALOG_FORMAT_INVALID")
    return Catalog(tuple(defaults), topics)


def read_goal_authority(registry_path: Path, goal_id: str) -> tuple[dict[str, object], dict[str, str]]:
    try:
        document = json.loads(registry_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise AuthorityCatalogError("REGISTRY_UNREADABLE") from error
    if not isinstance(document, dict):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    goals = document.get("goals")
    if not isinstance(goals, list):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    matches = [goal for goal in goals if isinstance(goal, dict) and goal.get("id") == goal_id]
    if len(matches) != 1:
        raise AuthorityCatalogError("GOAL_NOT_FOUND")
    authority = matches[0].get("authority_registry")
    if not isinstance(authority, dict):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    materials = authority.get("project_materials")
    topics = authority.get("topic_authority")
    if not isinstance(materials, dict) or not isinstance(topics, dict):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    if not all(
        isinstance(key, str) and _is_safe_identifier(key) and isinstance(value, dict) and value
        for key, value in materials.items()
    ):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    if not all(
        isinstance(key, str) and _is_safe_identifier(key) and isinstance(value, str) and _is_safe_identifier(value)
        for key, value in topics.items()
    ):
        raise AuthorityCatalogError("REGISTRY_UNREADABLE")
    return materials, topics


def _document_issue(docs_root: Path, relative: str) -> Issue | None:
    if docs_root.is_symlink():
        return Issue("DOC_PATH_INVALID", path=relative)
    if not _is_safe_relative_path(relative):
        return Issue("DOC_PATH_INVALID", path=relative)
    pure = PurePosixPath(relative)
    candidate = docs_root
    for part in pure.parts:
        candidate = candidate / part
        if candidate.is_symlink():
            return Issue("DOC_PATH_INVALID", path=relative)
    try:
        root_resolved = docs_root.resolve(strict=True)
        candidate_resolved = candidate.resolve(strict=True)
        candidate_resolved.relative_to(root_resolved)
    except (OSError, ValueError):
        return Issue("DOC_NOT_FOUND", path=relative)
    if not candidate_resolved.is_file():
        return Issue("DOC_NOT_FOUND", path=relative)
    return None


def validate(catalog_path: Path, docs_root: Path, registry_path: Path, goal_id: str) -> list[Issue]:
    catalog = read_catalog(catalog_path)
    materials, registry_topics = read_goal_authority(registry_path, goal_id)
    issues: set[Issue] = set()
    for relative in (*catalog.default_entry_docs, *catalog.topic_authority.values()):
        issue = _document_issue(docs_root, relative)
        if issue is not None:
            issues.add(issue)
    for topic, source_id in registry_topics.items():
        if topic not in catalog.topic_authority:
            issues.add(Issue("TOPIC_DOCUMENT_MISSING", topic=topic, source_id=source_id))
        if source_id not in materials:
            issues.add(Issue("TOPIC_SOURCE_MISSING", topic=topic, source_id=source_id))
    referenced = set(registry_topics.values())
    for source_id in materials:
        if source_id not in referenced:
            issues.add(Issue("MATERIAL_UNREFERENCED", source_id=source_id))
    return sorted(issues)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate Authority catalog entries")
    parser.add_argument("--catalog", required=True, type=Path)
    parser.add_argument("--docs-root", required=True, type=Path)
    parser.add_argument("--registry", required=True, type=Path)
    parser.add_argument("--goal-id", required=True)
    arguments = parser.parse_args(argv)
    try:
        issues = validate(arguments.catalog, arguments.docs_root, arguments.registry, arguments.goal_id)
        materials, topics = read_goal_authority(arguments.registry, arguments.goal_id)
    except AuthorityCatalogError as error:
        print(str(error), file=sys.stderr)
        return 1
    if issues:
        for issue in issues:
            print(issue.render(), file=sys.stderr)
        return 1
    print(f"authority catalog: OK topics={len(topics)} materials={len(materials)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
