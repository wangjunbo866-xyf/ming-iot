// 本文件验证 durable producer 的冻结 API、配置校验与有效 franz-go 选项。
package kafka

import (
	"crypto/tls"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

func TestNewDurableProducerAppliesAllEffectiveOptions(t *testing.T) {
	inputTLS := &tls.Config{MinVersion: tls.VersionTLS13}
	var deliveryError error
	cfg := ProducerConfig{
		Client: ClientConfig{
			Profile:       ProfileProduction,
			Brokers:       []string{"kafka-1.example.com:9093", "kafka-2.example.com:9093"},
			ClientID:      "iot.producer",
			Principal:     "svc-iot",
			TLSConfig:     inputTLS,
			SASLMechanism: plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism(),
		},
		MaxBufferedRecords: 128,
		MaxBufferedBytes:   1 << 20,
		DeliveryErrorHandler: func(_ *kgo.Record, err error) {
			deliveryError = err
		},
	}

	client, err := NewDurableProducer(cfg)
	if err != nil {
		t.Fatalf("NewDurableProducer() error = %v", err)
	}
	defer client.Close()

	if got, want := client.OptValue(kgo.SeedBrokers), cfg.Client.Brokers; !reflect.DeepEqual(got, want) {
		t.Fatalf("SeedBrokers = %#v, want %#v", got, want)
	}
	if got, want := client.OptValue(kgo.ClientID), cfg.Client.ClientID; got != want {
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
	mechanisms, ok := client.OptValue(kgo.SASL).([]sasl.Mechanism)
	if !ok || len(mechanisms) != 1 || mechanisms[0].Name() != "PLAIN" {
		t.Fatalf("SASL = %#v, want one PLAIN mechanism", client.OptValue(kgo.SASL))
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
	if got, want := client.OptValue(kgo.MaxBufferedRecords), int64(cfg.MaxBufferedRecords); got != want {
		t.Fatalf("MaxBufferedRecords = %#v, want %d", got, want)
	}
	if got, want := client.OptValue(kgo.MaxBufferedBytes), int64(cfg.MaxBufferedBytes); got != want {
		t.Fatalf("MaxBufferedBytes = %#v, want %d", got, want)
	}
	if got := client.OptValue(kgo.StopProducerOnDataLossDetected); got != true {
		t.Fatalf("StopProducerOnDataLossDetected = %#v, want true", got)
	}
	hooks := reflect.ValueOf(client.OptValue(kgo.WithHooks))
	if hooks.Kind() != reflect.Slice || hooks.Len() != 1 {
		t.Fatalf("WithHooks = %#v, want one delivery-error hook", client.OptValue(kgo.WithHooks))
	}
	deliveryHook, ok := hooks.Index(0).Interface().(kgo.HookProduceRecordUnbuffered)
	if !ok {
		t.Fatalf("hook = %#v, want HookProduceRecordUnbuffered", hooks.Index(0).Interface())
	}
	wantErr := errors.New("delivery failed")
	deliveryHook.OnProduceRecordUnbuffered(&kgo.Record{Topic: "managed-topic"}, wantErr)
	if !errors.Is(deliveryError, wantErr) {
		t.Fatalf("delivery error = %v, want %v", deliveryError, wantErr)
	}
}

func TestNewDurableProducerRejectsMissingDeliveryErrorHandler(t *testing.T) {
	client, err := NewDurableProducer(ProducerConfig{
		Client: ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:9092"},
			ClientID: "iot.local",
		},
		MaxBufferedRecords: 1,
		MaxBufferedBytes:   1,
	})
	if err == nil || !strings.Contains(err.Error(), "delivery error handler") {
		if client != nil {
			client.Close()
		}
		t.Fatalf("NewDurableProducer() error = %v, want missing delivery error handler", err)
	}
	if client != nil {
		client.Close()
		t.Fatal("NewDurableProducer() returned a client with an error")
	}
}

func TestNewDurableProducerRejectsInvalidClientConfigs(t *testing.T) {
	tests := []struct {
		name   string
		client ClientConfig
	}{
		{
			name: "local client configuration",
			client: ClientConfig{
				Profile:   ProfileLocal,
				Brokers:   []string{"127.0.0.1:9092"},
				ClientID:  "iot.local",
				Principal: "svc-iot",
			},
		},
		{
			name: "production client configuration",
			client: ClientConfig{
				Profile:  ProfileProduction,
				Brokers:  []string{"kafka.example.com:9093"},
				ClientID: "iot.production",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewDurableProducer(ProducerConfig{
				Client:             test.client,
				MaxBufferedRecords: 1,
				MaxBufferedBytes:   1,
			})
			if err == nil {
				if client != nil {
					client.Close()
				}
				t.Fatal("NewDurableProducer() error = nil, want configuration error")
			}
			if client != nil {
				client.Close()
				t.Fatal("NewDurableProducer() returned a client with an error")
			}
		})
	}
}

func TestNewDurableProducerRejectsZeroBufferLimits(t *testing.T) {
	base := ProducerConfig{
		Client: ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:9092"},
			ClientID: "iot.local",
		},
		MaxBufferedRecords: 1,
		MaxBufferedBytes:   1,
		DeliveryErrorHandler: func(*kgo.Record, error) {
		},
	}

	for _, test := range []struct {
		name string
		edit func(*ProducerConfig)
	}{
		{name: "records", edit: func(cfg *ProducerConfig) { cfg.MaxBufferedRecords = 0 }},
		{name: "bytes", edit: func(cfg *ProducerConfig) { cfg.MaxBufferedBytes = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			client, err := NewDurableProducer(cfg)
			if err == nil {
				if client != nil {
					client.Close()
				}
				t.Fatal("NewDurableProducer() error = nil, want buffer validation error")
			}
			if client != nil {
				client.Close()
				t.Fatal("NewDurableProducer() returned a client with an error")
			}
		})
	}
}

func TestNewDurableProducerValidatesClientBeforeBufferLimits(t *testing.T) {
	client, err := NewDurableProducer(ProducerConfig{
		Client: ClientConfig{
			Profile:   ProfileLocal,
			Brokers:   []string{"127.0.0.1:9092"},
			ClientID:  "iot.local",
			Principal: "svc-iot",
		},
	})
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("NewDurableProducer() error = nil, want client configuration error")
	}
	if client != nil {
		client.Close()
		t.Fatal("NewDurableProducer() returned a client with an error")
	}
	if strings.Contains(err.Error(), "buffer") {
		t.Fatalf("NewDurableProducer() error = %v, want client validation before buffer validation", err)
	}
}

