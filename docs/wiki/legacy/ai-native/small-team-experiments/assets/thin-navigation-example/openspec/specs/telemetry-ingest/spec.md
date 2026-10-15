# Telemetry Ingest Specification

## Purpose

<!-- thin-nav:begin -->
[功能导航：上下文、相关变更与历史](../../../map/data/telemetry-ingest.md)
<!-- thin-nav:end -->

说明设备遥测接收的示例行为。本文仅用于薄导航演示，不能作为真实项目需求。

## Requirements

### Requirement: Temperature value
系统 SHALL 接收整数温度值。

#### Scenario: Integer temperature
- WHEN 设备上报整数温度
- THEN 接收该值

### Requirement: Report timestamp
系统 SHALL 使用毫秒时间戳记录上报时间。

#### Scenario: Millisecond timestamp
- WHEN 消息带有合法毫秒时间戳
- THEN 保留该时间戳
