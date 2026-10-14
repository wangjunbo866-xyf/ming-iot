#!/usr/bin/env python3
"""Exercise pinned TeamAI with an isolated HOME and a local-only Git origin.

No TeamAI initialization, Git push, or config writes target the real workspace,
user HOME, or real team remote. npm package downloads still use the npm registry.
All generated files live under this repository's ignored .local/teamai-pilot/.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import urllib.request


ROOT = Path(__file__).resolve().parents[1]
PILOT_ROOT = ROOT / ".local" / "teamai-pilot"
TEAMAI_VERSION = "0.26.0"
NODE_VERSION = "22.21.1"
SYNTHETIC_URL = "https://teamai-pilot.invalid/ming/ai-native.git"
TEAMAI_INTEGRITY = "sha512-PsXmc8S7e6b88IK3Ze411q2Oj6i/2pl4FNg1t3WkXgLN2ndSY9H15Wwxg0KRjkbIaXH0tgNlsBlAq/N1DkhAug=="


def snapshot(base: Path) -> dict[str, str]:
    return {
        str(path.relative_to(base)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(base.rglob("*"))
        if path.is_file() and not path.is_symlink()
    }


def changed_files(before: dict[str, str], after: dict[str, str]) -> list[str]:
    return sorted(key for key in before.keys() | after.keys() if before.get(key) != after.get(key))


def download_metadata(package: str, version: str) -> dict:
    with urllib.request.urlopen(f"https://registry.npmjs.org/{package}/{version}", timeout=30) as response:
        return json.load(response)


def run(command: list[str], *, cwd: Path, env: dict[str, str], log: Path,
        allow_failure: bool = False, timeout: int = 180) -> subprocess.CompletedProcess:
    result = subprocess.run(command, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
    log.write_text(result.stdout, encoding="utf-8")
    if result.stderr:
        log.with_name(log.name + ".stderr.log").write_text(result.stderr, encoding="utf-8")
    if result.returncode and not allow_failure:
        raise RuntimeError(f"{Path(command[0]).name} failed with exit {result.returncode}; see {log}")
    return result


def main() -> int:
    if not shutil.which("npm") or not shutil.which("git"):
        raise RuntimeError("The pilot needs npm and git on PATH.")
    if sys.platform != "linux" or os.uname().machine != "x86_64":
        raise RuntimeError("This pilot pins the Linux x86_64 Node binary; use an isolated Linux x86_64 environment.")
    for required in ["teamai.yaml", "hooks/hooks.yaml", "skills/map-delivery/SKILL.md"]:
        if not (ROOT / required).is_file():
            raise RuntimeError(f"Missing pilot input: {required}")
    PILOT_ROOT.mkdir(parents=True, exist_ok=True)
    sandbox = Path(tempfile.mkdtemp(prefix="run-", dir=PILOT_ROOT))
    logs = sandbox / "logs"
    home = sandbox / "home"
    logs.mkdir()
    (home / ".codex").mkdir(parents=True)
    (home / ".agents" / "skills").mkdir(parents=True)
    # HOME is scoped to child processes to isolate the actual paths TeamAI uses.
    # No shell variable, shell profile, existing HOME, or existing CODEX_HOME is changed.
    env = {key: os.environ[key] for key in ["PATH", "LANG", "LC_ALL", "SSL_CERT_FILE",
                                          "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY"] if key in os.environ}
    env.update({"HOME": str(home), "USERPROFILE": str(home), "CODEX_HOME": str(home / ".codex"),
                "npm_config_cache": str(sandbox / "npm-cache"), "npm_config_update_notifier": "false",
                "npm_config_audit": "false", "npm_config_fund": "false", "GIT_CONFIG_NOSYSTEM": "1",
                "GIT_CONFIG_GLOBAL": str(sandbox / "gitconfig"), "GIT_TERMINAL_PROMPT": "0",
                "GIT_ALLOW_PROTOCOL": "file", "GIT_AUTHOR_NAME": "TeamAI Pilot",
                "GIT_AUTHOR_EMAIL": "pilot@example.invalid", "GIT_COMMITTER_NAME": "TeamAI Pilot",
                "GIT_COMMITTER_EMAIL": "pilot@example.invalid", "NO_COLOR": "1"})
    origin = sandbox / "origin.git"
    (sandbox / "gitconfig").write_text(
        '[user]\n\tname = TeamAI Pilot\n\temail = pilot@example.invalid\n'
        f'[url "{origin.as_uri()}"]\n\tinsteadOf = {SYNTHETIC_URL}\n', encoding="utf-8")
    metadata = download_metadata("teamai-cli", TEAMAI_VERSION)
    if metadata["dist"]["integrity"] != TEAMAI_INTEGRITY:
        raise RuntimeError("Pinned TeamAI registry integrity changed; review before running.")
    # Verify the published tarball itself, in addition to npm's lock integrity.
    with urllib.request.urlopen(metadata["dist"]["tarball"], timeout=30) as response:
        tarball = response.read()
    actual_integrity = "sha512-" + base64.b64encode(hashlib.sha512(tarball).digest()).decode()
    if actual_integrity != TEAMAI_INTEGRITY:
        raise RuntimeError("Published TeamAI tarball does not match the pinned integrity.")
    runtime = sandbox / "runtime"
    run(["npm", "install", "--ignore-scripts", "--no-audit", "--no-fund", "--prefix", str(runtime),
         f"teamai-cli@{TEAMAI_VERSION}", f"node-linux-x64@{NODE_VERSION}"],
        cwd=sandbox, env=env, log=logs / "npm-install.log", timeout=300)
    node = runtime / "node_modules" / "node-linux-x64" / "bin" / "node"
    cli = runtime / "node_modules" / "teamai-cli" / "dist" / "index.js"
    env["PATH"] = str(node.parent) + os.pathsep + env.get("PATH", "")
    cli_args = [str(node), str(cli)]
    node_actual = run([str(node), "--version"], cwd=sandbox, env=env, log=logs / "node-version.log").stdout.strip()
    teamai_actual = run(cli_args + ["--version"], cwd=sandbox, env=env, log=logs / "teamai-version.log").stdout.strip()
    if node_actual != f"v{NODE_VERSION}" or teamai_actual != TEAMAI_VERSION:
        raise RuntimeError("Installed runtime versions do not match the pins.")

    source = sandbox / "team-source"
    source.mkdir()
    for resource in ["skills", "rules", "hooks"]:
        if (ROOT / resource).is_dir():
            shutil.copytree(ROOT / resource, source / resource, symlinks=False)
    config_script = """
