import json
import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
APPROVED_ORIGIN = "https://gitee.com/ming-industry/ming-iot-rd-docs.git"


def git(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(REPO), *args],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


def run_hook(remote_url: str) -> subprocess.CompletedProcess[str]:
    hook = REPO / ".githooks" / "pre-push"
    return subprocess.run(
        [str(hook), "origin", remote_url],
        check=False,
        text=True,
        input="",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env={**os.environ, "LC_ALL": "C"},
    )


class RepositoryPolicyTest(unittest.TestCase):
    def test_repository_uses_only_origin(self) -> None:
        result = git("remote")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ["origin"])

    def test_repository_uses_approved_origin(self) -> None:
        approved_urls = ([APPROVED_ORIGIN], [APPROVED_ORIGIN.removesuffix(".git")])
        fetch_result = git("remote", "get-url", "--all", "origin")
        self.assertEqual(fetch_result.returncode, 0, fetch_result.stderr)
        self.assertIn(fetch_result.stdout.splitlines(), approved_urls)

        push_result = git("remote", "get-url", "--push", "--all", "origin")
        self.assertEqual(push_result.returncode, 0, push_result.stderr)
        self.assertIn(push_result.stdout.splitlines(), approved_urls)

    def test_repository_uses_local_hooks(self) -> None:
        result = git("config", "--get", "core.hooksPath")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), ".githooks")

    def test_pre_push_is_executable(self) -> None:
        hook = REPO / ".githooks" / "pre-push"
        mode = hook.stat().st_mode
        self.assertTrue(mode & stat.S_IXUSR)

    def test_pre_push_allows_approved_origin(self) -> None:
        result = run_hook(APPROVED_ORIGIN)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_pre_push_rejects_unapproved_remote(self) -> None:
        result = run_hook("https://example.invalid/ming-iot-rd-docs.git")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("may only be pushed", result.stderr)

    def test_pre_push_checks_map_from_git_root_and_blocks_invalid_tree(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            shutil.copytree(REPO / "map", root / "map")
            for directory in ("specs", "plans", "meta"):
                shutil.copytree(REPO / directory, root / directory)
            shutil.copy2(REPO / "AGENTS.md", root / "AGENTS.md")
            (root / "tools").mkdir()
            checker = REPO / "tools/map_docs.py"
            self.assertTrue(checker.is_file(), "map checker must ship with the hook")
            shutil.copy2(checker, root / "tools/map_docs.py")
            hook = root / "pre-push"
            shutil.copy2(REPO / ".githooks/pre-push", hook)
            nested = root / "nested"
            nested.mkdir()

            def push_check():
                return subprocess.run(
                    [str(hook), "origin", APPROVED_ORIGIN], cwd=nested,
                    input="", text=True, capture_output=True, check=False,
                )

            valid = push_check()
            self.assertEqual(valid.returncode, 0, valid.stderr + valid.stdout)
            self.assertIn("map docs: OK", valid.stdout)
            change = next((root / "map").rglob("change.md"))
            change.write_text("---\ndocument_kind: change\nstatus: bogus\n---\n", encoding="utf-8")
            invalid = push_check()
            self.assertNotEqual(invalid.returncode, 0)
            self.assertIn("MAP_", invalid.stdout + invalid.stderr)

    def test_map_workflow_runs_required_checks_for_master_pull_requests(self) -> None:
        workflow = REPO / ".workflow/map-docs-check.yml"
        self.assertTrue(workflow.is_file(), "map PR workflow must exist")
        source = workflow.read_text(encoding="utf-8")
        for command in (
            "python -m unittest discover -s tests -v",
            "python tools/check_delivery_metadata.py --docs-root . --since 2026-09-03",
            "python tools/map_docs.py check --docs-root .",
            "git diff --check",
        ):
            self.assertIn(command, source)
        self.assertIn("triggers:\n  pr:\n    branches:\n      precise:\n        - master", source)
        self.assertIn("blocking: true", source)
        self.assertIn("set -eu", source)
        self.assertNotIn("GITEE_ACCESS_TOKEN", source)
        self.assertNotIn("../ming-iot", source)

    def test_map_skill_has_a_tracked_external_source_contract(self) -> None:
        contract = REPO / "meta/SKILLS_SOURCE.json"
        self.assertTrue(contract.is_file())
        self.assertFalse(contract.is_symlink())
        source = json.loads(contract.read_text(encoding="utf-8"))
        self.assertEqual(source, {
            "schema_version": 1,
            "repository": "git:gitee.com/ming-industry/ai-native",
            "directory": "ai-native",
            "skills": {"map-delivery": "skills/map-delivery"},
            "lock_file": "skills.lock.json",
        })
        tracked = git("ls-files", "--error-unmatch", "meta/SKILLS_SOURCE.json")
        self.assertEqual(tracked.returncode, 0, "external source contract must be tracked")
        for directory in (".agents/skills/map-delivery", ".claude/skills/map-delivery", "skills/map-delivery"):
            copies = git("ls-files", "--", directory)
            self.assertEqual(copies.returncode, 0, copies.stderr)
            self.assertEqual(copies.stdout.strip(), "", "documentation repo must not track another skill source")

    def test_required_directories_exist(self) -> None:
        for relative in (
            "adr",
            "rfcs",
            "specs",
            "plans",
            "evidence",
            "reviews",
            "proposals",
            "archives",
        ):
            self.assertTrue((REPO / relative).is_dir(), relative)

    def test_agents_authorization_matches_candidate_pr_delivery_policy(self) -> None:
        agents = (REPO / "AGENTS.md").read_text(encoding="utf-8")
        self.assertNotIn("未经用户明确授权，不得执行 `git add` 或 `git commit`", agents)
        self.assertIn("自行 git add、commit、推送候选分支并创建或更新 Draft PR", agents)
        self.assertIn("禁止直接 push `master`", agents)
        self.assertIn("凭据创建、读取、轮换和权限变更", agents)
        self.assertIn("tools/gitee_pr.py", agents)
        self.assertIn("人在 Gitee 执行合并本身即为该次合并授权", agents)
        self.assertIn("绑定精确 PR、Head、目标分支和合并方式", agents)


if __name__ == "__main__":
    unittest.main()
