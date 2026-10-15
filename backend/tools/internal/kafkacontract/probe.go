// 本文件实现显式开启的 Kafka 真实写入、读回和清理探针。
package kafkacontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

var (
	probeKey     = []byte("ming-iot-kafka-contract-key-v1")
	probeValue   = []byte(`{"kind":"contract-probe","version":1}`)
	probeHeaders = []ProbeHeader{
		{Key: "message-kind", Value: []byte("contract-probe")},
		{Key: "traceparent", Value: []byte("00-00000000000000000000000000000001-0000000000000001-01")},
	}
)

// ProbeConfig 描述真实 Kafka 探针的显式连接范围。
type ProbeConfig struct {
	Brokers []string
	Topic   string
}

// ProbeHeader 是探针使用的受控 Header 值。
type ProbeHeader struct {
	Key   string
	Value []byte
}

// ProbeRecordSummary 只保留 key、header 和 value 的摘要，不暴露完整 payload。
type ProbeRecordSummary struct {
	KeyDigest    string `json:"key_digest"`
	HeaderDigest string `json:"header_digest"`
	ValueDigest  string `json:"value_digest"`
}

// ProbeResult 是真实 produce/consume 的可审计元数据。
type ProbeResult struct {
	Status    string `json:"status"`
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	ProbeRecordSummary
	ResourceDeleted bool `json:"resource_deleted"`
}

// RunProbe 创建固定测试 Topic，写入并读回固定 record，最后删除测试资源。
func RunProbe(ctx context.Context, config ProbeConfig) (result ProbeResult, returnErr error) {
	if len(config.Brokers) == 0 {
		return ProbeResult{}, fmt.Errorf("%s: at least one broker is required", CodeProbeUnavailable)
	}
	if config.Topic != expectedProbeTopic {
		return ProbeResult{}, fmt.Errorf("%s: probe topic must be %s", CodeCatalogEntryInvalid, expectedProbeTopic)
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(config.Brokers...),
		kgo.ClientID("ming-iot-kafka-contract-probe-v1"),
		kgo.ConsumerGroup("ming-iot-kafka-contract-probe-v1"),
		kgo.ConsumeTopics(config.Topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxWait(100*time.Millisecond),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.DisableIdempotentWrite(),
	)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("%s: create client: %w", CodeProbeUnavailable, err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		return ProbeResult{}, fmt.Errorf("%s: broker ping failed: %w", CodeProbeUnavailable, err)
	}
	if err := createProbeTopic(ctx, client, config.Topic); err != nil {
		return ProbeResult{}, err
	}
	created := true
	cleaned := false
	defer func() {
		if !created || cleaned {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		returnErr = mergeProbeErrors(returnErr, deleteProbeTopic(cleanupContext, client, config.Topic))
	}()

	record := &kgo.Record{Topic: config.Topic, Key: append([]byte(nil), probeKey...), Value: append([]byte(nil), probeValue...)}
	for _, header := range probeHeaders {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: header.Key, Value: append([]byte(nil), header.Value...)})
	}
	produceResult := client.ProduceSync(ctx, record)
	if err := produceResult.FirstErr(); err != nil {
		return ProbeResult{}, fmt.Errorf("KAFKA_PROBE_PRODUCE_FAILED: %w", err)
	}
	produced := produceResult[0].Record

	consumed, err := consumeProbeRecord(ctx, client, produced)
	if err != nil {
		return ProbeResult{}, err
	}
	if !bytes.Equal(consumed.Key, probeKey) || !bytes.Equal(consumed.Value, probeValue) || !equalProbeHeaders(consumed.Headers, probeHeaders) {
		return ProbeResult{}, fmt.Errorf("KAFKA_PROBE_RECORD_MISMATCH: produced and consumed record differ")
	}

	if err := deleteProbeTopic(ctx, client, config.Topic); err != nil {
		return ProbeResult{}, err
	}
	cleaned = true
	summary := summarizeProbeRecord(consumed.Key, probeHeaders, consumed.Value)
	return ProbeResult{
		Status:             "pass",
		Topic:              consumed.Topic,
		Partition:          consumed.Partition,
		Offset:             consumed.Offset,
		ProbeRecordSummary: summary,
		ResourceDeleted:    true,
	}, nil
}

func mergeProbeErrors(primary, cleanup error) error {
	return errors.Join(primary, cleanup)
}

