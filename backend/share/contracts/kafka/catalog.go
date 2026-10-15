// 本文件定义 Kafka Topic 数据面目录的类型、校验规则与确定性摘要。
package kafka

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Plane 表示 Kafka Topic 所属的数据平面。
type Plane string

const (
	// PlaneData 表示设备数据平面。
	PlaneData Plane = "data"
)

// CleanupPolicy 表示 Kafka Topic 的清理策略。
type CleanupPolicy string

const (
	// CleanupDelete 表示按保留时间删除消息。
	CleanupDelete CleanupPolicy = "delete"
)

// TimestampPolicy 表示 Kafka 消息的时间戳策略。
type TimestampPolicy string

const (
	// TimestampLogAppendTime 表示使用日志追加时间。
	TimestampLogAppendTime TimestampPolicy = "log-append-time"
)

// DeploymentProfile 表示 Topic 的部署环境配置。
type DeploymentProfile string

const (
	// ProfileProduction 表示生产部署配置。
	ProfileProduction DeploymentProfile = "production"
	// ProfileLocal 表示本地部署配置。
	ProfileLocal DeploymentProfile = "local"
)

// TopicSpec 描述一个 Kafka Topic 的完整声明。
type TopicSpec struct {
	// LogicalName 是 Topic 的逻辑名称。
	LogicalName LogicalTopicName
	// PhysicalName 是 Topic 的物理名称。
	PhysicalName PhysicalTopicName
	// Generation 是 Topic 的部署代数。
	Generation Generation
	// Plane 是 Topic 所属的数据平面。
	Plane Plane
	// FactKind 是 Topic 承载的事实类型。
	FactKind FactKind
	// Ordered 表示 Topic 是否要求有序处理。
	Ordered bool
	// Stateful 表示 Topic 是否承载有状态处理。
	Stateful bool
	// SchemaMajor 是消息 schema 的主版本号。
	SchemaMajor int
	// Partitions 是 Topic 的分区数。
	Partitions int
	// KeyCodec 是消息 key 的编码协议。
	KeyCodec string
	// CleanupPolicy 是 Topic 的清理策略。
	CleanupPolicy CleanupPolicy
	// Retention 是消息的保留时长。
	Retention time.Duration
	// TimestampPolicy 是消息时间戳策略。
	TimestampPolicy TimestampPolicy
	// ProducerOwner 是生产者所有者标识。
	ProducerOwner string
	// ConsumerOwners 是消费者所有者标识列表。
	ConsumerOwners []string
	// ACLProfile 是访问控制配置标识。
	ACLProfile string
	// DeploymentProfile 是部署环境配置。
	DeploymentProfile DeploymentProfile
	// ReplicationFactor 是 Topic 的副本数。
	ReplicationFactor int
	// MinISR 是最小同步副本数。
	MinISR int
	// AutoCreate 表示是否允许自动创建 Topic。
	AutoCreate bool
}

// Topic 是经过校验且不可变的 Kafka Topic 声明。
type Topic struct {
	spec TopicSpec
}

