import hashlib
import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[1] / "tools/setup_workspace_skills.py"
CONTRACT = SCRIPT.parents[1] / "meta/SKILLS_SOURCE.json"


class WorkspaceSkillsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.workspace = Path(self.temporary.name) / "workspace"
        self.product = self.workspace / "ming-iot"
        self.docs = self.workspace / "ming-iot-rd-docs"
        self.skills_repo = self.workspace / "ai-native"
        for repo in (self.product, self.docs, self.skills_repo):
            repo.mkdir(parents=True)
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
        self.git(self.docs, "remote", "add", "origin", "https://gitee.com/ming-industry/ming-iot-rd-docs.git")
        self.git(self.skills_repo, "remote", "add", "origin", "https://gitee.com/ming-industry/ai-native.git")
        self.canonical = self.skills_repo / "skills/map-delivery"
        self.canonical.mkdir(parents=True)
        (self.canonical / "SKILL.md").write_text(
            "---\nname: map-delivery\ndescription: Use the document workflow.\n---\n"
            "Read [workflow](references/workflow.md).\n", encoding="utf-8")
        (self.canonical / "references").mkdir()
        (self.canonical / "references/workflow.md").write_text("# Workflow\n", encoding="utf-8")
        self.write_lock()
        self.script = self.docs / "tools/setup_workspace_skills.py"
        self.script.parent.mkdir()
        (self.docs / "meta").mkdir()
        shutil.copy2(CONTRACT, self.docs / "meta/SKILLS_SOURCE.json")
        shutil.copy2(SCRIPT, self.script)

    def git(self, repo, *args):
        return subprocess.run(["git", "-C", str(repo), *args], check=True,
                              text=True, capture_output=True).stdout.strip()

    def write_lock(self):
        files = {path.relative_to(self.canonical).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
                 for path in self.canonical.rglob("*") if path.is_file()}
        self.lock = {"schema_version": 1, "skills": {
            "map-delivery": {"path": "skills/map-delivery", "files": files}}}
        self.save_lock()

    def save_lock(self):
        (self.skills_repo / "skills.lock.json").write_text(json.dumps(self.lock), encoding="utf-8")

    def installer(self, docs=None):
        file = (docs or self.docs) / "tools/setup_workspace_skills.py"
        spec = importlib.util.spec_from_file_location("fixture_workspace_skills", file)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def run_setup(self, product=None, arguments=(), script=None):
        self.assertTrue(SCRIPT.is_file(), "workspace skill installer must exist")
        return subprocess.run(
            [sys.executable, str(script or self.script), "--workspace-root", str(self.workspace),
             "--product-repo", str(product or self.product), *arguments],
            cwd=self.workspace, text=True, capture_output=True, check=False,
        )

    def targets(self):
        return [self.workspace / ".agents/skills/map-delivery",
                self.workspace / ".claude/skills/map-delivery",
                self.product / ".agents/skills/map-delivery",
                self.product / ".claude/skills/map-delivery",
                self.docs / ".agents/skills/map-delivery",
                self.docs / ".claude/skills/map-delivery"]

    def test_creates_six_relative_links_without_docs_source_and_is_idempotent(self):
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)
        before = {}
        for path in self.targets():
            self.assertTrue(path.is_symlink(), path)
            self.assertFalse(os.path.isabs(os.readlink(path)))
            self.assertEqual(path.resolve(), self.canonical)
            before[path] = path.lstat().st_mtime_ns
        self.assertEqual(result.stdout.count("created"), 6)
        again = self.run_setup()
        self.assertEqual(again.returncode, 0, again.stderr)
        self.assertEqual(again.stdout.count("existing"), 6)
        self.assertEqual(before, {path: path.lstat().st_mtime_ns for path in self.targets()})

    def test_rejects_regular_file_without_overwriting_or_partial_links(self):
        conflict = self.targets()[-1]
        conflict.parent.mkdir(parents=True)
        conflict.write_text("human content", encoding="utf-8")
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(conflict.read_text(), "human content")
        self.assertFalse(self.targets()[0].exists())

    def test_rejects_symlink_to_another_source(self):
        path = self.targets()[0]
        path.parent.mkdir(parents=True)
        path.symlink_to(self.workspace / "other")
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(os.readlink(path), str(self.workspace / "other"))

    def test_rejects_destination_escape(self):
        outside = Path(self.temporary.name) / "outside"
        outside.mkdir()
        (self.workspace / ".claude").symlink_to(outside, target_is_directory=True)
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((outside / "skills").exists())
        self.assertFalse(self.targets()[0].exists())

    def test_rejects_product_outside_workspace(self):
        outside = Path(self.temporary.name) / "product"
        subprocess.run(["git", "init", "-q", str(outside)], check=True)
        self.assertNotEqual(self.run_setup(outside).returncode, 0)
        self.assertFalse(self.targets()[0].exists())

    def test_rejects_symlink_skill_source_before_creating_links(self):
        skill = self.canonical / "SKILL.md"
        outside = Path(self.temporary.name) / "outside-skill.md"
        skill.rename(outside)
        skill.symlink_to(outside)
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.targets()[0].exists())

    def test_uses_git_exclude_without_changing_tracked_configuration(self):
        exclude = self.product / ".git/info/exclude"
        original = exclude.read_text(encoding="utf-8")
        gitignore = self.product / ".gitignore"
        gitignore.write_text("# keep this\n", encoding="utf-8")
        subprocess.run(["git", "-C", str(self.product), "add", ".gitignore"], check=True)
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(exclude.read_text(encoding="utf-8").startswith(original))
        first = exclude.read_bytes()
        self.assertEqual(self.run_setup().returncode, 0)
        self.assertEqual(exclude.read_bytes(), first)
        self.assertEqual(gitignore.read_text(), "# keep this\n")
        ignored = subprocess.run(
            ["git", "-C", str(self.product), "check-ignore",
             ".agents/skills/map-delivery", ".claude/skills/map-delivery"],
            capture_output=True, text=True,
        )
        self.assertEqual(ignored.returncode, 0)
        self.assertEqual(len(ignored.stdout.splitlines()), 2)

    def test_product_linked_worktree_uses_effective_git_exclude(self):
        subprocess.run(["git", "-C", str(self.product), "-c", "user.name=Test",
                        "-c", "user.email=test@example.invalid", "commit", "--allow-empty",
                        "-qm", "baseline"], check=True)
        linked = self.workspace / "product-linked"
        subprocess.run(["git", "-C", str(self.product), "worktree", "add", "-q", "-b",
                        "skills-test", str(linked)], check=True)
        result = self.run_setup(linked)
        self.assertEqual(result.returncode, 0, result.stderr)
        ignored = subprocess.run(["git", "-C", str(linked), "check-ignore",
                                  ".agents/skills/map-delivery", ".claude/skills/map-delivery"],
                                 text=True, capture_output=True)
        self.assertEqual(ignored.returncode, 0, ignored.stderr)
        self.assertEqual(len(ignored.stdout.splitlines()), 2)

    def test_rejects_exclude_parent_escape_before_creating_links(self):
        info = self.product / ".git/info"
        outside = Path(self.temporary.name) / "outside-info"
        info.rename(outside)
        info.symlink_to(outside, target_is_directory=True)
        original = (outside / "exclude").read_bytes()
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((outside / "exclude").read_bytes(), original)
        self.assertFalse(self.targets()[0].exists())

    def test_rejects_ignore_negation_before_creating_links(self):
        ignore = self.product / ".gitignore"
        original = "!.agents/skills/map-delivery\n!.claude/skills/map-delivery\n"
        ignore.write_text(original, encoding="utf-8")
        subprocess.run(["git", "-C", str(self.product), "add", ".gitignore"], check=True)
        exclude = self.product / ".git/info/exclude"
        before = exclude.read_bytes()
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(ignore.read_text(), original)
        self.assertEqual(exclude.read_bytes(), before)
        self.assertFalse(self.targets()[0].exists())

    def test_explicit_skills_repo_supports_another_checkout_directory(self):
        selected = self.workspace / "team-skills-checkout"
        self.skills_repo.rename(selected)
        result = self.run_setup(arguments=("--skills-repo", str(selected)))
        self.assertEqual(result.returncode, 0, result.stderr)
        for target in self.targets():
            self.assertEqual(target.resolve(), selected / "skills/map-delivery")

    def test_source_git_identity_must_match_contract(self):
        self.git(self.skills_repo, "remote", "set-url", "origin", "https://gitee.com/another/ai-native.git")
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Git identity", result.stderr)
        self.assertFalse(self.targets()[0].exists())

    def test_ssh_source_identity_is_supported(self):
        self.git(self.skills_repo, "remote", "set-url", "origin", "git@gitee.com:ming-industry/ai-native.git")
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_external_source_fails_without_recreating_docs_source(self):
        shutil.rmtree(self.skills_repo)
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        for target in self.targets():
            self.assertFalse(os.path.lexists(target))

    def test_missing_lock_or_tampered_source_fails_before_registration(self):
        (self.skills_repo / "skills.lock.json").unlink()
        self.assertNotEqual(self.run_setup().returncode, 0)
        self.save_lock()
        (self.canonical / "references/workflow.md").write_text("changed\n", encoding="utf-8")
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("SHA-256", result.stderr)
        self.assertFalse(self.targets()[0].exists())

    def test_lock_file_inventory_is_complete(self):
        del self.lock["skills"]["map-delivery"]["files"]["references/workflow.md"]
        self.save_lock()
        self.assertNotEqual(self.run_setup().returncode, 0)
        self.assertFalse(self.targets()[0].exists())

    def test_extra_skill_file_is_unreviewed_and_rejected(self):
        (self.canonical / "extra.py").write_text("unreviewed\n", encoding="utf-8")
        self.assertNotEqual(self.run_setup().returncode, 0)
        self.assertFalse(self.targets()[0].exists())

    def test_reviewed_inventory_still_rejects_missing_references(self):
        (self.canonical / "references/workflow.md").unlink()
        self.write_lock()
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("缺失的本地引用", result.stderr)
        self.assertFalse(self.targets()[0].exists())

    def test_locked_files_still_reject_wrong_duplicate_or_malformed_skill_header(self):
        headers = (
            "name: unexpected-skill\ndescription: Valid text.",
            "name: map-delivery\nname: unexpected-skill\ndescription: Valid text.",
            "name: map-delivery\ndescription: First.\ndescription: Second.",
            "name: 'map-delivery\ndescription: Valid text.",
            "name: map-delivery\ndescription: \"Unbalanced description.",
            "name: map-delivery\ndescription: >\n  Multiline text.",
            "name: map-delivery\ndescription: true",
        )
        for header in headers:
            with self.subTest(header=header):
                (self.canonical / "SKILL.md").write_text(f"---\n{header}\n---\n# Skill\n", encoding="utf-8")
                self.write_lock()
                result = self.run_setup()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("SKILL.md", result.stderr)
                for target in self.targets():
                    self.assertFalse(os.path.lexists(target))

    def test_quoted_skill_header_matches_supported_team_subset(self):
        (self.canonical / "SKILL.md").write_text(
            "---\nname: 'map-delivery'\ndescription: 'Team''s workflow.'\n---\n# Skill\n", encoding="utf-8")
        self.write_lock()
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_bytecode_caches_are_not_distribution_content(self):
        cache = self.canonical / "__pycache__"
        cache.mkdir()
        (cache / "resolver.cpython-312.pyc").write_bytes(b"cache")
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_unsafe_lock_paths_and_hashes_fail(self):
        for key, digest in (("../outside.md", "0" * 64),
                            ("C:/outside.md", "0" * 64),
                            ("SKILL.md", "invalid")):
            with self.subTest(key=key):
                self.write_lock()
                self.lock["skills"]["map-delivery"]["files"][key] = digest
                self.save_lock()
                self.assertNotEqual(self.run_setup().returncode, 0)
                self.assertFalse(self.targets()[0].exists())

    def test_default_rejects_known_legacy_symlink_without_changing_it(self):
        target = self.targets()[0]
        target.parent.mkdir(parents=True)
        old = os.path.relpath(self.docs / ".agents/skills/map-delivery", target.parent)
        target.symlink_to(old, target_is_directory=True)
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(os.readlink(target), old)
        self.assertFalse(self.targets()[1].exists())

    def test_explicit_migration_updates_verified_broken_legacy_links(self):
        legacy = self.docs / ".agents/skills/map-delivery"
        for target in self.targets()[:4]:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.symlink_to(os.path.relpath(legacy, target.parent), target_is_directory=True)
        self.assertFalse(legacy.exists())
        arguments = ("--migrate-known-source", "--legacy-docs-root", str(self.docs))
        result = self.run_setup(arguments=arguments)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.count("migrated"), 4)
        self.assertEqual(result.stdout.count("created"), 2)
        for target in self.targets():
            self.assertEqual(target.resolve(), self.canonical)
        previous = {target: target.lstat().st_mtime_ns for target in self.targets()}
        again = self.run_setup(arguments=arguments)
        self.assertEqual(again.returncode, 0, again.stderr)
        self.assertEqual(again.stdout.count("existing"), 6)
        self.assertEqual(previous, {target: target.lstat().st_mtime_ns for target in self.targets()})

    def test_migration_rejects_foreign_legacy_canonical_symlink_even_when_it_resolves_to_new_source(self):
        alias = self.workspace / "foreign-alias"
        alias.symlink_to(self.canonical, target_is_directory=True)
        legacy = self.docs / ".agents/skills/map-delivery"
        legacy.parent.mkdir(parents=True)
        legacy.symlink_to(os.path.relpath(alias, legacy.parent), target_is_directory=True)
        previous = os.readlink(legacy)
        result = self.run_setup(arguments=("--migrate-known-source", "--legacy-docs-root", str(self.docs)))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(os.readlink(legacy), previous)
        self.assertEqual(legacy.resolve(), self.canonical)
        self.assertFalse(os.path.lexists(self.targets()[0]))

    def test_migration_with_existing_legacy_source_uses_explicit_verified_root(self):
        legacy_repo = self.workspace / "legacy-docs"
        self.git(self.workspace, "init", "-q", str(legacy_repo))
        self.git(legacy_repo, "remote", "add", "origin", "https://gitee.com/ming-industry/ming-iot-rd-docs.git")
        legacy = legacy_repo / ".agents/skills/map-delivery"
        shutil.copytree(self.canonical, legacy)
        original = (legacy / "SKILL.md").read_bytes()
        target = self.targets()[0]
        target.parent.mkdir(parents=True)
        target.symlink_to(os.path.relpath(legacy, target.parent), target_is_directory=True)
        result = self.run_setup(arguments=("--migrate-known-source", "--legacy-docs-root", str(legacy_repo)))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(target.resolve(), self.canonical)
        self.assertEqual((legacy / "SKILL.md").read_bytes(), original)

    def test_migration_does_not_overwrite_unknown_or_regular_directory_targets(self):
        legacy = self.docs / ".agents/skills/map-delivery"
        first = self.targets()[0]
        first.parent.mkdir(parents=True)
        first.symlink_to(os.path.relpath(legacy, first.parent), target_is_directory=True)
        conflict = self.targets()[-1]
        conflict.mkdir(parents=True)
        (conflict / "human.md").write_text("keep\n", encoding="utf-8")
        before = os.readlink(first)
        result = self.run_setup(arguments=("--migrate-known-source",))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(os.readlink(first), before)
        self.assertEqual((conflict / "human.md").read_text(), "keep\n")
        self.assertFalse(self.targets()[1].exists())

    def test_migration_rejects_wrong_legacy_git_identity(self):
        self.git(self.docs, "remote", "set-url", "origin", "https://gitee.com/another/docs.git")
        result = self.run_setup(arguments=("--migrate-known-source",))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Git identity", result.stderr)
        self.assertFalse(self.targets()[0].exists())

    def test_tracked_docs_skill_cannot_be_overwritten_during_migration(self):
        target = self.docs / ".agents/skills/map-delivery"
        shutil.copytree(self.canonical, target)
        self.git(self.docs, "add", ".agents/skills/map-delivery")
        result = self.run_setup(arguments=("--migrate-known-source",))
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.targets()[0].exists())
        self.assertTrue((target / "SKILL.md").is_file())

    def test_runtime_link_failure_rolls_back_all_created_links_and_directories(self):
        module = self.installer()
        original = Path.symlink_to
        counter = 0
        excludes = {repo: (repo / ".git/info/exclude").read_bytes() for repo in (self.product, self.docs)}

        def fail_third(path, *args, **kwargs):
            nonlocal counter
            counter += 1
            if counter == 3:
                raise PermissionError("fixture Windows link permission denied")
            return original(path, *args, **kwargs)

        with patch.object(Path, "symlink_to", fail_third):
            with self.assertRaises(PermissionError):
                module.setup(self.workspace, self.product)
        for root in (self.workspace, self.product, self.docs):
            self.assertFalse((root / ".agents").exists())
            self.assertFalse((root / ".claude").exists())
        for repo, previous in excludes.items():
            self.assertEqual((repo / ".git/info/exclude").read_bytes(), previous)

    def test_migration_link_creation_failure_preserves_original_alias(self):
        module = self.installer()
        target = self.targets()[0]
        target.parent.mkdir(parents=True)
        previous = os.path.relpath(self.docs / ".agents/skills/map-delivery", target.parent)
        target.symlink_to(previous, target_is_directory=True)
        with patch.object(Path, "symlink_to", side_effect=PermissionError("fixture no symlinks")):
            with self.assertRaises(PermissionError):
                module.setup(self.workspace, self.product, migrate_known_source=True)
        self.assertEqual(os.readlink(target), previous)
        self.assertEqual(list(target.parent.glob(".skill-link-*")), [])

    def test_exclude_failure_rolls_back_new_links_and_previous_exclude(self):
        module = self.installer()
        original = module._atomic_bytes
        product_exclude = self.product / ".git/info/exclude"
        docs_exclude = self.docs / ".git/info/exclude"
        before = {path: path.read_bytes() for path in (product_exclude, docs_exclude)}

        def fail_docs(path, content):
            if path == docs_exclude:
                raise OSError("fixture exclude failure")
            return original(path, content)

        with patch.object(module, "_atomic_bytes", fail_docs):
            with self.assertRaisesRegex(OSError, "fixture exclude failure"):
                module.setup(self.workspace, self.product)
        for target in self.targets():
            self.assertFalse(os.path.lexists(target))
        for path, content in before.items():
            self.assertEqual(path.read_bytes(), content)

    def test_docs_linked_worktree_uses_contract_and_effective_exclude(self):
        self.git(self.docs, "add", "tools/setup_workspace_skills.py", "meta/SKILLS_SOURCE.json")
        self.git(self.docs, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture")
        linked = self.docs / ".worktrees/candidate"
        self.git(self.docs, "worktree", "add", "-q", "-b", "docs-linked", str(linked))
        result = self.run_setup(script=linked / "tools/setup_workspace_skills.py")
        self.assertEqual(result.returncode, 0, result.stderr)
        for directory in (".agents", ".claude"):
            self.assertEqual((linked / directory / "skills/map-delivery").resolve(), self.canonical)
        ignored = self.git(linked, "check-ignore", ".agents/skills/map-delivery", ".claude/skills/map-delivery")
        self.assertEqual(len(ignored.splitlines()), 2)


if __name__ == "__main__":
    unittest.main()
