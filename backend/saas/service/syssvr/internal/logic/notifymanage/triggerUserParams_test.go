package notifymanagelogic

import (
	"strings"
	"testing"
)

func TestFormatTriggerUserLine(t *testing.T) {
	auto := FormatTriggerUserLine(triggerUserFields{Type: triggerTypeAuto})
	if auto != "系统自动触发" {
		t.Fatalf("auto=%q", auto)
	}
	autoUser := FormatTriggerUserLine(triggerUserFields{Type: triggerTypeAuto, Nick: "李四"})
	if autoUser != "由 李四 触发" {
		t.Fatalf("autoUser=%q", autoUser)
	}
	manual := FormatTriggerUserLine(triggerUserFields{Type: triggerTypeManual, Nick: "张三"})
	if manual != "由 张三 手动触发" {
		t.Fatalf("manual=%q", manual)
	}
}

func TestFormatTriggerUserLineManualPrefersNick(t *testing.T) {
	fields := triggerUserFields{
		Type:    triggerTypeManual,
		UserID:  42,
		Nick:    " 张三 ",
		Account: " 13800138000 ",
	}
	want := "由 张三 手动触发"
	if got := FormatTriggerUserLine(fields); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatTriggerUserLineManualFallbacks(t *testing.T) {
	tests := []struct {
		name   string
		fields triggerUserFields
		want   string
	}{
		{"empty nick", triggerUserFields{Account: " 13800138000 ", UserID: 42}, "由 13800138000 手动触发"},
		{"blank nick", triggerUserFields{Nick: " \t\n ", Account: " 13800138000 ", UserID: 42}, "由 13800138000 手动触发"},
		{"positive ID", triggerUserFields{Nick: " \t ", Account: " \n ", UserID: 42}, "由 42 手动触发"},
		{"anonymous", triggerUserFields{}, "由用户手动触发"},
		{"nonpositive ID", triggerUserFields{Nick: " ", Account: " ", UserID: -1}, "由用户手动触发"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fields.Type = triggerTypeManual
			if got := FormatTriggerUserLine(tt.fields); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFormatTriggerUserLineNonManualKeepsAccountPriority(t *testing.T) {
	for _, triggerType := range []string{triggerTypeAuto, triggerTypeDeviceButton, "unknown", ""} {
		t.Run(triggerType, func(t *testing.T) {
			fields := triggerUserFields{Type: triggerType, Nick: " 张三 ", Account: " 13800138000 ", UserID: 42}
			want := "由 13800138000 触发"
			if got := FormatTriggerUserLine(fields); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
	if got := FormatTriggerUserLine(triggerUserFields{Type: triggerTypeAuto, Nick: "张三", Account: " 系统自动 "}); got != "系统自动触发" {
		t.Fatalf("auto system account=%q", got)
	}
}

func TestFormatSceneButtonTriggerLine(t *testing.T) {
	got := FormatSceneButtonTriggerLine(triggerUserFields{
		Type:    triggerTypeDeviceButton,
		Account: "13800138000",
	}, "按钮1")
	want := "13800138000触发了按钮1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	gotNick := FormatSceneButtonTriggerLine(triggerUserFields{
		Type:    triggerTypeDeviceButton,
		Nick:    "超级",
		Account: "13800138000",
	}, "遥控器1 开启")
	wantNick := "13800138000触发了遥控器1 开启"
	if gotNick != wantNick {
		t.Fatalf("prefer account: got %q want %q", gotNick, wantNick)
	}
}

func TestCompactRuleSceneActionBody(t *testing.T) {
	raw := "1 遥控器1 开启\nWIFI数字遥控1 遥控器1 开启"
	got := compactRuleSceneActionBody(raw)
	want := "WIFI数字遥控1 遥控器1 开启"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCompactRuleSceneActionBodyCountsRunesNotBytes(t *testing.T) {
	// 按 Unicode 字符数而非字节数比较："ab" 为 2 字符 2 字节，"中" 为 1 字符 3 字节，按字节会得出相反结果。
	tests := []struct {
		name string
		body string
		want string
	}{
		{"cjk bytes first", "中\nab", "ab"},
		{"cjk bytes later", "ab\n中", "ab"},
		{"cjk bytes middle", "a\n中中\nabcd", "abcd"},
		{"cjk bytes exceed latin runes", "开关开开\nremote", "remote"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRuleSceneActionBody(tt.body); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCompactRuleSceneActionBodyEqualRunesKeepsFirst(t *testing.T) {
	// 相同 Unicode 字符数取首个非空行："abcd" 与 "中文中文" 同为 4 字符但字节数不同。
	tests := []struct {
		name string
		body string
		want string
	}{
		{"same runes and bytes", "一二三\n四五六", "一二三"},
		{"same runes different bytes", "abcd\n中文中文", "abcd"},
		{"winner repeated later", "abcd\n中文中文\nabcd", "abcd"},
		{"blank lines and padding", "  abcd  \n\n\t中文中文\t\n", "abcd"},
		{"three way tie", "abc\n中文中\ndef", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRuleSceneActionBody(tt.body); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCompactRuleSceneActionBodySelectionBoundaries(t *testing.T) {
	// 防止缓存最长字符数后漏更新、同长覆盖首个，或把空白行作为动作。
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty", "", ""},
		{"blank only", " \t\r\n\u3000\n ", ""},
		{"later winner keeps first tie", "a\n 中华人民 \r\n abcd ", "中华人民"},
		{"later winner replaced again", "a\n 中华人民 \r\n abcdef ", "abcdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRuleSceneActionBody(tt.body); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkCompactRuleSceneActionBody(b *testing.B) {
	// 合成长首行场景，衡量重复扫描成本，不代表真实模板分布或发送吞吐量。
	want := strings.Repeat("长", 4096)
	body := want + strings.Repeat("\n开启", 128)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := compactRuleSceneActionBody(body); got != want {
			b.Fatalf("selected action differs from the longest first line")
		}
	}
}

func TestRuleSceneNotifyLinePicksRuneLongestAction(t *testing.T) {
	fields := triggerUserFields{Type: triggerTypeDeviceButton, Account: "13800138000", UserID: 42}
	if got := FormatRuleSceneNotifyLine(fields, "中\nab"); got != "13800138000触发了ab" {
		t.Fatalf("rune count=%q", got)
	}
	if got := FormatRuleSceneNotifyLine(fields, "abcd\n中文中文"); got != "13800138000触发了abcd" {
		t.Fatalf("equal runes first=%q", got)
	}
}

func TestPrependTriggerUserLine(t *testing.T) {
	got := PrependTriggerUserLine("设备已联动", triggerUserFields{Type: triggerTypeManual, Nick: "张三"})
	want := "由 张三 手动触发\n设备已联动"
	if got != want {
		t.Fatalf("prepend=%q", got)
	}
	legacy := PrependTriggerUserLine("设备已联动\n由 张三 手动触发", triggerUserFields{Type: triggerTypeManual, Nick: "张三"})
	if legacy != want {
		t.Fatalf("legacy=%q", legacy)
	}
}

func TestManualTriggerBodyCompatibility(t *testing.T) {
	fields := triggerUserFields{Type: triggerTypeManual, Nick: " 张三 ", Account: "13800138000", UserID: 42}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty body", " \n ", "由 张三 手动触发"},
		{"body only", " 设备已联动 ", "由 张三 手动触发\n设备已联动"},
		{"line only", "由 张三 手动触发", "由 张三 手动触发"},
		{"existing prefix", "由 张三 手动触发\n设备已联动", "由 张三 手动触发\n设备已联动"},
		{"legacy suffix", "设备已联动\n由 张三 手动触发", "由 张三 手动触发\n设备已联动"},
		{"old account prefix", "由 13800138000 手动触发\n设备已联动", "由 张三 手动触发\n设备已联动"},
		{"old account suffix", "设备已联动\n由 13800138000 手动触发", "由 张三 手动触发\n设备已联动"},
		{"old account line only", "由 13800138000 手动触发", "由 张三 手动触发"},
		{"old account in middle", "开始\n由 13800138000 手动触发\n结束", "由 张三 手动触发\n开始\n由 13800138000 手动触发\n结束"},
		{"old account boundaries and middle", "由 13800138000 手动触发\n开始\n由 13800138000 手动触发\n结束\n由 13800138000 手动触发", "由 张三 手动触发\n开始\n由 13800138000 手动触发\n结束"},
		{"new prefix and old suffix", "由 张三 手动触发\n设备已联动\n由 13800138000 手动触发", "由 张三 手动触发\n设备已联动"},
		{"partial old prefix", "由 13800138000 手动触发后继续执行", "由 张三 手动触发\n由 13800138000 手动触发后继续执行"},
		{"partial old suffix", "操作记录：由 13800138000 手动触发", "由 张三 手动触发\n操作记录：由 13800138000 手动触发"},
		{"different account", "由 13900139000 手动触发\n设备已联动", "由 张三 手动触发\n由 13900139000 手动触发\n设备已联动"},
	}
	for _, formatter := range []struct {
		name string
		fn   func(string, triggerUserFields) string
	}{
		{"notification", PrependTriggerUserLine},
		{"rule scene", PrependRuleSceneTriggerLine},
		{"legacy entry", AppendTriggerUserLine},
	} {
		t.Run(formatter.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := formatter.fn(tt.body, fields); got != tt.want {
						t.Fatalf("got %q want %q", got, tt.want)
					}
				})
			}
		})
	}
}

func TestManualTriggerBodyPreservesInteriorLegacyLine(t *testing.T) {
	fields := triggerUserFields{Type: triggerTypeManual, Nick: "张三", Account: "13800138000"}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"three LF lines", "由 13800138000 手动触发\n由 13800138000 手动触发\n由 13800138000 手动触发", "由 张三 手动触发\n由 13800138000 手动触发"},
		{"only legacy line", "由 13800138000 手动触发", "由 张三 手动触发"},
		{"two boundary lines", "由 13800138000 手动触发\n由 13800138000 手动触发", "由 张三 手动触发"},
		{"four LF lines", "由 13800138000 手动触发\n由 13800138000 手动触发\n由 13800138000 手动触发\n由 13800138000 手动触发", "由 张三 手动触发\n由 13800138000 手动触发\n由 13800138000 手动触发"},
		{"mixed CRLF and LF", "由 13800138000 手动触发\r\n由 13800138000 手动触发\n由 13800138000 手动触发", "由 张三 手动触发\n由 13800138000 手动触发"},
		{"three CRLF lines", "由 13800138000 手动触发\r\n由 13800138000 手动触发\r\n由 13800138000 手动触发", "由 张三 手动触发\n由 13800138000 手动触发"},
	}
	for _, entry := range []struct {
		name string
		fn   func(string, triggerUserFields) string
	}{
		{"notification", PrependTriggerUserLine},
		{"legacy alias", AppendTriggerUserLine},
		{"manual scene", PrependRuleSceneTriggerLine},
	} {
		t.Run(entry.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := entry.fn(tt.body, fields); got != tt.want {
						t.Fatalf("got %q want %q", got, tt.want)
					}
				})
			}
		})
	}
}

func TestTriggerBodyCompleteLinesWithCRLF(t *testing.T) {
	// BODY-COMPATIBILITY: 完整首行使用 CRLF 时仍应识别；其余正文原样保留。
	fields := triggerUserFields{Type: triggerTypeManual, Nick: "张三", Account: "13800138000"}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"current prefix", "由 张三 手动触发\r\n设备已联动\r\n请检查", "由 张三 手动触发\r\n设备已联动\r\n请检查"},
		{"legacy prefix", "由 13800138000 手动触发\r\n设备已联动\r\n请检查", "由 张三 手动触发\n设备已联动\r\n请检查"},
		{"legacy boundaries and middle", "由 13800138000 手动触发\r\n开始\r\n由 13800138000 手动触发\r\n结束\r\n由 13800138000 手动触发", "由 张三 手动触发\n开始\r\n由 13800138000 手动触发\r\n结束"},
		{"partial prefix", "由 张三 手动触发后继续\r\n设备已联动", "由 张三 手动触发\n由 张三 手动触发后继续\r\n设备已联动"},
		{"bare carriage return is not CRLF", "由 张三 手动触发\r设备已联动", "由 张三 手动触发\n由 张三 手动触发\r设备已联动"},
	}
	for _, entry := range []struct {
		name string
		fn   func(string, triggerUserFields) string
	}{
		{"notification", PrependTriggerUserLine},
		{"legacy alias", AppendTriggerUserLine},
		{"manual scene", PrependRuleSceneTriggerLine},
	} {
		t.Run(entry.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := entry.fn(tt.body, fields); got != tt.want {
						t.Fatalf("got %q want %q", got, tt.want)
					}
				})
			}
		})
	}
	// 没有触发人时不做换行标准化；自动触发仍用账号且保留完整首行。
	body := " 开始\r\n结束 "
	if got := PrependTriggerUserLine(body, triggerUserFields{}); got != body {
		t.Fatalf("empty trigger changed body: %q", got)
	}
	auto := triggerUserFields{Type: triggerTypeAuto, Nick: "张三", Account: "13800138000"}
	body = "由 13800138000 触发\r\n设备已联动"
	if got := PrependTriggerUserLine(body, auto); got != body {
		t.Fatalf("auto prefix: got %q want %q", got, body)
	}
}

