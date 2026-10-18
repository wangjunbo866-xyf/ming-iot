// 本文件验证真实 Cache 的恢复、并发失效和故障返回边界；存储与事件仅替换外部依赖。
package caches

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	shareerrors "gitee.com/ming-iot/share/errors"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/syncx"
)

// recoveryStore 仅实现测试涉及的 Redis 操作，其余操作不可被静默调用。
type recoveryStore struct {
	kv.Store
	mu               sync.Mutex
	values           map[string]string
	getErr, writeErr error
	gets, fills, ttl int
	beforeReturn     func()
}

// GetCtx 提供可控的在途读取快照以复现读取与失效竞争。
func (s *recoveryStore) GetCtx(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	s.gets++
	value, err, hook := s.values[key], s.getErr, s.beforeReturn
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return value, err
}

// SetexCtx 记录写入结果和 TTL。
func (s *recoveryStore) SetexCtx(_ context.Context, key, value string, ttl int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttl = ttl
	if s.writeErr != nil {
		return s.writeErr
	}
	s.values[key] = value
	return nil
}

// DelCtx 模拟删除失败或成功。
func (s *recoveryStore) DelCtx(_ context.Context, keys ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	for _, key := range keys {
		delete(s.values, key)
	}
	return len(keys), nil
}

// SetnxExCtx 记录意外的异步回填。
func (s *recoveryStore) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fills++
	if _, exists := s.values[key]; exists {
		return false, nil
	}
	s.values[key] = value
	return true, nil
}

// recoveryEvent 模拟订阅代际及通知回调。
type recoveryEvent struct {
	mu         sync.Mutex
	generation uint64
	ready      bool
	handler    FastFunc
	publishErr error
	publishes  int
}

// CacheState 返回原子状态快照。
func (e *recoveryEvent) CacheState() (uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.generation, e.ready
}

// Subscribe 安装真实 Cache 的通知处理器。
func (e *recoveryEvent) Subscribe(_ string, f FastFunc) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = f
	return nil
}

// Publish 保留发送错误，不自动回环，允许验证本地删除。
func (e *recoveryEvent) Publish(context.Context, string, any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.publishes++
	return e.publishErr
}

// transition 模拟连接状态变化，包括缓存未观察到的短暂离线。
func (e *recoveryEvent) transition(ready bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.generation++
	e.ready = ready
}

// notify 模拟消费失效事件。
func (e *recoveryEvent) notify(key string) error {
	e.mu.Lock()
	handler := e.handler
	e.mu.Unlock()
	return handler(context.Background(), time.Now(), []byte(key))
}

// newRecoveryTestCache 使用 NewCache 生产入口并隔离测试存储。
func newRecoveryTestCache(t *testing.T, event Event, get func(context.Context, string) (*string, error), expiry time.Duration) (*Cache[string, string], *recoveryStore) {
	t.Helper()
	previous := store
	s := &recoveryStore{values: map[string]string{}}
	store = s
	c, err := NewCache(CacheConfig[string, string]{KeyType: t.Name(), FastEvent: event, GetData: get, ExpireTime: expiry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store = previous
		cacheMutex.Lock()
		delete(cacheMap, t.Name())
		cacheMutex.Unlock()
		c.cache.Close()
	})
	return c, s
}

// TestRecoveryRejectsStaleL1 验证离线拒读与恢复后 Redis 权威值读取。
func TestRecoveryRejectsStaleL1(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	c, s := newRecoveryTestCache(t, e, nil, 0)
	ctx := context.Background()
	s.values[c.GenCacheKey("k")] = `"allow"`
	if value, err := c.GetData(ctx, "k"); err != nil || *value != "allow" {
		t.Fatalf("warm: %v %v", value, err)
	}
	e.transition(false)
	s.values[c.GenCacheKey("k")] = `"deny"`
	if _, err := c.GetData(ctx, "k"); !errors.Is(err, ErrCacheNotReady) {
		t.Fatalf("offline: %v", err)
	}
	e.transition(true)
	if value, err := c.GetData(ctx, "k"); err != nil || *value != "deny" {
		t.Fatalf("recovery: %v %v", value, err)
	}
	// 没有离线期间的 GetData，也必须因代际变化抛弃 L1。
	e.transition(false)
	e.transition(true)
	s.values[c.GenCacheKey("k")] = `"changed-again"`
	if value, err := c.GetData(ctx, "k"); err != nil || *value != "changed-again" {
		t.Fatalf("missed outage: %v %v", value, err)
	}
}

