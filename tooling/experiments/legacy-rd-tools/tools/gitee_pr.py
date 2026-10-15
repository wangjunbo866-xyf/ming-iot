#!/usr/bin/env python3
"""Create or update one Gitee Draft PR from a verified local candidate branch."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlparse
from urllib.request import HTTPRedirectHandler, Request, build_opener


_GIT_OUTPUT_LIMIT = 2_000
_HTTP_BODY_LIMIT = 1_000_000
_TEXT_FILE_LIMIT = 1_000_000
_IDENTIFIER = re.compile(r"^[A-Za-z0-9_.-]+$")
_SHA1 = re.compile(r"^[0-9a-f]{40}$")
_LOCAL_TEST_TOKEN_ENV = "GITEE_PR_TEST_TOKEN"


class PRToolError(RuntimeError):
    """A stable, public-safe failure that may be shown to an operator."""


class _RejectRedirectHandler(HTTPRedirectHandler):
    """Keep bearer credentials on the explicitly trusted API origin."""

    def redirect_request(
        self,
        req: Request,
        fp: Any,
        code: int,
        msg: str,
        headers: Any,
        newurl: str,
    ) -> None:
        return None


@dataclass(frozen=True)
class Candidate:
    repo_root: Path
    owner: str
    repository: str
    head_branch: str
    base_branch: str
    exact_head: str


@dataclass(frozen=True)
class PullRequest:
    number: int
    url: str
    draft: bool
    state: str
    head_branch: str
    head_repository: str
    base_branch: str
    head_sha: str | None
    title: str
    body: str


def _git(repo_root: Path, *args: str) -> str:
    try:
        result = subprocess.run(
            ["git", "-C", str(repo_root), *args],
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=20,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise PRToolError("Git command could not be completed") from exc
    if result.returncode != 0:
        raise PRToolError("Git command failed")
    return result.stdout[:_GIT_OUTPUT_LIMIT]


def _parse_gitee_origin(remote_url: str) -> tuple[str, str]:
    owner: str | None = None
    repository: str | None = None

    scp_match = re.fullmatch(r"git@gitee\.com:([^/]+)/([^/]+?)(?:\.git)?", remote_url)
    if scp_match:
        owner, repository = scp_match.groups()
    else:
        parsed = urlparse(remote_url)
        if (
            parsed.scheme in {"https", "ssh"}
            and parsed.hostname == "gitee.com"
            and parsed.username in {None, "git"}
            and parsed.password is None
            and not parsed.query
            and not parsed.fragment
        ):
            parts = [part for part in parsed.path.split("/") if part]
            if len(parts) == 2:
                owner, repository = parts
                if repository.endswith(".git"):
                    repository = repository[:-4]

    if not owner or not repository:
        raise PRToolError("origin is not a Gitee repository")
    if not _IDENTIFIER.fullmatch(owner) or not _IDENTIFIER.fullmatch(repository):
        raise PRToolError("origin is not a Gitee repository")
    return owner, repository


def _remote_branch_head(repo_root: Path, branch: str, label: str) -> str:
    remote_output = _git(
        repo_root,
        "ls-remote",
        "--heads",
        "origin",
        f"refs/heads/{branch}",
    ).strip()
    rows = [line.split() for line in remote_output.splitlines() if line.strip()]
    if not rows:
        raise PRToolError(f"{label} branch does not exist")
    if len(rows) != 1 or len(rows[0]) < 2 or not _SHA1.fullmatch(rows[0][0].lower()):
        raise PRToolError(f"{label} branch lookup was ambiguous")
    return rows[0][0].lower()


def resolve_candidate(repo_root: str | Path, base_branch: str) -> Candidate:
    """Resolve and verify a single clean, pushed Gitee candidate branch."""

    requested_root = Path(repo_root).resolve()
    if not requested_root.is_dir():
        raise PRToolError("not a Git repository")

    try:
        canonical_root = Path(_git(requested_root, "rev-parse", "--show-toplevel").strip()).resolve()
    except PRToolError as exc:
        raise PRToolError("not a Git repository") from exc

    if _git(canonical_root, "status", "--porcelain").strip():
        raise PRToolError("worktree is dirty")

    try:
        head_branch = _git(
            canonical_root, "symbolic-ref", "--quiet", "--short", "HEAD"
        ).strip()
    except PRToolError as exc:
        raise PRToolError("detached HEAD is not supported") from exc
    try:
        _git(canonical_root, "check-ref-format", "--branch", base_branch)
    except PRToolError as exc:
        raise PRToolError("base branch is invalid") from exc
    if not head_branch or head_branch == base_branch:
        raise PRToolError("head branch must differ from base branch")
    if head_branch == "master":
        raise PRToolError("master cannot be a head branch")

    exact_head = _git(canonical_root, "rev-parse", "HEAD").strip().lower()
    if not _SHA1.fullmatch(exact_head):
        raise PRToolError("local HEAD is not a valid commit")

    try:
        raw_origin = _git(canonical_root, "config", "--get", "remote.origin.url").strip()
    except PRToolError as exc:
        raise PRToolError("origin is not a Gitee repository") from exc
    owner, repository = _parse_gitee_origin(raw_origin)

    remote_head = _remote_branch_head(canonical_root, head_branch, "remote")
    if remote_head != exact_head:
        raise PRToolError("local HEAD does not match remote branch")
    _remote_branch_head(canonical_root, base_branch, "base")

    return Candidate(
        repo_root=canonical_root,
        owner=owner,
        repository=repository,
        head_branch=head_branch,
        base_branch=base_branch,
        exact_head=exact_head,
    )


def _branch_from_payload(value: object) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        for key in ("label", "ref"):
            branch = value.get(key)
            if isinstance(branch, str):
                return branch
    return ""


def _repository_full_name_from_payload(value: object) -> str:
    if not isinstance(value, dict):
        return ""
    full_name = value.get("full_name")
    if isinstance(full_name, str):
        parts = full_name.split("/")
        if len(parts) == 2 and all(_IDENTIFIER.fullmatch(part) for part in parts):
            return full_name
    owner = value.get("owner")
    repository = value.get("name")
    if isinstance(owner, dict):
        for key in ("login", "name"):
            owner_name = owner.get(key)
            if (
                isinstance(owner_name, str)
                and isinstance(repository, str)
                and _IDENTIFIER.fullmatch(owner_name)
                and _IDENTIFIER.fullmatch(repository)
            ):
                return f"{owner_name}/{repository}"
    return ""


def _head_branch_from_payload(value: object) -> str:
    if not isinstance(value, dict):
        return value if isinstance(value, str) else ""
    label = value.get("label")
    if isinstance(label, str):
        return label
    branch = value.get("ref")
    return branch if isinstance(branch, str) else ""


def _head_repository_from_payload(value: object) -> str:
    if not isinstance(value, dict):
        return ""
    return _repository_full_name_from_payload(value.get("repo"))


def _head_sha_from_payload(payload: dict[str, Any]) -> str | None:
    direct = payload.get("head_sha")
    if isinstance(direct, str):
        return direct.lower()
    head = payload.get("head")
    if isinstance(head, dict):
        nested = head.get("sha")
        if isinstance(nested, str):
            return nested.lower()
    return None


def _parse_pull_request(payload: object) -> PullRequest:
    if not isinstance(payload, dict):
        raise PRToolError("Gitee API returned an invalid pull request")
    number = payload.get("number")
    url = payload.get("html_url") or payload.get("url")
    draft = payload.get("draft")
    state = payload.get("state")
    title = payload.get("title")
    body = payload.get("body")
    if (
        not isinstance(number, int)
        or isinstance(number, bool)
        or not isinstance(url, str)
        or not isinstance(draft, bool)
        or not isinstance(state, str)
        or not isinstance(title, str)
        or not isinstance(body, str)
    ):
        raise PRToolError("Gitee API returned an invalid pull request")
    return PullRequest(
        number=number,
        url=url,
        draft=draft,
        state=state,
        head_branch=_head_branch_from_payload(payload.get("head")),
        head_repository=_head_repository_from_payload(payload.get("head")),
        base_branch=_branch_from_payload(payload.get("base")),
        head_sha=_head_sha_from_payload(payload),
        title=title,
        body=body,
    )


def _branch_matches(actual: str, expected: str, owner: str) -> bool:
    return actual in {expected, f"{owner}:{expected}"}


def _head_branch_matches(
    actual_branch: str,
    expected_branch: str,
    owner: str,
    repository: str,
    actual_repository: str,
) -> bool:
    if actual_repository:
        return (
            actual_repository == f"{owner}/{repository}"
            and actual_branch in {expected_branch, f"{owner}:{expected_branch}"}
        )
    return actual_branch == f"{owner}:{expected_branch}"


def _pull_request_url_matches(
    url: str, candidate: Candidate, number: int
) -> bool:
    try:
        parsed = urlparse(url)
        port = parsed.port
    except ValueError:
        return False
    return (
        parsed.scheme == "https"
        and parsed.hostname == "gitee.com"
        and port in {None, 443}
        and parsed.username is None
        and parsed.password is None
        and not parsed.params
        and not parsed.query
        and not parsed.fragment
        and parsed.path
        == f"/{candidate.owner}/{candidate.repository}/pulls/{number}"
    )


class GiteeClient:
    """Small Gitee API v5 client for one idempotent Draft PR operation."""

    def __init__(
        self,
        token: str,
        api_base: str = "https://gitee.com/api/v5",
        *,
        allow_local_test_api: bool = False,
    ) -> None:
        if not token.strip():
            raise PRToolError("GITEE_ACCESS_TOKEN is required")
        try:
            parsed = urlparse(api_base)
            hostname = parsed.hostname
            port = parsed.port
        except ValueError as exc:
            raise PRToolError("api base URL is invalid") from exc
        if (
            parsed.scheme not in {"http", "https"}
            or not parsed.netloc
            or parsed.username is not None
            or parsed.password is not None
            or parsed.query
            or parsed.fragment
        ):
            raise PRToolError("api base URL is invalid")
        is_public_gitee = (
            parsed.scheme == "https"
            and hostname == "gitee.com"
            and port in {None, 443}
        )
        is_local_test = hostname in {"127.0.0.1", "::1", "localhost"}
        if not is_public_gitee and not is_local_test:
            raise PRToolError("api base URL is not trusted")
        if is_local_test and not allow_local_test_api:
            raise PRToolError("local test API requires explicit opt-in")
        self._token = token.strip()
        self._api_base = api_base.rstrip("/")
        self._opener = build_opener(_RejectRedirectHandler())

    def _request_json(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        fields: dict[str, str] | None = None,
    ) -> object:
        url = f"{self._api_base}{path}"
        if query:
            url = f"{url}?{urlencode(query)}"
        data = urlencode(fields).encode("utf-8") if fields is not None else None
        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {self._token}",
            "User-Agent": "ming-iot-rd-docs-gitee-pr/1",
        }
        if data is not None:
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        request = Request(url, data=data, headers=headers, method=method)
        try:
            with self._opener.open(request, timeout=20) as response:
                raw = response.read(_HTTP_BODY_LIMIT + 1)
        except HTTPError as exc:
            raise PRToolError(f"Gitee API request failed with HTTP {exc.code}") from exc
        except (URLError, OSError, TimeoutError) as exc:
            raise PRToolError("Gitee API request could not be completed") from exc
        if len(raw) > _HTTP_BODY_LIMIT:
            raise PRToolError("Gitee API response was too large")
        try:
            return json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise PRToolError("Gitee API returned invalid JSON") from exc

    @staticmethod
    def _repository_path(candidate: Candidate) -> str:
        return f"/repos/{candidate.owner}/{candidate.repository}/pulls"

    def _matching_open_pull_requests(self, candidate: Candidate) -> list[PullRequest]:
        matches: list[PullRequest] = []
        for page in range(1, 101):
            payload = self._request_json(
                "GET",
                self._repository_path(candidate),
                query={"state": "open", "per_page": "100", "page": str(page)},
            )
            if not isinstance(payload, list):
                raise PRToolError("Gitee API returned an invalid pull request list")
            for item in payload:
                pull_request = _parse_pull_request(item)
                if _head_branch_matches(
                    pull_request.head_branch,
                    candidate.head_branch,
                    candidate.owner,
                    candidate.repository,
                    pull_request.head_repository,
                ) and _branch_matches(
                    pull_request.base_branch, candidate.base_branch, candidate.owner
                ):
                    matches.append(pull_request)
                    if len(matches) > 1:
                        return matches
            if len(payload) < 100:
                return matches
        raise PRToolError("Gitee API pull request list exceeded the safe page limit")

    def _read_pull_request(
        self,
        candidate: Candidate,
        number: int,
        *,
        title: str,
        body: str,
    ) -> PullRequest:
        payload = self._request_json(
            "GET", f"{self._repository_path(candidate)}/{number}"
        )
        pull_request = _parse_pull_request(payload)
        head_sha = pull_request.head_sha
        if head_sha is None:
            try:
                head_sha = _remote_branch_head(
                    candidate.repo_root, candidate.head_branch, "remote"
                )
            except PRToolError as exc:
                raise PRToolError(
                    "pull request read-back exact head could not be verified"
                ) from exc
        if (
            pull_request.number != number
            or not pull_request.draft
            or pull_request.state != "open"
            or not _head_branch_matches(
                pull_request.head_branch,
                candidate.head_branch,
                candidate.owner,
                candidate.repository,
                pull_request.head_repository,
            )
            or not _branch_matches(
                pull_request.base_branch, candidate.base_branch, candidate.owner
            )
            or head_sha != candidate.exact_head
            or pull_request.title != title
            or pull_request.body != body
            or not _pull_request_url_matches(pull_request.url, candidate, number)
        ):
            raise PRToolError("pull request read-back validation failed")
        return pull_request

    def ensure_draft(self, candidate: Candidate, title: str, body: str) -> PullRequest:
        """Create or update the unique open PR and verify its Draft state."""

        if not title.strip():
            raise PRToolError("pull request title is empty")
        if self._token in title or self._token in body:
            raise PRToolError("pull request content contains the API token")
        matches = self._matching_open_pull_requests(candidate)
        if len(matches) > 1:
            raise PRToolError("multiple open pull requests match the candidate")

        fields = {"title": title, "body": body, "draft": "true"}
        if matches:
            number = matches[0].number
            self._request_json(
                "PATCH",
                f"{self._repository_path(candidate)}/{number}",
                fields=fields,
            )
        else:
            payload = self._request_json(
                "POST",
                self._repository_path(candidate),
                fields={
                    **fields,
                    "head": candidate.head_branch,
                    "base": candidate.base_branch,
                },
            )
            number = _parse_pull_request(payload).number
        return self._read_pull_request(
            candidate,
            number,
            title=title,
            body=body,
        )


def _read_text_file(path: str, label: str) -> str:
    file_path = Path(path)
    try:
        if file_path.stat().st_size > _TEXT_FILE_LIMIT:
            raise PRToolError(f"{label} file is too large")
        return file_path.read_text(encoding="utf-8").rstrip("\n")
    except PRToolError:
        raise
    except (OSError, UnicodeDecodeError) as exc:
        raise PRToolError(f"{label} file could not be read") from exc


def _argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Create or update one verified Gitee Draft PR."
    )
    parser.add_argument("--repo-root", required=True)
    parser.add_argument("--base", required=True)
    parser.add_argument("--title-file", required=True)
    parser.add_argument("--body-file", required=True)
    parser.add_argument("--draft", action="store_true", required=True)
    parser.add_argument("--api-base", default="https://gitee.com/api/v5")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = _argument_parser().parse_args(argv)
    try:
        try:
            api_host = urlparse(args.api_base).hostname
        except ValueError as exc:
            raise PRToolError("api base URL is invalid") from exc
        is_local_test = api_host in {"127.0.0.1", "::1", "localhost"}
        if is_local_test:
            if os.environ.get("GITEE_ACCESS_TOKEN", "").strip():
                raise PRToolError("GITEE_ACCESS_TOKEN may only be sent to gitee.com")
            token = os.environ.get(_LOCAL_TEST_TOKEN_ENV, "")
            if not token.strip():
                raise PRToolError(f"{_LOCAL_TEST_TOKEN_ENV} is required for local tests")
        else:
            token = os.environ.get("GITEE_ACCESS_TOKEN", "")
            if not token.strip():
                raise PRToolError("GITEE_ACCESS_TOKEN is required")
        title = _read_text_file(args.title_file, "title")
        body = _read_text_file(args.body_file, "body")
        candidate = resolve_candidate(args.repo_root, args.base)
        pull_request = GiteeClient(
            token,
            api_base=args.api_base,
            allow_local_test_api=is_local_test,
        ).ensure_draft(
            candidate, title=title, body=body
        )
        receipt = {
            "draft": pull_request.draft,
            "exact_head": candidate.exact_head,
            "number": pull_request.number,
            "url": pull_request.url,
        }
        print(json.dumps(receipt, ensure_ascii=False, sort_keys=True, separators=(",", ":")))
        return 0
    except PRToolError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
