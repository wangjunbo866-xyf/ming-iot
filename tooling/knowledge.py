#!/usr/bin/env python3
"""Offline monorepo knowledge navigation and existing ming-specs entrypoint."""
from __future__ import annotations
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def local_path(root: Path, value: str) -> Path:
    if not isinstance(value, str) or not value or '\\' in value:
        raise ValueError(f'invalid repository path: {value!r}')
    path = Path(value)
    if path.is_absolute() or '..' in path.parts:
        raise ValueError(f'path must stay in repository: {value}')
    target = (root / path).resolve()
    if not target.is_relative_to(root.resolve()) or not target.is_file():
        raise ValueError(f'missing or external file: {value}')
    return target


def check(root: Path) -> dict:
    config = json.loads((root / 'tooling/knowledge.json').read_text(encoding='utf-8'))
    if config.get('schema_version') != 1:
        raise ValueError('unsupported knowledge manifest')
    problems = []
    for name in config['entrypoints']:
        try:
            local_path(root, config['entrypoints'][name])
        except ValueError as exc:
            problems.append(str(exc))
    for value in config['maintained_documents']:
        try:
            document = local_path(root, value)
            # Fenced examples are not live navigation links.
            body = re.sub(r'(?ms)^```.*?^```[^\n]*$', '', document.read_text(encoding='utf-8'))
            for link in re.findall(r'!?\[[^\]]*\]\(([^\n()]*)\)', body):
                link = link.strip()
                link = link[1:link.index('>')] if link.startswith('<') and '>' in link else link.split(' "', 1)[0]
                parsed = urlsplit(link)
                if parsed.scheme or parsed.netloc or not parsed.path:
                    continue
                target = (document.parent / unquote(parsed.path)).resolve()
                if not target.is_relative_to(root.resolve()) or not target.exists():
                    problems.append(f'{value}: unresolved local link: {link}')
        except (ValueError, OSError) as exc:
            problems.append(str(exc))
    discovered = sorted(p.relative_to(root).as_posix() for p in (root / '.agents/skills').rglob('SKILL.md'))
    if discovered != sorted(config['discovered_skills']):
        problems.append(f'Skill discovery differs from declared candidates: {discovered}')
    for value in config['excluded_reference_directories']:
        if (root / value).exists():
            problems.append(f'learning reference repository must remain outside product: {value}')
    return {'result': 'FAIL' if problems else 'PASS', 'entrypoints': config['entrypoints'],
            'documents_checked': len(config['maintained_documents']), 'discovered_skills': discovered,
            'scope': 'Local navigation and candidate Skill files; not host loading or business acceptance.',
            'problems': problems}


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
    except (ValueError, OSError, KeyError) as exc:
        print(json.dumps({'result': 'FAIL', 'error': str(exc)}, ensure_ascii=False)); return 1
    if args.command == 'where':
        result['repository_root'] = str(ROOT)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return int(result['result'] != 'PASS')


if __name__ == '__main__':
    raise SystemExit(main())
