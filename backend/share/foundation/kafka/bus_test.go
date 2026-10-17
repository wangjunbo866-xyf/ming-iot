// 本文件验证协调消费者的完成确认、失败前缀、代际隔离与并发退出边界。
package kafka

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "gitee.com/ming-iot/share/contracts/eventbus"
	"github.com/twmb/franz-go/pkg/kgo"
)

// busFakeClient 仅替代协议边界，不声明已经验证真实 Kafka 行为。
type busFakeClient struct {
	commits     []int64
	commitError error
	pingError   error
	fetches     kgo.Fetches
	pollEntered chan struct{}
	closed      atomic.Int32
}

// Ping 返回可控制的连接结果。
func (f *busFakeClient) Ping(context.Context) error { return f.pingError }

// PollRecords 返回一次数据后等待取消。
func (f *busFakeClient) PollRecords(ctx context.Context, _ int) kgo.Fetches {
	if f.pollEntered != nil {
		select {
		case f.pollEntered <- struct{}{}:
		default:
		}
	}
	if f.fetches != nil {
		result := f.fetches
		f.fetches = nil
		return result
	}
	<-ctx.Done()
	return nil
}

// CommitRecords 记录真实执行顺序中的提交位置。
func (f *busFakeClient) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	if f.commitError != nil {
		return f.commitError
	}
	for _, record := range records {
		f.commits = append(f.commits, record.Offset)
	}
	return nil
}

// AllowRebalance 保留假客户端所需的释放接口。
func (f *busFakeClient) AllowRebalance() {}

// Close 统计关闭次数以检查并发生命周期。
func (f *busFakeClient) Close() { f.closed.Add(1) }

// newBusTestConsumer 创建只用于单元边界的独立消费者。
func newBusTestConsumer(fake *busFakeClient) *BusConsumer {
	return &BusConsumer{client: fake, config: BusConsumerConfig{RequestTimeout: time.Second, PollTimeout: 5 * time.Millisecond, MaxPollRecords: 2}, generation: 1, done: make(chan struct{})}
}

// busTestRecords 创建同分区同键的连续记录。
func busTestRecords() []*kgo.Record {
	return []*kgo.Record{{Topic: contract.CoordinationTopic, Partition: 0, Offset: 10, Key: []byte("same")}, {Topic: contract.CoordinationTopic, Partition: 0, Offset: 11, Key: []byte("same")}, {Topic: contract.CoordinationTopic, Partition: 0, Offset: 12, Key: []byte("same")}}
}

// TestBusCompletionAndFailurePrefix 证明前一回调完成才提交，失败后不会执行同键后继副作用。
func TestBusCompletionAndFailurePrefix(t *testing.T) {
	fake := &busFakeClient{}
	consumer := newBusTestConsumer(fake)
	expected := errors.New("callback failed")
	var handled []int64
	err := consumer.processBatch(context.Background(), busTestRecords(), func(_ context.Context, record *kgo.Record) error {
		handled = append(handled, record.Offset)
		if record.Offset == 10 && len(fake.commits) != 0 {
			t.Fatal("committed before callback completion")
		}
		if record.Offset == 11 {
			if !reflect.DeepEqual(fake.commits, []int64{10}) {
				t.Fatal("preceding completion not committed")
			}
			return expected
		}
		return nil
	})
	if !errors.Is(err, expected) || !reflect.DeepEqual(fake.commits, []int64{10}) || !reflect.DeepEqual(handled, []int64{10, 11}) {
		t.Fatalf("err=%v commits=%v handled=%v", err, fake.commits, handled)
	}
}

// TestBusCancelRebalanceAndCommitFailure 不允许取消、重分配或提交失败越过当前未确认记录。
func TestBusCancelRebalanceAndCommitFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "rebalance", "commit"} {
		t.Run(mode, func(t *testing.T) {
			fake := &busFakeClient{}
			consumer := newBusTestConsumer(fake)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			err := consumer.processBatch(ctx, busTestRecords(), func(context.Context, *kgo.Record) error {
				calls++
				switch mode {
				case "cancel":
					cancel()
				case "rebalance":
					consumer.assignment([]int32{0}, false)
				case "commit":
					fake.commitError = errors.New("commit failed")
				}
				return nil
			})
			if err == nil || calls != 1 || len(fake.commits) != 0 {
				t.Fatalf("err=%v calls=%d commits=%v", err, calls, fake.commits)
			}
		})
	}
}

// TestBusStateRequiresConfirmedSubscription 检查真实连接成功之外还必须确认订阅及消费位置。
func TestBusStateRequiresConfirmedSubscription(t *testing.T) {
	consumer := newBusTestConsumer(&busFakeClient{})
	generation, _ := consumer.CacheState()
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); ready {
		t.Fatal("construction reported ready")
	}
	consumer.assignment([]int32{0}, true)
	generation, _ = consumer.CacheState()
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); ready {
		t.Fatal("assignment without offsets reported ready")
	}
	consumer.mu.Lock()
	consumer.offsetsFetched = true
	consumer.mu.Unlock()
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); !ready {
		t.Fatal("confirmed subscription did not become ready")
	}
	consumer.invalidate(errors.New("disconnect"))
	newer, ready := consumer.CacheState()
	if newer <= generation || ready || consumer.LastError() == nil {
		t.Fatal("disconnect did not isolate cache")
	}
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); ready {
		t.Fatal("stale connection check restored readiness")
	}
}

