---
status: draft
version: 0.2.2-draft
---

# protocol.uplink-normalization：设备上行解码与标准化闭环

- 所属能力域：[protocol / 协议适配](../README.md)。稳定 Feature ID 为 `protocol.uplink-normalization`；ProtocolApplication 是拟承载它的应用实现名称，部署方式尚未确定。
- 当前设计正文：[spec.md@0.2.1-draft](spec.md)。这是本 Feature 唯一的当前候选规格，尚无已实施生效版本；既有 DG/DM 解码现状与新目标分别列明。
- 文档归属：原始需求与澄清在本地 raw；当前规则与验收在 spec。入口修订不改变业务语义或实施状态。
- 需求来源：[本模块 raw.md](raw.md)，包含原始目标、PA-Q1–Q6 的真实答复、共享约束在本功能的适用范围及知识归属纠正。原 Eventbus 记录仅作带来源的历史留档，日常接手从本目录开始。
- 责任：需求负责人为用户；域与 Feature 长期负责人未分配。当前起草身份为 `rdoc-pilot-codex`，同一 Goal 的任务 `todo_5c930ff38c388b64f9fbbb5a` 负责文档及验证，独立 AI 复核候选。
- 本次入口维护：`todo_644e00236af7a191ba899708` 仅调整导航并验证协作方法；上行设计接续任务仍为上条所列任务。产品实现、部署配置与旧环境数据不在写范围。
- 实现线索：[DG 设备上行](../../../../backend/things/service/dgsvr/internal/event/deviceSub/deviceSub.go)、[协议 SDK](../../../../backend/things/sdk/protocol/coreProtocol.go)、[DM 属性处理](../../../../backend/things/service/dmsvr/internal/event/deviceMsgEvent/thing.go)。这些是既有源码依据，不是新 PA 实现。
- 登记状态：已加入文档地图、来源索引和有界文档检查；尚未加入产品差异映射 `tooling/specs-map.json`。该映射应在实施认领时按真实改动范围补齐，并覆盖与 Eventbus/DM 的共享路径。

## 接手顺序与下一步

日常理解先读本页，再读 [当前 spec](spec.md) 的目标、规则、异常、依赖与验收。追查决定来源或调整需求时读[本地 raw](raw.md)及相关候选；只有审计原时点或核对公共接口时才追溯跨模块材料。本轮已据仓内 packReport 历史样本细化契约，并确认批量、稳定编号及迟到规则。下一步取得首个真实设备协议的脱敏字节样本和重发/应答证据，再冻结 schema、profile、恢复及部署预算；已有样本不证明真实设备兼容。设计清楚不自动授予实施权限。

本轮通过标准是仅凭本目录即可复述目标、已确认与未决项、责任、当前规则和验收；本地来源可追溯，历史保留，依赖与状态版本一致。相关检查为 `make knowledge-check` 和文档差异检查；运行验收场景见 spec，全部保持未执行。文档检查或 AI 复审不等于设备、Kafka、DM 全链路验收。
