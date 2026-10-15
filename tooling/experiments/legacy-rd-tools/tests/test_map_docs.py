"""Behavioral contract for the offline map checker and scaffold command."""

import importlib
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


DELIVERY_ID = "ming-iot-rd-docs#IKEOFY"
ISSUE_URL = "https://gitee.com/ming-industry/ming-iot-rd-docs/issues/IKEOFY"
MODULE = "map/00_研发治理/01_文档管理"
CHANGE = MODULE + "/changes/FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理"
EXACT_HEAD = "a" * 40
BASE_HEAD = "b" * 40
REPOSITORY = "git:gitee.com/ming-industry/ming-iot-rd-docs"
CLI = Path(__file__).resolve().parents[1] / "tools/map_docs.py"


def front_matter(fields, body=""):
    lines = ["---"]
    for key, value in fields.items():
        if isinstance(value, (list, tuple)):
            lines.append(key + ":")
            lines.extend("  - " + item for item in value)
        else:
            lines.append(f"{key}: {value}")
    return "\n".join(lines + ["---", "", body, ""])


class MapDocsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def fixture_path(self, path):
        target = self.root / path
        self.assertTrue(target.resolve().is_relative_to(self.root))
        return target

    def write(self, path, text):
        target = self.fixture_path(path)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def append(self, path, text):
        target = self.fixture_path(path)
        self.write(path, target.read_text(encoding="utf-8") + text)

    def api(self):
        try:
            module = importlib.import_module("tools.map_docs")
        except ModuleNotFoundError:
            self.fail("tools.map_docs.validate and CLI are not implemented")
        self.assertTrue(callable(getattr(module, "validate", None)))
        return module

    def problems(self):
        result = self.api().validate(self.root)
        self.assertEqual(result, sorted(set(result)), "stable path + code ordering")
        return result

    def assert_problem(self, code, path=None):
        result = self.problems()
        if path:
            self.assertIn(self.api().Problem(path, code), result)
        else:
            self.assertIn(code, [problem.code for problem in result])

    def change(self, directory=CHANGE, delivery=DELIVERY_ID, status="approved",
               anchors=("behavior",), **overrides):
        fields = dict(document_kind="change", change_type="feature", status=status,
                      delivery_id=delivery,
                      issue="https://gitee.com/ming-industry/" + delivery.replace("#", "/issues/"),
                      hub=directory + "/README.md", target_module=MODULE,
                      affected_repositories=[REPOSITORY], affected_modules=[MODULE],
                      target_anchors=list(anchors))
        fields.update(overrides)
        self.write(directory + "/change.md", front_matter(fields, "[Hub](./README.md)"))
        self.write(directory + "/README.md", "\n".join([
            f"[Issue]({fields['issue']})", "[Change Spec](./change.md)",
            "Plan: 待编写", "PR: 待创建", "Review: 待评审", "Evidence: 待验证", ""]))
        return fields

    def tree(self, existing=True):
        self.write("map/README.md", f"[模块](./{MODULE[4:]}/README.md)\n")
        self.write(MODULE + "/README.md", "[交付](./changes/" + CHANGE.split("/")[-1] + "/README.md)\n")
        if existing:
            self.write(MODULE + "/spec.md", front_matter(
                dict(document_kind="spec", status="current", module=MODULE),
                '<a id="behavior"></a>\n## 行为\n当前行为。'))
            self.append(MODULE + "/README.md", "[当前规格](./spec.md)\n")
        return self.change(anchors=("behavior",) if existing else ("__module_root__",))

    def artifact(self, kind, **overrides):
        fields = dict(document_kind=kind, status={"plan": "approved", "review": "pass",
                      "evidence": "verified"}[kind], delivery_id=DELIVERY_ID,
                      issue=ISSUE_URL, hub=CHANGE + "/README.md")
        body = "[Hub](./README.md)\n"
        if kind == "plan":
            fields.update(change_spec=CHANGE + "/change.md", affected_repositories=[REPOSITORY])
        else:
            fields["repository_heads"] = [REPOSITORY + "@" + EXACT_HEAD]
            body += ("\n| repository | base | exact_head | verdict |\n"
                     "|---|---|---|---|\n"
                     f"| `{REPOSITORY}` | `{BASE_HEAD}` | `{EXACT_HEAD}` | {fields['status']} |\n")
        fields.update(overrides)
        self.write(CHANGE + f"/{kind}.md", front_matter(fields, body))
        self.append(CHANGE + "/README.md", f"[{kind}](./{kind}.md)\n")

    def cli(self, *args):
        return subprocess.run([sys.executable, str(CLI), *args, "--docs-root", str(self.root)],
                              capture_output=True, text=True, check=False)

    def init(self, *extra):
        return self.cli("init", "--module", MODULE, "--type", "FEATURE", "--delivery-id",
                        DELIVERY_ID, "--name", "业务地图文档管理", "--issue", ISSUE_URL,
                        "--repository", REPOSITORY, *extra)

    def test_valid_active_change_passes(self):
        self.tree()
        for kind in ("plan", "review", "evidence"):
            self.artifact(kind)
        self.assertEqual([], self.problems())

    def test_change_directory_type_and_delivery_id_must_match_metadata(self):
        self.tree()
        self.change(change_type="bugfix")
        self.assert_problem("MAP_CHANGE_TYPE_INVALID")
        self.change(delivery="ming-iot#I12345")
        self.assert_problem("MAP_DELIVERY_ID_INVALID")
        self.change(change_type="unknown")
        self.assert_problem("MAP_CHANGE_TYPE_INVALID")

    def test_duplicate_delivery_id_is_rejected(self):
        self.tree()
        self.change(directory=CHANGE + "副本")
        self.assert_problem("MAP_DELIVERY_DUPLICATE")

    def test_change_must_live_under_its_target_module(self):
        self.tree()
        self.change(target_module="map/other/module")
        self.assert_problem("MAP_RELATED_PATH_INVALID")

    def test_symlink_or_parent_escape_is_rejected(self):
        self.tree()
        for path in ("../secret.md", "/tmp/secret.md", "map/../secret.md"):
            with self.subTest(path=path):
                self.change(hub=path)
                self.assert_problem("MAP_RELATED_PATH_INVALID")
        self.change()
        with tempfile.TemporaryDirectory() as outside:
            (self.root / "map/escape").symlink_to(outside, target_is_directory=True)
            self.assert_problem("MAP_RELATED_PATH_INVALID", "map/escape")
            self.append(CHANGE + "/change.md", "\n[escape](../../../../../../secret.md)\n")
            self.assert_problem("MAP_RELATED_PATH_INVALID", CHANGE + "/change.md")

    def test_module_root_plan_is_rejected(self):
        self.tree()
        self.write(MODULE + "/plan.md", "# misplaced plan")
        self.assert_problem("MAP_MODULE_PLAN_FORBIDDEN", MODULE + "/plan.md")

    def test_navigation_readme_may_omit_front_matter(self):
        self.tree(False)
        self.assertEqual([], self.problems())
        self.write(MODULE + "/references/README.md", "# not a navigation exemption")
        self.assert_problem("MAP_FRONT_MATTER_INVALID")

    def test_front_matter_is_required_and_malformed_input_is_not_skipped(self):
        self.tree()
        for content in ("# no metadata", "---\nstatus: current\n", "---\nstatus: a\nstatus: b\n---",
                        "---\nitems:\n    - wrong indent\n---", "---\nitems: [one, two]\n---",
                        "---\nmodule: {nested: object}\n---", "---\nmodule: 'unclosed\n---"):
            with self.subTest(content=content):
                self.write(MODULE + "/spec.md", content)
                self.assert_problem("MAP_FRONT_MATTER_INVALID", MODULE + "/spec.md")

    def test_public_front_matter_shapes_and_quoted_scalars(self):
        fields = self.tree()
        fields["status"] = '"approved"'
        fields["delivery_id"] = "'" + DELIVERY_ID + "'"
        self.write(CHANGE + "/change.md", front_matter(fields, "[Hub](./README.md)"))
        self.assertEqual([], self.problems())
        api = self.api()
        fm = api.FrontMatter({"status": "approved"}, {"items": ("one",)})
        self.assertEqual(fm, api.MapDocument(Path("change.md"), fm).front_matter)

    def test_document_kind_and_filename_must_match(self):
        self.tree()
        self.change(document_kind="unknown")
        self.assert_problem("MAP_DOCUMENT_KIND_INVALID")
        self.change()
        self.write(CHANGE + "/extra.md", (self.root / (CHANGE + "/change.md")).read_text())
        self.assert_problem("MAP_DOCUMENT_KIND_INVALID", CHANGE + "/extra.md")

    def test_required_fields_and_statuses_have_stable_codes(self):
        self.tree()
        for field, value, code in (
            ("status", "completed", "MAP_STATUS_INVALID"),
            ("delivery_id", "other#IKEOFY", "MAP_DELIVERY_ID_INVALID"),
            ("issue", ISSUE_URL + "wrong", "MAP_ISSUE_URL_INVALID"),
            ("hub", MODULE + "/README.md", "MAP_HUB_INVALID"),
            ("affected_repositories", ["https://gitee.com/ming-industry/ming-iot"], "MAP_REPOSITORY_INVALID"),
            ("affected_repositories", [], "MAP_REPOSITORY_INVALID"),
            ("affected_modules", ["../outside"], "MAP_RELATED_PATH_INVALID")):
            with self.subTest(field=field):
                self.change(**{field: value})
                self.assert_problem(code, CHANGE + "/change.md")

    def test_all_document_kinds_accept_only_their_own_states(self):
        self.tree()
        states = {"spec": ("current",), "adr": ("accepted", "superseded"),
                  "reference": ("current", "superseded")}
        for kind, allowed in states.items():
            path = MODULE + {"spec": "/spec.md", "adr": "/adr/decision.md",
                             "reference": "/references/source.md"}[kind]
            for status in (*allowed, "invented"):
                self.write(path, front_matter(dict(document_kind=kind, status=status, module=MODULE,
                                                  source="https://example.com/source"),
                                             '<a id="behavior"></a>'))
                errors = [p for p in self.problems() if p.path == path and p.code == "MAP_STATUS_INVALID"]
                self.assertEqual(bool(errors), status == "invented")

    def test_hub_links_existing_artifacts_and_marks_missing_artifacts_as_text(self):
        self.tree()
        self.assertEqual([], self.problems())
        self.append(CHANGE + "/README.md", "[Plan](./plan.md)\n")
        self.assert_problem("MAP_DOCUMENT_MISSING", CHANGE + "/README.md")
        self.change()
        self.write(CHANGE + "/README.md", f"[Issue]({ISSUE_URL})\n[Change](./change.md)\n")
        self.assert_problem("MAP_HUB_INVALID", CHANGE + "/README.md")

    def test_navigation_indexes_and_artifact_backlinks_are_checked(self):
        self.tree()
        self.write(MODULE + "/README.md", "# module without index")
        self.assert_problem("MAP_HUB_INVALID", MODULE + "/README.md")
        self.tree()
        self.write("map/README.md", "# empty index")
        self.assert_problem("MAP_HUB_INVALID", "map/README.md")
        self.tree()
        self.artifact("plan", hub=MODULE + "/README.md")
        self.assert_problem("MAP_HUB_INVALID", CHANGE + "/plan.md")

    def test_links_outside_fences_include_reference_links_and_fragments(self):
        self.tree()
        self.append(CHANGE + "/change.md", "\n```markdown\n[example](missing.md)\n```\n"
                    "~~~markdown\n[example](missing-too.md)\n~~~\n[spec](../../spec.md#behavior)\n")
        self.assertEqual([], self.problems())
        self.append(CHANGE + "/change.md", "\n[broken][target]\n\n[target]: ./missing.md\n")
        self.assert_problem("MAP_DOCUMENT_MISSING")
        self.change()
        self.append(CHANGE + "/change.md", "\n[bad](../../spec.md#absent)\n")
        self.assert_problem("MAP_RELATED_PATH_INVALID")

    def test_review_and_evidence_heads_match_body_table(self):
        self.tree()
        for kind in ("review", "evidence"):
            self.artifact(kind)
        self.assertEqual([], self.problems())
        for kind in ("review", "evidence"):
            with self.subTest(kind=kind):
                self.artifact(kind, repository_heads=[REPOSITORY + "@" + "b" * 40])
                self.assert_problem("MAP_REPOSITORY_INVALID", CHANGE + f"/{kind}.md")
                self.artifact(kind, repository_heads=[REPOSITORY + "@abc123"])
                self.assert_problem("MAP_REPOSITORY_INVALID", CHANGE + f"/{kind}.md")
                self.artifact(kind)

    def test_existing_module_change_requires_stable_target_anchor(self):
        self.tree()
        for anchor in ("absent", "__module_root__", "行为"):
            self.change(anchors=(anchor,))
            self.assert_problem("MAP_TARGET_ANCHOR_INVALID")

    def test_new_module_feature_may_use_module_root_anchor(self):
        self.tree(False)
        self.assertEqual([], self.problems())
        self.change(anchors=("__module_root__",), change_type="patch")
        self.assert_problem("MAP_TARGET_ANCHOR_INVALID")

    def second_change(self, status):
        directory = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=directory, delivery="ming-iot-rd-docs#IOTHER", status=status)
        self.append(MODULE + "/README.md", "[其他](./changes/" + directory.split("/")[-1] + "/README.md)\n")
        return directory

    def test_two_active_changes_cannot_cover_the_same_anchor(self):
        self.tree()
        for status in ("approved", "implementing", "verifying"):
            self.second_change(status)
            self.assert_problem("MAP_ACTIVE_ANCHOR_CONFLICT")

    def test_rejected_and_effective_changes_do_not_conflict_with_active_change(self):
        self.tree()
        for status in ("draft", "rejected", "effective", "superseded"):
            directory = self.second_change(status)
            self.append(MODULE + "/spec.md", "\n[来源](./changes/" + directory.split("/")[-1] + "/change.md)\n")
            self.assertEqual([], self.problems(), status)

    def test_effective_change_requires_spec_provenance_link(self):
        self.tree()
        self.change(status="effective")
        self.assert_problem("MAP_SPEC_PROVENANCE_INVALID")
        self.append(MODULE + "/spec.md", "\n[来源](./changes/" + CHANGE.split("/")[-1] + "/change.md)\n")
        self.assertEqual([], self.problems())

    def test_effective_change_anchor_must_exist_in_module_spec(self):
        self.tree()
        self.change(status="effective", anchors=("absent",))
        self.append(MODULE + "/spec.md", "\n[来源](./changes/" + CHANGE.split("/")[-1] + "/change.md)\n")
        self.assert_problem("MAP_TARGET_ANCHOR_INVALID")

    def test_check_cli_has_stable_private_output_and_exit_status(self):
        self.tree()
        result = self.cli("check")
        self.assertEqual((0, "map docs: OK\n", ""), (result.returncode, result.stdout, result.stderr))
        self.change(issue="secret-token-do-not-print", status="invalid")
        result = self.cli("check")
        self.assertEqual(1, result.returncode)
        expected = "".join(f"{p.code} path={p.path}\n" for p in self.problems())
        self.assertEqual(expected, result.stdout)
        self.assertEqual("", result.stderr)
        self.assertNotIn(str(self.root), result.stdout)
        self.assertNotIn("secret-token", result.stdout)

    def test_init_creates_only_two_draft_documents_and_is_idempotent(self):
        self.write("map/README.md", f"[module](./{MODULE[4:]}/README.md)\n")
        self.write(MODULE + "/README.md", "[change](./changes/" + CHANGE.split("/")[-1] + "/README.md)\n")
        result = self.init()
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        directory = self.root / CHANGE
        self.assertEqual({"README.md", "change.md"}, {p.name for p in directory.iterdir()})
        self.assertIn("status: draft", (directory / "change.md").read_text())
        before = {p.name: p.read_bytes() for p in directory.iterdir()}
        self.assertEqual([], self.problems())
        result = self.init()
        self.assertEqual((0, "map change: existing\n"), (result.returncode, result.stdout))
        self.assertEqual(before, {p.name: p.read_bytes() for p in directory.iterdir()})

    def test_init_does_not_overwrite_conflicts_or_duplicate_delivery(self):
        self.write(CHANGE + "/README.md", "human prose")
        self.assertEqual(1, self.init().returncode)
        self.assertEqual("human prose", (self.root / CHANGE / "README.md").read_text())
        self.assertFalse((self.root / CHANGE / "change.md").exists())
        self.tree(False)
        result = self.init("--name", "另一个名字")
        self.assertEqual(1, result.returncode)
        self.assertFalse((self.root / (CHANGE.rsplit("_", 1)[0] + "_另一个名字")).exists())

    def test_init_rejects_unsafe_names_and_symlink_destinations_without_writes(self):
        for name in ("../escape", "bad/name", "bad\\name", ".", "..", "bad\nname", "bad#name"):
            with self.subTest(name=name):
                self.assertEqual(1, self.init("--name", name).returncode)
                self.assertEqual([], list(self.root.iterdir()))
        with tempfile.TemporaryDirectory() as outside:
            (self.root / "map").symlink_to(outside, target_is_directory=True)
            self.assertEqual(1, self.init().returncode)
            self.assertEqual([], list(Path(outside).iterdir()))

    def test_plan_front_matter_hub_is_a_sufficient_back_pointer(self):
        self.tree()
        self.artifact("plan")
        path = CHANGE + "/plan.md"
        text = (self.root / path).read_text().replace("[Hub](./README.md)", "实施步骤待完善。")
        self.write(path, text)
        self.assertEqual([], self.problems())

    def test_parenthesized_and_space_filenames_are_valid_link_targets(self):
        self.tree()
        for filename in ("report(1).md", "report 2.md"):
            self.write(MODULE + "/references/" + filename, front_matter(dict(
                document_kind="reference", status="current", module=MODULE, source="https://example.com")))
        self.append(CHANGE + "/change.md", '\n[one](../../references/report(1).md)\n'
                    '[two](<../../references/report 2.md> "title")\n')
        self.assertEqual([], self.problems())

    def test_change_hub_cannot_exist_without_change_spec(self):
        self.tree()
        (self.root / CHANGE / "change.md").unlink()
        self.write(CHANGE + "/README.md", "# abandoned hub\n")
        self.assert_problem("MAP_DOCUMENT_MISSING", CHANGE + "/change.md")

    def test_repository_relative_related_documents_are_confined(self):
        self.tree()
        self.change(related_documents=["../outside.md"])
        self.assert_problem("MAP_RELATED_PATH_INVALID", CHANGE + "/change.md")

    def test_reference_readme_with_front_matter_is_valid(self):
        self.tree()
        self.write(MODULE + "/references/README.md", front_matter(dict(
            document_kind="reference", status="current", module=MODULE, source="https://example.com")))
        self.assertEqual([], self.problems())

    def test_init_existing_module_does_not_guess_coverage(self):
        self.write(MODULE + "/spec.md", front_matter(dict(document_kind="spec", status="current", module=MODULE),
                   '<a id="unrelated-first-anchor"></a>\n<a id="another-anchor"></a>'))
        self.assertEqual(0, self.init().returncode)
        text = (self.root / CHANGE / "change.md").read_text()
        fm, _ = self.api().parse_front_matter(text)
        self.assertEqual((), fm.lists["target_anchors"])
        self.assertEqual("draft", fm.scalars["status"])

    def test_review_and_evidence_require_their_change_spec(self):
        self.tree()
        self.artifact("review")
        (self.root / CHANGE / "change.md").unlink()
        self.write(CHANGE + "/README.md", f"[Issue]({ISSUE_URL})\n[Review](./review.md)\n")
        self.assert_problem("MAP_DOCUMENT_MISSING", CHANGE + "/change.md")

    def test_optional_artifact_cannot_claim_another_delivery(self):
        self.tree()
        self.artifact("plan", delivery_id="ming-iot#I12345",
                      issue="https://gitee.com/ming-industry/ming-iot/issues/I12345")
        self.assert_problem("MAP_DELIVERY_ID_INVALID", CHANGE + "/plan.md")

    def test_stable_anchors_in_fenced_examples_do_not_count(self):
        self.tree()
        self.append(MODULE + "/spec.md", '\n```html\n<a id="example-only"></a>\n```\n')
        self.change(anchors=("example-only",))
        self.assert_problem("MAP_TARGET_ANCHOR_INVALID")

    def test_invalid_utf8_has_a_stable_front_matter_diagnostic(self):
        self.tree()
        (self.root / CHANGE / "change.md").write_bytes(b"\xff")
        self.assert_problem("MAP_FRONT_MATTER_INVALID", CHANGE + "/change.md")

    def test_empty_and_unknown_change_directories_are_rejected(self):
        self.tree()
        path = MODULE + "/changes/UNKNOWN-ming-iot-rd-docs-IOTHER_empty"
        (self.root / path).mkdir()
        self.assert_problem("MAP_CHANGE_TYPE_INVALID", path)
        self.assert_problem("MAP_DOCUMENT_MISSING", path + "/change.md")

    def test_cli_escapes_control_characters_in_invalid_filenames(self):
        self.tree()
        self.write(MODULE + "/references/bad\nname.md", "not metadata")
        result = self.cli("check")
        self.assertEqual(1, result.returncode)
        self.assertTrue(result.stdout.splitlines())
        self.assertTrue(all(re.match(r"^MAP_[A-Z_]+ path=map/", line)
                            for line in result.stdout.splitlines()), result.stdout)

    def test_required_fields_cannot_be_silently_omitted(self):
        fields = self.tree()
        required = {"document_kind": "MAP_DOCUMENT_KIND_INVALID", "status": "MAP_STATUS_INVALID",
                    "delivery_id": "MAP_DELIVERY_ID_INVALID", "issue": "MAP_ISSUE_URL_INVALID",
                    "hub": "MAP_HUB_INVALID", "change_type": "MAP_CHANGE_TYPE_INVALID",
                    "target_module": "MAP_RELATED_PATH_INVALID",
                    "affected_modules": "MAP_RELATED_PATH_INVALID",
                    "affected_repositories": "MAP_REPOSITORY_INVALID",
                    "target_anchors": "MAP_TARGET_ANCHOR_INVALID"}
        for key, code in required.items():
            with self.subTest(key=key):
                self.write(CHANGE + "/change.md", front_matter(
                    {k: v for k, v in fields.items() if k != key}, "[Hub](./README.md)"))
                self.assert_problem(code, CHANGE + "/change.md")

    def test_review_table_requires_base_exact_head_and_matching_verdict(self):
        self.tree()
        self.artifact("review")
        path = CHANGE + "/review.md"
        original = (self.root / path).read_text()
        for old, new in ((f"| `{BASE_HEAD}` |", "| |"), (f"| `{EXACT_HEAD}` |", "| `abc123` |"),
                         ("| pass |", "| changes_required |"), ("| exact_head |", "| head |")):
            with self.subTest(old=old):
                self.write(path, original.replace(old, new))
                self.assert_problem("MAP_REPOSITORY_INVALID", path)

    def test_review_and_evidence_require_immutable_full_base_in_normal_check(self):
        self.tree()
        for kind in ("review", "evidence"):
            self.artifact(kind)
        self.assertEqual(0, self.cli("check").returncode)
        for kind in ("review", "evidence"):
            path = CHANGE + f"/{kind}.md"
            original = (self.root / path).read_text()
            for base in ("master", "origin/master", "abc1234", "", "g" * 40):
                with self.subTest(kind=kind, base=base):
                    self.write(path, original.replace(f"`{BASE_HEAD}`", base))
                    result = self.cli("check")
                    self.assertEqual(1, result.returncode, result.stdout + result.stderr)
                    self.assertIn("MAP_REPOSITORY_INVALID", result.stdout + result.stderr)
                    self.assertIn(path, result.stdout + result.stderr)
            self.write(path, original)
            self.assertEqual(0, self.cli("check").returncode)

    def test_escaped_symlink_file_is_not_read(self):
        self.tree()
        with tempfile.TemporaryDirectory() as outside:
            secret = Path(outside) / "private.md"
            secret.write_text("must not be parsed or printed")
            (self.root / MODULE / "secret.md").symlink_to(secret)
            self.assert_problem("MAP_RELATED_PATH_INVALID", MODULE + "/secret.md")
            self.assertNotIn("must not be parsed", self.cli("check").stdout)

    def test_fixture_append_checks_confinement_before_reading(self):
        with tempfile.TemporaryDirectory(dir=self.root.parent) as outside:
            outside_file = Path(outside) / "outside.md"
            outside_file.write_text("synthetic outside content", encoding="utf-8")
            (self.root / "escape.md").symlink_to(outside_file)
            (self.root / "escape-dir").symlink_to(outside, target_is_directory=True)
            paths = ("../" + Path(outside).name + "/outside.md",
                     "escape.md", "escape-dir/outside.md", str(outside_file))
            for path in paths:
                with self.subTest(path=path):
                    # Observe the read boundary, not its data: even a read that
                    # is followed by a successful write guard violates isolation.
                    with patch.object(Path, "read_text", return_value="synthetic content") as read:
                        with self.assertRaises(AssertionError):
                            self.append(path, "must not append")
                        read.assert_not_called()
            self.assertEqual("synthetic outside content", outside_file.read_text(encoding="utf-8"))

    def test_fixture_append_preserves_in_root_behavior(self):
        self.write("nested/example.md", "first")
        self.append("nested/example.md", " second")
        self.assertEqual("first second", (self.root / "nested/example.md").read_text(encoding="utf-8"))

    def test_hub_empty_headings_and_punctuation_are_not_status_text(self):
        self.tree()
        for template in ("# {}", "## {}：...！？", "| {} | — |", "- **{}**: () []"):
            with self.subTest(template=template):
                self.write(CHANGE + "/README.md", f"[Issue]({ISSUE_URL})\n[Change](./change.md)\n"
                           + "\n".join(template.format(kind) for kind in ("Plan", "PR", "Review", "Evidence")))
                self.assert_problem("MAP_HUB_INVALID", CHANGE + "/README.md")

    def test_each_missing_hub_object_needs_status_text(self):
        self.tree()
        for kind in ("Plan", "PR", "Review", "Evidence"):
            with self.subTest(kind=kind):
                self.change()
                path = CHANGE + "/README.md"
                body = (self.root / path).read_text(encoding="utf-8")
                self.write(path, re.sub(r"^" + kind + r":.*$", "# " + kind, body, flags=re.M))
                self.assert_problem("MAP_HUB_INVALID", path)

    def test_hub_status_text_accepts_prose_without_fixed_vocabulary(self):
        self.tree()
        for template in ("# {} — Awaiting owner", "| {} | 尚未安排 |", "- **{}**: 等团队确认后安排"):
            with self.subTest(template=template):
                self.write(CHANGE + "/README.md", f"[Issue]({ISSUE_URL})\n[Change](./change.md)\n"
                           + "\n".join(template.format(kind) for kind in ("Plan", "PR", "Review", "Evidence")))
                self.assertEqual([], self.problems())

    def test_inline_code_and_commented_anchors_do_not_authorize_coverage(self):
        self.tree()
        anchor = '<a id="behavior"></a>'
        examples = (f'`{anchor}`', f'`` `{anchor}` ``', f'``\n{anchor}\n``',
                    f'<!-- {anchor} -->', f'<!--\n{anchor}\n-->', f'<!--\n{anchor}',
                    '    ' + anchor, '\t' + anchor)
        for body in examples:
            with self.subTest(body=body):
                self.write(MODULE + "/spec.md", front_matter(dict(
                    document_kind="spec", status="current", module=MODULE), body))
                self.assert_problem("MAP_TARGET_ANCHOR_INVALID", CHANGE + "/change.md")

    def test_stable_anchor_requires_an_actual_well_formed_id_attribute(self):
        self.tree()
        for body in ('<a data-id="behavior"></a>', '<a title=\'id="behavior"\'></a>',
                     '<a id="behavior\'></a>', '<a id="behavior">', '<a id="behavior" />',
                     '<a id="behavior" id="other"></a>',
                     '<div title=\'<a id="behavior"></a>\'></div>'):
            with self.subTest(body=body):
                self.write(MODULE + "/spec.md", front_matter(dict(
                    document_kind="spec", status="current", module=MODULE), body))
                self.assert_problem("MAP_TARGET_ANCHOR_INVALID", CHANGE + "/change.md")

    def test_real_anchors_support_quotes_attributes_and_unicode_ids(self):
        self.tree()
        for anchor, body in (("behavior", "<a id='behavior'></a>"),
                             ("behavior", '<a class="stable" id="behavior"></a>'),
                             ("行为-v2", '<a id="行为-v2"></a>')):
            with self.subTest(body=body):
                self.change(anchors=(anchor,))
                self.write(MODULE + "/spec.md", front_matter(dict(
                    document_kind="spec", status="current", module=MODULE), body))
                self.assertEqual([], self.problems())

    def test_fragment_links_cannot_target_inline_or_commented_anchor_examples(self):
        self.tree()
        for body in ('`<a id="example"></a>`', '<!-- <a id="example"></a> -->'):
            with self.subTest(body=body):
                self.write(MODULE + "/spec.md", front_matter(dict(
                    document_kind="spec", status="current", module=MODULE),
                    '<a id="behavior"></a>\n' + body))
                self.change()
                self.append(CHANGE + "/change.md", '\n[例子](../../spec.md#example)\n')
                self.assert_problem("MAP_RELATED_PATH_INVALID", CHANGE + "/change.md")


    def activation_tree(self):
        self.tree()
        self.change(status="effective")
        self.append(MODULE + "/spec.md", "\n[来源 Change](./changes/" + CHANGE.split("/")[-1] + "/change.md)\n")
        self.write(MODULE + "/README.md", "[规格](./spec.md)\n\n## 历史 Change\n[交付](./changes/" + CHANGE.split("/")[-1] + "/README.md)\n")
        for kind in ("review", "evidence"):
            self.artifact(kind)
        self.append(CHANGE + "/README.md", "\n| role | repository | pr | base | exact_head | status | merge_revision |\n"
                    "|---|---|---|---|---|---|---|\n"
                    f"| prerequisite | {REPOSITORY} | https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/1 | {'b' * 40} | {EXACT_HEAD} | merged | {'c' * 40} |\n"
                    f"| current | {REPOSITORY} | https://gitee.com/ming-industry/ming-iot-rd-docs/pulls/2 | {'b' * 40} | {EXACT_HEAD} | draft | - |\n")

    def test_unused_reference_definitions_are_not_root_navigation(self):
        self.tree()
        self.write("map/README.md", f"[module]: ./{MODULE[4:]}/README.md\n")
        self.assert_problem("MAP_HUB_INVALID", "map/README.md")

    def test_unused_reference_definitions_are_not_hub_navigation(self):
        self.tree()
        self.write(CHANGE + "/README.md", f"[issue]: {ISSUE_URL}\n[change]: ./change.md\n"
                   "Plan: pending\nReview: pending\nEvidence: pending\nPR: pending\n")
        self.assert_problem("MAP_HUB_INVALID", CHANGE + "/README.md")

    def test_used_reference_links_are_visible_navigation(self):
        self.tree()
        for link in ("[label][module]", "[module][]", "[module]", "[MODULE]"):
            with self.subTest(link=link):
                self.write("map/README.md", link + f"\n\n[module]: ./{MODULE[4:]}/README.md\n")
                self.assertEqual([], self.problems())

    def test_image_and_escaped_reference_are_not_navigation(self):
        self.tree()
        for link in ("![module]", r"\[module]", "![module](./" + MODULE[4:] + "/README.md)"):
            with self.subTest(link=link):
                self.write("map/README.md", link + f"\n\n[module]: ./{MODULE[4:]}/README.md\n")
                self.assert_problem("MAP_HUB_INVALID", "map/README.md")

    def test_unused_reference_still_checks_target_existence(self):
        self.tree()
        self.append("map/README.md", "\n[unused]: ./missing.md\n")
        self.assert_problem("MAP_DOCUMENT_MISSING", "map/README.md")

    def test_navigation_rejects_incomplete_links_and_title_references(self):
        self.tree()
        target = f"./{MODULE[4:]}/README.md"
        for body in (f"[模块]({target}", f"[模块](<{target}>",
                     f'[其他](https://example.invalid/ "[module]")\n\n[module]: {target}\n',
                     f"[module]\n[module]: {target}\n",
                     f"[模块]({target}\n\n)", f'[模块]({target} "one\n\ntwo")',
                     f"`正文`\n[module]: {target}\n\n[module]\n"):
            with self.subTest(body=body):
                self.write("map/README.md", body)
                self.assert_problem("MAP_HUB_INVALID", "map/README.md")

    def test_navigation_accepts_complete_titles_and_reference_definitions(self):
        self.tree()
        target = f"./{MODULE[4:]}/README.md"
        for body in (f'[模块]({target} "说明 [not-a-link]")',
                     f"[模块](<{target}> '说明')", f"[模块]({target} (说明))",
                     f"[module]: {target}\n\n[module]\n"):
            with self.subTest(body=body):
                self.write("map/README.md", body)
                self.assertEqual([], self.problems())

    def test_reference_definitions_after_block_boundaries_are_navigation(self):
        self.tree()
        target = f"./{MODULE[4:]}/README.md"
        for boundary in ("正文\n```text\nexample\n```\n", "正文\n***\n",
                         "正文\n---\n", "正文\n___\n"):
            with self.subTest(boundary=boundary):
                self.write("map/README.md", boundary + f"[module]: {target}\n\n[module]\n")
                self.assertEqual([], self.problems())

    def activate(self):
        return self.cli("activate", "--change", CHANGE)

    def commit_baseline(self):
        def git(*args):
            return subprocess.run(["git", "-C", str(self.root), *args],
                                  capture_output=True, text=True, check=True).stdout.strip()
        git("init", "-q")
        git("add", "map")
        git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
            "-c", "core.hooksPath=/dev/null", "commit", "-qm", "baseline")
        return git("rev-parse", "HEAD")

    def test_check_automatically_rejects_effective_without_evidence(self):
        self.tree()
        baseline = self.commit_baseline()
        self.activation_tree()
        for kind in ("review", "evidence"):
            (self.root / (CHANGE + f"/{kind}.md")).unlink()
        hub = CHANGE + "/README.md"
        self.write(hub, re.sub(r"^\[(review|evidence)\].*$", "", (self.root / hub).read_text(), flags=re.M))
        result = self.cli("check")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("MAP_DOCUMENT_MISSING", result.stdout)

    def test_check_automatically_rejects_new_effective_conflict(self):
        self.tree()
        baseline = self.commit_baseline()
        self.activation_tree()
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[另一个](./changes/" + other.split("/")[-1] + "/README.md)\n")
        result = self.cli("check", "--base", baseline)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", result.stdout)

    def test_check_allows_valid_new_effective_and_ordinary_verifying(self):
        self.tree()
        baseline = self.commit_baseline()
        self.change(status="verifying")
        self.assertEqual(0, self.cli("check", "--base", baseline).returncode)
        self.activation_tree()
        result = self.cli("check", "--base", baseline)
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_check_does_not_reactivate_unchanged_historical_effective(self):
        self.activation_tree()
        baseline = self.commit_baseline()
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[另一个](./changes/" + other.split("/")[-1] + "/README.md)\n")
        result = self.cli("check", "--base", baseline)
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)
        self.append(MODULE + "/spec.md", '\n<a id="unrelated"></a>\n## 其他章节\n无行为变化。\n')
        result = self.cli("check", "--base", baseline)
        self.assertEqual(0, result.returncode, result.stdout + result.stderr)

    def test_check_rechecks_updated_effective_spec_section(self):
        self.activation_tree()
        baseline = self.commit_baseline()
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[另一个](./changes/" + other.split("/")[-1] + "/README.md)\n")
        path = MODULE + "/spec.md"
        self.write(path, (self.root / path).read_text().replace("当前行为。", "本次修改行为。"))
        result = self.cli("check", "--base", baseline)
        self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", result.stdout)

    def test_committed_code_changes_in_spec_trigger_check_and_real_hook(self):
        self.activation_tree()
        spec = MODULE + "/spec.md"
        self.append(spec, '\n超时 `30` 秒。\n\n```yaml\nrollout_batch: 30\n'
                    '<a id="fake"></a>\n```\n\n    limit: 30\n')
        original = (self.root / spec).read_text()
        baseline = self.commit_baseline()
        subprocess.run(["git", "-C", str(self.root), "update-ref",
                        "refs/remotes/origin/master", baseline], check=True)
        (self.root / "tools").mkdir()
        shutil.copy2(CLI, self.root / "tools/map_docs.py")
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[候选](./changes/" + other.split("/")[-1] + "/README.md)\n")
        self.commit_baseline()

        def hook():
            return subprocess.run(["sh", str(CLI.parents[1] / ".githooks/pre-push"),
                                   "origin", "https://gitee.com/ming-industry/ming-iot-rd-docs.git"],
                                  cwd=self.root, input="", text=True, capture_output=True)

        self.assertEqual(0, self.cli("check").returncode)
        self.assertEqual(0, hook().returncode)
        self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", self.activate().stdout)
        for old, new in (("`30`", "`60`"), ("rollout_batch: 30", "rollout_batch: 60"),
                         ("limit: 30", "limit: 60"), ("当前行为。", "修改后的行为。")):
            with self.subTest(change=old):
                self.write(spec, original.replace(old, new))
                self.commit_baseline()
                for result in (self.cli("check"), hook()):
                    self.assertEqual(1, result.returncode, result.stdout + result.stderr)
                    self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", result.stdout)

    def test_check_without_baseline_does_not_silently_trust_effective(self):
        self.activation_tree()
        (self.root / (CHANGE + "/review.md")).unlink()
        hub = CHANGE + "/README.md"
        self.write(hub, (self.root / hub).read_text().replace("[review](./review.md)", ""))
        self.assertIn("MAP_DOCUMENT_MISSING", self.cli("check").stdout)

    def test_check_invalid_explicit_baseline_fails_closed(self):
        self.tree()
        self.assertNotEqual(0, self.cli("check", "--base", "missing-base").returncode)

    def test_real_hook_rejects_committed_effective_candidates_against_master(self):
        self.tree()
        baseline = self.commit_baseline()
        subprocess.run(["git", "-C", str(self.root), "update-ref", "refs/remotes/origin/master", baseline], check=True)
        (self.root / "tools").mkdir()
        shutil.copy2(CLI, self.root / "tools/map_docs.py")
        hook = CLI.parents[1] / ".githooks/pre-push"

        def check_hook():
            return subprocess.run(["sh", str(hook), "origin",
                                   "https://gitee.com/ming-industry/ming-iot-rd-docs.git"],
                                  cwd=self.root, input="", text=True, capture_output=True)

        self.assertEqual(0, check_hook().returncode)
        self.activation_tree()
        self.commit_baseline()
        self.assertEqual(0, check_hook().returncode)
        (self.root / (CHANGE + "/review.md")).unlink()
        hub = CHANGE + "/README.md"
        self.write(hub, (self.root / hub).read_text().replace("[review](./review.md)", ""))
        self.commit_baseline()
        missing = check_hook()
        self.assertNotEqual(0, missing.returncode)
        self.assertIn("MAP_DOCUMENT_MISSING", missing.stdout)
        self.activation_tree()
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[另一个](./changes/" + other.split("/")[-1] + "/README.md)\n")
        self.commit_baseline()
        conflict = check_hook()
        self.assertNotEqual(0, conflict.returncode)
        self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", conflict.stdout)

    def test_activation_passes_without_writing_files_or_requiring_current_pr_merged(self):
        self.activation_tree()
        before = {p.relative_to(self.root): p.read_bytes() for p in self.root.rglob("*.md")}
        result = self.activate()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("map activation: OK", result.stdout)
        self.assertEqual(before, {p.relative_to(self.root): p.read_bytes() for p in self.root.rglob("*.md")})

    def test_activation_requires_effective_status_in_candidate_tree(self):
        self.activation_tree()
        path = CHANGE + "/change.md"
        self.write(path, (self.root / path).read_text().replace("status: effective", "status: verifying"))
        self.assertIn("MAP_ACTIVATION_STATUS_INVALID", self.activate().stdout)

    def test_activation_requires_review_and_evidence_files(self):
        for kind in ("review", "evidence"):
            self.activation_tree()
            (self.root / (CHANGE + f"/{kind}.md")).unlink()
            result = self.activate()
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("MAP_DOCUMENT_MISSING", result.stdout)

    def test_activation_requires_pass_for_every_repository_head(self):
        self.activation_tree()
        for kind in ("review", "evidence"):
            path = CHANGE + f"/{kind}.md"
            original = (self.root / path).read_text()
            self.write(path, original.replace(EXACT_HEAD, "d" * 40))
            self.assertIn("MAP_ACTIVATION_EVIDENCE_INVALID", self.activate().stdout)
            self.write(path, original.replace("b" * 40, "master"))
            self.assertIn("MAP_ACTIVATION_EVIDENCE_INVALID", self.activate().stdout)
            self.write(path, original)
        path = CHANGE + "/change.md"
        self.write(path, (self.root / path).read_text().replace("affected_repositories:\n", "affected_repositories:\n  - git:gitee.com/ming-industry/ming-iot\n"))
        self.assertIn("MAP_ACTIVATION_EVIDENCE_INVALID", self.activate().stdout)

    def test_activation_requires_all_prerequisite_deliveries_merged(self):
        self.activation_tree()
        path = CHANGE + "/README.md"
        original = (self.root / path).read_text()
        for invalid in (original.replace("| merged |", "| open |"), original.replace("c" * 40, "-"),
                        original.replace("| prerequisite |", "| unknown |")):
            self.write(path, invalid)
            self.assertIn("MAP_ACTIVATION_DEPENDENCY_INVALID", self.activate().stdout)

    def test_activation_requires_module_history_index(self):
        self.activation_tree()
        path = MODULE + "/README.md"
        self.write(path, (self.root / path).read_text().replace("历史 Change", "活动 Change"))
        self.assertIn("MAP_ACTIVATION_HISTORY_INVALID", self.activate().stdout)

    def test_activation_requires_spec_anchor_and_nearby_provenance(self):
        self.activation_tree()
        path = MODULE + "/spec.md"
        original = (self.root / path).read_text()
        self.write(path, original.replace('[来源 Change]', '<a id="other-section"></a>\n[来源 Change]'))
        self.assertIn("MAP_SPEC_PROVENANCE_INVALID", self.activate().stdout)
        self.write(path, original.replace('<a id="behavior"></a>', '<a id="wrong"></a>'))
        self.assertNotEqual(self.activate().returncode, 0)

    def test_activation_rejects_unrelated_active_anchor_conflict(self):
        self.activation_tree()
        other = CHANGE.replace("IKEOFY", "IOTHER")
        self.change(directory=other, delivery="ming-iot-rd-docs#IOTHER")
        self.append(MODULE + "/README.md", "\n[另一个](./changes/" + other.split("/")[-1] + "/README.md)\n")
        self.assertIn("MAP_ACTIVE_ANCHOR_CONFLICT", self.activate().stdout)

    def test_activation_provenance_must_not_cross_unanchored_section(self):
        self.activation_tree()
        path = MODULE + "/spec.md"
        self.write(path, (self.root / path).read_text().replace("[来源 Change]", "## 不相关章节\n[来源 Change]"))
        self.assertIn("MAP_SPEC_PROVENANCE_INVALID", self.activate().stdout)

    def test_activation_canonical_anchor_cannot_be_attribute_text(self):
        self.activation_tree()
        path = MODULE + "/spec.md"
        self.write(path, (self.root / path).read_text().replace('<a id="behavior"></a>',
                   "<a id='behavior'></a>\n<div title='<a id=\"behavior\"></a>'></div>"))
        self.assertIn("MAP_TARGET_ANCHOR_INVALID", self.activate().stdout)

    def test_activation_history_cannot_be_a_commented_link(self):
        self.activation_tree()
        path = MODULE + "/README.md"
        self.write(path, (self.root / path).read_text().replace('[交付](', '<!-- [交付](').replace('/README.md)\n', '/README.md) -->\n'))
        self.assertIn("MAP_ACTIVATION_HISTORY_INVALID", self.activate().stdout)


if __name__ == "__main__":
    unittest.main()
