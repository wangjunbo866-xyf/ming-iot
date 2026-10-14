// 本文件验证 Kafka 原始记录的校验、确定性编码与严格解码契约。
package kafka

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestRawRecordAcceptsValidInputAndCanonicalizesHeaderOrder 验证合法记录可构造且 header 顺序不影响编码。
func TestRawRecordAcceptsValidInputAndCanonicalizesHeaderOrder(t *testing.T) {
	t.Parallel()

	key := []byte{0x01, 0x02}
	value := []byte("payload")
	headers := validRawHeaders()
	reversed := append([]Header(nil), headers...)
	sort.Slice(reversed, func(i, j int) bool { return reversed[i].Name > reversed[j].Name })

	left, err := NewRawRecord(key, value, headers)
	if err != nil {
		t.Fatalf("NewRawRecord() error = %v", err)
	}
	right, err := NewRawRecord(key, value, reversed)
	if err != nil {
		t.Fatalf("NewRawRecord(reversed) error = %v", err)
	}
	if !bytes.Equal(left.Encode(), right.Encode()) {
		t.Fatalf("Encode() differs for header order: left=%x right=%x", left.Encode(), right.Encode())
	}
	if !reflect.DeepEqual(left.Key(), key) || !reflect.DeepEqual(left.Value(), value) {
		t.Fatalf("record getters do not return original payload")
	}
}

// TestRawRecordRejectsInvalidInput 验证 key、value 及全部冻结 header 规则失败关闭。
func TestRawRecordRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     []byte
		value   []byte
		headers []Header
	}{
		{name: "空 key", key: nil, value: []byte("v"), headers: validRawHeaders()},
		{name: "空 value", key: []byte("k"), value: nil, headers: validRawHeaders()},
		{name: "缺少 message-kind", key: []byte("k"), value: []byte("v"), headers: []Header{{Name: "mqtt-qos", Value: []byte("1")}}},
		{name: "未知 header", key: []byte("k"), value: []byte("v"), headers: append(validRawHeaders(), Header{Name: "unknown", Value: []byte("x")})},
		{name: "空 header 名", key: []byte("k"), value: []byte("v"), headers: append(validRawHeaders(), Header{Name: "", Value: []byte("x")})},
		{name: "重复 header", key: []byte("k"), value: []byte("v"), headers: append(validRawHeaders(), Header{Name: "mqtt-qos", Value: []byte("2")})},
		{name: "message-kind 为空", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "message-kind", nil)},
		{name: "message-kind 非 stable token", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "message-kind", []byte("raw+kind"))},
		{name: "qos 非法", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "mqtt-qos", []byte("3"))},
		{name: "qos 非 ASCII 数字", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "mqtt-qos", []byte("１"))},
		{name: "retain 非法", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "mqtt-retain", []byte("TRUE"))},
		{name: "received-at 非 UTC Z", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "broker-received-at", []byte("2024-01-02T03:04:05+00:00"))},
		{name: "received-at 非规范 round-trip", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "broker-received-at", []byte("2024-01-02T03:04:05.100Z"))},
		{name: "traceparent 为空", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "traceparent", nil)},
		{name: "traceparent 不可打印", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "traceparent", []byte("00-\x00"))},
		{name: "traceparent 超长", key: []byte("k"), value: []byte("v"), headers: replaceRawHeader(validRawHeaders(), "traceparent", []byte(strings.Repeat("x", 513)))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRawRecord(test.key, test.value, test.headers); err == nil {
				t.Fatalf("NewRawRecord() error = nil, want rejection")
			}
		})
	}
}

// TestRawRecordRejectsTooManyHeaders 验证构造函数在逐项校验前拒绝超过冻结上限的 header。
func TestRawRecordRejectsTooManyHeaders(t *testing.T) {
	t.Parallel()

	headers := append(validRawHeaders(), Header{Name: HeaderMessageKind, Value: []byte("duplicate")})
	_, err := NewRawRecord([]byte("key"), []byte("value"), headers)
	if err == nil {
		t.Fatal("NewRawRecord() error = nil, want header count rejection")
	}
	for _, want := range []string{"actual=6", "limit=5"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("NewRawRecord() error = %q, want containing %q", err, want)
		}
	}
}

// TestRawRecordZeroValueEncodeReturnsNil 验证零值不会生成看似有效的 wire。
func TestRawRecordZeroValueEncodeReturnsNil(t *testing.T) {
	t.Parallel()

	if got := (RawRecord{}).Encode(); got != nil {
		t.Fatalf("RawRecord{}.Encode() = %x, want nil", got)
	}
}

