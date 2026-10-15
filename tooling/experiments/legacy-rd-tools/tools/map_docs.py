"""Offline validation and conservative scaffolding for map delivery documents.

This is deliberately not a YAML or CommonMark implementation. Front matter is
limited to scalars and two-space string lists; links support inline/reference
Markdown outside fenced examples. No network or activation operations occur.
"""

import argparse
import json
import os
import re
import subprocess
from dataclasses import dataclass
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit


@dataclass(frozen=True, order=True)
class Problem:
    path: str
    code: str


@dataclass(frozen=True)
class FrontMatter:
    scalars: dict[str, str]
    lists: dict[str, tuple[str, ...]]


@dataclass(frozen=True)
class MapDocument:
    path: Path
    front_matter: FrontMatter


STATES = {
    "spec": {"current"},
    "change": {"draft", "approved", "implementing", "verifying", "effective", "rejected", "superseded"},
    "plan": {"draft", "approved", "in_progress", "completed", "superseded"},
    "review": {"pass", "changes_required"},
    "evidence": {"verified", "failed"},
    "adr": {"accepted", "superseded"},
    "reference": {"current", "superseded"},
}
ACTIVE = {"approved", "implementing", "verifying"}
TYPES = {"FEATURE", "PATCH", "BUGFIX", "SUNSET"}
DELIVERY = re.compile(r"(ming-iot(?:-rd-docs)?)#(I[A-Z0-9]+)\Z")
REPOSITORY = re.compile(r"git:gitee\.com/ming-industry/[A-Za-z0-9][A-Za-z0-9._-]*\Z")
HEAD = re.compile(r"[0-9a-fA-F]{40}\Z")


def _scalar(value):
    value = value.strip()
    if not value:
        raise ValueError("empty scalar")
    if value.startswith('"'):
        parsed = json.loads(value)
        if not isinstance(parsed, str):
            raise ValueError("not a string")
        return parsed
    if value.startswith("'"):
        if not re.fullmatch(r"'(?:[^']|'')*'", value):
            raise ValueError("unclosed scalar")
        return value[1:-1].replace("''", "'")
    if value[0] in "[{>|&*!" or ": " in value or "\t" in value:
        raise ValueError("unsupported YAML")
    return value


def parse_front_matter(text):
    """Return metadata and body; reject unsupported or ambiguous YAML syntax."""
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        raise ValueError("missing front matter")
    try:
        end = lines.index("---", 1)
    except ValueError:
        raise ValueError("unclosed front matter") from None
    scalars, lists = {}, {}
    current = None
    for line in lines[1:end]:
        if not line.strip() or line.startswith("#"):
            continue
        if line.startswith("  - ") and current is not None:
            lists[current].append(_scalar(line[4:]))
            continue
        match = re.fullmatch(r"([a-z][a-z0-9_]*):(?: +(.*))?", line)
        if not match or match[1] in scalars or match[1] in lists:
            raise ValueError("invalid field")
        key, value = match.groups()
        current = None
        if value is None or not value.strip():
            lists[key] = []
            current = key
        else:
            scalars[key] = _scalar(value)
    return FrontMatter(scalars, {k: tuple(v) for k, v in lists.items()}), "\n".join(lines[end + 1:])


def _masked(text):
    return "".join(char if char in "\r\n" else " " for char in text)


def _outside_fences(text, *, preserve_offsets=False):
    result, fence = [], None
    for line in text.splitlines(keepends=preserve_offsets):
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
                fence = None
            if preserve_offsets:
                result.append(_masked(line))
        elif marker:
            fence = marker[1]
            if preserve_offsets:
                result.append(_masked(line))
        else:
            result.append(line)
    return ("" if preserve_offsets else "\n").join(result)


def _anchor_source(body, *, preserve_offsets=False):
    """Exclude code examples and comments before examining rendered anchors."""
    body = _outside_fences(body, preserve_offsets=preserve_offsets)
    body = re.sub(r"<!--.*?(?:-->|\Z)|(?<!`)(`+)(?!`).*?(?<!`)\1(?!`)",
                  lambda match: (_masked(match[0]) if preserve_offsets
                                 else " " + "\n" * match[0].count("\n")), body, flags=re.S)
    return re.sub(r"^(?: {4}|\t).*$",
                  lambda match: _masked(match[0]) if preserve_offsets else "", body, flags=re.M)