// TestRecoveryInflightReadFence 验证通知和重连都隔离旧 singleflight，旧读取不能回填。
func TestRecoveryInflightReadFence(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		t.Run(fmt.Sprint(reconnect), func(t *testing.T) {
			e := &recoveryEvent{generation: 1, ready: true}
			c, s := newRecoveryTestCache(t, e, nil, time.Minute)
			s.values[c.GenCacheKey("k")] = `"old"`
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			s.beforeReturn = func() { once.Do(func() { close(entered); <-release }) }
			result := make(chan error, 16)
			for i := 0; i < 16; i++ {
				go func() {
					value, err := c.GetData(context.Background(), "k")
					if value != nil && *value == "old" {
						result <- fmt.Errorf("returned stale value")
						return
					}
					result <- err
				}()
			}
			<-entered
			s.mu.Lock()
			s.values[c.GenCacheKey("k")] = `"new"`
			s.beforeReturn = nil
			s.mu.Unlock()
			if reconnect {
				e.transition(false)
				e.transition(true)
			} else if err := e.notify("k"); err != nil {
				t.Fatal(err)
			}
			// 必须在旧读取解除阻塞前完成，证明新请求不加入旧 singleflight。
			fresh := make(chan error, 1)
			go func() {
				value, err := c.GetData(context.Background(), "k")
				if err == nil && (value == nil || *value != "new") {
					err = fmt.Errorf("fresh read: %v", value)
				}
				fresh <- err
			}()
			select {
			case err := <-fresh:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(2 * time.Second):
				t.Error("fresh read joined stale flight")
			}
			close(release)
			for i := 0; i < 16; i++ {
				if err := <-result; err != nil && !errors.Is(err, ErrCacheInvalidated) {
					t.Error(err)
				}
			}
			if value, err := c.GetData(context.Background(), "k"); err != nil || *value != "new" {
				t.Fatalf("refill: %v %v", value, err)
			}
		})
	}
}

// TestRecoveryDBReadDoesNotResurrectRedis 验证删除期间的旧 DB 结果不会异步复活 Redis。
func TestRecoveryDBReadDoesNotResurrectRedis(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	entered, release := make(chan struct{}), make(chan struct{})
	c, s := newRecoveryTestCache(t, e, func(context.Context, string) (*string, error) {
		close(entered)
		<-release
		value := "old"
		return &value, nil
	}, time.Minute)
	result := make(chan error, 1)
	go func() { _, err := c.GetData(context.Background(), "k"); result <- err }()
	<-entered
	if err := c.SetData(context.Background(), "k", nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, ErrCacheInvalidated) {
		t.Fatalf("old DB read: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fills != 0 || len(s.values) != 0 {
		t.Fatalf("stale DB backfill: fills=%d values=%v", s.fills, s.values)
	}
}

// TestSetDataReportsPartialFailure 验证 Redis 与广播的非原子失败均可见。
func TestSetDataReportsPartialFailure(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		t.Run(fmt.Sprint(deleting), func(t *testing.T) {
			publishErr, redisErr := errors.New("bus down"), errors.New("Redis down")
			e := &recoveryEvent{generation: 1, ready: true, publishErr: publishErr}
			c, s := newRecoveryTestCache(t, e, nil, 1500*time.Millisecond)
			value := "value"
			data := &value
			if deleting {
				data = nil
			}
			if err := c.SetData(context.Background(), "k", data); !errors.Is(err, publishErr) {
				t.Fatalf("publish failure: %v", err)
			}
			if !deleting && (s.values[c.GenCacheKey("k")] != `"value"` || s.ttl != 2) {
				t.Fatalf("successful Redis write lost: %v ttl=%d", s.values, s.ttl)
			}
			s.writeErr = redisErr
			if err := c.SetData(context.Background(), "k", data); !errors.Is(err, redisErr) || !errors.Is(err, publishErr) {
				t.Fatalf("both failures: %v", err)
			}
		})
	}
}