func TestNewDurableProducerWrapsClientConstructionError(t *testing.T) {
	client, err := NewDurableProducer(ProducerConfig{
		Client: ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:not-a-port"},
			ClientID: "iot.local",
		},
		MaxBufferedRecords: 1,
		MaxBufferedBytes:   1,
		DeliveryErrorHandler: func(*kgo.Record, error) {
		},
	})
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("NewDurableProducer() error = nil, want franz-go construction error")
	}
	if client != nil {
		client.Close()
		t.Fatal("NewDurableProducer() returned a client with an error")
	}
	if !strings.Contains(err.Error(), "create durable Kafka producer") {
		t.Fatalf("NewDurableProducer() error = %v, want contextual construction error", err)
	}
}

func TestNewDurableProducerHasFrozenSignature(t *testing.T) {
	functionType := reflect.TypeOf(NewDurableProducer)
	if functionType.IsVariadic() || functionType.NumIn() != 1 || functionType.In(0) != reflect.TypeOf(ProducerConfig{}) {
		t.Fatalf("NewDurableProducer input signature = %v, want func(ProducerConfig)", functionType)
	}
	if functionType.NumOut() != 2 || functionType.Out(0) != reflect.TypeOf((*kgo.Client)(nil)) || functionType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("NewDurableProducer output signature = %v, want (*kgo.Client, error)", functionType)
	}
}
