from __future__ import annotations

import argparse
import hashlib
import json
import os
import stat
import sys
import tempfile
from pathlib import Path
from typing import Any


SCHEMA_VERSION = 1


class ArchiveError(RuntimeError):
    pass


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _validate_paths(root: Path, manifest: Path) -> tuple[Path, Path]:
    if root.is_symlink():
        raise ArchiveError(f"archive root is a symlink: {root}")
    root = root.resolve()
    manifest = manifest.resolve()
    if not root.is_dir():
        raise ArchiveError(f"archive root is not a directory: {root}")
    try:
        manifest.relative_to(root)
    except ValueError as error:
        raise ArchiveError("manifest must be inside archive root") from error
    return root, manifest


def _scan(root: Path, manifest: Path) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for candidate in sorted(root.rglob("*"), key=lambda item: item.as_posix()):
        if candidate.is_symlink():
            raise ArchiveError(f"archive contains symlink: {candidate.relative_to(root)}")
        if candidate.is_dir():
            continue
        if not candidate.is_file():
            raise ArchiveError(f"archive contains non-regular file: {candidate.relative_to(root)}")
        if candidate.resolve() == manifest:
            continue
        relative = candidate.relative_to(root).as_posix()
        mode = stat.S_IMODE(candidate.stat().st_mode)
        entries.append(
            {
                "path": relative,
                "size": candidate.stat().st_size,
                "mode": f"{mode:04o}",
                "sha256": sha256_file(candidate),
            }
        )
    return entries


def _document(root: Path, manifest: Path) -> dict[str, Any]:
    return {
        "schemaVersion": SCHEMA_VERSION,
        "root": root.name,
        "files": _scan(root, manifest),
    }


def _write_atomic(path: Path, content: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def build_manifest(root: Path, manifest: Path) -> dict[str, Any]:
    root, manifest = _validate_paths(root, manifest)
    document = _document(root, manifest)
    encoded = (json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")
    _write_atomic(manifest, encoded)
    return document


def verify_manifest(root: Path, manifest: Path) -> None:
    root, manifest = _validate_paths(root, manifest)
    try:
        expected = json.loads(manifest.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ArchiveError(f"cannot read manifest: {error}") from error
    if expected.get("schemaVersion") != SCHEMA_VERSION:
        raise ArchiveError("unsupported archive manifest schema")
    actual = _document(root, manifest)
    if actual != expected:
        raise ArchiveError("archive file set or digest changed")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Build or verify a deterministic local archive manifest")
    subparsers = parser.add_subparsers(dest="command", required=True)
    for command in ("build", "verify"):
        subparser = subparsers.add_parser(command)
        subparser.add_argument("--root", required=True, type=Path)
        subparser.add_argument("--manifest", required=True, type=Path)
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "build":
            document = build_manifest(arguments.root, arguments.manifest)
            print(f"archive manifest built: {len(document['files'])} files")
        else:
            verify_manifest(arguments.root, arguments.manifest)
            print("archive manifest verified")
    except ArchiveError as error:
        print(f"archive manifest: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
