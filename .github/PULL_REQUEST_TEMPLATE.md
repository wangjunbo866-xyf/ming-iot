## 变更与规格

- 需求、模块主规格与 Patch：
- 候选完整 Head：
- 用户可见行为与保持不变的边界：

## 验证与复审

- `make verify-pr`：
- `make publication-check`：
- 同一 Head 的独立 Review：
- 未运行的真实依赖验证、原因及补验证方式：

## 发布边界

技术检查通过不代表允许公开、合并或发布；公开前还须通过 `make publication-ready`，并取得绑定候选和动作的授权。许可证待确认项见 `tooling/policy/PUBLICATION_POLICY.json`。

## 完整差异声明

按 [PR 检查约定](../.agents/skills/ming-specs/references/pr-check.md) 填写一个 `ming-specs` JSON 块。
行为变化引用 Patch 及 revision；规则不变引用当前条款和理由；多模块分别声明。
下方空声明不会豁免任何实际变更。历史移动用 archives；未映射产品代码必须明确列出缺口和参考入口。

```ming-specs
{
  "schema_version": 1,
  "modules": [],
  "exceptions": [],
  "archives": []
}
```
