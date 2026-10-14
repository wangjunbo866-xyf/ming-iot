// 本文件验证 Kafka Topic Catalog 的数据面骨架、失败关闭规则与确定性摘要。
package kafka

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestCatalogExpressesV1DataPlaneSkeleton 验证第一版只冻结 Raw、Canonical 与两类 quarantine 名称骨架。
func TestCatalogExpressesV1DataPlaneSkeleton(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind        FactKind
		protocolKey string
		want        string
	}{
		{kind: FactRaw, protocolKey: "modbus-tcp", want: "iot.data.raw.v1.modbus-tcp"},
		{kind: FactCanonicalTelemetry, want: "iot.data.canonical.telemetry.v1"},
		{kind: FactCanonicalDeviceEvent, want: "iot.data.canonical.device-event.v1"},
		{kind: FactCanonicalLifecycle, want: "iot.data.canonical.lifecycle-observation.v1"},
		{kind: FactCanonicalCommandReply, want: "iot.data.canonical.command-reply-observation.v1"},
		{kind: FactCanonicalOutcome, want: "iot.data.canonical.outcome.v1"},
		{kind: FactQuarantineIngress, want: "iot.data.quarantine.ingress.v1"},
		{kind: FactQuarantineProtocol, want: "iot.data.quarantine.protocol.v1"},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()
			name, err := V1LogicalTopicName(test.kind, test.protocolKey)
			if err != nil {
				t.Fatalf("V1LogicalTopicName(%q) error = %v", test.kind, err)
			}
			if got := name.String(); got != test.want {
				t.Fatalf("V1LogicalTopicName(%q) = %q, want %q", test.kind, got, test.want)
			}
		})
	}

	if _, err := V1LogicalTopicName(FactKind("scene-event"), ""); err == nil {
		t.Fatal("V1LogicalTopicName(scene-event) error = nil, Scene semantics must remain unfrozen")
	}
}

// TestCatalogRequiresGenerationForOrderedOrStatefulTopics 验证有序或有状态 Topic 缺 generation 时失败关闭。
func TestCatalogRequiresGenerationForOrderedOrStatefulTopics(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		ordered  bool
		stateful bool
	}{
		{name: "有序", ordered: true},
		{name: "有状态", stateful: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			spec.Ordered = test.ordered
			spec.Stateful = test.stateful
			spec.Generation = Generation{}

			if _, err := NewTopic(spec); err == nil {
				t.Fatal("NewTopic() error = nil, want zero generation rejection")
			}
		})
	}
}

// TestCatalogRejectsOutOfRangeGeneration 验证 NewTopic 直接拒绝越界代数，即使物理名仍然有效。
func TestCatalogRejectsOutOfRangeGeneration(t *testing.T) {
	t.Parallel()

	spec := validTopicSpec(t, FactCanonicalTelemetry, "")
	spec.Generation = Generation{value: 100}
	if _, err := NewTopic(spec); err == nil {
		t.Fatal("NewTopic() error = nil, want out-of-range generation rejection")
	}
}

// TestCatalogRejectsMismatchedPhysicalName 验证物理名必须精确绑定逻辑名和同一代数。
func TestCatalogRejectsMismatchedPhysicalName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*TopicSpec)
	}{
		{
			name: "logical name",
			mutate: func(spec *TopicSpec) {
				spec.PhysicalName = PhysicalTopicName{value: "iot.data.quarantine.ingress.v1.g01"}
			},
		},
		{
			name: "generation",
			mutate: func(spec *TopicSpec) {
				spec.PhysicalName = PhysicalTopicName{value: spec.LogicalName.String() + ".g02"}
			},
		},
		{
			name: "generation padding",
			mutate: func(spec *TopicSpec) {
				spec.PhysicalName = PhysicalTopicName{value: spec.LogicalName.String() + ".g1"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			test.mutate(&spec)
			if _, err := NewTopic(spec); err == nil {
				t.Fatalf("NewTopic() error = nil for mismatched %s", test.name)
			}
		})
	}
}

