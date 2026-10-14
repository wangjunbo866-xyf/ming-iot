//go:build integration

package mqttClient

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gitee.com/ming-iot/share/conf"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

const (
	emqxContainerName = "mqtt-test-emqx"
	emqxMqttPort      = "18834"
	emqxBroker        = "tcp://localhost:18834"
	emqxNetwork       = "bridge"
)

// resetMqttClientState 在集成测试之间清理包级状态，避免 sync.Once 及后台重连 client 导致测试互相影响
func resetMqttClientState() {
	for _, mc := range mqttClient {
		for _, c := range mc.clients {
			c.Disconnect(0)
		}
	}
	mqttSetOnConnectHandler = nil
	mqttClient = map[string]*MqttClient{}
	initOnce = map[string]*sync.Once{}
	// 测试环境使用较短的 reconnect 间隔，避免 paho 指数退避导致长时间等待
	mqttMaxReconnectInterval = 1 * time.Second
	mqttConnectRetryInterval = 500 * time.Millisecond
}

func waitConnected(cli *MqttClient, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cli.IsConnected() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// waitBrokerReady 等待 broker 端口可连接
func waitBrokerReady(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// waitMqttReady 通过实际发起 MQTT 连接来确认 broker 已可处理 MQTT 协议（EMQX 启动时 TCP 端口就绪往往早于 MQTT 服务就绪）
func waitMqttReady(broker string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		opts := mqtt.NewClientOptions().
			AddBroker(broker).
			SetClientID("probe-" + uuid.NewString())
		c := mqtt.NewClient(opts)
		token := c.Connect()
		ok := token.WaitTimeout(2 * time.Second)
		if err := token.Error(); ok && err == nil {
			c.Disconnect(100)
			return true
		}
		if c.IsConnected() {
			c.Disconnect(100)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// ensureEMQXRunning 确保测试开始前 EMQX 容器在运行、已接入 bridge 网络且 MQTT 服务已就绪
func ensureEMQXRunning(t *testing.T) {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-q", "--filter", "name="+emqxContainerName).Output()
	if err != nil || len(out) == 0 {
		t.Log("EMQX 容器未运行，尝试启动...")
		exec.Command("docker", "start", emqxContainerName).Run()
	}
	// 如果之前测试把容器从网络断开，确保重新接回
	exec.Command("docker", "network", "connect", emqxNetwork, emqxContainerName).Run()
	if !assert.True(t, waitBrokerReady("localhost:"+emqxMqttPort, 10*time.Second), "broker TCP 端口应在 10s 内就绪") {
		t.Fatal("broker TCP 端口未就绪")
	}
	if !assert.True(t, waitMqttReady(emqxBroker, 60*time.Second), "broker MQTT 服务应在 60s 内就绪") {
		t.Fatal("broker MQTT 服务未就绪")
	}
}

// disconnectEMQX 通过 docker network disconnect 模拟 broker 网络分区，不重启容器
func disconnectEMQX(t *testing.T) {
	t.Helper()
	t.Log("disconnecting emqx from network...")
	err := exec.Command("docker", "network", "disconnect", emqxNetwork, emqxContainerName).Run()
	assert.NoError(t, err)
	time.Sleep(2 * time.Second)
}

// connectEMQX 恢复 EMQX 网络连接，等待 MQTT 服务真正就绪
func connectEMQX(t *testing.T) {
	t.Helper()
	t.Log("reconnecting emqx to network...")
	err := exec.Command("docker", "network", "connect", emqxNetwork, emqxContainerName).Run()
	assert.NoError(t, err)
	if !assert.True(t, waitBrokerReady("localhost:"+emqxMqttPort, 10*time.Second), "broker TCP 端口应在 10s 内就绪") {
		t.Fatal("broker TCP 端口未就绪")
	}
	if !assert.True(t, waitMqttReady(emqxBroker, 60*time.Second), "broker MQTT 服务应在 60s 内就绪") {
		t.Fatal("broker MQTT 服务未就绪")
	}
}

// kickTestClients 通过 emqx_ctl clients kick 踢掉被测 client，触发其自动重连
func kickTestClients(t *testing.T, cli *MqttClient) {
	t.Helper()
	t.Log("kicking test clients...")
	for _, c := range cli.clients {
		reader := c.OptionsReader()
		clientID := reader.ClientID()
		out, err := exec.Command("docker", "exec", emqxContainerName, "emqx_ctl", "clients", "kick", clientID).CombinedOutput()
		if err != nil {
			t.Logf("kick client %s output: %s, err: %v", clientID, string(out), err)
		}
	}
}

// newPublisher 创建一个独立的 paho publisher client，用于向被测 client 发布消息
func newPublisher(t *testing.T, broker string) mqtt.Client {
	t.Helper()
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("publisher-" + uuid.NewString()).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(1 * time.Second).
		SetMaxReconnectInterval(3 * time.Second)
	c := mqtt.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("publisher 连接超时")
	}
	if err := token.Error(); err != nil {
		t.Fatalf("publisher 连接失败: %v", err)
	}
	return c
}

func publishWithPublisher(t *testing.T, publisher mqtt.Client, topic string, payload []byte) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !publisher.IsConnected() {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		token := publisher.Publish(topic, 1, false, payload)
		if !token.WaitTimeout(5 * time.Second) {
			t.Logf("publisher 发布到 %s 单次超时，将重试", topic)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := token.Error(); err != nil {
			t.Logf("publisher 发布到 %s 失败: %v，将重试", topic, err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return
	}
	t.Fatalf("publisher 未能在 30s 内发布到 %s", topic)
}

// belongsToTest 通过 clientID 前缀判断该 mqtt.Client 是否属于当前测试
func belongsToTest(client mqtt.Client, cfg *conf.MqttConf) bool {
	reader := client.OptionsReader()
	return strings.HasPrefix(reader.ClientID(), cfg.ClientID+"_")
}

func TestMqttReconnect_SubscribeRestore(t *testing.T) {
	resetMqttClientState()
	ensureEMQXRunning(t)
	cfg := &conf.MqttConf{
		ClientID: "integration-test",
		Brokers:  []string{emqxBroker},
		User:     "",
		Pass:     "",
		ConnNum:  1,
	}

	topic := "test/reconnect"
	var received int32
	var connectCount int32
	onConnectCh := make(chan struct{}, 10)

	cli, err := NewMqttClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, cli)
	assert.True(t, waitConnected(cli, 5*time.Second), "首次连接应在 5s 内完成")

	publisher := newPublisher(t, cfg.Brokers[0])
	defer publisher.Disconnect(250)

	// 首次订阅手动完成（OnConnect handler 在 NewMqttClient 之前设置会错过首次连接）
	err = cli.SubscribeRaw(nil, topic, 1, func(c mqtt.Client, m mqtt.Message) {
		atomic.AddInt32(&received, 1)
		t.Logf("received message: %s", string(m.Payload()))
	})
	assert.NoError(t, err)

	// 设置重连恢复回调：重连成功后自动重新订阅
	SetMqttSetOnConnectHandler(func(client mqtt.Client) {
		if !belongsToTest(client, cfg) {
			return
		}
		cnt := atomic.AddInt32(&connectCount, 1)
		t.Logf("OnConnect callback invoked, count=%d", cnt)
		err := cli.SubscribeRaw(client, topic, 1, func(c mqtt.Client, m mqtt.Message) {
			atomic.AddInt32(&received, 1)
			t.Logf("received message: %s", string(m.Payload()))
		})
		if err != nil {
			t.Logf("re-subscribe error: %v", err)
		}
		onConnectCh <- struct{}{}
	})

	// 第一次发布消息，验证初始订阅已生效
	publishWithPublisher(t, publisher, topic, []byte("msg1"))
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) >= 1
	}, 10*time.Second, 200*time.Millisecond, "首次订阅后应收到消息")

	// 断开网络模拟 broker 不可达
	disconnectEMQX(t)

	// 恢复网络
	connectEMQX(t)

	// 等待 paho 自动重连 + OnConnect 回调重新订阅
	if !waitOnConnect(onConnectCh, 60*time.Second, 2*time.Second) {
		t.Fatal("未能在 60s 内重连并重新订阅")
	}
	assert.True(t, cli.IsConnected(), "重连后 client 应处于已连接状态")

	// 重连后发布消息
	publishWithPublisher(t, publisher, topic, []byte("msg2"))

	assert.GreaterOrEqual(t, atomic.LoadInt32(&connectCount), int32(1), "重连应至少触发 1 次 OnConnect 回调")
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) >= 2
	}, 10*time.Second, 200*time.Millisecond, "重连后应继续收到消息，订阅已恢复")
}

