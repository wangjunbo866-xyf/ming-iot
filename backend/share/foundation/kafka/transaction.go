// 本文件提供具备持久化写入安全边界与稳定事务身份的 Kafka transactional producer。
package kafka

import (
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// NewTransactionalProducer 创建一个使用稳定客户端配置、全量确认、幂等写入、
// 有界缓冲区、数据丢失保护与稳定 TransactionalID 的 Kafka transactional producer。
//
// producer 的选项由 TransactionalProducerConfig 和本包的安全默认值完全控制，
// 调用方不能通过额外 franz-go 选项覆盖这些约束。此函数只构造客户端，不执行
// transaction 的开始、提交或回滚流程。
func NewTransactionalProducer(config TransactionalProducerConfig) (*kgo.Client, error) {
	if err := validateStableIdentity("transactional ID", config.TransactionalID); err != nil {
		return nil, fmt.Errorf("validate Kafka transactional ID: %w", err)
	}

	opts, err := durableProducerOptions(config.Producer)
	if err != nil {
		return nil, fmt.Errorf("build Kafka transactional producer options: %w", err)
	}
	opts = append(opts, kgo.TransactionalID(config.TransactionalID))

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create transactional Kafka producer: %w", err)
	}
	return client, nil
}
