// 本文件在进入业务前拒绝未知、歧义和超限的消息，错误只包含脱敏结构信息。
package eventbus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// Encode 校验后编码完整信封；失败不返回可发送的部分数据。
func Encode(event Envelope) ([]byte, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	// data 是传输层可发送的完整 JSON。
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("invalid envelope encoding")
	}
	if len(data) > MaxEnvelopeBytes {
		return nil, fmt.Errorf("envelope exceeds size limit")
	}
	return data, nil
}

// Decode 解码受控 v1 信封；任何错误均返回零值，隔离记录不得作为成功输入。
func Decode(data []byte) (Envelope, error) {
	// event 仅在全部验证通过后交付，避免处理器消费部分字段。
	var event Envelope
	if err := decodeObject(data, &event, "eventId", "type", "schemaMajor", "source", "occurredAt", "acceptedAt", "scope", "tenantId", "object", "correlationId", "causationId", "legacySubject", "payload"); err != nil {
		return Envelope{}, err
	}
	// fields 用于对嵌套对象执行同样的精确字段校验。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Envelope{}, fmt.Errorf("invalid envelope object")
	}
	if err := decodeObject(fields["object"], &event.Object, "type", "id"); err != nil {
		return Envelope{}, err
	}
	if err := event.Validate(); err != nil {
		return Envelope{}, err
	}
	return event, nil
}

// decodeObject 对每层受控对象拒绝重复键、大小写别名、未知字段及尾随输入。
func decodeObject(data []byte, target any, allowed ...string) error {
	if len(data) == 0 || len(data) > MaxEnvelopeBytes || !utf8.Valid(data) {
		return fmt.Errorf("invalid JSON size or encoding")
	}
	// decoder 逐字段读取，避免标准解码静默接受重复键。
	decoder := json.NewDecoder(bytes.NewReader(data))
	// opening 必须是对象起始，null 和数组均不是受控对象。
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return fmt.Errorf("expected JSON object")
	}
	// seen 用于检测重复字段，包括转义后相同的字段名。
	seen := make(map[string]bool)
	for decoder.More() {
		// token 是解码后的字段名。
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON field")
		}
		// name 必须与 schema 中的字段名称完全一致。
		name, ok := token.(string)
		if !ok || seen[name] {
			return fmt.Errorf("duplicate or invalid JSON field")
		}
		seen[name] = true
		// known 只允许显式实现的字段；新增字段需更新受控 schema。
		known := false
		for _, field := range allowed {
			if field == name {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown JSON field")
		}
		// value 暂存字段，嵌套对象由对应 schema 单独验证。
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("invalid JSON value")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("null JSON field")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("invalid JSON object end")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON input")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("invalid JSON schema")
	}
	return nil
}
