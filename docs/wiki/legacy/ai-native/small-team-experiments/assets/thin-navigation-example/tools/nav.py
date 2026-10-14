#!/usr/bin/env python3
"""Thin-navigation example, Python stdlib only. Not an OpenSpec parser/validator.

Supports this fixture's Markdown shape; demonstrates links, not spec merging.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys

BEGIN = "<!-- thin-nav:begin -->"
END = "<!-- thin-nav:end -->"
NAV = re.compile(r"\n<!-- thin-nav:begin -->\n.*?\n<!-- thin-nav:end -->\n", re.S)
SLUG = re.compile(r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*\Z")
CAP = re.compile(r"[a-z][a-z0-9]*(?:[-/][a-z0-9]+)*\Z")


def without_nav(text):
    if text.count(BEGIN) != text.count(END) or text.count(BEGIN) > 1:
        raise ValueError("导航区块边界不完整或重复")
    cleaned = NAV.sub("", text)
    if BEGIN in cleaned or END in cleaned:
        raise ValueError("导航区块格式不受样例支持")
    return cleaned


def structure(text):
    """Ignore fenced examples and comments when reading headings."""
    text = re.sub(r"<!--.*?-->", "", text, flags=re.S)
    lines, fence = [], None
    for line in text.splitlines():
        match = re.match(r"^\s*(`{3,}|~{3,})", line)
        if match:
            token = match[1]
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            lines.append("")
        else:
            lines.append("" if fence else line)
    return "\n".join(lines)


def declared_capabilities(text):
    result, in_caps, in_section = set(), False, False
    for line in structure(text).splitlines():
        if line.startswith("## "):
            in_caps = line.strip() == "## Capabilities"
            in_section = False
        elif line.startswith("### "):
            in_section = in_caps and line.strip() in {
                "### New Capabilities", "### Modified Capabilities"
            }
        elif in_section:
            match = re.match(r"^- `([^`]+)`:", line)
            if match:
                result.add(match[1])
    return result


def operations(text):
    result, op = [], None
    for line in structure(text).splitlines():
        heading = re.match(r"^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements\s*$", line)
        if line.startswith("## "):
            op = heading[1] if heading else None
        req = re.match(r"^### Requirement:\s*(.+)", line)
        if op and req:
            result.append((op, req[1].strip()))
        elif op == "RENAMED" and re.match(r"^- (FROM|TO):", line):
            result.append((op, line[2:].strip()))
    if not result:
        raise ValueError("delta 无可识别要求；需人工核对格式，不能静默忽略")
    return result


def insert_nav(text, section, lines):
    text = without_nav(text)
    marker = f"## {section}\n"
    if structure(text).splitlines().count(marker.strip()) != 1:
        raise ValueError(f"需要唯一的 {marker.strip()} 节")
    block = "\n".join([BEGIN, *lines, END])
    return text.replace(marker, marker + "\n" + block + "\n", 1)


def generate(root):
    sources, outputs, changes = {}, {}, {}

    def read(path):
        value = without_nav((root / path).read_text(encoding="utf-8"))
        sources[path] = value
        return value

    def local(path):
        candidate = (root / path).resolve()
        if not candidate.is_relative_to(root):
            raise ValueError(f"样例只允许文档根内部路径: {path}")
        return candidate

    catalog = json.loads(read("map/catalog.json"))
    caps = {c["id"]: c for c in catalog["capabilities"]}
    if len(caps) != len(catalog["capabilities"]):
        raise ValueError("catalog capability ID 重复")
    for cap, info in caps.items():
        if not CAP.fullmatch(cap) or not SLUG.fullmatch(info["domain"]):
            raise ValueError(f"非法 capability/domain: {cap}")
        if info["domain"] not in catalog["domains"]:
            raise ValueError(f"业务域未登记: {info['domain']}")
        info["page"] = f"map/{info['domain']}/{cap}.md"
        info["spec"] = f"openspec/specs/{cap}/spec.md"
        if local(info["spec"]).is_file():
            read(info["spec"])
        for item in info.get("links", []):
            if not local(item["path"]).is_file():
                raise ValueError(f"上下文入口不存在: {item['path']}")

    base = root / "openspec/changes"
    active = sorted(p for p in base.iterdir() if p.is_dir() and p.name != "archive" and not p.name.startswith("."))
    archived = sorted(p for p in (base / "archive").iterdir() if p.is_dir()) if (base / "archive").exists() else []
    for folder in [*active, *archived]:
        archived_date = None
        identity = folder.name
        if folder.parent.name == "archive":
            match = re.fullmatch(r"(\d{4}-\d{2}-\d{2})-([a-z].*)", identity)
            if not match:
                raise ValueError(f"归档名称需要显式映射，样例不猜测: {identity}")
            archived_date, identity = match.groups()
        if not SLUG.fullmatch(identity):
            raise ValueError(f"样例 Change ID 必须字母开头、不带日期: {identity}")
        if identity in changes:
            raise ValueError(f"Change ID 重复（不可复用）: {identity}")
        directory = folder.relative_to(root).as_posix()
        proposal_path = f"{directory}/proposal.md"
        proposal = read(proposal_path)
        metadata = read(f"{directory}/.openspec.yaml")
        skip = bool(re.search(r"^skip_specs:\s*true\s*(?:#.*)?$", metadata, re.M))
        extra = re.findall(r"<!-- thin-nav-meta:\s*(.*?)\s*-->", proposal, re.S)
        if len(extra) > 1:
            raise ValueError(f"关系标记重复: {identity}")
        related = json.loads(extra[0]).get("related_capabilities", []) if extra else []
        if not isinstance(related, list) or any(not isinstance(c, str) for c in related):
            raise ValueError(f"related_capabilities 应为字符串列表: {identity}")
        declared = declared_capabilities(proposal)
        deltas = {}
        for path in sorted((folder / "specs").rglob("spec.md")):
            capability = path.parent.relative_to(folder / "specs").as_posix()
            rel = path.relative_to(root).as_posix()
            deltas[capability] = {"path": rel, "operations": operations(read(rel))}
        affected = declared | set(deltas) | set(related)
        if not affected:
            raise ValueError(f"变更尚未关联能力，不能从视图静默消失: {identity}")
        unknown = affected - caps.keys()
        if unknown:
            raise ValueError(f"能力尚未登记导航: {', '.join(sorted(unknown))}")
        if skip and deltas:
            raise ValueError(f"skip_specs 与已有差量不一致，需核对: {identity}")
        if not deltas and not declared and not skip:
            raise ValueError(f"仅关联实现时需明确 skip_specs: true: {identity}")
        if set(related) & (declared | set(deltas)):
            raise ValueError(f"重复声明能力关系；已有 delta/Capabilities 的不再填 related: {identity}")
        changes[identity] = {
            "id": identity, "date": archived_date, "dir": directory,
            "proposal": proposal_path, "affected": sorted(affected),
            "deltas": deltas, "declared": declared, "skip": skip,
            "page": f"map/changes/{identity}.md",
            "title": next((s[2:].strip() for s in structure(proposal).splitlines() if s.startswith("# ")), identity),
            "type": identity.split("-", 1)[0].upper() if identity.startswith(("feature-", "patch-", "sunset-")) else "未分类",
        }
        for artifact in ("design.md", "tasks.md"):
            target = f"{directory}/{artifact}"
            if (root / target).is_file():
                read(target)

    # Losing an already published stable address must be an explicit decision.
    for page in (root / "map/changes").glob("*.md"):
        if page.stem not in changes:
            raise ValueError(f"稳定入口失去来源，需保留记录或明确墓碑页: {page.name}")

    fingerprint = hashlib.sha256(json.dumps(sources, ensure_ascii=False, sort_keys=True).encode()).hexdigest()[:16]
    banner = f"> 自动生成；输入指纹 `{fingerprint}`。目录位置不代表同步、批准或发布。\n\n"

    def link(source, target, label):
        return f"[{label}]({Path(os.path.relpath(root / target, (root / source).parent)).as_posix()})"

    def scope(change, cap):
        if cap in change["deltas"]:
            return "；".join(f"{op} {name}" for op, name in change["deltas"][cap]["operations"]).replace("|", "\\|")
        return "声明影响；差量尚未编写" if cap in change["declared"] else "实现关联；无行为差量"

    for cap, info in caps.items():
        page, spec = info["page"], info["spec"]
        current = link(page, spec, "读取当前完整规格") if spec in sources else "未发现当前主规格；核对新增、退役或遗漏"
        rows = [f"# {info['title']}\n", banner, f"{info['summary']}\n", f"能力：`{catalog['root_id']}:{cap}`\n", f"现行 Feature：{current}\n", "## 上下文入口\n"]
        rows += [f"- {link(page, entry['path'], entry['title'])}" for entry in info.get("links", [])]
        for archived_flag, heading in ((False, "活动变更"), (True, "归档历史")):
            rows += [f"\n## {heading}\n", "| 变更 | 分类 | 关联范围 | 位置 |", "| --- | --- | --- | --- |"]
            selected = [c for c in changes.values() if cap in c["affected"] and bool(c["date"]) == archived_flag]
            for change in sorted(selected, key=lambda c: (c["date"] or "", c["id"]), reverse=True):
                location = f"归档 {change['date']}" if change["date"] else "活动目录"
                rows.append(f"| {link(page, change['page'], change['id'])} | {change['type']} | {scope(change, cap)} | {location} |")
            if not selected:
                rows.append("| 无 | — | — | — |")
        rows += ["\n历史用于追溯，不与现行规格自动叠加。修改本次需求时再读取选定 Change。\n"]
        outputs[page] = "\n".join(rows)
        if spec in sources:
            outputs[spec] = insert_nav(sources[spec], "Purpose", [link(spec, page, "功能导航：上下文、相关变更与历史")])

    for change in changes.values():
        page, proposal = change["page"], change["proposal"]
        rows = [f"# {change['title']}\n", banner, f"稳定标识：`{catalog['root_id']}:{change['id']}`\n", f"分类：{change['type']}；位置：{'归档 ' + change['date'] if change['date'] else '活动目录'}\n", "## 原始产物\n"]
        rows.append(f"- {link(page, proposal, 'proposal：意图与影响')}")
        for filename in ("design.md", "tasks.md"):
            target = f"{change['dir']}/{filename}"
            if target in sources:
                rows.append(f"- {link(page, target, filename)}")
        rows += ["\n## 关联功能\n"]
        nav_lines = [f"- {link(proposal, page, '此变更的稳定入口')}（自动导航）"]
        for cap in change["affected"]:
            info = caps[cap]
            current = link(page, info["spec"], "当前规格") if info["spec"] in sources else "当前规格未找到"
            rows.append(f"- {link(page, info['page'], info['title'])} · {current} · {scope(change, cap)}")
            if cap in change["deltas"]:
                rows.append(f"  - {link(page, change['deltas'][cap]['path'], '该能力的 delta')}")
            line = f"- {link(proposal, info['page'], info['title'] + '功能页')}"
            if info["spec"] in sources:
                line += f" · {link(proposal, info['spec'], '当前完整规格')}"
            nav_lines.append(line)
        rows += ["\n基线 revision、独立审查、文档整合及发布证据：此演示未提供，不从目录或任务勾选推断。\n"]
        outputs[page] = "\n".join(rows)
        outputs[proposal] = insert_nav(sources[proposal], "Impact", nav_lines)

    overview = ["# 业务地图\n", banner]
    for domain, name in sorted(catalog["domains"].items()):
        domain_page = f"map/{domain}/README.md"
        overview.append(f"- {link('map/README.md', domain_page, name)}")
        rows = [f"# {name}\n", banner]
        rows += [f"- {link(domain_page, c['page'], c['title'])}" for c in caps.values() if c["domain"] == domain]
        outputs[domain_page] = "\n".join(rows) + "\n"
    outputs["map/README.md"] = "\n".join(overview) + "\n"

    for source, value in outputs.items():
        for target in re.findall(r"\[[^\]]*\]\(([^)]+)\)", structure(value)):
            if re.match(r"[a-z]+://", target) or target.startswith("#"):
                continue
            path = ((root / source).parent / target.split("#", 1)[0]).resolve()
            if not path.is_relative_to(root):
                raise ValueError(f"链接越出样例根: {source}: {target}")
            relative = path.relative_to(root).as_posix()
            if relative not in outputs and not path.is_file():
                raise ValueError(f"失效链接: {source}: {target}")
    return outputs, changes, fingerprint


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "check"))
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()
    root = args.root.resolve()
    try:
        outputs, changes, fingerprint = generate(root)
        stale = [p for p, value in outputs.items() if not (root / p).exists() or (root / p).read_text(encoding="utf-8") != value]
        if args.action == "check" and stale:
            raise ValueError("索引或导航过期: " + ", ".join(stale))
        if args.action == "build":
            for path in stale:
                target = root / path
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(outputs[path], encoding="utf-8")
        print(f"{args.action}: {len(changes)} changes; {len(outputs)} outputs; {len(stale)} updates; input {fingerprint}")
    except (OSError, ValueError, KeyError, TypeError) as exc:
        print(f"nav: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
