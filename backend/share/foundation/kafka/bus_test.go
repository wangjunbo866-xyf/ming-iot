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
	"github.com/twmb/franz-go/pkg/kmsg"
)

// busFakeClient 仅替代协议边界，不声明已经验证真实 Kafka 行为。
type busFakeClient struct {
	commits     []int64
	commitError error
	pingError   error
	fetches     kgo.Fetches
	pollEntered chan struct{}
	closed      atomic.Int32
	// requestSharded 提供可控的不同分区 leader 响应，不能替代真实集群证据。
	requestSharded func(context.Context, kmsg.Request) []kgo.ResponseShard
}

// Ping 返回可控制的连接结果。
func (f *busFakeClient) Ping(context.Context) error { return f.pingError }

// RequestSharded 返回按 leader 划分的响应，用于检查局部可达性和代际竞态。
func (f *busFakeClient) RequestSharded(ctx context.Context, request kmsg.Request) []kgo.ResponseShard {
	if f.requestSharded == nil {
		return nil
	}
	return f.requestSharded(ctx, request)
}

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
	return &BusConsumer{client: fake, config: BusConsumerConfig{Topic: contract.CoordinationTopic, GroupID: "bus-test", RequestTimeout: time.Second, PollTimeout: 5 * time.Millisecond, MaxPollRecords: 2}, generation: 1, done: make(chan struct{})}
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
	if err := consumer.confirmOffsets(busOffsetsResponse(0)); err != nil {
		t.Fatal(err)
	}
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

// busOffsetsResponse 构造与 Kafka 位点回调一致的成功响应。
func busOffsetsResponse(partitions ...int32) *kmsg.OffsetFetchResponse {
	response := &kmsg.OffsetFetchResponse{Topics: []kmsg.OffsetFetchResponseTopic{{Topic: contract.CoordinationTopic}}}
	for _, partition := range partitions {
		response.Topics[0].Partitions = append(response.Topics[0].Partitions, kmsg.OffsetFetchResponseTopicPartition{Partition: partition, Offset: 0})
	}
	return response
}

// TestBusCooperativeAssignmentRetainsPartitions 按真实回调次序验证空增量没有位点回调时仍能恢复。
func TestBusCooperativeAssignmentRetainsPartitions(t *testing.T) {
	consumer, err := NewBusConsumer(BusConsumerConfig{Client: ClientConfig{Profile: ProfileLocal, Brokers: []string{"127.0.0.1:1"}, ClientID: "bus-callbacks"}, GroupID: "bus.callbacks", Topic: contract.CoordinationTopic})
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	client := consumer.client.(*kgo.Client)
	assigned := client.OptValue(kgo.OnPartitionsAssigned).(func(context.Context, *kgo.Client, map[string][]int32))
	revoked := client.OptValue(kgo.OnPartitionsRevoked).(func(context.Context, *kgo.Client, map[string][]int32))
	lost := client.OptValue(kgo.OnPartitionsLost).(func(context.Context, *kgo.Client, map[string][]int32))
	fetched := client.OptValue(kgo.OnOffsetsFetched).(func(context.Context, *kgo.Client, *kmsg.OffsetFetchResponse) error)
	ctx := context.Background()
	// checkReady 模拟成功的本代际全部分区 leader 检查，观察实际回调产生的位点确认状态。
	checkReady := func(want bool) {
		t.Helper()
		generation, _ := consumer.CacheState()
		consumer.markReady(generation)
		if _, ready := consumer.CacheState(); ready != want {
			t.Fatalf("ready=%v want=%v", ready, want)
		}
	}
	// 初次分配必须取得全部分区的位置；局部响应不能让未确认分区就绪。
	assigned(ctx, client, map[string][]int32{contract.CoordinationTopic: {0, 1}})
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(0)); err != nil {
		t.Fatal(err)
	}
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(1)); err != nil {
		t.Fatal(err)
	}
	checkReady(true)
	// 无新增分区时 franz-go 不调用 OnOffsetsFetched，必须保留已有确认。
	assigned(ctx, client, nil)
	checkReady(true)
	revoked(ctx, client, map[string][]int32{contract.CoordinationTopic: {1}})
	assigned(ctx, client, nil)
	checkReady(true)
	// 新增分区重新阻止就绪，直到实际位点回调确认。
	assigned(ctx, client, map[string][]int32{contract.CoordinationTopic: {2}})
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(2)); err != nil {
		t.Fatal(err)
	}
	checkReady(true)
	// 未确认的新分区被撤销后，不应阻断保留分区恢复。
	assigned(ctx, client, map[string][]int32{contract.CoordinationTopic: {3}})
	checkReady(false)
	revoked(ctx, client, map[string][]int32{contract.CoordinationTopic: {3}})
	assigned(ctx, client, nil)
	checkReady(true)
	// 全撤销及 lost 都不能因空回调或旧位点响应恢复。
	revoked(ctx, client, map[string][]int32{contract.CoordinationTopic: {0, 2}})
	assigned(ctx, client, nil)
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(0, 2)); err != nil {
		t.Fatal(err)
	}
	checkReady(false)
	assigned(ctx, client, map[string][]int32{contract.CoordinationTopic: {0}})
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(0)); err != nil {
		t.Fatal(err)
	}
	checkReady(true)
	lost(ctx, client, nil)
	checkReady(false)
	assigned(ctx, client, nil)
	checkReady(false)
	assigned(ctx, client, map[string][]int32{contract.CoordinationTopic: {0}})
	checkReady(false)
	if err := fetched(ctx, client, busOffsetsResponse(0)); err != nil {
		t.Fatal(err)
	}
	checkReady(true)
}

