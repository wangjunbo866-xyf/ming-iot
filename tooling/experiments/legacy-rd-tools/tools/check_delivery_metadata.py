from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from datetime import date
from pathlib import Path, PurePosixPath


ALLOWED_STATUS = {
    "rfc": {"draft", "accepted", "rejected", "superseded"},
    "spec": {"draft", "approved", "superseded"},
    "plan": {"draft", "approved", "completed", "superseded"},
    "review": {"pass", "pass_with_warnings", "changes_requested", "rejected"},
    "evidence": {"pass", "fail", "partial", "not_run"},
}

DIRECTORY_KIND = {
    "rfcs": "rfc",
    "specs": "spec",
    "plans": "plan",
    "reviews": "review",
    "evidence": "evidence",
}

CODE_REPOSITORY = "git:gitee.com/ming-industry/ming-iot"
SCALAR_FIELDS = {
    "document_kind",
    "status",
    "delivery_issue",
    "code_repository",
    "exact_head",
}
DATE_PREFIX = re.compile(r"^(\d{4}-\d{2}-\d{2})(?:-|$)")
DELIVERY_ISSUE = re.compile(r"ming-iot#I[A-Za-z0-9]+")
EXACT_HEAD = re.compile(r"[0-9a-f]{40}")


class FrontMatterError(RuntimeError):
    pass


@dataclass(frozen=True, order=True)
class Issue:
    path: str
    code: str

    def render(self) -> str:
        return f"{self.code} path={self.path}"


@dataclass(frozen=True)
class Metadata:
    scalars: dict[str, str]
    related_documents: tuple[str, ...] | None


def _scalar(raw: str) -> str:
    value = raw.strip()
    if not value:
        raise FrontMatterError
    if value[0] in {"'", '"'}:
        if len(value) < 2 or value[-1] != value[0]:
            raise FrontMatterError
        value = value[1:-1]
    if not value:
        raise FrontMatterError
    return value


def read_front_matter(path: Path) -> Metadata | None:
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeDecodeError) as error:
        raise FrontMatterError from error
    if not lines or lines[0] != "---":
        return None
    try:
        closing = lines.index("---", 1)
    except ValueError as error:
        raise FrontMatterError from error

    scalars: dict[str, str] = {}
    related_documents: list[str] | None = None
    reading_related_documents = False
    for raw in lines[1:closing]:
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if "\t" in raw:
            raise FrontMatterError
        if raw.startswith("  - "):
            if not reading_related_documents:
                raise FrontMatterError
            related_documents.append(_scalar(raw[4:]))
            continue
        if raw[0].isspace():
            raise FrontMatterError

        key, separator, value = raw.partition(":")
        if not separator or not key or key.strip() != key:
            raise FrontMatterError
        reading_related_documents = False
        if key == "related_documents":
            if value.strip() or related_documents is not None:
                raise FrontMatterError
            related_documents = []
            reading_related_documents = True
        elif key in SCALAR_FIELDS:
            if key in scalars:
                raise FrontMatterError
            scalars[key] = _scalar(value)
        else:
            raise FrontMatterError

    return Metadata(
        scalars=scalars,
        related_documents=(
            tuple(related_documents) if related_documents is not None else None
        ),
    )


def _document_date(relative: PurePosixPath) -> date | None:
    for part in relative.parts[1:]:
        match = DATE_PREFIX.match(part)
        if match is None:
            continue
        try:
            return date.fromisoformat(match.group(1))
        except ValueError:
            return None
    return None


def _delivery_documents(docs_root: Path, since: date) -> list[tuple[Path, str, str]]:
    documents: list[tuple[Path, str, str]] = []
    for directory, expected_kind in DIRECTORY_KIND.items():
        category_root = docs_root / directory
        if not category_root.is_dir():
            continue
        for path in category_root.rglob("*.md"):
            if path.name == "README.md":
                continue
            relative = path.relative_to(docs_root).as_posix()
            document_date = _document_date(PurePosixPath(relative))
            if document_date is not None and document_date >= since:
                documents.append((path, relative, expected_kind))
    return sorted(documents, key=lambda document: document[1])


def _safe_relative_path(relative: str) -> bool:
    if "\\" in relative or re.match(r"^[A-Za-z][A-Za-z0-9+.-]*:", relative):
        return False
    if not relative or any(part in {"", ".", ".."} for part in relative.split("/")):
        return False
    return not PurePosixPath(relative).is_absolute()


