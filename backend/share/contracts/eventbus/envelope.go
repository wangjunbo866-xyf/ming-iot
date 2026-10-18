// 本文件定义独立于数据面 Kafka 契约的受控 bus v1 缓存失效信封。
package eventbus

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// CoordinationTopic 是首个实施切片唯一开放的物理通道。
	CoordinationTopic = "iot.bus.coordination.v1.g01"
	// TypeCacheInvalidate 是首版唯一可执行的受控消息类型。
	TypeCacheInvalidate = "coord.cache.invalidate"
	// SchemaMajor 是当前支持的信封主版本。
	SchemaMajor = 1
	// MaxEnvelopeBytes 限制编码和解码的完整消息大小。
	MaxEnvelopeBytes = 1 << 20
)

// Scope 显式区分租户事件与系统事件，不能代替来源鉴权。
type Scope string

const (
	// ScopeTenant 要求来源提供可信租户身份。
	ScopeTenant Scope = "tenant"
	// ScopeSystem 表示经调用方授权的系统范围。
	ScopeSystem Scope = "system"
)

// Metadata 保存调用方首次接受操作时确定的身份；重试必须复用，不能由 codec 补造。
type Metadata struct {
	// EventID 标识原事件，不能用分区位点替代。
	EventID string
	// Source 标识承担身份校验责任的来源服务。
	Source string
	// OccurredAt 保存来源事件发生时间。
	OccurredAt time.Time
	// AcceptedAt 保存首次接受事件的时间。
	AcceptedAt time.Time
	// Scope 指明系统或租户范围。
	Scope Scope
	// TenantID 仅在租户范围携带可信身份。
	TenantID string
	// CorrelationID 关联原操作，不承担去重职责。
	CorrelationID string
	// CausationID 在确有触发事件时记录其身份。
	CausationID string
}

// ObjectRef 以类型和规范化身份描述缓存对象。
type ObjectRef struct {
	// Type 描述对象类别，当前仅允许 cache。
	Type string `json:"type"`
	// ID 保存缓存类型与键的规范 JSON 二元组，避免分隔符碰撞。
	ID string `json:"id"`
}

// CacheInvalidation 指定需要从权威数据源重新加载的本地缓存键。
type CacheInvalidation struct {
	// CacheType 对应已有缓存 subject 的单段缓存类型。
	CacheType string `json:"cacheType"`
	// Key 是缓存自身使用的原始键，不猜测或提取其中的租户身份。
	Key string `json:"key"`
}

// Envelope 承载受控事件；合法信封不等于持久完成，也不提供业务幂等。
type Envelope struct {
	// EventID 是重投保持不变的事件身份。
	EventID string `json:"eventId"`
	// Type 只允许已实现的消息类型。
	Type string `json:"type"`
	// SchemaMajor 拒绝尚未实现的主版本。
	SchemaMajor int `json:"schemaMajor"`
	// Source 标识可信来源，消费端仍需校验资源授权。
	Source string `json:"source"`
	// OccurredAt 保存来源时间，不承诺设备采样顺序。
	OccurredAt time.Time `json:"occurredAt"`
	// AcceptedAt 保存首次接受时间，不在重试时更新。
	AcceptedAt time.Time `json:"acceptedAt"`
	// Scope 指定消息作用域。
	Scope Scope `json:"scope"`
	// TenantID 在 tenant 范围必填，在 system 范围禁止携带。
	TenantID string `json:"tenantId,omitempty"`
	// Object 明确事件作用的缓存对象。
	Object ObjectRef `json:"object"`
	// CorrelationID 关联操作但不替代事件身份。
	CorrelationID string `json:"correlationId"`
	// CausationID 只在存在触发事件时出现。
	CausationID string `json:"causationId,omitempty"`
	// LegacySubject 仅用于受控兼容筛选，必须与类型及载荷一致。
	LegacySubject string `json:"legacySubject"`
	// Payload 只在完成类型校验后交给处理器。
	Payload json.RawMessage `json:"payload"`
}

// NewCacheInvalidation 根据真实操作元数据构造受控信封，不生成身份或时间。
func NewCacheInvalidation(meta Metadata, payload CacheInvalidation) (Envelope, error) {
	if !validToken(payload.CacheType) || !validText(payload.Key, 4096) {
		return Envelope{}, fmt.Errorf("invalid cache type or key")
	}
	// body 保留唯一已支持类型的规范 JSON。
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	// event 绑定类型、对象及兼容 subject，防止调用方分别填错。
	event := Envelope{EventID: meta.EventID, Type: TypeCacheInvalidate, SchemaMajor: SchemaMajor,
		Source: meta.Source, OccurredAt: meta.OccurredAt, AcceptedAt: meta.AcceptedAt,
		Scope: meta.Scope, TenantID: meta.TenantID, CorrelationID: meta.CorrelationID,
		CausationID: meta.CausationID, Object: cacheObject(payload),
		LegacySubject: "server.cache.sync." + payload.CacheType, Payload: body}
	if err := event.Validate(); err != nil {
		return Envelope{}, err
	}
	return event, nil
}

