"""Check explicit Markdown files; does not certify a repository-wide gate."""
from __future__ import annotations

import argparse
from dataclasses import asdict
import json
from pathlib import Path
import sys

from audit_docs import check_links, check_sensitive_content
from rdoc.config import canonical_path, load_project_config, path_is_excluded
from rdoc.io import add
from rdoc.models import Finding, FindingList


def check(root: Path, paths: list[Path]) -> list[Finding]:
    root = root.resolve()
    findings = FindingList()
    config, problems = load_project_config(root)
    for problem in problems:
        add(findings, problem.severity, problem.code, root, problem.path, problem.message)
    if not paths:
        add(findings, "error", "scope-empty", root, root, "name at least one Markdown file")
    files = []
    for requested in paths:
        path = root / requested
        canonical = canonical_path(root, path)
        if canonical is None or path.suffix.lower() != ".md" or not path.is_file():
            add(findings, "error", "scope-path", root, path, "scope must name existing Markdown files inside the project root")
        elif path_is_excluded(root, path, config) or path_is_excluded(root, canonical, config):
            add(findings, "error", "scope-excluded", root, path, "explicit scope conflicts with project exclusions")
        elif path not in files:
            files.append(path)
    check_links(root, files, findings)
    check_sensitive_content(root, files, findings, config)
    return findings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--files", type=Path, nargs="+", required=True, help="explicit paths relative to root")
    args = parser.parse_args()
    findings = check(args.root, args.files)
    failed = any(item.severity == "error" for item in findings)
    print(json.dumps({
        "status": "fail" if failed else "pass",
        "scope": "explicit_files",
        "files": [str(path) for path in args.files],
        "checks": ["project_config", "links_and_anchors", "sensitive_content"],
        "not_checked": ["metadata_and_lifecycle", "repository_navigation", "semantic_consistency", "stage_gates"],
        "findings": [asdict(item) for item in findings],
    }, ensure_ascii=False, indent=2))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