class _AnchorParser(HTMLParser):
    """Only a paired <a> with one nonempty, whitespace-free id is stable.

    Parsing attributes avoids treating data-id, quoted attribute contents or
    mismatched quotes as id declarations. Other attributes and Unicode IDs are
    allowed; self-closing and unclosed anchors are not declarations.
    """

    def __init__(self, source=""):
        super().__init__(convert_charrefs=True)
        self.anchors = set()
        self.pending = None
        self.source = source
        self.declarations = []

    def handle_starttag(self, tag, attrs):
        if tag == "a":
            ids = [value for key, value in attrs if key == "id"]
            self.pending = (ids[0] if len(ids) == 1 and ids[0]
                            and not any(char.isspace() for char in ids[0]) else None)
            line, column = self.getpos()
            offset = sum(len(part) for part in self.source.splitlines(keepends=True)[:line - 1]) + column
            match = re.compile(r'<a id="([a-z][a-z0-9]*(?:-[a-z0-9]+)*)"></a>').match(self.source, offset)
            if match:
                self.declarations.append(match)

    def handle_endtag(self, tag):
        if tag == "a":
            if self.pending is not None:
                self.anchors.add(self.pending)
            self.pending = None

    def handle_startendtag(self, tag, attrs):
        # HTML anchors need an explicit closing tag, unlike void elements.
        if tag == "a":
            self.pending = None


def _stable_anchors(body):
    parser = _AnchorParser()
    parser.feed(_anchor_source(body))
    parser.close()
    return parser.anchors


def _links(body, *, include_definitions=False):
    """Extract used navigation edges; optionally include definitions for linting.

    A reference definition alone is not rendered. Images are lint targets, not
    navigation. This remains the documented restricted Markdown dialect.
    """
    original_lines = body.splitlines(keepends=True)
    body = _anchor_source(body, preserve_offsets=True)
    title = r'''(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\((?:\\.|[^()\\])*\))'''
    definition = re.compile(r"^ {0,3}\[([^\]\n]+)\]:[ \t]*(<[^>\n]+>|\S+)"
                            r"(?:[ \t]+" + title + r")?[ \t]*$")
    normalize = lambda label: " ".join(label.split()).casefold()
    definitions = {}
    lines, definition_allowed = [], True
    for original, line in zip(original_lines, body.splitlines(keepends=True)):
        match = definition.fullmatch(line.rstrip("\r\n")) if definition_allowed else None
        if match:
            definitions.setdefault(normalize(match[1]), match[2])
            lines.append(_masked(line))
        else:
            lines.append(line)
            # A reference definition cannot interrupt a prose paragraph.
            definition_allowed = (
                not original.strip()
                or bool(re.match(r"^ {0,3}(?:#{1,6}\s|`{3,}|~{3,})", original))
                or bool(re.fullmatch(r" {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})",
                                     original.rstrip("\r\n")))
                or (definition_allowed and bool(re.match(r"^(?: {4}|\t)", original)))
            )
    body = "".join(lines)
    destinations = []
    occupied = []
    for match in re.finditer(r"(?<![\\\[])!?\[[^\]\n]*\]\(\s*", body):
        if occupied and match.start() < occupied[-1][1]:
            continue
        start = end = match.end()
        if body[start:start + 1] == "<":
            end = body.find(">", start)
            if end == -1 or "\n" in body[start:end]:
                continue
            end += 1
        else:
            depth = 0
            while end < len(body):
                char = body[end]
                if char == "\\" and end + 1 < len(body):
                    end += 2
                    continue
                if char.isspace() or (char == ")" and not depth):
                    break
                if char == "(":
                    depth += 1
                elif char == ")":
                    depth -= 1
                end += 1
            if depth:
                continue
        closing = re.compile(r"(?:\s+" + title + r")?\s*\)").match(body, end)
        if (end > start and closing
                and not re.search(r"\n[ \t\r]*\n", body[match.start():closing.end()])):
            if include_definitions or not match[0].startswith("!"):
                destinations.append(re.sub(r"\\([()])", r"\1", body[start:end]))
            # Mask the entire link, including its title, before reference parsing.
            occupied.append((match.start(), closing.end()))
    for start, end in reversed(occupied):
        body = body[:start] + " " * (end - start) + body[end:]
    for match in re.finditer(r"(?<![\\\[])!?\[([^\]\n]+)\](?:\[([^\]\n]*)\])?", body):
        if match[0].startswith("!") and not include_definitions:
            continue
        label = match[2] or match[1]
        if normalize(label) in definitions:
            destinations.append(definitions[normalize(label)])
    if include_definitions:
        destinations.extend(definitions.values())
    return [value[1:-1] if value.startswith("<") else value for value in destinations]


def _has_status_text(body, kind):
    """A label or Markdown punctuation alone cannot describe an object's state."""
    for line in _outside_fences(body).splitlines():
        label = re.search(r"\b" + kind + r"\b(?:\.md)?", line, re.I)
        if label and any(char.isalnum() for char in line[label.end():]):
            return True
    return False


