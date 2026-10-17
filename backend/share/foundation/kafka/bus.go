// 本文件实现显式选择的协调总线消费者；串行完成回调后才提交单条记录。
package kafka

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	contract "gitee.com/ming-iot/share/contracts/eventbus"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// BusConsumerConfig 限定协调总线的连接、独立消费身份与处理上界。
type BusConsumerConfig struct {
	// Client 复用已有安全客户端配置。
	Client ClientConfig
	// GroupID 显式指定广播实例或逻辑竞争组身份。
	GroupID string
	// Topic 仅允许当前开放的协调物理通道。
	Topic string
	// PollTimeout 限制每次拉取等待时间。
	PollTimeout time.Duration
	// RequestTimeout 限制连接检查与手动提交等待时间。
	RequestTimeout time.Duration
	// MaxPollRecords 限制每轮串行回调的数量。
	MaxPollRecords int
}

// busClient 定义可验证的提交和消费边界。
type busClient interface {
	Ping(context.Context) error
	RequestSharded(context.Context, kmsg.Request) []kgo.ResponseShard
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
	Close()
}

// BusConsumer 拥有独立生命周期；所有回调按批次串行执行，不承诺跨键并行。
type BusConsumer struct {
	// config 保存受控连接、主题与等待上界。
	config BusConsumerConfig
	// client 是当前实例独立拥有的 Kafka 协议客户端。
	client busClient
	// mu 保护代际、分区确认及生命周期状态，不跨网络请求持有。
	mu sync.Mutex
	// generation 在连接或分配变化时隔离此前的就绪证据。
	generation uint64
	// ready 仅在当前全部分区确认且本代际 leader 检查成功后允许缓存读取。
	ready bool
	// partitions 保存仍拥有的分区及各自已确认的消费位置，空增量分配不能丢失旧确认。
	partitions map[int32]bool
	// lastError 保留最近故障供上层诊断。
	lastError error
	// started 阻止重复启动同一消费生命周期。
	started bool
	// closed 阻止退出后重新报告就绪或再次启动。
	closed bool
	// cancel 允许 Close 取消正在执行的同步处理。
	cancel context.CancelFunc
	// done 在 Run 完全释放客户端后通知并发关闭者。
	done chan struct{}
	// closeOnce 保证底层客户端只释放一次。
	closeOnce sync.Once
}

// NewBusConsumer 创建尚未就绪的受控协调主题消费者，禁止任意 franz-go 选项覆盖。
func NewBusConsumer(config BusConsumerConfig) (*BusConsumer, error) {
	if err := contract.ValidateTopic(config.Topic); err != nil {
		return nil, err
	}
	if err := validateStableIdentity("bus group ID", config.GroupID); err != nil {
		return nil, err
	}
	opts, err := commonClientOptions(config.Client)
	if err != nil {
		return nil, err
	}
	if config.PollTimeout == 0 {
		config.PollTimeout = 250 * time.Millisecond
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 3 * time.Second
	}
	if config.MaxPollRecords == 0 {
		config.MaxPollRecords = 32
	}
	if config.PollTimeout < time.Millisecond || config.PollTimeout > 5*time.Second || config.RequestTimeout < 100*time.Millisecond || config.RequestTimeout > 30*time.Second || config.MaxPollRecords < 1 || config.MaxPollRecords > 1024 {
		return nil, fmt.Errorf("invalid bus poll/request timeout or record bound")
	}
	c := &BusConsumer{config: config, generation: 1, done: make(chan struct{})}
	opts = append(opts,
		kgo.ConsumeTopics(config.Topic), kgo.ConsumerGroup(config.GroupID),
		kgo.Balancers(kgo.CooperativeStickyBalancer()), kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.FetchMaxWait(config.PollTimeout), kgo.FetchMaxBytes(2*contract.MaxEnvelopeBytes),
		kgo.FetchMaxPartitionBytes(contract.MaxEnvelopeBytes), kgo.MaxConcurrentFetches(1),
		kgo.BrokerMaxReadBytes(4*contract.MaxEnvelopeBytes),
		kgo.DialTimeout(config.RequestTimeout), kgo.RequestTimeoutOverhead(config.RequestTimeout),
		kgo.OnPartitionsAssigned(func(_ context.Context, _ *kgo.Client, partitions map[string][]int32) {
			c.assignment(partitions[config.Topic], true)
		}),
		kgo.OnPartitionsRevoked(func(_ context.Context, _ *kgo.Client, partitions map[string][]int32) {
			c.assignment(partitions[config.Topic], false)
		}),
		kgo.OnPartitionsLost(func(_ context.Context, _ *kgo.Client, _ map[string][]int32) {
			c.lostAssignment()
		}),
		kgo.OnPartitionsCallbackBlocked(func(_ context.Context, _ *kgo.Client) { c.invalidate(errors.New("bus rebalance pending")) }),
		kgo.OnOffsetsFetched(func(_ context.Context, _ *kgo.Client, response *kmsg.OffsetFetchResponse) error {
			return c.confirmOffsets(response)
		}), kgo.WithHooks(busDisconnectHook{consumer: c}),
	)
	cli, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create bus consumer: %w", err)
	}
	c.client = cli
	return c, nil
}

