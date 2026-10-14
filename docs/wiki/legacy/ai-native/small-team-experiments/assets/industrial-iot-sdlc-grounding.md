# 工业 IoT 的知识与上下文演进：聚焦证据

核查日期：2026-10-11。目标是减少同一业务的理解、规则、配置与文档漂移；不比较星数或通用 Skills。
仅检查公开源码、官方文档与真实变更；没有执行第三方仓库代码，没有修改远端。以下证据不能证明项目已采用 AI Native，也不能证明所有文档始终同步。

## 快照与证据边界

| 范围 | 固定快照／版本 |
|---|---|
| `edgexfoundry/edgex-go` | `23a76301aafb3d7a0d03c544dabb5a9a31f71555` |
| `edgexfoundry/edgex-docs` | `be375ae546643dba7a07f44693c4bc6723621136` |
| `thingsboard/thingsboard` | `682333ee4ecf7a7a7df18579aaec6803f99374c7` |
| EdgeX 发布文档 | 官方 `4.1` 文档；不假设等同于当前 main |

三个仓库均成功读取完整 recursive Git tree（`truncated=false`）；后续有个别 API 连接重置。未读取受保护分支设置，不把 YAML 存在等同于必需合并检查已启用。

## EdgeX：让设备数据的含义成为共享契约

