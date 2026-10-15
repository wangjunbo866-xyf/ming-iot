#!/usr/bin/env python3
"""Map the complete PR three-dot diff to specs; success still requires semantic review."""
from __future__ import annotations

import argparse
import fnmatch
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import posixpath
import re
import subprocess
from urllib.parse import unquote, urlsplit

MAP = 'tooling/specs-map.json'
HISTORY = 'docs/wiki/legacy/'
ROOT = Path(__file__).resolve().parents[1]
_spec = importlib.util.spec_from_file_location('ming_patch', ROOT / '.agents/skills/ming-specs/scripts/preflight.py')
patch = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(patch)


def git(*args):
    return subprocess.check_output(['git', *args], stderr=subprocess.PIPE)


def text(value, label):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f'{label}: nonempty text required')
    return value


def load_json(raw):
    def unique(pairs):
        out = {}
        for k, v in pairs:
            if k in out:
                raise ValueError(f'duplicate JSON field: {k}')
            out[k] = v
        return out
    return json.loads(raw, object_pairs_hook=unique)


def path(value):
    patch.repo_path(value)
    if any(c in value for c in '\0\r\n') or value == '.':
        raise ValueError(f'invalid repository path: {value!r}')
    return value


def patterns(values):
    if not isinstance(values, list) or not values:
        raise ValueError('paths must be a nonempty list')
    for v in values:
        path(v)
        literal = v[:-3] if v.endswith('/**') else v
        if any(c in literal for c in '*?['):
            raise ValueError(f'only literal paths or directory/** allowed: {v}')
    return values


def matches(name, scopes):
    return any(name == s or (s.endswith('/**') and name.startswith(s[:-2])) for s in scopes)


class Snapshot:
    def __init__(self, revision):
        self.revision = revision
        self.files = {}
        for row in git('ls-tree', '-r', '-z', revision).split(b'\0'):
            if row:
                meta, name = row.split(b'\t', 1)
                mode, kind, oid = meta.decode().split()
                self.files[name.decode()] = (mode, kind, oid)
        self.cache = {}

    def read(self, name):
        path(name)
        meta = self.files.get(name)
        if meta is None or meta[0] not in ('100644', '100755') or meta[1] != 'blob':
            raise ValueError(f'{self.revision[:12]}: missing regular file: {name}')
        if name not in self.cache:
            self.cache[name] = git('cat-file', 'blob', meta[2]).decode('utf-8')
        return self.cache[name]

    def exists(self, name):
        return name in self.files or any(p.startswith(name.rstrip('/') + '/') for p in self.files)


def field(body, key):
    if not body.startswith('---\n') or '\n---\n' not in body[4:]:
        raise ValueError('current spec requires frontmatter')
    values = re.findall(rf'^{re.escape(key)}: ([A-Za-z0-9_.-]+)$', body[4:].split('\n---\n', 1)[0], re.M)
    if len(values) != 1:
        raise ValueError(f'current spec requires one simple {key}')
    return values[0]


def module_map(snapshot, optional=False):
    if MAP not in snapshot.files:
        if optional:
            return {}
        raise ValueError('candidate missing ' + MAP)
    config = load_json(snapshot.read(MAP))
    if type(config.get('schema_version')) is not int or config['schema_version'] != 1:
        raise ValueError('unsupported module map schema')
    if not isinstance(config.get('modules'), list):
        raise ValueError('module map requires modules list')
    out = {}
    for m in config['modules']:
        mid = text(m.get('id'), 'module id')
        if mid in out:
            raise ValueError('duplicate module: ' + mid)
        patterns(m.get('paths'))
        for k in ('spec', 'entry'):
            path(m.get(k))
            if not matches(m[k], m['paths']) or m[k].startswith(HISTORY):
                raise ValueError(f'{mid}: current {k} outside module or under history')
        out[mid] = m
    return out


def strip_examples(body):
    return re.sub(r'(?ms)^(```|~~~).*?^\1[^\n]*$', '', body)


def anchors(body):
    result = set(re.findall(r'<a\s+id="([^"]+)"', body))
    counts = {}
    for h in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', strip_examples(body), re.M):
        slug = re.sub(r'[^\w\-\s]', '', h.lower()).replace(' ', '-')
        n = counts.get(slug, 0)
        counts[slug] = n + 1
        result.add(slug if not n else f'{slug}-{n}')
    return result


