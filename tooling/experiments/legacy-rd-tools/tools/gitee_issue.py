#!/usr/bin/env python3
"""Prepare/verify one existing Gitee Issue's map navigation without body writes.

Production credentials come only from GITEE_ACCESS_TOKEN. Loopback CLI fixtures
must instead set GITEE_ISSUE_TEST_TOKEN and unset GITEE_ACCESS_TOKEN. No Issue
creation, deletion, body/title update, or state transition is supported. Without
a verified atomic conditional-write API, whole-body PATCH is unsafe for teams.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from http.client import HTTPException
from urllib.error import HTTPError, URLError
from urllib.parse import unquote, urlencode, urlparse
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener


_API_BASE = "https://gitee.com/api/v5"
_HTTP_BODY_LIMIT = 1_000_000
_LOCAL_HOSTS = {"127.0.0.1", "::1", "localhost"}
_LOCAL_TEST_TOKEN_ENV = "GITEE_ISSUE_TEST_TOKEN"
_IDENTIFIER = re.compile(r"[A-Za-z0-9_-][A-Za-z0-9_.-]*")
_NUMBER = re.compile(r"[A-Z0-9]+")
_START = "<!-- map-delivery:start -->"
_END = "<!-- map-delivery:end -->"
_MARKER_PREFIX = re.compile(r"<!--\s*map-delivery", re.IGNORECASE)


class IssueToolError(RuntimeError):
    """A stable public-safe error; never include input or response content."""


class _RejectRedirectHandler(HTTPRedirectHandler):
    """Mirror gitee_pr's credential boundary for every HTTP method."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def _parse_url(url: str):
    if not isinstance(url, str) or any(c.isspace() or ord(c) < 32 or ord(c) == 127 for c in url):
        raise IssueToolError("URL is invalid")
    try:
        parsed = urlparse(url)
        port = parsed.port
    except ValueError:
        raise IssueToolError("URL is invalid") from None
    if (not parsed.netloc or parsed.username is not None or parsed.password is not None
            or parsed.params or parsed.query or parsed.fragment or "\\" in url):
        raise IssueToolError("URL is invalid")
    return parsed, port


def _api_is_local(api_base: str) -> bool:
    parsed, port = _parse_url(api_base)
    public = parsed.scheme == "https" and parsed.hostname == "gitee.com" and port in {None, 443}
    local = parsed.scheme == "http" and parsed.hostname in _LOCAL_HOSTS
    if not (public or local) or parsed.path.rstrip("/") != "/api/v5":
        raise IssueToolError("api base URL is not trusted")
    return local


def _validate_link(owner: str, repo: str, number: str, delivery_id: str, hub_url: str) -> None:
    if (not _IDENTIFIER.fullmatch(owner) or not _IDENTIFIER.fullmatch(repo)
            or not _NUMBER.fullmatch(number)):
        raise IssueToolError("Issue identity is invalid")
    if delivery_id != f"{repo}#{number}":
        raise IssueToolError("Delivery ID must match the Issue")
    parsed, port = _parse_url(hub_url)
    decoded = unquote(hub_url)
    # The product Issue remains the Delivery identity; its documentation lives
    # in the paired private repository. This does not allow arbitrary cross-repo URLs.
    hub_repos = {repo}
    if owner == "ming-industry" and repo == "ming-iot":
        hub_repos.add("ming-iot-rd-docs")
    prefixes = [f"/{owner}/{hub_repo}/blob/" for hub_repo in hub_repos]
    prefix = next((value for value in prefixes if parsed.path.startswith(value)), None)
    parts = unquote(parsed.path[len(prefix):]).split("/") if prefix else []
    if (parsed.scheme != "https" or parsed.hostname != "gitee.com" or port not in {None, 443}
            or len(parts) < 7 or parts[-6] != "map" or parts[-3] != "changes"
            or parts[-1] != "README.md"
            or re.search(r"%2f", parsed.path, re.I)
            or any(c.isspace() or ord(c) < 32 or ord(c) == 127 or c in "<>`\\" for c in decoded)
            or any(part in {"", ".", ".."} for part in parts)):
        raise IssueToolError("Change Hub URL is invalid")


def _managed_body(body: str, delivery_id: str, hub_url: str) -> str:
    block = f"{_START}\nDelivery ID: `{delivery_id}`\nChange Hub: <{hub_url}>\n{_END}"
    marker_count = len(_MARKER_PREFIX.findall(body))
    if marker_count == 0:
        separator = ""
        if body and not body.endswith(("\n\n", "\r\n\r\n")):
            separator = "\n" if body.endswith("\n") else "\n\n"
        return body + separator + block + "\n"
    if marker_count != 2 or body.count(_START) != 1 or body.count(_END) != 1:
        raise IssueToolError("Issue managed block is duplicate or malformed")
    start, end = body.index(_START), body.index(_END)
    if start >= end:
        raise IssueToolError("Issue managed block is duplicate or malformed")
    for offset, marker in ((start, _START), (end, _END)):
        after = body[offset + len(marker):]
        if ((offset and body[offset - 1] != "\n")
                or (after and not after.startswith(("\n", "\r\n")))):
            raise IssueToolError("Issue managed block is duplicate or malformed")
    # Do not normalize, trim, or reformat any human text outside the markers.
    return body[:start] + block + body[end + len(_END):]


