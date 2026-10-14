---
document_kind: evidence
status: partial
delivery_issue: ming-iot#IKD4OR
code_repository: git:gitee.com/ming-industry/ming-iot
exact_head: def1ce15c010cd3c7d446299500227c758b42579
related_documents:
  - specs/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-design.md
  - plans/2026-09-03-ming-iot-gitee-delivery-and-verification-profile.md
  - reviews/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-review.md
---

# Evidence — IKD4OR 交付与验证档位独立复审（脱敏）

- **goal：** ming-iot-framework-v1
- **评审 lane：** ming-review-claude（独立评审）
- **评审对象：** Gitee Draft PR #3（https://gitee.com/ming-industry/ming-iot/pulls/3），分支 feature/gitee-delivery-verification-profile
- **exact head：** `def1ce15c010cd3c7d446299500227c758b42579`
- **base：** master@`eb13958829855cbafe0845b8f122302ed159cfd1`
- **评审 worktree：** ming-iot `.worktrees/review-ikd4or-pr3`（detached @def1ce15，评审前后 clean）+ rd-docs `.worktrees/ikd4or-pr3-rereview`
- **verdict：** **PASS_WITH_WARNINGS**（blocker 0 / warning 2 接受并跟踪 / note 3）
- **review_ref：** `reviews/2026-09-03-ming-iot-gitee-delivery-and-verification-profile-review.md`（DOC_REGISTRY：gitee_delivery_verification_review）

## 复跑命令与结果（绑定 exact head def1ce15）

```text
make verify-pr（context+diff-check+build+test+vet+contract-static+boundary-scan 全链）
                                  → PASS exit 0（55 ok 包行；repositorycontract 四契约测试 ok；
                                    Kafka contract PASS digest cc117b59… 完好）
make verify-fast VERIFY_SCOPE=tools → PASS
make contract-static（verify-pr 内含）→ PASS（apicheck 244/242/242 detached=2 + 2×rpcgen
                                       + things-baseline-contract mktemp 0600 守卫；docker 29.4.0）
boundary-scan（verify-pr 内含）     → PASS
make verify-integration（kafka/things-startup）→ NOT_RUN（重型档未授权；机制经代码审阅：
                                       参数白名单 exit2、EXIT trap 清理传播最差状态、env 0600 校验 exit3）
make verify-release                → NOT_RUN（重型确认档未授权）
```

## B1 修复核验（相对第一轮 BLOCK @07620d3）

vet 既有项修复（auth.go keyed、eventLog/propertyLatest 取址免锁拷贝等）；share/errors i18n 断言按实际行为修正（确定性）；NATS/MQTT/geoip 环境驱动 `t.Skipf` 关联缺陷 Issue IKD8AN；StepFloat 步长量化语义修正；死代码 89 行删除经 build 验证。全量门禁在本 head 独立复跑 exit 0。

## W1/W2 裁决

W1（CI 产物收集移除）→ ACCEPTED_FOR_PR_GATE（发布流水线另行补齐，保留跟踪）；W2（go-build 缓存 root 路径）→ ACCEPTED（Gitee runner 惯例）。Note：go.mod/go.sum 整体豁免延续（Kafka 周期已记录遗留，超出本 PR 范围）。

## 边界声明

零产品代码修改；除本 evidence、配对 review 文件与 meta/DOC_REGISTRY.yaml 登记外无写操作；未 commit/merge/push/发布/生产；重型档 NOT_RUN 如实记录。