func TestMqttReconnect_WithoutHandler_SubscribeLost(t *testing.T) {
	resetMqttClientState()
	ensureEMQXRunning(t)
	cfg := &conf.MqttConf{
		ClientID: "integration-test-no-handler",
		Brokers:  []string{emqxBroker},
		User:     "",
		Pass:     "",
		ConnNum:  1,
	}

	cli, err := NewMqttClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, cli)

	publisher := newPublisher(t, cfg.Brokers[0])
	defer publisher.Disconnect(250)

	var received int32
	topic := "test/reconnect/no-handler"

	assert.True(t, waitConnected(cli, 5*time.Second), "应在 5s 内连接成功")

	// 不设置任何重连回调，只进行一次订阅
	err = cli.SubscribeRaw(nil, topic, 1, func(c mqtt.Client, m mqtt.Message) {
		atomic.AddInt32(&received, 1)
		t.Logf("received message: %s", string(m.Payload()))
	})
	assert.NoError(t, err)

	publishWithPublisher(t, publisher, topic, []byte("msg1"))
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) >= 1
	}, 10*time.Second, 200*time.Millisecond, "首次订阅后应收到消息")

	// 断开并恢复网络
	disconnectEMQX(t)
	connectEMQX(t)

	// 等待重连成功
	assert.True(t, waitConnected(cli, 60*time.Second), "应在 60s 内重连")
	time.Sleep(2 * time.Second)

	// 重连后发布消息
	publishWithPublisher(t, publisher, topic, []byte("msg2"))

	// 由于没有设置重连回调，重连后订阅丢失，不应收到 msg2
	assert.Never(t, func() bool {
		return atomic.LoadInt32(&received) >= 2
	}, 3*time.Second, 200*time.Millisecond, "未设置重连回调时，重连后订阅应丢失")
}

