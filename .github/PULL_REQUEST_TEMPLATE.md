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
