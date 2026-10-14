// 本文件定义 Kafka 原始记录引用的严格文本格式、校验与编码契约。
package kafka

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	minRawRefTextLength = 49
	maxRawRefTextLength = 76
)

// RawRef 是经过校验且使用 canonical 文本表示的 Kafka 原始记录引用。
type RawRef struct {
	// sourceClusterID 保存规范的来源集群 ID。
	sourceClusterID string
	// topicID 保存规范的 Topic ID。
	topicID string
	// partition 保存非负分区号。
	partition int32
	// offset 保存非负 offset。
	offset int64
}

// NewRawRef 校验并构造 Kafka 原始记录引用。
func NewRawRef(sourceClusterID, topicID string, partition int32, offset int64) (RawRef, error) {
	if err := validateRawRefID("source cluster ID", sourceClusterID); err != nil {
		return RawRef{}, err
	}
	if err := validateRawRefID("topic ID", topicID); err != nil {
		return RawRef{}, err
	}
	if partition < 0 {
		return RawRef{}, fmt.Errorf("partition must be non-negative")
	}
	if offset < 0 {
		return RawRef{}, fmt.Errorf("offset must be non-negative")
	}
	return RawRef{
		sourceClusterID: sourceClusterID,
		topicID:         topicID,
		partition:       partition,
		offset:          offset,
	}, nil
}

// ParseRawRef 严格解析 sourceClusterId/topicId/partition/offset 文本。
func ParseRawRef(text string) (RawRef, error) {
	if len(text) < minRawRefTextLength {
		return RawRef{}, fmt.Errorf("raw ref text too short: actual=%d limit=%d", len(text), minRawRefTextLength)
	}
	if len(text) > maxRawRefTextLength {
		return RawRef{}, fmt.Errorf("raw ref text too large: actual=%d limit=%d", len(text), maxRawRefTextLength)
	}
	sourceClusterID, remaining, found := strings.Cut(text, "/")
	if !found {
		return RawRef{}, fmt.Errorf("raw ref must contain exactly four segments")
	}
	topicID, remaining, found := strings.Cut(remaining, "/")
	if !found {
		return RawRef{}, fmt.Errorf("raw ref must contain exactly four segments")
	}
	partitionText, offsetText, found := strings.Cut(remaining, "/")
	if !found || strings.Contains(offsetText, "/") {
		return RawRef{}, fmt.Errorf("raw ref must contain exactly four segments")
	}
	if err := validateRawRefDecimal("partition", partitionText); err != nil {
		return RawRef{}, err
	}
	if err := validateRawRefDecimal("offset", offsetText); err != nil {
		return RawRef{}, err
	}
	partition, err := strconv.ParseInt(partitionText, 10, 32)
	if err != nil {
		return RawRef{}, fmt.Errorf("invalid partition: %w", err)
	}
	offset, err := strconv.ParseInt(offsetText, 10, 64)
	if err != nil {
		return RawRef{}, fmt.Errorf("invalid offset: %w", err)
	}
	return NewRawRef(sourceClusterID, topicID, int32(partition), offset)
}

// DecodeRawRef 解码 RawRef 的 canonical UTF-8 文本字节。
func DecodeRawRef(encoded []byte) (RawRef, error) {
	return ParseRawRef(string(encoded))
}

// String 返回 RawRef 的 canonical 文本表示。
func (ref RawRef) String() string {
	if ref.sourceClusterID == "" && ref.topicID == "" && ref.partition == 0 && ref.offset == 0 {
		return ""
	}
	return ref.sourceClusterID + "/" + ref.topicID + "/" + strconv.FormatInt(int64(ref.partition), 10) + "/" + strconv.FormatInt(ref.offset, 10)
}

// Encode 返回 RawRef canonical 文本的字节副本。
func (ref RawRef) Encode() []byte {
	text := ref.String()
	if text == "" {
		return nil
	}
	return []byte(text)
}

// SourceClusterID 返回来源集群 ID。
func (ref RawRef) SourceClusterID() string {
	return ref.sourceClusterID
}

// TopicID 返回 Topic ID。
func (ref RawRef) TopicID() string {
	return ref.topicID
}

// Partition 返回分区号。
func (ref RawRef) Partition() int32 {
	return ref.partition
}

// Offset 返回 offset。
func (ref RawRef) Offset() int64 {
	return ref.offset
}

// validateRawRefID 校验 ID 的长度、编码字母表、字节长度和 canonical round-trip。
func validateRawRefID(label, value string) error {
	if len(value) != 22 {
		return fmt.Errorf("%s must be 22 characters", label)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", label, err)
	}
	if len(decoded) != 16 {
		return fmt.Errorf("%s must decode to 16 bytes", label)
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != value {
		return fmt.Errorf("%s is not canonical base64.RawURLEncoding", label)
	}
	return nil
}

// validateRawRefDecimal 校验非负数字的规范十进制表示。
func validateRawRefDecimal(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if value == "0" {
		return nil
	}
	if value[0] < '1' || value[0] > '9' {
		return fmt.Errorf("%s must be canonical non-negative decimal", label)
	}
	for _, char := range value[1:] {
		if char < '0' || char > '9' {
			return fmt.Errorf("%s must be canonical non-negative decimal", label)
		}
	}
	return nil
}
