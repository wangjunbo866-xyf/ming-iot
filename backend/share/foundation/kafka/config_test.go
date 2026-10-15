// 本文件验证 Kafka 基础配置的校验规则、不可覆盖的客户端选项与防御性复制。
package kafka

import (
	"crypto/tls"
	"reflect"
	"strings"
	"testing"

	contract "gitee.com/ming-iot/share/contracts/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

func TestValidateClientConfigAcceptsLocalAndProduction(t *testing.T) {
	t.Parallel()

	local := ClientConfig{
		Profile:  ProfileLocal,
		Brokers:  []string{"127.0.0.1:9092"},
		ClientID: "iot.local",
	}
	if err := validateClientConfig(local); err != nil {
		t.Fatalf("validateClientConfig(local) error = %v", err)
	}

	production := ClientConfig{
		Profile:       ProfileProduction,
		Brokers:       []string{"kafka-1.example.com:9093", "kafka-2.example.com:9093"},
		ClientID:      "iot.production",
		Principal:     "svc-iot",
		TLSConfig:     &tls.Config{MinVersion: tls.VersionTLS13},
		SASLMechanism: plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism(),
	}
	if err := validateClientConfig(production); err != nil {
		t.Fatalf("validateClientConfig(production) error = %v", err)
	}
}

func TestValidateClientConfigRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	base := func() ClientConfig {
		return ClientConfig{
			Profile:  ProfileLocal,
			Brokers:  []string{"127.0.0.1:9092"},
			ClientID: "iot.local",
		}
	}
	production := func() ClientConfig {
		cfg := base()
		cfg.Profile = ProfileProduction
		cfg.Brokers = []string{"kafka.example.com:9093"}
		cfg.Principal = "svc-iot"
		cfg.TLSConfig = &tls.Config{}
		cfg.SASLMechanism = plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism()
		return cfg
	}

	tests := []struct {
		name string
		cfg  func() ClientConfig
	}{
		{name: "invalid profile", cfg: func() ClientConfig { cfg := base(); cfg.Profile = Profile("staging"); return cfg }},
		{name: "missing brokers", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = nil; return cfg }},
		{name: "blank broker", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{""}; return cfg }},
		{name: "broker has surrounding whitespace", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{" 127.0.0.1:9092"}; return cfg }},
		{name: "broker has internal whitespace", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{"host :9093"}; return cfg }},
		{name: "duplicate brokers", cfg: func() ClientConfig { cfg := base(); cfg.Brokers = []string{"a:9092", "a:9092"}; return cfg }},
		{name: "missing client ID", cfg: func() ClientConfig { cfg := base(); cfg.ClientID = ""; return cfg }},
		{name: "client ID too long", cfg: func() ClientConfig { cfg := base(); cfg.ClientID = strings.Repeat("a", 129); return cfg }},
		{name: "client ID invalid character", cfg: func() ClientConfig { cfg := base(); cfg.ClientID = "iot local"; return cfg }},
		{name: "client ID must start with alphanumeric", cfg: func() ClientConfig { cfg := base(); cfg.ClientID = ".iot"; return cfg }},
		{name: "client ID must end with alphanumeric", cfg: func() ClientConfig { cfg := base(); cfg.ClientID = "iot-"; return cfg }},
		{name: "production missing TLS", cfg: func() ClientConfig { cfg := production(); cfg.TLSConfig = nil; return cfg }},
		{name: "production insecure TLS", cfg: func() ClientConfig {
			cfg := production()
			cfg.TLSConfig.InsecureSkipVerify = true
			return cfg
		}},
		{name: "production missing SASL", cfg: func() ClientConfig { cfg := production(); cfg.SASLMechanism = nil; return cfg }},
		{name: "production missing principal", cfg: func() ClientConfig { cfg := production(); cfg.Principal = ""; return cfg }},
		{name: "production invalid principal", cfg: func() ClientConfig { cfg := production(); cfg.Principal = "svc iot"; return cfg }},
		{name: "local SASL without principal", cfg: func() ClientConfig {
			cfg := base()
			cfg.SASLMechanism = plain.Auth{User: "svc", Pass: "secret"}.AsMechanism()
			return cfg
		}},
		{name: "local principal without SASL", cfg: func() ClientConfig { cfg := base(); cfg.Principal = "svc-iot"; return cfg }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateClientConfig(test.cfg()); err == nil {
				t.Fatalf("validateClientConfig() error = nil, want rejection for %s", test.name)
			}
		})
	}
}