def _validated_issue(payload: object, owner: str, repo: str, number: str) -> dict:
    if not isinstance(payload, dict):
        raise IssueToolError("Gitee API returned an invalid Issue")
    repository = payload.get("repository")
    url = f"https://gitee.com/{owner}/{repo}/issues/{number}"
    if (payload.get("number") != number
            or not isinstance(repository, dict)
            or repository.get("full_name") != f"{owner}/{repo}"
            or payload.get("html_url") != url
            or not isinstance(payload.get("title"), str)
            or (payload.get("body") is not None and not isinstance(payload.get("body"), str))
            or "body" not in payload):
        raise IssueToolError("Issue identity or content validation failed")
    try:
        payload["title"].encode("utf-8")
        (payload["body"] or "").encode("utf-8")
    except UnicodeError:
        raise IssueToolError("Issue content encoding is invalid") from None
    return {**payload, "body": payload["body"] or ""}


class GiteeClient:
    """Small independent Issue client using gitee_pr's fail-closed patterns."""

    def __init__(self, token: str, api_base: str = _API_BASE, *, allow_local_test_api: bool = False):
        local = _api_is_local(api_base)
        if local and not allow_local_test_api:
            raise IssueToolError("local test API requires explicit opt-in")
        if not token.strip():
            raise IssueToolError("API token is required")
        self._token = token.strip()
        if any(not 33 <= ord(c) <= 126 for c in self._token):
            raise IssueToolError("API token is invalid")
        self._api_base = api_base.rstrip("/")
        # Avoid ambient proxy settings routing local test credentials off-loopback.
        self._opener = build_opener(ProxyHandler({}), _RejectRedirectHandler())

    def _request_json(self, method: str, path: str, *, fields: dict[str, str] | None = None) -> object:
        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {self._token}",
            "User-Agent": "ming-iot-rd-docs-gitee-issue/1",
        }
        data = None
        try:
            if fields is not None:
                data = urlencode(fields).encode("utf-8")
                headers["Content-Type"] = "application/x-www-form-urlencoded"
            request = Request(self._api_base + path, data=data, headers=headers, method=method)
            with self._opener.open(request, timeout=20) as response:
                raw = response.read(_HTTP_BODY_LIMIT + 1)
        except HTTPError as exc:
            code = exc.code
            exc.close()
            raise IssueToolError(f"Gitee API request failed with HTTP {code}") from None
        except (URLError, OSError, HTTPException, ValueError):
            raise IssueToolError("Gitee API request could not be completed") from None
        if len(raw) > _HTTP_BODY_LIMIT:
            raise IssueToolError("Gitee API response was too large")
        try:
            return json.loads(raw.decode("utf-8"))
        except (ValueError, RecursionError):
            raise IssueToolError("Gitee API returned invalid JSON") from None

    def link(self, *, owner: str, repo: str, number: str, delivery_id: str, hub_url: str) -> dict:
        """Confirm a saved block, or return a manual-edit block with linked=false.

        No automatic full-body write, retries, or title update is permitted.
        The receipt proves only what was observed, not a lock against later edits.
        """
        if any(self._token in value for value in (owner, repo, number, delivery_id, hub_url)):
            raise IssueToolError("Issue link input contains the API token")
        _validate_link(owner, repo, number, delivery_id, hub_url)
        read_path = f"/repos/{owner}/{repo}/issues/{number}"
        current = _validated_issue(self._request_json("GET", read_path), owner, repo, number)
        if self._token in current["body"] or self._token in current["title"]:
            raise IssueToolError("Issue content contains the API token")
        body = _managed_body(current["body"], delivery_id, hub_url)
        if body != current["body"]:
            return {"number": number, "url": current["html_url"], "linked": False,
                    "manual_update_required": True,
                    "managed_block": _managed_body("", delivery_id, hub_url).rstrip("\n")}
        readback = self._request_json("GET", read_path)
        try:
            verified = _validated_issue(readback, owner, repo, number)
        except IssueToolError:
            raise IssueToolError("Issue read-back validation failed") from None
        if _managed_body(verified["body"], delivery_id, hub_url) != verified["body"]:
            raise IssueToolError("Issue read-back validation failed")
        return {"number": number, "url": verified["html_url"], "linked": True}


class _SafeArgumentParser(argparse.ArgumentParser):
    def error(self, message):
        # argparse's default error can echo arbitrary credentials or private input.
        raise IssueToolError("invalid command arguments; use --help")


def _argument_parser() -> argparse.ArgumentParser:
    parser = _SafeArgumentParser(description="Link an existing Gitee Issue to a map delivery.")
    commands = parser.add_subparsers(dest="command", required=True)
    link = commands.add_parser("link", help="verify saved block, or prepare manual update (exit 2); never PATCH")
    for name in ("owner", "repo", "number", "delivery-id", "hub-url"):
        link.add_argument(f"--{name}", required=True)
    link.add_argument("--api-base", default=_API_BASE,
                      help="Gitee API v5; loopback fixtures require GITEE_ISSUE_TEST_TOKEN")
    return parser


def main(argv: list[str] | None = None) -> int:
    try:
        args = _argument_parser().parse_args(argv)
        local = _api_is_local(args.api_base)
        if local:
            if os.environ.get("GITEE_ACCESS_TOKEN", "").strip():
                raise IssueToolError("GITEE_ACCESS_TOKEN may only be sent to gitee.com")
            token = os.environ.get(_LOCAL_TEST_TOKEN_ENV, "")
            if not token.strip():
                raise IssueToolError(f"{_LOCAL_TEST_TOKEN_ENV} is required for local tests")
        else:
            token = os.environ.get("GITEE_ACCESS_TOKEN", "")
            if not token.strip():
                raise IssueToolError("GITEE_ACCESS_TOKEN is required")
        receipt = GiteeClient(token, args.api_base, allow_local_test_api=local).link(
            owner=args.owner, repo=args.repo, number=args.number,
            delivery_id=args.delivery_id, hub_url=args.hub_url,
        )
        print(json.dumps(receipt, ensure_ascii=False, sort_keys=True, separators=(",", ":")))
        return 0 if receipt["linked"] else 2
    except IssueToolError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