// TestBusConcurrentCloseCancelsHandler 验证 Close 等待同步回调取消且重复关闭不会重复释放客户端。
func TestBusConcurrentCloseCancelsHandler(t *testing.T) {
	fake := &busFakeClient{fetches: kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: contract.CoordinationTopic, Partitions: []kgo.FetchPartition{{Partition: 0, Records: busTestRecords()}}}}}}}
	consumer := newBusTestConsumer(fake)
	entered := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- consumer.Run(context.Background(), func(ctx context.Context, _ *kgo.Record) error { close(entered); <-ctx.Done(); return ctx.Err() })
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = consumer.Close() }()
	}
	wg.Wait()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error=%v", err)
	}
	if fake.closed.Load() != 1 || len(fake.commits) != 0 {
		t.Fatalf("close=%d commits=%v", fake.closed.Load(), fake.commits)
	}
	if err := consumer.Run(context.Background(), func(context.Context, *kgo.Record) error { return nil }); err == nil {
		t.Fatal("restarted closed consumer")
	}
}

// TestBusRejectsUncontrolledConfig 确保协调入口不放松安全配置和物理主题限制。
func TestBusRejectsUncontrolledConfig(t *testing.T) {
	config := BusConsumerConfig{Client: ClientConfig{Profile: ProfileLocal, Brokers: []string{"localhost:9092"}, ClientID: "bus-test"}, GroupID: "bus.test", Topic: contract.CoordinationTopic}
	for _, mutate := range []func(*BusConsumerConfig){func(c *BusConsumerConfig) { c.Topic = "telemetry.raw" }, func(c *BusConsumerConfig) { c.GroupID = "" }, func(c *BusConsumerConfig) { c.MaxPollRecords = 1025 }, func(c *BusConsumerConfig) { c.RequestTimeout = -time.Second }, func(c *BusConsumerConfig) { c.Client.Profile = ProfileProduction }} {
		changed := config
		mutate(&changed)
		if c, err := NewBusConsumer(changed); err == nil {
			_ = c.Close()
			t.Fatal("accepted invalid config")
		}
	}
}

// TestBusEffectiveSafetyOptions 验证新入口确实禁用自动提交、固定隔离并限制预取，不改变数据面入口。
func TestBusEffectiveSafetyOptions(t *testing.T) {
	consumer, err := NewBusConsumer(BusConsumerConfig{Client: ClientConfig{Profile: ProfileLocal, Brokers: []string{"127.0.0.1:1"}, ClientID: "bus-options"}, GroupID: "bus.options", Topic: contract.CoordinationTopic})
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	client := consumer.client.(*kgo.Client)
	if client.OptValue(kgo.DisableAutoCommit) != true || client.OptValue(kgo.BlockRebalanceOnPoll) != true {
		t.Fatal("unsafe commit/rebalance options")
	}
	if client.OptValue(kgo.AllowAutoTopicCreation) != false {
		t.Fatal("automatic topic creation enabled")
	}
	if client.OptValue(kgo.FetchIsolationLevel) != int8(1) {
		t.Fatal("uncommitted records visible")
	}
	if client.OptValue(kgo.MaxConcurrentFetches) != 1 {
		t.Fatal("unbounded concurrent fetching")
	}
	if got := client.OptValue(kgo.FetchMaxBytes); got != int32(2*contract.MaxEnvelopeBytes) {
		t.Fatalf("fetch bytes=%v", got)
	}
}

// TestBusCooperativeAssignmentRetainsPartitions 验证协作式局部撤销不会忘记仍拥有的分区。
func TestBusCooperativeAssignmentRetainsPartitions(t *testing.T) {
	consumer := newBusTestConsumer(&busFakeClient{})
	consumer.assignment([]int32{0, 1}, true)
	consumer.assignment([]int32{1}, false)
	consumer.assignment(nil, true)
	consumer.mu.Lock()
	retained := consumer.assigned && len(consumer.partitions) == 1
	consumer.offsetsFetched = true
	consumer.mu.Unlock()
	if !retained {
		t.Fatal("partial revoke forgot retained partition")
	}
	generation, _ := consumer.CacheState()
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); !ready {
		t.Fatal("retained subscription never recovers")
	}
	consumer.assignment([]int32{0}, false)
	generation, _ = consumer.CacheState()
	consumer.markReady(generation)
	if _, ready := consumer.CacheState(); ready {
		t.Fatal("fully revoked subscription is ready")
	}
}
