已确认字段冲突，但任务未全部完成，**`report.json` 写入失败**。

- 三个指定 schema：快照均为 `moduleCode: integer/int64`；当前接口均为 `firmwareModuleCode: string`。因此局部结论为 `stale`，建议 `resync_snapshot`，需更新字段名称和类型，不能只刷新日期。
- 父仓固定版本与子仓实际检出版本一致，均为 `e38d162feba9445c3bedb6ec78d96ee79dee9a49`。
- 快照实测 SHA-256 与来源说明一致：`f3bdeb9163899005cccd40552e8b014e31d1d6d69c4869b7eac5503b9da9d64b`。
- 正式 Spec 第 3 节明确绑定旧基线 `a6b28035…`，应 `preserve_historical`。第 4.1 节批准目标与当前三个接口字段一致，限定范围内为 `aligned`。

执行环境中途丢失 `codex-linux-sandbox`，备用读取和报告写入也失败。因此尚未完成来源 Git blob 字节比对、本地前后提交变化方向及末尾状态复核，不能宣称核验完成。未修改原文件，未运行产品测试；系统及全部仓库验收不能据此推断。