func TestValidateStableIdentityRejectsUnstableValues(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "too long", value: strings.Repeat("a", 129)},
		{name: "space", value: "svc iot"},
		{name: "unicode", value: "服务"},
		{name: "leading punctuation", value: ".svc"},
		{name: "trailing punctuation", value: "svc_"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateStableIdentity("identity", test.value); err == nil {
				t.Fatalf("validateStableIdentity(%q) error = nil, want rejection", test.value)
			}
		})
	}
}

func TestCommonClientOptionsMaterializeValidatedProductionConfig(t *testing.T) {
	t.Parallel()

	inputTLS := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg := ClientConfig{
		Profile:       ProfileProduction,
		Brokers:       []string{"kafka-1.example.com:9093", "kafka-2.example.com:9093"},
		ClientID:      "iot.production",
		Principal:     "svc-iot",
		TLSConfig:     inputTLS,
		SASLMechanism: plain.Auth{User: "svc-iot", Pass: "secret"}.AsMechanism(),
	}

	opts, err := commonClientOptions(cfg)
	if err != nil {
		t.Fatalf("commonClientOptions() error = %v", err)
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		t.Fatalf("kgo.NewClient() error = %v", err)
	}
	defer client.Close()

	if got, want := client.OptValue(kgo.SeedBrokers), cfg.Brokers; !reflect.DeepEqual(got, want) {
		t.Fatalf("SeedBrokers = %#v, want %#v", got, want)
	}
	if got, want := client.OptValue(kgo.ClientID), cfg.ClientID; got != want {
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
	configuredTLS.MinVersion = tls.VersionTLS12
	if inputTLS.MinVersion != tls.VersionTLS13 {
		t.Fatal("mutating configured TLS changed caller TLS config")
	}

	mechanisms, ok := client.OptValue(kgo.SASL).([]sasl.Mechanism)
	if !ok || len(mechanisms) != 1 || mechanisms[0].Name() != "PLAIN" {
		t.Fatalf("SASL = %#v, want one PLAIN mechanism", client.OptValue(kgo.SASL))
	}
	if got := client.OptValue(kgo.AllowAutoTopicCreation); got != false {
		t.Fatalf("AllowAutoTopicCreation = %#v, want false", got)
	}
}

func TestCommonClientOptionsMaterializeLocalWithoutOptionalSecurity(t *testing.T) {
	t.Parallel()

	opts, err := commonClientOptions(ClientConfig{
		Profile:  ProfileLocal,
		Brokers:  []string{"127.0.0.1:9092"},
		ClientID: "iot.local",
	})
	if err != nil {
		t.Fatalf("commonClientOptions() error = %v", err)
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		t.Fatalf("kgo.NewClient() error = %v", err)
	}
	defer client.Close()

	localTLS, ok := client.OptValue(kgo.DialTLSConfig).(*tls.Config)
	if !ok || localTLS != nil {
		t.Fatalf("DialTLSConfig = %#v, want nil *tls.Config", client.OptValue(kgo.DialTLSConfig))
	}
	if got := client.OptValue(kgo.SASL); got != nil && reflect.ValueOf(got).Len() != 0 {
		t.Fatalf("SASL = %#v, want no mechanisms", got)
	}
	if got := client.OptValue(kgo.AllowAutoTopicCreation); got != false {
		t.Fatalf("AllowAutoTopicCreation = %#v, want false", got)
	}
}

func TestConfigAPIUsesContractTopicsAndRejectsArbitraryOptions(t *testing.T) {
	t.Parallel()

	var _ []contract.Topic = (ConsumerConfig{}).Topics
	consumerTopics, ok := reflect.TypeOf(ConsumerConfig{}).FieldByName("Topics")
	if !ok {
		t.Fatal("ConsumerConfig.Topics field is missing")
	}
	if consumerTopics.Type != reflect.TypeOf([]contract.Topic{}) {
		t.Fatalf("ConsumerConfig.Topics type = %v, want []contract.Topic", consumerTopics.Type)
	}

	functionType := reflect.TypeOf(commonClientOptions)
	if functionType.NumIn() != 1 || functionType.In(0) != reflect.TypeOf(ClientConfig{}) {
		t.Fatalf("commonClientOptions input type = %v, want exactly ClientConfig", functionType)
	}
	if functionType.NumOut() != 2 || functionType.Out(0) != reflect.TypeOf([]kgo.Opt{}) || functionType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("commonClientOptions output type = %v, want ([]kgo.Opt, error)", functionType)
	}
}
