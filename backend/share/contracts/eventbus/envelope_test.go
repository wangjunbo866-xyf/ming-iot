// 本文件验证受控信封、隔离边界和分区身份，测试不代替业务持久化或幂等证据。
package eventbus

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testEvent 提供有明确来源身份的完整测试事件。
func testEvent(t testing.TB) Envelope {
	t.Helper()
	// timestamp 固定首次发生与接受时间，避免测试制造重试身份。
	timestamp := time.Date(2026, 10, 17, 1, 2, 3, 0, time.UTC)
	// event 保留跨编解码不变的业务身份。
	event, err := NewCacheInvalidation(Metadata{EventID: "event-1", Source: "syssvr", OccurredAt: timestamp, AcceptedAt: timestamp,
		Scope: ScopeTenant, TenantID: "tenant-1", CorrelationID: "operation-1"}, CacheInvalidation{CacheType: "user", Key: "key.含分隔符"})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

// TestEnvelopeRoundTrip 验证身份、时间、载荷和分区键在重投编码时保持一致。
func TestEnvelopeRoundTrip(t *testing.T) {
	// original 是发布方第一次创建的消息。
	original := testEvent(t)
	// encoded 是待发送的规范信封。
	encoded, err := Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	// decoded 是进入业务之前验证完成的消息。
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip changed event: %#v", decoded)
	}
	// repeated 证明 codec 没有重新生成身份或接受时间。
	repeated, err := Encode(decoded)
	if err != nil || !bytes.Equal(encoded, repeated) {
		t.Fatal("retry encoding changed")
	}
	// key 是包含版本、范围及对象的无歧义分区身份。
	key, err := decoded.PartitionKey()
	if err != nil {
		t.Fatal(err)
	}
	// parts 检查元组保留每个身份边界。
	var parts []string
	if err := json.Unmarshal(key, &parts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parts, []string{"bus.v1", "tenant", "tenant-1", "cache", `["user","key.含分隔符"]`}) {
		t.Fatalf("unexpected key %s", key)
	}
}

// TestInvalidEnvelopeIsIsolated 验证畸形或未知输入不会产生可交给处理器的部分信封。
func TestInvalidEnvelopeIsIsolated(t *testing.T) {
	// cases 每次只破坏一个必须满足的约束。
	cases := map[string]func(*Envelope){
		"unknown major":           func(e *Envelope) { e.SchemaMajor = 2 },
		"unknown type":            func(e *Envelope) { e.Type = "command.request" },
		"empty identity":          func(e *Envelope) { e.EventID = "" },
		"identity whitespace":     func(e *Envelope) { e.EventID = " event-1" },
		"identity control":        func(e *Envelope) { e.EventID = "event\n1" },
		"identity too long":       func(e *Envelope) { e.EventID = strings.Repeat("x", 257) },
		"empty source":            func(e *Envelope) { e.Source = "" },
		"empty correlation":       func(e *Envelope) { e.CorrelationID = "" },
		"empty occurred":          func(e *Envelope) { e.OccurredAt = time.Time{} },
		"empty accepted":          func(e *Envelope) { e.AcceptedAt = time.Time{} },
		"empty tenant":            func(e *Envelope) { e.TenantID = "" },
		"blank tenant":            func(e *Envelope) { e.TenantID = " " },
		"system with tenant":      func(e *Envelope) { e.Scope = ScopeSystem },
		"unknown scope":           func(e *Envelope) { e.Scope = "global" },
		"wrong object":            func(e *Envelope) { e.Object.ID = "other" },
		"wrong object type":       func(e *Envelope) { e.Object.Type = "device" },
		"wrong legacy subject":    func(e *Envelope) { e.LegacySubject = "device.down.user" },
		"empty legacy subject":    func(e *Envelope) { e.LegacySubject = "" },
		"wrong payload":           func(e *Envelope) { e.Payload = json.RawMessage(`{"commandId":"c1"}`) },
		"null payload":            func(e *Envelope) { e.Payload = json.RawMessage(`null`) },
		"array payload":           func(e *Envelope) { e.Payload = json.RawMessage(`[]`) },
		"duplicate payload field": func(e *Envelope) { e.Payload = json.RawMessage(`{"cacheType":"user","key":"bad","key":"good"}`) },
		"payload field alias":     func(e *Envelope) { e.Payload = json.RawMessage(`{"CacheType":"user","key":"good"}`) },
		"empty key":               func(e *Envelope) { e.Payload = json.RawMessage(`{"cacheType":"user","key":""}`) },
		"cache wildcard":          func(e *Envelope) { e.Payload = json.RawMessage(`{"cacheType":"*","key":"key"}`) },
		"cache multiple segments": func(e *Envelope) { e.Payload = json.RawMessage(`{"cacheType":"a.b","key":"key"}`) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// event 表示来自不可信记录的候选信封。
			event := testEvent(t)
			mutate(&event)
			if encoded, err := Encode(event); err == nil || encoded != nil {
				t.Fatal("invalid event encoded")
			}
			if key, err := event.PartitionKey(); err == nil || key != nil {
				t.Fatal("invalid event received partition key")
			}
			// raw 绕过生产编码器，模拟 broker 中实际可能存在的坏消息。
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := Decode(raw); err == nil || !reflect.DeepEqual(got, Envelope{}) {
				t.Fatalf("invalid event was not isolated: %#v, %v", got, err)
			}
		})
	}
}

