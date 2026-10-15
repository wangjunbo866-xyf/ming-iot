import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ENTRY = Path(__file__).resolve().parents[1] / "loopx"


class LoopXEntryTests(unittest.TestCase):
    def test_worktree_uses_main_private_route_without_creating_state(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "repo"
            root.mkdir()
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / "tooling").mkdir()
            shutil.copy2(ENTRY, root / "tooling/loopx")
            subprocess.run(["git", "-C", str(root), "add", "tooling/loopx"], check=True)
            subprocess.run(["git", "-C", str(root), "-c", "user.name=Test", "-c",
                            "user.email=test@example.invalid", "commit", "-qm", "entry"], check=True)
            worktree = Path(tmp) / "worktree"
            subprocess.run(["git", "-C", str(root), "worktree", "add", "--detach", "-q", str(worktree)], check=True)
            (root / ".local").mkdir()
            registry = Path(tmp) / "existing-registry.json"
            registry.write_text("{}\n")
            binary = Path(tmp) / "loopx-stub"
            binary.write_text("#!/usr/bin/env python3\nimport json,os,sys\nprint(json.dumps({'args':sys.argv[1:],'cwd':os.getcwd()}))\n")
            binary.chmod(0o755)
            config = root / ".local/loopx.json"
            config.write_text(json.dumps({"binary": str(binary), "registry": str(registry)}))
            result = subprocess.run([str(worktree / "tooling/loopx"), "status", "--goal-id", "existing"],
                                    cwd=worktree, text=True, capture_output=True, check=True)
            self.assertEqual(json.loads(result.stdout), {
                "args": ["--registry", str(registry), "status", "--goal-id", "existing"],
                "cwd": str(worktree),
            })
            self.assertFalse((worktree / ".loopx").exists())
            self.assertFalse((worktree / ".local").exists())
            self.assertEqual(registry.read_text(), "{}\n")
            for invalid in [None, "{", json.dumps({"binary": str(binary), "registry": str(Path(tmp) / 'missing')})]:
                with self.subTest(invalid=invalid):
                    if invalid is None:
                        config.unlink()
                    else:
                        config.write_text(invalid)
                    failure = subprocess.run([str(worktree / "tooling/loopx"), "status"], cwd=worktree, capture_output=True)
                    self.assertEqual(failure.returncode, 2)
                    self.assertFalse(failure.stdout)
                    self.assertFalse((worktree / ".loopx").exists())
