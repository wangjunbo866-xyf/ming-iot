"""Bounded, read-only documentation checks. No repair or semantic acceptance."""
from __future__ import annotations

from collections import Counter
import fnmatch
import os
from pathlib import Path, PurePosixPath
import re
from urllib.parse import unquote, urlsplit

from doclinks import markdown_anchors, navigation_targets, validation_targets, undefined_references

MAX_TEXT_BYTES = 2_000_000
PRIVATE_PARTS = {'.git', '.loopx', '.codex', '.local', '.cache', '.venv', 'node_modules', 'private', 'secrets'}


def safe_path(root: Path, value: str, *, file: bool = True) -> Path:
    if not isinstance(value, str) or not value or '\\' in value or '\x00' in value:
        raise ValueError('invalid repository path')
    path = PurePosixPath(value)
    if path.is_absolute() or '..' in path.parts or str(path) != value:
        raise ValueError('path must be a normalized repository-relative path: ' + value)
    if any(p in PRIVATE_PARTS or p.startswith('.env') for p in path.parts):
        raise ValueError('protected path: ' + value)
    target = root / value
    # Reject symlinks before existence tests or content reads, including internal aliases.
    for part in [target, *target.parents]:
        if part == root:
            break
        if part.is_symlink():
            raise ValueError('symlink is outside document-read contract: ' + value)
    if not target.resolve().is_relative_to(root.resolve()):
        raise ValueError('path escapes repository: ' + value)
    if not (target.is_file() if file else target.exists()):
        raise ValueError('missing file or target: ' + value)
    return target


def read_text(root: Path, name: str) -> str:
    target = safe_path(root, name)
    with target.open('rb') as stream:
        raw = stream.read(MAX_TEXT_BYTES + 1)
    if len(raw) > MAX_TEXT_BYTES:
        raise ValueError('document exceeds bounded read limit: ' + name)
    return raw.decode('utf-8')


def yaml_mapping(text: str) -> dict:
    try:
        import yaml
    except ImportError as exc:
        raise ValueError('PyYAML missing; install tooling/requirements-docs.txt in the active environment') from exc

    class UniqueLoader(yaml.SafeLoader):
        pass

    def mapping(loader, node, deep=False):
        loader.flatten_mapping(node)
        result = {}
        for key_node, val_node in node.value:
            key = loader.construct_object(key_node, deep=deep)
            if not isinstance(key, (str, int)) or key in result:
                raise ValueError('invalid or duplicate YAML mapping key')
            result[key] = loader.construct_object(val_node, deep=deep)
        return result

    UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, mapping)
    try:
        # Aliases are unnecessary in the small source index and can hide recursive records.
        if any(isinstance(e, yaml.AliasEvent) for e in yaml.parse(text)):
            raise ValueError('YAML aliases are unsupported in source bindings')
        value = yaml.load(text, Loader=UniqueLoader)
    except yaml.YAMLError as exc:
        raise ValueError('invalid YAML metadata') from exc
    if not isinstance(value, dict):
        raise ValueError('YAML document must be a mapping')
    return value


def frontmatter(text: str) -> dict:
    lines = text.splitlines()
    if not lines or lines[0].strip() != '---':
        raise ValueError('required frontmatter missing')
    end = next((i for i in range(1, len(lines)) if lines[i].strip() == '---'), None)
    if end is None:
        raise ValueError('frontmatter closing delimiter missing')
    return yaml_mapping('\n'.join(lines[1:end]))


def strings(value, label: str) -> list[str]:
    if not isinstance(value, list) or any(not isinstance(v, str) or not v.strip() for v in value):
        raise ValueError(label + ' must be a list of nonempty strings')
    if len(value) != len(set(value)):
        raise ValueError(label + ' contains duplicates')
    return value


def target_name(source: str, raw: str) -> tuple[str, str] | None:
    raw = raw.strip()
    if raw.startswith('<') and '>' in raw:
        raw = raw[1:raw.index('>')]
    else:
        raw = raw.split()[0] if raw else ''
    raw = re.sub(r'\\([()\\])', r'\1', raw)
    url = urlsplit(raw)
    if url.scheme or url.netloc:
        return None
    path = unquote(url.path)
    if path.startswith('/'):
        raise ValueError('absolute local link is unsupported')
    base = PurePosixPath(source).parent if path else PurePosixPath('.')
    parts = []
    for p in (base / path if path else PurePosixPath(source)).parts:
        if p == '..':
            if not parts:
                raise ValueError('link escapes repository')
            parts.pop()
        elif p != '.':
            parts.append(p)
    return '/'.join(parts) or '.', unquote(url.fragment)