func createProbeTopic(ctx context.Context, client *kgo.Client, topicName string) error {
	request := kmsg.NewPtrCreateTopicsRequest()
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic = topicName
	topic.NumPartitions = 1
	topic.ReplicationFactor = 1
	for key, value := range map[string]string{
		"cleanup.policy":         "delete",
		"retention.ms":           "3600000",
		"message.timestamp.type": "LogAppendTime",
		"min.insync.replicas":    "1",
	} {
		config := kmsg.NewCreateTopicsRequestTopicConfig()
		config.Name = key
		config.Value = kmsg.StringPtr(value)
		topic.Configs = append(topic.Configs, config)
	}
	sort.Slice(topic.Configs, func(i, j int) bool { return topic.Configs[i].Name < topic.Configs[j].Name })
	request.Topics = append(request.Topics, topic)
	response, err := request.RequestWith(ctx, client)
	if err != nil {
		return fmt.Errorf("KAFKA_PROBE_TOPIC_CREATE_FAILED: %w", err)
	}
	if len(response.Topics) != 1 {
		return fmt.Errorf("KAFKA_PROBE_TOPIC_CREATE_FAILED: expected one topic response")
	}
	if topicErr := kerr.ErrorForCode(response.Topics[0].ErrorCode); topicErr != nil {
		if errors.Is(topicErr, kerr.TopicAlreadyExists) {
			return fmt.Errorf("KAFKA_PROBE_TOPIC_BUSY: fixed probe topic already exists")
		}
		return fmt.Errorf("KAFKA_PROBE_TOPIC_CREATE_FAILED: %w", topicErr)
	}
	return nil
}

func deleteProbeTopic(ctx context.Context, client *kgo.Client, topicName string) error {
	request := kmsg.NewPtrDeleteTopicsRequest()
	request.TopicNames = []string{topicName}
	topic := kmsg.NewDeleteTopicsRequestTopic()
	topic.Topic = kmsg.StringPtr(topicName)
	request.Topics = append(request.Topics, topic)
	response, err := request.RequestWith(ctx, client)
	if err != nil {
		return fmt.Errorf("KAFKA_PROBE_TOPIC_DELETE_FAILED: %w", err)
	}
	if len(response.Topics) != 1 {
		return fmt.Errorf("KAFKA_PROBE_TOPIC_DELETE_FAILED: expected one topic response")
	}
	if topicErr := kerr.ErrorForCode(response.Topics[0].ErrorCode); topicErr != nil && !errors.Is(topicErr, kerr.UnknownTopicOrPartition) {
		return fmt.Errorf("KAFKA_PROBE_TOPIC_DELETE_FAILED: %w", topicErr)
	}
	return nil
}

func consumeProbeRecord(ctx context.Context, client *kgo.Client, produced *kgo.Record) (*kgo.Record, error) {
	for {
		fetches := client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			return nil, fmt.Errorf("KAFKA_PROBE_CONSUME_FAILED: %w", err)
		}
		var matched *kgo.Record
		fetches.EachRecord(func(record *kgo.Record) {
			if matched == nil && record.Topic == produced.Topic && record.Partition == produced.Partition && record.Offset == produced.Offset {
				copyRecord := *record
				matched = &copyRecord
			}
		})
		if matched != nil {
			return matched, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("KAFKA_PROBE_CONSUME_FAILED: %w", err)
		}
	}
}

func summarizeProbeRecord(key []byte, headers []ProbeHeader, value []byte) ProbeRecordSummary {
	return ProbeRecordSummary{
		KeyDigest:    digestBytes(key),
		HeaderDigest: digestHeaders(headers),
		ValueDigest:  digestBytes(value),
	}
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func digestHeaders(headers []ProbeHeader) string {
	canonical := make([]ProbeHeader, len(headers))
	for index, header := range headers {
		canonical[index] = ProbeHeader{Key: header.Key, Value: append([]byte(nil), header.Value...)}
	}
	sort.SliceStable(canonical, func(i, j int) bool {
		if canonical[i].Key != canonical[j].Key {
			return canonical[i].Key < canonical[j].Key
		}
		return bytes.Compare(canonical[i].Value, canonical[j].Value) < 0
	})
	hash := sha256.New()
	var length [8]byte
	for _, header := range canonical {
		binary.BigEndian.PutUint64(length[:], uint64(len(header.Key)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(header.Key))
		binary.BigEndian.PutUint64(length[:], uint64(len(header.Value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(header.Value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func equalProbeHeaders(actual []kgo.RecordHeader, expected []ProbeHeader) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index].Key != expected[index].Key || !bytes.Equal(actual[index].Value, expected[index].Value) {
			return false
		}
	}
	return true
}