// validateBusOffsets 拒绝新旧协议版本任何分组或分区的位点错误。
func validateBusOffsets(response *kmsg.OffsetFetchResponse) error {
	for _, topic := range response.Topics {
		for _, partition := range topic.Partitions {
			if partition.ErrorCode != 0 {
				return fmt.Errorf("bus offset fetch error %d", partition.ErrorCode)
			}
		}
	}
	for _, group := range response.Groups {
		if group.ErrorCode != 0 {
			return fmt.Errorf("bus group offset fetch error %d", group.ErrorCode)
		}
		for _, topic := range group.Topics {
			for _, partition := range topic.Partitions {
				if partition.ErrorCode != 0 {
					return fmt.Errorf("bus partition offset fetch error %d", partition.ErrorCode)
				}
			}
		}
	}
	if response.ErrorCode != 0 {
		return fmt.Errorf("bus offset fetch error %d", response.ErrorCode)
	}
	return nil
}

// assignment 在重新分配时撤销缓存就绪，等待新的已确认消费位置。
func (c *BusConsumer) assignment(partitions []int32, assigned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.ready = false
	if c.partitions == nil {
		c.partitions = make(map[int32]bool)
	}
	for _, partition := range partitions {
		if assigned {
			if _, exists := c.partitions[partition]; !exists {
				c.partitions[partition] = false
			}
		} else {
			delete(c.partitions, partition)
		}
	}
}

// lostAssignment 在失去组会话时清空所有旧分区确认，防止旧身份恢复就绪。
func (c *BusConsumer) lostAssignment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.ready = false
	c.partitions = nil
}

