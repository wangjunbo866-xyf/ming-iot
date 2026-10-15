# Skill Benchmark: r-doc

**Date**: 2026-10-12T12:45:50Z
**Evals**: 完整历史树与 Git 拓扑：complete (1 runs each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|-----------|--------------|-------|
| Pass Rate | 67% ± 0% | 67% ± 0% | +0.00 |

## Per-Case Results

### 完整历史树与 Git 拓扑：complete (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/baseline/r-doc/evals/fixtures/scripts/check-complete.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809108967311541/script: line 1: import: command not found
/tmp/skill-up-judge-1791809108967311541/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809108967311541/script: line 2: `root=pathlib.Path.cwd()'] |

### 完整历史树与 Git 拓扑：complete (without_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/baseline/r-doc/evals/fixtures/scripts/check-complete.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809150689809577/script: line 1: import: command not found
/tmp/skill-up-judge-1791809150689809577/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809150689809577/script: line 2: `root=pathlib.Path.cwd()'] |

