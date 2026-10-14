# Skill Benchmark: r-doc

**Date**: 2026-10-12T16:50:07Z
**Evals**: 真实快照语义核验：stale-snapshot (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 100% ± 0% |

## Per-Case Results

### 真实快照语义核验：stale-snapshot (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-ota-v4/r-doc/evals/fixtures/scripts/check-stale-snapshot.py | ✅ | {"pass": true, "errors": [], "immutable_file_count": 2927} |

