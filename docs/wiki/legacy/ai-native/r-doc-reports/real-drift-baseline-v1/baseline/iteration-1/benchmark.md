# Skill Benchmark: r-doc

**Date**: 2026-10-12T13:41:37Z
**Evals**: 真实快照语义核验：stale-snapshot (1 runs each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|-----------|--------------|-------|
| Pass Rate | 50% ± 0% | 67% ± 0% | -0.17 |

## Per-Case Results

### 真实快照语义核验：stale-snapshot (with_skill)

- **Pass Rate**: 50% (1/2)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ❌ | expected file "report.json" does not exist |

### 真实快照语义核验：stale-snapshot (without_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-drift-v1/baseline/r-doc/evals/fixtures/scripts/check-stale-snapshot.py | ❌ | {"pass": false, "errors": ["原文引用无效: repos/ming-iot/backend/things/service/apisvr/http/things/device/moduleVersion.api", "原文引用无效: repos/ming-iot/backend/things/service/dmsvr/proto/deviceManage.proto", "原文引用无效: repos/ming-iot/backend/things/service/dmsvr/internal/repo/relationDB/model.go"], "immutable_file_count": 2927} |