def _related_document_issue(docs_root: Path, relative: str) -> str | None:
    if not _safe_relative_path(relative):
        return "RELATED_DOCUMENT_INVALID"
    candidate = docs_root
    for part in PurePosixPath(relative).parts:
        candidate = candidate / part
        if candidate.is_symlink():
            return "RELATED_DOCUMENT_INVALID"
    try:
        root_resolved = docs_root.resolve(strict=True)
        candidate_resolved = candidate.resolve(strict=True)
        candidate_resolved.relative_to(root_resolved)
    except (OSError, ValueError):
        return "RELATED_DOCUMENT_NOT_FOUND"
    if not candidate_resolved.is_file():
        return "RELATED_DOCUMENT_NOT_FOUND"
    return None


def _document_path_invalid(docs_root: Path, path: Path) -> bool:
    try:
        relative = path.relative_to(docs_root)
    except ValueError:
        return True
    candidate = docs_root
    for part in relative.parts:
        candidate = candidate / part
        if candidate.is_symlink():
            return True
    try:
        root_resolved = docs_root.resolve(strict=True)
        path_resolved = path.resolve(strict=True)
        path_resolved.relative_to(root_resolved)
    except (OSError, ValueError):
        return True
    return False


def _validate_document(
    docs_root: Path, path: Path, relative: str, expected_kind: str
) -> set[Issue]:
    if _document_path_invalid(docs_root, path):
        return {Issue(relative, "DOCUMENT_PATH_INVALID")}
    try:
        metadata = read_front_matter(path)
    except FrontMatterError:
        return {Issue(relative, "FRONT_MATTER_INVALID")}
    if metadata is None:
        return {Issue(relative, "FRONT_MATTER_MISSING")}

    issues: set[Issue] = set()
    kind = metadata.scalars.get("document_kind")
    if kind not in ALLOWED_STATUS:
        issues.add(Issue(relative, "DOCUMENT_KIND_INVALID"))
    elif kind != expected_kind:
        issues.add(Issue(relative, "DOCUMENT_KIND_PATH_MISMATCH"))

    status = metadata.scalars.get("status")
    if kind in ALLOWED_STATUS and status not in ALLOWED_STATUS[kind]:
        issues.add(Issue(relative, "STATUS_INVALID"))
    if DELIVERY_ISSUE.fullmatch(metadata.scalars.get("delivery_issue", "")) is None:
        issues.add(Issue(relative, "DELIVERY_ISSUE_INVALID"))
    if metadata.scalars.get("code_repository") != CODE_REPOSITORY:
        issues.add(Issue(relative, "CODE_REPOSITORY_INVALID"))

    if metadata.related_documents is None:
        issues.add(Issue(relative, "RELATED_DOCUMENT_INVALID"))
    else:
        for related in metadata.related_documents:
            code = _related_document_issue(docs_root, related)
            if code is not None:
                issues.add(Issue(relative, code))

    if expected_kind in {"review", "evidence"}:
        exact_head = metadata.scalars.get("exact_head", "")
        if EXACT_HEAD.fullmatch(exact_head) is None:
            issues.add(Issue(relative, "EXACT_HEAD_INVALID"))
    return issues


def validate(docs_root: Path, since: date) -> list[Issue]:
    issues: set[Issue] = set()
    for path, relative, expected_kind in _delivery_documents(docs_root, since):
        issues.update(_validate_document(docs_root, path, relative, expected_kind))
    return sorted(issues)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate Delivery Packet metadata")
    parser.add_argument("--docs-root", required=True, type=Path)
    parser.add_argument("--since", required=True)
    arguments = parser.parse_args(argv)
    try:
        since = date.fromisoformat(arguments.since)
    except ValueError:
        print("DELIVERY_METADATA_ARGUMENT_INVALID", file=sys.stderr)
        return 1
    if not arguments.docs_root.is_dir():
        print("DELIVERY_METADATA_ARGUMENT_INVALID", file=sys.stderr)
        return 1
    issues = validate(arguments.docs_root, since)

    if issues:
        for issue in issues:
            print(issue.render(), file=sys.stderr)
        return 1
    print("delivery metadata: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
