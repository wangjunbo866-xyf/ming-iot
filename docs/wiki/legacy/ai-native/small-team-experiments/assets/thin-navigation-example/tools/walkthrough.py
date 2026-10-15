#!/usr/bin/env python3
"""Run navigation scenarios in temporary copies; never archive the source fixture."""
from contextlib import contextmanager
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
from nav import generate, without_nav

SOURCE = Path(__file__).resolve().parents[1]
SCRIPT = SOURCE / "tools/nav.py"
PATCH = "patch-temperature-decimal"
ARCHIVE = f"openspec/changes/archive/2026-10-10-{PATCH}"


@contextmanager
def example():
    with tempfile.TemporaryDirectory(prefix="thin-nav-walkthrough-") as temp:
        root = Path(temp) / "example"
        shutil.copytree(SOURCE, root, ignore=shutil.ignore_patterns("__pycache__"))
        yield root


def run(root, action, expected=0):
    result = subprocess.run([sys.executable, str(SCRIPT), action, "--root", str(root)], capture_output=True, text=True)
    if result.returncode != expected:
        raise AssertionError(f"{action}: expected {expected}, got {result.returncode}: {result.stdout}{result.stderr}")
    return result.stdout + result.stderr


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def main():
    results = []
    with example() as root:
        run(root, "check")
        before = {p.relative_to(root): p.read_bytes() for p in root.rglob("*.md")}
        run(root, "build")
        after = {p.relative_to(root): p.read_bytes() for p in root.rglob("*.md")}
        require(before == after, "unchanged input caused Markdown churn")
        outputs, changes, _ = generate(root)
        ingest = outputs["map/data/telemetry-ingest.md"]
        query = outputs["map/data/telemetry-query.md"]
        require(len(changes) == 4, "expected four source changes")
        require(all(c in ingest for c in changes), "feature/patch history missing from ingest")
        require(PATCH in query and "feature-telemetry-ingest" in query, "cross-capability relation missing")
        results.append("PASS 原生目录聚合：4 个 Change；同一 Patch 出现在两个功能页；重复生成无差异。")
        require("patch-telemetry-buffer" in ingest and "patch-telemetry-buffer" not in query, "no-delta change misassociated")
        require("实现关联；无行为差量" in ingest, "no-delta change mislabeled")
        results.append("PASS 无差量修复：skip_specs + 单次 Impact 关联被收录，没有伪造业务差量。")

    with example() as root:
        folder = root / "openspec/changes/patch-proposal-only"
        folder.mkdir()
        (folder / ".openspec.yaml").write_text("schema: spec-driven\ncreated: 2026-10-10\n")
        (folder / "proposal.md").write_text("# 只有 proposal 的草案\n\n## Why\n演示\n\n## What Changes\n待补\n\n## Capabilities\n\n### Modified Capabilities\n- `telemetry-ingest`: 草案\n\n## Impact\n尚未实现。\n")
        output, _, _ = generate(root)
        require("声明影响；差量尚未编写" in output["map/data/telemetry-ingest.md"], "proposal-only change hidden")
        results.append("PASS 草案阶段：尚未编写 delta 的 Capabilities 声明仍可发现，明确标为待补差量。")

    with example() as root:
        source = root / f"openspec/changes/{PATCH}"
        body_before = {p.relative_to(source): without_nav(p.read_text()) for p in source.rglob("*.md")}
        specs_before = {p.relative_to(root): p.read_bytes() for p in (root / "openspec/specs").rglob("*.md")}
        stable_page = root / f"map/changes/{PATCH}.md"
        shutil.move(str(source), str(root / ARCHIVE))
        require("过期" in run(root, "check", 1), "stale index was not rejected")
        run(root, "build")
        run(root, "check")
        require(stable_page.is_file() and ARCHIVE in stable_page.read_text(), "stable page failed to follow archive")
        archived = root / ARCHIVE
        body_after = {p.relative_to(archived): without_nav(p.read_text()) for p in archived.rglob("*.md")}
        require(body_before == body_after, "business body changed while fixing links")
        require(specs_before == {p.relative_to(root): p.read_bytes() for p in (root / "openspec/specs").rglob("*.md")}, "navigation changed current behavior")
        require("../../../../map/changes/patch-temperature-decimal.md" in (archived / "proposal.md").read_text(), "archive backlink depth was not repaired")
        results.append("PASS 模拟归档：旧索引被拒绝；重建后稳定入口地址不变，正反链接有效，导航区块外正文完全相同。")
        results.append("PASS 语义边界：仅移动目录没有改变现行整数规格，也没有生成已同步、已批准或已发布结论。")

    with example() as root:
        shutil.copytree(root / f"openspec/changes/{PATCH}", root / ARCHIVE)
        require("Change ID 重复" in run(root, "build", 1), "duplicate logical ID accepted")
        results.append("PASS 标识冲突：活动与归档出现同名 Change 时拒绝生成，不按日期选一个。")

    with example() as root:
        shutil.rmtree(root / f"openspec/changes/{PATCH}")
        require("稳定入口失去来源" in run(root, "build", 1), "orphaned stable page silently removed")
        results.append("PASS 来源丢失：删除已被引用的 Change 后拒绝静默移除稳定入口，要求明确处置。")

    with example() as root:
        path = root / "openspec/changes/patch-telemetry-buffer/proposal.md"
        text = path.read_text().replace('<!-- thin-nav-meta: {"related_capabilities": ["telemetry-ingest"]} -->', "")
        path.write_text(text)
        require("尚未关联能力" in run(root, "check", 1), "unlinked change silently ignored")
        results.append("PASS 关联遗漏：无 delta、无能力关联时明确报错，不从功能页静默消失。")

    for result in results:
        print(result)
    print(f"{len(results)} 个检查通过。仅检验导航；归档用目录移动模拟，未调用 OpenSpec CLI，也不证明业务验收。")


if __name__ == "__main__":
    main()