- 发布文档按服务组织 Purpose、Configuration、API Reference、Source Code，再细分设备 Profile、事件与读数、迁移等主题；它提供领域概念到实现的导航，而不是把所有上下文放入一个 Spec。[4.1 服务与设计入口](https://docs.edgexfoundry.org/4.1/design/Process/)
- Device Profile 描述设备资源的 `valueType`、`units`、读写能力、范围与转换；SDK 按 `base → scale → offset → mask → shift` 处理。4.1 文档还明确：首次加载后 Profile 存在 Core Metadata，后续启动从那里读取；仅修改本地 Profile 文件不会改变生效配置。[设备 Profile](https://docs.edgexfoundry.org/4.1/microservices/device/details/DeviceProfiles/)
- 因此需要区分“版本控制中的预期契约”和“某环境实际加载的契约”。业务地图应链接契约及生效范围，验收记录应说明运行版本与配置；这条建议是本次推导，不是 EdgeX 原文规定。

### 设计演进已有真实记录

- `docs_src/design/TOC.md` 索引实际 UCR 与 ADR，例如设备父子关系、数据扩展、保留策略、Profile 修改规则、单位、公共配置。[固定目录](https://github.com/edgexfoundry/edgex-docs/blob/be375ae546643dba7a07f44693c4bc6723621136/docs_src/design/TOC.md)
- 官方流程区分小修复与较大需求；后者使用 UCR，重要跨服务设计用 ADR，并保留设计决策到 Issue／PR 的关联。UCR、设计分别有评审责任与状态，试验可能促使设计修订或废弃；这些是公开治理规定，不是对每次执行的审计。[流程源码](https://github.com/edgexfoundry/edgex-docs/blob/be375ae546643dba7a07f44693c4bc6723621136/docs_src/design/Process.md)
- **ADR 0021** 记录 Profile 已被设备／Provision Watcher 引用时的修改约束，并要求 Reading 携带 Units，使 Profile 演进不必重新解释旧读数；另有严格修改／删除的配置选择。[0021-Device-Profile-Changes.md](https://github.com/edgexfoundry/edgex-docs/blob/be375ae546643dba7a07f44693c4bc6723621136/docs_src/design/adr/core/0021-Device-Profile-Changes.md)
- **ADR 0022** 记录从任意单位字符串走向可配置单位集合与 Profile 创建／修改时校验的设计；为兼容旧部署，校验默认关闭。它还区分用于说明来源的 `Source` 与实际参与校验的单位列表。[0022-UoM.md](https://github.com/edgexfoundry/edgex-docs/blob/be375ae546643dba7a07f44693c4bc6723621136/docs_src/design/adr/core/0022-UoM.md)
- 上述两个 ADR 是 2022 年获批的历史设计记录；本次未逐项验证其全部规则在 4.1 的实现及具体部署开关。不要把“Approved”当成“当前环境已执行”。

### 一个真实 Patch 链：温度 50 被读成 5

- [Issue #5251](https://github.com/edgexfoundry/edgex-go/issues/5251) 指出 Core Data 在 4.1 中设置温度 `50`，查询返回 `5`，并给出请求、响应及版本；这是明确的跨写入／查询语义回归。
- [PR #5252](https://github.com/edgexfoundry/edgex-go/pull/5252) 关联该 Issue，2025-09-15 获另一维护者批准并合并。页面显示七项检查通过，但这不能推出仓库所有分支保护设置。
- [固定代码提交](https://github.com/edgexfoundry/edgex-go/commit/4dd75c8bf9e79b0b9b6b24f811fc87d5650be400) 修正 PostgreSQL Numeric 转换，并新增／更新 `internal/pkg/infrastructure/postgres/reading_test.go`、`readingagg_test.go`，覆盖指数、整数边界及非法值；本次只检查源码，没有重跑。
- PR 的文档变更项未勾选，Testing Instructions 为空；不能补造“完整文档闭环”。它证明的是原始行为缺陷可以经 Issue、代码、回归测试和独立审查追溯，也提示模板本身不能保证证据完整。

## ThingsBoard：共享遥测语义，配置说明贴近生效来源

- 官方时间序列页统一说明 key／value／timestamp：支持整数、double 等类型，时间戳为毫秒，缺省由服务端补齐；不同接入协议映射到统一内部表示，并链接保留策略、规则引擎、告警及查询。[时间序列领域文档](https://thingsboard.io/docs/user-guide/digital-twins/time-series-data/)
- PR 模板要求关联 Issue、说明文档影响、保持兼容或提供升级脚本；新 REST API 要同步 Java client 并为 Python client 建 Issue，新 YAML 属性应就近添加说明。这是跨产物影响检查，并不要求统一个人实现步骤。[固定 PR 模板](https://github.com/thingsboard/thingsboard/blob/682333ee4ecf7a7a7df18579aaec6803f99374c7/pull_request_template.md)
- `.github/workflows/check-configuration-files.yml` 对主服务及 HTTP／MQTT／CoAP／LwM2M／SNMP 等相关 YAML 变化调用检查脚本。[固定 workflow](https://github.com/thingsboard/thingsboard/blob/682333ee4ecf7a7a7df18579aaec6803f99374c7/.github/workflows/check-configuration-files.yml)
- `tools/src/main/python/check_yml_file.py` 从配置附近提取属性、环境变量、默认值与注释；描述为空或少于五字符时报告问题并最终退出失败。它检查说明是否存在，**不检查业务语义是否正确，也不证明网站文档同步**。[固定脚本](https://github.com/thingsboard/thingsboard/blob/682333ee4ecf7a7a7df18579aaec6803f99374c7/tools/src/main/python/check_yml_file.py)
- [PR #15560](https://github.com/thingsboard/thingsboard/pull/15560) 是另一条真实语义修复：负数 long／double 混合遥测的 MAX 返回极小正数。修改 SQL、Cassandra 聚合与 TBEL rollingArg，并增加负数回归测试；已在 2026-05-05 合并。[固定合并版本](https://github.com/thingsboard/thingsboard/commit/28b376aa3ca02c037bf6e0203961d21b3020ebf3)
- 该 PR 正文没有可确认的具体关联 Issue 号；本次未补全其 Issue 与文档 PR 链，也未确认最终 CI 全部日志。不能由它推断团队所有需求都遵循完整固定流程。

## 本地案例：OTA 与物模型的 ModuleCode 消歧

- [2026-09-05 实施规格](/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/specs/2026-09-05-ota-model-module-code-disambiguation-activation-design.md) 记录两个不同领域共用 `ModuleCode` 的歧义，以及 OTA HTTP 字段为 `int64`、Proto／PostgreSQL 为 `string` 的不一致。目标词汇拆为 `FirmwareModuleCode` 与 `ModelModuleCode`，并给出各接口表面的名称和类型。
- 规格同时限定“一次性消歧”依赖首次发布前、没有旧客户端／固件／生产数据等前提；前提改变就必须重新设计兼容与迁移，不能把该历史授权套到另一环境。
- 当前[静态契约测试](/home/wangjunbo/workspace-iot-platform/ming-iot/backend/tools/internal/repositorycontract/module_code_disambiguation_contract_test.go) 分 OTA、Model、CrossDomain，检查字段类型、JSON 名称、Proto 字段号、数据库 tag、默认值及禁止混用的跨域标识；它把共同词汇的一部分变成可重复检查的底线。
- 本轮根 Agent 已在产品 HEAD `6742201e9666fd3b3067216b2efbb3b8ba7338d7` 执行：`go -C ming-iot/backend test ./tools/internal/repositorycontract -run '^TestModuleCodeDisambiguation$' -count=1 -v`。退出码 0；OTA／Model／CrossDomain 三子项 PASS；包耗时 0.521s。此前 tracked 工作树 clean，本轮未修改产品代码。
- 这次只证明测试所列静态源码契约；没有运行数据库、设备或端到端验证，不证明全部业务语义或线上生效。历史 2026-09-05 Review 的全量 PASS **不是本轮验证结果**，不得挪用。

## 可用于我们试点的机制（建议，非外部项目事实）

1. 同一业务规则有明确的权威位置：单位、精度、时间语义、标识符含义及兼容约束，优先保存在共享模型／协议及最贴近它的说明中；业务地图负责导航、责任、规则关系。
2. 可执行的契约不在多个 Markdown 重抄；地图链接模型、配置、代码和回归测试。争议与取舍记录为 ADR；每次交付只记录差异及验收条件。
3. 改单位、类型、默认值、标识符或历史数据解释时，检查生产端、存储、消费端、SDK 与迁移；明确何时生效，不能只改文字。
4. 能机械检查的底线进入 CI，例如配置注释、Schema 兼容、引用有效性和关键跨域类型契约；“有说明”与“语义正确”分别由自动检查和责任人审查负责。
5. 成员可选择个人 Skill、Plan 和执行方式；团队共享的是规则、证据及变更批准。以上案例只提供工业领域治理依据，不能证明个人方法自由必然提高效能。

## 限制

这是两个工业 IoT 项目的窄样本，不是完善 SDLC 的效果评估。未核实部署状态、全部文档新鲜度、私有设计讨论或分支保护。EdgeX README 中仍有旧工具版本文字，因此没有把它当作当前运行环境依据。

## 当前 map-delivery 的定位与空缺（本轮本地读取）

- [现有 Skill](/home/wangjunbo/workspace-iot-platform/ai-native/skills/map-delivery/SKILL.md:12)主要组织权威入口、Issue、Change、Plan、Review、Evidence、双向链接和激活；它已有候选／现行区分及锚点冲突处理，不能笼统说它没有治理。
- [map 入口](/home/wangjunbo/workspace-iot-platform/ming-iot-rd-docs/map/README.md:5)仍标注设计试点、旧顶层交付目录继续权威；本轮可见 map 文件集中在“研发治理／文档管理”。这不等于真实 IoT 领域知识已迁入地图，也不是对远端合并或部署状态的新核查。
- ModuleCode 的业务含义能从现有实施规格和代码契约测试恢复，但本轮在 map、ADR、产品根 AGENTS 与 Things Markdown 中未检索到这两个明确术语的知识入口。该结果仅限所查范围，不声称整个仓库不存在其他说明。
- 因此应先验证一条真实业务规则在跨会话、跨成员和持续变更中能否被正确恢复，再决定是否更换目录或拆分 Skills。文件齐全及静态检查通过不能单独证明语义一致。

## 一条规则的最小试点草案（未实施，未成为团队规则）

目标：两位成员可采用不同实现工具，但在两次连续变更及新 AI 会话接手后，仍能准确说清 OTA 固件模块编码与物模型模块编码的区别、适用范围和变更依据。

| 内容 | 建议权威位置／责任 | 最小维护动作 |
|---|---|---|
| 当前业务含义与约束 | 研发文档仓的功能入口；业务责任人 P 确认，交付者 D 维护 | 从入口连接现有规格、源定义和测试；不复制所有字段定义 |
| 精确字段、类型和可执行规则 | 产品仓 `.proto`／`.api`／模型及测试 | 在所属权威源修改并生成；受影响消费者一起验证 |
| SOP | 团队 ai-native 仓的版本化流程正文，候选位置 `sop/` | 共享默认方法和必要交接点；普通 Patch 不强制另写 Plan |
| Skills／AGENTS | Skills 封装任务入口，AGENTS 指向适用 SOP 与知识入口 | 引用正文，避免再抄一套业务规则 |
| 必须满足的结果 | 团队 rules 与产品测试／CI／独立 Review | 个人方法可替换；共同契约和验收要求不能被个人配置改写 |

默认 SOP 只需要五个动作：

1. 找到本次功能的术语、规则、源定义、已知限制和回归测试；核对目标软件、设备／固件、协议及配置版本。依据版本可自动汇入 PR，不另建手工维护的上下文台账。
2. 区分“实现偏离既有规则”与“要求改变规则”。前者复用既有验收；后者在 Issue 或候选规格明确前后差异，由业务责任人确认。聊天、个人笔记和 RAG 召回结果不自行成为批准依据。
3. 成员选择自己的实现方法，补与业务语义有关的验证；个人 Plan、工具编排及临时思路可留在个人工作区。长程任务才使用 LoopX。
4. PR 明确本次依赖的规则入口及修订、规则是否改变、验证结果、知识需要更新的部分。相关规则在并行变更中发生变化时，重新核对该部分及受影响测试；其他无关文档变化不要求全量返工。
5. D 更新本次改变的知识，P 确认语义，R 独立检查代码与证据。已批准规则、已实现版本及环境实际生效状态分别记录在其已有权威位置；未发布变化不伪装成线上事实。

建议的试点验收（本轮尚未执行）：

- 新会话不读旧聊天，可从业务入口找到规则、实现与验证，并回答两个编码能否互换及为什么。
- 在隔离候选中恢复无修饰 `moduleCode` 或错误类型，聚焦契约检查能报错；本轮仅重跑正确基线，尚未做错误注入。
- 第二次变更沿用旧认知时，能识别受影响规则已改变，而不是将过期方案重新写成现行知识；这是语义与版本核对，不能仅靠文档哈希证明。
- 两位成员用不同方法完成任务，交给 Reviewer 的业务意图、依据与证据仍可比较；有意修改规则时，测试预期与知识变更需独立审查，不能通过作者自行改测试获得假通过。

未来若加入知识检索服务，应保留来源、适用版本和批准状态；个人经验的共享沿用已确认的人工确认边界。检索索引可以重建，不能成为脱离源文件的第二套业务事实库。