// TestCatalogRejectsUnfrozenAndMismatchedFactKinds 验证 Topic 只接受 v1 已冻结且匹配名称模板的事实类型。
func TestCatalogRejectsUnfrozenAndMismatchedFactKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*TopicSpec)
	}{
		{
			name:   "scene-event",
			mutate: func(spec *TopicSpec) { spec.FactKind = FactKind("scene-event") },
		},
		{
			name:   "unknown",
			mutate: func(spec *TopicSpec) { spec.FactKind = FactKind("future-event") },
		},
		{
			name: "canonical template",
			mutate: func(spec *TopicSpec) {
				spec.FactKind = FactCanonicalDeviceEvent
			},
		},
		{
			name: "raw template",
			mutate: func(spec *TopicSpec) {
				spec.LogicalName = LogicalTopicName{value: "iot.data.raw.v1.modbus-tcp.extra"}
				spec.PhysicalName = PhysicalTopicName{value: "iot.data.raw.v1.modbus-tcp.extra.g01"}
				spec.FactKind = FactRaw
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			test.mutate(&spec)
			if _, err := NewTopic(spec); err == nil {
				t.Fatalf("NewTopic() error = nil for %s fact kind", test.name)
			}
		})
	}
}

// TestCatalogRejectsMissingRequiredMetadata 验证关键 codec、owner、schema 与 timestamp 元数据缺失时失败关闭。
func TestCatalogRejectsMissingRequiredMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*TopicSpec)
	}{
		{name: "key codec", mutate: func(spec *TopicSpec) { spec.KeyCodec = "" }},
		{name: "producer owner", mutate: func(spec *TopicSpec) { spec.ProducerOwner = "" }},
		{name: "consumer owner", mutate: func(spec *TopicSpec) { spec.ConsumerOwners = nil }},
		{name: "schema major", mutate: func(spec *TopicSpec) { spec.SchemaMajor = 0 }},
		{name: "timestamp policy", mutate: func(spec *TopicSpec) { spec.TimestampPolicy = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			test.mutate(&spec)
			if _, err := NewTopic(spec); err == nil {
				t.Fatalf("NewTopic() error = nil after removing %s", test.name)
			}
		})
	}
}

// TestCatalogRejectsBlankConsumerOwner 验证任意空或纯空白消费者 owner 都会失败关闭。
func TestCatalogRejectsBlankConsumerOwner(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		owners []string
	}{
		{name: "空 owner", owners: []string{"materializer", ""}},
		{name: "纯空白 owner", owners: []string{"materializer", " \t\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			spec.ConsumerOwners = test.owners
			if _, err := NewTopic(spec); err == nil {
				t.Fatalf("NewTopic() error = nil for ConsumerOwners %q", test.owners)
			}
		})
	}
}

// TestCatalogRejectsRawRetentionBelowMinimum 验证 Raw Topic 的保留时间不能低于七十二小时。
func TestCatalogRejectsRawRetentionBelowMinimum(t *testing.T) {
	t.Parallel()

	spec := validTopicSpec(t, FactRaw, "modbus-tcp")
	spec.Retention = 72*time.Hour - time.Nanosecond
	if _, err := NewTopic(spec); err == nil {
		t.Fatal("NewTopic() error = nil, want Raw retention rejection")
	}
}

// TestCatalogRejectsProductionDurabilityDowngrades 验证生产 Topic 不能降级 RF、minISR 或 auto-create。
func TestCatalogRejectsProductionDurabilityDowngrades(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*TopicSpec)
		wantError string
	}{
		{
			name: "RF=2",
			mutate: func(spec *TopicSpec) {
				spec.ReplicationFactor = 2
			},
			wantError: "production replication factor must be at least 3",
		},
		{
			name:      "minISR<2",
			mutate:    func(spec *TopicSpec) { spec.MinISR = 1 },
			wantError: "production min isr must be at least 2",
		},
		{
			name:      "auto-create",
			mutate:    func(spec *TopicSpec) { spec.AutoCreate = true },
			wantError: "production topic cannot enable auto-create",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			test.mutate(&spec)
			_, err := NewTopic(spec)
			if err == nil {
				t.Fatalf("NewTopic() error = nil for production %s", test.name)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("NewTopic() error = %q, want error containing %q", err, test.wantError)
			}
		})
	}
}

