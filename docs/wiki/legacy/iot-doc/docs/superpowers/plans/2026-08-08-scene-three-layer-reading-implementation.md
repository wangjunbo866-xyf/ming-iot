# 场景联动三层阅读结构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 README 阅读路由、最多 250 行的 15 分钟导读和四份长规格阅读卡片，让新读者通过一个复杂多设备规则和标准时序图理解 Scene 端到端链路，再按链接下钻到唯一权威章节。

**Architecture:** 保持 `场景联动 Runtime 主规格总纲.md` 为唯一规范性架构决策源，三份实施契约继续从属，附录继续非规范。新增导读只负责解释目标架构，用 `[现有实现]`、`[目标设计]`、`[待 ADR]` 区分落地状态；不移动、不重命名、不拆分现有文件，也不补造未冻结的物理 Topic、Consumer Group 或 transitionType。

**Tech Stack:** Markdown、Mermaid `sequenceDiagram`、仓库相对链接、Bash、`rg`、Node.js 20、临时使用 `@mermaid-js/mermaid-cli@11.4.2` 与 `markdown-link-check@3.13.7` 做验证。

---

## File map

**Create**

- `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`：第二层导读；唯一贯穿示例、标准时序图、状态/事务解释和按问题下钻入口。

**Modify**

- `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`：第一层阅读路由；首次阅读和按问题查阅入口。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`：在元信息后增加主规格阅读卡片。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`：在元信息后增加控制面阅读卡片。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`：在元信息后增加算子/事实阅读卡片。
- `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`：在元信息后增加 SLO/实施阅读卡片。

**Read but do not modify**

- `docs/superpowers/specs/2026-08-08-scene-documentation-three-layer-reading-design.md`：本计划的已确认设计输入。
- `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`：Reducer、ComputationPlan、Command 和平台代码归属的父架构来源。
- `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`：Raw、Canonical、CommandReply observation 的 Topic 来源。
- `docs/superpowers/specs/2026-08-04-scene-temperature-alarm-e2e-flow.html`：用户已有未提交修改；不得修改、暂存或提交。
- `.superpowers/`：用户已有未跟踪目录；不得修改、暂存或提交。

## Authority and status facts used by every task

- `scene_event` 与 `scene_state` 是已定义的 Scene 逻辑/物理 Topic 名；Scene Runtime Consumer Group 的物理名称仍是 `[待 ADR]`。
- `iot.data.raw.v1.<protocolKey>`、`iot.data.canonical.telemetry.v1`、`iot.data.canonical.command-reply-observation.v1` 已在 Kafka 设计列出；各 production group 的最终命名仍是 `[待 ADR]`。
- Computed Metric 结果流和 Accepted Transition 结果流已有逻辑消息契约，但物理 Topic 名仍是 `[待 ADR]`。
- `scene-current-fact-computed-metric-v1` 是文档已经给出的 Computed Metric Current Fact projector group 名，可以直接使用。
- 当前仓库没有 `things/runtime/{presence,command,automation,timeseries,latest}`、`protocol/runtime` 或 `protocol/applications`；这些程序节点统一标记 `[目标设计]`。
- 当前 `things/service/dmsvr` 存在，但只是目标架构中的 facade，旧的在线/回复直写逻辑不得标成新 reducer 实现。

### Task 1: Freeze the evidence matrix and prove the old reading entry fails the new contract

**Files:**

- Read: `docs/superpowers/specs/2026-08-08-scene-documentation-three-layer-reading-design.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Read: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`
- Read: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
- Read: `things/service/dmsvr/`

- [ ] **Step 1: Verify implementation-status evidence before writing labels**

  Run from `iot-doc/`:

  ```bash
  for path in \
    ../things/runtime/presence \
    ../things/runtime/command \
    ../things/runtime/automation \
    ../things/runtime/timeseries \
    ../things/runtime/latest \
    ../protocol/runtime \
    ../protocol/applications
  do
    test ! -e "$path"
  done

  test -d ../things/service/dmsvr

  if rg -n \
    'ComputedMetricUpdated|ComputationPlan|AcceptedStateTransition|AcceptedTransitionDictionary' \
    ../things ../protocol 2>/dev/null
  then
    exit 1
  fi
  ```

  Expected: exit 0 and no target-contract match. If any target package or contract now exists, stop and reclassify that node from repository evidence before editing the guide.

- [ ] **Step 2: Verify the exact known Topic and group facts**

  Run:

  ```bash
  rg -n \
    'iot\.data\.raw\.v1\.<protocolKey>|iot\.data\.canonical\.telemetry\.v1|iot\.data\.canonical\.command-reply-observation\.v1' \
    '亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md'

  rg -n \
    'scene-current-fact-computed-metric-v1|计算结果物理 Topic.*尚未冻结|consumer-group 名.*尚未冻结' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'

  rg -n \
    '\[P0 阻塞\].*consumer-group 命名' \
    '亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md'
  ```

  Expected: all three commands return matches. These are the only physical names the guide may present as frozen.

- [ ] **Step 3: Run the structural checks before implementation and confirm they fail**

  Run:

  ```bash
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'

  test -f "$scene_dir/00-15分钟读懂场景联动.md"
  ```

  Expected: exit 1 because the second-layer guide does not exist.

  Run:

  ```bash
  rg -n '00-15分钟读懂场景联动\.md|第一次阅读' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
  ```

  Expected: no output and exit 1 because README still sends readers directly to the 1,784-line main specification.

### Task 2: Create the 15-minute guide foundation

**Files:**

- Create: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Read: `亿级点位高吞吐架构/enterprise-iot-platform-architecture-design.md`

- [ ] **Step 1: Create the authority header, one-sentence model, and status legend**

  Start the new guide with this exact structure and meaning:

  ```markdown
  # 15 分钟读懂场景联动

  > 本文是解释性导读，不是新的架构决策源。
  > [场景联动 Runtime 主规格](场景联动 Runtime 主规格总纲.md)仍是本目录唯一规范性架构决策源；实现时必须下钻到链接章节。

  ## 1. 先记住一句话

  Scene 把 PropertyReport、DeviceEvent、ComputedMetricUpdated 和 reducer 已接受的
  AcceptedStateTransition 规范化为统一 SceneEvent，在同一 evaluation subject lane
  串行求值；只有 PostgreSQL Action Outbox 事务 commit 后，命令或 Webhook 才算被业务接受。

  ## 2. 先看状态标记

  | 标记 | 含义 |
  | --- | --- |
  | `[现有实现]` | 当前仓库可以定位到对应目标契约的代码实现 |
  | `[目标设计]` | 架构已经指定职责，但目标 runtime/package 或消息实现尚不存在 |
  | `[待 ADR]` | 物理 Topic、Consumer Group 名、完整枚举或事务细节尚未冻结 |

  > 本文时序图描述目标链路。当前仓库没有 `things/runtime/{presence,command,automation,timeseries,latest}`、
  > `protocol/runtime` 或 `protocol/applications`；现有 `things/service/dmsvr` 只是 facade/旧流程，
  > 不能据此把新 reducer、Scene Runtime 或 Computed Metric Runtime 标成已实现。
  ```

- [ ] **Step 2: Add the single scenario and four trusted fact sources**

  Add:

  ```markdown
  ## 3. 本文唯一示例

  R1 区域的 `temp-01/02/03` 共同计算 5 分钟平均温度。平均值超过 30℃，同时
  `door-01=CLOSED`，并持续 2 分钟后，平台向 `fan-01/02` 下发启动命令并发送 Webhook。
  两台风机的回复先由 Command reducer 收敛；只有合法状态变化才成为
  `AcceptedStateTransition`。

  示例中的 30℃、5 分钟和 2 分钟只用于讲解，不是产品默认值或容量承诺。

  ### 四类受信事实

  | Scene factType | 具体输入 | 谁产生 | 时间形态 |
  | --- | --- | --- | --- |
  | `PROPERTY` | `door-01=CLOSED` 等 PropertyReport 点值 | ProtocolApplication | `LATEST_STATE`，写 Current Fact |
  | `DEVICE_EVENT` | 设备一次独立事件 | ProtocolApplication | `OCCURRENCE`，不覆盖前一事件 |
  | `COMPUTED_METRIC` | R1 三个温度点的 5 分钟 AVG | Flink Computed Metric Runtime `[目标设计]` | `LATEST_STATE`，写 Current Fact |
  | `STATE_TRANSITION` | Command reducer 接受的命令状态变化 | Presence/Command reducer `[目标设计]` | `OCCURRENCE`，只能触发 |

  原始 Lifecycle/CommandReply 只是 observation，不能绕过 reducer 直接冒充
  `AcceptedStateTransition`。完整来源身份见[算子术语 §15.1](算子语义与术语.md#151-事实身份与来源)。
  ```

- [ ] **Step 3: Add the participant and ownership table**

  Add a compact table with exactly these roles:

  ```markdown
  ## 4. 先认清程序和状态所有者

  | 程序节点 | 本例职责 | 唯一写入状态/输出 | 状态 |
  | --- | --- | --- | --- |
  | Access Adapter | MQTT payload 机械投影到 Raw | Kafka Raw record | `[目标设计]` |
  | ProtocolApplication | 解码、校验并事务性产生 Canonical | PropertyReport / CommandReply observation | `[目标设计]` |
  | Timeseries Materializer | 一次消费完成 history、latest 和 PROPERTY Scene-live 槽 | TDengine + latest + PROPERTY Current Fact | `[目标设计]` |
  | Flink Computed Metric Runtime | 按共享 ComputationPlan 维护 5 分钟窗口 | ComputedMetricUpdated | `[目标设计]` |
  | ComputedMetricCurrentFactProjector | 投影最终计算指标 | COMPUTED_METRIC Current Fact | `[目标设计]` |
  | Scene Admission | 校验来源契约并规范化为 SceneEvent | `scene_event` SourceDelta | `[目标设计]` |
  | Scene Runtime | matching、latest-value join、duration 和动作接受 | 内存 SceneBindingState、异步 `scene_state`、PG Action Outbox | `[目标设计]` |
  | Command Runtime | 以 effectId 幂等创建并投递命令 | CommandExecution / Attempt / Outbox | `[目标设计]` |
  | Command reducer | 收敛重复、迟到和冲突回复 | command state/outcome + AcceptedStateTransition Outbox | `[目标设计]` |
  ```

- [ ] **Step 4: Verify the foundation before adding the diagram**

  Run:

  ```bash
  guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'

  test -f "$guide"
  test "$(rg -c '^## 1\. 先记住一句话$' "$guide")" -eq 1
  test "$(rg -c '^## 3\. 本文唯一示例$' "$guide")" -eq 1
  test "$(rg -c '^### 四类受信事实$' "$guide")" -eq 1
  test "$(rg -c '^## 4\. 先认清程序和状态所有者$' "$guide")" -eq 1
  test "$(rg -c '\[目标设计\]' "$guide")" -ge 9
  ```

  Expected: exit 0.

- [ ] **Step 5: Commit the guide foundation**

  ```bash
  git add '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  git commit -m 'docs: add scene linkage 15-minute guide foundation'
  ```

### Task 3: Add the standard end-to-end sequence diagram

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Read: `亿级点位高吞吐架构/4.1.Kafka/1.nats-to-kafka-greenfield-design.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`

- [ ] **Step 1: Add one standard Mermaid sequence diagram**

  Insert this as `## 5. 端到端时序图` after the ownership table. Keep it as `sequenceDiagram`; do not replace it with flowchart syntax or an image that needs localhost.

  ```mermaid
  sequenceDiagram
      autonumber
      box Devices
          participant SRC as temp-01/02/03 + door-01
          participant FAN as fan-01/02
      end
      box Ingress and Protocol
          participant EDGE as EMQX + Access Adapter<br/>[目标设计]
          participant RAW as «Kafka Topic» Raw<br/>iot.data.raw.v1.{protocolKey}
          participant PA as ProtocolApplication<br/>[目标设计]
          participant CAN as «Kafka Topic» Canonical Topics
      end
      box Computed Metric
          participant FLINK as Flink Metric Runtime<br/>[目标设计]
          participant DERIVED as «Kafka Topic» Derived Fact Streams<br/>physical topics [待 ADR]
          participant MCF as Metric CurrentFact Projector<br/>[目标设计]
      end
      box Scene
          participant MAT as Timeseries Materializer<br/>[目标设计]
          participant CF as «State Store» Scene Current Fact<br/>[目标设计]
          participant ADM as Scene Admission<br/>[目标设计]
          participant EVENT as «Kafka Topic» scene_event
          participant RT as Scene Runtime<br/>[目标设计]
          participant STATE as «Kafka Topic» scene_state
          participant PG as «PostgreSQL» Outbox + Command State
      end
      box Command
          participant CMD as Command Runtime<br/>[目标设计]
          participant RED as Command reducer<br/>[目标设计]
      end

      Note over SRC,FAN: R1 AVG(temp-01/02/03, 5m) > 30C<br/>AND door-01=CLOSED FOR 2m<br/>THEN start fan-01/02 + Webhook

      rect rgb(240, 248, 255)
          Note over SRC,CAN: Phase 1 - device reports become Durable Canonical
          par temperature reports
              SRC->>EDGE: MQTT temperature payloads
          and door report
              SRC->>EDGE: MQTT door=CLOSED payload
          end
          EDGE->>RAW: produce RawRecord<br/>producer=Access Adapter, acks=all
          loop Kafka redelivery until Raw transaction commits
              RAW-->>PA: poll RawRecord<br/>group=protocolKey production group [name 待 ADR]
              PA->>CAN: Kafka transaction produce<br/>iot.data.canonical.telemetry.v1<br/>PropertyReport(outputId, processingContext)
              PA->>RAW: sendOffsetsToTransaction(nextOffset)
          end
      end

      rect rgb(245, 255, 245)
          Note over CAN,CF: Phase 2 - the same Canonical stream has independent consumers
          par Materializer group [name 待 ADR]
              CAN-->>MAT: PropertyReport temp + door
              MAT->>CF: write PROPERTY Scene-live slots<br/>door-01=CLOSED
          and Flink group [name 待 ADR]
              CAN-->>FLINK: PropertyReport temp-01/02/03
              FLINK->>DERIVED: ComputedMetricUpdated<br/>planReleaseId + outputOrdinal + outputId<br/>finality=FINAL, production+LIVE
          and Scene Admission group [name 待 ADR]
              CAN-->>ADM: PropertyReport door-01
              ADM->>EVENT: SourceDelta(PROPERTY)<br/>CONTEXT_ONLY update has no trigger fanout
          end
          par group=scene-current-fact-computed-metric-v1
              DERIVED-->>MCF: ComputedMetricUpdated
              MCF->>CF: CAS COMPUTED_METRIC Current Fact<br/>R1 5m AVG temperature
          and Scene Admission group [name 待 ADR]
              DERIVED-->>ADM: ComputedMetricUpdated
              ADM->>EVENT: SourceDelta(COMPUTED_METRIC)<br/>sourceEventId + trigger identity
          end
      end

      rect rgb(255, 250, 235)
          Note over EVENT,STATE: Phase 3 - matching, latest-value join, duration and timer recheck
          EVENT-->>RT: poll SourceDelta<br/>group=Scene Runtime group [name 待 ADR]
          RT->>CF: fixed read-barrier overlay<br/>AVG temperature + door current facts
          CF-->>RT: READY values + coverage evidence
          RT->>RT: match Binding and evaluate<br/>AVG>30 AND door=CLOSED<br/>arm duration dueAt=triggerTime+2m
          RT-->>STATE: async best-effort full SceneBindingState
          Note over RT: after 2 minutes, callback returns to the same evaluation-subject lane
          RT->>RT: validate generation, Binding, slot and freshness
          RT->>CF: re-read and overlay required Current Facts
          CF-->>RT: current AVG + door state
      end

      rect rgb(255, 242, 245)
          Note over RT,PG: Phase 4 - a PG commit is the action-acceptance boundary
          alt predicate false or NOT_READY
              RT->>RT: cancel or keep timer by explicit policy<br/>no action and no Outbox row
          else predicate remains true
              RT->>PG: BEGIN<br/>insert EffectSetReceipt + DeviceCommand(fan-01)<br/>+ DeviceCommand(fan-02) + Webhook
              alt PostgreSQL commit fails
                  PG--xRT: ROLLBACK
                  RT->>RT: discard tentative state<br/>retry with the same trigger/effect identities
              else PostgreSQL commit succeeds
                  PG-->>RT: COMMIT acceptedAt<br/>action is now accepted
                  RT-->>STATE: async best-effort full SceneBindingState
                  CMD->>PG: claim DeviceCommand actions by effectId<br/>create CommandExecution + CommandOutbox idempotently
                  PG-->>CMD: commandId + attemptNo + frozen releases
                  CMD->>PA: encode immutable MQTT topic + payload
                  PA-->>CMD: encoded attempt
                  CMD->>EDGE: publish fan-01/02 command<br/>MQTT QoS 1
                  EDGE->>FAN: deliver command attempts
                  Note over PG,CMD: Webhook uses its own Outbox dispatcher<br/>the same effectId is the idempotency key
              end
          end
      end

      rect rgb(248, 242, 255)
          Note over FAN,ADM: Phase 5 - replies are observations until the reducer accepts a state change
          FAN->>EDGE: command replies(commandId, correlationId, attemptNo)
          EDGE->>RAW: produce reply RawRecord
          loop Kafka redelivery until Canonical/reducer terminal outcome
              RAW-->>PA: poll reply RawRecord<br/>protocol production group [name 待 ADR]
              PA->>CAN: Kafka transaction produce<br/>iot.data.canonical.command-reply-observation.v1
              CAN-->>RED: CommandReply observation<br/>group=Command reducer group [name 待 ADR]
              RED->>PG: transactionally update command state/outcome<br/>and register accepted-transition Outbox
          end
          alt duplicate, stale, conflict or unmatched
              RED->>RED: no accepted transition<br/>durable terminal outcome where required
          else legal monotonic state transition
              PG->>DERIVED: publish AcceptedStateTransition<br/>physical topic [待 ADR]<br/>causationOutputId + reducerVersion
              DERIVED-->>ADM: accepted transition<br/>Scene Admission group [name 待 ADR]
              ADM->>EVENT: SourceDelta(STATE_TRANSITION)<br/>only interested Bindings can trigger
          end
      end
  ```

- [ ] **Step 2: Add the five-stage fallback table immediately after the diagram**

  Use this exact information so the guide remains understandable when Mermaid is not rendered:

  ```markdown
  ### 图不显示时，按这五步读

  | 阶段 | 输入 → 处理 → 输出 | 本阶段成功不代表什么 |
  | --- | --- | --- |
  | 1. 上报与 Canonical | MQTT → Raw → ProtocolApplication Kafka transaction → PropertyReport | 不代表 Current Fact、Scene 或动作已完成 |
  | 2. 共享计算与投影 | Canonical 被 Materializer、Flink、Admission 各自 group 并行消费；Flink 输出 ComputedMetricUpdated | 一个 group 完成不代表其他 group 已完成 |
  | 3. Scene 求值 | SceneEvent → matching → Current Fact overlay → duration arm/fire | timer 到期不自动证明条件仍成立 |
  | 4. 动作接受 | EffectSetReceipt 与三条 action row 同一 PG transaction commit | 不代表设备或 Webhook 已完成 |
  | 5. 回复收敛 | CommandReply observation → Command reducer → AcceptedStateTransition | 原始回复和重复回复不能直接触发 Scene |
  ```

- [ ] **Step 3: Parse the Mermaid block before continuing**

  Run:

  ```bash
  guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  verify_dir=$(mktemp -d)

  awk '
    /^```mermaid$/ { capture=1; next }
    capture && /^```$/ { exit }
    capture { print }
  ' "$guide" > "$verify_dir/scene-e2e.mmd"

  npx --yes @mermaid-js/mermaid-cli@11.4.2 \
    -p <(node -e 'process.stdout.write(JSON.stringify({args:["--no-sandbox","--disable-setuid-sandbox"]}))') \
    -i "$verify_dir/scene-e2e.mmd" \
    -o "$verify_dir/scene-e2e.svg"

  test -s "$verify_dir/scene-e2e.svg"
  ```

  Expected: Mermaid CLI exits 0 and produces a non-empty SVG. The SVG is a temporary verification artifact and must not be added to Git.

- [ ] **Step 4: Verify Topic/Group labeling completeness**

  Run:

  ```bash
  guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'

  rg -n \
    'iot\.data\.raw\.v1|iot\.data\.canonical\.telemetry\.v1|iot\.data\.canonical\.command-reply-observation\.v1|scene-current-fact-computed-metric-v1|Kafka Topic.*scene_event|Kafka Topic.*scene_state|physical topic \[待 ADR\]|group=.*\[name 待 ADR\]' \
    "$guide"
  ```

  Expected: every listed known Topic appears; the exact projector group appears; every other physical group/output Topic is visibly marked pending ADR.

- [ ] **Step 5: Commit the standard sequence diagram**

  ```bash
  git add '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  git commit -m 'docs: add scene linkage end-to-end sequence'
  ```

### Task 4: Complete the guide with state, failure, identity, and deep-link explanations

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

- [ ] **Step 1: Add the three-state comparison**

  Add this section after the five-stage table:

  ```markdown
  ## 6. 三类状态不要混在一起

  | 状态 | 回答的问题 | key/所有者 | 本例内容 |
  | --- | --- | --- | --- |
  | Scene Current Fact | “现在已知的业务事实是什么？” | `(tenantCode,factOwnerKey,factType,factKey)`；每类来源一个唯一 writer | door=CLOSED、R1 5m AVG temperature |
  | SceneBindingState | “这个 Binding 在这个计算主体上算到哪里？” | `(tenantCode,bindingId,evaluationSubjectKey)`；Scene Runtime | 参数快照、duration ARMED/FIRED、dueAt、timer slot |
  | CommandExecution/Attempt | “命令创建、编码、投递和回复到哪一步？” | commandId/attemptNo；Command Runtime/reducer | fan-01/02 的两次独立执行和回复结果 |

  Current Fact 不是 Scene 状态，SceneBindingState 也不是命令状态。Admission 和 Scene Runtime
  都不是 Current Fact writer。完整定义见[算子术语 §15.3](算子语义与术语.md#153-current-fact-投影)
  和[主规格 §8](场景联动 Runtime 主规格总纲.md#8-scene_state-逻辑与物理模型)。
  ```

- [ ] **Step 2: Add the failure and redelivery table**

  Add:

  ```markdown
  ## 7. 重投为什么不应重复动作

  | 失败或重投位置 | 收敛依据 | 结果 |
  | --- | --- | --- |
  | Raw 被 ProtocolApplication 重读 | Kafka transaction、确定性 outputId、read_committed | 事务 abort 的输出不可见；重试保持同一业务身份 |
  | ComputedMetricUpdated 重投 | outputId、planReleaseId、revision/finality、Current Fact comparator | duplicate 不刷新 Current Fact，也不再次取得值更新权 |
  | SceneEvent 重投 | sourceEventId/trigger identity、EffectSetReceipt、effectId | 相同 effect set 幂等；不同 payload 形成 conflict，不静默覆盖 |
  | Action Outbox commit 前崩溃 | PG transaction | 没有 accepted action；tentative state/timer 丢弃并重算 |
  | Action Outbox commit 后崩溃 | durable Outbox row + effectId | dispatcher 可至少一次重试；已接受动作不因 scope exit 自动撤销 |
  | CommandReply 重投或迟到 | commandId + attemptNo + 单调状态机 | duplicate/stale 不发布新的 AcceptedStateTransition |

  “唯一 Outbox row”不等于设备或 Webhook exactly-once；外部执行仍必须使用 effectId 作为幂等键。
  事务顺序见[主规格 §12.3](场景联动 Runtime 主规格总纲.md#123-状态与-outbox-顺序)，
  故障承诺见[SLO 文档 §4](SLO故障测试与实施.md#4-外部效果承诺边界)。
  ```

- [ ] **Step 3: Add the identity chain and question-oriented links**

  Add:

  ```markdown
  ## 8. 本例的关键身份怎样传下去

  | 身份 | 谁产生 | 用途 |
  | --- | --- | --- |
  | outputId | ProtocolApplication / Flink / reducer | 稳定识别一条来源输出 |
  | sourceEventId / trigger identity | Scene Admission | Scene 重投与直接触发身份 |
  | transitionId / effectId | Scene Runtime | 语义迁移与每个目标动作的幂等身份 |
  | commandId / attemptNo | Command Runtime | 命令状态机与每次传输尝试 |
  | causationOutputId | reducer | 把 AcceptedStateTransition 追溯到原 CommandReply output |

  这些 ID 不是一个字段不断改名，而是不同边界上的稳定身份；完整 hash 和 provenance
  只在[算子术语 §15.2](算子语义与术语.md#152-wire-与身份-hash)查阅。

  ## 9. 接下来按问题读

  | 我想弄清楚 | 权威入口 |
  | --- | --- |
  | 整体 Runtime、scene_event、scene_state、恢复和事务顺序 | [主规格 §4～§14](场景联动 Runtime 主规格总纲.md#4-总体数据流) |
  | Binding、Scope、TRIGGER/CONTEXT_ONLY、matching/fanout | [控制面与匹配契约 §1～§6](控制面与匹配契约.md#1-控制面实体与稳定身份) |
  | Reducer、ComputationPlan、Current Fact、duration 和 timer | [算子语义与术语 §2、§8、§13、§15](算子语义与术语.md#2-共享术语) |
  | Action Outbox、故障窗口、POC 和 rollout | [SLO、故障测试与实施](SLO故障测试与实施.md#1-文档职责与判定语言) |
  | Raw/Canonical Topic 与 CommandReply 路径 | [Kafka 设计 §5.2、§9.2](../4.1.Kafka/1.nats-to-kafka-greenfield-design.md#52-数据面-topic) |
  | Flink/ComputationPlan 与命令目标架构 | [父架构 §11](../enterprise-iot-platform-architecture-design.md#11-自动化告警通知与命令) |
  ```

- [ ] **Step 4: Enforce the guide budget and non-authority boundary**

  Run:

  ```bash
  guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  line_count=$(wc -l < "$guide")

  test "$line_count" -le 250
  test "$(rg -c '本文是解释性导读，不是新的架构决策源' "$guide")" -eq 1
  test "$(rg -c '^```mermaid$' "$guide")" -eq 1
  test "$(rg -c '^sequenceDiagram$' "$guide")" -eq 1

  if rg -n 'ONLINE_TO_OFFLINE|DISPATCHED_TO_RESULT_SUCCEEDED|computed-metric-topic-v1|accepted-transition-topic-v1' "$guide"
  then
    exit 1
  fi
  ```

  Expected: exit 0. If the guide exceeds 250 lines, shorten prose or move field detail into the authority links; do not remove the sequence participants, phase table, three-state table, failure table, or question navigation.

- [ ] **Step 5: Commit the completed guide**

  ```bash
  git add '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  git commit -m 'docs: complete scene linkage quick-start guide'
  ```

### Task 5: Rewrite README as the first-layer reading router

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Read: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`

- [ ] **Step 1: Confirm the new-reader route is still absent before editing**

  Run:

  ```bash
  readme='亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'

  rg -n '^## 第一次阅读$|00-15分钟读懂场景联动\.md' "$readme"
  ```

  Expected: no output and exit 1.

- [ ] **Step 2: Replace the sequential-reading table with a two-route README**

  Keep the existing status warning and cross-directory synchronization boundary, but rewrite the navigation body to this structure:

  ```markdown
  ## 第一次阅读

  第一次接触本模块时，只需先读[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)。
  它用一个多设备规则串起程序节点、Topic、Consumer Group、状态和事务；读完再按问题下钻，
  不要求先顺序通读 1,000 行以上的规范。

  ## 按问题查阅

  | 我正在解决的问题 | 直接阅读 |
  | --- | --- |
  | 确认总体 Runtime、数据流、state、恢复和事务决策 | [主规格](场景联动 Runtime 主规格总纲.md) |
  | 实现 Binding、Scope、ProjectIndex、matching 或 fanout | [控制面与匹配契约](控制面与匹配契约.md) |
  | 查询 Reducer、ComputationPlan、Fact、算子、timer 或 Current Fact | [算子语义与术语](算子语义与术语.md) |
  | 制定 SLO、POC、故障注入、rollout 或 RACI | [SLO、故障测试与实施](SLO故障测试与实施.md) |
  | 了解方案来源和竞品证据 | [竞品状态调研](附录-竞品状态方案调研.md) · [ThingsBoard 源码分析](附录2-thingsboard规则引擎与CF实现分析.md) |

  ## 文档权威层级

  [主规格](场景联动 Runtime 主规格总纲.md)是 scene-linkage 范围内唯一的规范性
  架构决策源，负责 what、why 和架构不变量。三份配套文档是从属实施契约；导读只负责解释，
  两份附录只提供非规范性研究。发生冲突时不能按阅读顺序覆盖，必须联合修订受影响的规范。
  ```

  Do not copy the guide's sequence, status table, source table, or failure table into README.

- [ ] **Step 3: Keep the existing status and cross-directory boundary without duplicating architecture prose**

  The final README must still say:

  - core方案已确认，但恢复 ADR、POC 和全文冻结未完成；
  - 父架构 owns platform/product boundaries；Kafka 设计 owns non-Scene physical contracts；Scene main spec owns Scene Runtime decisions；
  - cross-document conflicts require synchronized revision and cannot be resolved by later wording alone.

- [ ] **Step 4: Verify README length and both routes**

  Run:

  ```bash
  readme='亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'

  test "$(wc -l < "$readme")" -le 80
  test "$(rg -c '^## 第一次阅读$' "$readme")" -eq 1
  test "$(rg -c '^## 按问题查阅$' "$readme")" -eq 1
  test "$(rg -c '^## 文档权威层级$' "$readme")" -eq 1
  test "$(rg -c '00-15分钟读懂场景联动\.md' "$readme")" -eq 1
  test "$(rg -c '唯一的规范性' "$readme")" -eq 1
  test "$(rg -c '架构决策源' "$readme")" -ge 1
  ```

  Expected: exit 0.

- [ ] **Step 5: Commit the first-layer router**

  ```bash
  git add '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'
  git commit -m 'docs: route scene readers through the quick-start guide'
  ```

### Task 6: Add reading cards to the main and control-plane specifications

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md`

- [ ] **Step 1: Add the main-spec reading card after its metadata block**

  Insert before `## 1. 目标与非目标`:

  ```markdown
  > **阅读卡**
  >
  > - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)，无需顺序通读本文。
  > - 本文回答：Scene Runtime 的架构决策、数据流、状态、恢复、事务、分区与容量边界。
  > - 本文不回答：Raw/Canonical 的非 Scene 物理契约，以及字段/算子/SLO 的从属实施细节。
  > - 快速跳转：[总体数据流](#4-总体数据流) · [`scene_event`](#5-scene_event-合约) · [`scene_state`](#8-scene_state-逻辑与物理模型) · [Timer](#10-timer-与-delayed-action) · [Outbox](#12-outbox-与动作幂等) · [Kafka 失败窗口](#13-kafka-交付事务与失败窗口)
  > - 权威关系：本文仍是本目录唯一规范性架构决策源；导读和三份配套文档不得覆盖本文。
  ```

- [ ] **Step 2: Add the control-plane reading card after its metadata block**

  Insert before `## 1. 控制面实体与稳定身份`:

  ```markdown
  > **阅读卡**
  >
  > - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)。
  > - 本文回答：SceneBinding、Scope、SubjectContext、ProjectIndex、matching、fanout 和发布期容量门槛。
  > - 本文不回答：算子 FSM、timer、Current Fact、Outbox 事务、恢复和 Kafka 物理拓扑。
  > - 快速跳转：[控制面实体](#1-控制面实体与稳定身份) · [Scope](#2-组织轴scope-与当前-subjectcontext) · [ProjectIndex](#3-pg控制流与-projectindex) · [matching](#4-冻结的-matching-管线语义) · [路由边界](#5-物理路由边界)
  > - 权威关系：本文是主规格边界内的从属实施契约；冲突时必须回到主规格联合修订。
  ```

- [ ] **Step 3: Verify both cards are short and unique**

  Run:

  ```bash
  main='亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'
  control='亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md'

  test "$(rg -c '^> \*\*阅读卡\*\*$' "$main")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$control")" -eq 1
  test "$(rg -c '00-15分钟读懂场景联动\.md' "$main")" -eq 1
  test "$(rg -c '00-15分钟读懂场景联动\.md' "$control")" -eq 1
  ```

  Expected: exit 0. Each card must stay below 15 Markdown lines.

- [ ] **Step 4: Commit the two reading cards**

  ```bash
  git add \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md'
  git commit -m 'docs: add scene architecture reading cards'
  ```

### Task 7: Add reading cards to the operator and SLO specifications

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md`
- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

- [ ] **Step 1: Add the operator/terminology reading card before `## 1. 适用范围`**

  ```markdown
  > **阅读卡**
  >
  > - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)。
  > - 本文回答：Reducer、ComputationPlan、事实身份、Current Fact、更新排序、算子 FSM 和 timer 语义。
  > - 本文不回答：Runtime 数量、Topic/分区物理配置、控制面 matching 和 SLO/rollout 决策。
  > - 快速跳转：[共享术语](#2-共享术语) · [统一求值](#5-统一求值顺序) · [duration](#8-duration--for) · [普通 timer](#13-普通-timer-实施契约) · [Fact 投影](#15-canonicalscene-fact-投影契约术语)
  > - 权威关系：本文是主规格边界内的从属实施契约；身份补充仍处于其文首声明的联合修订状态。
  ```

- [ ] **Step 2: Add the SLO/implementation reading card before `## 1. 文档职责与判定语言`**

  ```markdown
  > **阅读卡**
  >
  > - 第一次阅读：先看[《15 分钟读懂场景联动》](00-15分钟读懂场景联动.md)。
  > - 本文回答：冻结约束、POC 数值、故障窗口、实施工件、rollout、准入检查和 RACI。
  > - 本文不回答：完整数据流、Binding/matching 规则、算子 FSM 或字段级 wire 定义。
  > - 快速跳转：[约束分类](#2-约束与数值的三类清单) · [故障矩阵](#6-可审计故障窗口矩阵) · [实施工件](#7-实施与交付工件) · [Rollout](#8-rollout-顺序与-gate) · [RACI](#10-owner--raci)
  > - 权威关系：本文是主规格边界内的从属验收契约；恢复 ADR 仍须完成证据与签字后另行冻结。
  ```

- [ ] **Step 3: Verify both cards are unique and preserve status warnings**

  Run:

  ```bash
  terms='亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'
  slo='亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'

  test "$(rg -c '^> \*\*阅读卡\*\*$' "$terms")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$slo")" -eq 1
  test "$(rg -c '00-15分钟读懂场景联动\.md' "$terms")" -ge 1
  test "$(rg -c '00-15分钟读懂场景联动\.md' "$slo")" -eq 1
  test "$(rg -c '恢复 ADR 状态.*未冻结' "$slo")" -eq 1
  ```

  Expected: exit 0.

- [ ] **Step 4: Commit the two reading cards**

  ```bash
  git add \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
  git commit -m 'docs: add scene implementation reading cards'
  ```

### Task 8: Run full mechanical verification and the fresh-reader test

**Files:**

- Verify: `亿级点位高吞吐架构/6.规则引擎与场景联动/README.md`
- Verify: `亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md`
- Verify: all four normative Markdown files modified above
- Do not touch: `docs/superpowers/specs/2026-08-04-scene-temperature-alarm-e2e-flow.html`
- Do not touch: `.superpowers/`

- [ ] **Step 1: Run line-budget, structure, and status checks**

  ```bash
  scene_dir='亿级点位高吞吐架构/6.规则引擎与场景联动'
  guide="$scene_dir/00-15分钟读懂场景联动.md"
  readme="$scene_dir/README.md"

  test "$(wc -l < "$readme")" -le 80
  test "$(wc -l < "$guide")" -le 250
  test "$(rg -c '^```mermaid$' "$guide")" -eq 1
  test "$(rg -c '^sequenceDiagram$' "$guide")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$scene_dir/场景联动 Runtime 主规格总纲.md")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$scene_dir/控制面与匹配契约.md")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$scene_dir/算子语义与术语.md")" -eq 1
  test "$(rg -c '^> \*\*阅读卡\*\*$' "$scene_dir/SLO故障测试与实施.md")" -eq 1

  test "$(rg -c '\[现有实现\]|\[目标设计\]|\[待 ADR\]' "$guide")" -ge 12

  if rg -n \
    'ONLINE_TO_OFFLINE|DISPATCHED_TO_RESULT_SUCCEEDED|computed-metric-topic-v1|accepted-transition-topic-v1' \
    "$guide"
  then
    exit 1
  fi
  ```

  Expected: exit 0, with no invented transition/topic name.

- [ ] **Step 2: Verify all required authority headings still exist**

  ```bash
  rg -n '^## 4\. 总体数据流$|^## 8\. `scene_state` 逻辑与物理模型$|^## 12\. Outbox 与动作幂等$|^## 13\. Kafka 交付、事务与失败窗口$' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md'

  rg -n '^## 2\. 共享术语$|^## 13\. 普通 timer 实施契约$|^## 15\. Canonical→Scene Fact 投影契约术语$' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md'

  rg -n '^## 4\. 外部效果承诺边界$|^## 6\. 可审计故障窗口矩阵$|^## 10\. Owner / RACI$' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
  ```

  Expected: all linked authority headings are present.

- [ ] **Step 3: Validate Markdown links**

  Run:

  ```bash
  npx --yes markdown-link-check@3.13.7 \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md'

  npx --yes markdown-link-check@3.13.7 \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  ```

  Expected: both commands report 0 dead links. If the checker does not validate a Unicode heading fragment, confirm the target heading with Step 2 and keep the repository-relative anchor rather than replacing it with a line-number link.

- [ ] **Step 4: Parse and render the final sequence diagram**

  Run this check serially. Do not start another Mermaid CLI invocation in parallel because first-use Puppeteer
  installation shares one executable cache.

  ```bash
  guide='亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md'
  verify_dir=$(mktemp -d)

  awk '
    /^```mermaid$/ { capture=1; next }
    capture && /^```$/ { exit }
    capture { print }
  ' "$guide" > "$verify_dir/scene-e2e.mmd"

  npx --yes @mermaid-js/mermaid-cli@11.4.2 \
    -p <(node -e 'process.stdout.write(JSON.stringify({args:["--no-sandbox","--disable-setuid-sandbox"]}))') \
    -i "$verify_dir/scene-e2e.mmd" \
    -o "$verify_dir/scene-e2e.svg"

  test -s "$verify_dir/scene-e2e.svg"
  ```

  Expected: exit 0 and a non-empty temporary SVG. Do not add the SVG to Git.

- [ ] **Step 5: Run whitespace, fence, diff-scope, and dirty-worktree checks**

  ```bash
  git diff --check -- \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'

  for doc in \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
  do
    fence_count=$(rg -c '^```' "$doc")
    test $((fence_count % 2)) -eq 0
  done

  git status --short
  ```

  Expected: `git diff --check` and all fence assertions exit 0. `git status --short` may still show the user's pre-existing modified HTML and `.superpowers/`; neither may appear in any implementation commit.

- [ ] **Step 6: Conduct the 15-minute fresh-reader test**

  Give a reviewer who did not implement these changes only these two files:

  ```text
  亿级点位高吞吐架构/6.规则引擎与场景联动/README.md
  亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md
  ```

  Limit reading to 15 minutes, then ask exactly:

  1. 四类 Scene 事实来源是什么？
  2. 为什么 CommandReply 不能直接触发 Scene？
  3. Reducer 写什么状态，什么时候才发布 AcceptedStateTransition？
  4. ComputationPlan 在哪里生成、哪里执行？
  5. 为什么 Canonical 或 ComputedMetricUpdated 会被多个 Consumer Group 消费？
  6. Current Fact、SceneBindingState、CommandExecution/Attempt 分别是什么？
  7. Kafka 重投为什么不会自然等同于重复业务动作？
  8. 哪个事务提交后才算 action acceptance？
  9. 当前 `things` 仓库是否已经实现 Flink、Reducer 和 Scene Runtime？

  Expected: at least 8/9 correct, and question 9 must explicitly answer “没有，图中的这些角色是目标设计/待 ADR，而不是当前实现”。If the test fails, revise ordering or plain-language explanations without adding full contract tables, then rerun Steps 1–6.

- [ ] **Step 7: Commit only verification-driven corrections**

  If Steps 1–6 required edits, stage only the six documentation files in this plan and commit:

  ```bash
  git add \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/README.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/00-15分钟读懂场景联动.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/控制面与匹配契约.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/算子语义与术语.md' \
    '亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md'
  git commit -m 'docs: fix scene reading validation findings'
  ```

  If no file changed, do not create an empty commit.

## Final handoff checklist

- [ ] README is no more than 80 lines and points first-time readers only to the quick-start guide.
- [ ] The guide is no more than 250 lines and contains exactly one parseable Mermaid `sequenceDiagram`.
- [ ] The one R1 multi-device scenario is used throughout; no competing example is introduced.
- [ ] Every program node is correctly labeled from repository evidence.
- [ ] Every Kafka interaction names the Topic/message and consumer group, or visibly says `[待 ADR]`.
- [ ] Current Fact, SceneBindingState, CommandExecution/Attempt, and their unique writers are distinct.
- [ ] Action acceptance is tied to PostgreSQL Action Outbox commit, not timer fire or device reply.
- [ ] All key conclusions link to an existing authority heading.
- [ ] The four long specifications each have one reading card shorter than 15 lines.
- [ ] No existing document was moved, renamed, deleted, or stripped of authority/status warnings.
- [ ] The user's modified HTML and `.superpowers/` remain untouched and uncommitted.
- [ ] Mechanical verification and the 15-minute fresh-reader test pass.
