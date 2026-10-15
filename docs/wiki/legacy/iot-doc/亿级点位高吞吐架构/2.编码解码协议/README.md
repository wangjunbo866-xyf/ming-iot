# 编码解码协议（ProtocolApplication 域）

> 本目录覆盖 Raw→Canonical 的契约层与各协议 codec 的高性能实现。
> 正式定义以[父架构](../enterprise-iot-platform-architecture-design.md) §7 为准。

## 按问题查阅

| 我正在解决的问题 | 直接阅读 |
| --- | --- |
| Raw/Canonical 的契约语义、成本与分期、故障故事、主流对照（CDC/Kafka Streams/DLQ） | [原始流与标准流](./原始流与标准流/README.md) |
| 数据质量码（availability/quality）：Canonical 状态词汇、无码协议默认 GOOD、行业映射、TDengine 总码+例外明细落地 | [质量码契约设计](./质量码契约设计.md) |
| 多点位二进制高性能链路（内存优化、typed arena、SQL 批写、事件 schema 缓存、typed decode） | [large-binary-codec/](./large-binary-codec/) |
| 低点位设备通用 JSON codec（schema 驱动零分配提取、三层批策略） | [small-json-codec/](./small-json-codec/) |

## 边界

- 质量码规则的**所有权**在发布物（`availabilityPolicyRef` 等），**执行点**在
  ProtocolApplication；codec 系列文档管"怎么解"，[质量码契约设计](./质量码契约设计.md)管
  "解出来的状态是什么"；
- Scene 侧的 DIRECT_TD 读取档位以[质量码契约设计](./质量码契约设计.md) §5/§7 为硬前提，
  见[《Scene Current Fact 设计》](../6.规则引擎与场景联动/Scene%20current%20fact设计.md) §9。