// ValidateTopic 只允许当前已实现通道；其他消息族仍是未来设计。
func ValidateTopic(topic string) error {
	if topic != CoordinationTopic {
		return fmt.Errorf("unsupported bus topic")
	}
	return nil
}

// Validate 检查身份、作用域与受控载荷；不声明处理结果已经持久化。
func (event Envelope) Validate() error {
	if event.SchemaMajor != SchemaMajor {
		return fmt.Errorf("unsupported bus schema major")
	}
	if event.Type != TypeCacheInvalidate {
		return fmt.Errorf("unsupported bus event type")
	}
	for _, value := range []string{event.EventID, event.Source, event.CorrelationID} {
		if !validText(value, 256) {
			return fmt.Errorf("missing or invalid event identity")
		}
	}
	if event.CausationID != "" && !validText(event.CausationID, 256) {
		return fmt.Errorf("invalid causation identity")
	}
	if event.OccurredAt.IsZero() || event.AcceptedAt.IsZero() {
		return fmt.Errorf("event timestamps are required")
	}
	if event.OccurredAt.Year() < 1 || event.OccurredAt.Year() > 9999 || event.AcceptedAt.Year() < 1 || event.AcceptedAt.Year() > 9999 {
		return fmt.Errorf("event timestamps outside supported range")
	}
	if _, err := event.OccurredAt.MarshalJSON(); err != nil {
		return fmt.Errorf("invalid occurrence timestamp")
	}
	if _, err := event.AcceptedAt.MarshalJSON(); err != nil {
		return fmt.Errorf("invalid acceptance timestamp")
	}
	switch event.Scope {
	case ScopeTenant:
		if !validText(event.TenantID, 256) {
			return fmt.Errorf("tenant scope requires tenant identity")
		}
	case ScopeSystem:
		if event.TenantID != "" {
			return fmt.Errorf("system scope cannot carry tenant identity")
		}
	default:
		return fmt.Errorf("unsupported event scope")
	}
	// payload 经类型解码后才参与对象与兼容路由核对。
	payload, err := event.CacheInvalidation()
	if err != nil {
		return err
	}
	if event.Object != cacheObject(payload) {
		return fmt.Errorf("cache object does not match payload")
	}
	if event.LegacySubject != "server.cache.sync."+payload.CacheType {
		return fmt.Errorf("legacy subject does not match payload")
	}
	return nil
}

// CacheInvalidation 仅解码已知版本及类型的载荷，失败时不返回部分对象。
func (event Envelope) CacheInvalidation() (CacheInvalidation, error) {
	if event.SchemaMajor != SchemaMajor || event.Type != TypeCacheInvalidate {
		return CacheInvalidation{}, fmt.Errorf("unsupported cache event")
	}
	// payload 保存校验成功后才可交付的缓存指令。
	var payload CacheInvalidation
	if err := decodeObject(event.Payload, &payload, "cacheType", "key"); err != nil {
		return CacheInvalidation{}, err
	}
	if !validToken(payload.CacheType) || !validText(payload.Key, 4096) {
		return CacheInvalidation{}, fmt.Errorf("invalid cache type or key")
	}
	return payload, nil
}

// PartitionKey 返回版本化 JSON 元组，隔离租户、系统和含分隔符的对象身份。
func (event Envelope) PartitionKey() ([]byte, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal([]string{"bus.v1", string(event.Scope), event.TenantID, event.Object.Type, event.Object.ID})
}

// cacheObject 将缓存类型和原始键编码为无歧义对象身份。
func cacheObject(payload CacheInvalidation) ObjectRef {
	// identity 仅包含字符串，因此编码不会失败。
	identity, _ := json.Marshal([]string{payload.CacheType, payload.Key})
	return ObjectRef{Type: "cache", ID: string(identity)}
}

// validText 拒绝空值、无效 UTF-8、控制字符及边界空白，且限制身份长度。
func validText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

// validToken 限制可参与受控命名的单段标识，拒绝通配符及分隔符。
func validToken(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}