// TestMqttReconnect_MultipleCycles 使用 emqx_ctl clients kick 快速触发多次断连-重连，验证订阅恢复稳定性
func TestMqttReconnect_MultipleCycles(t *testing.T) {
	resetMqttClientState()
	ensureEMQXRunning(t)
	cfg := &conf.MqttConf{
		ClientID: "integration-test-multi-cycle",
		Brokers:  []string{emqxBroker},
		User:     "",
		Pass:     "",
		ConnNum:  1,
	}

	topic := "test/reconnect/multi"
	var received int32
	var connectCount int32
	onConnectCh := make(chan struct{}, 20)

	cli, err := NewMqttClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, cli)
	assert.True(t, waitConnected(cli, 5*time.Second), "首次连接应在 5s 内完成")

	publisher := newPublisher(t, cfg.Brokers[0])
	defer publisher.Disconnect(250)

	SetMqttSetOnConnectHandler(func(client mqtt.Client) {
		if !belongsToTest(client, cfg) {
			return
		}
		cnt := atomic.AddInt32(&connectCount, 1)
		t.Logf("OnConnect callback invoked, count=%d", cnt)
		err := cli.SubscribeRaw(client, topic, 1, func(c mqtt.Client, m mqtt.Message) {
			atomic.AddInt32(&received, 1)
			t.Logf("received message: %s", string(m.Payload()))
		})
		if err != nil {
			t.Logf("re-subscribe error: %v", err)
		}
		onConnectCh <- struct{}{}
	})

	// 首次订阅
	err = cli.SubscribeRaw(nil, topic, 1, func(c mqtt.Client, m mqtt.Message) {
		atomic.AddInt32(&received, 1)
		t.Logf("received message: %s", string(m.Payload()))
	})
	assert.NoError(t, err)

	// 进行 3 次断连-重连循环
	for i := 1; i <= 3; i++ {
		t.Logf("=== cycle %d ===", i)

		// 发布并确认当前订阅生效（prev 必须在发布前读取，避免并发收到消息后 prev 已经包含本次计数）
		prev := atomic.LoadInt32(&received)
		publishWithPublisher(t, publisher, topic, []byte(fmt.Sprintf("msg-before-%d", i)))
		assert.Eventually(t, func() bool {
			return atomic.LoadInt32(&received) > prev
		}, 10*time.Second, 200*time.Millisecond, "断连前应能收到消息")

		// 踢掉 client 触发自动重连
		kickTestClients(t, cli)

		// 等待这次重连的 OnConnect 回调完成
		if !waitOnConnect(onConnectCh, 30*time.Second, 2*time.Second) {
			t.Fatalf("第 %d 次重连未在 30s 内完成", i)
		}
		assert.True(t, cli.IsConnected(), "第 %d 次重连后应处于已连接状态", i)

		// 重连后发布消息
		prev = atomic.LoadInt32(&received)
		publishWithPublisher(t, publisher, topic, []byte(fmt.Sprintf("msg-after-%d", i)))
		assert.Eventually(t, func() bool {
			return atomic.LoadInt32(&received) > prev
		}, 10*time.Second, 200*time.Millisecond, "第 %d 次重连后应继续收到消息", i)
	}

	assert.GreaterOrEqual(t, atomic.LoadInt32(&connectCount), int32(3), "应至少触发 3 次重连 OnConnect 回调")
}

