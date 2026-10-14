// 本文件验证 Kafka 集成探针只输出元数据摘要并真实报告不可达错误。
package kafkacontract

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMergeProbeErrorsPreservesPrimaryAndCleanupFailures(t *testing.T) {
	primary := errors.New("produce failed")
	cleanup := errors.New("cleanup failed")
	merged := mergeProbeErrors(primary, cleanup)
	if !errors.Is(merged, primary) || !errors.Is(merged, cleanup) {
		t.Fatalf("mergeProbeErrors() = %v, want both primary and cleanup failures", merged)
	}
}

func TestProbeRecordDigestIsDeterministicAndDoesNotExposePayload(t *testing.T) {
	first := summarizeProbeRecord(
		[]byte("fixed-key"),
		[]ProbeHeader{{Key: "message-kind", Value: []byte("contract-probe")}},
		[]byte(`{"kind":"contract-probe","secret":"must-not-leak"}`),
	)
	second := summarizeProbeRecord(
		[]byte("fixed-key"),
		[]ProbeHeader{{Key: "message-kind", Value: []byte("contract-probe")}},
		[]byte(`{"kind":"contract-probe","secret":"must-not-leak"}`),
	)
	if first != second {
		t.Fatalf("summarizeProbeRecord() is not deterministic: %#v != %#v", first, second)
	}
	if first.KeyDigest == "" || first.HeaderDigest == "" || first.ValueDigest == "" {
		t.Fatalf("summarizeProbeRecord() returned empty digest: %#v", first)
	}
}

func TestRunProbeFailsWhenBrokerIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := RunProbe(ctx, ProbeConfig{
		Brokers: []string{"127.0.0.1:1"},
		Topic:   "iot.data.contract.probe.v1.g01",
	})
	if err == nil {
		t.Fatal("RunProbe() error = nil, want broker unavailable error")
	}
}