// TestCatalogRejectsZeroLocalDurability 验证本地 Topic 也必须使用正数副本和最小同步副本配置。
func TestCatalogRejectsZeroLocalDurability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*TopicSpec)
	}{
		{name: "RF=0", mutate: func(spec *TopicSpec) { spec.ReplicationFactor = 0 }},
		{name: "minISR=0", mutate: func(spec *TopicSpec) { spec.MinISR = 0 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			spec := validTopicSpec(t, FactCanonicalTelemetry, "")
			spec.DeploymentProfile = ProfileLocal
			spec.ReplicationFactor = 1
			spec.MinISR = 1
			test.mutate(&spec)
			if _, err := NewTopic(spec); err == nil {
				t.Fatalf("NewTopic() error = nil for local %s", test.name)
			}
		})
	}
}

// TestCatalogOrdersAndDigestsDeterministically 验证输入顺序不影响 Catalog 排序、摘要或 SHA-256。
func TestCatalogOrdersAndDigestsDeterministically(t *testing.T) {
	t.Parallel()

	raw := mustTopic(t, validTopicSpec(t, FactRaw, "modbus-tcp"))
	telemetrySpec := validTopicSpec(t, FactCanonicalTelemetry, "")
	telemetrySpec.ConsumerOwners = []string{"scene-admission", "materializer"}
	telemetry := mustTopic(t, telemetrySpec)

	left, err := NewCatalog([]Topic{raw, telemetry})
	if err != nil {
		t.Fatalf("NewCatalog(left) error = %v", err)
	}
	right, err := NewCatalog([]Topic{telemetry, raw})
	if err != nil {
		t.Fatalf("NewCatalog(right) error = %v", err)
	}

	entries := left.Entries()
	if got, want := entries[0].Spec().LogicalName.String(), "iot.data.canonical.telemetry.v1"; got != want {
		t.Fatalf("Entries()[0] logical name = %q, want %q", got, want)
	}
	if left.Summary() != right.Summary() {
		t.Fatalf("Summary() differs for equivalent input:\nleft=%s\nright=%s", left.Summary(), right.Summary())
	}
	if left.Digest() != right.Digest() {
		t.Fatalf("Digest() differs: left=%q right=%q", left.Digest(), right.Digest())
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(left.Summary())))
	if got := left.Digest(); got != wantDigest {
		t.Fatalf("Digest() = %q, want %q", got, wantDigest)
	}
}

// TestTopicDefensivelyCopiesConsumerOwners 验证 Topic 不与构造输入或 Spec 返回值共享 owner slice。
func TestTopicDefensivelyCopiesConsumerOwners(t *testing.T) {
	t.Parallel()

	owners := []string{"materializer", "scene-admission"}
	spec := validTopicSpec(t, FactCanonicalTelemetry, "")
	spec.ConsumerOwners = owners
	topic := mustTopic(t, spec)
	wantOwners := "materializer,scene-admission"

	owners[0] = "mutated-input"
	if got := strings.Join(topic.Spec().ConsumerOwners, ","); got != wantOwners {
		t.Fatalf("Topic owners after input mutation = %q, want %q", got, wantOwners)
	}

	returnedSpec := topic.Spec()
	returnedSpec.ConsumerOwners[0] = "mutated-spec"
	if got := strings.Join(topic.Spec().ConsumerOwners, ","); got != wantOwners {
		t.Fatalf("Topic owners after Spec mutation = %q, want %q", got, wantOwners)
	}
}

// TestCatalogDefensivelyCopiesTopics 验证 Catalog 不与输入或 Entries 返回值共享 Topic 内部 slice。
func TestCatalogDefensivelyCopiesTopics(t *testing.T) {
	t.Parallel()

	spec := validTopicSpec(t, FactCanonicalTelemetry, "")
	spec.ConsumerOwners = []string{"materializer", "scene-admission"}
	input := []Topic{mustTopic(t, spec)}
	catalog, err := NewCatalog(input)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	wantSummary := catalog.Summary()
	wantDigest := catalog.Digest()
	wantOwners := "materializer,scene-admission"

	input[0].spec.ConsumerOwners[0] = "mutated-input"
	if got := strings.Join(catalog.Entries()[0].Spec().ConsumerOwners, ","); got != wantOwners {
		t.Fatalf("Catalog owners after input mutation = %q, want %q", got, wantOwners)
	}

	entries := catalog.Entries()
	entries[0].spec.ConsumerOwners[0] = "mutated-entries"
	if got := strings.Join(catalog.Entries()[0].Spec().ConsumerOwners, ","); got != wantOwners {
		t.Fatalf("Catalog owners after Entries mutation = %q, want %q", got, wantOwners)
	}
	if got := catalog.Summary(); got != wantSummary {
		t.Fatalf("Summary() changed after external mutations:\ngot=%s\nwant=%s", got, wantSummary)
	}
	if got := catalog.Digest(); got != wantDigest {
		t.Fatalf("Digest() changed after external mutations: got=%q want=%q", got, wantDigest)
	}
}

