import json
import os
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from tools import gitee_issue


REPO_ROOT = Path(__file__).resolve().parents[1]
SCRIPT = REPO_ROOT / "tools" / "gitee_issue.py"
DUMMY_TOKEN = "dummy-issue-token-for-loopback-tests-only"
OWNER = "ming-industry"
REPO = "ming-iot-rd-docs"
NUMBER = "IKEOFY"
DELIVERY_ID = f"{REPO}#{NUMBER}"
ISSUE_URL = f"https://gitee.com/{OWNER}/{REPO}/issues/{NUMBER}"
HUB_URL = (
    f"https://gitee.com/{OWNER}/{REPO}/blob/master/"
    "map/00_研发治理/01_文档管理/changes/"
    "FEATURE-ming-iot-rd-docs-IKEOFY_业务地图文档管理/README.md"
)
START = "<!-- map-delivery:start -->"
END = "<!-- map-delivery:end -->"
BLOCK = f"{START}\nDelivery ID: `{DELIVERY_ID}`\nChange Hub: <{HUB_URL}>\n{END}"
HUMAN_BODY = "## 背景\r\n保留人工内容与空格。  \r\n\r\n"


def issue_payload(body=HUMAN_BODY, **overrides):
    return {
        "number": NUMBER,
        "repository": {"full_name": f"{OWNER}/{REPO}"},
        "html_url": ISSUE_URL,
        "title": "研发治理（人工标题）",
        "body": body,
        "state": "open",
        **overrides,
    }


class FakeIssueServer:
    """Independent loopback fixture; records only dummy-auth equality, not headers."""

    def __init__(self):
        self.payload = issue_payload()
        self.readback_overrides = {}
        self.requests = []
        self.fail_at = None
        self.fail_status = 401
        self.redirect_at = None
        self.redirect_status = 302
        self.redirect_url = None
        self.raw_at = None
        self.raw = b""
        self.after_first_get = None
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, format, *args):
                pass

            def handle_request(self):
                raw_body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                parsed = urlparse(self.path)
                fields = parse_qs(raw_body.decode("utf-8"), keep_blank_values=True)
                fixture.requests.append({
                    "method": self.command,
                    "path": parsed.path,
                    "query": parsed.query,
                    "fields": fields,
                    "authorized": self.headers.get("Authorization") == f"Bearer {DUMMY_TOKEN}",
                })
                index = len(fixture.requests)
                if index == fixture.redirect_at:
                    self.send_response(fixture.redirect_status)
                    self.send_header("Location", fixture.redirect_url)
                    self.end_headers()
                    return
                status = 200
                if index == fixture.fail_at:
                    status = fixture.fail_status
                    payload = {"message": f"{DUMMY_TOKEN}: {HUMAN_BODY}"}
                elif self.command == "PATCH":
                    fixture.payload.update({key: fields[key][0] for key in ("title", "body") if key in fields})
                    payload = fixture.payload
                else:
                    payload = fixture.payload
                    if index > 1:
                        payload = {**payload, **fixture.readback_overrides}
                raw = fixture.raw if index == fixture.raw_at else json.dumps(payload).encode("utf-8")
                # Snapshot GET response first, then save the concurrent edit
                # before the client can start its next request.
                if index == 1 and self.command == "GET" and fixture.after_first_get:
                    fixture.payload.update(fixture.after_first_get)
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            do_GET = handle_request
            do_PATCH = handle_request
            do_POST = handle_request
            do_DELETE = handle_request

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(
            target=lambda: self.server.serve_forever(poll_interval=0.01), daemon=True
        )

    @property
    def api_base(self):
        return f"http://127.0.0.1:{self.server.server_port}/api/v5"

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *args):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