def _module(value):
    parts = value.split("/")
    return len(parts) == 3 and parts[0] == "map" and all(_filename(p) for p in parts)


def _filename(value):
    return (bool(value) and value not in {".", ".."} and value == value.strip()
            and not re.search(r'[\x00-\x1f\x7f/\\:#?<>|"*]', value)
            and not value.endswith("."))


def _confined(root, value, base=None, metadata=False):
    """Check lexical and resolved boundaries before any read or write."""
    if not value or "\\" in value or re.search(r"[\x00-\x1f\x7f]", value):
        raise ValueError("invalid path")
    path = Path(value)
    if path.is_absolute() or (metadata and (".." in path.parts or urlsplit(value).scheme)):
        raise ValueError("absolute or parent path")
    target = (base or root) / path
    # Refuse symlinks even when currently pointing inward: scaffolds and indexes
    # should not acquire alternate identities or depend on mutable indirection.
    for part in (target, *target.parents):
        if part == root:
            break
        if part.is_symlink():
            raise ValueError("symlink")
    resolved = target.resolve()
    if not resolved.is_relative_to(root):
        raise ValueError("path escape")
    return resolved


def _navigation(path):
    parts = path.parts
    return path.name == "README.md" and (len(parts) == 2 or len(parts) == 4
            or (len(parts) == 6 and parts[3] == "changes"))


def _expected_kind(path):
    parts = path.parts
    if len(parts) == 4 and path.name == "spec.md":
        return "spec"
    if len(parts) == 5 and parts[3] in {"adr", "references"}:
        return {"adr": "adr", "references": "reference"}[parts[3]]
    if len(parts) == 6 and parts[3] == "changes" and path.stem in {"change", "plan", "review", "evidence"}:
        return path.stem
    return None


def _markdown_paths(root, problems):
    start = root / "map"
    if start.is_symlink():
        problems.add(Problem("map", "MAP_RELATED_PATH_INVALID"))
        return []
    if not start.is_dir():
        problems.add(Problem("map/README.md", "MAP_DOCUMENT_MISSING"))
        return []
    paths = []
    def onerror(error):
        path = Path(error.filename).relative_to(root).as_posix()
        problems.add(Problem(path, "MAP_RELATED_PATH_INVALID"))
    for directory, dirs, files in os.walk(start, followlinks=False, onerror=onerror):
        relative = Path(directory).relative_to(root)
        if len(relative.parts) == 5 and relative.parts[3] == "changes":
            if relative.name.split("-", 1)[0] not in TYPES:
                problems.add(Problem(relative.as_posix(), "MAP_CHANGE_TYPE_INVALID"))
            for filename in ("README.md", "change.md"):
                if filename not in files:
                    problems.add(Problem((relative / filename).as_posix(), "MAP_DOCUMENT_MISSING"))
        for name in dirs[:] + files:
            path = Path(directory) / name
            if path.is_symlink():
                problems.add(Problem(path.relative_to(root).as_posix(), "MAP_RELATED_PATH_INVALID"))
                if name in dirs:
                    dirs.remove(name)
            elif name in files and name.endswith(".md"):
                paths.append(path)
    return sorted(paths)


def _table_heads(body, status):
    rows, header = [], None
    for line in _outside_fences(body).splitlines():
        if not line.lstrip().startswith("|"):
            header = None
            continue
        cells = [cell.strip().strip("`") for cell in line.strip().strip("|").split("|")]
        if {"repository", "base", "exact_head", "verdict"}.issubset(cells):
            header = cells
            continue
        if header is None or all(re.fullmatch(r":?-+:?", cell) for cell in cells):
            continue
        if len(cells) != len(header):
            raise ValueError("table width")
        row = dict(zip(header, cells))
        if not REPOSITORY.fullmatch(row["repository"]) or not HEAD.fullmatch(row["base"]) or not HEAD.fullmatch(row["exact_head"]) or row["verdict"] != status:
            raise ValueError("table identity")
        rows.append(row["repository"] + "@" + row["exact_head"])
    return rows