class DocumentCheck:
    def __init__(self, root: Path, config: dict):
        self.root, self.config = root.resolve(), config
        self.findings, self.unchecked = [], []
        self.bodies, self.edges = {}, {}
        self.classes = {}
        self.relation_reads = set()

    def issue(self, code, path, detail):
        self.findings.append({'code': code, 'path': path, 'detail': detail})

    def body(self, name):
        if name not in self.bodies:
            self.bodies[name] = read_text(self.root, name)
        return self.bodies[name]

    def classify(self, name):
        if name in self.current:
            return 'maintained'
        kinds = [k for k in ('historical', 'reference') if any(fnmatch.fnmatchcase(name, p) for p in self.scope[k])]
        if len(kinds) > 1:
            raise ValueError('overlapping document classes: ' + name)
        return kinds[0] if kinds else 'unclassified'

    def inventory(self):
        self.scope = self.config['document_scope']
        if not isinstance(self.scope, dict) or self.scope.get('schema_version') != 1:
            raise ValueError('unsupported document scope')
        self.current = set(strings(self.config['maintained_documents'], 'maintained_documents'))
        for key in ('historical', 'reference', 'discovery_roots', 'navigation_roots'):
            strings(self.scope.get(key), key)
        for pattern in self.scope['historical'] + self.scope['reference']:
            if not pattern.startswith('docs/') or '..' in PurePosixPath(pattern).parts or '\\' in pattern:
                raise ValueError('classification patterns must stay within docs/')
        roots = self.scope['navigation_roots']
        if not roots or any(p not in self.current for p in roots):
            raise ValueError('navigation roots must be maintained documents')
        for name in self.current:
            if not name.endswith('.md'):
                raise ValueError('maintained document must be Markdown: ' + name)
            safe_path(self.root, name)
            self.classes[name] = 'maintained'
        for name in self.scope['discovery_roots']:
            if name != 'docs' and not name.startswith('docs/'):
                raise ValueError('discovery roots must stay within docs/')
            directory = safe_path(self.root, name, file=False)
            if not directory.is_dir():
                raise ValueError('discovery root must be a directory')
            for parent, dirs, files in os.walk(directory, followlinks=False):
                dirs[:] = sorted(d for d in dirs if d not in PRIVATE_PARTS and not d.startswith('.env') and not (Path(parent)/d).is_symlink())
                for f in sorted(files):
                    if not f.endswith('.md') or f.startswith('.env'):
                        continue
                    rel = (Path(parent)/f).relative_to(self.root).as_posix()
                    safe_path(self.root, rel)
                    kind = self.classify(rel)
                    self.classes[rel] = kind
                    if kind == 'unclassified':
                        self.issue('unclassified-document', rel, 'Add to maintained scope or explicitly classify as historical/reference')

    def links(self):
        for name in sorted(self.current):
            self.edges[name] = set()
            try:
                for line, key in undefined_references(self.body(name)):
                    self.issue("undefined-reference", name, f"line {line}: {key}")
                targets = validation_targets(self.body(name))
                for target in targets:
                    try:
                        resolved = target_name(name, target.raw_target)
                        if resolved is None:
                            continue  # Offline; external URLs neither fetched nor certified.
                        dst, fragment = resolved
                        safe_path(self.root, dst, file=False)
                        if dst in self.current:
                            if fragment and fragment not in markdown_anchors(self.body(dst)):
                                self.issue('missing-anchor', name, f'{dst}#{fragment} (line {target.line})')
                            if not target.is_image and not target.is_definition:
                                self.edges[name].add(dst)
                        elif fragment:
                            self.unchecked.append({'source': name, 'target': dst, 'fragment': fragment, 'reason': 'target body outside maintained read scope'})
                    except (ValueError, OSError, UnicodeError) as exc:
                        self.issue('invalid-link', name, f'line {target.line}: {exc}')
            except (ValueError, OSError, UnicodeError) as exc:
                self.issue('document-read', name, str(exc))
        reachable, pending = set(), list(self.scope['navigation_roots'])
        while pending:
            node = pending.pop()
            if node not in reachable:
                reachable.add(node)
                pending.extend(self.edges.get(node, set()) - reachable)
        for name in sorted(self.current - reachable):
            self.issue('unreachable-document', name, 'No route through maintained Markdown links from navigation roots')

    def source_index(self):
        name = self.config['entrypoints']['source_registry']
        registry = yaml_mapping(read_text(self.root, name))
        if registry.get('schema_version') != 1 or not isinstance(registry.get('materials'), list):
            raise ValueError('invalid source index schema')
        records, paths, relations = {}, {}, {}
        for item in registry['materials']:
            if not isinstance(item, dict):
                self.issue('index-record', name, 'material must be a mapping'); continue
            sid = item.get('source_id')
            if not isinstance(sid, str) or not sid.strip():
                self.issue('index-id', name, 'source_id must be a nonempty string'); continue
            if sid in records:
                self.issue('duplicate-source-id', name, sid); continue
            records[sid] = item
            try:
                p = item['path']; safe_path(self.root, p)
                for field in ('status', 'revision'):
                    if not isinstance(item.get(field), str) or not item[field].strip():
                        raise ValueError(field + ' must be a nonempty string')
                if item.get('lifecycle') not in ('current', 'historical'):
                    raise ValueError('lifecycle must explicitly be current or historical')
                for rel in strings(item.get('related', []), sid + '.related'):
                    safe_path(self.root, rel)
                bindings = item.get('body_bindings', {})
                if not isinstance(bindings, dict) or set(bindings) - {'revision', 'status'}:
                    raise ValueError('unsupported body bindings')
                if item['lifecycle'] == 'current':
                    if p not in self.current:
                        raise ValueError('current source must be a maintained document')
                    if p in paths:
                        self.issue('duplicate-current-source', name, f'{sid} and {paths[p]} claim {p}')
                    paths[p] = sid
                elif bindings:
                    raise ValueError('historical revision is a snapshot label; do not bind it to current body metadata')
                for key, binding in bindings.items():
                    body = self.body(p)
                    if (key, binding) in (('revision', 'frontmatter.version'), ('status', 'frontmatter.status')):
                        actual = frontmatter(body).get(binding.split('.')[1])
                    elif binding == 'skill.version' and key == 'revision':
                        found = re.findall(r'^版本：([A-Za-z0-9_.-]+)', body, re.M)
                        actual = found[0] if len(found) == 1 else None
                    else:
                        raise ValueError('unsupported metadata binding')
                    if not isinstance(actual, str) or actual != item[key]:
                        self.issue('index-body-mismatch', name, f'{sid}: {key} differs from {binding}')
                edges = item.get('supersedes', [])
                if not isinstance(edges, list):
                    raise ValueError('supersedes must be a list')
                relations[sid] = []
                for edge in edges:
                    if not isinstance(edge, dict) or set(edge) != {'source_id', 'scope'} or any(not isinstance(edge[k], str) or not edge[k].strip() for k in edge):
                        raise ValueError('supersedes needs source_id and a nonempty scope')
                    if edge['source_id'] in relations[sid]:
                        raise ValueError('duplicate supersession edge')
                    relations[sid].append(edge['source_id'])
            except (KeyError, ValueError, OSError, UnicodeError) as exc:
                self.issue('index-record', name, f'{sid}: {exc}')
        # Only explicitly named relation endpoints are read; no traversal of legacy bodies.
        for sid, old_ids in relations.items():
            for old in old_ids:
                if old == sid or old not in records:
                    self.issue('invalid-supersession', name, f'{sid} -> {old}'); continue
                a, b = records[sid].get('path'), records[old].get('path')
                try:
                    if a == b:
                        raise ValueError('supersession endpoints must have distinct document paths')
                    for src, dst in ((a, b), (b, a)):
                        if (not isinstance(src, str) or src.startswith('docs/wiki/legacy/')
                                or self.classify(src) not in ('maintained', 'historical')):
                            raise ValueError('supersession body must be maintained or historical, and non-legacy')
                        safe_path(self.root, src);safe_path(self.root, dst)
                        self.relation_reads.add(src)
                        links = {t[0] for x in navigation_targets(self.body(src)) if (t := target_name(src, x.raw_target)) is not None}
                        if dst not in links:
                            self.issue('missing-supersession-link', src, f'Expected link to {dst}')
                except (ValueError, OSError, UnicodeError) as exc:
                    self.issue('supersession-read', name, str(exc))
        visited, stack = set(), set()
        def visit(sid):
            if sid in stack:
                self.issue('supersession-cycle', name, sid); return
            if sid in visited:
                return
            stack.add(sid)
            for old in relations.get(sid, []):
                if old in records:
                    visit(old)
            stack.remove(sid);visited.add(sid)
        for sid in records:
            visit(sid)
        self.source_count = len(records)

    def run(self):
        try:
            self.inventory();self.links();self.source_index()
        except (ValueError, KeyError, TypeError, OSError, UnicodeError) as exc:
            self.issue('document-config', 'tooling/knowledge.json', str(exc))
        return {'findings': self.findings, 'unverified_fragments': self.unchecked,
                'classification_counts': dict(Counter(self.classes.values())),
                'bodies_read': sorted(self.bodies), 'supersession_bodies_read': sorted(self.relation_reads),
                'source_records_checked': getattr(self, 'source_count', 0),
                'semantic_review': 'NOT_RUN', 'external_urls': 'NOT_FETCHED',
                'scope': 'Declared maintained documents and explicit source relationships only; historical bodies are not recursively checked.'}