func TestNonManualTriggerBodyPreservesManualAccountLine(t *testing.T) {
	body := "由 13800138000 手动触发\n设备已联动\n由 13800138000 手动触发"
	want := "由 13800138000 触发\n由 13800138000 手动触发\n设备已联动\n由 13800138000 手动触发"
	for _, triggerType := range []string{triggerTypeAuto, triggerTypeDeviceButton, "unknown", ""} {
		t.Run(triggerType, func(t *testing.T) {
			fields := triggerUserFields{Type: triggerType, Nick: "张三", Account: "13800138000"}
			if got := PrependTriggerUserLine(body, fields); got != want {
				t.Fatalf("notification: got %q want %q", got, want)
			}
			if got := PrependRuleSceneTriggerLine(body, fields); got != want {
				t.Fatalf("rule scene: got %q want %q", got, want)
			}
		})
	}
}

func TestRuleSceneTriggerKeepsDeviceButtonBehavior(t *testing.T) {
	fields := triggerUserFields{Type: triggerTypeDeviceButton, Nick: "张三", Account: " 13800138000 ", UserID: 42}
	body := "开启\n遥控器1 开启"
	if got := FormatRuleSceneNotifyLine(fields, body); got != "13800138000触发了遥控器1 开启" {
		t.Fatalf("device button line=%q", got)
	}
	if got := PrependRuleSceneTriggerLine(body, fields); got != "13800138000触发了遥控器1 开启\n开启\n遥控器1 开启" {
		t.Fatalf("device button body=%q", got)
	}
	fields.UserID = 0
	if got := FormatRuleSceneNotifyLine(fields, body); got != "由 13800138000 触发" {
		t.Fatalf("without positive ID=%q", got)
	}
}
