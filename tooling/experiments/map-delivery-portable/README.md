# 可跨项目复用的 map-delivery

本地初版：`0.1.0-draft`。公共部分提供业务地图、完整模块规格、增量变更、场景验收和交接流程。项目继续决定文档位置、语言、Issue 平台、批准方式、独立评审和生效条件。

技能源码位于 [`skills/map-delivery/`](skills/map-delivery/SKILL.md)。这个目录可以独立复制和分发，不依赖本工作区的其他仓库。当前团队中的专用同名技能仍保持原状；本包没有自动安装到任何项目，也没有公开发布。

## 在其他项目使用

将 `skills/map-delivery/` 整个目录复制到目标项目的 `.agents/skills/map-delivery/`。如果目标位置已有同名技能，先核对其来源和项目约定，合并或选择版本，不直接覆盖。团队应固定所选源码提交或分发包摘要，升级时检查变化。

在支持项目技能的 Agent 中加载该目录后，可以这样开始：

> 使用 $map-delivery，在本项目建立业务地图。先梳理模块边界，再为订单导出编写初版规格与验收场景，明确尚未实施。

> 使用 $map-delivery，为订单导出增加筛选条件。沿用项目审批约定，先读当前规格，编写本次增量和验证计划。

安装目录存在只能说明文件已放好。请在目标项目的新会话中核实技能被发现，并运行一次实际请求；宿主的技能发现方式以其自身说明为准。若还未被自动发现，可在请求中给出 `SKILL.md` 的实际路径供 Agent 读取。

## 两种文档布局

默认在当前项目的 `map/` 工作，跟随项目语言，无须配置文件。使用仓内其他位置时，在项目根创建 `.map-delivery.json`：

```json
{
  "schema_version": 1,
  "docs_root": ".",
  "map_root": "docs/map",
  "language": "中文"
}
```

独立文档仓示例：产品与文档仓按团队约定放在同一工作区，产品仓配置为：

```json
{
  "schema_version": 1,
  "docs_root": "../project-docs",
  "map_root": "map"
}
```

`docs_root` 相对配置文件所在项目根；`map_root` 相对文档根。两者使用 `/` 分隔的相对路径。配置不保存个人绝对路径、凭据或自动执行命令。成员的 checkout 布局不同、或需要操作候选 worktree 时，显式选择文档目录：

```bash
python3 /path/to/map-delivery/scripts/context.py \
  --project-root /path/to/product \
  --docs-root /path/to/selected-docs-worktree --json
```

该脚本需要 Python 3.10+，仅使用标准库；Git 可选。它只定位目录、读取配置并观察本地文档仓提交。不创建地图，不查询远端，不判断授权、生效、工作树是否干净，也不保证文档质量。配置错误或指定目录不存在时会失败，不转到另一个目录继续。

## 与现有严格流程一起使用

公共内核保留信息职责，允许小型变更把计划与证据写在 Change 中。已经要求独立 Hub、Plan、Review、Evidence、真实 Issue、特定仓库身份或专用校验器的项目，继续使用这些要求。接入时读取现有治理文件并明确各材料位置；严格策略缺失或校验失败时报告缺口，不能自动退回宽松默认值。

本包不包含原项目的专用全量文档校验器。已有项目按实际检查命令验证；新项目先使用技能中的人工结构检查和真实行为验证。后续通用自动检查应由具体使用需求驱动。

## 包内验证

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
```

此命令验证定位工具在单仓、独立文档仓、显式候选工作树、错误配置及路径边界下的行为。技能写作质量与实际执行另用独立 Agent 场景试验验证，结果记录在 [`VALIDATION.md`](VALIDATION.md)。脚本单测通过不代表团队交付收益已得到验证。