const fs = require('node:fs');
const YAML = require(process.argv[1]);
const c = YAML.parse(fs.readFileSync(process.argv[2], 'utf8'));
if (c.usageReport !== false || c.autoUpdate !== false || c.sharing?.env?.injectShellProfile !== false
 || c.sharing?.mcp?.autoApply !== false || c.sharing?.recall?.enabled !== false
 || c.sharing?.webhooks?.enabled !== false || Object.keys(c.toolPaths).join(',') !== 'codex'
 || c.toolPaths.codex.skills !== '.agents/skills' || c.toolPaths.codex.settings
 || c.toolPaths.codex.hooks || c.toolPaths.codex.mcp) throw new Error('Unexpected pilot profile');
for (const key of ['sources', 'scripts', 'packages', 'models'])
 if (c[key]) throw new Error('Unexpected executable or external config: ' + key);
c.repo = process.argv[4];
const h = YAML.parse(fs.readFileSync(process.argv[5], 'utf8'));
const required = ['session-start', 'stop', 'post-tool-use wildcard', 'post-tool-use Skill',
                  'post-tool-use TodoWrite', 'prompt-submit'].map(s => 'Hook dispatch ' + s);
if ((h.hooks || []).length || !required.every(k => (h.builtin?.disabled || []).includes(k)))
 throw new Error('Built-in hook disable list is incomplete or custom hooks are present');
