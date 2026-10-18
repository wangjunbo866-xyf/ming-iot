// 本文件验证已实施通道、消费者隔离和兼容选择器的正反例。
package eventbus

import "testing"

// TestCatalogOnlyOpensCoordination 防止未来设计通道被误当作当前可用能力。
func TestCatalogOnlyOpensCoordination(t *testing.T) {
	if err := ValidateTopic(CoordinationTopic); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"", "iot.bus.uplink.v1.g01", "iot.bus.command.v1.mqtt.g01", "iot.bus.reply.v1.g01", "iot.bus.domain.v1.g01", "iot.bus.view.v1.g01", "iot.bus.coordination.v2.g01", "iot.bus.coordination.v1.g02", "iot.data.raw.v1.mqtt.g01"} {
		if err := ValidateTopic(topic); err == nil {
			t.Fatalf("unsupported topic accepted: %s", topic)
		}
	}
}

// TestExplicitConsumerGroups 验证同逻辑订阅者竞争、不同稳定实例广播且不混淆身份边界。
func TestExplicitConsumerGroups(t *testing.T) {
	// first 和 second 对应同一角色的两个长期稳定实例。
	first, err := BroadcastGroup("api", "instance-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := BroadcastGroup("api", "instance-b")
	if err != nil {
		t.Fatal(err)
	}
	// restart 表示同一实例重启复用身份。
	restart, err := BroadcastGroup("api", "instance-a")
	if err != nil || first != restart || first == second || first != "coord.api.instance-a.v1" {
		t.Fatal("broadcast group identity invalid")
	}
	// competitors 表示同一个明确逻辑任务的副本。
	competitors, err := CompetingGroup("timer.cleanup")
	if err != nil || competitors != "jobs.timer.cleanup.v1" {
		t.Fatal("logical group invalid")
	}
	// other 确保不同任务不共享同一竞争组。
	other, err := CompetingGroup("timer.refresh")
	if err != nil || other == competitors || other == first {
		t.Fatal("logical subscribers collide")
	}
	for _, bad := range []string{"", " ", ".", "a..b", "a.*", "a.>", "a/secret"} {
		if _, err := CompetingGroup(bad); err == nil {
			t.Fatalf("invalid logical group accepted: %q", bad)
		}
		if _, err := BroadcastGroup("api", bad); err == nil {
			t.Fatalf("invalid instance accepted: %q", bad)
		}
	}
	if _, err := BroadcastGroup("api.a", "b"); err == nil {
		t.Fatal("ambiguous role accepted")
	}
	if _, err := BroadcastGroup("api", "a.b"); err == nil {
		t.Fatal("ambiguous instance accepted")
	}
}

// TestMatchSubject 检查星号单段、大于号一或多段及非法选择器拒绝。
func TestMatchSubject(t *testing.T) {
	// cases 覆盖动态任务兼容选择器的边界，不代表已实现任务执行器。
	cases := []struct {
		selector, subject string
		match, invalid    bool
	}{
		{"server.cache.sync.user", "server.cache.sync.user", true, false},
		{"server.cache.sync.*", "server.cache.sync.user", true, false},
		{"server.cache.sync.*", "server.cache.sync", false, false},
		{"server.cache.sync.*", "server.cache.sync.user.extra", false, false},
		{"server.cache.sync.>", "server.cache.sync.user", true, false},
		{"server.cache.sync.>", "server.cache.sync.user.extra", true, false},
		{"server.cache.sync.>", "server.cache.sync", false, false},
		{"a.*.c", "a.b.c", true, false}, {"a.*.c", "a.b.d", false, false},
		{"a.*.>", "a.b", false, false}, {"a.*.>", "a.b.c", true, false},
		{">", "a", true, false}, {">", "a.b", true, false},
		{"*", "a", true, false}, {"*", "a.b", false, false},
		{"a", "ab", false, false}, {"a.b", "a.b.c", false, false},
		{"a.>.b", "a.b", false, true}, {"a.b*", "a.bc", false, true},
		{"a.>x", "a.b", false, true}, {"a..b", "a.b", false, true},
		{"", "a", false, true}, {"a", "", false, true}, {"a.", "a.b", false, true},
		{"a.>", "a..b", false, true}, {"a.>", "a.*", false, true},
		{"a.>", "a.>", false, true}, {"a.>", "a. b", false, true},
		{"a. b", "a.b", false, true}, {"a.>", "a.\n", false, true},
	}
	for _, item := range cases {
		// matched 只有输入合法且逐段匹配时才允许筛选通过。
		matched, err := MatchSubject(item.selector, item.subject)
		if (err != nil) != item.invalid || matched != item.match {
			t.Errorf("%q / %q: got %v, %v", item.selector, item.subject, matched, err)
		}
	}
}
