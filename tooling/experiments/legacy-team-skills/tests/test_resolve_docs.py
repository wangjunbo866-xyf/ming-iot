import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
SCRIPT = REPO / "skills/map-delivery/scripts/resolve_docs.py"
EXPECTED_IDENTITY = "git:gitee.com/ming-industry/ming-iot-rd-docs"
ORIGIN = "https://gitee.com/ming-industry/ming-iot-rd-docs.git"


class DocsResolverTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.workspace = self.root / "workspace"
        self.workspace.mkdir()
        self.environment = dict(
            os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
            GIT_OPTIONAL_LOCKS="0",
        )

    def git(self, repo, *arguments):
        return subprocess.run(
            ["git", "-C", str(repo), *arguments], check=True,
            capture_output=True, text=True, env=self.environment,
        ).stdout.strip()

    def make_docs(self, directory=None, origin=ORIGIN):
        repo = directory or self.workspace / "ming-iot-rd-docs"
        repo.mkdir(parents=True)
        self.git(repo, "init", "-q")
        self.git(repo, "remote", "add", "origin", origin)
        for name in ("AGENTS.md", "map/README.md", "tools/map_docs.py"):
            path = repo / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture\n", encoding="utf-8")
        self.git(repo, "add", ".")
        self.git(repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                 "-c", f"core.hooksPath={os.devnull}", "commit", "-qm", "fixture")
        return repo

    def run_resolver(self, *arguments, script=SCRIPT, cwd=None):
        return subprocess.run(
            [sys.executable, str(script), *map(str, arguments)],
            cwd=cwd or self.workspace, capture_output=True, text=True,
            env=self.environment,
        )

    def test_explicit_renamed_docs_root_reports_identity_and_exact_head(self):
        docs = self.make_docs(self.workspace / "renamed docs")
        result = self.run_resolver("--docs-root", docs, "--json")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {
            "docs_root": str(docs), "identity": EXPECTED_IDENTITY,
            "exact_head": self.git(docs, "rev-parse", "HEAD"),
        })

    def test_wrong_explicit_identity_does_not_fall_back_to_valid_sibling(self):
        self.make_docs()
        wrong = self.make_docs(self.workspace / "wrong", "https://gitee.com/other/docs.git")
        result = self.run_resolver("--docs-root", wrong, "--workspace-root", self.workspace)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertIn("身份不符", result.stderr)

    def test_missing_explicit_root_does_not_fall_back(self):
        self.make_docs()
        result = self.run_resolver("--docs-root", self.root / "missing")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")

    def test_explicit_subdirectory_is_not_a_worktree_root(self):
        docs = self.make_docs()
        result = self.run_resolver("--docs-root", docs / "map")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("根目录", result.stderr)

    def test_explicit_linked_worktree_preserves_candidate_head(self):
        docs = self.make_docs()
        linked = docs / ".worktrees/candidate"
        self.git(docs, "worktree", "add", "-q", "--detach", str(linked), "HEAD")
        (linked / "AGENTS.md").write_text("candidate governance\n", encoding="utf-8")
        self.git(linked, "add", "AGENTS.md")
        self.git(linked, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                 "-c", f"core.hooksPath={os.devnull}", "commit", "-qm", "candidate")
        result = self.run_resolver("--docs-root", linked, "--json")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual(data["docs_root"], str(linked))
        self.assertEqual(data["exact_head"], self.git(linked, "rev-parse", "HEAD"))
        self.assertNotEqual(data["exact_head"], self.git(docs, "rev-parse", "HEAD"))

    def test_missing_required_map_fails(self):
        docs = self.make_docs()
        (docs / "map/README.md").unlink()
        result = self.run_resolver("--docs-root", docs)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("map/README.md", result.stderr)

    def test_origin_url_forms_identify_same_repository(self):
        docs = self.make_docs()
        for origin in (
            "https://gitee.com/ming-industry/ming-iot-rd-docs",
            "ssh://git@gitee.com/ming-industry/ming-iot-rd-docs.git",
            "git@gitee.com:ming-industry/ming-iot-rd-docs.git",
        ):
            with self.subTest(origin=origin):
                self.git(docs, "remote", "set-url", "origin", origin)
                result = self.run_resolver("--docs-root", docs, "--json")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout)["identity"], EXPECTED_IDENTITY)

    def test_multiple_fetch_destinations_with_different_identity_fail(self):
        docs = self.make_docs()
        self.git(docs, "remote", "set-url", "--add", "origin", "https://gitee.com/other/docs.git")
        result = self.run_resolver("--docs-root", docs)
        self.assertNotEqual(result.returncode, 0)

    def test_remote_credentials_are_never_reported(self):
        fake_secret = "fixture-secret-not-a-real-token"
        docs = self.make_docs(origin=f"https://fixture:{fake_secret}@gitee.com/other/docs.git")
        result = self.run_resolver("--docs-root", docs)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(fake_secret, result.stdout + result.stderr)
        self.assertNotIn("https://", result.stdout + result.stderr)

    def test_arbitrary_install_path_uses_product_start_for_sibling_discovery(self):
        docs = self.make_docs()
        start = self.workspace / "product/backend/module"
        start.mkdir(parents=True)
        copied = self.root / "unrelated-install/scripts/resolve_docs.py"
        copied.parent.mkdir(parents=True)
        shutil.copy2(SCRIPT, copied)
        result = self.run_resolver("--start", start, script=copied, cwd=self.root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(docs))

    def test_default_current_directory_finds_nearest_docs_workspace(self):
        self.make_docs()
        inner = self.workspace / "nested-workspace"
        inner.mkdir()
        nearest = self.make_docs(inner / "ming-iot-rd-docs")
        start = inner / "product/src"
        start.mkdir(parents=True)
        result = self.run_resolver(cwd=start)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(nearest))

    def test_discovery_from_docs_subdirectory_keeps_current_worktree(self):
        docs = self.make_docs(self.workspace / "renamed")
        result = self.run_resolver("--start", docs / "map")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(docs))

    def test_ambiguous_same_level_roots_require_explicit_selection(self):
        outer = self.make_docs(self.workspace / "docs-workspace")
        self.make_docs(outer / "ming-iot-rd-docs")
        result = self.run_resolver("--workspace-root", outer)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("多个", result.stderr)
        explicit = self.run_resolver("--docs-root", outer)
        self.assertEqual(explicit.returncode, 0, explicit.stderr)

    def test_workspace_discovery_does_not_scan_nested_directories(self):
        self.make_docs(self.workspace / "nested/ming-iot-rd-docs")
        result = self.run_resolver("--workspace-root", self.workspace)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("未找到", result.stderr)

    def test_resolver_does_not_modify_worktree_or_local_git_configuration(self):
        docs = self.make_docs()
        paths = [docs / name for name in ("AGENTS.md", "map/README.md", "tools/map_docs.py",
                                         ".git/config", ".git/HEAD", ".git/index")]
        before = {path: (path.read_bytes(), path.stat().st_mtime_ns) for path in paths}
        result = self.run_resolver("--docs-root", docs, "--json")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(before, {path: (path.read_bytes(), path.stat().st_mtime_ns)
                                  for path in paths})


if __name__ == "__main__":
    unittest.main()