// TestRawRecordDefensivelyCopiesAllByteSlices 验证构造参数、getter 和 Encode 返回值彼此隔离。
func TestRawRecordDefensivelyCopiesAllByteSlices(t *testing.T) {
	t.Parallel()

	key := []byte("key")
	value := []byte("value")
	headers := validRawHeaders()
	record := mustRawRecord(t, key, value, headers)
	wantKey := append([]byte(nil), key...)
	wantValue := append([]byte(nil), value...)
	wantHeaders := cloneRawHeaders(headers)
	sort.Slice(wantHeaders, func(i, j int) bool { return wantHeaders[i].Name < wantHeaders[j].Name })

	key[0] = 'X'
	value[0] = 'X'
	headers[0].Name = "mutated"
	headers[0].Value[0] = 'X'
	if !bytes.Equal(record.Key(), wantKey) || !bytes.Equal(record.Value(), wantValue) || !reflect.DeepEqual(record.Headers(), wantHeaders) {
		t.Fatalf("record changed after constructor input mutation")
	}

	gotKey := record.Key()
	gotValue := record.Value()
	gotHeaders := record.Headers()
	gotKey[0] = 'X'
	gotValue[0] = 'X'
	gotHeaders[0].Name = "mutated"
	gotHeaders[0].Value[0] = 'X'
	if !bytes.Equal(record.Key(), wantKey) || !bytes.Equal(record.Value(), wantValue) || !reflect.DeepEqual(record.Headers(), wantHeaders) {
		t.Fatalf("record changed after getter result mutation")
	}

	encoded := record.Encode()
	encoded[0] = 'X'
	if record.Encode()[0] != 'M' {
		t.Fatalf("record changed after Encode() result mutation")
	}
}

// TestRawRecordEncodeUsesStableGoldenBytes 验证冻结 wire 的 magic、长度、顺序和内容字节稳定。
func TestRawRecordEncodeUsesStableGoldenBytes(t *testing.T) {
	t.Parallel()

	headers := []Header{
		{Name: "traceparent", Value: []byte("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")},
		{Name: "mqtt-retain", Value: []byte("false")},
		{Name: "message-kind", Value: []byte("raw")},
		{Name: "broker-received-at", Value: []byte("2024-01-02T03:04:05.123Z")},
		{Name: "mqtt-qos", Value: []byte("1")},
	}
	record := mustRawRecord(t, []byte{0x01, 0x02}, []byte("v"), headers)
	want, err := hex.DecodeString("4d5252310201020176051262726f6b65722d72656365697665642d617418323032342d30312d30325430333a30343a30352e3132335a0c6d6573736167652d6b696e6403726177086d7174742d716f7301310b6d7174742d72657461696e0566616c73650b7472616365706172656e743730302d34626639326633353737623334646136613363653932396430653065343733362d303066303637616130626139303262372d3031")
	if err != nil {
		t.Fatalf("decode golden bytes: %v", err)
	}
	if got := record.Encode(); !bytes.Equal(got, want) {
		t.Fatalf("Encode() = %x, want %x", got, want)
	}
}

// TestRawRecordDecodeRoundTripsValidWire 验证合法 wire 可解码并保持逐字节 round-trip。
func TestRawRecordDecodeRoundTripsValidWire(t *testing.T) {
	t.Parallel()

	want := mustRawRecord(t, []byte("key"), []byte("value"), validRawHeaders())
	got, err := DecodeRawRecord(want.Encode())
	if err != nil {
		t.Fatalf("DecodeRawRecord() error = %v", err)
	}
	if !bytes.Equal(got.Encode(), want.Encode()) {
		t.Fatalf("decoded record does not round-trip: got=%x want=%x", got.Encode(), want.Encode())
	}
}

// TestRawRecordDecodeAcceptsStableGoldenBytes 验证 Decoder 可独立解析冻结 golden wire 的全部字段。
func TestRawRecordDecodeAcceptsStableGoldenBytes(t *testing.T) {
	t.Parallel()

	golden, err := hex.DecodeString("4d5252310201020176051262726f6b65722d72656365697665642d617418323032342d30312d30325430333a30343a30352e3132335a0c6d6573736167652d6b696e6403726177086d7174742d716f7301310b6d7174742d72657461696e0566616c73650b7472616365706172656e743730302d34626639326633353737623334646136613363653932396430653065343733362d303066303637616130626139303262372d3031")
	if err != nil {
		t.Fatalf("decode golden bytes: %v", err)
	}
	record, err := DecodeRawRecord(golden)
	if err != nil {
		t.Fatalf("DecodeRawRecord(golden) error = %v", err)
	}
	if got, want := record.Key(), []byte{0x01, 0x02}; !bytes.Equal(got, want) {
		t.Fatalf("Key() = %x, want %x", got, want)
	}
	if got, want := record.Value(), []byte("v"); !bytes.Equal(got, want) {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
	wantHeaders := []Header{
		{Name: HeaderBrokerReceivedAt, Value: []byte("2024-01-02T03:04:05.123Z")},
		{Name: HeaderMessageKind, Value: []byte("raw")},
		{Name: HeaderMQTTQoS, Value: []byte("1")},
		{Name: HeaderMQTTRetain, Value: []byte("false")},
		{Name: HeaderTraceparent, Value: []byte("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")},
	}
	if got := record.Headers(); !reflect.DeepEqual(got, wantHeaders) {
		t.Fatalf("Headers() = %#v, want %#v", got, wantHeaders)
	}
}

// TestRawRecordDecodeRejectsTooManyHeadersBeforeAllocation 验证 Decoder 在分配 header slice 前应用冻结上限。
func TestRawRecordDecodeRejectsTooManyHeadersBeforeAllocation(t *testing.T) {
	t.Parallel()

	wire := []byte("MRR1")
	wire = appendRawRecordField(wire, []byte("key"))
	wire = appendRawRecordField(wire, []byte("value"))
	wire = appendRawRecordUvarint(wire, 6)
	_, err := DecodeRawRecord(wire)
	if err == nil {
		t.Fatal("DecodeRawRecord() error = nil, want header count rejection")
	}
	for _, want := range []string{"actual=6", "limit=5"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("DecodeRawRecord() error = %q, want containing %q", err, want)
		}
	}
}

