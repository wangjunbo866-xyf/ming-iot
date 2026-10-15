# Skill Benchmark: r-doc

**Date**: 2026-10-12T12:21:55Z
**Evals**: 双仓材料齐全的真实切片, 文档在场而代码缺失, 单独打开产品代码切片 (1 runs each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|-----------|--------------|-------|
| Pass Rate | 100% ± 0% | 67% ± 47% | +0.33 |

## Per-Case Results

### 双仓材料齐全的真实切片 (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-v1/baseline/r-doc/evals/fixtures/scripts/check-paired-slice.py | ✅ | {"pass": true, "errors": []} |

### 文档在场而代码缺失 (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-v1/baseline/r-doc/evals/fixtures/scripts/check-missing-code.py | ✅ | {"pass": true, "errors": []} |

### 单独打开产品代码切片 (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-v1/baseline/r-doc/evals/fixtures/scripts/check-code-standalone.py | ✅ | {"pass": true, "errors": []} |

### 双仓材料齐全的真实切片 (without_skill)

- **Pass Rate**: 0% (0/0)

### 文档在场而代码缺失 (without_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-v1/baseline/r-doc/evals/fixtures/scripts/check-missing-code.py | ✅ | {"pass": true, "errors": []} |

### 单独打开产品代码切片 (without_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-v1/baseline/r-doc/evals/fixtures/scripts/check-code-standalone.py | ✅ | {"pass": true, "errors": []} |

