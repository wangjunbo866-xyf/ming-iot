---
status: draft
version: 0.1.0-draft
---

# protocol.uplink-normalization：设备上行解码与标准化闭环

- 所属能力域：[protocol / 协议适配](../README.md)。稳定 Feature ID 为 `protocol.uplink-normalization`；ProtocolApplication 是拟承载它的应用实现名称，部署方式尚未确定。
- 当前设计正文：[spec.md@0.1.0-draft](spec.md)。这是本 Feature 唯一的当前候选规格，尚无已实施生效版本；既有 DG/DM 解码现状与新目标分别列明。
- 本轮动作：受用户授权建立文档及规格入口，不实施新链路。长期 Feature 身份不决定本次变更类型；后续实施认领按实际新增能力或既有行为调整分类，不因新建目录自动认定 `type=feature`。
- 需求来源：[RAW-012–015](../../platform/eventbus/raw.md#raw-015)。D13–D15 的原始决定表及 D16 文档组织授权保留在 [Kafka Patch](../../platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md#protocol-domain-extraction)，不复制确认记录。
- 责任：需求负责人为用户；域与 Feature 长期负责人未分配。当前起草身份为 `rdoc-pilot-codex`，同一 Goal 的任务 `todo_5c930ff38c388b64f9fbbb5a` 负责文档及验证，独立 AI 复核候选。
- 本轮认领：团队约定、ming-specs、协议域与上行规格、Kafka 来源/依赖导航、总纲/索引及有界文档检查清单。产品实现、部署配置与旧环境数据不在写范围，保留旧仓和私有备份。
- 实现线索：[DG 设备上行](../../../../backend/things/service/dgsvr/internal/event/deviceSub/deviceSub.go)、[协议 SDK](../../../../backend/things/sdk/protocol/coreProtocol.go)、[DM 属性处理](../../../../backend/things/service/dmsvr/internal/event/deviceMsgEvent/thing.go)。这些是既有源码依据，不是新 PA 实现。
- 登记状态：已加入文档地图、来源索引和有界文档检查；尚未加入产品差异映射 `tooling/specs-map.json`。该映射应在实施认领时按真实改动范围补齐，并覆盖与 Eventbus/DM 的共享路径。

## 接手顺序与下一步

先读当前 spec 的已确认边界、候选条款和未决契约，再按需追溯 Kafka Patch 的来源。下一步从现有 DG 脚本及设备样本选一个真实协议，细化输入/输出、接受与应答时序、部分失败、去重与顺序，并确认服务部署选择。设计清楚不自动授予实施权限。

本轮通过标准是正文可定位、来源和依赖双向可追溯、状态及版本一致，独立接手能区分域/Feature/服务。相关检查为 `make knowledge-check` 和 Skill 校验；运行验收场景见 spec，全部保持未执行。文档检查或 AI 复审不等于设备、Kafka、DM 全链路验收。
