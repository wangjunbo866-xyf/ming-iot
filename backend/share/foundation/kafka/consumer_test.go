// 本文件验证 read-committed consumer 的冻结 API、受管 Topic 与安全消费选项。
package kafka

import (
	"crypto/tls"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	contract "gitee.com/ming-iot/share/contracts/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

func TestNewReadCommittedConsumerAppliesEffectiveOptions(t *testing.T) {
	first := mustConsumerTopic(t, contract.FactRaw, "modbus-tcp")
	second := mustConsumerTopic(t, contract.FactCanonicalTelemetry, "")
	config := ConsumerConfig{
		Client: ClientConfig{
			Profile:       ProfileProduction,
			Brokers:       []string{"kafka-1.example.com:9093", "kafka-2.example.com:9093"},
			ClientID:      "iot.consumer",
			Principal:     "svc-iot",
			TLSConfig:     &tls.Config{MinVersion: tls.VersionTLS13},
			SASLMechanism: plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism(),
		},
		GroupID: "materializer.v1",
		Topics:  []contract.Topic{first, second},
	}

	client, err := NewReadCommittedConsumer(config)
	if err != nil {
		t.Fatalf("NewReadCommittedConsumer() error = %v", err)
	}
	defer client.Close()

	wantTopics := map[string]*regexp.Regexp{
		first.Spec().PhysicalName.String():  nil,
		second.Spec().PhysicalName.String(): nil,
	}
	if got := client.OptValue(kgo.ConsumeTopics); !reflect.DeepEqual(got, wantTopics) {
		t.Fatalf("ConsumeTopics = %#v, want %#v", got, wantTopics)
	}
	if got := client.OptValue(kgo.ConsumerGroup); got != config.GroupID {
		t.Fatalf("ConsumerGroup = %#v, want %q", got, config.GroupID)
	}
	if got := client.OptValue(kgo.GroupProtocol); got != "consumer" {
		t.Fatalf("GroupProtocol = %#v, want consumer", got)
	}
	balancers, ok := client.OptValue(kgo.Balancers).([]kgo.GroupBalancer)
	if !ok || len(balancers) != 1 || balancers[0].ProtocolName() != "cooperative-sticky" {
		t.Fatalf("Balancers = %#v, want one cooperative-sticky balancer", client.OptValue(kgo.Balancers))
	}
	if got, ok := client.OptValue(kgo.FetchIsolationLevel).(int8); !ok || got != 1 {
		t.Fatalf("FetchIsolationLevel = %#v, want read committed", client.OptValue(kgo.FetchIsolationLevel))
	}
	if got := client.OptValue(kgo.DisableAutoCommit); got != true {
		t.Fatalf("DisableAutoCommit = %#v, want true", got)
	}
	if got := client.OptValue(kgo.ConsumeResetOffset); !reflect.DeepEqual(got, kgo.NoResetOffset()) {
		t.Fatalf("ConsumeResetOffset = %#v, want NoResetOffset", got)
	}
	if got := client.OptValue(kgo.AllowAutoTopicCreation); got != false {
		t.Fatalf("AllowAutoTopicCreation = %#v, want false", got)
	}
}

func TestNewReadCommittedConsumerUsesAtStartForLocal(t *testing.T) {
	config := ConsumerConfig{
		Client: ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:9092"},
			ClientID: "iot.local.consumer",
		},
		GroupID: "local-materializer",
		Topics:  []contract.Topic{mustConsumerTopic(t, contract.FactCanonicalTelemetry, "")},
	}

	client, err := NewReadCommittedConsumer(config)
	if err != nil {
		t.Fatalf("NewReadCommittedConsumer() error = %v", err)
	}
	defer client.Close()

	if got := client.OptValue(kgo.ConsumeResetOffset); !reflect.DeepEqual(got, kgo.NewOffset().AtStart()) {
		t.Fatalf("local ConsumeResetOffset = %#v, want AtStart", got)
	}
}