// NewTopic 校验并构造不可变的 Topic。
func NewTopic(spec TopicSpec) (Topic, error) {
	if strings.TrimSpace(spec.LogicalName.String()) == "" {
		return Topic{}, fmt.Errorf("logical name is required")
	}
	if strings.TrimSpace(spec.PhysicalName.String()) == "" {
		return Topic{}, fmt.Errorf("physical name is required")
	}
	if spec.Generation.value < 1 || spec.Generation.value > 99 {
		return Topic{}, fmt.Errorf("generation must be between 1 and 99: %d", spec.Generation.value)
	}
	if spec.Plane != PlaneData {
		return Topic{}, fmt.Errorf("plane must be %q", PlaneData)
	}
	if strings.TrimSpace(string(spec.FactKind)) == "" {
		return Topic{}, fmt.Errorf("fact kind is required")
	}
	if err := validateV1TopicTemplate(spec.LogicalName, spec.FactKind); err != nil {
		return Topic{}, err
	}
	expectedPhysicalName, err := NewPhysicalTopicName(spec.LogicalName, spec.Generation)
	if err != nil {
		return Topic{}, fmt.Errorf("invalid physical topic name: %w", err)
	}
	if spec.PhysicalName != expectedPhysicalName {
		return Topic{}, fmt.Errorf("physical name must match logical name and generation")
	}
	if spec.SchemaMajor <= 0 {
		return Topic{}, fmt.Errorf("schema major must be positive")
	}
	if spec.Partitions <= 0 {
		return Topic{}, fmt.Errorf("partitions must be positive")
	}
	if strings.TrimSpace(spec.KeyCodec) == "" {
		return Topic{}, fmt.Errorf("key codec is required")
	}
	if spec.CleanupPolicy != CleanupDelete {
		return Topic{}, fmt.Errorf("cleanup policy must be %q", CleanupDelete)
	}
	if spec.Retention <= 0 {
		return Topic{}, fmt.Errorf("retention must be positive")
	}
	if spec.FactKind == FactRaw && spec.Retention < 72*time.Hour {
		return Topic{}, fmt.Errorf("raw retention must be at least 72h")
	}
	if spec.TimestampPolicy != TimestampLogAppendTime {
		return Topic{}, fmt.Errorf("timestamp policy must be %q", TimestampLogAppendTime)
	}
	if strings.TrimSpace(spec.ProducerOwner) == "" {
		return Topic{}, fmt.Errorf("producer owner is required")
	}
	for _, owner := range spec.ConsumerOwners {
		if strings.TrimSpace(owner) == "" {
			return Topic{}, fmt.Errorf("consumer owner must not be blank")
		}
	}

	consumerOwners := normalizeConsumerOwners(spec.ConsumerOwners)
	if len(consumerOwners) == 0 {
		return Topic{}, fmt.Errorf("consumer owner is required")
	}
	if strings.TrimSpace(spec.ACLProfile) == "" {
		return Topic{}, fmt.Errorf("acl profile is required")
	}
	if spec.DeploymentProfile != ProfileProduction && spec.DeploymentProfile != ProfileLocal {
		return Topic{}, fmt.Errorf("deployment profile is invalid")
	}
	if spec.ReplicationFactor <= 0 {
		return Topic{}, fmt.Errorf("replication factor must be positive")
	}
	if spec.MinISR <= 0 {
		return Topic{}, fmt.Errorf("min isr must be positive")
	}
	if spec.MinISR > spec.ReplicationFactor {
		return Topic{}, fmt.Errorf("min isr must be no greater than replication factor")
	}
	if spec.DeploymentProfile == ProfileProduction {
		if spec.ReplicationFactor < 3 {
			return Topic{}, fmt.Errorf("production replication factor must be at least 3")
		}
		if spec.MinISR < 2 {
			return Topic{}, fmt.Errorf("production min isr must be at least 2")
		}
		if spec.AutoCreate {
			return Topic{}, fmt.Errorf("production topic cannot enable auto-create")
		}
	}

	spec.ConsumerOwners = consumerOwners
	return Topic{spec: spec}, nil
}

// Spec 返回 Topic 声明的防御性副本。
func (topic Topic) Spec() TopicSpec {
	spec := topic.spec
	spec.ConsumerOwners = append([]string(nil), topic.spec.ConsumerOwners...)
	return spec
}

// Catalog 是经过校验、排序且不可变的 Kafka Topic 目录。
type Catalog struct {
	entries []Topic
	summary string
	digest  string
}

// NewCatalog 校验、排序并构造不可变的 Topic 目录。
func NewCatalog(topics []Topic) (Catalog, error) {
	if len(topics) == 0 {
		return Catalog{}, fmt.Errorf("catalog must not be empty")
	}

	entries := make([]Topic, len(topics))
	for i, topic := range topics {
		entries[i] = cloneTopic(topic)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].spec
		right := entries[j].spec
		leftLogical, rightLogical := left.LogicalName.String(), right.LogicalName.String()
		if leftLogical != rightLogical {
			return leftLogical < rightLogical
		}
		return left.PhysicalName.String() < right.PhysicalName.String()
	})

	logicalNames := make(map[string]struct{}, len(entries))
	physicalNames := make(map[string]struct{}, len(entries))
	for _, topic := range entries {
		if strings.TrimSpace(topic.spec.LogicalName.String()) == "" || strings.TrimSpace(topic.spec.PhysicalName.String()) == "" {
			return Catalog{}, fmt.Errorf("catalog contains topic with missing name")
		}
		logicalName := topic.spec.LogicalName.String()
		physicalName := topic.spec.PhysicalName.String()
		if _, exists := logicalNames[logicalName]; exists {
			return Catalog{}, fmt.Errorf("duplicate logical name %q", logicalName)
		}
		if _, exists := physicalNames[physicalName]; exists {
			return Catalog{}, fmt.Errorf("duplicate physical name %q", physicalName)
		}
		logicalNames[logicalName] = struct{}{}
		physicalNames[physicalName] = struct{}{}
	}

	summaryBytes, err := json.Marshal(catalogSummary{Entries: makeTopicSummaries(entries)})
	if err != nil {
		return Catalog{}, fmt.Errorf("marshal catalog summary: %w", err)
	}
	summary := string(summaryBytes)
	digestBytes := sha256.Sum256([]byte(summary))
	return Catalog{
		entries: entries,
		summary: summary,
		digest:  hex.EncodeToString(digestBytes[:]),
	}, nil
}