// TestRawRecordDecodeRejectsNonCanonicalWire 验证 magic、varint、header 顺序、完整消费和语义校验。
func TestRawRecordDecodeRejectsNonCanonicalWire(t *testing.T) {
	t.Parallel()

	valid := mustRawRecord(t, []byte("key"), []byte("value"), validRawHeaders()).Encode()
	unsortedHeaders := validRawHeaders()
	sort.Slice(unsortedHeaders, func(i, j int) bool { return unsortedHeaders[i].Name > unsortedHeaders[j].Name })

	tests := []struct {
		name string
		wire []byte
	}{
		{name: "magic", wire: append([]byte("XRR1"), valid[4:]...)},
		{name: "truncated", wire: valid[:len(valid)-1]},
		{name: "trailing bytes", wire: append(append([]byte(nil), valid...), 0x00)},
		{name: "noncanonical key length", wire: replaceRawRecordKeyLengthWithOverlong(valid)},
		{name: "overflow varint", wire: append([]byte("MRR1"), bytes.Repeat([]byte{0x80}, 10)...)},
		{name: "noncanonical header order", wire: encodeRawRecordWire([]byte("key"), []byte("value"), unsortedHeaders)},
		{name: "invalid semantic header", wire: encodeRawRecordWire([]byte("key"), []byte("value"), replaceRawHeader(validRawHeaders(), "message-kind", nil))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeRawRecord(test.wire); err == nil {
				t.Fatalf("DecodeRawRecord() error = nil, want rejection for %s", test.name)
			}
		})
	}
}

// validRawHeaders 返回一组满足冻结约束的 header。
func validRawHeaders() []Header {
	return []Header{
		{Name: "message-kind", Value: []byte("raw")},
		{Name: "mqtt-qos", Value: []byte("1")},
		{Name: "mqtt-retain", Value: []byte("false")},
		{Name: "broker-received-at", Value: []byte("2024-01-02T03:04:05.123Z")},
		{Name: "traceparent", Value: []byte("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")},
	}
}

// replaceRawHeader 返回替换指定 header 值后的副本。
func replaceRawHeader(headers []Header, name string, value []byte) []Header {
	cloned := cloneRawHeaders(headers)
	for i := range cloned {
		if cloned[i].Name == name {
			cloned[i].Value = append([]byte(nil), value...)
		}
	}
	return cloned
}

// cloneRawHeaders 返回 header slice 及其值的深复制。
func cloneRawHeaders(headers []Header) []Header {
	cloned := make([]Header, len(headers))
	for i, header := range headers {
		cloned[i] = Header{Name: header.Name, Value: append([]byte(nil), header.Value...)}
	}
	return cloned
}

// mustRawRecord 构造测试记录，失败时终止当前测试。
func mustRawRecord(t *testing.T, key, value []byte, headers []Header) RawRecord {
	t.Helper()
	record, err := NewRawRecord(key, value, headers)
	if err != nil {
		t.Fatalf("NewRawRecord() error = %v", err)
	}
	return record
}

// encodeRawRecordWire 按测试给定顺序生成原始 wire，用于构造非规范输入。
func encodeRawRecordWire(key, value []byte, headers []Header) []byte {
	wire := []byte("MRR1")
	wire = appendRawRecordField(wire, key)
	wire = appendRawRecordField(wire, value)
	wire = appendRawRecordUvarint(wire, uint64(len(headers)))
	for _, header := range headers {
		wire = appendRawRecordField(wire, []byte(header.Name))
		wire = appendRawRecordField(wire, header.Value)
	}
	return wire
}

// appendRawRecordField 追加一个 canonical uvarint 长度字段及其数据。
func appendRawRecordField(wire, field []byte) []byte {
	wire = appendRawRecordUvarint(wire, uint64(len(field)))
	return append(wire, field...)
}

// appendRawRecordUvarint 追加最短 uvarint。
func appendRawRecordUvarint(wire []byte, value uint64) []byte {
	var encoded [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(encoded[:], value)
	return append(wire, encoded[:n]...)
}

// replaceRawRecordKeyLengthWithOverlong 将合法 key 长度替换为同值的非最短 uvarint。
func replaceRawRecordKeyLengthWithOverlong(wire []byte) []byte {
	return append(append(append([]byte(nil), wire[:4]...), 0x83, 0x00), wire[5:]...)
}
