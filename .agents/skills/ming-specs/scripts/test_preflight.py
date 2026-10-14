"""隔离 Git 仓库中的反例；不访问开发者索引或服务。"""

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("preflight.py").resolve()


class PreflightTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "baseline")
        self.base = self.git("rev-parse", "HEAD").strip()
        self.write("spec.md", 'version: v0\n<a id="rule"></a>\n原行为\n')
        self.write("map.md", '<a id="patches"></a>\n[候选](change.md)\n')
        self.write("change.md", '[基线](spec.md#rule)\n[反向入口](map.md#patches)\n')
        self.record = dict(schema_version=1, module="tool", task="task1", type="patch", status="confirmed",
                           claim="change.md", confirmation_source="当前用户的任务授权", spec="spec.md#rule",
                           spec_version="v0", backlink="map.md#patches", write_scope=["*.md", "check.json", "src/**"])
        self.save_record()
        self.write("src/main.go", "package main\n")
        self.git("add", ".")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, stderr=subprocess.PIPE, text=True)

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def save_record(self):
        self.write("check.json", json.dumps(self.record, ensure_ascii=False))

    def run_check(self, *args):
        command = ["python3", str(SCRIPT), "--change", "check.json", "--base", self.base]
        run = subprocess.run(command + list(args or ("--staged",)), cwd=self.root, capture_output=True, text=True)
        return run.returncode, json.loads(run.stdout)

    def test_valid_is_not_semantic_or_approval_pass(self):
        code, report = self.run_check()
        self.assertEqual(code, 0)
        self.assertEqual(report["semantic_review"], "NOT_RUN")
        self.assertEqual(report["authorization"], "NOT_VERIFIED")
        self.assertEqual(report["hook_ci"], "NOT_VERIFIED")
        self.assertIn("src/main.go", report["changed_files"])

    def test_unstaged_repair_does_not_fix_staged_broken_backlink(self):
        self.write("map.md", '<a id="patches"></a>\n缺回链\n')
        self.git("add", "map.md")
        self.write("map.md", '<a id="patches"></a>\n[修复](change.md)\n')
        code, report = self.run_check()
        self.assertEqual(code, 1)
        self.assertIn("反向指针", report["reason"])
        self.git("add", "map.md")
        self.assertEqual(self.run_check()[0], 0)

    def test_missing_anchor_blocks(self):
        self.write("spec.md", "version: v0\n丢失条款\n")
        self.git("add", "spec.md")
        self.assertIn("锚点不存在", self.run_check()[1]["reason"])

    def test_outside_claim_blocks(self):
        self.write("unclaimed.txt", "意外修改\n")
        self.git("add", "unclaimed.txt")
        self.assertIn("unclaimed.txt", self.run_check()[1]["reason"])

    def test_untracked_is_explicitly_excluded(self):
        self.write("untracked.txt", "不在候选里\n")
        code, report = self.run_check()
        self.assertEqual(code, 0)
        self.assertEqual(report["untracked_files_not_checked"], ["untracked.txt"])

    def test_commit_snapshot_ignores_later_workspace(self):
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "candidate")
        head = self.git("rev-parse", "HEAD").strip()
        self.write("spec.md", "工作区破坏\n")
        code, report = self.run_check("--head", head)
        self.assertEqual(code, 0)
        self.assertEqual(report["candidate_commit"], head)
        self.assertEqual(report["unstaged_files_not_checked"], ["spec.md"])

    def test_missing_confirmation_field_blocks_without_validating_identity(self):
        self.record.pop("confirmation_source")
        self.save_record()
        self.git("add", "check.json")
        self.assertIn("confirmation_source", self.run_check()[1]["reason"])

    def test_historical_candidate_reports_later_staged_changes(self):
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "candidate")
        head = self.git("rev-parse", "HEAD").strip()
        self.write("map.md", "新暂存内容\n")
        self.git("add", "map.md")
        code, report = self.run_check("--head", head)
        self.assertEqual(code, 0)
        self.assertEqual(report["unstaged_files_not_checked"], [])
        self.assertEqual(report["candidate_vs_index_files"], ["map.md"])
        self.assertEqual(report["candidate_vs_worktree_files"], ["map.md"])
        self.assertEqual(report["candidate_vs_current_head_files"], [])

    def test_historical_candidate_reports_later_committed_changes(self):
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "candidate")
        head = self.git("rev-parse", "HEAD").strip()
        self.write("map.md", "已经提交的新版本\n")
        self.git("add", "map.md")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "later")
        code, report = self.run_check("--head", head)
        self.assertEqual(code, 0)
        self.assertEqual(report["unstaged_files_not_checked"], [])
        self.assertEqual(report["candidate_vs_current_head_files"], ["map.md"])
        self.assertEqual(report["candidate_vs_index_files"], ["map.md"])
        self.assertEqual(report["candidate_vs_worktree_files"], ["map.md"])
        self.assertNotEqual(report["candidate_commit"], report["current_head_commit"])

    def versioned_candidate(self, status="confirmed", revision="R2"):
        self.record["schema_version"] = 2
        self.record.pop("status")
        self.save_record()
        self.write("change.md", f'---\nstatus: {status}\nrevision: {revision}\n---\n'
                   '[基线](spec.md#rule)\n[反向入口](map.md#patches)\n')
        self.git("add", ".")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "versioned")
        return self.git("rev-parse", "HEAD").strip()

    def packet(self, head):
        code, packet = self.run_check("--head", head, "--emit-handoff")
        self.assertEqual(code, 0, packet)
        return packet

    def accept(self, head, packet):
        path = self.root / "packet.json"
        path.write_text(json.dumps(packet, ensure_ascii=False))
        return self.run_check("--head", head, "--handoff", str(path))

    def test_v2_decision_is_read_from_change_only(self):
        head = self.versioned_candidate()
        code, report = self.run_check("--head", head)
        self.assertEqual(code, 0, report)
        self.assertEqual(report["change_revision"], "R2")
        self.assertEqual(report["change_status"], "confirmed")
        self.assertEqual(report["handoff_check"], "NOT_RUN")

    def test_duplicate_decision_in_manifest_blocks(self):
        self.versioned_candidate()
        self.record["status"] = "confirmed"
        self.save_record()
        self.git("add", "check.json")
        code, report = self.run_check()
        self.assertEqual(code, 1)
        self.assertIn("不复制", report["reason"])

    def test_missing_revision_in_candidate_not_repaired_by_worktree(self):
        self.versioned_candidate()
        valid = (self.root / "change.md").read_text()
        self.write("change.md", valid.replace("revision: R2\n", ""))
        self.git("add", "change.md")
        self.write("change.md", valid)
        code, report = self.run_check()
        self.assertEqual(code, 1)
        self.assertIn("revision", report["reason"])

    def test_history_and_example_revision_cannot_replace_metadata(self):
        self.versioned_candidate()
        self.write("change.md", '---\nstatus: confirmed\n---\n'
                   '旧记录 revision: R2\n[基线](spec.md#rule)\n[反向入口](map.md#patches)\n')
        self.git("add", "change.md")
        self.assertEqual(self.run_check()[0], 1)

    def test_duplicate_or_noncanonical_metadata_blocks(self):
        self.versioned_candidate()
        for metadata in ('revision: R2\nrevision: "R1"', 'revision: R2\n revision: R1',
                         'revision: "R2"', 'revision: R2\nstatus: draft'):
            with self.subTest(metadata=metadata):
                self.write("change.md", '---\nstatus: confirmed\n'+metadata+'\n---\n'
                           '[基线](spec.md#rule)\n[反向入口](map.md#patches)\n')
                self.git("add", "change.md")
                self.assertEqual(self.run_check()[0], 1)

    def test_h2_round_trip_still_requires_semantic_review(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        code, report = self.accept(head, packet)
        self.assertEqual(code, 0, report)
        self.assertEqual(report["handoff_check"], "MATCHED_REVIEW_REQUIRED")
        self.assertEqual(report["semantic_review"], "NOT_RUN")
        self.assertEqual(report["authorization"], "NOT_VERIFIED")

    def test_h1_stale_revision_blocks(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        packet["change_revision"] = "R1"
        code, report = self.accept(head, packet)
        self.assertEqual(code, 1)
        self.assertIn("change_revision", report["reason"])

    def test_handoff_rejects_each_binding_mismatch(self):
        head = self.versioned_candidate()
        original = self.packet(head)
        changes = {"candidate_commit": "0" * 40, "base_commit": "1" * 40,
                   "candidate_tree": "2" * 40, "change_status": "effective",
                   "write_scope": ["**"], "inputs_sha256": {}, "diff_sha256": "0" * 64,
                   "task": "someone-else", "module": "elsewhere", "change": "other.json"}
        for field, value in changes.items():
            with self.subTest(field=field):
                packet = dict(original, **{field: value})
                code, report = self.accept(head, packet)
                self.assertEqual(code, 1)
                self.assertIn(field, report["reason"])

    def test_accept_rejects_dirty_tracked_worktree(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        self.write("src/main.go", "package changed\n")
        code, report = self.accept(head, packet)
        self.assertEqual(code, 1)
        self.assertIn("接收工作树", report["reason"])

    def test_accept_rejects_later_commit_even_if_worktree_clean(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        self.write("src/main.go", "package later\n")
        self.git("add", "src/main.go")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "later")
        code, report = self.accept(head, packet)
        self.assertEqual(code, 1)
        self.assertIn("接收工作树", report["reason"])

    def test_accept_rejects_different_head_with_same_tree(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                 "commit", "--allow-empty", "-qm", "audit")
        self.assertEqual(self.git("rev-parse", "HEAD^{tree}"), self.git("rev-parse", head+"^{tree}"))
        code, report = self.accept(head, packet)
        self.assertEqual(code, 1)
        self.assertIn("HEAD", report["reason"])

    def test_handoff_fingerprint_ignores_local_diff_display_configuration(self):
        head = self.versioned_candidate()
        packet = self.packet(head)
        self.write("diff-order", "src/**\ncheck.json\n")
        settings = {"diff.noprefix": "true", "diff.mnemonicPrefix": "true", "diff.context": "0",
                    "color.ui": "always", "diff.algorithm": "histogram",
                    "diff.orderFile": str(self.root / "diff-order")}
        for key, value in settings.items():
            with self.subTest(key=key):
                self.git("config", key, value)
                code, report = self.accept(head, packet)
                self.git("config", "--unset", key)
                self.assertEqual(code, 0, report)
                self.assertEqual(report["handoff_check"], "MATCHED_REVIEW_REQUIRED")

    def test_unstaged_candidate_cannot_be_handed_off(self):
        self.versioned_candidate()
        code, report = self.run_check("--staged", "--emit-handoff")
        self.assertEqual(code, 1)
        self.assertIn("固定 commit", report["reason"])

    def test_draft_allows_structure_check_but_not_implementation_handoff(self):
        head = self.versioned_candidate(status="draft")
        self.assertEqual(self.run_check("--head", head)[0], 0)
        code, report = self.run_check("--head", head, "--emit-handoff")
        self.assertEqual(code, 1)
        self.assertIn("实施交接", report["reason"])

    def test_legacy_structure_does_not_claim_versioned_handoff(self):
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "legacy")
        head = self.git("rev-parse", "HEAD").strip()
        self.assertEqual(self.run_check("--head", head)[0], 0)
        code, report = self.run_check("--head", head, "--emit-handoff")
        self.assertEqual(code, 1)
        self.assertIn("schema_version=2", report["reason"])

    def test_malformed_or_missing_packet_is_structured_failure(self):
        head = self.versioned_candidate()
        for value in ([], {}, {"schema_version": 99, "kind": "ming_specs_handoff"},
                      dict(self.packet(head), schema_version=True)):
            self.assertEqual(self.accept(head, value)[0], 1)
        code, report = self.run_check("--head", head, "--handoff", "missing.json")
        self.assertEqual(code, 1)
        self.assertEqual(report["result"], "BLOCKED")


if __name__ == "__main__":
    unittest.main()