def validate(docs_root):
    """Return sorted, deduplicated diagnostics without exposing document contents."""
    root = Path(docs_root).resolve()
    problems, documents, bodies, links = set(), {}, {}, {}

    def report(path, code):
        problems.add(Problem(str(path), code))

    def related(owner, value, directory=False):
        try:
            target = _confined(root, value, metadata=True)
        except (ValueError, OSError, RuntimeError):
            report(owner, "MAP_RELATED_PATH_INVALID")
            return None
        if not (target.is_dir() if directory else target.is_file()):
            report(owner, "MAP_DOCUMENT_MISSING")
        return target.relative_to(root).as_posix()

    for path in _markdown_paths(root, problems):
        relative = path.relative_to(root)
        name = relative.as_posix()
        if not all(_filename(part) for part in relative.parts):
            report(name, "MAP_RELATED_PATH_INVALID")
        if len(relative.parts) == 4 and path.name == "plan.md":
            report(name, "MAP_MODULE_PLAN_FORBIDDEN")
        try:
            text = path.read_text(encoding="utf-8")
            if _navigation(relative) and not text.startswith("---"):
                body = text
            else:
                fm, body = parse_front_matter(text)
                documents[name] = MapDocument(relative, fm)
            bodies[name] = body
        except (ValueError, OSError):
            report(name, "MAP_FRONT_MATTER_INVALID")

    for name, body in bodies.items():
        links[name] = set()
        visible = set(_links(body))
        for destination in _links(body, include_definitions=True):
            try:
                url = urlsplit(destination)
                if url.scheme in {"https", "http", "mailto"}:
                    if destination in visible:
                        links[name].add(destination)
                    continue
                if url.scheme or url.netloc:
                    raise ValueError("unsupported link")
                target = _confined(root, unquote(url.path) or Path(name).name, base=(root / name).parent)
                rel = target.relative_to(root).as_posix()
                if target.is_dir():
                    target = _confined(root, "README.md", base=target)
                    rel = target.relative_to(root).as_posix()
                if destination in visible:
                    links[name].add(rel)
                if not target.is_file():
                    report(name, "MAP_DOCUMENT_MISSING")
                elif url.fragment:
                    target_body = bodies.get(rel)
                    if target_body is None:
                        target_body = _outside_fences(target.read_text(encoding="utf-8"))
                    anchors = _stable_anchors(target_body)
                    for heading in re.findall(r"^ {0,3}#{1,6}\s+(.+?)\s*#*\s*$", _anchor_source(target_body), re.M):
                        anchors.add(re.sub(r"[^\w\- ]", "", heading.lower()).replace(" ", "-"))
                    if unquote(url.fragment) not in anchors:
                        report(name, "MAP_RELATED_PATH_INVALID")
            except (ValueError, OSError, RuntimeError):
                report(name, "MAP_RELATED_PATH_INVALID")

    deliveries, changes = {}, {}
    for name, document in documents.items():
        s, lists = document.front_matter.scalars, document.front_matter.lists
        for value in lists.get("related_documents", ()):
            related(name, value)
        kind = s.get("document_kind", "")
        if kind not in STATES or (not _navigation(document.path) and kind != _expected_kind(document.path)):
            report(name, "MAP_DOCUMENT_KIND_INVALID")
            continue
        if s.get("status") not in STATES[kind]:
            report(name, "MAP_STATUS_INVALID")
        module = "/".join(document.path.parts[:3])
        if kind in {"spec", "adr", "reference"}:
            if s.get("module") != module or not _module(s.get("module", "")):
                report(name, "MAP_RELATED_PATH_INVALID")
            if kind == "reference" and not s.get("source"):
                report(name, "MAP_FRONT_MATTER_INVALID")
            continue
        delivery = s.get("delivery_id", "")
        match = DELIVERY.fullmatch(delivery)
        if not match:
            report(name, "MAP_DELIVERY_ID_INVALID")
        expected_issue = "https://gitee.com/ming-industry/" + delivery.replace("#", "/issues/")
        if not match or s.get("issue") != expected_issue:
            report(name, "MAP_ISSUE_URL_INVALID")
        hub = document.path.with_name("README.md").as_posix()
        related(name, s.get("hub", ""))
        if s.get("hub") != hub:
            report(name, "MAP_HUB_INVALID")
        if kind in {"change", "plan"}:
            repositories = lists.get("affected_repositories", ())
            if not repositories or any(not REPOSITORY.fullmatch(repo) for repo in repositories):
                report(name, "MAP_REPOSITORY_INVALID")
        if kind == "plan":
            expected_change = document.path.with_name("change.md").as_posix()
            if related(name, s.get("change_spec", "")) != expected_change:
                report(name, "MAP_RELATED_PATH_INVALID")
        if kind in {"review", "evidence"}:
            heads = lists.get("repository_heads", ())
            try:
                if not heads or len(set(heads)) != len(heads):
                    raise ValueError("missing or duplicate heads")
                for item in heads:
                    repo, head = item.rsplit("@", 1)
                    if not REPOSITORY.fullmatch(repo) or not HEAD.fullmatch(head):
                        raise ValueError("invalid head")
                if sorted(_table_heads(bodies[name], s.get("status"))) != sorted(heads):
                    raise ValueError("table does not match metadata")
            except ValueError:
                report(name, "MAP_REPOSITORY_INVALID")
        if kind != "change":
            continue
        changes[name] = document
        deliveries.setdefault(delivery, []).append(name)
        change_type = s.get("change_type", "")
        directory = document.path.parent.name
        if change_type.upper() not in TYPES or not directory.startswith(change_type.upper() + "-"):
            report(name, "MAP_CHANGE_TYPE_INVALID")
        prefix = change_type.upper() + "-" + delivery.replace("#", "-") + "_"
        if not directory.startswith(prefix) or not _filename(directory[len(prefix):]):
            report(name, "MAP_DELIVERY_ID_INVALID")
        target_module = s.get("target_module", "")
        if not _module(target_module) or target_module != module:
            report(name, "MAP_RELATED_PATH_INVALID")
        related(name, target_module, directory=True)
        affected = lists.get("affected_modules", ())
        if not affected or target_module not in affected:
            report(name, "MAP_RELATED_PATH_INVALID")
        for affected_module in affected:
            if not _module(affected_module):
                report(name, "MAP_RELATED_PATH_INVALID")
            related(name, affected_module, directory=True)
            index = affected_module + "/README.md"
            related(name, index)
            if hub not in links.get(index, set()):
                report(index if index in bodies else name, "MAP_HUB_INVALID")

    for owners in deliveries.values():
        if len(owners) > 1:
            for name in owners:
                report(name, "MAP_DELIVERY_DUPLICATE")

    coverage = {}
    for name, document in changes.items():
        s, lists = document.front_matter.scalars, document.front_matter.lists
        module, status = s.get("target_module", ""), s.get("status")
        spec = module + "/spec.md"
        anchors = lists.get("target_anchors", ())
        stable = _stable_anchors(bodies.get(spec, ""))
        if not anchors:
            report(name, "MAP_TARGET_ANCHOR_INVALID")
        for anchor in anchors:
            new_module = (anchor == "__module_root__" and spec not in bodies
                          and s.get("change_type", "").lower() == "feature" and status != "effective")
            if anchor not in stable and not new_module:
                report(name, "MAP_TARGET_ANCHOR_INVALID")
            if status in ACTIVE:
                coverage.setdefault((module, anchor), set()).add(name)
        if status == "effective" and name not in links.get(spec, set()):
            report(name, "MAP_SPEC_PROVENANCE_INVALID")
        directory = document.path.parent.as_posix()
        hub = directory + "/README.md"
        hub_links, hub_body = links.get(hub, set()), bodies.get(hub, "")
        if name not in hub_links or s.get("issue") not in hub_links:
            report(hub, "MAP_HUB_INVALID")
        for kind in ("plan", "review", "evidence"):
            artifact = directory + f"/{kind}.md"
            if artifact in bodies:
                if artifact not in hub_links:
                    report(hub, "MAP_HUB_INVALID")
                other = documents.get(artifact)
                if other and other.front_matter.scalars.get("delivery_id") != s.get("delivery_id"):
                    report(artifact, "MAP_DELIVERY_ID_INVALID")
            elif not _has_status_text(hub_body, kind):
                report(hub, "MAP_HUB_INVALID")
        if not _has_status_text(hub_body, "PR"):
            report(hub, "MAP_HUB_INVALID")
    for owners in coverage.values():
        if len(owners) > 1:
            for name in owners:
                report(name, "MAP_ACTIVE_ANCHOR_CONFLICT")

    # Indexes may lead directly to a Change Hub (the bootstrap map does this).
    root_index = "map/README.md"
    if root_index not in bodies:
        report(root_index, "MAP_DOCUMENT_MISSING")
    change_directories = {Path(name).parent for name in bodies
                          if len(Path(name).parts) == 6 and Path(name).parts[3] == "changes"}
    for directory in change_directories:
        for filename in ("README.md", "change.md"):
            required = (directory / filename).as_posix()
            if required not in bodies:
                report(required, "MAP_DOCUMENT_MISSING")
    modules = {"/".join(Path(name).parts[:3]) for name in bodies if len(Path(name).parts) >= 4}
    for module in modules:
        index, spec = module + "/README.md", module + "/spec.md"
        if index not in bodies:
            report(index, "MAP_DOCUMENT_MISSING")
        if not any(link.startswith(module + "/") for link in links.get(root_index, set())):
            report(root_index, "MAP_HUB_INVALID")
        if spec in bodies and spec not in links.get(index, set()):
            report(index, "MAP_HUB_INVALID")
    return sorted(problems)


