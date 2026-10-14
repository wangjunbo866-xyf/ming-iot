#!/usr/bin/env python3
"""Query Codex's read-only skills/list API without starting a model turn."""

from __future__ import annotations

import argparse
import json
import os
import selectors
import subprocess
import sys
import time
from pathlib import Path


def inspect(cwds: list[Path], expected: Path | None) -> dict:
    process = subprocess.Popen(
        ["codex", "app-server", "--stdio", "-c", "analytics.enabled=false"],
        cwd=cwds[0], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ)
    pending = b""

    def request(identifier: int, method: str, params: dict) -> dict:
        nonlocal pending
        message = {"id": identifier, "method": method, "params": params}
        process.stdin.write((json.dumps(message) + "\n").encode())
        process.stdin.flush()
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if b"\n" in pending:
                line, pending = pending.split(b"\n", 1)
                response = json.loads(line)
                if response.get("id") == identifier:
                    if "error" in response:
                        raise RuntimeError("Codex API 请求失败")
                    return response["result"]
                continue
            if selector.select(max(0, deadline - time.monotonic())):
                chunk = os.read(process.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError("Codex API 连接提前关闭")
                pending += chunk
        raise RuntimeError("Codex API 请求超时")

    try:
        request(1, "initialize", {
            "clientInfo": {"name": "ming-skill-discovery-check", "version": "1.0"},
            "capabilities": {"experimentalApi": True, "explicitGatewayOauth": True},
        })
        process.stdin.write(b'{"method":"initialized"}\n')
        process.stdin.flush()
        response = request(2, "skills/list", {
            "cwds": [str(path) for path in cwds], "forceReload": True,
        })
        entries = {Path(entry["cwd"]).resolve(): entry
                   for entry in response.get("data", [])}
        records = []
        for cwd in cwds:
            entry = entries.get(cwd)
            matches = [] if entry is None else [
                {"path": skill["path"], "enabled": skill.get("enabled", True)}
                for skill in entry.get("skills", [])
                if skill.get("name") == "map-delivery"
            ]
            paths_match = expected is None or all(
                Path(match["path"]).resolve().parent == expected for match in matches
            )
            error_count = len(entry.get("errors", [])) if entry else 0
            records.append({
                "cwd": str(cwd), "matches": matches,
                "scan_error_count": error_count,
                "passed": bool(entry is not None and matches and paths_match
                               and all(match["enabled"] for match in matches)
                               and error_count == 0),
            })
        return {"passed": all(record["passed"] for record in records),
                "verification": "Codex app-server skills/list; no model turn",
                "directories": records}
    finally:
        selector.close()
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        for stream in (process.stdin, process.stdout):
            if stream:
                stream.close()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cwd", type=Path, action="append", required=True)
    parser.add_argument("--expected-source", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        result = inspect([path.resolve(strict=True) for path in args.cwd],
                         args.expected_source.resolve(strict=True)
                         if args.expected_source else None)
    except (OSError, ValueError, KeyError, RuntimeError):
        print("无法完成 Codex 技能发现检查；确认 codex 命令和本地 app-server API 可用。",
              file=sys.stderr)
        return 2
    encoded = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(encoded, encoding="utf-8")
    print(encoded, end="")
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
