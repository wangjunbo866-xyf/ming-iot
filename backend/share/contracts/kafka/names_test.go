// 本文件验证 Kafka Topic 名称值对象与第一版 Raw Topic 模板约束。
package kafka

import "testing"

// TestTopicNameAcceptsManagedNames 验证合法逻辑名、代数和物理名可被确定构造。
func TestTopicNameAcceptsManagedNames(t *testing.T) {
	t.Parallel()

	logical, err := NewLogicalTopicName("iot.data.canonical.telemetry.v1")
	if err != nil {
		t.Fatalf("NewLogicalTopicName() error = %v", err)
	}
	if got, want := logical.String(), "iot.data.canonical.telemetry.v1"; got != want {
		t.Fatalf("LogicalTopicName.String() = %q, want %q", got, want)
	}

	generation, err := NewGeneration(1)
	if err != nil {
		t.Fatalf("NewGeneration() error = %v", err)
	}
	physical, err := NewPhysicalTopicName(logical, generation)
	if err != nil {
		t.Fatalf("NewPhysicalTopicName() error = %v", err)
	}
	if got, want := physical.String(), "iot.data.canonical.telemetry.v1.g01"; got != want {
		t.Fatalf("PhysicalTopicName.String() = %q, want %q", got, want)
	}

	raw, err := V1LogicalTopicName(FactRaw, "modbus-tcp")
	if err != nil {
		t.Fatalf("V1LogicalTopicName(FactRaw) error = %v", err)
	}
	if got, want := raw.String(), "iot.data.raw.v1.modbus-tcp"; got != want {
		t.Fatalf("raw LogicalTopicName.String() = %q, want %q", got, want)
	}
}

// TestTopicNameAcceptsStableProtocolPrefixes 验证稳定协议名可使用动态维度单词作为普通前缀。
func TestTopicNameAcceptsStableProtocolPrefixes(t *testing.T) {
	t.Parallel()

	for _, protocolKey := range []string{"devicenet", "tenantnet", "scenenet"} {
		protocolKey := protocolKey
		t.Run(protocolKey, func(t *testing.T) {
			t.Parallel()
			if _, err := V1LogicalTopicName(FactRaw, protocolKey); err != nil {
				t.Fatalf("V1LogicalTopicName(FactRaw, %q) error = %v", protocolKey, err)
			}
		})
	}
}

// TestTopicNameRejectsUnsafePatterns 验证通配符、空 token 和动态资源维度不能进入受管名称。
func TestTopicNameRejectsUnsafePatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "星号通配符", value: "iot.data.raw.v1.*"},
		{name: "井号通配符", value: "iot.data.raw.v1.#"},
		{name: "加号通配符", value: "iot.data.raw.v1.+"},
		{name: "空 token", value: "iot.data..raw.v1"},
		{name: "设备动态维度", value: "iot.data.raw.v1.device.device-42"},
		{name: "租户动态维度", value: "iot.data.raw.v1.tenant.acme"},
		{name: "场景动态维度", value: "iot.data.raw.v1.scene.scene-7"},
		{name: "设备动态 token", value: "device-42"},
		{name: "租户动态 token", value: "tenant-acme"},
		{name: "场景动态 token", value: "scene-7"},
		{name: "设备动态 pattern", value: "iot.data.device-42.v1"},
		{name: "租户动态 pattern", value: "iot.data.tenant-acme.v1"},
		{name: "场景动态 pattern", value: "iot.data.scene-7.v1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewLogicalTopicName(test.value); err == nil {
				t.Fatalf("NewLogicalTopicName(%q) error = nil, want rejection", test.value)
			}
		})
	}
}

// TestTopicNameRejectsUnsafeRawProtocolKeys 验证 Raw 模板只接受单个稳定协议 token。
func TestTopicNameRejectsUnsafeRawProtocolKeys(t *testing.T) {
	t.Parallel()

	for _, protocolKey := range []string{"", "*", "modbus.tcp", "device-42", "tenant-42", "scene-42"} {
		protocolKey := protocolKey
		t.Run(protocolKey, func(t *testing.T) {
			t.Parallel()
			if _, err := V1LogicalTopicName(FactRaw, protocolKey); err == nil {
				t.Fatalf("V1LogicalTopicName(FactRaw, %q) error = nil, want rejection", protocolKey)
			}
		})
	}
}

// TestTopicNameRejectsInvalidGeneration 验证零值和超出两位代数范围的 generation 被拒绝。
func TestTopicNameRejectsInvalidGeneration(t *testing.T) {
	t.Parallel()

	for _, value := range []int{0, -1, 100} {
		if _, err := NewGeneration(value); err == nil {
			t.Fatalf("NewGeneration(%d) error = nil, want rejection", value)
		}
	}
}
