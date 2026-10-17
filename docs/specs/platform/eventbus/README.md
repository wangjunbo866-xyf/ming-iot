# platform.eventbus：跨服务事件总线

- 当前正文：[spec.md@p1-v1](spec.md)。完整描述本地 P1 显式 Kafka 接入能力，同时保留 observed-v0 的旧 NATS 路径；不表示全面切换或生产生效。
- 来源：[raw.md](raw.md)；用户确认的 D1–D15 继续适用，RAW-011 授权 P1，RAW-012 委托 ProtocolApplication 设计评估，RAW-013 确认首版上行及受控发布；RAW-014 选择先解码再持久化标准消息，暂不承诺原报文补算。
- 在途变更：[PATCH-20261017-kafka-migration / R12](patches/PATCH-20261017-kafka-migration/change.md)。第 10 节保留 P1 认领和验收；[第 11 节](patches/PATCH-20261017-kafka-migration/change.md#protocol-application-assessment)评估 ProtocolApplication 与 P2 前置依赖，PA-Q1–PA-Q3 已确认，具体契约待细化。P2–P6 仍是候选，不整体批准或吸收。
- 长期模块负责人未分配；需求负责人为用户，任务协调实施者为 `rdoc-pilot-codex`，独立 AI 负责最终候选复审，不冒充真人批准。
- 模块边界：Share 契约、跨服务传输与缓存及 SaaS/Things/SDK 调用方；本轮实际写范围以 Patch 认领为准，前台和旧服务配置不变。
- 差异导航：[specs-map.json](../../../../tooling/specs-map.json)登记本模块；[v2 检查投影](patches/PATCH-20261017-kafka-migration/check.json)只声明 P1 写范围和确认来源，不能用它给后续业务切换授权。

<a id="kafka-patch"></a>
当前主规格的 BUS-P1-CONTRACT、BUS-P1-COMPLETION、BUS-P1-CACHE、BUS-P1-VERIFY 回链到 [同一 Patch](patches/PATCH-20261017-kafka-migration/change.md)。复验通过入口找到实际代码、参数和证据；不需要另建交接包。
