"""Check every reachable Git object; keep technical checks separate from release permission.

Reports contain object IDs, paths and rule IDs only. Gitleaks runs locally with
redaction enabled; its raw report and extracted objects are temporary.
"""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import stat
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MAX_ARCHIVE_DEPTH = 4
MAX_EXPANDED_BYTES = 512 * 1024 * 1024
MAX_MEMBER_BYTES = 64 * 1024 * 1024
MAX_ARCHIVE_MEMBERS = 10000


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args])


def forbidden(path, mode):
    parts = path.lower().split("/")
    if mode in ("160000", "120000"):
        return True
    if any(p in (".loopx", ".codex", ".scratch", ".git", "thingsboard", "airmq") for p in parts):
        return True
    return parts[-1] in (".gitmodules", ".env", "id_rsa", "id_ed25519") or path.endswith((".pem", ".key"))


def archive_suffix(data):
    # Classify by bytes at every layer, never by untrusted member names.
    if data.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")):
        return ".zip"
    if data.startswith(b"\x1f\x8b"):
        return ".gz"
    if data[257:262] == b"ustar":
        return ".tar"
    if data.startswith((b"7z\xbc\xaf\x27\x1c", b"Rar!", b"BZh", b"\xfd7zXZ", b"\x28\xb5\x2f\xfd")):
        raise ValueError("archive format requires a separately verified reader")
    return ""


def archive_leaves(data, budget, depth=0):
    kind = archive_suffix(data)
    if not kind:
        yield data
        return
    if depth >= MAX_ARCHIVE_DEPTH:
        raise ValueError("archive nesting limit exceeded")

    def read_member(stream):
        content = stream.read(MAX_MEMBER_BYTES + 1)
        budget[0] += len(content)
        budget[1] += 1
        if len(content) > MAX_MEMBER_BYTES or budget[0] > MAX_EXPANDED_BYTES or budget[1] > MAX_ARCHIVE_MEMBERS:
            raise ValueError("archive expansion limit exceeded")
        return content

    try:
        if kind == ".zip":
            with zipfile.ZipFile(io.BytesIO(data)) as archive:
                for member in archive.infolist():
                    if member.is_dir():
                        continue
                    if member.flag_bits & 1 or stat.S_ISLNK(member.external_attr >> 16):
                        raise ValueError("encrypted or symlink archive member refused")
                    with archive.open(member) as stream:
                        yield from archive_leaves(read_member(stream), budget, depth + 1)
        elif kind == ".gz":
            with gzip.GzipFile(fileobj=io.BytesIO(data)) as stream:
                yield from archive_leaves(read_member(stream), budget, depth + 1)
        else:
            with tarfile.open(fileobj=io.BytesIO(data), mode="r:") as archive:
                for member in archive:
                    if member.isdir():
                        continue
                    if not member.isfile():
                        raise ValueError("non-regular archive member refused")
                    with archive.extractfile(member) as stream:
                        yield from archive_leaves(read_member(stream), budget, depth + 1)
    except (zipfile.BadZipFile, tarfile.TarError, RuntimeError, EOFError) as error:
        raise ValueError("archive cannot be completely read") from error


