package eventBus

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/events"
)

const integrationDebtIssue = "https://gitee.com/ming-industry/ming-iot/issues/IKD8AN"

// 环境变量约定：
// NATS_URL、NATS_USER、NATS_PASS（可选）
// MQTT_BROKERS（逗号分隔）、MQTT_USER、MQTT_PASS（可选）、MQTT_CLIENT_ID（可选）

// TestHandleFuncCompatibility 测试通用 HandleFunc 的兼容性
func TestHandleFuncCompatibility(t *testing.T) {
	// 测试通用 HandleFunc 定义
	var handleFunc events.HandleFunc = func(ctx context.Context, ts time.Time, msg []byte) error {
		return nil
	}

	// 验证函数签名正确
	if handleFunc == nil {
		t.Fatal("HandleFunc 定义失败")
	}

	// 测试调用
	err := handleFunc(context.Background(), time.Now(), []byte("test"))
	if err != nil {
		t.Fatalf("HandleFunc 调用失败: %v", err)
	}
}

func TestNATS_SendReceive(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("NATS_URL"))
	if url == "" {
		t.Skipf("未设置 NATS_URL，跳过需要真实 NATS 的集成测试；跟踪：%s", integrationDebtIssue)
	}
	resetFastEventForTest(t)

	c := conf.EventConf{
		Mode: conf.EventModeNats,
		Nats: conf.NatsConf{
			Url:   url,
			User:  os.Getenv("NATS_USER"),
			Pass:  os.Getenv("NATS_PASS"),
			Token: os.Getenv("NATS_TOKEN"),
		},
	}

	bus, err := NewFastEvent(c, "share-test", time.Now().UnixNano())
	if err != nil {
		t.Fatalf("NewFastEvent(NATS) 失败: %v", err)
	}

	topic := "server.test.nats.echo"
	recv := make(chan string, 1)

	if err := bus.Subscribe(topic, func(ctx context.Context, t time.Time, body []byte) error {
		recv <- string(body)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe 失败: %v", err)
	}

	if err := bus.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := "hello-nats"
	if err := bus.Publish(ctx, topic, msg); err != nil {
		t.Fatalf("Publish 失败: %v", err)
	}

	select {
	case got := <-recv:
		if got != msg {
			t.Fatalf("收消息不一致: got=%q want=%q", got, msg)
		}
	case <-ctx.Done():
		t.Fatalf("等待接收超时")
	}
}

func TestMQTT_SendReceive(t *testing.T) {
	rawBrokers := strings.TrimSpace(os.Getenv("MQTT_BROKERS"))
	if rawBrokers == "" {
		t.Skipf("未设置 MQTT_BROKERS，跳过需要真实 MQTT 的集成测试；跟踪：%s", integrationDebtIssue)
	}
	resetFastEventForTest(t)

	brokers := strings.Split(rawBrokers, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	c := conf.EventConf{
		Mode: conf.EventModeMqtt,
		Mqtt: conf.MqttConf{
			ClientID: firstNonEmpty(os.Getenv("MQTT_CLIENT_ID"), "share-test-client"),
			Brokers:  brokers,
			User:     os.Getenv("MQTT_USER"),
			Pass:     os.Getenv("MQTT_PASS"),
			ConnNum:  1,
		},
	}

	bus, err := NewFastEvent(c, "share-test", time.Now().UnixNano())
	if err != nil {
		t.Fatalf("NewFastEvent(MQTT) 失败: %v", err)
	}

	// 使用点分主题，内部会转换为 MQTT 斜杠主题
	topic := "server/test/mqtt/echo"
	recv := make(chan string, 1)

	if err := bus.Subscribe(topic, func(ctx context.Context, t time.Time, body []byte) error {
		recv <- string(body)
		return nil
	}); err != nil {
		t.Fatalf("Subscribe 失败: %v", err)
	}

	if err := bus.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()

	msg := "hello-mqtt"
	if err := bus.Publish(ctx, topic, msg); err != nil {
		t.Fatalf("Publish 失败: %v", err)
	}

	select {
	case got := <-recv:
		if got != msg {
			t.Fatalf("收消息不一致: got=%q want=%q", got, msg)
		}
	case <-ctx.Done():
		t.Fatalf("等待接收超时")
	}
}

func resetFastEventForTest(t *testing.T) {
	t.Helper()
	fastEvent = nil
	fastOnce = sync.Once{}
	t.Cleanup(func() {
		fastEvent = nil
		fastOnce = sync.Once{}
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
