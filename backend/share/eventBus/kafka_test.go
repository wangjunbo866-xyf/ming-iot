// 本文件验证缓存适配器的同步回调、错误传播、受控路由及显式实例隔离。
package eventBus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	contract "gitee.com/ming-iot/share/contracts/eventbus"
	foundation "gitee.com/ming-iot/share/foundation/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
)

// kafkaTestRecord 构造带合法键的受控缓存失效记录。
func kafkaTestRecord(t *testing.T) *kgo.Record {
	t.Helper()
	now := time.Now().UTC()
	event, err := contract.NewCacheInvalidation(contract.Metadata{EventID: "test-event", Source: "test", OccurredAt: now, AcceptedAt: now, Scope: contract.ScopeSystem, CorrelationID: "test-operation"}, contract.CacheInvalidation{CacheType: "unit", Key: "cache.key.1"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := contract.Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	key, err := event.PartitionKey()
	if err != nil {
		t.Fatal(err)
	}
	return &kgo.Record{Topic: contract.CoordinationTopic, Key: key, Value: body}
}

// TestKafkaSynchronousCompletionAndErrors 验证适配器等待同步完成，失败停止后继处理器。
func TestKafkaSynchronousCompletionAndErrors(t *testing.T) {
	bus := &KafkaBus{handlers: make(map[string][]FastFunc)}
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	expected := errors.New("handler failure")
	calls := 0
	if err := bus.Subscribe("server.cache.sync.unit", func(_ context.Context, _ time.Time, body []byte) error {
		close(entered)
		<-release
		if string(body) != "cache.key.1" {
			t.Error("legacy body changed")
		}
		return expected
	}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Subscribe("server.cache.sync.unit", func(context.Context, time.Time, []byte) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	record := kafkaTestRecord(t)
	go func() { result <- bus.handleRecord(context.Background(), record) }()
	<-entered
	select {
	case err := <-result:
		t.Fatalf("returned before callback completed: %v", err)
	default:
	}
	close(release)
	if err := <-result; !errors.Is(err, expected) {
		t.Fatalf("error=%v", err)
	}
	if calls != 0 {
		t.Fatal("executed handler after failure")
	}
}

// TestKafkaRejectsMalformedRecordBeforeCallback 验证错误信封和错误键不能调用业务回调。
func TestKafkaRejectsMalformedRecordBeforeCallback(t *testing.T) {
	for _, mode := range []string{"body", "key", "topic", "panic"} {
		t.Run(mode, func(t *testing.T) {
			bus := &KafkaBus{handlers: make(map[string][]FastFunc)}
			calls := 0
			_ = bus.Subscribe("server.cache.sync.unit", func(context.Context, time.Time, []byte) error {
				calls++
				if mode == "panic" {
					panic("private data")
				}
				return nil
			})
			record := kafkaTestRecord(t)
			switch mode {
			case "body":
				record.Value = []byte(`{"type":"unknown"}`)
			case "key":
				record.Key = []byte("forged")
			case "topic":
				record.Topic = "telemetry.raw"
			}
			if err := bus.handleRecord(context.Background(), record); err == nil {
				t.Fatal("accepted invalid record or panic")
			}
			if mode != "panic" && calls != 0 {
				t.Fatal("invalid record reached handler")
			}
		})
	}
}

// TestKafkaIndependentInstancesAndClosedPublish 验证没有借用 FastEvent 单例，关闭和发布错误明确可见。
func TestKafkaIndependentInstancesAndClosedPublish(t *testing.T) {
	config := KafkaBusConfig{Client: foundation.ClientConfig{Profile: foundation.ProfileLocal, Brokers: []string{"127.0.0.1:1"}, ClientID: "kafka-unit"}, Role: "unit", Instance: "a", RequestTimeout: 100 * time.Millisecond}
	first, err := NewKafkaBus(config)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	config.Instance = "b"
	second, err := NewKafkaBus(config)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first == second || first.consumer == second.consumer {
		t.Fatal("shared bus identity")
	}
	if _, ready := first.CacheState(); ready {
		t.Fatal("constructor reported ready")
	}
	if err := first.Subscribe("server.cache.sync.*", func(context.Context, time.Time, []byte) error { return nil }); err == nil {
		t.Fatal("opened arbitrary selector")
	}
	if err := first.Publish(context.Background(), "server.cache.sync.unit", 123); err == nil {
		t.Fatal("accepted ambiguous cache key")
	}
	started := time.Now()
	if err := first.Publish(context.Background(), "server.cache.sync.unit", "key"); err == nil {
		t.Fatal("unavailable broker succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatal("publish exceeded bound")
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = first.Close() }()
	}
	wg.Wait()
	if err := first.Publish(context.Background(), "server.cache.sync.unit", "key"); err == nil {
		t.Fatal("closed publish succeeded")
	}
	if err := second.Subscribe("server.cache.sync.unit", func(context.Context, time.Time, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

// TestKafkaSystemAdapterRejectsTenantEnvelope 防止无租户隔离的缓存适配器执行租户信封。
func TestKafkaSystemAdapterRejectsTenantEnvelope(t *testing.T) {
	record := kafkaTestRecord(t)
	event, err := contract.Decode(record.Value)
	if err != nil {
		t.Fatal(err)
	}
	event.Scope = contract.ScopeTenant
	event.TenantID = "tenant-a"
	record.Value, err = contract.Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	record.Key, err = event.PartitionKey()
	if err != nil {
		t.Fatal(err)
	}
	bus := &KafkaBus{handlers: make(map[string][]FastFunc)}
	calls := 0
	_ = bus.Subscribe("server.cache.sync.unit", func(context.Context, time.Time, []byte) error { calls++; return nil })
	if err := bus.handleRecord(context.Background(), record); err == nil || calls != 0 {
		t.Fatalf("tenant callback calls=%d error=%v", calls, err)
	}
	if err := bus.PublishEnvelope(context.Background(), event); err == nil {
		t.Fatal("tenant publish accepted")
	}
}
