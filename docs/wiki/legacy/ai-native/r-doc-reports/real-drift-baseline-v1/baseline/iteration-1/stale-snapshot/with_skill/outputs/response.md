核验未完成，`report.json` 也未能写入：本地执行工具中途报“无法创建进程”，备用读取工具和文件写入随后均失败。

已实测确认：

- 父仓固定版本与子仓实际检出均为 `e38d162feba9445c3bedb6ec78d96ee79dee9a49`。
- 该提交的父版本为 `a6b28035c577131c1a4b0262af6ddbdc393104c5`。
- 快照入口指向 `docs/vendor/ming-iot-device-api.json`，来源说明为 `docs/vendor/source.json`。

尚未取得快照哈希、三个 schema、接口及正式 Spec 正文，**不能判定 `stale/aligned`，也不能判定第 3 节应清除**。应待读取恢复后完成字节、前后版本与批准目标的核对，再决定同步或保留快照。

未修改原文件或 Git HEAD/index，未运行产品测试、网络或远端操作。