// confirmOffsets 只确认响应中仍由本实例拥有的分区，不将局部确认扩大为全部确认。
func (c *BusConsumer) confirmOffsets(response *kmsg.OffsetFetchResponse) error {
	if err := validateBusOffsets(response); err != nil {
		c.invalidate(err)
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range response.Topics {
		if topic.Topic != c.config.Topic {
			continue
		}
		for _, partition := range topic.Partitions {
			if _, owned := c.partitions[partition.Partition]; owned {
				c.partitions[partition.Partition] = true
			}
		}
	}
	for _, group := range response.Groups {
		if group.Group != c.config.GroupID {
			continue
		}
		for _, topic := range group.Topics {
			if topic.Topic != c.config.Topic {
				continue
			}
			for _, partition := range topic.Partitions {
				if _, owned := c.partitions[partition.Partition]; owned {
					c.partitions[partition.Partition] = true
				}
			}
		}
	}
	return nil
}

// invalidate 立即隔离可能漏收通知的缓存，并保留可查询的故障。
func (c *BusConsumer) invalidate(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.ready = false
	if err != nil {
		c.lastError = err
	}
}

// CacheState 返回轻量的恢复代际与真实订阅就绪状态。
func (c *BusConsumer) CacheState() (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation, c.ready
}

// LastError 保留最近一次连接或处理故障，恢复成功也不抹掉故障证据。
func (c *BusConsumer) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// markReady 只允许同一代际在实际连接成功且订阅位置确认后恢复读取。
func (c *BusConsumer) markReady(generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.generation != generation || len(c.partitions) == 0 {
		return
	}
	for _, confirmed := range c.partitions {
		if !confirmed {
			return
		}
	}
	c.ready = true
}

// checkReadiness 用当前拥有分区的 leader 响应证明本代际可服务，而非任意 broker 可达。
// 此证明受下一次故障检测的时间窗口约束，不代表集群高可用或业务端到端完成。
func (c *BusConsumer) checkReadiness(ctx context.Context) error {
	c.mu.Lock()
	// generation 和 owned 是同一时刻的分配快照，网络等待期间不持有状态锁。
	generation := c.generation
	owned := make(map[int32]bool, len(c.partitions))
	for partition := range c.partitions {
		owned[partition] = false
	}
	c.mu.Unlock()
	if len(owned) == 0 {
		return c.client.Ping(ctx)
	}
	// request 由 franz-go 按分区 leader 定向拆分；read_committed 返回稳定末尾位置。
	request := kmsg.NewPtrListOffsetsRequest()
	request.IsolationLevel = 1
	request.Topics = []kmsg.ListOffsetsRequestTopic{{Topic: c.config.Topic}}
	for partition := range owned {
		entry := kmsg.NewListOffsetsRequestTopicPartition()
		entry.Partition = partition
		entry.Timestamp = -1
		request.Topics[0].Partitions = append(request.Topics[0].Partitions, entry)
	}
	// shards 必须全部成功，单个健康 broker 的响应不能覆盖其他 leader 的失败。
	shards := c.client.RequestSharded(ctx, request)
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, shard := range shards {
		if shard.Err != nil {
			return fmt.Errorf("bus partition leader request: %w", shard.Err)
		}
		response, ok := shard.Resp.(*kmsg.ListOffsetsResponse)
		if !ok || response == nil {
			return errors.New("bus partition leader returned invalid response")
		}
		for _, topic := range response.Topics {
			if topic.Topic != c.config.Topic {
				return errors.New("bus partition leader returned unexpected topic")
			}
			for _, partition := range topic.Partitions {
				seen, expected := owned[partition.Partition]
				if !expected || seen {
					return errors.New("bus partition leader returned unexpected or duplicate partition")
				}
				if partition.ErrorCode != 0 || partition.Offset < 0 {
					return fmt.Errorf("bus partition leader unavailable for partition %d (code %d)", partition.Partition, partition.ErrorCode)
				}
				owned[partition.Partition] = true
			}
		}
	}
	for _, confirmed := range owned {
		if !confirmed {
			return errors.New("bus partition leader response missing owned partition")
		}
	}
	c.markReady(generation)
	return nil
}

// Run 单次启动消费；连接故障保持隔离并重试，处理或提交失败立即返回并保留重投。
// handler 必须同步完成且响应 ctx 取消；调用方不能在 handler 内等待 Close。
func (c *BusConsumer) Run(ctx context.Context, handler func(context.Context, *kgo.Record) error) (err error) {
	if handler == nil {
		return errors.New("bus handler is required")
	}
	c.mu.Lock()
	if c.started || c.closed {
		c.mu.Unlock()
		return errors.New("bus consumer already started or closed")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.started = true
	c.generation++
	c.mu.Unlock()
	defer func() {
		cancel()
		c.invalidate(nil)
		c.client.AllowRebalance()
		c.closeClient()
		c.invalidate(err)
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		close(c.done)
	}()
	for {
		if err := runCtx.Err(); err != nil {
			return err
		}
		healthCtx, healthCancel := context.WithTimeout(runCtx, c.config.RequestTimeout)
		healthErr := c.checkReadiness(healthCtx)
		healthCancel()
		if healthErr != nil {
			c.invalidate(fmt.Errorf("bus connectivity: %w", healthErr))
			c.client.AllowRebalance()
			timer := time.NewTimer(c.config.PollTimeout)
			select {
			case <-runCtx.Done():
				timer.Stop()
				return runCtx.Err()
			case <-timer.C:
			}
			continue
		}
		pollCtx, pollCancel := context.WithTimeout(runCtx, c.config.PollTimeout)
		fetches := c.client.PollRecords(pollCtx, c.config.MaxPollRecords)
		pollCancel()
		if err := runCtx.Err(); err != nil {
			return err
		}
		fetchFailed := false
		for _, fetchErr := range fetches.Errors() {
			if errors.Is(fetchErr.Err, context.DeadlineExceeded) {
				continue
			}
			// 含记录的失败批次不能被跳过，否则本地拉取位置可能越过未处理记录。
			c.invalidate(fmt.Errorf("bus fetch: %w", fetchErr.Err))
			fetchFailed = true
			break
		}
		if fetchFailed {
			c.client.AllowRebalance()
			if len(fetches.Records()) > 0 {
				return c.LastError()
			}
			timer := time.NewTimer(c.config.PollTimeout)
			select {
			case <-runCtx.Done():
				timer.Stop()
				return runCtx.Err()
			case <-timer.C:
			}
			continue
		}
		if err := c.processBatch(runCtx, fetches.Records(), handler); err != nil {
			return err
		}
		c.client.AllowRebalance()
	}
}

// processBatch 将同步副作用与单条提交绑定；任一失败停止后继记录，包括同键记录。
func (c *BusConsumer) processBatch(ctx context.Context, records []*kgo.Record, handler func(context.Context, *kgo.Record) error) error {
	generation, _ := c.CacheState()
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if current, _ := c.CacheState(); current != generation {
			return errors.New("bus assignment or connection changed before handler")
		}
		if err := handler(ctx, record); err != nil {
			return fmt.Errorf("bus handler %s/%d/%d: %w", record.Topic, record.Partition, record.Offset, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current, _ := c.CacheState(); current != generation {
			return errors.New("bus assignment or connection changed before commit")
		}
		commitCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		err := c.client.CommitRecords(commitCtx, record)
		cancel()
		if err != nil {
			return fmt.Errorf("bus commit %s/%d/%d: %w", record.Topic, record.Partition, record.Offset, err)
		}
	}
	return nil
}

// closeClient 确保客户端仅关闭一次。
func (c *BusConsumer) closeClient() { c.closeOnce.Do(c.client.Close) }

// Close 取消消费并等待正在执行的同步回调退出；并发重复调用安全。
func (c *BusConsumer) Close() error {
	c.mu.Lock()
	c.closed = true
	c.ready = false
	started, cancel := c.started, c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		<-c.done
	} else {
		c.closeClient()
	}
	return nil
}

// busDisconnectHook 将底层连接丢失立即映射为缓存隔离，连接建立本身不等于就绪。
type busDisconnectHook struct{ consumer *BusConsumer }

// OnBrokerDisconnect 在任一 broker 连接失去时增加缓存代际。
func (hook busDisconnectHook) OnBrokerDisconnect(_ kgo.BrokerMetadata, _ net.Conn) {
	hook.consumer.invalidate(errors.New("bus broker disconnected"))
}
