---
status: draft
version: 0.2.0-draft
---

# protocol：协议适配能力域

本域将客户设备协议转换为平台可理解的标准消息，集中维护协议解释、适配版本和转换结果的知识。按[团队划分标准](../../runbook/team-development.md#domain-feature-service)组织；这是知识与职责边界，不表示已经形成独立部署或完整 DDD 限界上下文。

## 职责与相邻边界

| 责任 | 归属与协作 |
| --- | --- |
| 客户 payload 的协议解释、标准化、适配版本选择与转换结果 | 本域；本轮具体规则只在[上行闭环规格](uplink-normalization/spec.md)维护 |
| 连接、认证、可信设备绑定、接入应答 | Access Adapter / 接入实现；向本域提供可信上下文，双方共同明确接受时序 |
| 跨服务信封、传输、消费完成及隔离机制 | [Eventbus](../platform/eventbus/README.md)；本域定义标准消息的业务含义，公共格式和提交接口共同评审 |
| 设备领域权限与归属、模型处理、业务落库、去重及副作用安排 | DM；本域不以 Codec 替代这些责任，最终业务结果用于上行闭环验收 |

## 能力地图

| 能力 | 入口与当前状态 |
| --- | --- |
| 上行解码、标准化与结果交付 | [protocol.uplink-normalization](uplink-normalization/README.md)：0.2.0-draft 已细化契约、故障与验收，PA-Q4–Q6 已确认；尚未实施新链路 |
| 适配器版本与发布管理 | 规划能力，暂未单独建 Feature；首版上行所需的版本约束先在上行规格维护，出现独立工作流与验收后再提取 |
| 下行编码 | 后续能力，未建规格；上行中的命令回执仍属于当前上行范围 |

## 与 ProtocolApplication 服务的关系

`ProtocolApplication` 是承载协议适配职责的应用实现名称，服务化方案可覆盖本域多个 Feature，不作为业务域与 Feature 的同义词。当前只确认职责分离；独立进程、部署单元、扩缩容、故障预算及与 Access Adapter 的调用方式仍待设计。客户定制或发布版本不同，不自动要求每客户一个服务或一个 Feature。

本域长期负责人未分配。当前用户确认需求与文档组织范围，`rdoc-pilot-codex` 在同一 `ming-iot-knowledge-pilot` 任务中起草，独立 AI 复核本地候选；这不替代人的长期责任。

## 来源与维护入口

原始需求与决定保留在 [Eventbus raw](../platform/eventbus/raw.md#raw-016)及 [Kafka Patch R13](../platform/eventbus/patches/PATCH-20261017-kafka-migration/change.md#protocol-domain-extraction)。Patch 第 11 节保留 R10–R12 的历史评估；本域当前设计从上述 Feature 入口维护，旧评估不会因保留而成为第二份当前规格。
