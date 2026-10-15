## MODIFIED Requirements

### Requirement: Temperature value
系统 SHALL 接收整数或小数温度值。

#### Scenario: Integer temperature
- WHEN 设备上报整数温度
- THEN 接收该值

#### Scenario: Decimal temperature
- WHEN 设备上报小数温度
- THEN 接收该值并保留小数
