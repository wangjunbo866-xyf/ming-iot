package mqttClient

import (
	"errors"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

// mockToken 实现 mqtt.Token 接口
type mockToken struct {
	err error
	done chan struct{}
}

func newMockToken(err error) *mockToken {
	t := &mockToken{err: err, done: make(chan struct{})}
	close(t.done)
	return t
}

func (m *mockToken) Wait() bool {
	<-m.done
	return true
}

func (m *mockToken) WaitTimeout(timeout time.Duration) bool {
	<-m.done
	return true
}

func (m *mockToken) Done() <-chan struct{} {
	return m.done
}

func (m *mockToken) Error() error {
	return m.err
}

// mockMqttClient 实现 mqtt.Client 接口，用于记录订阅/取消订阅行为
type mockMqttClient struct {
	mu            sync.Mutex
	connected     bool
	subscribes    []subscribeCall
	unsubscribes  []string
	publishes     []publishCall
}

type subscribeCall struct {
	topic    string
	qos      byte
	callback mqtt.MessageHandler
}

type publishCall struct {
	topic    string
	qos      byte
	retained bool
	payload  interface{}
}

func newMockMqttClient(connected bool) *mockMqttClient {
	return &mockMqttClient{connected: connected}
}

func (m *mockMqttClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockMqttClient) IsConnectionOpen() bool {
	return m.IsConnected()
}

func (m *mockMqttClient) Connect() mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return newMockToken(nil)
}

func (m *mockMqttClient) Disconnect(quiesce uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

func (m *mockMqttClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes = append(m.publishes, publishCall{topic: topic, qos: qos, retained: retained, payload: payload})
	return newMockToken(nil)
}

func (m *mockMqttClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribes = append(m.subscribes, subscribeCall{topic: topic, qos: qos, callback: callback})
	return newMockToken(nil)
}

func (m *mockMqttClient) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token {
	for topic, qos := range filters {
		m.Subscribe(topic, qos, callback)
	}
	return newMockToken(nil)
}

func (m *mockMqttClient) Unsubscribe(topics ...string) mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsubscribes = append(m.unsubscribes, topics...)
	return newMockToken(nil)
}

func (m *mockMqttClient) AddRoute(topic string, callback mqtt.MessageHandler) {}

func (m *mockMqttClient) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.ClientOptionsReader{}
}

func (m *mockMqttClient) getSubscribes() []subscribeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]subscribeCall, len(m.subscribes))
	copy(result, m.subscribes)
	return result
}

func (m *mockMqttClient) getUnsubscribes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.unsubscribes))
	copy(result, m.unsubscribes)
	return result
}

// TestSubscribeRaw_Dispatch 验证 SubscribeRaw 在 cli 为 nil 时订阅所有客户端，指定 cli 时只订阅该客户端
func TestSubscribeRaw_Dispatch(t *testing.T) {
	c1 := newMockMqttClient(true)
	c2 := newMockMqttClient(true)
	m := MqttClient{clients: []mqtt.Client{c1, c2}}

	// 不指定 cli，应该两个 client 都订阅
	err := m.SubscribeRaw(nil, "topic/a", 1, func(mqtt.Client, mqtt.Message) {})
	assert.NoError(t, err)
	assert.Len(t, c1.getSubscribes(), 1)
	assert.Len(t, c2.getSubscribes(), 1)

	// 指定 cli，应该只订阅 c1
	err = m.SubscribeRaw(c1, "topic/b", 2, func(mqtt.Client, mqtt.Message) {})
	assert.NoError(t, err)
	assert.Len(t, c1.getSubscribes(), 2)
	assert.Len(t, c2.getSubscribes(), 1)
}

// TestMqttSubscription_Unsubscribe 验证 Unsubscribe 应真正调用底层 client.Unsubscribe
func TestMqttSubscription_Unsubscribe(t *testing.T) {
	c1 := newMockMqttClient(true)
	m := MqttClient{clients: []mqtt.Client{c1}}
	ms := &MqttSubscription{
		topic:  "topic/a",
		qos:    1,
		client: &m,
	}

	err := ms.Unsubscribe()
	assert.NoError(t, err)
	// 当前实现只是打印日志，没有真正取消订阅，这个测试会失败
	unsubs := c1.getUnsubscribes()
	if assert.Len(t, unsubs, 1, "Unsubscribe 应该调用底层 client.Unsubscribe") {
		assert.Equal(t, "topic/a", unsubs[0])
	}
}

// TestSetMqttSetOnConnectHandler_Override 验证多客户端场景下全局回调被覆盖
func TestSetMqttSetOnConnectHandler_Override(t *testing.T) {
	// 模拟两个不同的 MqttClient 实例（不同 ClientID）
	c1 := newMockMqttClient(true)
	c2 := newMockMqttClient(true)
	_ = MqttClient{clients: []mqtt.Client{c1}, cfg: nil}
	_ = MqttClient{clients: []mqtt.Client{c2}, cfg: nil}

	var call1, call2 int
	SetMqttSetOnConnectHandler(func(cli mqtt.Client) {
		// 这个 handler 属于第一个 MqttClient 实例
		call1++
	})
	SetMqttSetOnConnectHandler(func(cli mqtt.Client) {
		// 这个 handler 属于第二个 MqttClient 实例，会覆盖上面的
		call2++
	})

	// 模拟第一个实例的 client 重连触发 OnConnect
	mqttSetOnConnectHandler(c1)

	// 由于全局只有一个回调，最后设置的是第二个实例的回调，所以第一个实例重连时也会调用第二个实例的 handler
	assert.Equal(t, 0, call1, "第一个实例的回调不应被调用，因为它已被第二个实例覆盖")
	assert.Equal(t, 1, call2, "全局只保存了最后设置的回调")

	// 模拟第二个实例的 client 重连
	mqttSetOnConnectHandler(c2)
	assert.Equal(t, 0, call1)
	assert.Equal(t, 2, call2)
}

// TestSubscribeRaw_ErrorPropagation 验证 SubscribeRaw 遇到错误会返回
func TestSubscribeRaw_ErrorPropagation(t *testing.T) {
	errSubscribe := errors.New("subscribe error")
	c1 := &mockMqttClientWithError{err: errSubscribe}
	m := MqttClient{clients: []mqtt.Client{c1}}

	err := m.SubscribeRaw(nil, "topic/a", 1, func(mqtt.Client, mqtt.Message) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscribe error")
}

// mockMqttClientWithError 订阅时返回错误
type mockMqttClientWithError struct {
	mockMqttClient
	err error
}

func (m *mockMqttClientWithError) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	return newMockToken(m.err)
}
