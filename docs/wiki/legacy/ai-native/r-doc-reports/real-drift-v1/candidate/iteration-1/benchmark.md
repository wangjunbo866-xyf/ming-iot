# Skill Benchmark: r-doc

**Date**: 2026-10-12T13:34:21Z
**Evals**: 真实快照语义核验：stale-snapshot, 真实快照语义核验：aligned-snapshot (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 50% ± 0% |

## Per-Case Results

### 真实快照语义核验：stale-snapshot (with_skill)

- **Pass Rate**: 50% (1/2)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ❌ | expected file "report.json" does not exist |

### 真实快照语义核验：aligned-snapshot (with_skill)

- **Pass Rate**: 50% (1/2)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ❌ | expected file "report.json" does not exist |

