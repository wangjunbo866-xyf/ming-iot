# Skill Benchmark: r-doc

**Date**: 2026-10-12T11:03:09Z
**Evals**: 候选方案不能被历史讨论升级为现行规则 (1 runs each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|-----------|--------------|-------|
| Pass Rate | 100% ± 0% | 100% ± 0% | +0.00 |

## Per-Case Results

### 候选方案不能被历史讨论升级为现行规则 (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/baseline/r-doc/evals/fixtures/scripts/check-status.py | ✅ | {"pass": true, "errors": []} |

### 候选方案不能被历史讨论升级为现行规则 (without_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/baseline/r-doc/evals/fixtures/scripts/check-status.py | ✅ | {"pass": true, "errors": []} |

