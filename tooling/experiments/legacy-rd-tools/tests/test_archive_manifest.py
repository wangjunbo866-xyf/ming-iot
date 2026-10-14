import os
import tempfile
import unittest
from pathlib import Path

from tools.archive_manifest import ArchiveError, build_manifest, verify_manifest


class ArchiveManifestTest(unittest.TestCase):
    def make_archive(self, parent: Path) -> tuple[Path, Path]:
        archive = parent / "archives"
        manifest = archive / "manifests" / "archive-files.json"
        (archive / "snapshots").mkdir(parents=True)
        (archive / "snapshots" / "proof.txt").write_text("proof\n", encoding="utf-8")
        return archive, manifest

    def test_build_is_deterministic_and_verify_accepts_exact_tree(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            first = build_manifest(archive, manifest)
            first_bytes = manifest.read_bytes()
            second = build_manifest(archive, manifest)
            self.assertEqual(first, second)
            self.assertEqual(first_bytes, manifest.read_bytes())
            verify_manifest(archive, manifest)

    def test_verify_rejects_modified_file(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            build_manifest(archive, manifest)
            (archive / "snapshots" / "proof.txt").write_text("changed\n", encoding="utf-8")
            with self.assertRaisesRegex(ArchiveError, "archive file set or digest changed"):
                verify_manifest(archive, manifest)

    def test_verify_rejects_added_file(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            build_manifest(archive, manifest)
            (archive / "snapshots" / "extra.txt").write_text("extra\n", encoding="utf-8")
            with self.assertRaisesRegex(ArchiveError, "archive file set or digest changed"):
                verify_manifest(archive, manifest)

    def test_build_rejects_symlink(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            archive, manifest = self.make_archive(Path(temporary))
            os.symlink(archive / "snapshots" / "proof.txt", archive / "snapshots" / "alias.txt")
            with self.assertRaisesRegex(ArchiveError, "symlink"):
                build_manifest(archive, manifest)


if __name__ == "__main__":
    unittest.main()
