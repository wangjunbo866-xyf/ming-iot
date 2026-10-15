# 薄导航可运行样例

这是 Wayfinder 接入讨论的隔离演示，业务内容和归档历史均为虚构。方案详见 [遥测上报薄导航](../10-thin-navigation-telemetry.md)。

先看 [业务地图](map/README.md) → [遥测上报](map/data/telemetry-ingest.md) → [小数温度 Patch](map/changes/patch-temperature-decimal.md)。从 Patch 打开 proposal 后，可通过自动导航返回功能页及当前规格。

Python 3.9+，仅使用标准库。从本目录执行：

```bash
python3 tools/nav.py build
python3 tools/nav.py check
python3 tools/walkthrough.py
```

- `build` 生成 map 页面，并维护主 spec 的 Purpose、proposal 的 Impact 内标记区块；不改写业务正文。
- `check` 只读检查，索引过期、链接缺失或关系有歧义时返回非零。
- `walkthrough.py` 在临时副本模拟归档、草案、关系遗漏和重名，不改变这里保留的活动 Patch。

人工来源只有 [catalog](map/catalog.json) 和原生 OpenSpec 文档；动态列表都在生成页面。无 delta 的修复用 Impact 中的单次关系标记，已有 delta 的变更不重复填这个标记。原生 OpenSpec 不会自动运行此工具。

Change ID 限制为字母开头、不可复用。标识已带日期的旧 Change 需要另行映射，样例不猜测身份。此脚本不是完整的 Markdown/OpenSpec 解析器，未接入真实 OpenSpec CLI、store、CI、版本化验收证据或退役/入口重命名流程。

运行结果见 [验证记录](validation.md)。
