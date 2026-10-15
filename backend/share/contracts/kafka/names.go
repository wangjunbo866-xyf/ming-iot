// 本文件定义 Kafka 受管 Topic 名称、代数及第一版事实类型契约。
package kafka

import (
	"fmt"
	"strings"
)

// FactKind 标识 Kafka 数据面中的事实类型。
type FactKind string

const (
	// FactRaw 标识协议原始事实。
	FactRaw FactKind = "raw"
	// FactCanonicalTelemetry 标识标准遥测事实。
	FactCanonicalTelemetry FactKind = "canonical-telemetry"
	// FactCanonicalDeviceEvent 标识标准设备事件事实。
	FactCanonicalDeviceEvent FactKind = "canonical-device-event"
	// FactCanonicalLifecycle 标识标准生命周期观测事实。
	FactCanonicalLifecycle FactKind = "canonical-lifecycle"
	// FactCanonicalCommandReply 标识标准命令回复观测事实。
	FactCanonicalCommandReply FactKind = "canonical-command-reply"
	// FactCanonicalOutcome 标识标准结果事实。
	FactCanonicalOutcome FactKind = "canonical-outcome"
	// FactQuarantineIngress 标识入口隔离事实。
	FactQuarantineIngress FactKind = "quarantine-ingress"
	// FactQuarantineProtocol 标识协议隔离事实。
	FactQuarantineProtocol FactKind = "quarantine-protocol"
)

// LogicalTopicName 是不可变的受管逻辑 Topic 名称值对象。
type LogicalTopicName struct {
	// value 保存已校验的逻辑 Topic 名称。
	value string
}

// Generation 是不可变的 Topic 代数值对象，取值范围为 1 到 99。
type Generation struct {
	// value 保存已校验的 Topic 代数。
	value int
}

// PhysicalTopicName 是不可变的物理 Topic 名称值对象。
type PhysicalTopicName struct {
	// value 保存已校验的物理 Topic 名称。
	value string
}

// NewLogicalTopicName 校验并构造受管逻辑 Topic 名称。
func NewLogicalTopicName(value string) (LogicalTopicName, error) {
	if err := validateLogicalTopicName(value); err != nil {
		return LogicalTopicName{}, err
	}
	return LogicalTopicName{value: value}, nil
}

// NewGeneration 校验并构造 Topic 代数。
func NewGeneration(value int) (Generation, error) {
	if value < 1 || value > 99 {
		return Generation{}, fmt.Errorf("generation must be between 1 and 99: %d", value)
	}
	return Generation{value: value}, nil
}

// NewPhysicalTopicName 根据逻辑 Topic 名称和代数构造确定性的物理 Topic 名称。
func NewPhysicalTopicName(logical LogicalTopicName, generation Generation) (PhysicalTopicName, error) {
	if err := validateLogicalTopicName(logical.value); err != nil {
		return PhysicalTopicName{}, fmt.Errorf("invalid logical topic name: %w", err)
	}
	if generation.value < 1 || generation.value > 99 {
		return PhysicalTopicName{}, fmt.Errorf("invalid generation: %d", generation.value)
	}
	return PhysicalTopicName{
		value: fmt.Sprintf("%s.g%02d", logical.value, generation.value),
	}, nil
}

