# 文档仓定位

执行当前已加载 Skill 目录中的 `scripts/resolve_docs.py`。已选定候选文档工作树时显式传入它，避免把候选读成主工作树：

```bash
python "${map_skill_dir:?设置已加载Skill目录}/scripts/resolve_docs.py" \
  --docs-root "${map_docs_root:?设置选定文档工作树}" --json
```

尚未选定文档工作树时，可传入当前产品目录或工作区：

```bash
python "${map_skill_dir:?设置已加载Skill目录}/scripts/resolve_docs.py" \
  --start "${map_start:?设置当前项目目录}" --json
```

`--workspace-root` 限定一个工作区；省略时，从 `--start`（默认当前目录）向上最多检查 12 层，在最近存在候选的目录中选择。每层只检查自身 Git 工作树与直接子目录 `ming-iot-rd-docs`，不递归扫描。存在多个不同候选时失败，需显式传入 `--docs-root`；显式路径缺失、身份不符或内容不完整时失败，不回退到其他仓库。

成功 JSON 返回 `docs_root`、稳定 `identity` 和完整 `exact_head`；默认只输出路径。Git `origin` 的所有 fetch URL 必须归一为 `git:gitee.com/ming-industry/ming-iot-rd-docs`，并具有可读的 `AGENTS.md`、`map/README.md`、`tools/map_docs.py` 和有效提交。HTTPS、SSH 和 SCP 形式只用于身份判断；具体 remote 配置仍受文档仓 `AGENTS.md` 约束。

定位完全只读，不联网、不调用凭据助手、不输出原始 remote URL。返回 Head 是当时的本地观察，不证明远端状态、工作树干净或该候选已获批准。读取实际 `AGENTS.md` 后，在返回文档工作树中继续流程；缺口按实际批准范围处理。
