# 许可证与公开范围

本仓按组件保留原许可证；此索引不为整个仓库授予一个新许可证。M4 是本地迁移候选，25文件公开范围及本次 GitHub 候选交付已获用户确认；发布清单已记录该次授权。已有组件许可和下述来源限制继续保留。

| 范围 | 已有声明与证据 |
| --- | --- |
| backend/saas | [AGPL v3](backend/saas/LICENSE)、[NOTICE](backend/saas/NOTICE) |
| backend/things | [AGPL v3](backend/things/LICENSE)、[NOTICE](backend/things/NOTICE)、[来源](backend/things/PROVENANCE.yaml) |
| backend/share | [Apache-2.0](backend/share/LICENSE)、[来源](backend/share/SOURCE.json)；冻结版本再发布授权见[原有确认](docs/rd/specs/2026-08-27-ming-iot-local-share-migration-activation-design.md)，保留文件中的第三方署名 |
| tooling/experiments/r-doc-v3、r-doc-v4 | 各自 LICENSE、UPSTREAM-LICENSE 保留 RiESA 版权与 MIT 正文；均为未正式采纳的实验 |
| 原创工具、Specs 与团队文档 | 本次不新增开放许可，不从其他组件许可证推断；不是整仓原创或权属认证 |
| 转载文章、图片、旧 Mermaid bundle、历史资料 | 按已确认的 [25文件范围](docs/meta/PUBLIC_SCOPE.md) 处理；Mermaid与历史摘录保留已整理声明和明确限制 |

许可证/NOTICE 的固定摘要由 `make publication-check` 核验。该命令只证明技术扫描和声明保留；`make publication-ready` 在待确认项存在时返回失败。任何公开操作还须获得用户针对候选和动作的授权。

当前检查不替代全部传递依赖许可审计或运行环境中的源码提供要求。不会因为保留许可文本就宣称全产品发布合规。

历史源码摘录及设计参考的具体对应另见 [来源与声明](docs/vendor/historical-snippets/README.md)。该补充只增加归属信息，不授予远程操作权限。

补充声明：[历史计划模板](docs/vendor/historical-snippets/TEMPLATE-SOURCES.md)；[Mermaid 精确发行物声明](docs/vendor/mermaid/README.md)。它们保留第三方原声明；本次远程交付授权不改变整仓许可，不消除已披露的来源限制。
