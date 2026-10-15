from pathlib import Path
import os
import sys
import tempfile
import unittest

# Override only for a matched run against the untouched upstream scripts.
sys.path.insert(0, os.environ.get("RDOC_SCRIPTS_ROOT", str(Path(__file__).parents[1] / "scripts")))

import audit_docs
from helpers import write_file


class PilotRegressionTests(unittest.TestCase):
    def minimal_project(self, root: Path, entry: str) -> None:
        write_file(root, ".r-doc.yaml", "version: 1\ngovernance_level: minimal\n")
        write_file(root, "AGENTS.md", "# Entry\n\n" + entry)

    def test_inline_angle_link_preserves_spaces_and_fragment(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[Device](<docs/device status.md#设备状态>)\n")
            write_file(root, "docs/device status.md", "# Device\n\n## 设备状态\n")
            self.assertEqual(audit_docs.audit(root), [])

    def test_reference_angle_link_preserves_spaces(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[Device][device]\n\n[device]: <docs/device status.md>\n")
            write_file(root, "docs/device status.md", "# Device\n")
            self.assertEqual(audit_docs.audit(root), [])

    def test_encoded_space_and_plain_link_title_still_work(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, '[Device](docs/device%20status.md "Device status")\n')
            write_file(root, "docs/device status.md", "# Device\n")
            self.assertEqual(audit_docs.audit(root), [])

    def test_disconnected_index_does_not_make_its_topic_reachable(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "No documentation route yet.\n")
            write_file(root, "docs/README.md", "# Docs\n\n[Topic](topic.md)\n")
            write_file(root, "docs/topic.md", "# Topic\n")
            findings = audit_docs.audit(root)
            unreachable = {item.path for item in findings if item.code == "unindexed-document"}
            self.assertEqual(unreachable, {"docs/README.md", "docs/topic.md"})

    def test_disconnected_index_cycle_is_not_reachable(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[Docs](docs/README.md)\n")
            write_file(root, "docs/README.md", "# Docs\n")
            write_file(root, "docs/a/README.md", "# A\n\n[B](../b/README.md)\n")
            write_file(root, "docs/b/README.md", "# B\n\n[A](../a/README.md)\n")
            findings = audit_docs.audit(root)
            unreachable = {item.path for item in findings if item.code == "unindexed-document"}
            self.assertEqual(unreachable, {"docs/a/README.md", "docs/b/README.md"})

    def test_reachable_cycle_and_supplementary_topic_pass(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[Docs](docs/README.md)\n")
            write_file(root, "docs/README.md", "# Docs\n\n[Topic](topic.md)\n")
            write_file(root, "docs/topic.md", "# Topic\n\n[Details](details.md)\n")
            write_file(root, "docs/details.md", "# Details\n\n[Topic](topic.md)\n")
            self.assertEqual(audit_docs.audit(root), [])

    def test_root_readme_can_route_to_docs_without_indexing_other_root_files(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[Project](README.md)\n")
            write_file(root, "README.md", "# Project\n\n[Docs](docs/README.md)\n")
            write_file(root, "docs/README.md", "# Docs\n\n[Topic](topic.md)\n")
            write_file(root, "docs/topic.md", "# Topic\n")
            write_file(root, "LICENSE.md", "# License\n")
            self.assertEqual(audit_docs.audit(root), [])

    def test_unused_definition_does_not_create_a_route(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.minimal_project(root, "[unused]: <docs/device status.md>\n")
            write_file(root, "docs/device status.md", "# Device\n")
            findings = audit_docs.audit(root)
            self.assertEqual([item.code for item in findings], ["unindexed-document"])


if __name__ == "__main__":
    unittest.main()