// TestCatalogSummaryGolden 验证稳定摘要的字段、类型、单位及已知 SHA-256。
func TestCatalogSummaryGolden(t *testing.T) {
	t.Parallel()

	topic := mustTopic(t, validTopicSpec(t, FactRaw, "modbus-tcp"))
	catalog, err := NewCatalog([]Topic{topic})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	const wantSummary = `{"entries":[{"logical_name":"iot.data.raw.v1.modbus-tcp","physical_name":"iot.data.raw.v1.modbus-tcp.g01","generation":"1","plane":"data","fact_kind":"raw","ordered":true,"stateful":false,"schema_major":1,"partitions":12,"key_codec":"canonical-device-key-v1","cleanup_policy":"delete","retention_ns":259200000000000,"timestamp_policy":"log-append-time","producer_owner":"protocol-application","consumer_owners":["materializer"],"acl_profile":"data-plane-v1","deployment_profile":"production","replication_factor":3,"min_isr":2,"auto_create":false}]}`
	const wantDigest = "cd7f29a09b2b29245eaa58890a1ed80f77d2d1cedd38ad0559ea0daf77062f3c"
	if got := catalog.Summary(); got != wantSummary {
		t.Fatalf("Summary() = %s, want %s", got, wantSummary)
	}
	if got := catalog.Digest(); got != wantDigest {
		t.Fatalf("Digest() = %q, want %q", got, wantDigest)
	}
}

// validTopicSpec 返回符合生产基线的 TopicSpec，供单一失败条件测试复用。
func validTopicSpec(t *testing.T, kind FactKind, protocolKey string) TopicSpec {
	t.Helper()

	logical, err := V1LogicalTopicName(kind, protocolKey)
	if err != nil {
		t.Fatalf("V1LogicalTopicName(%q) error = %v", kind, err)
	}
	generation, err := NewGeneration(1)
	if err != nil {
		t.Fatalf("NewGeneration() error = %v", err)
	}
	physical, err := NewPhysicalTopicName(logical, generation)
	if err != nil {
		t.Fatalf("NewPhysicalTopicName() error = %v", err)
	}

	retention := 48 * time.Hour
	if kind == FactRaw {
		retention = 72 * time.Hour
	}
	if kind == FactQuarantineIngress || kind == FactQuarantineProtocol {
		retention = 7 * 24 * time.Hour
	}

	return TopicSpec{
		LogicalName:       logical,
		PhysicalName:      physical,
		Generation:        generation,
		Plane:             PlaneData,
		FactKind:          kind,
		Ordered:           true,
		SchemaMajor:       1,
		Partitions:        12,
		KeyCodec:          "canonical-device-key-v1",
		CleanupPolicy:     CleanupDelete,
		Retention:         retention,
		TimestampPolicy:   TimestampLogAppendTime,
		ProducerOwner:     "protocol-application",
		ConsumerOwners:    []string{"materializer"},
		ACLProfile:        "data-plane-v1",
		DeploymentProfile: ProfileProduction,
		ReplicationFactor: 3,
		MinISR:            2,
		AutoCreate:        false,
	}
}

// mustTopic 构造测试 Topic，并把非预期错误升级为测试失败。
func mustTopic(t *testing.T, spec TopicSpec) Topic {
	t.Helper()

	topic, err := NewTopic(spec)
	if err != nil {
		t.Fatalf("NewTopic() error = %v", err)
	}
	return topic
}