func TestNewReadCommittedConsumerRejectsInvalidConfiguration(t *testing.T) {
	base := func() ConsumerConfig {
		return ConsumerConfig{
			Client: ClientConfig{
				Profile:  ProfileLocal,
				Brokers:  []string{"127.0.0.1:9092"},
				ClientID: "iot.consumer",
			},
			GroupID: "materializer.v1",
			Topics:  []contract.Topic{mustConsumerTopic(t, contract.FactCanonicalTelemetry, "")},
		}
	}

	tests := []struct {
		name string
		edit func(*ConsumerConfig)
	}{
		{name: "invalid profile", edit: func(config *ConsumerConfig) { config.Client.Profile = Profile("staging") }},
		{name: "missing group", edit: func(config *ConsumerConfig) { config.GroupID = "" }},
		{name: "unstable group", edit: func(config *ConsumerConfig) { config.GroupID = "materializer group" }},
		{name: "zero topics", edit: func(config *ConsumerConfig) { config.Topics = nil }},
		{name: "invalid topic", edit: func(config *ConsumerConfig) { config.Topics = []contract.Topic{{}} }},
		{name: "duplicate physical topic", edit: func(config *ConsumerConfig) { config.Topics = append(config.Topics, config.Topics[0]) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base()
			test.edit(&config)
			client, err := NewReadCommittedConsumer(config)
			if err == nil {
				if client != nil {
					client.Close()
				}
				t.Fatal("NewReadCommittedConsumer() error = nil, want rejection")
			}
			if client != nil {
				client.Close()
				t.Fatal("NewReadCommittedConsumer() returned a client with an error")
			}
		})
	}
}

func TestNewReadCommittedConsumerWrapsClientConstructionError(t *testing.T) {
	config := ConsumerConfig{
		Client: ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:not-a-port"},
			ClientID: "iot.consumer",
		},
		GroupID: "materializer.v1",
		Topics:  []contract.Topic{mustConsumerTopic(t, contract.FactCanonicalTelemetry, "")},
	}

	client, err := NewReadCommittedConsumer(config)
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("NewReadCommittedConsumer() error = nil, want construction error")
	}
	if client != nil {
		client.Close()
		t.Fatal("NewReadCommittedConsumer() returned a client with an error")
	}
	if !strings.Contains(err.Error(), "create read-committed Kafka consumer") {
		t.Fatalf("NewReadCommittedConsumer() error = %v, want contextual construction error", err)
	}
}

func TestNewReadCommittedConsumerHasFrozenSignature(t *testing.T) {
	functionType := reflect.TypeOf(NewReadCommittedConsumer)
	if functionType.IsVariadic() || functionType.NumIn() != 1 || functionType.In(0) != reflect.TypeOf(ConsumerConfig{}) {
		t.Fatalf("NewReadCommittedConsumer input signature = %v, want func(ConsumerConfig)", functionType)
	}
	if functionType.NumOut() != 2 || functionType.Out(0) != reflect.TypeOf((*kgo.Client)(nil)) || functionType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("NewReadCommittedConsumer output signature = %v, want (*kgo.Client, error)", functionType)
	}
}

// mustConsumerTopic 构造满足 Kafka Topic 契约的测试 Topic。
func mustConsumerTopic(t *testing.T, kind contract.FactKind, protocolKey string) contract.Topic {
	t.Helper()

	logical, err := contract.V1LogicalTopicName(kind, protocolKey)
	if err != nil {
		t.Fatalf("V1LogicalTopicName() error = %v", err)
	}
	generation, err := contract.NewGeneration(1)
	if err != nil {
		t.Fatalf("NewGeneration() error = %v", err)
	}
	physical, err := contract.NewPhysicalTopicName(logical, generation)
	if err != nil {
		t.Fatalf("NewPhysicalTopicName() error = %v", err)
	}

	retention := 48 * time.Hour
	if kind == contract.FactRaw {
		retention = 72 * time.Hour
	}
	if kind == contract.FactQuarantineIngress || kind == contract.FactQuarantineProtocol {
		retention = 7 * 24 * time.Hour
	}
	topic, err := contract.NewTopic(contract.TopicSpec{
		LogicalName:       logical,
		PhysicalName:      physical,
		Generation:        generation,
		Plane:             contract.PlaneData,
		FactKind:          kind,
		Ordered:           true,
		SchemaMajor:       1,
		Partitions:        12,
		KeyCodec:          "canonical-device-key-v1",
		CleanupPolicy:     contract.CleanupDelete,
		Retention:         retention,
		TimestampPolicy:   contract.TimestampLogAppendTime,
		ProducerOwner:     "protocol-application",
		ConsumerOwners:    []string{"materializer"},
		ACLProfile:        "data-plane-v1",
		DeploymentProfile: contract.ProfileProduction,
		ReplicationFactor: 3,
		MinISR:            2,
		AutoCreate:        false,
	})
	if err != nil {
		t.Fatalf("NewTopic() error = %v", err)
	}
	return topic
}
