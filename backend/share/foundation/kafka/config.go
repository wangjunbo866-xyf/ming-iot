// 本文件定义 Kafka 客户端配置契约、校验规则与受控 franz-go 客户端选项。
package kafka

import (
	"crypto/tls"
	"fmt"
	"strings"
	"unicode"

	contract "gitee.com/ming-iot/share/contracts/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
)

// Profile 表示 Kafka 客户端的部署环境。
type Profile string

const (
	// ProfileLocal 表示本地开发部署环境。
	ProfileLocal Profile = "local"
	// ProfileProduction 表示生产部署环境。
	ProfileProduction Profile = "production"
)

// ClientConfig 描述 Kafka 客户端的连接、身份与安全配置。
type ClientConfig struct {
	// Profile 是客户端的部署环境。
	Profile Profile
	// Brokers 是客户端使用的 Kafka broker 地址列表。
	Brokers []string
	// ClientID 是发送给 Kafka 的稳定客户端标识。
	ClientID string
	// Principal 是访问 Kafka 的稳定主体标识。
	Principal string
	// TLSConfig 是可选的 TLS 连接配置。
	TLSConfig *tls.Config
	// SASLMechanism 是可选的 SASL 认证机制。
	SASLMechanism sasl.Mechanism
}

// ProducerConfig 描述 Kafka producer 的配置。
type ProducerConfig struct {
	// Client 是 producer 使用的客户端配置。
	Client ClientConfig
	// MaxBufferedRecords 是 producer 允许缓冲的最大记录数。
	MaxBufferedRecords int
	// MaxBufferedBytes 是 producer 允许缓冲的最大字节数。
	MaxBufferedBytes int
	// DeliveryErrorHandler 接收所有异步 record 投递失败；不能为空。
	// 即使调用方给 Produce 传入 nil promise，该处理器仍由 franz-go hook 调用。
	DeliveryErrorHandler func(*kgo.Record, error)
}

// ConsumerConfig 描述 Kafka consumer 的配置。
type ConsumerConfig struct {
	// Client 是 consumer 使用的客户端配置。
	Client ClientConfig
	// GroupID 是 consumer group 的稳定标识。
	GroupID string
	// Topics 是 consumer 订阅的 contracts Kafka Topic 列表。
	Topics []contract.Topic
}

// TransactionalProducerConfig 描述 Kafka transactional producer 的配置。
type TransactionalProducerConfig struct {
	// Producer 是 transactional producer 使用的 producer 配置。
	Producer ProducerConfig
	// TransactionalID 是 Kafka transaction 的稳定标识。
	TransactionalID string
}

// validateClientConfig 校验 Kafka 客户端配置的稳定性与环境安全要求。
func validateClientConfig(config ClientConfig) error {
	if config.Profile != ProfileLocal && config.Profile != ProfileProduction {
		return fmt.Errorf("profile must be %q or %q", ProfileLocal, ProfileProduction)
	}
	if len(config.Brokers) == 0 {
		return fmt.Errorf("brokers must contain at least one broker")
	}
	seenBrokers := make(map[string]struct{}, len(config.Brokers))
	for _, broker := range config.Brokers {
		if broker == "" {
			return fmt.Errorf("broker must not be empty")
		}
		if strings.IndexFunc(broker, unicode.IsSpace) >= 0 {
			return fmt.Errorf("broker %q must not contain whitespace", broker)
		}
		if _, exists := seenBrokers[broker]; exists {
			return fmt.Errorf("broker %q is duplicated", broker)
		}
		seenBrokers[broker] = struct{}{}
	}
	if err := validateStableIdentity("client ID", config.ClientID); err != nil {
		return err
	}

	hasPrincipal := config.Principal != ""
	hasSASL := config.SASLMechanism != nil
	if config.Profile == ProfileLocal {
		if hasPrincipal != hasSASL {
			return fmt.Errorf("local principal and SASL mechanism must be provided together")
		}
		if hasPrincipal {
			return validateStableIdentity("principal", config.Principal)
		}
		return nil
	}

	if config.TLSConfig == nil {
		return fmt.Errorf("production TLS config is required")
	}
	if config.TLSConfig.InsecureSkipVerify {
		return fmt.Errorf("production TLS config cannot skip certificate verification")
	}
	if !hasSASL {
		return fmt.Errorf("production SASL mechanism is required")
	}
	if !hasPrincipal {
		return fmt.Errorf("production principal is required")
	}
	return validateStableIdentity("principal", config.Principal)
}

// commonClientOptions 将已校验的客户端配置转换为不可覆盖的 franz-go 选项。
func commonClientOptions(config ClientConfig) ([]kgo.Opt, error) {
	if err := validateClientConfig(config); err != nil {
		return nil, err
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cloneStrings(config.Brokers)...),
		kgo.ClientID(config.ClientID),
	}
	if config.TLSConfig != nil {
		tlsConfig := config.TLSConfig.Clone()
		if config.Profile == ProfileProduction && tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}
	if config.SASLMechanism != nil {
		opts = append(opts, kgo.SASL(config.SASLMechanism))
	}
	return opts, nil
}

// validateStableIdentity 校验只能由稳定 ASCII 字符组成的身份标识。
func validateStableIdentity(label, value string) error {
	if len(value) < 1 || len(value) > 128 {
		return fmt.Errorf("%s length must be between 1 and 128", label)
	}
	if !isASCIIAlphaNumeric(value[0]) || !isASCIIAlphaNumeric(value[len(value)-1]) {
		return fmt.Errorf("%s must start and end with an ASCII letter or digit", label)
	}
	for i := 0; i < len(value); i++ {
		char := value[i]
		if !isASCIIAlphaNumeric(char) && char != '.' && char != '_' && char != '-' {
			return fmt.Errorf("%s contains invalid character %q", label, char)
		}
	}
	return nil
}

// isASCIIAlphaNumeric 判断字节是否为 ASCII 字母或数字。
func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

// cloneStrings 返回字符串切片的防御性副本。
func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