// busLeaderResponse 构造某个 broker 对指定分区的真实 ListOffsets 响应形状。
func busLeaderResponse(nodeID int32, partitions ...int32) kgo.ResponseShard {
	response := &kmsg.ListOffsetsResponse{Topics: []kmsg.ListOffsetsResponseTopic{{Topic: contract.CoordinationTopic}}}
	for _, partition := range partitions {
		response.Topics[0].Partitions = append(response.Topics[0].Partitions, kmsg.ListOffsetsResponseTopicPartition{Partition: partition, Offset: 20})
	}
	return kgo.ResponseShard{Meta: kgo.BrokerMetadata{NodeID: nodeID}, Resp: response}
}

// TestBusReadinessRequiresEveryOwnedLeader 验证一个 broker 健康不代表另一个分区 leader 可服务。
func TestBusReadinessRequiresEveryOwnedLeader(t *testing.T) {
	for _, mode := range []string{"unreachable-leader", "partition-error", "negative-offset", "missing-partition", "wrong-topic", "duplicate-partition", "generation-changed", "all-leaders-recovered"} {
		t.Run(mode, func(t *testing.T) {
			fake := &busFakeClient{}
			consumer := newBusTestConsumer(fake)
			consumer.assignment([]int32{0, 1}, true)
			if err := consumer.confirmOffsets(busOffsetsResponse(0, 1)); err != nil {
				t.Fatal(err)
			}
			// 先确认已有两个 leader 健康，再模拟断连而保留消费位点确认。
			fake.requestSharded = func(context.Context, kmsg.Request) []kgo.ResponseShard {
				return []kgo.ResponseShard{busLeaderResponse(1, 0), busLeaderResponse(2, 1)}
			}
			if err := consumer.checkReadiness(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, ready := consumer.CacheState(); !ready {
				t.Fatal("initial leaders did not become ready")
			}
			consumer.invalidate(errors.New("connection lost"))
			if err := fake.Ping(context.Background()); err != nil {
				t.Fatal("healthy metadata broker unavailable")
			}
			fake.requestSharded = func(_ context.Context, request kmsg.Request) []kgo.ResponseShard {
				offsets, ok := request.(*kmsg.ListOffsetsRequest)
				if !ok || offsets.IsolationLevel != 1 || offsets.ReplicaID != -1 || len(offsets.Topics) != 1 || offsets.Topics[0].Topic != contract.CoordinationTopic {
					t.Fatal("health check is not a read-committed owned-topic leader request")
				}
				requested := make(map[int32]bool)
				for _, partition := range offsets.Topics[0].Partitions {
					if partition.Timestamp != -1 {
						t.Fatal("not querying stable end offset")
					}
					requested[partition.Partition] = true
				}
				if !reflect.DeepEqual(requested, map[int32]bool{0: true, 1: true}) {
					t.Fatalf("requested=%v", requested)
				}
				// 两个分区分别由不同 broker 服务，只允许完整成功响应恢复。
				healthy, other := busLeaderResponse(1, 0), busLeaderResponse(2, 1)
				switch mode {
				case "unreachable-leader":
					other.Resp = nil
					other.Err = context.DeadlineExceeded
				case "partition-error":
					other.Resp.(*kmsg.ListOffsetsResponse).Topics[0].Partitions[0].ErrorCode = 6
				case "negative-offset":
					other.Resp.(*kmsg.ListOffsetsResponse).Topics[0].Partitions[0].Offset = -1
				case "missing-partition":
					return []kgo.ResponseShard{healthy}
				case "wrong-topic":
					other.Resp.(*kmsg.ListOffsetsResponse).Topics[0].Topic = "other"
				case "duplicate-partition":
					return []kgo.ResponseShard{healthy, healthy, other}
				case "generation-changed":
					consumer.invalidate(errors.New("new disconnect during request"))
				}
				return []kgo.ResponseShard{healthy, other}
			}
			err := consumer.checkReadiness(context.Background())
			_, ready := consumer.CacheState()
			if mode == "all-leaders-recovered" {
				if err != nil || !ready {
					t.Fatalf("recovery error=%v ready=%v", err, ready)
				}
				return
			}
			if ready {
				t.Fatal("partial connectivity or stale generation restored readiness")
			}
			if mode != "generation-changed" && err == nil {
				t.Fatal("leader failure was swallowed")
			}
		})
	}
}

// TestBusLeaderCheckWaitIsBounded 验证 leader 不可达时遵循请求取消且不持有状态锁。
func TestBusLeaderCheckWaitIsBounded(t *testing.T) {
	fake := &busFakeClient{}
	consumer := newBusTestConsumer(fake)
	consumer.assignment([]int32{0}, true)
	if err := consumer.confirmOffsets(busOffsetsResponse(0)); err != nil {
		t.Fatal(err)
	}
	fake.requestSharded = func(ctx context.Context, _ kmsg.Request) []kgo.ResponseShard {
		consumer.invalidate(errors.New("broker unreachable"))
		<-ctx.Done()
		return []kgo.ResponseShard{{Meta: kgo.BrokerMetadata{NodeID: 2}, Err: ctx.Err()}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := consumer.checkReadiness(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("leader check exceeded deadline")
	}
	if _, ready := consumer.CacheState(); ready {
		t.Fatal("timed out leader is ready")
	}
}
