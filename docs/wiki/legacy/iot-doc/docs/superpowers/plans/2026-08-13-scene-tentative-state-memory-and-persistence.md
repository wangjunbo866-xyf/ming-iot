# Scene Tentative State、内存预算与持久化说明补充计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 澄清 tentative state 只要求提交隔离而不要求完整深拷贝，补齐单设备 5 分钟 rolling 的一轮处理、`scene_state` 发布/Compaction 说明，以及百万设备下的常驻内存、临时分配与 GC 验收预算。

**Architecture:** 《场景有状态计算规范》继续拥有业务求值和状态组件语义；《场景联动 Runtime 主规格总纲》继续拥有状态发布、Kafka 物理协议与容量模型；《SLO、故障测试与实施》只建立可执行验收项。机器契约 schema 不变，生成目录不修改。

**Tech Stack:** Markdown、Kafka compacted topic、Go 运行时内存模型术语。

---

## 执行约束

- 工作区根 `AGENTS.md` 禁止未经授权执行 `git add`、`git commit` 或 `git push`，本计划只修改并验证文件。
- 不改变 `SceneBindingState` schema、`effectId`、Action Outbox 接受边界、Kafka Topic 或 partition 算法。
- 不把 copy-on-write 固定成唯一代码实现；规范冻结的是 accepted state 不可被 tentative 失败污染。
- 不修改生成区 `BEGIN/END GENERATED: stateful-computation-catalog`。

## Task 1：补齐有状态计算读者路径

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景有状态计算规范.md`

- [x] 在第 3 章增加 `scene_state` 写入与 Compaction 的集中说明，明确写入触发、完整 image、异步 best-effort 和 Compaction 不降低入口写放大。
- [x] 在第 4 章增加“tentative 是逻辑提交隔离，不是深拷贝”小节，规定 accepted base、mutation overlay、freeze/encode、commit/publish/discard 顺序和别名泄漏禁令。
- [x] 在 rolling 章节增加单设备 5 分钟平均温度的一轮处理示例，分别说明 warm state、cold bootstrap、无动作和有动作分支何时发布恢复副本。
- [x] 在五类算子章节补足简短实例，并在实施检查表增加 tentative 隔离、状态发布和内存预算检查项。

## Task 2：冻结 Runtime 内存模型与容量公式

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/场景联动 Runtime 主规格总纲.md`

- [x] 改写 §12.3 第一步，取消“副本”可能等于完整深拷贝的歧义。
- [x] 明确允许 `base + mutation overlay`、copy-on-write、持久化结构或可证明回滚的等价实现；默认不得无条件深拷贝完整对象图。
- [x] 在 §15 增加每节点 accepted state 常驻内存、并发 tentative 峰值、异步 Producer 编码缓冲和分配带宽公式。
- [x] 明确 32 KiB 是单条序列化硬上限，不是平均状态大小、解码后内存或节点内存预算。
- [x] 在 §16 和 §19 增加 accepted state 数量/字节、tentative 分配、Producer in-flight、GC 和 heap headroom 指标及 POC 证据。

## Task 3：补齐故障与容量验收

**Files:**

- Modify: `亿级点位高吞吐架构/6.规则引擎与场景联动/SLO故障测试与实施.md`

- [x] 把 tentative commit isolation 加入冻结正确性约束。
- [x] 把节点常驻状态、每轮分配、GC、Producer in-flight buffer 和并发数加入 POC 待定参数。
- [x] 扩展 F-16：在 rolling/map/slice 已发生 tentative 修改后注入序列化失败和 PG rollback，断言 accepted state、Timer handle 与 receipt 均未变化。
- [x] 在指标到证据映射及 Runtime 实施工件中加入 mutation overlay/COW 正确性测试和 allocation benchmark。

## Task 4：一致性验证

**Files:**

- Verify all modified Markdown files.

- [x] 运行 `git diff --check`，预期无输出。
- [x] 搜索 `tentative`、`深拷贝`、`Compaction`、`StateTxn`、内存公式和新增验收项，确认定义位置与交叉引用一致。
- [x] 检查生成目录 marker 和 hash 未变化。
- [x] 运行 Markdown 相对链接检查；若仓库没有专用工具，解析本次新增/修改的本地链接并确认目标存在。
- [x] 运行 `git status --short`，确认只有本计划和三个预期规范文件发生变化。
