# ming-iot-rd-docs

The private R&D authority for the Ming IoT personal development framework.

- `rfcs/`: long-lived architectural decisions with real tradeoffs;
- `specs/`: current approved designs;
- `plans/`: reviewed executable plans;
- `evidence/`: compact deterministic execution evidence;
- `reviews/`: independent reviewer findings and verdicts;
- `proposals/`: alternatives awaiting a user decision;
- `adr/`: durable architectural and workflow decisions, stored at the repository root;
- `references/`: local reference materials retained for reproducible R&D decisions;
- `archives/`: superseded material and disaster-recovery bundles, never current authority.

This private repository uses `https://gitee.com/ming-industry/ming-iot-rd-docs.git` as its approved `origin`. Product code remains in `ming-iot`; formal IoT architecture remains in `iot-doc`; dynamic Goal/Todo state remains in LoopX.

## 团队技能

`map-delivery` 源码由 [ai-native](https://gitee.com/ming-industry/ai-native) 维护；本仓库保留文档规范、模板、工具和交付事实。来源契约为 [meta/SKILLS_SOURCE.json](meta/SKILLS_SOURCE.json)。本机 `.agents/skills/map-delivery` 与 `.claude/skills/map-delivery` 是消费入口，不是被 Git 跟踪的技能副本。

在工作区克隆 `ai-native` 后，从已经合入的文档主工作树注册工作区、产品仓和文档仓的 Codex/Claude 入口：

```bash
python3 ming-iot-rd-docs/tools/setup_workspace_skills.py \
  --workspace-root "$PWD" --product-repo "$PWD/ming-iot" \
  --skills-repo "$PWD/ai-native"
```

默认命令拒绝覆盖异源入口；旧文档仓来源的迁移选项、合入顺序和验证要求见 [迁移计划](plans/2026-10-04-map-delivery-ai-native-source.md)。真实 TeamAI 安装和技能内容检查使用 `ai-native` 的入口。单独检出文档仓可运行 CI，不要求兄弟仓已经安装；这不能证明成员机器或模型加载了技能。
