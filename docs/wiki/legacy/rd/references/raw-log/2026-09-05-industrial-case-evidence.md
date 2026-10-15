# 工业与车联网部署：Raw Log 证据边界

研究日期：2026-09-05（Asia/Shanghai）。公开检索约六分钟；仅使用一手资料，按材料发表时的部署状态判断，不推定其代表当前架构。未研究 Cognite/Aker BP、Netflix、DoorDash、Uber。

以下三个案例证明具名企业实际采用的数据路径，不构成三个“协议解码前全量 Raw Log”成功案例。这里区分车载／工业协议解码、消息 schema 统一和分析转换；材料中的 raw、Decoder、数据湖均不自动等于原协议字节。未公开表示本次所用来源未披露。

## 1. BMW：客户车辆自动驾驶数据分析

来源：[BMW 客户车辆数据技术文](https://aws.amazon.com/blogs/industries/scaling-autonomous-driving-data-processing-and-data-management-with-bmw-group-on-aws/)。2023-09-12；BMW 工程负责人和 AWS 联合署名，供应商网站刊载的一手技术文。定位：Collection、Data privacy、Solution overview。

- **实际路径**：客户车时序／事件数据 → OTA → 采集平台转换 → CDH 的 S3/Parquet → EMR 作业 → S3 与分析工具。
- **持久化边界**：CDH 落盘在格式转换后；相对车载协议解码的位置、此前是否另存原报文，未知。
- **原文动因**：自动驾驶开发验证、数据增长及团队自主。**推断**：支持分析复用，未证明原字节重放需求。
- **代价／反例**：受必要采集和匿名化约束；原始传感器采集当时仍属下一步。Raw 专属开销未公开。
- **保留／完整性**：期限未公开；仅述行程首尾检查，全量报文及解析失败覆盖保证未公开。

## 2. Scania：生产中的车辆数据预处理流水线

来源：[Pei Zhang：车辆流处理流水线性能研究](https://aaltodoc.aalto.fi/server/api/core/bitstreams/0a1120db-6907-403e-bc7f-441b1636ad44/content)。2019-07-24；在 Scania 完成的 Aalto 硕士论文，现场参与者一手研究，非公司正式博客。定位：致谢、§3.1、§3.4（第26–27页）、§5.4（第40页）。PDF 间歇403；身份经正文、章节经搜索索引核验。

- **实际路径**：车辆消息 → Kinesis Data Streams → Lambda 的 Decoder、Enricher、Cleaner、Normalizer；阶段间亦有流。原文明示已生产运行。
- **持久化边界**：入口流在车型 schema 统一前；相对原协议解码的持久化位置、长期 Raw 归档未知。
- **原文动因**：提供干净统一的数据；解码前留存动因未公开。**推断**：阶段间流可解耦。
- **代价／反例**：清洗丢弃重复、缺时间戳及过迟消息；批大小和轮询影响延迟。Flink 替代仅属研究。
- **保留／完整性**：部署期限、入站完整性及失败报文归档未公开。

## 3. Siemens：SIMATIC 电子工厂 PCB 切割主轴预测维护

来源：[Gunter Beitinger：Lean Digital Factory 第三章](https://blog.siemens.com/2021/06/lean-digital-factory-a-blueprint-for-digitalization-of-factories-chapter-3-workstreams-as-enablers/)。2021-06-14；Siemens 自述自身工厂实施，兼为方案供应商。定位：Big Data and Analytics。厂址未明确，不能擅称 Amberg；网页需 JavaScript，正文经搜索索引核验。

- **实际路径**：机器数据 → Edge 采集 → MindSphere 训练 → 模型下载至 Edge → 转速／电流本地推理；已装备18个主轴。
- **持久化边界**：总架构含 MDP raw store；该工序存储对象及协议解码先后未知，不能套用总架构补齐。
- **原文动因**：利用云算力训练、提前安排维护。**推断**：训练留数与在线推理可分开。
- **代价／反例**：边缘执行不证明持续全量上传。原文承认故障换件、调参负担；Raw 开销未公开。
- **保留／完整性**：期限、持续覆盖及失败报文留存未公开。维护收益不能归因 Raw 层。

## Grilling 审视

按 grilling 的证据审视方式，当前决策树停在三个待证明的分支，而不是让品牌案例替项目作决定：

1. **重算什么？** 分析转换重算、schema 迁移、协议解码纠错需要的输入不同；须先绑定 ming-iot 的具体故障与消费者。
2. **完整到哪里？** 成功接纳消息、解析失败载荷、原协议帧和未接纳流量不是同一范围；这三例未证明最后两项。
3. **收益由谁产生？** 云迁移、查询优化、算法预测维护的收益，不能算作 Raw 层收益；还需项目自己的故障恢复证据与成本测量。

最重要的反直觉发现：公开架构中的“解码前”可能只是业务 schema 统一前；即使接入了流系统，后续仍可能主动丢弃数据。因此，案例支持的是按用途选择留存边界，尚不足以支持默认全量原帧归档。