def export_objects(root, destination):
    if git(root, "rev-parse", "--is-shallow-repository").strip() != b"false":
        raise ValueError("complete history required; shallow clone refused")
    ids = {line.split(b" ", 1)[0] for line in git(root, "rev-list", "--objects", "--all", "HEAD").splitlines()}
    objects = {}
    budget = [0, 0]
    process = subprocess.Popen(["git", "-C", str(root), "cat-file", "--batch"], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
    try:
        for oid in sorted(ids):
            process.stdin.write(oid + b"\n")
            process.stdin.flush()
            header = process.stdout.readline().split()
            kind, size = header[1].decode(), int(header[2])
            data = process.stdout.read(size)
            if len(data) != size or process.stdout.read(1) != b"\n":
                raise ValueError("incomplete Git object read")
            name = oid.decode()
            objects[name] = {"type": kind, "sha256": hashlib.sha256(data).hexdigest(), "bytes": size}
            if kind in ("blob", "commit", "tag"):
                (destination / name).write_bytes(data)
                if archive_suffix(data):
                    for leaf in archive_leaves(data, budget):
                        # Do not use original member paths, extensions or symlinks.
                        # Extracted content never inherits a top-level exception.
                        leaf_name = name + "__" + hashlib.sha256(leaf).hexdigest() + ".txt"
                        (destination / leaf_name).write_bytes(leaf)
    finally:
        process.stdin.close()
        process.stdout.close()
        process.wait()
    if process.returncode:
        raise ValueError("Git object export failed")
    return objects


def history_paths(root, objects):
    violations = set()
    for oid, info in objects.items():
        if info["type"] != "commit":
            continue
        for entry in git(root, "ls-tree", "-rz", oid).split(b"\0"):
            if not entry:
                continue
            metadata, name = entry.split(b"\t", 1)
            mode = metadata.split()[0].decode()
            path = name.decode("utf-8")
            if forbidden(path, mode):
                violations.add(path)
    return sorted(violations)


def unresolved_findings(findings, objects, exceptions):
    allowed = {(e["sha256"], e["rule"], e["line"]) for e in exceptions if e.get("reason")}
    unresolved = []
    for finding in findings:
        outer = finding["File"].split("!", 1)[0]
        exported_name = Path(outer).name.split(".", 1)[0]
        oid = exported_name.split("__", 1)[0]
        info = objects.get(oid, {})
        key = (info.get("sha256"), finding["RuleID"], finding["StartLine"])
        # Archive members and decoded embedded data cannot inherit an exception.
        archive_member = "__" in exported_name or "!" in finding["File"] or finding.get("ArchivePath") or finding.get("DecodeDepth", 0)
        if key not in allowed or archive_member:
            unresolved.append({"object": oid, "rule": finding["RuleID"], "line": finding["StartLine"]})
    return unresolved


def publication_ready(policy):
    return policy.get("publication_status") == "ready" and policy.get("pending_decisions") == []


def check(root):
    if git(root, "status", "--porcelain", "--untracked-files=all").strip():
        raise ValueError("check requires a clean committed candidate")
    policy = json.loads((root / "docs/meta/PUBLICATION_POLICY.json").read_text())
    for item in policy["retained_notices"]:
        if hashlib.sha256((root / item["path"]).read_bytes()).hexdigest() != item["sha256"]:
            raise ValueError("license/source notice changed: " + item["path"])
    scanner = root / ".cache/ci-tools/gitleaks"
    expected = json.loads((root / "tooling/ci-tools.json").read_text())["gitleaks"]["version"]
    if subprocess.check_output([str(scanner), "version"], text=True).strip() != expected:
        raise ValueError("unexpected gitleaks version; run make ci-tools")
    exceptions = json.loads((root / "docs/meta/SCAN_EXCEPTIONS.json").read_text())["exceptions"]
    with tempfile.TemporaryDirectory(prefix="ming-publication-") as temp:
        work = Path(temp)
        blobs = work / "objects"
        blobs.mkdir()
        objects = export_objects(root, blobs)
        paths = history_paths(root, objects)
        # Explicit empty ignore/config surfaces prevent repo or environment allowlists.
        config = work / "gitleaks.toml"
        config.write_text("[extend]\nuseDefault = true\n")
        report = work / "findings.json"
        with (work / "scanner.log").open("w") as log:
            result = subprocess.run([str(scanner), "dir", str(blobs), "--config", str(config),
                                     "--gitleaks-ignore-path", str(work / "no-ignore"),
                                     "--ignore-gitleaks-allow", "--max-archive-depth=0",
                                     "--redact=100", "--no-banner", "--report-format=json",
                                     "--report-path", str(report)], stdout=log, stderr=log)
        if result.returncode not in (0, 1) or not report.exists():
            raise ValueError("scanner failed; raw output retained only in temporary private storage")
        findings = json.loads(report.read_text())
        unresolved = unresolved_findings(findings, objects, exceptions)
        summary = {"head": git(root, "rev-parse", "HEAD").decode().strip(),
                   "scope": "HEAD and all local refs; all reachable blobs, commit/tag messages and commit paths",
                   "objects": {kind: sum(i["type"] == kind for i in objects.values()) for kind in ("commit", "tree", "blob", "tag")},
                   "archive_leaves_scanned": sum("__" in f.name for f in blobs.iterdir()),
                   "findings": len(findings), "reviewed_exceptions": len(findings) - len(unresolved),
                   "unresolved": unresolved, "forbidden_paths": paths,
                   "technical_result": "FAIL" if paths or unresolved else "PASS",
                   "publication_status": policy["publication_status"]}
        print(json.dumps(summary, ensure_ascii=False, indent=2))
        return 1 if paths or unresolved else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "status", "ready"))
    args = parser.parse_args()
    try:
        if args.action == "check":
            return check(ROOT)
        policy = json.loads((ROOT / "docs/meta/PUBLICATION_POLICY.json").read_text())
        print(json.dumps({k: policy[k] for k in ("publication_status", "pending_decisions")}, ensure_ascii=False, indent=2))
        if args.action == "ready" and not publication_ready(policy):
            return 2
        return 0
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"FAIL: {error}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