// TestMqttReconnect_LongRunningStress 长时间压力测试：20 次 kick 触发的断连-重连循环，验证订阅恢复稳定性
func TestMqttReconnect_LongRunningStress(t *testing.T) {
	resetMqttClientState()
	ensureEMQXRunning(t)
	cfg := &conf.MqttConf{
		ClientID: "integration-test-stress",
		Brokers:  []string{emqxBroker},
		User:     "",
		Pass:     "",
		ConnNum:  1,
	}

	topic := "test/reconnect/stress"
	var received int32
	var connectCount int32
	onConnectCh := make(chan struct{}, 50)

	cli, err := NewMqttClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, cli)
	assert.True(t, waitConnected(cli, 5*time.Second), "首次连接应在 5s 内完成")

	publisher := newPublisher(t, cfg.Brokers[0])
	defer publisher.Disconnect(250)

	SetMqttSetOnConnectHandler(func(client mqtt.Client) {
		if !belongsToTest(client, cfg) {
			return
		}
		cnt := atomic.AddInt32(&connectCount, 1)
		t.Logf("OnConnect callback invoked, count=%d", cnt)
		err := cli.SubscribeRaw(client, topic, 1, func(c mqtt.Client, m mqtt.Message) {
			atomic.AddInt32(&received, 1)
			t.Logf("received message: %s", string(m.Payload()))
		})
		if err != nil {
			t.Logf("re-subscribe error: %v", err)
		}
		onConnectCh <- struct{}{}
	})

	err = cli.SubscribeRaw(nil, topic, 1, func(c mqtt.Client, m mqtt.Message) {
		atomic.AddInt32(&received, 1)
		t.Logf("received message: %s", string(m.Payload()))
	})
	assert.NoError(t, err)

	cycles := 20
	start := time.Now()
	for i := 1; i <= cycles; i++ {
		t.Logf("=== stress cycle %d/%d ===", i, cycles)

		prev := atomic.LoadInt32(&received)
		publishWithPublisher(t, publisher, topic, []byte(fmt.Sprintf("msg-before-%d", i)))
		assert.Eventually(t, func() bool {
			return atomic.LoadInt32(&received) > prev
		}, 10*time.Second, 200*time.Millisecond, "断连前应能收到消息")

		kickTestClients(t, cli)

		if !waitOnConnect(onConnectCh, 30*time.Second, 2*time.Second) {
			t.Fatalf("第 %d 次重连未在 30s 内完成", i)
		}
		assert.True(t, cli.IsConnected(), "第 %d 次重连后应处于已连接状态", i)

		prev = atomic.LoadInt32(&received)
		publishWithPublisher(t, publisher, topic, []byte(fmt.Sprintf("msg-after-%d", i)))
		assert.Eventually(t, func() bool {
			return atomic.LoadInt32(&received) > prev
		}, 10*time.Second, 200*time.Millisecond, "第 %d 次重连后应继续收到消息", i)
	}

	duration := time.Since(start)
	t.Logf("压力测试完成: %d 次循环, 总耗时 %v, 收到消息数 %d, OnConnect 次数 %d", cycles, duration, atomic.LoadInt32(&received), atomic.LoadInt32(&connectCount))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&connectCount), int32(cycles), "应至少触发 %d 次重连 OnConnect 回调", cycles)
}

// waitOnConnect 等待 OnConnect 回调触发一次，并给 broker 处理 SUBSCRIBE 留 settle 时间
func waitOnConnect(ch <-chan struct{}, timeout time.Duration, settle time.Duration) bool {
	select {
	case <-ch:
		time.Sleep(settle)
		return true
	case <-time.After(timeout):
		return false
	}
}
