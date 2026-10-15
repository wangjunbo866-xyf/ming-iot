# Skill Benchmark: r-doc

**Date**: 2026-10-12T16:56:30Z
**Evals**: 真实快照语义核验：aligned-snapshot (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 67% ± 0% |

## Per-Case Results

### 真实快照语义核验：aligned-snapshot (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-ota-v4/r-doc/evals/fixtures/scripts/check-aligned-snapshot.py | ❌ | {"pass": false, "errors": ["schema 判断错误: DeviceModuleVersionIndexReq/code_field", "schema 判断错误: DeviceModuleVersionReadReq/code_field", "schema 判断错误: DeviceModuleVersion/code_field"], "immutable_file_count": 2927} |