class GiteeIssueTest(unittest.TestCase):
    def setUp(self):
        self.module = gitee_issue

    def client(self, server):
        return self.module.GiteeClient(
            DUMMY_TOKEN, api_base=server.api_base, allow_local_test_api=True
        )

    def link(self, server, **overrides):
        return self.client(server).link(**{
            "owner": OWNER, "repo": REPO, "number": NUMBER,
            "delivery_id": DELIVERY_ID, "hub_url": HUB_URL, **overrides,
        })

    def cli(self, server, *, simulate_production_token=False, extra_args=(), token=True):
        environment = dict(os.environ)
        # Never inherit a real credential, even for the rejection test.
        environment.pop("GITEE_ACCESS_TOKEN", None)
        environment.pop("GITEE_ISSUE_TEST_TOKEN", None)
        if token:
            environment["GITEE_ISSUE_TEST_TOKEN"] = DUMMY_TOKEN
        command = [sys.executable, str(SCRIPT)]
        if simulate_production_token:
            # Simulate the production variable only inside the child, from our dummy env.
            command = [sys.executable, "-c", (
                "import os, runpy, sys; "
                "os.environ['GITEE_ACCESS_TOKEN'] = os.environ['GITEE_ISSUE_TEST_TOKEN']; "
                "sys.argv = sys.argv[1:]; runpy.run_path(sys.argv[0], run_name='__main__')"
            ), str(SCRIPT)]
        return subprocess.run(command + [
            "link", "--owner", OWNER, "--repo", REPO, "--number", NUMBER,
            "--delivery-id", DELIVERY_ID, "--hub-url", HUB_URL,
            "--api-base", server.api_base, *extra_args,
        ], env=environment, capture_output=True, text=True, check=False, timeout=10)

    def test_link_missing_block_prepares_manual_update_without_patch(self):
        with FakeIssueServer() as server:
            receipt = self.link(server)
        self.assertEqual(receipt, {"number": NUMBER, "url": ISSUE_URL, "linked": False,
                                  "manual_update_required": True, "managed_block": BLOCK})
        self.assertEqual([r["method"] for r in server.requests], ["GET"])
        self.assertEqual(server.payload["body"], HUMAN_BODY)
        self.assertTrue(all(r["authorized"] and not r["query"] for r in server.requests))

    def test_product_issue_can_confirm_docs_repository_hub(self):
        delivery = "ming-iot#IPRODUCT"
        url = f"https://gitee.com/{OWNER}/ming-iot/issues/IPRODUCT"
        hub = HUB_URL.replace("ming-iot-rd-docs-IKEOFY", "ming-iot-IPRODUCT")
        block = f"{START}\nDelivery ID: `{delivery}`\nChange Hub: <{hub}>\n{END}"
        with FakeIssueServer() as server:
            server.payload.update(number="IPRODUCT", repository={"full_name": f"{OWNER}/ming-iot"},
                                  html_url=url, body=HUMAN_BODY + block)
            result = self.cli(server, extra_args=("--repo", "ming-iot", "--number", "IPRODUCT",
                                                 "--delivery-id", delivery, "--hub-url", hub))
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertTrue(json.loads(result.stdout)["linked"])
        self.assertEqual(["GET", "GET"], [r["method"] for r in server.requests])
        self.assertTrue(all(r["path"] == f"/api/v5/repos/{OWNER}/ming-iot/issues/IPRODUCT"
                            for r in server.requests))

    def test_hub_must_belong_to_allowed_docs_map(self):
        for hub in (HUB_URL.replace(OWNER, "other"), HUB_URL.replace(REPO, "other-repo"),
                    HUB_URL.replace("/map/", "/unrelated/"),
                    HUB_URL.replace("README.md", "spec.md")):
            with self.subTest(hub=hub), FakeIssueServer() as server:
                result = self.cli(server, extra_args=("--hub-url", hub))
                self.assertEqual(1, result.returncode, result.stdout)
                self.assertEqual([], server.requests)

    def test_link_updates_existing_managed_block_without_duplication(self):
        prefix, suffix = "人工前文  \r\n\n", "\r\n人工后文  \n\n"
        old_block = BLOCK.replace(HUB_URL, HUB_URL.replace("master", "old-branch"))
        with FakeIssueServer() as server:
            server.payload["body"] = prefix + old_block + suffix
            receipt = self.link(server)
            self.assertFalse(receipt["linked"])
            self.assertEqual(server.payload["body"], prefix + old_block + suffix)
            # Human applies the displayed block; subsequent confirmations never write.
            server.payload["body"] = prefix + receipt["managed_block"] + suffix
            self.assertTrue(self.link(server)["linked"])
            self.assertTrue(self.link(server)["linked"])
        self.assertEqual(server.payload["body"], prefix + BLOCK + suffix)
        self.assertTrue(all(r["method"] == "GET" for r in server.requests))

    def test_link_handles_empty_body_without_changing_human_whitespace(self):
        for body, expected in (("", BLOCK + "\n"), (None, BLOCK + "\n"),
                               ("text  ", "text  \n\n" + BLOCK + "\n")):
            with self.subTest(body=body), FakeIssueServer() as server:
                server.payload["body"] = body
                receipt = self.link(server)
                self.assertFalse(receipt["linked"])
                self.assertEqual(receipt["managed_block"], BLOCK)
                self.assertEqual(server.payload["body"], body)

    def test_link_never_overwrites_concurrent_human_edit(self):
        with FakeIssueServer() as server:
            server.after_first_get = {"title": "人工新标题", "body": HUMAN_BODY + "新增验收条件"}
            receipt = self.link(server)
        self.assertFalse(receipt["linked"])
        self.assertEqual(server.payload["title"], "人工新标题")
        self.assertIn("新增验收条件", server.payload["body"])
        self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_link_accepts_concurrent_human_append_when_block_is_unchanged(self):
        with FakeIssueServer() as server:
            server.payload["body"] = HUMAN_BODY + BLOCK
            server.after_first_get = {"title": "人工新标题", "body": HUMAN_BODY + BLOCK + "\n新增段落"}
            self.assertTrue(self.link(server)["linked"])
        self.assertEqual([r["method"] for r in server.requests], ["GET", "GET"])
        self.assertIn("新增段落", server.payload["body"])

    def test_link_rejects_multiple_managed_blocks(self):
        with FakeIssueServer() as server:
            server.payload["body"] = BLOCK + "\n" + BLOCK
            with self.assertRaisesRegex(self.module.IssueToolError, "managed block"):
                self.link(server)
        self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_link_rejects_malformed_marker_blocks_before_write(self):
        bodies = [START, END, END + "\n" + START, START + "\n" + BLOCK + "\n" + END,
                  "prefix " + BLOCK, BLOCK + " suffix", "<!-- map-delivery:start-->\n" + END,
                  "<!-- map-delivery:START -->", "<!-- map-delivery:unknown -->",
                  "<!-- map-delivery:start", BLOCK + "\n<!-- map-delivery:end-->"]
        for body in bodies:
            with self.subTest(body=body), FakeIssueServer() as server:
                server.payload["body"] = body
                with self.assertRaisesRegex(self.module.IssueToolError, "managed block"):
                    self.link(server)
                self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_link_rejects_issue_repository_or_number_mismatch(self):
        overrides = [
            {"number": "OTHER"}, {"number": True}, {"number": None},
            {"repository": {"full_name": f"other/{REPO}"}},
            {"repository": {"full_name": f"{OWNER}/other"}}, {"repository": None},
            {"repository": {}}, {"html_url": "https://example.invalid/leak"},
            {"html_url": ISSUE_URL + "?secret=value"}, {"html_url": ISSUE_URL + "/../OTHER"},
            {"html_url": ISSUE_URL.replace("https://", "https://user:pass@")},
            {"body": 5}, {"title": None},
        ]
        for override in overrides:
            with self.subTest(override=override), FakeIssueServer() as server:
                server.payload.update(override)
                with self.assertRaises(self.module.IssueToolError):
                    self.link(server)
                self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_link_rejects_mismatching_readback(self):
        for override in ({"body": HUMAN_BODY}, {"number": "OTHER"},
                         {"repository": {"full_name": "other/repo"}},
                         {"html_url": "https://example.invalid/"}):
            with self.subTest(override=override), FakeIssueServer() as server:
                server.readback_overrides = override
                server.payload["body"] = HUMAN_BODY + BLOCK
                with self.assertRaisesRegex(self.module.IssueToolError, "read-back"):
                    self.link(server)
                self.assertEqual([r["method"] for r in server.requests], ["GET", "GET"])

    def test_link_rejects_redirect_without_forwarding_authorization(self):
        for status in (301, 302, 303, 307, 308):
            for step in (1, 2):
                with self.subTest(status=status, step=step), FakeIssueServer() as target, FakeIssueServer() as source:
                    source.redirect_at = step
                    source.payload["body"] = HUMAN_BODY + BLOCK
                    source.redirect_status = status
                    source.redirect_url = target.api_base + "/capture"
                    with self.assertRaisesRegex(self.module.IssueToolError, f"HTTP {status}"):
                        self.link(source)
                    self.assertEqual(target.requests, [])
                    self.assertEqual(len(source.requests), step)

    def test_link_error_never_prints_token_or_issue_body(self):
        for step in (1, 2):
            with self.subTest(step=step), FakeIssueServer() as server:
                server.fail_at = step
                server.payload["body"] = HUMAN_BODY + BLOCK
                result = self.cli(server)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertIn("HTTP 401", result.stderr)
                self.assertNotIn(DUMMY_TOKEN, result.stderr)
                self.assertNotIn("保留人工内容", result.stderr)
                self.assertNotIn("Traceback", result.stderr)

    def test_link_rejects_invalid_json_and_oversized_responses_safely(self):
        for raw in (b"not-json", b"\xff", b"x" * 1_000_001, b"[]"):
            with self.subTest(size=len(raw)), FakeIssueServer() as server:
                server.raw_at, server.raw = 1, raw
                with self.assertRaises(self.module.IssueToolError):
                    self.link(server)
                self.assertEqual(len(server.requests), 1)

    def test_cli_rejects_unencodable_response_content_safely(self):
        for field in ("title", "body"):
            with self.subTest(field=field), FakeIssueServer() as server:
                server.payload[field] = "\ud800"
                result = self.cli(server)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertNotIn("Traceback", result.stderr)
                self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_cli_rejects_excessive_json_integer_safely(self):
        with FakeIssueServer() as server:
            server.raw_at, server.raw = 1, b'{"id":' + b"9" * 5000 + b"}"
            result = self.cli(server)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertNotIn("Traceback", result.stderr)
        self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_cli_rejects_invalid_api_base_without_echoing_it(self):
        for base in ("http://[", "https://gitee.com:bad/api/v5", "https://example.invalid/" + DUMMY_TOKEN):
            with self.subTest(), FakeIssueServer() as server:
                result = self.cli(server, extra_args=("--api-base", base))
                self.assertEqual(result.returncode, 1)
                self.assertNotIn("Traceback", result.stderr)
                self.assertNotIn(DUMMY_TOKEN, result.stdout + result.stderr)
                self.assertEqual(server.requests, [])

    def test_link_rejects_unsafe_arguments_before_network(self):
        overrides = [
            {"owner": ".."}, {"repo": "a/b"}, {"number": "../OTHER"},
            {"delivery_id": "placeholder"}, {"delivery_id": f"{REPO}#OTHER"},
            {"hub_url": "https://example.invalid/hub"},
            {"hub_url": HUB_URL + "\n" + START}, {"hub_url": HUB_URL + ">%0aevil"},
            {"hub_url": HUB_URL.replace("https://", "http://")},
            {"hub_url": HUB_URL.replace(OWNER, "other")},
            {"hub_url": HUB_URL.replace("gitee.com", "gitee.com:444")},
            {"hub_url": HUB_URL + "?access_token=" + DUMMY_TOKEN},
        ]
        for override in overrides:
            with self.subTest(keys=list(override)), FakeIssueServer() as server:
                with self.assertRaises(self.module.IssueToolError):
                    self.link(server, **override)
                self.assertEqual(server.requests, [])

    def test_link_refuses_to_write_token_already_in_issue_content(self):
        for field in ("title", "body"):
            with self.subTest(field=field), FakeIssueServer() as server:
                server.payload[field] = DUMMY_TOKEN
                result = self.cli(server)
                self.assertEqual(result.returncode, 1)
                self.assertNotIn(DUMMY_TOKEN, result.stdout + result.stderr)
                self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_client_restricts_api_origin_and_requires_local_opt_in(self):
        for base in ("http://gitee.com/api/v5", "https://example.invalid/api/v5",
                     "https://gitee.com.evil.invalid/api/v5", "https://gitee.com:444/api/v5",
                     "https://user:pass@gitee.com/api/v5", "https://gitee.com/api/v5?x=y",
                     "https://gitee.com/other", "http://[", "https://gitee.com:bad/api/v5"):
            with self.subTest(base=base), self.assertRaises(self.module.IssueToolError):
                self.module.GiteeClient(DUMMY_TOKEN, api_base=base)
        with FakeIssueServer() as server:
            with self.assertRaises(self.module.IssueToolError):
                self.module.GiteeClient(DUMMY_TOKEN, api_base=server.api_base)
            self.assertEqual(server.requests, [])

    def test_cli_outputs_only_verified_receipt(self):
        with FakeIssueServer() as server:
            server.payload["body"] = HUMAN_BODY + BLOCK
            result = self.cli(server)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"number": NUMBER, "url": ISSUE_URL, "linked": True})
        self.assertEqual(result.stderr, "")
        self.assertNotIn(DUMMY_TOKEN, result.stdout)
        self.assertNotIn("保留人工内容", result.stdout)

    def test_cli_manual_update_is_non_success_and_does_not_echo_human_body(self):
        with FakeIssueServer() as server:
            result = self.cli(server)
        self.assertEqual(result.returncode, 2)
        self.assertFalse(json.loads(result.stdout)["linked"])
        self.assertNotIn("保留人工内容", result.stdout)
        self.assertEqual([r["method"] for r in server.requests], ["GET"])

    def test_cli_rejects_real_token_for_local_test_server(self):
        # Only a synthetic token stands in for the production variable.
        with FakeIssueServer() as server:
            result = self.cli(server, simulate_production_token=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("GITEE_ACCESS_TOKEN may only be sent to gitee.com", result.stderr)
        self.assertEqual(server.requests, [])
        self.assertNotIn(DUMMY_TOKEN, result.stdout + result.stderr)

    def test_cli_requires_dedicated_test_token(self):
        with FakeIssueServer() as server:
            result = self.cli(server, token=False)
        self.assertEqual(result.returncode, 1)
        self.assertIn("GITEE_ISSUE_TEST_TOKEN is required for local tests", result.stderr)
        self.assertEqual(server.requests, [])

    def test_cli_readback_failure_is_public_safe(self):
        with FakeIssueServer() as server:
            server.payload["body"] = HUMAN_BODY + BLOCK
            server.readback_overrides = {"html_url": "https://example.invalid/" + DUMMY_TOKEN}
            result = self.cli(server)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("read-back", result.stderr)
        self.assertNotIn(DUMMY_TOKEN, result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_cli_invalid_arguments_do_not_echo_values(self):
        with FakeIssueServer() as server:
            result = self.cli(server, extra_args=("--unexpected", DUMMY_TOKEN))
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(DUMMY_TOKEN, result.stdout + result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        self.assertEqual(server.requests, [])

    def test_issue_template_has_five_sections_and_explicit_link_gate(self):
        template = (REPO_ROOT / ".gitee" / "ISSUE_TEMPLATE" / "研发治理.md").read_text(encoding="utf-8")
        self.assertEqual([line for line in template.splitlines() if line.startswith("## ")],
                         ["## 背景", "## 目标", "## 非目标", "## 验收条件", "## Delivery 导航"])
        self.assertIn("真实 Delivery ID", template)
        self.assertIn("自然语言结构本身不是门禁", template)
        self.assertIn("gitee_issue.py link", template)
        self.assertIn("回读确认", template)


if __name__ == "__main__":
    unittest.main()
