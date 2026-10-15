// 本文件提供具备持久化写入安全边界的 Kafka producer 构造函数。
package kafka

import (
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// NewDurableProducer 创建一个使用稳定客户端配置、全量确认、幂等写入、
// 有界缓冲区并在检测到数据丢失时停止生产的 Kafka producer。
//
// producer 的选项由 ProducerConfig 和本包的安全默认值完全控制，调用方
// 不能通过额外 franz-go 选项覆盖这些持久化约束。
func NewDurableProducer(config ProducerConfig) (*kgo.Client, error) {
	opts, err := durableProducerOptions(config)
	if err != nil {
		return nil, err
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create durable Kafka producer: %w", err)
	}
	return client, nil
}

// durableProducerOptions 构造 durable producer 与 transactional producer
// 共用的安全选项。幂等写入和禁止自动创建 topic 由 franz-go 的安全默认值
// 保持开启与关闭，本函数不提供可覆盖这些约束的选项入口。
func durableProducerOptions(config ProducerConfig) ([]kgo.Opt, error) {
	if err := validateClientConfig(config.Client); err != nil {
		return nil, fmt.Errorf("validate Kafka client config: %w", err)
	}
	if config.DeliveryErrorHandler == nil {
		return nil, fmt.Errorf("delivery error handler is required")
	}
	if config.MaxBufferedRecords <= 0 {
		return nil, fmt.Errorf("max buffered records must be greater than zero")
	}
	if config.MaxBufferedBytes <= 0 {
		return nil, fmt.Errorf("max buffered bytes must be greater than zero")
	}

	commonOpts, err := commonClientOptions(config.Client)
	if err != nil {
		return nil, fmt.Errorf("build Kafka client options: %w", err)
	}
	return append(commonOpts,
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.MaxBufferedRecords(config.MaxBufferedRecords),
		kgo.MaxBufferedBytes(config.MaxBufferedBytes),
		kgo.StopProducerOnDataLossDetected(),
		kgo.WithHooks(deliveryErrorHook{handler: config.DeliveryErrorHandler}),
	), nil
}

// deliveryErrorHook 保证异步 Produce 即使没有 promise，也不会静默丢失投递错误。
type deliveryErrorHook struct {
	handler func(*kgo.Record, error)
}

func (hook deliveryErrorHook) OnProduceRecordUnbuffered(record *kgo.Record, err error) {
	if err != nil {
		hook.handler(record, err)
	}
}
