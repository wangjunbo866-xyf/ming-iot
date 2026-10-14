# 支持小数温度（演示 Patch）

## Why

演示一项 Patch 同时关联上报和查询两个功能。不是已批准的真实需求。

## What Changes

上报与查询均支持小数温度，整数兼容性保留。

## Capabilities

### New Capabilities

### Modified Capabilities
- `telemetry-ingest`: 接受小数温度。
- `telemetry-query`: 查询保留小数温度。

## Impact

<!-- thin-nav:begin -->
- [此变更的稳定入口](../../../map/changes/patch-temperature-decimal.md)（自动导航）
- [遥测上报功能页](../../../map/data/telemetry-ingest.md) · [当前完整规格](../../specs/telemetry-ingest/spec.md)
- [遥测查询功能页](../../../map/data/telemetry-query.md) · [当前完整规格](../../specs/telemetry-query/spec.md)
<!-- thin-nav:end -->

影响接收校验与查询序列化；代码位置、基线 revision 和交付证据尚未提供。