// TestMalformedJSONIsIsolated 验证重复字段、空值、尾随输入和超限在执行前被拒绝。
func TestMalformedJSONIsIsolated(t *testing.T) {
	// valid 为单字段破坏提供基准。
	valid, err := Encode(testEvent(t))
	if err != nil {
		t.Fatal(err)
	}
	// cases 包括 JSON 库默认会容忍而协议不允许的输入。
	cases := [][]byte{
		nil, []byte("null"), []byte("[]"), []byte("{"), []byte(`{"eventId":"secret-value",`),
		append(append([]byte{}, valid...), []byte(" {}")...),
		append([]byte(`{"eventId":"bad",`), valid[1:]...),
		bytes.Replace(valid, []byte(`"eventId"`), []byte(`"EventID"`), 1),
		bytes.Replace(valid, []byte(`"tenantId":"tenant-1"`), []byte(`"tenantId":null`), 1),
		bytes.Replace(valid, []byte(`"object":{`), []byte(`"object":{"type":"bad",`), 1),
		bytes.Replace(valid, []byte(`"source":"syssvr"`), []byte(`"source":"`+string([]byte{0xff})+`"`), 1),
		append(bytes.Repeat([]byte(" "), MaxEnvelopeBytes), valid...),
		bytes.Replace(valid, []byte(`"schemaMajor":1`), []byte(`"schemaMajor":1.5`), 1),
		bytes.Replace(valid, []byte(`"eventId"`), []byte(`"unknown"`), 1),
	}
	for index, raw := range cases {
		if got, err := Decode(raw); err == nil || !reflect.DeepEqual(got, Envelope{}) {
			t.Fatalf("case %d escaped isolation: %v", index, err)
		} else if strings.Contains(err.Error(), "secret-value") {
			t.Fatal("error leaked raw value")
		}
	}
}

// TestPartitionKeySeparatesScopesAndObjects 防止对象分隔符、租户与系统范围发生分区身份碰撞。
func TestPartitionKeySeparatesScopesAndObjects(t *testing.T) {
	// base 提供统一操作的公共身份。
	base := testEvent(t)
	// seen 收集不同对象的分区身份。
	seen := map[string]bool{}
	// cases 同时覆盖点号、引号与 JSON 文本形式的原始键。
	cases := []struct {
		scope                  Scope
		tenant, cacheType, key string
	}{
		{ScopeTenant, "tenant-a", "a", "b.c"}, {ScopeTenant, "tenant-a", "a-b", "c"},
		{ScopeTenant, "tenant-b", "a", "b.c"}, {ScopeSystem, "", "a", "b.c"},
		{ScopeTenant, "tenant-a", "a", `["b","c"]`}, {ScopeTenant, "tenant-a", "a", `b","c`},
	}
	for _, item := range cases {
		// event 使用真实作用域及对象重新构建受控身份。
		event, err := NewCacheInvalidation(Metadata{EventID: base.EventID, Source: base.Source, OccurredAt: base.OccurredAt, AcceptedAt: base.AcceptedAt,
			Scope: item.scope, TenantID: item.tenant, CorrelationID: base.CorrelationID}, CacheInvalidation{CacheType: item.cacheType, Key: item.key})
		if err != nil {
			t.Fatal(err)
		}
		// key 是当前输入组合唯一的规范化身份。
		key, err := event.PartitionKey()
		if err != nil {
			t.Fatal(err)
		}
		if seen[string(key)] {
			t.Fatalf("partition identity collision: %s", key)
		}
		seen[string(key)] = true
	}
}

// TestConstructorRejectsInvalidUTF8 验证构造过程不以替换字符偷偷改变缓存键。
func TestConstructorRejectsInvalidUTF8(t *testing.T) {
	// base 为测试提供完整公共元数据。
	base := testEvent(t)
	if _, err := NewCacheInvalidation(Metadata{EventID: base.EventID, Source: base.Source, OccurredAt: base.OccurredAt, AcceptedAt: base.AcceptedAt,
		Scope: base.Scope, TenantID: base.TenantID, CorrelationID: base.CorrelationID}, CacheInvalidation{CacheType: "user", Key: string([]byte{0xff})}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

// FuzzDecodeIsolation 检查任意输入不能让未验证的部分消息越过解码边界。
func FuzzDecodeIsolation(f *testing.F) {
	// valid 为模糊测试提供完整 schema 种子。
	valid, err := Encode(testEvent(f))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"schemaMajor":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		// event 只有错误为空时才允许继续交给业务。
		event, err := Decode(raw)
		if err != nil {
			if !reflect.DeepEqual(event, Envelope{}) {
				t.Fatal("error returned partial event")
			}
			return
		}
		if err := event.Validate(); err != nil {
			t.Fatal("unvalidated event escaped decode")
		}
		if _, err := Encode(event); err != nil {
			t.Fatal("decoded event cannot encode")
		}
	})
}