// Entries 返回目录条目的防御性副本。
func (catalog Catalog) Entries() []Topic {
	entries := make([]Topic, len(catalog.entries))
	for i, topic := range catalog.entries {
		entries[i] = cloneTopic(topic)
	}
	return entries
}

// Summary 返回目录的确定性 JSON 摘要。
func (catalog Catalog) Summary() string {
	return catalog.summary
}

// Digest 返回摘要内容的小写 SHA-256 十六进制摘要。
func (catalog Catalog) Digest() string {
	return catalog.digest
}

// catalogSummary 是目录摘要的稳定 JSON 外壳。
type catalogSummary struct {
	Entries []topicSummary `json:"entries"`
}

// topicSummary 是 Topic 摘要的稳定 JSON 表示。
type topicSummary struct {
	LogicalName       string            `json:"logical_name"`
	PhysicalName      string            `json:"physical_name"`
	Generation        string            `json:"generation"`
	Plane             Plane             `json:"plane"`
	FactKind          FactKind          `json:"fact_kind"`
	Ordered           bool              `json:"ordered"`
	Stateful          bool              `json:"stateful"`
	SchemaMajor       int               `json:"schema_major"`
	Partitions        int               `json:"partitions"`
	KeyCodec          string            `json:"key_codec"`
	CleanupPolicy     CleanupPolicy     `json:"cleanup_policy"`
	Retention         time.Duration     `json:"retention_ns"`
	TimestampPolicy   TimestampPolicy   `json:"timestamp_policy"`
	ProducerOwner     string            `json:"producer_owner"`
	ConsumerOwners    []string          `json:"consumer_owners"`
	ACLProfile        string            `json:"acl_profile"`
	DeploymentProfile DeploymentProfile `json:"deployment_profile"`
	ReplicationFactor int               `json:"replication_factor"`
	MinISR            int               `json:"min_isr"`
	AutoCreate        bool              `json:"auto_create"`
}

// normalizeConsumerOwners 清理、排序并去重消费者所有者。
func normalizeConsumerOwners(owners []string) []string {
	cleaned := make([]string, 0, len(owners))
	for _, owner := range owners {
		owner = strings.TrimSpace(owner)
		if owner != "" {
			cleaned = append(cleaned, owner)
		}
	}
	sort.Strings(cleaned)
	if len(cleaned) == 0 {
		return nil
	}

	unique := cleaned[:1]
	for _, owner := range cleaned[1:] {
		if owner != unique[len(unique)-1] {
			unique = append(unique, owner)
		}
	}
	return append([]string(nil), unique...)
}

// cloneTopic 复制 Topic 及其内部 slice，保持目录边界不可变。
func cloneTopic(topic Topic) Topic {
	topic.spec.ConsumerOwners = append([]string(nil), topic.spec.ConsumerOwners...)
	return topic
}

// makeTopicSummaries 将已排序 Topic 转换为稳定摘要表示。
func makeTopicSummaries(entries []Topic) []topicSummary {
	summaries := make([]topicSummary, len(entries))
	for i, entry := range entries {
		spec := entry.spec
		summaries[i] = topicSummary{
			LogicalName:       spec.LogicalName.String(),
			PhysicalName:      spec.PhysicalName.String(),
			Generation:        spec.Generation.String(),
			Plane:             spec.Plane,
			FactKind:          spec.FactKind,
			Ordered:           spec.Ordered,
			Stateful:          spec.Stateful,
			SchemaMajor:       spec.SchemaMajor,
			Partitions:        spec.Partitions,
			KeyCodec:          spec.KeyCodec,
			CleanupPolicy:     spec.CleanupPolicy,
			Retention:         spec.Retention,
			TimestampPolicy:   spec.TimestampPolicy,
			ProducerOwner:     spec.ProducerOwner,
			ConsumerOwners:    append([]string(nil), spec.ConsumerOwners...),
			ACLProfile:        spec.ACLProfile,
			DeploymentProfile: spec.DeploymentProfile,
			ReplicationFactor: spec.ReplicationFactor,
			MinISR:            spec.MinISR,
			AutoCreate:        spec.AutoCreate,
		}
	}
	return summaries
}