def _scaffold(args):
    root = Path(args.docs_root).resolve()
    delivery = DELIVERY.fullmatch(args.delivery_id)
    if not delivery:
        return "MAP_DELIVERY_ID_INVALID", "map"
    if args.issue != "https://gitee.com/ming-industry/" + args.delivery_id.replace("#", "/issues/"):
        return "MAP_ISSUE_URL_INVALID", "map"
    if not REPOSITORY.fullmatch(args.repository):
        return "MAP_REPOSITORY_INVALID", "map"
    if args.type not in TYPES:
        return "MAP_CHANGE_TYPE_INVALID", "map"
    if not _module(args.module) or not _filename(args.name):
        return "MAP_RELATED_PATH_INVALID", "map"
    directory = args.module + "/changes/" + args.type + "-" + args.delivery_id.replace("#", "-") + "_" + args.name
    try:
        target = _confined(root, directory, metadata=True)
        for filename in ("README.md", "change.md"):
            _confined(root, directory + "/" + filename, metadata=True)
        scan_problems = set()
        for path in _markdown_paths(root, scan_problems):
            if path.name != "change.md":
                continue
            try:
                fm, _ = parse_front_matter(path.read_text(encoding="utf-8"))
            except ValueError:
                return "MAP_FRONT_MATTER_INVALID", path.relative_to(root).as_posix()
            if fm.scalars.get("delivery_id") == args.delivery_id and path.parent != target:
                return "MAP_DELIVERY_DUPLICATE", directory
        if any(p.code == "MAP_RELATED_PATH_INVALID" for p in scan_problems):
            return "MAP_RELATED_PATH_INVALID", "map"
        spec = _confined(root, args.module + "/spec.md", metadata=True)
        anchors = []
        if not spec.is_file() and args.type == "FEATURE":
            anchors = ["__module_root__"]
        # Existing modules need an explicit author choice of coverage. Do not
        # silently claim all their anchors: the draft remains unapproved.
        anchor_lines = "\n".join("  - " + anchor for anchor in anchors)
        change = (f"---\ndocument_kind: change\nchange_type: {args.type.lower()}\nstatus: draft\n"
                  f"delivery_id: {args.delivery_id}\nissue: {args.issue}\nhub: {directory}/README.md\n"
                  f"target_module: {args.module}\naffected_repositories:\n  - {args.repository}\n"
                  f"affected_modules:\n  - {args.module}\ntarget_anchors:\n{anchor_lines}\n---\n\n"
                  f"# {args.name}\n\n[交付导航](./README.md)\n\n覆盖范围和语义变更待完善。\n")
        hub = (f"# {args.name}\n\n[Delivery Issue]({args.issue})\n\n[Change Spec](./change.md)\n\n"
               "Plan: 待编写\n\n" + f"PR ({args.repository}): 待创建\n\n"
               "Review: 待评审\n\nEvidence: 待验证\n")
        files = {target / "README.md": hub, target / "change.md": change}
        for path, content in files.items():
            if path.exists() and (not path.is_file() or path.read_text(encoding="utf-8") not in {"", content}):
                return "MAP_INIT_CONFLICT", path.relative_to(root).as_posix()
        existing = all(path.is_file() and path.read_text(encoding="utf-8") == content for path, content in files.items())
        if not existing:
            target.mkdir(parents=True, exist_ok=True)
            for path, content in files.items():
                if not path.exists() or not path.stat().st_size:
                    path.write_text(content, encoding="utf-8")
        print("map change: existing" if existing else "map change: created")
        return None
    except (ValueError, OSError, RuntimeError):
        return "MAP_RELATED_PATH_INVALID", directory


