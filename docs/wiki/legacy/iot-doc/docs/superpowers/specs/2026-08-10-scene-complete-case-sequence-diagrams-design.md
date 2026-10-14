# 场景联动完整案例时序图重绘设计

## 目标

为“3 个温度点求 5 分钟平均值 + 门状态持续满足 + 启动两台风机并调用 Webhook”的完整场景联动案例创建一份可独立阅读的 HTML 时序图文档。解决现有 Mermaid 整图参与者过多、在 VS Code 和普通笔记本屏幕上文字过小的问题。

## 输出与边界

- 新增 `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动完整案例端到端时序图.html`。
- 不修改现有 `场景联动端到端时序图.html`。
- 不修改 `00-15分钟读懂场景联动.md`，新 HTML 以其中第 5 节的完整时序为事实来源。
- 使用原生 HTML、CSS 和内联 SVG，不依赖 Mermaid、外部字体、CDN 或本地服务。

## 阅读结构

页面采用“两层阅读”结构。

### 第一层：端到端总览

总览只表达六个阶段及其主要持久化边界：

1. 设备与接入；
2. Durable Canonical；
3. 指标计算和 Current Fact；
4. Scene 求值与持续条件；
5. 动作接受和命令投递；
6. 回复归并及状态变化重新入场。

总览不展开 Topic 名、consumer group、事务字段和失败分支，目的是让读者先建立完整链路。

### 第二层：五张阶段细图

1. **设备上报与 Durable Canonical**：温度和门状态并行上报，Access Adapter 写 Raw，ProtocolApplication 以 Kafka transaction 产生 Canonical 并提交 Raw offset。
2. **共享计算、投影与 Admission**：Canonical 被独立 consumer group 消费；Materializer 写 PROPERTY Current Fact；Flink 产生 `ComputedMetricUpdated`；Projector 写 COMPUTED_METRIC Current Fact；Admission 产生 `SourceDelta`。
3. **Scene 求值与持续复核**：Scene Runtime matching，按固定读屏障读取 Current Fact，满足条件后武装 2 分钟定时器；到期后重新读取并复核条件。
4. **动作接受与投递**：以 PostgreSQL transaction 同时写 EffectSetReceipt、两条 DeviceCommand 和 Webhook；提交成功后 Command Runtime 按 effectId 幂等创建并投递命令。
5. **回复归并与状态联动**：设备回复重新经过 Raw 和 Canonical；Reducer 将 duplicate、stale、conflict、unmatched 与合法单调迁移分开处理；合法迁移通过 Outbox 发布 `AcceptedStateTransition`，再由 Admission 规范化为 SceneEvent。

## 可读性约束

- 每张细图最多 7 条生命线；跨阶段组件只在需要时重复出现。
- 正文消息字号不小于 15px，参与者标题不小于 16px，辅助说明不小于 13px。
- 消息标签原则上不超过两行；协议字段过多时移到图下说明卡。
- 每张图使用独立的响应式 SVG 画布；窄屏允许局部横向滚动，不整体压缩到不可读字号。
- 阶段保持稳定配色：接入为灰色、流为深灰、计算与 Current Fact 为青色、Scene 为蓝色、动作与命令为绿色、回复归并为紫色、失败边界为橙红色。
- 页面顶部提供阶段锚点导航和字号/间距一致的图例。

## 一致性与失败语义

细图必须显式保留以下边界：

- Raw 只有在 ProtocolApplication 的 Kafka transaction 成功后才推进 offset；失败时按相同业务身份重投。
- Canonical 的各 consumer group 彼此独立，一个分支完成不代表其他分支完成。
- Scene Runtime 未越过 Current Fact 的读屏障时必须等待或重试，不能把未追平误判为无数据。
- duration 到期不是动作触发证明，Runtime 必须重新读取事实并复核 missing、freshness 和 predicate。
- PG commit 是动作被接受的唯一边界；回滚时不得留下已接受动作。
- Command reply 是 observation；只有 Reducer 接受的合法状态迁移才能产生 `AcceptedStateTransition`。

每张细图下方增加“本阶段成功意味着什么 / 不意味着什么”，帮助读者识别局部完成与端到端完成的区别。

## 实现组织

新文件保持单文件可移植性：

- 页面级 CSS 定义颜色、排版、阶段卡片、滚动容器和 SVG 通用样式。
- 每张图使用独立 SVG，marker id 带阶段前缀，避免内联 SVG 的箭头定义冲突。
- SVG 使用语义化 `<title>` 和 `<desc>`，页面文本提供同等的阶段摘要。
- 不引入交互脚本；缩放依赖浏览器原生缩放，正常阅读不要求缩放。

## 验证

实现完成后执行以下检查：

1. 确认新 HTML 能由 HTML parser 解析，标签闭合且不存在重复的页面级 id。
2. 检查恰好存在一张总览和五张阶段细图。
3. 检查五个阶段标题、关键组件、关键 Topic/状态和主要失败分支均出现。
4. 扫描 SVG 字号，确认正文、参与者标题和辅助说明符合最低字号约束。
5. 在浏览器中检查桌面宽度和窄屏横向滚动效果。
6. 比较 Git 状态，确认原有 `场景联动端到端时序图.html` 未被修改。

## 验收标准

- 读者在普通桌面浏览器 100% 缩放下能直接读清每条主要消息。
- 读者能先从总览理解完整链路，再通过五张细图理解事务、重投、持续条件和 Reducer 边界。
- 新图覆盖 Markdown 第 5 节的完整案例，不退化为旧 HTML 的单温度点告警案例。
- 页面离线打开即可使用，且不依赖 VS Code Mermaid 扩展。