def check_links(snapshot, name):
    body = strip_examples(snapshot.read(name))
    for raw in re.findall(r'!?\[[^\]]*\]\(([^\n()]*)\)', body):
        raw = raw.strip()
        raw = raw[1:raw.index('>')] if raw.startswith('<') and '>' in raw else raw.split(' "', 1)[0]
        url = urlsplit(raw)
        if url.scheme or url.netloc:
            continue
        target = posixpath.normpath(posixpath.join(posixpath.dirname(name), unquote(url.path))) if url.path else name
        path(target)
        if not snapshot.exists(target):
            raise ValueError(f'{name}: missing link {raw}')
        if url.fragment and target.endswith('.md') and unquote(url.fragment) not in anchors(snapshot.read(target)):
            raise ValueError(f'{name}: missing anchor {raw}')


def owners(name, maps):
    return {mid for registry in maps for mid, m in registry.items() if matches(name, m['paths'])}


def declaration_from_event(event):
    pr = event.get('pull_request')
    if not isinstance(pr, dict):
        raise ValueError('expected pull_request event')
    body = pr.get('body') or ''
    blocks = re.findall(r'^```ming-specs\s*\n(.*?)^```\s*$', body, re.M | re.S)
    if len(blocks) != 1:
        raise ValueError('PR body must contain exactly one ming-specs JSON block')
    base, head = pr['base']['sha'], pr['head']['sha']
    for sha in (base, head):
        if not isinstance(sha, str) or not re.fullmatch('[0-9a-f]{40}', sha):
            raise ValueError('event base/head must be full commit SHA')
    if git('rev-parse', 'HEAD').decode().strip() != head:
        raise ValueError('event check requires checkout of PR head, not merge ref or another commit')
    return base, head, load_json(blocks[0])


