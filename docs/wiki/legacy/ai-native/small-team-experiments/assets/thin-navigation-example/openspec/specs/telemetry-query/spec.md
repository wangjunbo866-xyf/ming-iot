# Telemetry Query Specification

## Purpose

<!-- thin-nav:begin -->
[功能导航：上下文、相关变更与历史](../../../map/data/telemetry-query.md)
<!-- thin-nav:end -->

说明遥测查询的示例行为。本文仅用于薄导航演示，不能作为真实项目需求。

## Requirements

### Requirement: Temperature serialization
系统 SHALL 在温度查询中返回已存储的整数值。

#### Scenario: Integer response
- WHEN 查询整数温度记录
- THEN 返回相同整数值
