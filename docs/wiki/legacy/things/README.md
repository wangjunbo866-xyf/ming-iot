# 旧 Things areastat

Things 产品代码与 M2 核对的增量已在 `backend/things`。这里只保存 `tools/areastat` 的 `main.go.source.txt` 与 `go.mod.source.txt`，摘要见 [来源表](../../../../tooling/policy/LEGACY_MATERIALS.json)，不加入 Go workspace 或默认构建。

旧工具存在两类缺口：依赖的 RPC 在当前契约中不存在；区域统计只取得首个设备的聚合值。恢复前需要确定跨设备聚合和云端写入语义，再实现接口与测试。用户尚未确认这些业务规则，本次不猜测其结果，不启动统计或写入设备数据。
