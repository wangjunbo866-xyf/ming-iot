import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "skills/map-delivery/scripts/context.py"
spec = importlib.util.spec_from_file_location("map_context", SCRIPT)
context = importlib.util.module_from_spec(spec)
spec.loader.exec_module(context)


class ContextTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.project = self.base / "product"
        self.project.mkdir()

    def config(self, **kwargs):
        data = dict(schema_version=1, **kwargs)
        (self.project / ".map-delivery.json").write_text(json.dumps(data), encoding="utf-8")

    def test_default_does_not_create_map_or_config(self):
        before = list(self.project.rglob("*"))
        result = context.resolve(self.project)
        self.assertFalse(result["map_exists"])
        self.assertEqual(result["map_root"], str(self.project / "map"))
        self.assertEqual(before, list(self.project.rglob("*")))

    def test_monorepo_custom_map_and_language(self):
        self.config(map_root="docs/business-map", language="English")
        path = self.project / "docs/business-map"
        path.mkdir(parents=True)
        (path / "README.md").write_text("# Map\n", encoding="utf-8")
        result = context.resolve(self.project)
        self.assertTrue(result["map_index_exists"])
        self.assertEqual(result["language"], "English")

    def test_separate_docs_repo(self):
        docs = self.base / "docs"
        docs.mkdir()
        self.config(docs_root="../docs")
        self.assertEqual(context.resolve(self.project)["docs_root"], str(docs))

    def test_explicit_candidate_worktree_wins(self):
        docs = self.base / "candidate"
        docs.mkdir()
        self.config(docs_root="../missing-main", map_root="business-map")
        result = context.resolve(self.project, docs)
        self.assertEqual(result["map_root"], str(docs / "business-map"))
        self.assertEqual(result["selection"], "explicit_docs_root")

    def test_missing_configured_root_never_falls_back(self):
        self.config(docs_root="../missing")
        (self.project / "map").mkdir()
        with self.assertRaises(FileNotFoundError):
            context.resolve(self.project)

    def test_malformed_config_never_falls_back(self):
        (self.project / ".map-delivery.json").write_text("{bad", encoding="utf-8")
        with self.assertRaises(context.ContextError):
            context.resolve(self.project, self.project)

    def test_duplicate_keys_rejected(self):
        (self.project / ".map-delivery.json").write_text('{"schema_version":1,"docs_root":".","docs_root":"../other"}')
        with self.assertRaises(context.ContextError):
            context.resolve(self.project)

    def test_bad_schema_or_fields_rejected(self):
        for data in [{"schema_version": True}, {"schema_version": 2}, {"schema_version": 1, "run": "echo bad"}]:
            with self.subTest(data=data):
                (self.project / ".map-delivery.json").write_text(json.dumps(data))
                with self.assertRaises(context.ContextError):
                    context.resolve(self.project)

    def test_map_cannot_escape_docs(self):
        for value in ["../outside", "/tmp/map", "C:/map", "map\\other"]:
            with self.subTest(value=value):
                self.config(map_root=value)
                with self.assertRaises(context.ContextError):
                    context.resolve(self.project)

    def test_symlink_map_cannot_escape(self):
        outside = self.base / "outside"
        outside.mkdir()
        (self.project / "map").symlink_to(outside, target_is_directory=True)
        with self.assertRaises(context.ContextError):
            context.resolve(self.project)

    def test_symlink_config_cannot_escape(self):
        outside = self.base / "config.json"
        outside.write_text('{"schema_version":1}')
        (self.project / ".map-delivery.json").symlink_to(outside)
        with self.assertRaises(context.ContextError):
            context.resolve(self.project)

    def test_local_git_without_remote_is_supported(self):
        def git(*args):
            return subprocess.run(["git", "-C", str(self.project), *args], capture_output=True, text=True, check=True).stdout.strip()
        git("init", "-q")
        git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "fixture")
        (self.project / "uncommitted.txt").write_text("local change")
        observation = context.resolve(self.project)["docs_revision"]
        self.assertEqual(observation["head"], git("rev-parse", "HEAD"))
        self.assertEqual(observation["working_tree"], "not_checked")

    def test_cli_json_and_missing_root_exit(self):
        import sys
        good = subprocess.run([sys.executable, str(SCRIPT), "--project-root", str(self.project), "--json"], capture_output=True, text=True)
        self.assertEqual(good.returncode, 0)
        self.assertEqual(json.loads(good.stdout)["scope"], "local_location_only")
        bad = subprocess.run([sys.executable, str(SCRIPT), "--project-root", str(self.base / "missing")], capture_output=True, text=True)
        self.assertEqual(bad.returncode, 1)


if __name__ == "__main__":
    unittest.main()
