## MODIFIED Requirements

### Requirement: Temperature serialization
系统 SHALL 在温度查询中保留已存储数值的小数部分。

#### Scenario: Integer response
- WHEN 查询整数温度记录
- THEN 返回相同整数值

#### Scenario: Decimal response
- WHEN 查询小数温度记录
- THEN 返回该值并保留小数
