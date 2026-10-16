#!/usr/bin/env python3
"""Offline monorepo knowledge navigation and existing ming-specs entrypoint."""
from __future__ import annotations
import argparse
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


# Support both script execution and importlib-based repository regression tests.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from doccheck import DocumentCheck, read_text, safe_path, strings


def local_path(root: Path, value: str) -> Path:
    return safe_path(root.resolve(), value)


def check(root: Path) -> dict:
    root = root.resolve()
    config = json.loads(read_text(root, 'tooling/knowledge.json'))
    if not isinstance(config, dict) or config.get('schema_version') != 2:
        raise ValueError('unsupported knowledge manifest; expected schema_version 2')
    if not isinstance(config.get('entrypoints'), dict):
        raise ValueError('entrypoints must be a mapping')
    problems = []
    for value in config['entrypoints'].values():
        try:
            local_path(root, value)
        except ValueError as exc:
            problems.append(str(exc))
    report = DocumentCheck(root, config).run()
    problems.extend(f"{f['code']}: {f['path']}: {f['detail']}" for f in report['findings'])
    discovered = sorted(p.relative_to(root).as_posix() for p in (root / '.agents/skills').rglob('SKILL.md'))
    if discovered != sorted(strings(config.get('discovered_skills'), 'discovered_skills')):
        problems.append(f'Skill discovery differs from declared candidates: {discovered}')
    for value in strings(config.get('excluded_reference_directories'), 'excluded_reference_directories'):
        path = Path(value)
        if path.is_absolute() or '..' in path.parts:
            raise ValueError('reference exclusion must stay in repository')
        if (root / path).exists():
            problems.append(f'learning reference repository must remain outside product: {value}')
    return {**report, 'result': 'FAIL' if problems else 'PASS', 'entrypoints': config['entrypoints'],
            'documents_checked': report['classification_counts'].get('maintained', 0),
            'discovered_skills': discovered, 'problems': problems}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['check', 'where', 'specs'])
    parser.add_argument('arguments', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if args.command == 'specs':
        forwarded = args.arguments[1:] if args.arguments[:1] == ['--'] else args.arguments
        return subprocess.run([sys.executable, str(ROOT / '.agents/skills/ming-specs/scripts/preflight.py'), *forwarded], cwd=ROOT).returncode
    if args.arguments:
        parser.error('check/where do not accept additional arguments')
    try:
        result = check(ROOT)
    except (ValueError, OSError, KeyError, TypeError) as exc:
        print(json.dumps({'result': 'FAIL', 'error': str(exc)}, ensure_ascii=False)); return 1
    if args.command == 'where':
        result['repository_root'] = str(ROOT)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return int(result['result'] != 'PASS')


if __name__ == '__main__':
    raise SystemExit(main())