fs.writeFileSync(process.argv[3], YAML.stringify(c));
"""
    run([str(node), "-e", config_script, str(runtime / "node_modules" / "yaml"), str(ROOT / "teamai.yaml"),
         str(source / "teamai.yaml"), SYNTHETIC_URL, str(ROOT / "hooks" / "hooks.yaml")],
        cwd=sandbox, env=env, log=logs / "profile.log")
    run(["git", "init", "--initial-branch=main", str(source)], cwd=sandbox, env=env, log=logs / "source-init.log")
    run(["git", "add", "."], cwd=source, env=env, log=logs / "source-add.log")
    run(["git", "commit", "-m", "Isolated TeamAI source"], cwd=source, env=env, log=logs / "source-commit.log")
    run(["git", "clone", "--bare", str(source), str(origin)], cwd=sandbox, env=env, log=logs / "origin.log")
    project = sandbox / "project"
    project.mkdir()
    (project / ".agents" / "skills").mkdir(parents=True)
    (project / ".codex").mkdir()
    (project / "README.md").write_text("Isolated TeamAI consumer\n", encoding="utf-8")
    run(["git", "init", "--initial-branch=main", str(project)], cwd=sandbox, env=env, log=logs / "project-init.log")
    run(["git", "add", "README.md"], cwd=project, env=env, log=logs / "project-add.log")
    run(["git", "commit", "-m", "Isolated consumer"], cwd=project, env=env, log=logs / "project-commit.log")
    run(cli_args + ["init", SYNTHETIC_URL, "--provider", "git", "--agent", "codex", "--scope", "project",
                    "--no-inherit-user-scope", "--force"], cwd=project, env=env, log=logs / "teamai-init.log")

    before = snapshot(project)
    home_before = snapshot(home)
    run(cli_args + ["--dry-run", "pull"], cwd=project, env=env, log=logs / "teamai-preview.log")
    preview_changed = changed_files(before, snapshot(project))
    preview_home_changed = changed_files(home_before, snapshot(home))
    # Check the source content as well as the tool-reported delivery state.
    pull = run(cli_args + ["pull"], cwd=project, env=env, log=logs / "teamai-pull.log", allow_failure=True)
    doctor = run(cli_args + ["doctor", "--json"], cwd=project, env=env, log=logs / "teamai-doctor.json", allow_failure=True)
    skill = project / ".agents" / "skills" / "map-delivery"
    missing = []
    mismatched = []
    verified = []
    for expected in sorted((source / "skills" / "map-delivery").rglob("*")):
        if not expected.is_file():
            continue
        relative = expected.relative_to(source / "skills" / "map-delivery")
        actual = skill / relative
        if not actual.is_file():
            missing.append(str(relative))
        elif expected.read_bytes() != actual.read_bytes():
            mismatched.append(str(relative))
        else:
            verified.append(str(relative))
    branches = run(["git", "--git-dir", str(origin), "for-each-ref", "--format=%(refname:short)"],
                   cwd=sandbox, env=env, log=logs / "origin-branches.log").stdout.splitlines()
    try:
        doctor_json = json.loads(doctor.stdout)
    except json.JSONDecodeError:
        doctor_json = {"unparsed": True, "log": str(logs / "teamai-doctor.json")}
    discovery = None
    inspector = ROOT / "tools" / "check_codex_discovery.py"
    if inspector.is_file() and shutil.which("codex", path=env["PATH"]):
        native_check = run([sys.executable, str(inspector), "--cwd", str(project),
                            "--expected-source", str(skill), "--output", str(logs / "codex-discovery.json")],
                           cwd=project, env=env, log=logs / "codex-discovery-output.log", allow_failure=True)
        discovery = json.loads((logs / "codex-discovery.json").read_text()) if (logs / "codex-discovery.json").is_file() else {
            "passed": False, "exit_code": native_check.returncode}
    result = {
        "ok": not missing and not mismatched and pull.returncode == 0 and doctor.returncode == 0,
        "teamai": {"version": teamai_actual, "gitHead": metadata.get("gitHead"),
                   "integrity": actual_integrity, "repository": metadata.get("repository")},
        "node": node_actual, "sandbox": str(sandbox), "skill_installation": str(skill),
        "skill_source_files_verified": verified, "missing": missing, "mismatched": mismatched,
        "pull_exit_code": pull.returncode, "doctor_exit_code": doctor.returncode, "doctor": doctor_json,
        "codex_discovery": discovery,
        "dry_run_project_changed_files": preview_changed, "dry_run_home_changed_files": preview_home_changed,
        "local_bare_origin_branches": branches, "git_allowed_protocols": env["GIT_ALLOW_PROTOCOL"],
        "side_effects": {"member_registration": "local bare origin only",
                         "user_configuration": "isolated child-process HOME only",
                         "real_remote_write": False, "host_codex_initialized": False},
        "limits": ["Codex app-server discovery, when present, does not prove this open chat has reloaded its skill catalog.",
                   "Real Gitee private-repository permissions are not tested.",
                   "Rules delivery does not prove Codex reads them when lifecycle hooks are disabled.",
                   "Administrative docs are omitted from the isolated fixture; full TeamAI pull also mirrors root docs."],
        "logs": str(logs),
    }
    if discovery is not None:
        result["ok"] = result["ok"] and discovery.get("passed", False)
    evidence = PILOT_ROOT / "result.json"
    evidence.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"ok": result["ok"], "version": teamai_actual, "evidence": str(evidence),
                      "missing": missing, "mismatched": mismatched, "doctor_exit_code": doctor.returncode},
                     ensure_ascii=False))
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError, subprocess.SubprocessError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