// TestCacheMissingAndDefaultTTL 验证 nil 结果不 panic，且默认 L1 不会立即过期。
func TestCacheMissingAndDefaultTTL(t *testing.T) {
	for _, recoverable := range []bool{false, true} {
		t.Run(fmt.Sprint(recoverable), func(t *testing.T) {
			var event Event
			if recoverable {
				event = &recoveryEvent{generation: 1, ready: true}
			}
			c, s := newRecoveryTestCache(t, event, nil, 0)
			if _, err := c.GetData(context.Background(), "missing"); !shareerrors.Cmp(err, shareerrors.NotFind) {
				t.Fatalf("missing: %v", err)
			}
			s.values[c.GenCacheKey("k")] = `"value"`
			if _, err := c.GetData(context.Background(), "k"); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			gets := s.gets
			s.mu.Unlock()
			entry, found := c.cache.Extension().GetEntry("k")
			if !found || entry.TTL() < time.Minute {
				t.Fatalf("default L1 TTL too short: found=%v ttl=%v", found, entry.TTL())
			}
			if _, err := c.GetData(context.Background(), "k"); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.gets != gets {
				t.Fatal("default L1 expired immediately")
			}
		})
	}
}

// TestRecoveryNotifyRegistrationRace 验证注册与通知并发不破坏插槽，并允许回调再注册。
func TestRecoveryNotifyRegistrationRace(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	c, _ := newRecoveryTestCache(t, e, nil, time.Minute)
	var calls atomic.Int64
	c.AddNotifySlot(func(context.Context, []byte) { calls.Add(1); c.AddNotifySlot(func(context.Context, []byte) {}) })
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			c.AddNotifySlot(func(context.Context, []byte) {})
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			if err := e.notify("k"); err != nil {
				t.Error(err)
			}
		}
	}()
	workers.Wait()
	if calls.Load() != 100 {
		t.Fatalf("notification count: %d", calls.Load())
	}
}

// TestRecoveryDBFallbackAndNilFormatter 验证 Redis 故障降级读取及格式化返回 nil 的安全性。
func TestRecoveryDBFallbackAndNilFormatter(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	c, s := newRecoveryTestCache(t, e, func(context.Context, string) (*string, error) { value := "db"; return &value, nil }, time.Minute)
	s.getErr = errors.New("Redis down")
	if value, err := c.GetData(context.Background(), "k"); err != nil || *value != "db" {
		t.Fatalf("DB fallback: %v %v", value, err)
	}
	c.fmt = func(context.Context, string, *string) *string { return nil }
	if _, err := c.GetData(context.Background(), "other"); !shareerrors.Cmp(err, shareerrors.NotFind) {
		t.Fatalf("nil formatter: %v", err)
	}
}

// returningFlight 在真实 singleflight 完成后暂停返回，模拟共享等待者被调度延迟。
type returningFlight struct {
	syncx.SingleFlight
	after func()
}

// Do 保留真实 singleflight 行为，只暴露读取已完成而调用者尚未返回的竞争窗口。
func (f returningFlight) Do(key string, fn func() (any, error)) (any, error) {
	value, err := f.SingleFlight.Do(key, fn)
	f.after()
	return value, err
}

// TestRecoverySharedResultReturnFence 验证失效发生于回填后时，等待者也不返回旧权限值。
func TestRecoverySharedResultReturnFence(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	c, s := newRecoveryTestCache(t, e, nil, time.Minute)
	s.values[c.GenCacheKey("k")] = `"old"`
	c.sf = returningFlight{SingleFlight: c.sf, after: func() { e.transition(false) }}
	if value, err := c.GetData(context.Background(), "k"); value != nil || !errors.Is(err, ErrCacheNotReady) {
		t.Fatalf("returned pre-disconnect result: value=%v err=%v", value, err)
	}
}

// TestRecoverySingleflightSharesAuthority 验证并发读取共享权威查询。
func TestRecoverySingleflightSharesAuthority(t *testing.T) {
	e := &recoveryEvent{generation: 1, ready: true}
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	c, _ := newRecoveryTestCache(t, e, func(context.Context, string) (*string, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		value := "db"
		return &value, nil
	}, time.Minute)
	var workers sync.WaitGroup
	workers.Add(32)
	for i := 0; i < 32; i++ {
		go func() {
			defer workers.Done()
			if value, err := c.GetData(context.Background(), "k"); err != nil || value == nil || *value != "db" {
				t.Errorf("shared read: %v %v", value, err)
			}
		}()
	}
	<-entered
	close(release)
	workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("authority queries: %d", calls.Load())
	}
}
