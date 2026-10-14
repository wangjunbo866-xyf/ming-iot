package notifymanagelogic

import "testing"

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
