import hashlib
import gzip
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

SPEC = importlib.util.spec_from_file_location("publication", Path(__file__).resolve().parents[1] / "publication.py")
publication = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publication)


class PublicationTests(unittest.TestCase):
    def test_deleted_history_and_other_refs_remain_in_scope(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d) / "repo"
            root.mkdir()
            def git(*args):
                return publication.git(root, *args)
            git("init", "-q")
            git("config", "user.name", "fixture")
            git("config", "user.email", "fixture@example.invalid")
            (root / "old.txt").write_text("historical material")
            git("add", ".")
            git("commit", "-qm", "old")
            git("branch", "retained")
            git("rm", "old.txt")
            git("commit", "-qm", "delete")
            destination = Path(d) / "objects"
            destination.mkdir()
            objects = publication.export_objects(root, destination)
            self.assertEqual(sum(o["type"] == "commit" for o in objects.values()), 2)
            self.assertIn(hashlib.sha256(b"historical material").hexdigest(), [o["sha256"] for o in objects.values()])
            (root / ".git/shallow").write_bytes(git("rev-parse", "HEAD"))
            with self.assertRaisesRegex(ValueError, "shallow"):
                publication.export_objects(root, destination)

    def test_exception_cannot_hide_changed_blob_or_other_rule(self):
        objects = {"old": {"sha256": "a"}, "new": {"sha256": "b"}}
        exception = [{"sha256": "a", "rule": "test-rule", "line": 3, "reason": "known deterministic vector"}]
        findings = [{"File": "/objects/old", "RuleID": "test-rule", "StartLine": 3},
                    {"File": "/objects/new", "RuleID": "test-rule", "StartLine": 3},
                    {"File": "/objects/old", "RuleID": "private-key", "StartLine": 3}]
        self.assertEqual(len(publication.unresolved_findings(findings, objects, exception)), 2)
        member = [{"File": "/objects/old.zip!nested.txt", "RuleID": "test-rule", "StartLine": 3}]
        self.assertEqual(len(publication.unresolved_findings(member, objects, exception)), 1)

    def test_unknown_archive_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "archive format"):
            publication.archive_suffix(b"7z\xbc\xaf\x27\x1c" + bytes(20))
        nested = b"text"
        for _ in range(publication.MAX_ARCHIVE_DEPTH + 1):
            nested = gzip.compress(nested)
        with self.assertRaisesRegex(ValueError, "nesting limit"):
            list(publication.archive_leaves(nested, [0, 0]))
        with self.assertRaisesRegex(ValueError, "expansion limit"):
            list(publication.archive_leaves(gzip.compress(b"text"), [publication.MAX_EXPANDED_BYTES, 0]))

    @unittest.skipUnless((publication.ROOT / ".cache/ci-tools/gitleaks").exists(), "run make ci-tools for scanner integration")
    def test_scanner_detects_deleted_plain_and_compressed_secrets(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d) / "repo"
            root.mkdir()
            def git(*args):
                return publication.git(root, *args)
            git("init", "-q")
            git("config", "user.name", "fixture")
            git("config", "user.email", "fixture@example.invalid")
            token = "ghp_" + hashlib.sha256(b"synthetic-review-only-fixture").hexdigest()[:36]
            payload = ('github_token="' + token + '"\n').encode()
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("example.txt", payload)
            tar_buffer = io.BytesIO()
            with tarfile.open(fileobj=tar_buffer, mode="w") as archive:
                member = tarfile.TarInfo("hidden.bin")
                member.size = len(payload)
                archive.addfile(member, io.BytesIO(payload))
            nested_buffer = io.BytesIO()
            with zipfile.ZipFile(nested_buffer, "w", zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("../../nested.bin", buffer.getvalue())
            files = (("plain.txt", payload), ("hidden.bin", buffer.getvalue()), ("compressed.bin", gzip.compress(payload)),
                     ("tar-compressed.bin", gzip.compress(tar_buffer.getvalue())), ("nested.bin", nested_buffer.getvalue()))
            for name, data in files:
                (root / name).write_bytes(data)
            git("add", ".")
            git("commit", "-qm", "synthetic audit fixture")
            git("rm", *[name for name, _ in files])
            git("commit", "-qm", "delete fixtures")
            objects_dir = Path(d) / "objects"
            objects_dir.mkdir()
            objects = publication.export_objects(root, objects_dir)
            report = Path(d) / "report.json"
            config = Path(d) / "gitleaks.toml"
            config.write_text("[extend]\nuseDefault = true\n")
            result = subprocess.run([str(publication.ROOT / ".cache/ci-tools/gitleaks"), "dir", str(objects_dir),
                                     "--config", str(config), "--gitleaks-ignore-path", str(Path(d) / "none"),
                                     "--ignore-gitleaks-allow", "--max-archive-depth=0", "--redact=100",
                                     "--report-format=json", "--report-path", str(report), "--no-banner"], capture_output=True)
            self.assertEqual(result.returncode, 1, "scanner must reject synthetic secrets")
            findings = json.loads(report.read_text())
            failures = publication.unresolved_findings(findings, objects, [])
            self.assertEqual(len({x["object"] for x in failures}), 5)
            self.assertTrue(all(x["object"] in objects for x in failures))

    def test_reference_repositories_and_runtime_files_fail(self):
        for path, mode in [("third_party/ThingsBoard/file", "100644"), ("AirMQ/code", "100644"),
                           ("docs/.loopx/state.json", "100644"), ("child", "160000"),
                           ("escape", "120000"), ("deploy/.env", "100644")]:
            self.assertTrue(publication.forbidden(path, mode), path)
        self.assertFalse(publication.forbidden("docs/architecture/thingsboard-notes.md", "100644"))
        self.assertFalse(publication.forbidden("deploy/.env.example", "100644"))

    def test_publication_hold_fails_closed(self):
        for policy in ({}, {"publication_status": "ready"},
                       {"publication_status": "hold", "pending_decisions": []},
                       {"publication_status": "ready", "pending_decisions": [{"id": "rights"}]}):
            self.assertFalse(publication.publication_ready(policy))
        self.assertTrue(publication.publication_ready({"publication_status": "ready", "pending_decisions": []}))


if __name__ == "__main__":
    unittest.main()
