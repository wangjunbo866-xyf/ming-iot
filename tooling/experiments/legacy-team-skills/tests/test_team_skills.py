"""Isolated repository fixtures for required-skill registration and enforcement."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch


MODULE_PATH = Path(__file__).resolve().parents[1] / "tools/team_skills.py"
SPEC = importlib.util.spec_from_file_location("team_skills", MODULE_PATH)
skills = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(skills)


class TeamSkillsTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.home = self.base / "home"
        self.home.mkdir()
        self.environment = patch.dict(os.environ, {
            "HOME": str(self.home), "CODEX_HOME": str(self.home / ".codex"),
            "XDG_CONFIG_HOME": str(self.home / ".config"), "GIT_CONFIG_NOSYSTEM": "1",
        })
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.config = self.base / "config"
        self.config.mkdir()
        self.source = self.config / "skills/map-delivery"
        (self.source / "references").mkdir(parents=True)
        (self.source / "SKILL.md").write_text(
            "---\nname: map-delivery\ndescription: Deliver using project documents.\n---\n"
            "Read [workflow](references/workflow.md).\n", encoding="utf-8")
        (self.source / "references/workflow.md").write_text("# Workflow\nUse exact evidence.\n", encoding="utf-8")
        self.manifest = {
            "schema_version": 1,
            "skills": {"map-delivery": {"path": "skills/map-delivery", "required": True}},
        }
        self.write_manifest()
        skills.write_lock(self.config)
        self.workspace = self.base / "workspace"
        self.workspace.mkdir()
        self.project = self.workspace / "product"
        self.project.mkdir()
        self.git(self.project, "init", "--quiet")
        self.git(self.project, "config", "user.name", "Fixture")
        self.git(self.project, "config", "user.email", "fixture@example.invalid")
        (self.project / "README.md").write_text("fixture\n", encoding="utf-8")
        self.git(self.project, "add", "README.md")
        self.git(self.project, "commit", "--quiet", "-m", "fixture")

    def git(self, root, *args):
        result = subprocess.run(["git", "-C", str(root), *args], check=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        return result.stdout.strip()

    def write_manifest(self):
        (self.config / "manifest.json").write_text(json.dumps(self.manifest), encoding="utf-8")

    def write_lock(self, lock):
        (self.config / "skills.lock.json").write_text(json.dumps(lock), encoding="utf-8")

    def target(self, root=None):
        return (root or self.project) / ".agents/skills/map-delivery"

    def install(self, projects=None):
        return skills.install(self.config, self.workspace, projects or [self.project])

    def check(self):
        return skills.check(self.config, self.workspace, [self.project])

    def test_install_relative_links_is_idempotent_and_git_ignored(self):
        first = self.install()
        self.assertEqual(2, len(first["created"]))
        for root in (self.workspace, self.project):
            self.assertTrue(self.target(root).is_symlink())
            self.assertFalse(os.path.isabs(os.readlink(self.target(root))))
            self.assertEqual(self.source.resolve(), self.target(root).resolve())
        self.assertEqual("", self.git(self.project, "status", "--porcelain", "--untracked-files=all"))
        self.assertEqual([], self.install()["created"])
        self.assertTrue(self.check()["ok"])

    def test_conflict_anywhere_prevents_all_mutations(self):
        second = self.workspace / "second"
        second.mkdir()
        self.git(second, "init", "--quiet")
        target = self.target(second)
        target.mkdir(parents=True)
        (target / "SKILL.md").write_text("conflict\n", encoding="utf-8")
        exclude = self.project / ".git/info/exclude"
        before = exclude.read_bytes()
        with self.assertRaisesRegex(skills.SkillError, "frontmatter"):
            self.install([self.project, second])
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertFalse((self.project / ".agents").exists())
        self.assertEqual(before, exclude.read_bytes())
        self.assertEqual("conflict\n", (target / "SKILL.md").read_text())

    def test_runtime_link_failure_restores_links_directories_and_git_exclude(self):
        original = Path.symlink_to
        exclude = self.project / ".git/info/exclude"
        before = exclude.read_bytes()

        def create_or_fail(path, *args, **kwargs):
            if path == self.target():
                raise OSError("fixture link creation failed")
            return original(path, *args, **kwargs)

        with patch.object(Path, "symlink_to", create_or_fail):
            with self.assertRaisesRegex(OSError, "fixture link creation failed"):
                self.install()
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertFalse((self.project / ".agents").exists())
        self.assertEqual(before, exclude.read_bytes())

    def test_missing_skill_check_is_read_only(self):
        with self.assertRaisesRegex(skills.SkillError, "missing"):
            self.check()
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertFalse((self.project / ".agents").exists())

    def test_materialized_teamai_copies_match_locked_files(self):
        for root in (self.workspace, self.project):
            shutil.copytree(self.source, self.target(root))
        self.assertTrue(self.check()["ok"])
        self.assertEqual([], self.install()["created"])
        self.assertFalse(self.target().is_symlink())

    def test_teamai_exclusion_cannot_remove_required_skill(self):
        (self.project / ".teamai").mkdir()
        (self.project / ".teamai/config.json").write_text(
            json.dumps({"excludeSkills": ["map-delivery"]}), encoding="utf-8")
        shutil.copytree(self.source, self.target(self.workspace))
        with self.assertRaisesRegex(skills.SkillError, "required skill is missing"):
            self.check()

    def test_missing_reference_in_materialized_copy_fails(self):
        self.install()
        self.target().unlink()
        shutil.copytree(self.source, self.target())
        (self.target() / "references/workflow.md").unlink()
        with self.assertRaisesRegex(skills.SkillError, "missing reference"):
            self.check()

    def test_modified_materialized_file_fails(self):
        self.install()
        self.target().unlink()
        shutil.copytree(self.source, self.target())
        (self.target() / "references/workflow.md").write_text("changed\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "changed=references/workflow.md"):
            self.check()

    def test_unexpected_file_in_materialized_copy_fails(self):
        self.install()
        self.target().unlink()
        shutil.copytree(self.source, self.target())
        (self.target() / "unexpected.md").write_text("unreviewed\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "unexpected=unexpected.md"):
            self.check()

    def test_conflicting_project_legacy_duplicate_fails(self):
        self.install()
        legacy = self.project / ".codex/skills/map-delivery"
        shutil.copytree(self.source, legacy)
        (legacy / "references/workflow.md").write_text("old\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "conflicting duplicate skill"):
            self.check()

    def test_equivalent_legacy_and_user_duplicates_are_allowed(self):
        self.install()
        shutil.copytree(self.source, self.project / ".codex/skills/map-delivery")
        shutil.copytree(self.source, self.home / ".agents/skills/map-delivery")
        shutil.copytree(self.source, self.home / ".codex/skills/map-delivery")
        self.assertTrue(self.check()["ok"])

    def test_conflicting_user_duplicate_fails(self):
        self.install()
        legacy = self.home / ".codex/skills/map-delivery"
        shutil.copytree(self.source, legacy)
        (legacy / "references/workflow.md").write_text("old\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "conflicting duplicate user skill"):
            self.check()

    def test_disabled_user_skill_fails_before_install(self):
        config = self.home / ".codex/config.toml"
        config.parent.mkdir(parents=True)
        config.write_text(f"[[skills.config]]\npath = '{self.source / 'SKILL.md'}'\nenabled = false\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "disabled"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())

    def test_disabled_project_relative_path_uses_config_directory(self):
        self.install()
        config = self.project / ".codex/config.toml"
        config.parent.mkdir()
        config.write_text("[[skills.config]]\npath = '../.agents/skills/map-delivery/SKILL.md'\nenabled = false\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "disabled"):
            self.check()

    def test_ancestor_configuration_is_applied(self):
        self.install()
        config = self.workspace / ".codex/config.toml"
        config.parent.mkdir()
        config.write_text(f"[[skills.config]]\npath = '{self.target() / 'SKILL.md'}'\nenabled = false\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "disabled"):
            self.check()

    def test_tracked_target_prevents_registration_everywhere(self):
        shutil.copytree(self.source, self.target())
        self.git(self.project, "add", ".agents/skills/map-delivery")
        with self.assertRaisesRegex(skills.SkillError, "already tracked"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())

    def test_ignore_negation_prevents_registration_everywhere(self):
        (self.project / ".gitignore").write_text("!.agents/skills/map-delivery\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "ignore negation"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())

    def test_linked_worktree_uses_effective_git_exclude(self):
        worktree = self.workspace / "feature-worktree"
        self.git(self.project, "worktree", "add", "--quiet", "-b", "fixture-worktree", str(worktree))
        result = self.install([worktree])
        self.assertTrue(self.target(worktree).is_symlink())
        self.assertEqual("", self.git(worktree, "status", "--porcelain", "--untracked-files=all"))
        expected = Path(self.git(worktree, "rev-parse", "--git-path", "info/exclude")).resolve()
        self.assertIn(str(expected), result["git_excludes_updated"])

    def test_symlinked_git_info_outside_workspace_is_rejected_before_writes(self):
        outside = self.base / "outside-info"
        shutil.move(self.project / ".git/info", outside)
        (self.project / ".git/info").symlink_to(outside, target_is_directory=True)
        before = (outside / "exclude").read_bytes()
        with self.assertRaisesRegex(skills.SkillError, "Git exclude file escapes workspace"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertFalse((self.project / ".agents").exists())
        self.assertEqual(before, (outside / "exclude").read_bytes())

    def test_symlinked_git_info_inside_workspace_is_rejected_before_writes(self):
        location = self.workspace / "shared-info"
        shutil.move(self.project / ".git/info", location)
        (self.project / ".git/info").symlink_to(location, target_is_directory=True)
        before = (location / "exclude").read_bytes()
        with self.assertRaisesRegex(skills.SkillError, "Git exclude path contains a symlink"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertEqual(before, (location / "exclude").read_bytes())

    def test_external_git_directory_is_rejected_before_writes(self):
        outside = self.base / "outside-git"
        shutil.move(self.project / ".git", outside)
        (self.project / ".git").write_text(f"gitdir: {outside}\n", encoding="utf-8")
        before = (outside / "info/exclude").read_bytes()
        with self.assertRaisesRegex(skills.SkillError, "external Git directories are unsupported"):
            self.install()
        self.assertFalse((self.workspace / ".agents").exists())
        self.assertFalse((self.project / ".agents").exists())
        self.assertEqual(before, (outside / "info/exclude").read_bytes())

    def test_project_outside_workspace_is_rejected(self):
        outside = self.base / "outside"
        outside.mkdir()
        self.git(outside, "init", "--quiet")
        with self.assertRaisesRegex(skills.SkillError, "inside workspace"):
            self.install([outside])
        self.assertFalse((self.workspace / ".agents").exists())

    def test_symlink_escape_in_parent_is_rejected(self):
        outside = self.base / "outside"
        outside.mkdir()
        (self.project / ".agents").symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(skills.SkillError, "parent escapes"):
            self.install()
        self.assertFalse((outside / "skills").exists())

    def test_project_symlink_outside_workspace_is_rejected(self):
        outside = self.base / "outside"
        outside.mkdir()
        self.git(outside, "init", "--quiet")
        link = self.workspace / "outside-link"
        link.symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(skills.SkillError, "inside workspace"):
            self.install([link])

    def test_incomplete_source_reference_fails_lock(self):
        (self.source / "references/workflow.md").unlink()
        with self.assertRaisesRegex(skills.SkillError, "missing reference"):
            skills.write_lock(self.config)

    def test_duplicate_name_or_description_frontmatter_fails_before_writes(self):
        for duplicate in ("name: unexpected-skill", "description: Override description."):
            with self.subTest(duplicate=duplicate):
                (self.source / "SKILL.md").write_text(
                    "---\nname: map-delivery\ndescription: Valid description.\n"
                    f"{duplicate}\n---\n# Skill\n", encoding="utf-8")
                with self.assertRaisesRegex(skills.SkillError, "duplicate YAML frontmatter field"):
                    skills.write_lock(self.config)
                with self.assertRaisesRegex(skills.SkillError, "duplicate YAML frontmatter field"):
                    self.install()
                self.assertFalse((self.workspace / ".agents").exists())
                self.assertFalse((self.project / ".agents").exists())

    def test_unbalanced_yaml_quotes_fail(self):
        for name, description in (("'map-delivery", "Valid description."),
                                  ('"map-delivery', "Valid description."),
                                  ("map-delivery", "'Unbalanced description."),
                                  ("map-delivery", '"Unbalanced description.')):
            with self.subTest(name=name, description=description):
                (self.source / "SKILL.md").write_text(
                    f"---\nname: {name}\ndescription: {description}\n---\n# Skill\n", encoding="utf-8")
                with self.assertRaisesRegex(skills.SkillError, "malformed.*quotes"):
                    skills.write_lock(self.config)

    def test_valid_quoted_yaml_scalars_are_supported(self):
        for description in ("'Team''s workflow.'", '"Read the \\\"current\\\" workflow."'):
            with self.subTest(description=description):
                (self.source / "SKILL.md").write_text(
                    f"---\nname: 'map-delivery'\ndescription: {description}\n---\n# Skill\n", encoding="utf-8")
                skills.write_lock(self.config)
                self.assertIn("map-delivery", skills.validate(self.config)["skills"])

    def test_unsupported_yaml_constructs_and_nonstring_values_fail(self):
        headers = (
            "name: map-delivery\ndescription: >\n  Multiline description.",
            "name: map-delivery\ndescription: !!str tagged",
            "name: map-delivery\ndescription: [sequence]",
            "name: map-delivery\ndescription: key: value",
            "name: map-delivery\ndescription: true",
            "name: map-delivery\ndescription: 123",
            "name: map-delivery\ndescription: Text # inline comment",
            "name: map-delivery\ndescription: Valid.\nmetadata:\n  version: 1",
            "name: map-delivery\ndescription: \"Escaped\\nline\"",
        )
        for header in headers:
            with self.subTest(header=header):
                (self.source / "SKILL.md").write_text(f"---\n{header}\n---\n# Skill\n", encoding="utf-8")
                with self.assertRaises(skills.SkillError):
                    skills.write_lock(self.config)

    def test_inline_script_paths_use_skill_root_and_markdown_uses_document(self):
        (self.source / "scripts").mkdir()
        (self.source / "scripts/helper.py").write_text("print('helper')\n", encoding="utf-8")
        (self.source / "references/locator.md").write_text(
            "Run `scripts/helper.py`. Read [helper](../scripts/helper.py).\n", encoding="utf-8")
        skills.write_lock(self.config)
        self.assertIn("map-delivery", skills.validate(self.config)["skills"])
        (self.source / "scripts/helper.py").unlink()
        with self.assertRaisesRegex(skills.SkillError, "missing reference"):
            skills.write_lock(self.config)

    def test_modified_source_requires_explicit_new_lock(self):
        (self.source / "references/workflow.md").write_text("updated\n", encoding="utf-8")
        with self.assertRaisesRegex(skills.SkillError, "source map-delivery: lock mismatch"):
            skills.validate(self.config)
        skills.write_lock(self.config)
        self.assertIn("map-delivery", skills.validate(self.config)["skills"])

    def test_bytecode_is_ignored(self):
        (self.source / "__pycache__").mkdir()
        (self.source / "__pycache__/resolver.cpython-311.pyc").write_bytes(b"runtime cache")
        self.assertIn("map-delivery", skills.validate(self.config)["skills"])

    def test_source_symlink_escape_is_rejected(self):
        outside = self.base / "outside.md"
        outside.write_text("outside\n", encoding="utf-8")
        (self.source / "outside.md").symlink_to(outside)
        with self.assertRaisesRegex(skills.SkillError, "file escapes"):
            skills.write_lock(self.config)

    def test_empty_or_nonrequired_manifest_is_rejected(self):
        for entries in ({}, {"map-delivery": {"path": "skills/map-delivery", "required": False}}):
            with self.subTest(entries=entries):
                self.manifest["skills"] = entries
                self.write_manifest()
                with self.assertRaises(skills.SkillError):
                    skills.write_lock(self.config)

    def test_manifest_paths_and_names_cannot_escape(self):
        for name, path in (("../map-delivery", "skills/map-delivery"),
                           ("map-delivery", "../skills/map-delivery"),
                           ("map-delivery", "/tmp/skill"),
                           ("map-delivery", "skills\\map-delivery")):
            with self.subTest(name=name, path=path):
                self.manifest["skills"] = {name: {"path": path, "required": True}}
                self.write_manifest()
                with self.assertRaises(skills.SkillError):
                    skills.write_lock(self.config)

    def test_missing_inventory_tampered_hash_and_unsafe_lock_fail(self):
        original = skills.read_json(self.config / "skills.lock.json")
        for mutation in ("missing", "hash", "escape", "skillset"):
            with self.subTest(mutation=mutation):
                locked = json.loads(json.dumps(original))
                entry = locked["skills"]["map-delivery"]
                if mutation == "missing":
                    del entry["files"]["references/workflow.md"]
                elif mutation == "hash":
                    entry["files"]["SKILL.md"] = "not-a-sha256"
                elif mutation == "escape":
                    entry["files"]["../outside.md"] = "0" * 64
                else:
                    locked["skills"] = {}
                self.write_lock(locked)
                with self.assertRaises(skills.SkillError):
                    skills.validate(self.config)

    def test_cli_json_reports_failure_without_mutation(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            result = skills.main(["check", "--config-root", str(self.config),
                                  "--workspace-root", str(self.workspace),
                                  "--project", str(self.project), "--json"])
        self.assertEqual(1, result)
        report = json.loads(output.getvalue())
        self.assertFalse(report["ok"])
        self.assertEqual("check", report["command"])
        self.assertFalse((self.workspace / ".agents").exists())


if __name__ == "__main__":
    unittest.main()
