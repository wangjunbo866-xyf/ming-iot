import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from tools.check_authority_catalog import AuthorityCatalogError, validate


REPO = Path(__file__).resolve().parents[1]
SCRIPT = REPO / "tools" / "check_authority_catalog.py"


class AuthorityCatalogTest(unittest.TestCase):
    def make_fixture(self, parent: Path) -> tuple[Path, Path, Path]:
        docs_root = parent / "docs"
        (docs_root / "specs").mkdir(parents=True)
        (docs_root / "archives").mkdir()
        (docs_root / "README.md").write_text("entry\n", encoding="utf-8")
        (docs_root / "specs" / "design.md").write_text("design\n", encoding="utf-8")
        (docs_root / "archives" / "historical.md").write_text("history\n", encoding="utf-8")
        catalog = docs_root / "DOC_REGISTRY.yaml"
        registry = parent / "registry.json"
        self.write_catalog(catalog)
        self.write_registry(registry)
        return docs_root, catalog, registry

    def write_catalog(self, path: Path, *, design_path: str = "specs/design.md") -> None:
        path.write_text(
            "default_entry_docs:\n"
            "  - README.md\n"
            "topic_authority:\n"
            f"  design: {design_path}\n"
            "  history: archives/historical.md\n",
            encoding="utf-8",
        )

    def write_registry(
        self,
        path: Path,
        *,
        design_source: str = "design-source",
        extra_material: bool = False,
        raw_source_ref: str | None = None,
    ) -> None:
        materials = {
            "design-source": {"id": "design-source", "source_ref_sha256": "abc123"},
            "history-source": {"id": "history-source", "source_ref_sha256": "def456"},
        }
        if extra_material:
            materials["orphan-source"] = {"id": "orphan-source", "source_ref_sha256": "orphan"}
        if raw_source_ref is not None:
            materials["design-source"]["source_ref"] = raw_source_ref
        path.write_text(
            json.dumps(
                {
                    "goals": [
                        {
                            "id": "ming-iot-framework-v1",
                            "authority_registry": {
                                "project_materials": materials,
                                "topic_authority": {
                                    "design": design_source,
                                    "history": "history-source",
                                },
                            },
                        }
                    ]
                }
            ),
            encoding="utf-8",
        )

    def test_valid_catalog_and_registry_pass(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.assertEqual(
                validate(catalog, docs_root, registry, "ming-iot-framework-v1"),
                [],
            )
            for invalid_document in ([], None, 1, "invalid"):
                registry.write_text(json.dumps(invalid_document), encoding="utf-8")
                with self.assertRaises(AuthorityCatalogError) as raised:
                    validate(catalog, docs_root, registry, "ming-iot-framework-v1")
                self.assertEqual(str(raised.exception), "REGISTRY_UNREADABLE")
            self.write_registry(registry)
            document = json.loads(registry.read_text(encoding="utf-8"))
            duplicate = document["goals"][0].copy()
            document["goals"].append(duplicate)
            registry.write_text(json.dumps(document), encoding="utf-8")
            with self.assertRaises(AuthorityCatalogError) as raised:
                validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertEqual(str(raised.exception), "GOAL_NOT_FOUND")
            document["goals"] = []
            registry.write_text(json.dumps(document), encoding="utf-8")
            with self.assertRaises(AuthorityCatalogError) as raised:
                validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertEqual(str(raised.exception), "GOAL_NOT_FOUND")
            self.write_registry(registry)
            document = json.loads(registry.read_text(encoding="utf-8"))
            document["goals"][0]["authority_registry"]["project_materials"]["design-source"] = {}
            registry.write_text(json.dumps(document), encoding="utf-8")
            with self.assertRaises(AuthorityCatalogError) as raised:
                validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertEqual(str(raised.exception), "REGISTRY_UNREADABLE")

    def test_unreferenced_material_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_registry(registry, extra_material=True)
            codes = {issue.code for issue in validate(catalog, docs_root, registry, "ming-iot-framework-v1")}
            self.assertIn("MATERIAL_UNREFERENCED", codes)

    def test_topic_pointing_to_missing_source_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_registry(registry, design_source="missing-source")
            codes = {issue.code for issue in validate(catalog, docs_root, registry, "ming-iot-framework-v1")}
            self.assertIn("TOPIC_SOURCE_MISSING", codes)
            self.write_registry(registry)
            document = json.loads(registry.read_text(encoding="utf-8"))
            document["goals"][0]["authority_registry"]["topic_authority"]["unmapped"] = "design-source"
            registry.write_text(json.dumps(document), encoding="utf-8")
            codes = {issue.code for issue in validate(catalog, docs_root, registry, "ming-iot-framework-v1")}
            self.assertIn("TOPIC_DOCUMENT_MISSING", codes)

    def test_missing_document_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_catalog(catalog, design_path="specs/missing.md")
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertIn(
                ("DOC_NOT_FOUND", "specs/missing.md"),
                {(issue.code, issue.path) for issue in issues},
            )

    def test_absolute_and_parent_paths_are_rejected(self) -> None:
        for invalid in (
            "/tmp/private.md",
            "../private.md",
            "specs\\private.md",
            "./specs/design.md",
            "specs/./design.md",
            "https://internal.example.invalid/private-doc",
            "C:/private.md",
        ):
            with self.subTest(path=invalid), tempfile.TemporaryDirectory() as temporary:
                docs_root, catalog, registry = self.make_fixture(Path(temporary))
                self.write_catalog(catalog, design_path=invalid)
                issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
                self.assertIn(
                    ("DOC_PATH_INVALID", invalid),
                    {(issue.code, issue.path) for issue in issues},
                )
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            (docs_root / "linked").symlink_to(docs_root / "specs", target_is_directory=True)
            linked_path = "linked/design.md"
            self.write_catalog(catalog, design_path=linked_path)
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertIn(
                ("DOC_PATH_INVALID", linked_path),
                {(issue.code, issue.path) for issue in issues},
            )
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            linked_root = Path(temporary) / "linked-docs"
            linked_root.symlink_to(docs_root, target_is_directory=True)
            issues = validate(catalog, linked_root, registry, "ming-iot-framework-v1")
            self.assertIn("DOC_PATH_INVALID", {issue.code for issue in issues})
            self.write_catalog(catalog, design_path="specs")
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertIn(("DOC_NOT_FOUND", "specs"), {(issue.code, issue.path) for issue in issues})

    def test_historical_archive_document_is_allowed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            issues = validate(catalog, docs_root, registry, "ming-iot-framework-v1")
            self.assertNotIn("DOC_PATH_INVALID", {issue.code for issue in issues})
            self.assertEqual(issues, [])
            for invalid_catalog in (
                "default_entry_docs:\n    - README.md\ntopic_authority:\n  design: specs/design.md\n",
                "default_entry_docs:\n  - README.md\ndefault_entry_docs:\n  - README.md\ntopic_authority:\n  design: specs/design.md\n",
                "default_entry_docs:\n  - README.md\ntopic_authority:\n  design: specs/design.md\ntopic_authority:\n  history: archives/historical.md\n",
                "default_entry_docs: README.md\ntopic_authority:\n  design: specs/design.md\n",
                "default_entry_docs:\n\t- README.md\ntopic_authority:\n  design: specs/design.md\n",
            ):
                catalog.write_text(invalid_catalog, encoding="utf-8")
                with self.assertRaises(AuthorityCatalogError) as raised:
                    validate(catalog, docs_root, registry, "ming-iot-framework-v1")
                self.assertEqual(str(raised.exception), "CATALOG_FORMAT_INVALID")
            catalog.write_text(
                "metadata:\n    ignored: value\ndefault_entry_docs:\n  - README.md\ntopic_authority:\n  design: specs/design.md\n  history: archives/historical.md\n",
                encoding="utf-8",
            )
            self.assertEqual(validate(catalog, docs_root, registry, "ming-iot-framework-v1"), [])

    def test_cli_output_does_not_leak_absolute_path_or_raw_source_ref(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root, catalog, registry = self.make_fixture(Path(temporary))
            self.write_catalog(catalog, design_path="specs/missing.md")
            secret = "https://internal.example.invalid/private-doc"
            self.write_registry(registry, raw_source_ref=secret)
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            combined = result.stdout + result.stderr
            self.assertEqual(result.returncode, 1)
            self.assertIn("DOC_NOT_FOUND", combined)
            self.assertIn("path=specs/missing.md", combined)
            self.assertNotIn(str(docs_root), combined)
            self.assertNotIn(secret, combined)
            for invalid_path in ("/tmp/private.md", secret):
                self.write_catalog(catalog, design_path=invalid_path)
                result = subprocess.run(
                    [
                        sys.executable,
                        str(SCRIPT),
                        "--catalog",
                        str(catalog),
                        "--docs-root",
                        str(docs_root),
                        "--registry",
                        str(registry),
                        "--goal-id",
                        "ming-iot-framework-v1",
                    ],
                    cwd=REPO,
                    check=False,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )
                combined = result.stdout + result.stderr
                self.assertEqual(result.returncode, 1)
                self.assertIn("DOC_PATH_INVALID", combined)
                self.assertNotIn(invalid_path, combined)
            catalog.write_bytes(b"\xff")
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            combined = result.stdout + result.stderr
            self.assertEqual(result.returncode, 1)
            self.assertIn("CATALOG_UNREADABLE", combined)
            self.assertNotIn("Traceback", combined)
            self.assertNotIn(str(docs_root), combined)
            self.write_catalog(catalog)
            registry.write_bytes(b"\xff")
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            combined = result.stdout + result.stderr
            self.assertEqual(result.returncode, 1)
            self.assertIn("REGISTRY_UNREADABLE", combined)
            self.assertNotIn("Traceback", combined)
            self.assertNotIn(str(docs_root), combined)
            self.write_registry(registry)
            document = json.loads(registry.read_text(encoding="utf-8"))
            authority = document["goals"][0]["authority_registry"]
            authority["project_materials"] = {
                "https://internal.example.invalid/material": {"id": "x"},
                "bad\nmaterial": {"id": "y"},
                "bad\x1bmaterial": {"id": "z"},
            }
            authority["topic_authority"] = {
                "/absolute-topic": "https://internal.example.invalid/source",
                "bad\ntopic": "bad\x1bsource",
            }
            registry.write_text(json.dumps(document), encoding="utf-8")
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            combined = result.stdout + result.stderr
            self.assertEqual(combined, "REGISTRY_UNREADABLE\n")
            self.write_catalog(catalog)
            self.write_registry(registry)
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--catalog",
                    str(catalog),
                    "--docs-root",
                    str(docs_root),
                    "--registry",
                    str(registry),
                    "--goal-id",
                    "ming-iot-framework-v1",
                ],
                cwd=REPO,
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "authority catalog: OK topics=2 materials=2\n")
            self.assertEqual(result.stderr, "")


if __name__ == "__main__":
    unittest.main()
