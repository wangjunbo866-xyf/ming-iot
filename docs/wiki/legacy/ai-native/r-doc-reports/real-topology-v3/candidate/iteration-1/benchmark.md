# Skill Benchmark: r-doc

**Date**: 2026-10-12T14:21:04Z
**Evals**: 完整历史树与 Git 拓扑：complete, 完整历史树与 Git 拓扑：missing-child, 完整历史树与 Git 拓扑：standalone, 完整历史树与 Git 拓扑：version-drift (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 100% ± 0% |

## Per-Case Results

### 完整历史树与 Git 拓扑：complete (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v3/candidate/r-doc/evals/fixtures/scripts/check-complete.py | ✅ | {"pass": true, "errors": [], "source_file_count": 2924, "git_heads": {".": "c66b3055be4e199d2529883133e67369fb67600a", "repos/ming-iot": "e38d162feba9445c3bedb6ec78d96ee79dee9a49"}} |

### 完整历史树与 Git 拓扑：missing-child (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v3/candidate/r-doc/evals/fixtures/scripts/check-missing-child.py | ✅ | {"pass": true, "errors": [], "source_file_count": 160, "git_heads": {".": "c66b3055be4e199d2529883133e67369fb67600a"}} |

### 完整历史树与 Git 拓扑：standalone (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v3/candidate/r-doc/evals/fixtures/scripts/check-standalone.py | ✅ | {"pass": true, "errors": [], "source_file_count": 2764, "git_heads": {".": "e38d162feba9445c3bedb6ec78d96ee79dee9a49"}} |

### 完整历史树与 Git 拓扑：version-drift (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v3/candidate/r-doc/evals/fixtures/scripts/check-version-drift.py | ✅ | {"pass": true, "errors": [], "source_file_count": 2923, "git_heads": {".": "c66b3055be4e199d2529883133e67369fb67600a", "repos/ming-iot": "a6b28035c577131c1a4b0262af6ddbdc393104c5"}} |

