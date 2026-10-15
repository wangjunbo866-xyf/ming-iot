"""Install pinned Linux amd64 audit tools in the repository's ignored cache."""
import hashlib
import io
import json
from pathlib import Path
import platform
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def main():
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "AMD64"):
        raise SystemExit("CI tools currently require Linux amd64")
    destination = ROOT / ".cache/ci-tools"
    destination.mkdir(parents=True, exist_ok=True)
    for name, spec in json.loads((ROOT / "tooling/ci-tools.json").read_text()).items():
        data = urllib.request.urlopen(spec["url"], timeout=120).read()
        if hashlib.sha256(data).hexdigest() != spec["sha256"]:
            raise SystemExit(f"checksum mismatch: {name}")
        with tarfile.open(fileobj=io.BytesIO(data)) as archive:
            member = archive.getmember(name)
            if not member.isfile():
                raise SystemExit(f"unexpected archive member: {name}")
            content = archive.extractfile(member).read()
        target = destination / name
        target.write_bytes(content)
        target.chmod(0o755)
        print(f"installed {name} {spec['version']} with verified archive checksum")


if __name__ == "__main__":
    main()