def check(base_ref, head_ref, declaration):
    base = git('rev-parse', '--verify', '--end-of-options', base_ref + '^{commit}').decode().strip()
    head = git('rev-parse', '--verify', '--end-of-options', head_ref + '^{commit}').decode().strip()
    merges = git('merge-base', '--all', base, head).decode().split()
    if len(merges) != 1:
        raise ValueError('PR diff needs exactly one merge base')
    merge = merges[0]
    before, after = Snapshot(merge), Snapshot(head)
    # Names are NUL delimited; treating renames as delete+add accounts for both sides.
    changed = [p for p in git('diff', '--no-renames', '--no-relative', '--ignore-submodules=none',
                             '--name-only', '-z', merge, head, '--').decode().split('\0') if p]
    old_map, new_map = module_map(before, optional=True), module_map(after)
    if old_map.keys() - new_map.keys():
        raise ValueError('module retirement needs a separate explicit migration; map entries cannot silently disappear')
    maps = (old_map, new_map)
    if (not isinstance(declaration, dict) or type(declaration.get('schema_version')) is not int
            or declaration['schema_version'] != 1):
        raise ValueError('unsupported PR declaration schema')
    for k in ('modules', 'exceptions', 'archives'):
        if not isinstance(declaration.get(k, []), list):
            raise ValueError(k + ' must be a list')
    if set(declaration) - {'schema_version', 'modules', 'exceptions', 'archives'}:
        raise ValueError('unknown PR declaration field')

    # Check all registered current entrypoints and projections, not every historical document.
    patch_reports = {}
    historical = []
    claimed_changes = {c.get('change') for c in declaration.get('modules', []) if isinstance(c, dict)}
    for mid, m in new_map.items():
        spec = after.read(m['spec'])
        field(spec, 'version')
        if field(spec, 'status') not in ('observed', 'draft', 'confirmed', 'effective'):
            raise ValueError(mid + ': invalid current spec status')
        if m['spec'] not in patch.links(m['entry'], after.read(m['entry'])):
            raise ValueError(mid + ': module entry does not link current spec')
        for name in (m['entry'], m['spec']):
            check_links(after, name)
        prefix = posixpath.dirname(m['spec']) + '/'
        for name in after.files:
            if name.startswith(prefix) and name.endswith('/check.json'):
                manifest = load_json(after.read(name))
                metadata = (patch.patch_metadata(after.read(manifest['claim']))
                            if manifest.get('schema_version') == 2 else manifest)
                if (metadata.get('status') in ('absorbed', 'withdrawn') and name not in claimed_changes
                        and name not in changed and manifest.get('claim') not in changed):
                    historical.append(name)
                    continue
                report = patch.check_structure(name, head)
                if report['module'] != mid:
                    raise ValueError(name + ': module identity mismatch')
                if manifest['spec'].partition('#')[0] != m['spec']:
                    raise ValueError(name + ': projection must reference the registered current spec')
                patch_reports[name] = report

    covered = {p: set() for p in changed}
    claims = []
    for claim in declaration.get('modules', []):
        mid = text(claim.get('module'), 'module')
        if mid not in new_map:
            raise ValueError('unknown claimed module: ' + mid)
        m = new_map[mid]
        kind = claim.get('kind')
        if kind == 'patch':
            name = path(claim.get('change'))
            report = patch_reports.get(name)
            if not report or report['module'] != mid or report['manifest_schema'] != 2:
                raise ValueError(mid + ': current v2 Patch projection required')
            if claim.get('revision') != report['change_revision']:
                raise ValueError(mid + ': stale Patch revision')
            if report['change_status'] not in ('confirmed', 'effective', 'absorbed'):
                raise ValueError(mid + ': Patch not ready for implementation')
            record = load_json(after.read(name))
            if report['change_status'] == 'absorbed' and record['claim'] not in changed:
                raise ValueError(mid + ': unchanged historical Patch cannot authorize new changes')
            if m['spec'] not in changed:
                raise ValueError(mid + ': behavior Patch must consolidate current spec in this PR')
            scope = report['write_scope']
            selected = [p for p in changed if mid in owners(p, maps) and any(fnmatch.fnmatchcase(p, s) for s in scope)]
        elif kind == 'no_behavior_change':
            text(claim.get('reason'), mid + ': reason')
            if claim.get('spec_version') != field(after.read(m['spec']), 'version'):
                raise ValueError(mid + ': stale spec version')
            clauses = claim.get('clauses')
            if not isinstance(clauses, list) or not clauses:
                raise ValueError(mid + ': current clauses required')
            for clause in clauses:
                if not isinstance(clause, str) or f'<a id="{clause}"></a>' not in after.read(m['spec']):
                    raise ValueError(mid + ': missing stable clause anchor')
            scope = patterns(claim.get('paths'))
            selected = [p for p in changed if mid in owners(p, maps) and matches(p, scope)]
        else:
            raise ValueError(mid + ': unsupported claim kind; use patch or no_behavior_change')
        if not selected:
            raise ValueError(mid + ': claim covers no changed module paths')
        for p in selected:
            covered[p].add(mid)
        claims.append({'module': mid, 'kind': kind, 'paths': selected})

    archives = []
    for item in declaration.get('archives', []):
        src, dst = path(item.get('from')), path(item.get('to'))
        text(item.get('reason'), 'archive reason')
        if not src.startswith('docs/') or not dst.startswith(HISTORY):
            raise ValueError('archive must move docs into docs/wiki/legacy')
        if src not in changed or dst not in changed or src in after.files or dst in before.files:
            raise ValueError('archive needs actual deleted source and newly added target')
        if before.files.get(src) != after.files.get(dst):
            raise ValueError('archive must preserve source blob and mode exactly')
        if any(src in (m['spec'], m['entry']) for registry in maps for m in registry.values()):
            raise ValueError('current spec/entry cannot be silently archived')
        # A move is only a history operation; any mapped source still needs its module claim.
        for p in (src, dst):
            if not owners(p, maps):
                covered[p].add('archive')
        archives.append({'from': src, 'to': dst, 'blob': before.files[src][2]})

    exceptions = []
    for item in declaration.get('exceptions', []):
        kind = item.get('kind')
        text(item.get('reason'), 'exception reason')
        scope = patterns(item.get('paths'))
        selected = [p for p in changed if matches(p, scope)]
        if not selected:
            raise ValueError('exception covers no changed paths')
        for p in selected:
            if owners(p, maps):
                raise ValueError(p + ': exception cannot bypass registered module')
            allowed = False
            if kind == 'documentation':
                allowed = p.endswith('.md') and not p.startswith(HISTORY)
            elif kind == 'tooling':
                allowed = p.startswith(('tooling/', '.github/', '.agents/skills/')) or p == 'Makefile' or p == 'docs/index.yaml'
            elif kind == 'history':
                allowed = p.startswith(HISTORY) and p in after.files
            elif kind == 'unmapped':
                # Explicit gap, never implicit exemption; reviewer must resolve semantics/ownership.
                ref = path(item.get('reference'))
                after.read(ref)
                allowed = p.startswith(('backend/', 'deploy/', 'apps/', 'components/'))
            if not allowed:
                raise ValueError(p + ': invalid exception kind/path: ' + str(kind))
            covered[p].add('exception:' + kind)
        exceptions.append({'kind': kind, 'paths': selected, 'reason': item['reason']})

    missing = []
    for p in changed:
        required = owners(p, maps)
        if (required and not required.issubset(covered[p])) or not covered[p]:
            missing.append(p)
    if missing:
        raise ValueError('unmapped or outside claim scope: ' + ', '.join(missing))
    raw = git('diff-tree', '--raw', '--no-abbrev', '--no-commit-id', '-r', '-z', '--no-renames',
              '--no-ext-diff', '--no-textconv', '--no-relative', '--ignore-submodules=none',
              '-O', os.devnull, merge, head, '--')
    return {
        'result': 'STRUCTURE_OK_REVIEW_REQUIRED', 'requested_base': base, 'merge_base': merge,
        'candidate_commit': head, 'candidate_tree': git('rev-parse', head + '^{tree}').decode().strip(),
        'changed_files': changed, 'diff_sha256': hashlib.sha256(raw).hexdigest(),
        'declaration_sha256': hashlib.sha256(json.dumps(declaration, sort_keys=True, ensure_ascii=False).encode()).hexdigest(),
        'map_sha256': hashlib.sha256(after.read(MAP).encode()).hexdigest(),
        'bootstrap_map': MAP not in before.files, 'mapping_changed': MAP in changed,
        'module_claims': claims, 'exceptions': exceptions, 'archives': archives,
        'current_modules_checked': sorted(new_map), 'patch_projections_checked': sorted(patch_reports),
        'historical_projections_not_revalidated': sorted(historical),
        'semantic_review': 'NOT_RUN', 'authorization': 'NOT_VERIFIED', 'server_merge_rules': 'NOT_VERIFIED',
        'unmapped_requires_review': [p for e in exceptions if e['kind'] == 'unmapped' for p in e['paths']],
        'checker_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', help='PR target branch tip; comparison uses its merge base with head')
    parser.add_argument('--head', help='immutable candidate commit')
    parser.add_argument('--declaration', help='local JSON equivalent of PR ming-specs block')
    parser.add_argument('--event', help='GitHub pull_request event JSON; body is data, never shell text')
    args = parser.parse_args()
    try:
        if Path.cwd().resolve() != Path(git('rev-parse', '--show-toplevel').decode().strip()).resolve():
            raise ValueError('run from repository root')
        if args.event:
            if args.base or args.head or args.declaration:
                raise ValueError('--event cannot be mixed with local comparison inputs')
            event = load_json(Path(args.event).read_text())
            base, head, declaration = declaration_from_event(event)
        else:
            if not all((args.base, args.head, args.declaration)):
                raise ValueError('provide --event or all of --base/--head/--declaration')
            base, head = args.base, args.head
            declaration = load_json(Path(args.declaration).read_text())
        report = check(base, head, declaration)
        report['input_kind'] = 'event_file' if args.event else 'local_declaration'
        report['current_head'] = git('rev-parse', 'HEAD').decode().strip()
        report['worktree_changes_not_checked'] = git('status', '--porcelain', '-z').decode().split('\0')[:-1]
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, KeyError, TypeError, AttributeError, OSError, UnicodeError, subprocess.CalledProcessError) as exc:
        print(json.dumps({'result': 'BLOCKED', 'reason': str(exc)}, ensure_ascii=False))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
