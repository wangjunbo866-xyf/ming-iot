# Mermaid 11.4.0 来源和声明

本目录供本地候选审阅，公开仍 HOLD。它保留已固定的发行物、依赖、生成器和补丁声明，不能理解为已完成全部权利判断。原 bundle 和其内嵌声明均未改动。

- [声明原文集合](THIRD-PARTY-NOTICES.md)：逐项来源与原文；字节定位及摘要见 [机器来源表](sources.json)。
- [roughjs 原补丁](roughjs.patch)：固定上游 `56f522ec50b5c746be5532686dd465a06215273d` 的类型声明改动；不是另一次运行时构建。
- [历史片段归属](../historical-snippets/README.md)。

70 个 pnpm 标识保留并存版本、peer 后缀与 patch 标记；对应 71 份许可/声明文本，另保留 Mermaid、parser、Jison README 许可和 Jison Lex README 标签。不是用一个根 MIT 覆盖全部内容。1,090 个 source map 条目中有 1,073 个整段源码字节对应，17 个生成条目未完整重建。此前对 16 个解析器的运行时函数及入口作了限定比对，不据此声称完整生成产物可重现。

Jison Lex 0.3.4 的 [固定源码](https://github.com/zaach/jison-lex/tree/28aa5ac329310ac74eff0d4259fcf594429134f7) 和 README 标示 MIT，源码头为 `// MIT Licensed`；上游 package.json 作者为 Zach Carter。精确包及已抓取 33 个提交、46 个 blob 的限定检索未定位完整版权/许可正文。这是材料缺口，不能改写成“没有许可证”；此处没有编造年份、版权声明或许可授予，也不要求重复无边界搜索。最终保留该 bundle 时需对这一精确剩余项作处置；可选继续保留带明确限制的本地候选，或在公开范围决定中单独选择暂不分发。完整重建不是本轮新设通用门禁。
