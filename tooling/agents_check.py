#!/usr/bin/env python3
"""Opt-in, read-only line budget check for this checkout's root AGENTS.md."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WARN_LINES = 115
MAX_LINES = 120
MAX_BYTES = 2 * 1024 * 1024


def check(root: Path) -> dict:
    report = {
        'path': 'AGENTS.md', 'warning_at': WARN_LINES, 'maximum': MAX_LINES,
        'lines': None, 'result': 'ERROR', 'semantic_review': 'NOT_RUN',
    }
    path = root / 'AGENTS.md'
    try:
        if path.is_symlink() or not path.is_file():
            raise ValueError('AGENTS.md must be a regular file, not a symlink')
        with path.open('rb') as stream:
            data = stream.read(MAX_BYTES + 1)
        if len(data) > MAX_BYTES:
            raise ValueError('AGENTS.md exceeds the 2 MiB read limit')
        content = data.decode('utf-8-sig')
        if not content.strip():
            raise ValueError('AGENTS.md is empty')
        # LF and CRLF count identically; a terminal newline adds no extra line.
        lines = content.count('\n') + int(not content.endswith('\n'))
        report['lines'] = lines
        if lines > MAX_LINES:
            report.update(result='ERROR', message='Over budget: move details to their maintained document and retain navigation.')
        elif lines >= WARN_LINES:
            report.update(result='WARN', message='Near budget: review entry content before adding more lines.')
        else:
            report.update(result='PASS', message='Within line budget; review navigation and meaning separately.')
    except (OSError, ValueError) as exc:
        report['message'] = str(exc)
    return report


def main(argv: list[str] | None = None, *, root: Path = ROOT) -> int:
    argparse.ArgumentParser(description=__doc__).parse_args(argv)
    report = check(root)
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return int(report['result'] == 'ERROR')


if __name__ == '__main__':
    raise SystemExit(main())
