// 本文件提供显式启用的 Kafka 协调总线，与旧 FastEvent 工厂和全局身份完全分离。
package eventBus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	contract "gitee.com/ming-iot/share/contracts/eventbus"
	foundation "gitee.com/ming-iot/share/foundation/kafka"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaBusConfig 显式指定进程实例；同角色不同实例必须使用不同 Instance。
type KafkaBusConfig struct {
	// Client 复用基础层的安全连接配置。
	Client foundation.ClientConfig
	// Role 标识服务角色，用于广播消费组身份。
	Role string
	// Instance 标识当前进程实例，部署时必须唯一。
	Instance string
	// PollTimeout 限制一次拉取等待时间。
	PollTimeout time.Duration
	// RequestTimeout 限制连接检查、提交与发布的等待时间。
	RequestTimeout time.Duration
	// MaxPollRecords 限制每轮串行处理的记录数。
	MaxPollRecords int
}

// KafkaBus 为一个实例拥有生产者、广播消费者与同步回调注册表。
type KafkaBus struct {
	config    KafkaBusConfig
	consumer  *foundation.BusConsumer
	producer  *kgo.Client
	mu        sync.RWMutex
	handlers  map[string][]FastFunc
	closed    bool
	closeOnce sync.Once
	// deliveryErr 保留异步确认最终失败，避免调用方超时后丢失诊断。
	deliveryErr error
}

// NewKafkaBus 仅构造独立实例，连接及分区就绪由 Run 与 CacheState 明确报告。
func NewKafkaBus(config KafkaBusConfig) (*KafkaBus, error) {
	group, err := contract.BroadcastGroup(config.Role, config.Instance)
	if err != nil {
		return nil, err
	}
	consumer, err := foundation.NewBusConsumer(foundation.BusConsumerConfig{Client: config.Client, GroupID: group, Topic: contract.CoordinationTopic, PollTimeout: config.PollTimeout, RequestTimeout: config.RequestTimeout, MaxPollRecords: config.MaxPollRecords})
	if err != nil {
		return nil, err
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 3 * time.Second
	}
	bus := &KafkaBus{config: config, consumer: consumer, handlers: make(map[string][]FastFunc)}
	producer, err := foundation.NewDurableProducer(foundation.ProducerConfig{Client: config.Client, MaxBufferedRecords: 64, MaxBufferedBytes: 2 * contract.MaxEnvelopeBytes, DeliveryErrorHandler: bus.deliveryError})
	if err != nil {
		_ = consumer.Close()
		return nil, err
	}
	bus.producer = producer
	return bus, nil
}

// deliveryError 保留所有最终投递失败，包括调用方等待超时后的异步结果。
func (bus *KafkaBus) deliveryError(_ *kgo.Record, err error) {
	bus.mu.Lock()
	bus.deliveryErr = err
	bus.mu.Unlock()
}

// cacheTypeFromSubject 只接受首个实际缓存类型的精确旧 subject，不开启任意消息路由。
func cacheTypeFromSubject(topic string) (string, error) {
	const prefix = "server.cache.sync."
	if !strings.HasPrefix(topic, prefix) {
		return "", errors.New("Kafka bus only supports cache invalidation subjects")
	}
	cacheType := strings.TrimPrefix(topic, prefix)
	// 使用权威契约校验缓存类型，避免适配层生成第二份类型规则。
	now := time.Now().UTC()
	_, err := contract.NewCacheInvalidation(contract.Metadata{EventID: "validate", Source: "validate", OccurredAt: now, AcceptedAt: now, Scope: contract.ScopeSystem, CorrelationID: "validate"}, contract.CacheInvalidation{CacheType: cacheType, Key: "validate"})
	if err != nil {
		return "", err
	}
	return cacheType, nil
}

