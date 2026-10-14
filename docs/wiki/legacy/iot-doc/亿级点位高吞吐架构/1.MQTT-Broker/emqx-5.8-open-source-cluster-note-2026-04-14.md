# EMQX 5.8 开源版集群能力说明

## 结论

截至 `2026-04-14` 的官方资料核查结果：

- `EMQX 5.8` 开源版可以做集群。
- 开源版仅支持 `Core-only` 集群，不支持 `Core + Replicant` 架构。
- 官方建议开源版集群规模控制在 `3 节点左右`，以获得更稳定的运行效果。

一句话结论：

> `EMQX 5.8 开源版支持集群，但仅支持 Core-only 集群；如果需要 Core + Replicant 或更大规模集群能力，应使用企业版。`

## 可直接转发给客户或同事的话术

`EMQX 5.8` 开源版不是只能单机。根据 EMQX 官方文档，开源版仍然支持集群，但从 `5.8` 开始，开源版去掉了 `Core-Replicant` 模式，所有开源版节点都以 `Core` 角色运行，因此其可用形态是 `Core-only` 集群。官方同时建议开源版集群规模控制在 `3` 个节点左右。如果项目需要 `Core + Replicant` 架构、复制型持久会话或更大规模集群，建议使用企业版。

## 官方依据

### 1. EMQX 5.8 变更说明

官方文档明确说明：

- 从 `5.8` 开始，开源版移除了 `Core-replicant` 功能。
- 开源版运行的所有节点都将以 `Core` 角色运行。

这说明 `5.8` 开源版并不是不能集群，而是集群形态变成了 `Core-only`。

链接：

- https://docs.emqx.com/en/emqx/latest/changes/breaking-changes-ee-5.8.html

### 2. 集群架构说明

官方文档明确说明：

- `Core + Replicant` 集群架构只在企业版提供。
- 开源版支持 `Core-only clusters`。

链接：

- https://docs.emqx.com/en/emqx/latest/deploy/cluster/mria-introduction.html

### 3. 集群总览说明

官方文档明确说明：

- 开源版没有严格的节点上限。
- 但官方建议将开源版集群规模控制在 `3` 个节点左右。
- 原因是开源版只有 `Core` 类型节点，较小规模通常更稳定。

链接：

- https://docs.emqx.com/en/emqx/latest/deploy/cluster/introduction.html

### 4. 集群创建与管理

官方提供了完整的集群创建文档，包含手动组集群、静态节点、DNS、etcd、Kubernetes 等方式。这进一步说明开源版的集群能力是官方文档体系内的正式能力，而不是社区非正式实践。

链接：

- https://docs.emqx.com/en/emqx/latest/deploy/cluster/create-cluster.html

## GitHub 侧证据说明

### 1. 官方文档仓库

EMQX 的官方文档仓库是：

- https://github.com/emqx/emqx-docs

该仓库 README 明确说明：

- 该仓库是 `docs.emqx.com` 文档站的源码仓库。
- 社区版和企业版文档使用同一个仓库，并按 `release-Major.Minor` 分支管理。

因此，`docs.emqx.com` 上述页面可以视为该官方 GitHub 文档仓库对应发布内容的正式文档结果。
本次核查中，GitHub 侧能直接确认的是“文档仓库来源”和“官方 Discussion 补充说明”；关于 `EMQX 5.8 开源版是否支持集群` 的主结论，仍以 `docs.emqx.com` 的官方文档页为准。

### 2. 官方 GitHub Discussion 的补充说明

官方 GitHub Discussion 中有一条与 `EMQX v5.8.4 open source cluster` 直接相关的问答：

- https://github.com/emqx/emqx/discussions/14522

该问答的关键信息是：

- 提问场景本身就是 `EMQX 5.8.4 open source cluster`。
- 官方协作者说明，`replicated durable sessions` 是企业版能力。
- 开源版可用的是 `non-replicated durable sessions`，因此该能力只能用于单节点，不适用于开源版集群。

这条 Discussion 不能替代集群主文档，但它从侧面证明了：

- `EMQX 5.8` 开源版确实存在集群使用场景。
- 受限的是某些企业特性，而不是“开源版完全不能做集群”。

## 边界与注意事项

- 本文讨论的是 `EMQX 5.8 开源版` 是否支持集群，不等同于“开源版和企业版集群能力相同”。
- `Core + Replicant` 架构属于企业版能力，开源版不能使用。
- 与复制相关的某些高级能力，例如复制型持久会话，不应按“开源版集群可用”来理解。
- 如果项目目标是更高节点规模、更高稳定性或更完整的高可用特性，应优先评估企业版。

## 建议对外表述

建议统一使用下面这段话：

> `EMQX 5.8 开源版支持集群，但仅支持 Core-only 集群；从 5.8 开始开源版不再支持 Core + Replicant。官方建议开源版集群规模控制在 3 个节点左右。`

如果需要补一句版本边界，可再加：

> `如果项目需要 Core + Replicant、复制型持久会话或更大规模集群能力，应考虑企业版。`

## 参考链接

- 官方文档仓库：https://github.com/emqx/emqx-docs
- 5.8 变更说明：https://docs.emqx.com/en/emqx/latest/changes/breaking-changes-ee-5.8.html
- 集群架构说明：https://docs.emqx.com/en/emqx/latest/deploy/cluster/mria-introduction.html
- 集群总览：https://docs.emqx.com/en/emqx/latest/deploy/cluster/introduction.html
- 创建和管理集群：https://docs.emqx.com/en/emqx/latest/deploy/cluster/create-cluster.html
- GitHub Discussion（5.8.4 open source cluster）：https://github.com/emqx/emqx/discussions/14522
