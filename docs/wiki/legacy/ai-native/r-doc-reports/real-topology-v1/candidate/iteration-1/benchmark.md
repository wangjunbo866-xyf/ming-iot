# Skill Benchmark: r-doc

**Date**: 2026-10-12T12:48:55Z
**Evals**: 完整历史树与 Git 拓扑：complete, 完整历史树与 Git 拓扑：missing-child, 完整历史树与 Git 拓扑：standalone, 完整历史树与 Git 拓扑：version-drift (1 runs each per configuration)

## Summary

| Metric | With Skill |
|--------|------------|
| Pass Rate | 67% ± 0% |

## Per-Case Results

### 完整历史树与 Git 拓扑：complete (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/candidate/r-doc/evals/fixtures/scripts/check-complete.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809052652331344/script: line 1: import: command not found
/tmp/skill-up-judge-1791809052652331344/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809052652331344/script: line 2: `root=pathlib.Path.cwd()'] |

### 完整历史树与 Git 拓扑：missing-child (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/candidate/r-doc/evals/fixtures/scripts/check-missing-child.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809051899235715/script: line 1: import: command not found
/tmp/skill-up-judge-1791809051899235715/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809051899235715/script: line 2: `root=pathlib.Path.cwd()'] |

### 完整历史树与 Git 拓扑：standalone (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/candidate/r-doc/evals/fixtures/scripts/check-standalone.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809335182708349/script: line 1: import: command not found
/tmp/skill-up-judge-1791809335182708349/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809335182708349/script: line 2: `root=pathlib.Path.cwd()'] |

### 完整历史树与 Git 拓扑：version-drift (with_skill)

- **Pass Rate**: 67% (2/3)

| Expectation | Result | Evidence |
|-------------|--------|----------|
| expect.exit_code | ✅ | all checks passed |
| expect.files_exist | ✅ | all checks passed |
| script: /home/wangjunbo/workspace-iot-platform/ai-native/.scratch/r-doc-pilot/real-eval-topology-v1/candidate/r-doc/evals/fixtures/scripts/check-version-drift.py | ❌ | script exited with code 2 [stderr: /tmp/skill-up-judge-1791809256903481539/script: line 1: import: command not found
/tmp/skill-up-judge-1791809256903481539/script: line 2: syntax error near unexpected token `('
/tmp/skill-up-judge-1791809256903481539/script: line 2: `root=pathlib.Path.cwd()'] |

