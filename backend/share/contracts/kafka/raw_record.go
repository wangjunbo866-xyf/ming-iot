// 本文件定义 Kafka 原始记录的 header 校验、确定性 wire 编码与严格解码契约。
package kafka

import (
	"encoding/binary"
	"fmt"
	"sort"
	"time"
)

const (
	// HeaderMessageKind 标识原始记录事实类型 header。
	HeaderMessageKind = "message-kind"
	// HeaderMQTTQoS 标识 MQTT QoS header。
	HeaderMQTTQoS = "mqtt-qos"
	// HeaderMQTTRetain 标识 MQTT retain header。
	HeaderMQTTRetain = "mqtt-retain"
	// HeaderBrokerReceivedAt 标识 broker 接收时间 header。
	HeaderBrokerReceivedAt = "broker-received-at"
	// HeaderTraceparent 标识 W3C traceparent header。
	HeaderTraceparent = "traceparent"
)

const (
	rawRecordMagic      = "MRR1"
	maxRawRecordHeaders = 5
)

// Header 表示原始记录中的一个命名 header。
type Header struct {
	// Name 是 header 的冻结名称。
	Name string
	// Value 是 header 的字节值。
	Value []byte
}

// RawRecord 是经过校验且可确定性编码的 Kafka 原始记录。
type RawRecord struct {
	// key 保存记录 key 的私有副本。
	key []byte
	// value 保存记录 value 的私有副本。
	value []byte
	// headers 保存按名称排序的 header 深复制。
	headers []Header
}

// NewRawRecord 校验并构造 Kafka 原始记录。
func NewRawRecord(key, value []byte, headers []Header) (RawRecord, error) {
	if len(key) == 0 {
		return RawRecord{}, fmt.Errorf("raw record key must not be empty")
	}
	if len(value) == 0 {
		return RawRecord{}, fmt.Errorf("raw record value must not be empty")
	}
	if len(headers) > maxRawRecordHeaders {
		return RawRecord{}, fmt.Errorf("raw record header count too large: actual=%d limit=%d", len(headers), maxRawRecordHeaders)
	}

	clonedHeaders := make([]Header, len(headers))
	seen := make(map[string]struct{}, len(headers))
	messageKindFound := false
	for i, header := range headers {
		if header.Name == "" {
			return RawRecord{}, fmt.Errorf("raw record header name must not be empty")
		}
		if !isRawRecordHeaderName(header.Name) {
			return RawRecord{}, fmt.Errorf("unknown raw record header %q", header.Name)
		}
		if _, exists := seen[header.Name]; exists {
			return RawRecord{}, fmt.Errorf("duplicate raw record header %q", header.Name)
		}
		seen[header.Name] = struct{}{}
		if err := validateRawRecordHeader(header); err != nil {
			return RawRecord{}, err
		}
		if header.Name == HeaderMessageKind {
			messageKindFound = true
		}
		clonedHeaders[i] = Header{
			Name:  header.Name,
			Value: append([]byte(nil), header.Value...),
		}
	}
	if !messageKindFound {
		return RawRecord{}, fmt.Errorf("raw record header %q is required", HeaderMessageKind)
	}
	sort.Slice(clonedHeaders, func(i, j int) bool {
		return clonedHeaders[i].Name < clonedHeaders[j].Name
	})

	return RawRecord{
		key:     append([]byte(nil), key...),
		value:   append([]byte(nil), value...),
		headers: clonedHeaders,
	}, nil
}

// DecodeRawRecord 严格解码并校验 Kafka 原始记录 wire。
func DecodeRawRecord(encoded []byte) (RawRecord, error) {
	if len(encoded) < len(rawRecordMagic) || string(encoded[:len(rawRecordMagic)]) != rawRecordMagic {
		return RawRecord{}, fmt.Errorf("invalid raw record magic")
	}
	offset := len(rawRecordMagic)
	key, err := readRawRecordField(encoded, &offset)
	if err != nil {
		return RawRecord{}, fmt.Errorf("decode raw record key: %w", err)
	}
	value, err := readRawRecordField(encoded, &offset)
	if err != nil {
		return RawRecord{}, fmt.Errorf("decode raw record value: %w", err)
	}
	headerCount, err := readRawRecordUvarint(encoded, &offset)
	if err != nil {
		return RawRecord{}, fmt.Errorf("decode raw record header count: %w", err)
	}
	if headerCount > maxRawRecordHeaders {
		return RawRecord{}, fmt.Errorf("raw record header count too large: actual=%d limit=%d", headerCount, maxRawRecordHeaders)
	}
	if headerCount > uint64(len(encoded)-offset) {
		return RawRecord{}, fmt.Errorf("raw record header count exceeds remaining bytes: actual=%d limit=%d", headerCount, len(encoded)-offset)
	}
	headers := make([]Header, 0, int(headerCount))
	previousHeaderName := ""
	for i := uint64(0); i < headerCount; i++ {
		name, err := readRawRecordField(encoded, &offset)
		if err != nil {
			return RawRecord{}, fmt.Errorf("decode raw record header %d name: %w", i, err)
		}
		headerName := string(name)
		if i > 0 && headerName <= previousHeaderName {
			return RawRecord{}, fmt.Errorf("raw record header names must be strictly increasing: previous=%q actual=%q", previousHeaderName, headerName)
		}
		previousHeaderName = headerName
		value, err := readRawRecordField(encoded, &offset)
		if err != nil {
			return RawRecord{}, fmt.Errorf("decode raw record header %d value: %w", i, err)
		}
		headers = append(headers, Header{Name: headerName, Value: value})
	}
	if offset != len(encoded) {
		return RawRecord{}, fmt.Errorf("raw record contains trailing bytes")
	}

	record, err := NewRawRecord(key, value, headers)
	if err != nil {
		return RawRecord{}, fmt.Errorf("validate decoded raw record: %w", err)
	}
	return record, nil
}