def _activation_table(body, columns):
    rows, header = [], None
    for line in re.sub(r"<!--.*?-->", "", _outside_fences(body), flags=re.S).splitlines():
        if not line.startswith("|"):
            header = None
            continue
        cells = [cell.strip().strip("`") for cell in line.strip().strip("|").split("|")]
        if set(columns).issubset(cells):
            if len(set(cells)) != len(cells):
                raise ValueError("duplicate columns")
            header = cells
        elif header and not all(re.fullmatch(r":?-+:?", cell) for cell in cells):
            if len(cells) != len(header):
                raise ValueError("table width")
            rows.append(dict(zip(header, cells)))
    return rows


def activate(docs_root, change_path):
    """Validate a candidate tree only; never query remotes or publish authority."""
    root = Path(docs_root).resolve()
    problems = set(validate(root))

    def report(path, code):
        problems.add(Problem(path, code))

    def read(path):
        return _confined(root, path, metadata=True).read_text(encoding="utf-8")

    try:
        directory = _confined(root, change_path, metadata=True).relative_to(root).as_posix()
        name = directory + "/change.md"
        fm, _ = parse_front_matter(read(name))
        if fm.scalars.get("document_kind") != "change":
            raise ValueError("not a change")
        if fm.scalars.get("status") != "effective":
            report(name, "MAP_ACTIVATION_STATUS_INVALID")
        module = fm.scalars["target_module"]
        hub = directory + "/README.md"
        hub_text = read(hub)
        rows = _activation_table(hub_text, ("role", "repository", "pr", "base", "exact_head", "status", "merge_revision"))
        if not rows or not any(row["role"] == "current" for row in rows):
            report(hub, "MAP_ACTIVATION_DEPENDENCY_INVALID")
        expected_heads = set()
        expected_repos = set(fm.lists.get("affected_repositories", ()))
        for row in rows:
            repo, head = row["repository"], row["exact_head"]
            expected_heads.add((repo, row["base"], head))
            pr_pattern = "https://" + re.escape(repo.removeprefix("git:")) + r"/pulls/[1-9][0-9]*"
            valid = (REPOSITORY.fullmatch(repo) and HEAD.fullmatch(row["base"])
                     and HEAD.fullmatch(head) and re.fullmatch(pr_pattern, row["pr"]))
            role, status, merge = row["role"], row["status"], row["merge_revision"]
            if role == "prerequisite":
                valid = valid and status == "merged" and HEAD.fullmatch(merge)
            elif role == "current":
                valid = valid and ((status in {"draft", "open"} and merge == "-")
                                   or (status == "merged" and HEAD.fullmatch(merge)))
            else:
                valid = False
            if not valid:
                report(hub, "MAP_ACTIVATION_DEPENDENCY_INVALID")
        if {row["repository"] for row in rows} != expected_repos:
            report(hub, "MAP_ACTIVATION_EVIDENCE_INVALID")
        for kind, verdict in (("review", "pass"), ("evidence", "verified")):
            path = directory + f"/{kind}.md"
            try:
                artifact, body = parse_front_matter(read(path))
                records = _activation_table(body, ("repository", "base", "exact_head", "verdict"))
                recorded = {(row["repository"], row["base"], row["exact_head"]) for row in records}
                if (artifact.scalars.get("status") != verdict or not expected_heads
                        or not expected_heads.issubset(recorded)
                        or any(row["verdict"] != verdict or not HEAD.fullmatch(row["base"])
                               or not HEAD.fullmatch(row["exact_head"]) for row in records)):
                    report(path, "MAP_ACTIVATION_EVIDENCE_INVALID")
            except FileNotFoundError:
                report(path, "MAP_DOCUMENT_MISSING")
            except ValueError:
                report(path, "MAP_ACTIVATION_EVIDENCE_INVALID")
        index = module + "/README.md"
        history = re.split(r"^#{1,6}\s+.*?(?:历史|History).*?$", _anchor_source(read(index)),
                           maxsplit=1, flags=re.M | re.I)
        section = re.split(r"^#{1,6}\s", history[-1], maxsplit=1, flags=re.M)[0] if len(history) == 2 else ""
        def points_to(body, base, target):
            for destination in _links(body):
                url = urlsplit(destination)
                if not url.scheme and not url.netloc:
                    if _confined(root, unquote(url.path), base=root / base) == root / target:
                        return True
            return False
        if not points_to(section, module, hub):
            report(index, "MAP_ACTIVATION_HISTORY_INVALID")
        spec = module + "/spec.md"
        source = _anchor_source(read(spec))
        parser = _AnchorParser(source)
        parser.feed(source)
        parser.close()
        declarations = parser.declarations
        targets = set(fm.lists.get("target_anchors", ()))
        for anchor in targets:
            matching = [pos for pos, match in enumerate(declarations) if match[1] == anchor]
            if len(matching) != 1:
                report(name, "MAP_TARGET_ANCHOR_INVALID")
                continue
            position = matching[0]
            start = declarations[position].end()
            end = declarations[position + 1].start() if position + 1 < len(declarations) else len(source)
            section = source[start:end]
            # The heading immediately following the anchor names its own
            # section; every later heading starts a different source scope.
            own_heading = re.match(r"\s*#{1,6}[^\n]*(?:\n|$)", section)
            offset = own_heading.end() if own_heading else 0
            next_heading = re.search(r"^ {0,3}#{1,6}\s", section[offset:], re.M)
            if next_heading:
                section = section[:offset + next_heading.start()]
            if not points_to(section, module, name):
                report(name, "MAP_SPEC_PROVENANCE_INVALID")
        for other in _markdown_paths(root, problems):
            if other.name != "change.md" or other == root / name:
                continue
            other_fm, _ = parse_front_matter(other.read_text(encoding="utf-8"))
            if (other_fm.scalars.get("status") in ACTIVE
                    and other_fm.scalars.get("target_module") == module
                    and targets.intersection(other_fm.lists.get("target_anchors", ()))):
                report(name, "MAP_ACTIVE_ANCHOR_CONFLICT")
    except (OSError, ValueError, KeyError, RuntimeError):
        report("map", "MAP_ACTIVATION_INPUT_INVALID")
    return sorted(problems)