// V1LogicalTopicName 返回第一版已冻结事实类型对应的逻辑 Topic 名称。
func V1LogicalTopicName(kind FactKind, protocolKey string) (LogicalTopicName, error) {
	if kind == FactRaw {
		if err := validateRawProtocolKey(protocolKey); err != nil {
			return LogicalTopicName{}, err
		}
		return NewLogicalTopicName("iot.data.raw.v1." + protocolKey)
	}
	if protocolKey != "" {
		return LogicalTopicName{}, fmt.Errorf("protocol key is only supported for raw facts")
	}

	var value string
	switch kind {
	case FactCanonicalTelemetry:
		value = "iot.data.canonical.telemetry.v1"
	case FactCanonicalDeviceEvent:
		value = "iot.data.canonical.device-event.v1"
	case FactCanonicalLifecycle:
		value = "iot.data.canonical.lifecycle-observation.v1"
	case FactCanonicalCommandReply:
		value = "iot.data.canonical.command-reply-observation.v1"
	case FactCanonicalOutcome:
		value = "iot.data.canonical.outcome.v1"
	case FactQuarantineIngress:
		value = "iot.data.quarantine.ingress.v1"
	case FactQuarantineProtocol:
		value = "iot.data.quarantine.protocol.v1"
	default:
		return LogicalTopicName{}, fmt.Errorf("fact kind %q is not frozen in v1", kind)
	}
	return NewLogicalTopicName(value)
}

// validateV1TopicTemplate 验证逻辑 Topic 名称与已冻结事实类型模板完全一致。
func validateV1TopicTemplate(logical LogicalTopicName, kind FactKind) error {
	const rawPrefix = "iot.data.raw.v1."

	protocolKey := ""
	if kind == FactRaw {
		value := logical.String()
		if !strings.HasPrefix(value, rawPrefix) {
			return fmt.Errorf("raw logical name must use v1 template")
		}
		protocolKey = strings.TrimPrefix(value, rawPrefix)
	}

	expected, err := V1LogicalTopicName(kind, protocolKey)
	if err != nil {
		return fmt.Errorf("invalid v1 fact kind or logical name: %w", err)
	}
	if logical != expected {
		return fmt.Errorf("logical name does not match fact kind %q", kind)
	}
	return nil
}

// String 返回逻辑 Topic 名称的字符串表示。
func (name LogicalTopicName) String() string {
	return name.value
}

// String 返回代数的十进制字符串表示。
func (generation Generation) String() string {
	return fmt.Sprintf("%d", generation.value)
}

// String 返回物理 Topic 名称的字符串表示。
func (name PhysicalTopicName) String() string {
	return name.value
}

// validateLogicalTopicName 校验逻辑 Topic 名称的 token 和动态维度约束。
func validateLogicalTopicName(value string) error {
	if value == "" {
		return fmt.Errorf("logical topic name must not be empty")
	}
	for _, token := range strings.Split(value, ".") {
		if token == "" {
			return fmt.Errorf("logical topic name must not contain empty tokens")
		}
		if isDynamicDimension(token) {
			return fmt.Errorf("logical topic name must not contain dynamic dimension %q", token)
		}
		if err := validateStableToken(token); err != nil {
			return fmt.Errorf("invalid logical topic token %q: %w", token, err)
		}
	}
	return nil
}

// validateRawProtocolKey 校验 Raw Topic 使用的稳定协议 token。
func validateRawProtocolKey(value string) error {
	if err := validateStableToken(value); err != nil {
		return fmt.Errorf("invalid raw protocol key: %w", err)
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"device", "tenant", "scene"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"-") {
			return fmt.Errorf("raw protocol key must not use dynamic dimension prefix %q", prefix)
		}
	}
	return nil
}

// validateStableToken 校验不含通配符和不安全字符的单个 token。
func validateStableToken(value string) error {
	if value == "" {
		return fmt.Errorf("token must not be empty")
	}
	if strings.ContainsAny(value, "*#+") {
		return fmt.Errorf("token must not contain wildcard characters")
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return fmt.Errorf("token contains unsafe character %q", char)
	}
	return nil
}

// isDynamicDimension 判断 token 是否代表禁止冻结的动态资源维度。
func isDynamicDimension(token string) bool {
	lower := strings.ToLower(token)
	if lower == "device-event" {
		return false
	}
	for _, dimension := range []string{"device", "tenant", "scene"} {
		if lower == dimension || strings.HasPrefix(lower, dimension+"-") {
			return true
		}
	}
	return false
}