// Encode 返回 RawRecord 的 canonical wire 编码副本。
func (record RawRecord) Encode() []byte {
	if len(record.key) == 0 && len(record.value) == 0 && len(record.headers) == 0 {
		return nil
	}
	encoded := make([]byte, 0, len(rawRecordMagic)+len(record.key)+len(record.value))
	encoded = append(encoded, rawRecordMagic...)
	encoded = appendRawRecordEncodedField(encoded, record.key)
	encoded = appendRawRecordEncodedField(encoded, record.value)
	encoded = appendRawRecordEncodedUvarint(encoded, uint64(len(record.headers)))
	for _, header := range record.headers {
		encoded = appendRawRecordEncodedField(encoded, []byte(header.Name))
		encoded = appendRawRecordEncodedField(encoded, header.Value)
	}
	return encoded
}

// Key 返回记录 key 的防御性副本。
func (record RawRecord) Key() []byte {
	return append([]byte(nil), record.key...)
}

// Value 返回记录 value 的防御性副本。
func (record RawRecord) Value() []byte {
	return append([]byte(nil), record.value...)
}

// Headers 返回 header 及其值的防御性深复制。
func (record RawRecord) Headers() []Header {
	headers := make([]Header, len(record.headers))
	for i, header := range record.headers {
		headers[i] = Header{Name: header.Name, Value: append([]byte(nil), header.Value...)}
	}
	return headers
}

// isRawRecordHeaderName 判断 header 名是否属于冻结集合。
func isRawRecordHeaderName(name string) bool {
	switch name {
	case HeaderMessageKind, HeaderMQTTQoS, HeaderMQTTRetain, HeaderBrokerReceivedAt, HeaderTraceparent:
		return true
	default:
		return false
	}
}

// validateRawRecordHeader 校验单个冻结 header 的值约束。
func validateRawRecordHeader(header Header) error {
	switch header.Name {
	case HeaderMessageKind:
		if err := validateStableToken(string(header.Value)); err != nil {
			return fmt.Errorf("invalid %s header: %w", header.Name, err)
		}
	case HeaderMQTTQoS:
		if len(header.Value) != 1 || (header.Value[0] != '0' && header.Value[0] != '1' && header.Value[0] != '2') {
			return fmt.Errorf("invalid %s header", header.Name)
		}
	case HeaderMQTTRetain:
		if string(header.Value) != "true" && string(header.Value) != "false" {
			return fmt.Errorf("invalid %s header", header.Name)
		}
	case HeaderBrokerReceivedAt:
		if err := validateRawRecordReceivedAt(string(header.Value)); err != nil {
			return fmt.Errorf("invalid %s header: %w", header.Name, err)
		}
	case HeaderTraceparent:
		if len(header.Value) == 0 || len(header.Value) > 512 {
			return fmt.Errorf("invalid %s header length", header.Name)
		}
		for _, char := range header.Value {
			if char < 0x20 || char > 0x7e {
				return fmt.Errorf("invalid %s header: value must be printable ASCII", header.Name)
			}
		}
	}
	return nil
}

// validateRawRecordReceivedAt 校验 UTC Z 的规范 RFC3339Nano 时间。
func validateRawRecordReceivedAt(value string) error {
	if len(value) == 0 || value[len(value)-1] != 'Z' {
		return fmt.Errorf("timestamp must use UTC Z")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("timestamp is not RFC3339Nano: %w", err)
	}
	if parsed.UTC().Format(time.RFC3339Nano) != value {
		return fmt.Errorf("timestamp is not canonical")
	}
	return nil
}

// appendRawRecordEncodedField 追加字段的 canonical 长度和内容。
func appendRawRecordEncodedField(encoded, field []byte) []byte {
	encoded = appendRawRecordEncodedUvarint(encoded, uint64(len(field)))
	return append(encoded, field...)
}

// appendRawRecordEncodedUvarint 追加最短 uvarint。
func appendRawRecordEncodedUvarint(encoded []byte, value uint64) []byte {
	var buffer [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buffer[:], value)
	return append(encoded, buffer[:n]...)
}

// readRawRecordField 读取一个 canonical 长度字段及其原始数据视图。
func readRawRecordField(encoded []byte, offset *int) ([]byte, error) {
	length, err := readRawRecordUvarint(encoded, offset)
	if err != nil {
		return nil, err
	}
	remaining := len(encoded) - *offset
	if length > uint64(remaining) {
		return nil, fmt.Errorf("field length %d exceeds remaining bytes %d", length, remaining)
	}
	end := *offset + int(length)
	field := encoded[*offset:end]
	*offset = end
	return field, nil
}

// readRawRecordUvarint 读取并验证最短无符号变长整数。
func readRawRecordUvarint(encoded []byte, offset *int) (uint64, error) {
	if *offset >= len(encoded) {
		return 0, fmt.Errorf("truncated uvarint")
	}
	value, count := binary.Uvarint(encoded[*offset:])
	if count == 0 {
		return 0, fmt.Errorf("truncated uvarint")
	}
	if count < 0 {
		return 0, fmt.Errorf("uvarint overflow")
	}
	if count != rawRecordUvarintLen(value) {
		return 0, fmt.Errorf("noncanonical uvarint")
	}
	*offset += count
	return value, nil
}

// rawRecordUvarintLen 返回无符号整数的最短 uvarint 字节数。
func rawRecordUvarintLen(value uint64) int {
	length := 1
	for value >= 0x80 {
		value >>= 7
		length++
	}
	return length
}
