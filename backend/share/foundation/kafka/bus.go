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
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
	Close()
}

// BusConsumer 拥有独立生命周期；所有回调按批次串行执行，不承诺跨键并行。
type BusConsumer struct {
	config     BusConsumerConfig
	client     busClient
	mu         sync.Mutex
	generation uint64
	ready      bool
	assigned   bool
	// partitions 保存协作式重分配后仍属于本实例的分区。
	partitions     map[int32]struct{}
	offsetsFetched bool
	lastError      error
	started        bool
	closed         bool
	cancel         context.CancelFunc
	done           chan struct{}
	closeOnce      sync.Once
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
		kgo.OnPartitionsLost(func(_ context.Context, _ *kgo.Client, partitions map[string][]int32) {
			c.assignment(partitions[config.Topic], false)
		}),
		kgo.OnPartitionsCallbackBlocked(func(_ context.Context, _ *kgo.Client) { c.invalidate(errors.New("bus rebalance pending")) }),
		kgo.OnOffsetsFetched(func(_ context.Context, _ *kgo.Client, response *kmsg.OffsetFetchResponse) error {
			if err := validateBusOffsets(response); err != nil {
				c.invalidate(err)
				return err
			}
			c.mu.Lock()
			c.offsetsFetched = true
			c.mu.Unlock()
			return nil
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
		c.partitions = make(map[int32]struct{})
	}
	for _, partition := range partitions {
		if assigned {
			c.partitions[partition] = struct{}{}
		} else {
			delete(c.partitions, partition)
		}
	}
	c.assigned = len(c.partitions) > 0
	c.offsetsFetched = false
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
	if !c.closed && c.generation == generation && c.assigned && c.offsetsFetched {
		c.ready = true
	}
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
		generation, _ := c.CacheState()
		pingCtx, pingCancel := context.WithTimeout(runCtx, c.config.RequestTimeout)
		pingErr := c.client.Ping(pingCtx)
		pingCancel()
		if pingErr != nil {
			c.invalidate(fmt.Errorf("bus connectivity: %w", pingErr))
			timer := time.NewTimer(c.config.PollTimeout)
			select {
			case <-runCtx.Done():
				timer.Stop()
				return runCtx.Err()
			case <-timer.C:
			}
			continue
		}
		c.markReady(generation)
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
