// 本文件验证 transactional producer 的冻结 API、安全选项与配置校验。
package kafka

import (
	"crypto/tls"
	"reflect"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

func TestNewTransactionalProducerAppliesTransactionalAndDurableOptions(t *testing.T) {
	inputTLS := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg := TransactionalProducerConfig{
		Producer: ProducerConfig{
			Client: ClientConfig{
				Profile:       ProfileProduction,
				Brokers:       []string{"kafka-1.example.com:9093", "kafka-2.example.com:9093"},
				ClientID:      "iot.txn-producer",
				Principal:     "svc-iot",
				TLSConfig:     inputTLS,
				SASLMechanism: plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism(),
			},
			MaxBufferedRecords: 128,
			MaxBufferedBytes:   1 << 20,
			DeliveryErrorHandler: func(*kgo.Record, error) {
			},
		},
		TransactionalID: "iot.txn-producer.v1",
	}

	client, err := NewTransactionalProducer(cfg)
	if err != nil {
		t.Fatalf("NewTransactionalProducer() error = %v", err)
	}
	defer client.Close()

	if got, want := client.OptValue(kgo.SeedBrokers), cfg.Producer.Client.Brokers; !reflect.DeepEqual(got, want) {
		t.Fatalf("SeedBrokers = %#v, want %#v", got, want)
	}
	if got, want := client.OptValue(kgo.ClientID), cfg.Producer.Client.ClientID; got != want {
		t.Fatalf("ClientID = %#v, want %#v", got, want)
	}
	configuredTLS, ok := client.OptValue(kgo.DialTLSConfig).(*tls.Config)
	if !ok || configuredTLS == nil {
		t.Fatalf("DialTLSConfig = %#v, want non-nil *tls.Config", client.OptValue(kgo.DialTLSConfig))
	}
	if configuredTLS == inputTLS {
		t.Fatal("DialTLSConfig reused caller TLS config pointer")
	}
	if configuredTLS.MinVersion != tls.VersionTLS13 {
		t.Fatalf("TLS MinVersion = %d, want TLS 1.3", configuredTLS.MinVersion)
	}
	if got := client.OptValue(kgo.SASL); got == nil {
		t.Fatal("SASL = nil, want configured mechanism")
	}

	if got := client.OptValue(kgo.AllowAutoTopicCreation); got != false {
		t.Fatalf("AllowAutoTopicCreation = %#v, want false", got)
	}
	if got, ok := client.OptValue(kgo.RequiredAcks).(kgo.Acks); !ok || got != kgo.AllISRAcks() {
		t.Fatalf("RequiredAcks = %#v, want all in-sync replicas", client.OptValue(kgo.RequiredAcks))
	}
	if got := client.OptValue(kgo.DisableIdempotentWrite); got != false {
		t.Fatalf("DisableIdempotentWrite = %#v, want false", got)
	}
	if got, want := client.OptValue(kgo.MaxBufferedRecords), int64(cfg.Producer.MaxBufferedRecords); got != want {
		t.Fatalf("MaxBufferedRecords = %#v, want %d", got, want)
	}
	if got, want := client.OptValue(kgo.MaxBufferedBytes), int64(cfg.Producer.MaxBufferedBytes); got != want {
		t.Fatalf("MaxBufferedBytes = %#v, want %d", got, want)
	}
	if got := client.OptValue(kgo.StopProducerOnDataLossDetected); got != true {
		t.Fatalf("StopProducerOnDataLossDetected = %#v, want true", got)
	}
	transactionalIDValues := client.OptValues(kgo.TransactionalID)
	if len(transactionalIDValues) != 2 {
		t.Fatalf("TransactionalID values = %#v, want two values", transactionalIDValues)
	}
	transactionalID, ok := transactionalIDValues[0].(*string)
	if !ok || transactionalID == nil || *transactionalID != cfg.TransactionalID || transactionalIDValues[1] != true {
		t.Fatalf("TransactionalID values = %#v, want [%q true]", transactionalIDValues, cfg.TransactionalID)
	}
}

func TestNewTransactionalProducerRejectsMissingSecurity(t *testing.T) {
	tests := []struct {
		name   string
		client ClientConfig
	}{
		{
			name: "local principal without SASL",
			client: ClientConfig{
				Profile:   ProfileLocal,
				Brokers:   []string{"127.0.0.1:9092"},
				ClientID:  "iot.local",
				Principal: "svc-iot",
			},
		},
		{
			name: "production without security",
			client: ClientConfig{
				Profile:  ProfileProduction,
				Brokers:  []string{"kafka.example.com:9093"},
				ClientID: "iot.production",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewTransactionalProducer(TransactionalProducerConfig{
				Producer: ProducerConfig{
					Client:             test.client,
					MaxBufferedRecords: 1,
					MaxBufferedBytes:   1,
				},
				TransactionalID: "iot.txn",
			})
			if err == nil {
				if client != nil {
					client.Close()
				}
				t.Fatal("NewTransactionalProducer() error = nil, want security configuration error")
			}
			if client != nil {
				client.Close()
				t.Fatal("NewTransactionalProducer() returned a client with an error")
			}
		})
	}
}

func TestNewTransactionalProducerRejectsInvalidBufferAndTransactionalID(t *testing.T) {
	base := TransactionalProducerConfig{
		Producer: ProducerConfig{
			Client: ClientConfig{
				Profile:  ProfileLocal,
				Brokers:  []string{"127.0.0.1:9092"},
				ClientID: "iot.local",
			},
			MaxBufferedRecords: 1,
			MaxBufferedBytes:   1,
			DeliveryErrorHandler: func(*kgo.Record, error) {
			},
		},
		TransactionalID: "iot.txn",
	}

	tests := []struct {
		name string
		edit func(*TransactionalProducerConfig)
	}{
		{name: "zero records", edit: func(cfg *TransactionalProducerConfig) { cfg.Producer.MaxBufferedRecords = 0 }},
		{name: "zero bytes", edit: func(cfg *TransactionalProducerConfig) { cfg.Producer.MaxBufferedBytes = 0 }},
		{name: "empty transactional ID", edit: func(cfg *TransactionalProducerConfig) { cfg.TransactionalID = "" }},
		{name: "unstable transactional ID", edit: func(cfg *TransactionalProducerConfig) { cfg.TransactionalID = "iot txn" }},
		{name: "trailing punctuation transactional ID", edit: func(cfg *TransactionalProducerConfig) { cfg.TransactionalID = "iot.txn-" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			client, err := NewTransactionalProducer(cfg)
			if err == nil {
				if client != nil {
					client.Close()
				}
				t.Fatal("NewTransactionalProducer() error = nil, want configuration error")
			}
			if client != nil {
				client.Close()
				t.Fatal("NewTransactionalProducer() returned a client with an error")
			}
		})
	}
}

func TestNewTransactionalProducerWrapsClientConstructionError(t *testing.T) {
	client, err := NewTransactionalProducer(TransactionalProducerConfig{
		Producer: ProducerConfig{
			Client: ClientConfig{
				Profile:  ProfileLocal,
				Brokers:  []string{"127.0.0.1:not-a-port"},
				ClientID: "iot.local",
			},
			MaxBufferedRecords: 1,
			MaxBufferedBytes:   1,
			DeliveryErrorHandler: func(*kgo.Record, error) {
			},
		},
		TransactionalID: "iot.txn",
	})
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("NewTransactionalProducer() error = nil, want franz-go construction error")
	}
	if client != nil {
		client.Close()
		t.Fatal("NewTransactionalProducer() returned a client with an error")
	}
	if !strings.Contains(err.Error(), "create transactional Kafka producer") {
		t.Fatalf("NewTransactionalProducer() error = %v, want contextual construction error", err)
	}
}

func TestNewTransactionalProducerHasFrozenSignature(t *testing.T) {
	functionType := reflect.TypeOf(NewTransactionalProducer)
	if functionType.IsVariadic() || functionType.NumIn() != 1 || functionType.In(0) != reflect.TypeOf(TransactionalProducerConfig{}) {
		t.Fatalf("NewTransactionalProducer input signature = %v, want func(TransactionalProducerConfig)", functionType)
	}
	if functionType.NumOut() != 2 || functionType.Out(0) != reflect.TypeOf((*kgo.Client)(nil)) || functionType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("NewTransactionalProducer output signature = %v, want (*kgo.Client, error)", functionType)
	}
}