def _source_sections(text):
    """Locate real anchors in masked Markdown, but compare original content.

    Code cannot declare an anchor; it can still carry normative configuration.
    Preserving offsets separates those two concerns without losing code edits.
    """
    source = _anchor_source(text, preserve_offsets=True)
    parser = _AnchorParser(source)
    parser.feed(source)
    parser.close()
    sections = {}
    for position, match in enumerate(parser.declarations):
        end = (parser.declarations[position + 1].start()
               if position + 1 < len(parser.declarations) else len(source))
        sections[match[1]] = text[match.end():end].strip()
    return sections


def check_candidate(docs_root, base=None):
    """Lint the whole tree and apply activation rules to this Git delta.

    No baseline means full activation validation, never implicit trust in HEAD.
    The default comparison is the merge-base with origin/master, so committed
    PR changes cannot disappear from checks after a local commit.
    """
    root = Path(docs_root).resolve()
    problems = set(validate(root))

    def git(*args):
        return subprocess.run(["git", "-C", str(root), *args], capture_output=True,
                              text=True, timeout=10, check=False)

    revision = None
    try:
        top = git("rev-parse", "--show-toplevel")
        is_repo = top.returncode == 0 and Path(top.stdout.strip()).resolve() == root
        if base is not None:
            resolved = git("rev-parse", "--verify", "--end-of-options", base + "^{commit}")
            if not is_repo or resolved.returncode or not HEAD.fullmatch(resolved.stdout.strip()):
                return sorted(problems | {Problem("map", "MAP_BASELINE_INVALID")})
            revision = resolved.stdout.strip()
        elif is_repo:
            resolved = git("merge-base", "origin/master", "HEAD")
            if resolved.returncode == 0 and HEAD.fullmatch(resolved.stdout.strip()):
                revision = resolved.stdout.strip()

        def previous(path):
            if revision is None:
                return ""
            result = git("show", revision + ":" + path)
            return result.stdout if result.returncode == 0 else ""

        for path in _markdown_paths(root, problems):
            if path.name != "change.md":
                continue
            name = path.relative_to(root).as_posix()
            current = path.read_text(encoding="utf-8")
            fm, _ = parse_front_matter(current)
            if fm.scalars.get("status") != "effective":
                continue
            spec = fm.scalars["target_module"] + "/spec.md"
            old_sections = _source_sections(previous(spec))
            spec_path = _confined(root, spec, metadata=True)
            new_sections = _source_sections(spec_path.read_text(encoding="utf-8")) if spec_path.is_file() else {}
            changed_sections = any(old_sections.get(anchor) != new_sections.get(anchor)
                                   for anchor in fm.lists.get("target_anchors", ()))
            if current != previous(name) or changed_sections or revision is None:
                problems.update(activate(root, path.parent.relative_to(root).as_posix()))
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError):
        problems.add(Problem("map", "MAP_BASELINE_INVALID"))
    return sorted(problems)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    check = commands.add_parser("check")
    check.add_argument("--docs-root", default=".")
    check.add_argument("--base", help="trusted Git baseline; defaults to merge-base(origin/master, HEAD)")
    activation = commands.add_parser("activate")
    activation.add_argument("--docs-root", default=".")
    activation.add_argument("--change", required=True)
    init = commands.add_parser("init")
    init.add_argument("--docs-root", default=".")
    for option in ("module", "type", "delivery-id", "name", "issue", "repository"):
        init.add_argument("--" + option, required=True)
    args = parser.parse_args(argv)
    if args.command == "init":
        error = _scaffold(args)
        if error:
            print(f"{error[0]} path={error[1]}")
            return 1
        return 0
    problems = activate(args.docs_root, args.change) if args.command == "activate" else check_candidate(args.docs_root, args.base)
    for problem in problems:
        display_path = json.dumps(problem.path, ensure_ascii=False)[1:-1]
        print(f"{problem.code} path={display_path}")
    if not problems:
        print("map activation: OK" if args.command == "activate" else "map docs: OK")
    return int(bool(problems))


if __name__ == "__main__":
    raise SystemExit(main())
