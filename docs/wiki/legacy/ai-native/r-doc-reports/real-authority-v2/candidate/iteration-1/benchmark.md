# Skill Benchmark: r-doc

**Date**: 2026-10-12T13:17:43Z
**Evals**: 真实 OTA 历史：维护唯一权威正文与索引 (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 100% ± 0% |

## Per-Case Results

### 真实 OTA 历史：维护唯一权威正文与索引 (with_skill)

- **Pass Rate**: 100% (3/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-authority-v2/candidate/r-doc/evals/fixtures/scripts/check-authority-update.py | ✅ | {"pass": true, "errors": [], "project_metadata_exit": 0, "immutable_file_count": 2922} |

