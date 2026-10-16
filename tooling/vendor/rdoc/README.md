# Markdown 解析来源

`tooling/doclinks.py` 选择性复用 r-doc 的 Markdown 链接与锚点解析，不加载其审计、修复或归档流程。

- 上游：[riesaexe/r-doc](https://github.com/riesaexe/r-doc)，固定版本 `bc23420d7df5ec1d2c5fb45e066c919790ab9cf9`。
- 实际复制来源：仓内 r-doc v4 的 `skills/r-doc/scripts/rdoc/markdown.py`（保留 v4 的尖括号带标题链接修复）；源文件 SHA-256：`e0ddd0ca01b04245cf44cf6e974f605306b8210021aa33e8272cf9afcecbaedb`。
- 版权：Copyright (c) 2026 RiESA；完整 [MIT 许可](LICENSE)。
- 本地调整：只保留纯文本链接/锚点函数与数据结构；移除磁盘读取、YAML 元数据和配置依赖，路径与读取范围由本仓检查器统一控制；补齐重复标题冲突、转义链接、精确反引号闭合、空图片标签及未定义引用检查。
- 不据此声称完整 CommonMark/HTML 渲染兼容；实际支持与限制见[检查说明](../../../docs/runbook/document-checks.md)。
