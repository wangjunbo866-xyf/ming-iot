import json
import os
import subprocess
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from tools import gitee_pr


GITEE_ORIGIN = "https://gitee.com/example/demo.git"
EXACT_HEAD = "0123456789abcdef0123456789abcdef01234567"
MARKER_TOKEN = "private-marker-token-never-print"
REPO_ROOT = Path(__file__).resolve().parents[1]


def run(*args: str, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        cwd=cwd,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=10,
    )


def create_candidate_repository(root: Path) -> tuple[Path, Path]:
    remote = root / "remote.git"
    repo = root / "repo"

    run("git", "init", "--bare", "--initial-branch=master", str(remote))
    run("git", "init", "--initial-branch=master", str(repo))
    run("git", "config", "user.name", "Test User", cwd=repo)
    run("git", "config", "user.email", "test@example.com", cwd=repo)
    (repo / "README.md").write_text("baseline\n", encoding="utf-8")
    run("git", "add", "README.md", cwd=repo)
    run("git", "commit", "-m", "baseline", cwd=repo)
    run("git", "remote", "add", "origin", GITEE_ORIGIN, cwd=repo)
    run("git", "config", f"url.file://{remote}.insteadOf", GITEE_ORIGIN, cwd=repo)
    run("git", "push", "-u", "origin", "master", cwd=repo)
    run("git", "switch", "-c", "feature/gitee-pr-test", cwd=repo)
    (repo / "feature.txt").write_text("feature\n", encoding="utf-8")
    run("git", "add", "feature.txt", cwd=repo)
    run("git", "commit", "-m", "feature", cwd=repo)
    run("git", "push", "-u", "origin", "feature/gitee-pr-test", cwd=repo)
    return repo, remote


class CandidateTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        root = Path(self.temporary_directory.name)
        self.repo, self.remote = create_candidate_repository(root)

    def test_candidate_resolves_gitee_identity_and_exact_remote_head(self) -> None:
        expected_head = run("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()

        candidate = gitee_pr.resolve_candidate(self.repo, "master")

        self.assertEqual(candidate.repo_root, self.repo.resolve())
        self.assertEqual(candidate.owner, "example")
        self.assertEqual(candidate.repository, "demo")
        self.assertEqual(candidate.head_branch, "feature/gitee-pr-test")
        self.assertEqual(candidate.base_branch, "master")
        self.assertEqual(candidate.exact_head, expected_head)

    def test_candidate_rejects_non_git_directory(self) -> None:
        outside = Path(self.temporary_directory.name) / "outside"
        outside.mkdir()

        with self.assertRaisesRegex(gitee_pr.PRToolError, "not a Git repository"):
            gitee_pr.resolve_candidate(outside, "master")

    def test_candidate_rejects_dirty_worktree(self) -> None:
        (self.repo / "dirty.txt").write_text("dirty\n", encoding="utf-8")

        with self.assertRaisesRegex(gitee_pr.PRToolError, "worktree is dirty"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_candidate_rejects_base_branch_as_head(self) -> None:
        run("git", "switch", "master", cwd=self.repo)

        with self.assertRaisesRegex(gitee_pr.PRToolError, "head branch must differ"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_candidate_rejects_master_even_when_base_differs(self) -> None:
        run("git", "switch", "master", cwd=self.repo)

        with self.assertRaisesRegex(gitee_pr.PRToolError, "master cannot be a head branch"):
            gitee_pr.resolve_candidate(self.repo, "feature/gitee-pr-test")

    def test_candidate_rejects_non_gitee_origin(self) -> None:
        run("git", "remote", "set-url", "origin", f"file://{self.remote}", cwd=self.repo)

        with self.assertRaisesRegex(gitee_pr.PRToolError, "origin is not a Gitee repository"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_candidate_rejects_missing_remote_branch(self) -> None:
        run(
            "git",
            "--git-dir",
            str(self.remote),
            "update-ref",
            "-d",
            "refs/heads/feature/gitee-pr-test",
        )

        with self.assertRaisesRegex(gitee_pr.PRToolError, "remote branch does not exist"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_candidate_rejects_missing_remote_base_branch(self) -> None:
        run(
            "git",
            "--git-dir",
            str(self.remote),
            "update-ref",
            "-d",
            "refs/heads/master",
        )

        with self.assertRaisesRegex(gitee_pr.PRToolError, "base branch does not exist"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_candidate_rejects_local_remote_head_mismatch(self) -> None:
        (self.repo / "feature.txt").write_text("new local commit\n", encoding="utf-8")
        run("git", "add", "feature.txt", cwd=self.repo)
        run("git", "commit", "-m", "unpushed", cwd=self.repo)

        with self.assertRaisesRegex(gitee_pr.PRToolError, "local HEAD does not match remote branch"):
            gitee_pr.resolve_candidate(self.repo, "master")

    def test_git_failure_does_not_expose_stderr_details(self) -> None:
        secret_path = Path("/") / MARKER_TOKEN

        with self.assertRaises(gitee_pr.PRToolError) as caught:
            gitee_pr._git(secret_path, "status")

        self.assertEqual(str(caught.exception), "Git command failed")
        self.assertNotIn(MARKER_TOKEN, str(caught.exception))


def candidate() -> gitee_pr.Candidate:
    return gitee_pr.Candidate(
        repo_root=Path("/example/repo"),
        owner="example",
        repository="demo",
        head_branch="feature/gitee-pr-test",
        base_branch="master",
        exact_head=EXACT_HEAD,
    )


def pr_payload(
    number: int = 3,
    *,
    draft: bool = True,
    head: str = "feature/gitee-pr-test",
    head_owner: str = "example",
    base: str = "master",
    head_sha: str = EXACT_HEAD,
    title: str = "Title",
    body: str = "Body",
    url: str | None = None,
) -> dict[str, object]:
    return {
        "number": number,
        "html_url": url or f"https://gitee.com/example/demo/pulls/{number}",
        "draft": draft,
        "state": "open",
        "head": {"ref": head, "label": f"{head_owner}:{head}", "sha": head_sha},
        "base": {"ref": base},
        "title": title,
        "body": body,
    }


class FakeGiteeServer:
    def __init__(self) -> None:
        self.list_payload: object = []
        self.post_payload: object = pr_payload()
        self.patch_payload: object = pr_payload()
        self.read_payload: object = pr_payload()
        self.list_status = 200
        self.redirect_url: str | None = None
        self.requests: list[dict[str, object]] = []

        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, format: str, *args: object) -> None:
                return

            def _record(self, body: bytes = b"") -> None:
                authorization = self.headers.get("Authorization", "")
                owner.requests.append(
                    {
                        "method": self.command,
                        "path": urlparse(self.path).path,
                        "query": parse_qs(urlparse(self.path).query),
                        "fields": parse_qs(body.decode("utf-8")) if body else {},
                        "authorized": authorization == f"Bearer {MARKER_TOKEN}",
                    }
                )

            def _respond(self, status: int, payload: object) -> None:
                raw = json.dumps(payload).encode("utf-8")
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def do_GET(self) -> None:
                self._record()
                if owner.redirect_url is not None:
                    self.send_response(302)
                    self.send_header("Location", owner.redirect_url)
                    self.end_headers()
                    return
                path = urlparse(self.path).path
                if path.endswith("/pulls"):
                    payload = owner.list_payload
                    if owner.list_status != 200:
                        payload = {"message": f"request rejected: {MARKER_TOKEN}"}
                    self._respond(owner.list_status, payload)
                    return
                self._respond(200, owner.read_payload)

            def do_POST(self) -> None:
                body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                self._record(body)
                self._respond(201, owner.post_payload)

            def do_PATCH(self) -> None:
                body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                self._record(body)
                self._respond(200, owner.patch_payload)

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    @property
    def api_base(self) -> str:
        host, port = self.httpd.server_address
        return f"http://{host}:{port}/api/v5"

    def __enter__(self) -> "FakeGiteeServer":
        self.thread.start()
        return self

    def __exit__(self, *args: object) -> None:
        self.httpd.shutdown()
        self.thread.join(timeout=5)
        self.httpd.server_close()


class APITest(unittest.TestCase):
    @staticmethod
    def local_client(server: FakeGiteeServer) -> gitee_pr.GiteeClient:
        return gitee_pr.GiteeClient(
            MARKER_TOKEN,
            api_base=server.api_base,
            allow_local_test_api=True,
        )

    def test_api_rejects_untrusted_token_destination(self) -> None:
        for api_base in (
            "https://example.invalid/api/v5",
            "https://gitee.com:8443/api/v5",
        ):
            with self.subTest(api_base=api_base), self.assertRaisesRegex(
                gitee_pr.PRToolError, "api base URL is not trusted"
            ):
                gitee_pr.GiteeClient(MARKER_TOKEN, api_base=api_base)

    def test_api_rejects_local_token_destination_by_default(self) -> None:
        with self.assertRaisesRegex(gitee_pr.PRToolError, "local test API requires explicit opt-in"):
            gitee_pr.GiteeClient(MARKER_TOKEN, api_base="http://127.0.0.1:1/api/v5")

    def test_api_creates_and_reads_back_draft(self) -> None:
        with FakeGiteeServer() as server:
            client = self.local_client(server)

            created = client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual(created.number, 3)
        self.assertTrue(created.draft)
        self.assertEqual([request["method"] for request in server.requests], ["GET", "POST", "GET"])
        self.assertTrue(all(request["authorized"] for request in server.requests))
        self.assertNotIn("head", server.requests[0]["query"])
        self.assertNotIn("base", server.requests[0]["query"])
        self.assertEqual(server.requests[1]["fields"]["draft"], ["true"])
        self.assertEqual(server.requests[1]["fields"]["head"], ["feature/gitee-pr-test"])
        self.assertEqual(server.requests[1]["fields"]["base"], ["master"])

    def test_api_updates_the_unique_existing_pr(self) -> None:
        with FakeGiteeServer() as server:
            existing = pr_payload(7)
            server.list_payload = [existing]
            server.patch_payload = pr_payload(7, title="New title", body="New body")
            server.read_payload = pr_payload(7, title="New title", body="New body")
            client = self.local_client(server)

            updated = client.ensure_draft(candidate(), title="New title", body="New body")

        self.assertEqual(updated.number, 7)
        self.assertEqual([request["method"] for request in server.requests], ["GET", "PATCH", "GET"])
        self.assertEqual(server.requests[1]["path"], "/api/v5/repos/example/demo/pulls/7")
        self.assertEqual(server.requests[1]["fields"]["title"], ["New title"])
        self.assertEqual(server.requests[1]["fields"]["body"], ["New body"])

    def test_api_does_not_match_another_owners_same_named_branch(self) -> None:
        with FakeGiteeServer() as server:
            other_fork = pr_payload(7)
            head = other_fork["head"]
            self.assertIsInstance(head, dict)
            head.pop("label")
            head["repo"] = {"full_name": "another-owner/demo"}
            server.list_payload = [other_fork]
            client = self.local_client(server)

            created = client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual(created.number, 3)
        self.assertEqual(
            [request["method"] for request in server.requests], ["GET", "POST", "GET"]
        )

    def test_api_matches_bare_label_when_repo_proves_same_owner(self) -> None:
        with FakeGiteeServer() as server:
            existing = pr_payload(7)
            head = existing["head"]
            self.assertIsInstance(head, dict)
            head["label"] = "feature/gitee-pr-test"
            head["repo"] = {"full_name": "example/demo"}
            server.list_payload = [existing]
            server.patch_payload = pr_payload(7)
            server.read_payload = pr_payload(7)
            client = self.local_client(server)

            updated = client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual(updated.number, 7)
        self.assertEqual(
            [request["method"] for request in server.requests], ["GET", "PATCH", "GET"]
        )

    def test_api_does_not_match_same_owners_other_repository(self) -> None:
        with FakeGiteeServer() as server:
            other_repository = pr_payload(7)
            head = other_repository["head"]
            self.assertIsInstance(head, dict)
            head["label"] = "feature/gitee-pr-test"
            head["repo"] = {"full_name": "example/other-repository"}
            server.list_payload = [other_repository]
            client = self.local_client(server)

            created = client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual(created.number, 3)
        self.assertEqual(
            [request["method"] for request in server.requests], ["GET", "POST", "GET"]
        )

    def test_api_rejects_multiple_matching_open_prs(self) -> None:
        with FakeGiteeServer() as server:
            server.list_payload = [pr_payload(3), pr_payload(4)]
            client = self.local_client(server)

            with self.assertRaisesRegex(gitee_pr.PRToolError, "multiple open pull requests"):
                client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual([request["method"] for request in server.requests], ["GET"])

    def test_api_error_does_not_expose_token_or_response_body(self) -> None:
        with FakeGiteeServer() as server:
            server.list_status = 401
            client = self.local_client(server)

            with self.assertRaises(gitee_pr.PRToolError) as caught:
                client.ensure_draft(candidate(), title="Title", body="Body")

        message = str(caught.exception)
        self.assertIn("HTTP 401", message)
        self.assertNotIn(MARKER_TOKEN, message)
        self.assertNotIn("request rejected", message)

    def test_api_rejects_token_in_title_or_body_before_network_access(self) -> None:
        for title, body in (
            (f"Title {MARKER_TOKEN}", "Body"),
            ("Title", f"Body {MARKER_TOKEN}"),
        ):
            with self.subTest(title=title, body=body), FakeGiteeServer() as server:
                client = self.local_client(server)

                with self.assertRaisesRegex(
                    gitee_pr.PRToolError, "pull request content contains the API token"
                ):
                    client.ensure_draft(candidate(), title=title, body=body)

            self.assertEqual(server.requests, [])

    def test_api_refuses_redirect_without_forwarding_authorization(self) -> None:
        with FakeGiteeServer() as destination, FakeGiteeServer() as source:
            source.redirect_url = (
                f"{destination.api_base}/repos/example/demo/pulls"
            )
            client = self.local_client(source)

            with self.assertRaisesRegex(
                gitee_pr.PRToolError, "Gitee API request failed with HTTP 302"
            ):
                client.ensure_draft(candidate(), title="Title", body="Body")

        self.assertEqual(destination.requests, [])

    def test_api_rejects_read_back_with_wrong_draft_branch_or_head(self) -> None:
        invalid_payloads = (
            pr_payload(draft=False),
            pr_payload(head="another-branch"),
            pr_payload(base="another-base"),
            pr_payload(head_sha="f" * 40),
            pr_payload(title="Wrong title"),
            pr_payload(body="Wrong body"),
            pr_payload(url="https://example.invalid/example/demo/pulls/3"),
        )
        for payload in invalid_payloads:
            with self.subTest(payload=payload), FakeGiteeServer() as server:
                server.read_payload = payload
                client = self.local_client(server)
                with self.assertRaisesRegex(gitee_pr.PRToolError, "read-back validation failed"):
                    client.ensure_draft(candidate(), title="Title", body="Body")

    def test_api_verifies_remote_ref_when_read_back_omits_head_sha(self) -> None:
        payload = pr_payload()
        head = payload["head"]
        self.assertIsInstance(head, dict)
        head.pop("sha")

        with tempfile.TemporaryDirectory() as temporary_directory:
            repo, _ = create_candidate_repository(Path(temporary_directory))
            verified_candidate = gitee_pr.resolve_candidate(repo, "master")
            with FakeGiteeServer() as server:
                server.read_payload = payload
                client = self.local_client(server)

                created = client.ensure_draft(
                    verified_candidate, title="Title", body="Body"
                )

        self.assertEqual(created.number, 3)


class CLITest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.root = Path(self.temporary_directory.name)
        self.repo, _ = create_candidate_repository(self.root)
        self.exact_head = run("git", "rev-parse", "HEAD", cwd=self.repo).stdout.strip()
        self.title_file = self.root / "title.txt"
        self.body_file = self.root / "body.md"
        self.title_file.write_text("Draft title\n", encoding="utf-8")
        self.body_file.write_text("Sensitive-looking PR body\n", encoding="utf-8")

    def command(self, api_base: str) -> list[str]:
        return [
            "python3",
            str(REPO_ROOT / "tools" / "gitee_pr.py"),
            "--repo-root",
            str(self.repo),
            "--base",
            "master",
            "--title-file",
            str(self.title_file),
            "--body-file",
            str(self.body_file),
            "--draft",
            "--api-base",
            api_base,
        ]

    def test_cli_outputs_only_safe_machine_readable_receipt(self) -> None:
        environment = dict(os.environ)
        environment.pop("GITEE_ACCESS_TOKEN", None)
        environment["GITEE_PR_TEST_TOKEN"] = MARKER_TOKEN
        with FakeGiteeServer() as server:
            server.post_payload = pr_payload(
                head_sha=self.exact_head,
                title="Draft title",
                body="Sensitive-looking PR body",
            )
            server.read_payload = pr_payload(
                head_sha=self.exact_head,
                title="Draft title",
                body="Sensitive-looking PR body",
            )
            result = subprocess.run(
                self.command(server.api_base),
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                env=environment,
                timeout=20,
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        receipt = json.loads(result.stdout)
        self.assertEqual(
            receipt,
            {
                "draft": True,
                "exact_head": self.exact_head,
                "number": 3,
                "url": "https://gitee.com/example/demo/pulls/3",
            },
        )
        combined_output = result.stdout + result.stderr
        self.assertNotIn(MARKER_TOKEN, combined_output)
        self.assertNotIn("Sensitive-looking PR body", combined_output)
        self.assertNotIn("Draft title", combined_output)

    def test_cli_rejects_missing_token_before_network_access(self) -> None:
        environment = dict(os.environ)
        environment.pop("GITEE_ACCESS_TOKEN", None)
        environment.pop("GITEE_PR_TEST_TOKEN", None)
        result = subprocess.run(
            self.command("http://127.0.0.1:1/api/v5"),
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=environment,
            timeout=20,
        )

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertIn("GITEE_PR_TEST_TOKEN is required for local tests", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_cli_never_sends_gitee_access_token_to_local_test_api(self) -> None:
        environment = dict(os.environ)
        environment.pop("GITEE_PR_TEST_TOKEN", None)
        environment["GITEE_ACCESS_TOKEN"] = MARKER_TOKEN
        with FakeGiteeServer() as server:
            result = subprocess.run(
                self.command(server.api_base),
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                env=environment,
                timeout=20,
            )

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(server.requests, [])
        self.assertNotIn(MARKER_TOKEN, result.stdout + result.stderr)

    def test_cli_wraps_malformed_api_base_without_traceback(self) -> None:
        environment = dict(os.environ)
        environment["GITEE_ACCESS_TOKEN"] = MARKER_TOKEN
        for api_base in ("http://[", "https://gitee.com:notaport/api/v5"):
            with self.subTest(api_base=api_base):
                result = subprocess.run(
                    self.command(api_base),
                    check=False,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    env=environment,
                    timeout=20,
                )

                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")
                self.assertIn("api base URL is invalid", result.stderr)
                self.assertNotIn("Traceback", result.stderr)


if __name__ == "__main__":
    unittest.main()
