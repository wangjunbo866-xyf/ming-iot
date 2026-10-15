// 本文件提供只读已提交消息的 Kafka consumer 构造函数与安全配置边界。
package kafka

import (
	"fmt"
	"strings"

	contract "gitee.com/ming-iot/share/contracts/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
)

// NewReadCommittedConsumer 创建一个使用受管物理 Topic、标准 consumer
// 分组类型、协作式 sticky 分配、read-committed 隔离级别及手动提交的 consumer。
//
// consumer 的选项由 ConsumerConfig 和本包的安全默认值完全控制，调用方
// 不能通过额外 franz-go 选项覆盖这些消费约束。
func NewReadCommittedConsumer(config ConsumerConfig) (*kgo.Client, error) {
	if err := validateClientConfig(config.Client); err != nil {
		return nil, fmt.Errorf("validate Kafka client config: %w", err)
	}
	if err := validateStableIdentity("group ID", config.GroupID); err != nil {
		return nil, fmt.Errorf("validate Kafka consumer group: %w", err)
	}

	topicNames, err := validateConsumerTopics(config.Topics)
	if err != nil {
		return nil, err
	}

	commonOpts, err := commonClientOptions(config.Client)
	if err != nil {
		return nil, fmt.Errorf("build Kafka client options: %w", err)
	}
	consumerOpts := []kgo.Opt{
		kgo.ConsumeTopics(topicNames...),
		kgo.ConsumerGroup(config.GroupID),
		kgo.Balancers(kgo.CooperativeStickyBalancer()),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.DisableAutoCommit(),
	}
	if config.Client.Profile == ProfileProduction {
		consumerOpts = append(consumerOpts, kgo.ConsumeResetOffset(kgo.NoResetOffset()))
	} else {
		consumerOpts = append(consumerOpts, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	}

	client, err := kgo.NewClient(append(commonOpts, consumerOpts...)...)
	if err != nil {
		return nil, fmt.Errorf("create read-committed Kafka consumer: %w", err)
	}
	return client, nil
}

// validateConsumerTopics 校验受管 Topic 并提取不重复的物理名称。
func validateConsumerTopics(topics []contract.Topic) ([]string, error) {
	if len(topics) == 0 {
		return nil, fmt.Errorf("consumer topics must not be empty")
	}

	physicalNames := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for index, topic := range topics {
		spec := topic.Spec()
		physicalName := spec.PhysicalName.String()
		if strings.TrimSpace(physicalName) == "" {
			return nil, fmt.Errorf("consumer topic %d has an empty physical name", index)
		}
		if _, err := contract.NewTopic(spec); err != nil {
			return nil, fmt.Errorf("validate consumer topic %d: %w", index, err)
		}
		if _, exists := seen[physicalName]; exists {
			return nil, fmt.Errorf("consumer topic %q is duplicated", physicalName)
		}
		seen[physicalName] = struct{}{}
		physicalNames = append(physicalNames, physicalName)
	}
	return physicalNames, nil
}
