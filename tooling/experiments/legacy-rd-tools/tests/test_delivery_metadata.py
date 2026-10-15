import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tools import check_delivery_metadata


REPO = Path(__file__).resolve().parents[1]
SCRIPT = REPO / "tools" / "check_delivery_metadata.py"
VALID_EXACT_HEAD = "a" * 40
VALID_REPOSITORY = "git:gitee.com/ming-industry/ming-iot"


class DeliveryMetadataTest(unittest.TestCase):
    def make_docs_root(self, parent: Path) -> Path:
        docs_root = parent / "docs"
        docs_root.mkdir()
        (docs_root / "CONTEXT.md").write_text("context\n", encoding="utf-8")
        return docs_root

    def write_document(self, docs_root: Path, relative: str, content: str) -> None:
        path = docs_root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def front_matter(
        self,
        kind: str,
        status: str,
        *,
        delivery_issue: str = "ming-iot#IKD4OR",
        code_repository: str = VALID_REPOSITORY,
        related_documents: tuple[str, ...] = ("CONTEXT.md",),
        exact_head: str | None = None,
    ) -> str:
        lines = [
            "---",
            f"document_kind: {kind}",
            f"status: {status}",
            f"delivery_issue: {delivery_issue}",
            f"code_repository: {code_repository}",
        ]
        if exact_head is not None:
            lines.append(f"exact_head: {exact_head}")
        lines.append("related_documents:")
        lines.extend(f"  - {relative}" for relative in related_documents)
        lines.extend(("---", "", "# Document", ""))
        return "\n".join(lines)

    def run_checker(
        self, docs_root: Path, since: str = "2026-09-03"
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--docs-root",
                str(docs_root),
                "--since",
                since,
            ],
            cwd=REPO,
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    def test_legacy_documents_and_readmes_do_not_require_front_matter(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            for relative in (
                "rfcs/README.md",
                "specs/README.md",
                "plans/2026-09-02-legacy.md",
                "reviews/2026-09-02-legacy-review.md",
                "evidence/2026-09-02-legacy/evidence.md",
            ):
                self.write_document(docs_root, relative, "# Legacy or entry document\n")

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, "delivery metadata: OK\n")
            self.assertEqual(result.stderr, "")

    def test_readme_in_dated_directory_is_not_a_delivery_document(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            self.write_document(
                docs_root,
                "evidence/2026-09-03-run/README.md",
                "# Evidence entry document\n",
            )

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, "delivery metadata: OK\n")
            self.assertEqual(result.stderr, "")

    def test_new_delivery_documents_without_front_matter_are_rejected_in_path_order(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            relatives = (
                "rfcs/2026-09-03-rfc.md",
                "specs/2026-09-03-spec.md",
                "plans/2026-09-03-plan.md",
                "reviews/2026-09-03-review.md",
                "evidence/2026-09-03-run/evidence.md",
            )
            for relative in relatives:
                self.write_document(docs_root, relative, "# Missing metadata\n")

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, "")
            self.assertEqual(
                result.stderr.splitlines(),
                [
                    f"FRONT_MATTER_MISSING path={relative}"
                    for relative in sorted(relatives)
                ],
            )

    def test_dates_in_review_and_evidence_subdirectories_are_validated(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            relatives = (
                "reviews/delivery/2026-09-03-run/review.md",
                "evidence/delivery/2026-09-03-run/evidence.md",
            )
            for relative in relatives:
                self.write_document(docs_root, relative, "# Missing metadata\n")

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr.splitlines(),
                [
                    f"FRONT_MATTER_MISSING path={relative}"
                    for relative in sorted(relatives)
                ],
            )

    def test_document_kind_is_valid_and_matches_its_directory(self) -> None:
        cases = (
            ("proposal", "DOCUMENT_KIND_INVALID"),
            ("plan", "DOCUMENT_KIND_PATH_MISMATCH"),
        )
        for kind, expected_code in cases:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                docs_root = self.make_docs_root(Path(temporary))
                relative = "specs/2026-09-03-kind.md"
                self.write_document(
                    docs_root,
                    relative,
                    self.front_matter(kind, "approved" if kind == "plan" else "draft"),
                )

                result = self.run_checker(docs_root)

                self.assertEqual(result.returncode, 1)
                self.assertIn(f"{expected_code} path={relative}", result.stderr.splitlines())

    def test_delivery_document_symlink_cannot_escape_docs_root(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            parent = Path(temporary)
            docs_root = self.make_docs_root(parent)
            outside = parent / "outside.md"
            outside.write_text(
                self.front_matter("spec", "approved"),
                encoding="utf-8",
            )
            relative = "specs/2026-09-03-linked.md"
            linked = docs_root / relative
            linked.parent.mkdir(parents=True)
            try:
                linked.symlink_to(outside)
            except (OSError, NotImplementedError) as error:
                self.skipTest(f"symbolic links are unavailable: {error}")

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr,
                f"DOCUMENT_PATH_INVALID path={relative}\n",
            )

    def test_status_delivery_issue_and_code_repository_are_validated(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            invalid_documents = {
                "plans/2026-09-03-status.md": self.front_matter("plan", "accepted"),
                "rfcs/2026-09-03-issue.md": self.front_matter(
                    "rfc", "accepted", delivery_issue="other#IKD4OR"
                ),
                "specs/2026-09-03-repository.md": self.front_matter(
                    "spec",
                    "approved",
                    code_repository="git:gitee.com/example/ming-iot",
                ),
            }
            for relative, content in invalid_documents.items():
                self.write_document(docs_root, relative, content)

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr.splitlines(),
                [
                    "STATUS_INVALID path=plans/2026-09-03-status.md",
                    "DELIVERY_ISSUE_INVALID path=rfcs/2026-09-03-issue.md",
                    "CODE_REPOSITORY_INVALID path=specs/2026-09-03-repository.md",
                ],
            )

    def test_related_document_paths_must_be_safe_repository_relative_paths(self) -> None:
        for related in (
            "/tmp/private.md",
            "../private.md",
            "specs/../private.md",
            r"specs\private.md",
        ):
            with self.subTest(related=related), tempfile.TemporaryDirectory() as temporary:
                docs_root = self.make_docs_root(Path(temporary))
                relative = "specs/2026-09-03-related.md"
                self.write_document(
                    docs_root,
                    relative,
                    self.front_matter(
                        "spec", "approved", related_documents=(related,)
                    ),
                )

                result = self.run_checker(docs_root)

                self.assertEqual(result.returncode, 1)
                self.assertEqual(
                    result.stderr,
                    f"RELATED_DOCUMENT_INVALID path={relative}\n",
                )

    def test_related_document_symlink_cannot_escape_docs_root(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            parent = Path(temporary)
            docs_root = self.make_docs_root(parent)
            outside = parent / "private.md"
            outside.write_text("private\n", encoding="utf-8")
            try:
                (docs_root / "linked.md").symlink_to(outside)
            except (OSError, NotImplementedError) as error:
                self.skipTest(f"symbolic links are unavailable: {error}")
            relative = "specs/2026-09-03-related.md"
            self.write_document(
                docs_root,
                relative,
                self.front_matter(
                    "spec", "approved", related_documents=("linked.md",)
                ),
            )

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr,
                f"RELATED_DOCUMENT_INVALID path={relative}\n",
            )

    def test_missing_related_document_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            relative = "plans/2026-09-03-related.md"
            self.write_document(
                docs_root,
                relative,
                self.front_matter(
                    "plan", "approved", related_documents=("specs/missing.md",)
                ),
            )

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr,
                f"RELATED_DOCUMENT_NOT_FOUND path={relative}\n",
            )

    def test_review_and_evidence_require_a_lowercase_40_hex_exact_head(self) -> None:
        cases = (
            ("review", "reviews/2026-09-03-review.md", None),
            ("evidence", "evidence/2026-09-03-run/evidence.md", None),
            ("review", "reviews/2026-09-03-uppercase.md", "A" * 40),
            ("evidence", "evidence/2026-09-03-short/evidence.md", "a" * 39),
        )
        for kind, relative, exact_head in cases:
            with self.subTest(kind=kind, exact_head=exact_head), tempfile.TemporaryDirectory() as temporary:
                docs_root = self.make_docs_root(Path(temporary))
                self.write_document(
                    docs_root,
                    relative,
                    self.front_matter(kind, "pass", exact_head=exact_head),
                )

                result = self.run_checker(docs_root)

                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stderr, f"EXACT_HEAD_INVALID path={relative}\n")

    def test_malformed_front_matter_is_reported_instead_of_skipped(self) -> None:
        malformed_documents = (
            "---\ndocument_kind spec\n---\n# Invalid\n",
            "---\ndocument_kind: spec\n",
        )
        for content in malformed_documents:
            with self.subTest(content=content), tempfile.TemporaryDirectory() as temporary:
                docs_root = self.make_docs_root(Path(temporary))
                relative = "specs/2026-09-03-malformed.md"
                self.write_document(docs_root, relative, content)

                result = self.run_checker(docs_root)

                self.assertEqual(result.returncode, 1)
                self.assertEqual(
                    result.stderr,
                    f"FRONT_MATTER_INVALID path={relative}\n",
                )

    def test_all_allowed_statuses_and_a_complete_delivery_packet_pass(self) -> None:
        allowed_statuses = {
            "rfc": ("draft", "accepted", "rejected", "superseded"),
            "spec": ("draft", "approved", "superseded"),
            "plan": ("draft", "approved", "completed", "superseded"),
            "review": ("pass", "pass_with_warnings", "changes_requested", "rejected"),
            "evidence": ("pass", "fail", "partial", "not_run"),
        }
        directories = {
            "rfc": "rfcs",
            "spec": "specs",
            "plan": "plans",
            "review": "reviews",
            "evidence": "evidence",
        }
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            for kind, statuses in allowed_statuses.items():
                for status in statuses:
                    if kind == "evidence":
                        relative = f"evidence/2026-09-03-{status}/evidence.md"
                    else:
                        relative = f"{directories[kind]}/2026-09-03-{status}.md"
                    exact_head = VALID_EXACT_HEAD if kind in {"review", "evidence"} else None
                    self.write_document(
                        docs_root,
                        relative,
                        self.front_matter(kind, status, exact_head=exact_head),
                    )

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, "delivery metadata: OK\n")
            self.assertEqual(result.stderr, "")

    def test_invalid_since_is_reported_as_an_argument_error(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))

            result = self.run_checker(docs_root, since="2026-13-40")

            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, "DELIVERY_METADATA_ARGUMENT_INVALID\n")

    def test_missing_docs_root_is_reported_as_an_argument_error(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = Path(temporary) / "missing"

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, "DELIVERY_METADATA_ARGUMENT_INVALID\n")

    def test_internal_value_error_is_not_reported_as_an_argument_error(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            with mock.patch.object(
                check_delivery_metadata,
                "validate",
                side_effect=ValueError("internal validation error"),
            ), self.assertRaisesRegex(ValueError, "internal validation error"):
                check_delivery_metadata.main(
                    ["--docs-root", str(docs_root), "--since", "2026-09-03"]
                )

    def test_multiple_errors_are_sorted_by_path_then_error_code(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = self.make_docs_root(Path(temporary))
            plan = "plans/2026-09-03-multiple.md"
            spec = "specs/2026-09-03-missing.md"
            self.write_document(
                docs_root,
                plan,
                self.front_matter(
                    "plan",
                    "invalid",
                    delivery_issue="invalid",
                    code_repository="invalid",
                ),
            )
            self.write_document(docs_root, spec, "# Missing metadata\n")

            result = self.run_checker(docs_root)

            self.assertEqual(result.returncode, 1)
            self.assertEqual(
                result.stderr.splitlines(),
                [
                    f"CODE_REPOSITORY_INVALID path={plan}",
                    f"DELIVERY_ISSUE_INVALID path={plan}",
                    f"STATUS_INVALID path={plan}",
                    f"FRONT_MATTER_MISSING path={spec}",
                ],
            )


if __name__ == "__main__":
    unittest.main()