// Subscribe 注册同步完成回调；新注册缓存从空 L1 开始，可在 Run 期间增加。
func (bus *KafkaBus) Subscribe(topic string, handler FastFunc) error {
	if _, err := cacheTypeFromSubject(topic); err != nil {
		return err
	}
	if handler == nil {
		return errors.New("Kafka bus handler is required")
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if bus.closed {
		return errors.New("Kafka bus is closed")
	}
	bus.handlers[topic] = append(bus.handlers[topic], handler)
	return nil
}

// Publish 将缓存键转换为一次系统级失效操作；内部重投复用此调用生成的同一信封。
// 此适配器仅供受信任的系统缓存，不能从任意业务 subject 推导租户权限。
func (bus *KafkaBus) Publish(ctx context.Context, topic string, arg any) error {
	cacheType, err := cacheTypeFromSubject(topic)
	if err != nil {
		return err
	}
	key, ok := arg.(string)
	if !ok {
		return errors.New("Kafka cache invalidation requires a string cache key")
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	event, err := contract.NewCacheInvalidation(contract.Metadata{EventID: id, Source: bus.config.Role + "/" + bus.config.Instance, OccurredAt: now, AcceptedAt: now, Scope: contract.ScopeSystem, CorrelationID: id}, contract.CacheInvalidation{CacheType: cacheType, Key: key})
	if err != nil {
		return err
	}
	return bus.PublishEnvelope(ctx, event)
}

// PublishEnvelope 允许调用方在显式重试时复用原事件身份；成功只表示 broker 确认。
func (bus *KafkaBus) PublishEnvelope(ctx context.Context, event contract.Envelope) error {
	if event.Scope != contract.ScopeSystem {
		return errors.New("Kafka cache adapter requires system scope")
	}
	body, err := contract.Encode(event)
	if err != nil {
		return err
	}
	key, err := event.PartitionKey()
	if err != nil {
		return err
	}
	bus.mu.RLock()
	closed := bus.closed
	bus.mu.RUnlock()
	if closed {
		return errors.New("Kafka bus is closed")
	}
	publishCtx, cancel := context.WithTimeout(ctx, bus.config.RequestTimeout)
	defer cancel()
	// 有界队列满时直接失败；等待超时不等于 broker 拒绝，调用方重试应复用原信封。
	result := make(chan error, 1)
	bus.producer.TryProduce(publishCtx, &kgo.Record{Topic: contract.CoordinationTopic, Key: key, Value: body, Timestamp: event.OccurredAt}, func(_ *kgo.Record, err error) { result <- err })
	select {
	case err := <-result:
		return err
	case <-publishCtx.Done():
		return publishCtx.Err()
	}
}

// Run 同步等待消费终止；处理失败由调用方显式恢复，不能越过失败消息继续产生副作用。
func (bus *KafkaBus) Run(ctx context.Context) error { return bus.consumer.Run(ctx, bus.handleRecord) }

// handleRecord 在全部匹配处理器完成后才允许 foundation 提交；非本实例订阅的已知类型可跳过。
func (bus *KafkaBus) handleRecord(ctx context.Context, record *kgo.Record) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("Kafka bus handler panicked")
		}
	}()
	if record.Topic != contract.CoordinationTopic {
		return errors.New("unexpected Kafka bus physical topic")
	}
	event, err := contract.Decode(record.Value)
	if err != nil {
		return err
	}
	if event.Scope != contract.ScopeSystem {
		return errors.New("Kafka cache adapter requires system scope")
	}
	key, err := event.PartitionKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(key, record.Key) {
		return errors.New("Kafka bus partition key does not match envelope")
	}
	payload, err := event.CacheInvalidation()
	if err != nil {
		return err
	}
	bus.mu.RLock()
	handlers := append([]FastFunc(nil), bus.handlers[event.LegacySubject]...)
	bus.mu.RUnlock()
	for _, handler := range handlers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := handler(ctx, event.OccurredAt, []byte(payload.Key)); err != nil {
			return fmt.Errorf("cache invalidation handler: %w", err)
		}
	}
	return nil
}

// CacheState 提供缓存所需的恢复代际，不以客户端构造或连接对象存在冒充就绪。
func (bus *KafkaBus) CacheState() (uint64, bool) { return bus.consumer.CacheState() }

// LastError 返回最近消费者故障，短暂重连不会抹掉诊断结果。
func (bus *KafkaBus) LastError() error {
	bus.mu.RLock()
	err := bus.deliveryErr
	bus.mu.RUnlock()
	if err != nil {
		return err
	}
	return bus.consumer.LastError()
}

// Close 取消并等待消费完成，再关闭生产者；并发调用只关闭一次。
func (bus *KafkaBus) Close() error {
	bus.closeOnce.Do(func() {
		bus.mu.Lock()
		bus.closed = true
		bus.mu.Unlock()
		_ = bus.consumer.Close()
		bus.producer.Close()
	})
	return nil
}
