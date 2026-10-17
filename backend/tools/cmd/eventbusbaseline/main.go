// 本程序仅在专属隔离环境中验证真实缓存与 Kafka 消费边界。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gitee.com/ming-iot/share/caches"
	contract "gitee.com/ming-iot/share/contracts/eventbus"
	"gitee.com/ming-iot/share/eventBus"
	foundation "gitee.com/ming-iot/share/foundation/kafka"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// value 是隔离权限缓存的测试值。
type value struct {
	Version string `json:"version"`
}

// request 描述单条测试写入。
type request struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// clientConfig 将探针固定到 Compose 内部 Kafka，不接受外部连接参数。
func clientConfig(id string) foundation.ClientConfig {
	return foundation.ClientConfig{Profile: foundation.ProfileLocal, Brokers: []string{"kafka:29092"}, ClientID: id}
}

// newBus 以稳定实例名显式接入新路径，旧 EventConf 不受影响。
func newBus(id string) (*eventBus.KafkaBus, error) {
	return eventBus.NewKafkaBus(eventBus.KafkaBusConfig{Client: clientConfig("probe-" + id), Role: "p1-cache", Instance: id, PollTimeout: 200 * time.Millisecond, RequestTimeout: 2 * time.Second, MaxPollRecords: 32})
}

// worker 在独立进程中创建真实 NewCache；两个实例不会共享单例。
func worker(id string) error {
	caches.InitStore(cache.ClusterConf{{RedisConf: redis.RedisConf{Host: "redis:6379", Type: "node"}, Weight: 100}})
	bus, err := newBus(id)
	if err != nil {
		return err
	}
	defer bus.Close()
	c, err := caches.NewCache[value, string](caches.CacheConfig[value, string]{KeyType: "p1-permissions", FastEvent: bus, ExpireTime: 10 * time.Minute})
	if err != nil {
		return err
	}
	var mu sync.Mutex
	notifications := map[string]int{}
	c.AddNotifySlot(func(_ context.Context, key []byte) { mu.Lock(); notifications[string(key)]++; mu.Unlock() })
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	go func() {
		if err := bus.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, err)
			cancel()
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		g, ready := bus.CacheState()
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, map[string]any{"instance": id, "pid": os.Getpid(), "ready": ready, "generation": g, "notifications": notifications})
	})
	mux.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
			writeError(w, err)
			return
		}
		err := c.SetData(r.Context(), req.Key, &value{Version: req.Value})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		v, err := c.GetData(r.Context(), r.URL.Query().Get("key"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "value": v})
	})
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		_ = server.Shutdown(shutdown)
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// writeJSON 将结果交给宿主验证器，不输出连接凭据。
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 保留失败状态，验证器不可把错误当作完成。
func writeError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusServiceUnavailable)
	writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
}

// call 从同一容器回环地址读取 worker，即使网络断开仍可观察其拒绝旧缓存。
func call(path, body string) error {
	method := "GET"
	if body != "" {
		method = "POST"
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err == nil {
		fmt.Printf("EVENTBUS_RESULT %s\n", bytes.TrimSpace(bodyBytes))
	}
	return err
}

// completion 用真实 broker 验证失败不提交、后续同键不执行、重启重投与提交后不再投递。
func completion(ctx context.Context) error {
	id := fmt.Sprintf("completion-%d", time.Now().UnixNano())
	subject := "server.cache.sync." + id
	first, err := newBus(id)
	if err != nil {
		return err
	}
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls int
	err = first.Subscribe(subject, func(ctx context.Context, _ time.Time, body []byte) error {
		calls++
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return errors.New("injected handler failure")
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- first.Run(ctx) }()
	defer first.Close()
	if err = waitReady(ctx, first); err != nil {
		return err
	}
	if err = first.Publish(ctx, subject, "same-object"); err != nil {
		return err
	}
	if err = first.Publish(ctx, subject, "same-object"); err != nil {
		return err
	}
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	close(release)
	select {
	case err = <-done:
		if err == nil {
			return errors.New("handler failure silently succeeded")
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if calls != 1 {
		return fmt.Errorf("later same-key side effect executed: %d", calls)
	}
	_ = first.Close()
	second, err := newBus(id)
	if err != nil {
		return err
	}
	defer second.Close()
	processed := make(chan string, 4)
	if err = second.Subscribe(subject, func(_ context.Context, _ time.Time, body []byte) error { processed <- string(body); return nil }); err != nil {
		return err
	}
	secondCtx, cancel := context.WithCancel(ctx)
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Run(secondCtx) }()
	for i := 0; i < 2; i++ {
		select {
		case got := <-processed:
			if got != "same-object" {
				return fmt.Errorf("wrong replay %s", got)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// 等待 handler 返回后的 broker 提交，以 broker 组位点作为重启前事实。
	group, err := contract.BroadcastGroup("p1-cache", id)
	if err != nil {
		return err
	}
	if err = waitCommitted(ctx, group); err != nil {
		return err
	}
	cancel()
	_ = second.Close()
	<-secondDone
	third, err := newBus(id)
	if err != nil {
		return err
	}
	defer third.Close()
	if err = third.Subscribe(subject, func(_ context.Context, _ time.Time, _ []byte) error { processed <- "unexpected"; return nil }); err != nil {
		return err
	}
	thirdCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = third.Run(thirdCtx) }()
	if err = waitReady(ctx, third); err != nil {
		return err
	}
	select {
	case <-processed:
		return errors.New("committed callback replayed after restart")
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println(`{"case":"completion-restart","result":"PASS","replayed":2,"first_callbacks":1}`)
	return nil
}

// waitReady 等待真实分配和连接检查，超时不是通过。
func waitReady(ctx context.Context, b *eventBus.KafkaBus) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, ready := b.CacheState()
		if ready {
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// main 只公开隔离 worker 和验证操作。
func main() {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if len(os.Args) < 2 {
		err = errors.New("mode required")
	} else {
		switch os.Args[1] {
		case "worker":
			if len(os.Args) != 3 {
				err = errors.New("instance required")
			} else {
				err = worker(os.Args[2])
			}
		case "call":
			if len(os.Args) < 3 {
				err = errors.New("path required")
			} else {
				body := ""
				if len(os.Args) > 3 {
					body = os.Args[3]
				}
				err = call(os.Args[2], body)
			}
		case "completion":
			err = completion(ctx)
		case "competition":
			err = competition(ctx)
		default:
			err = errors.New("unknown mode")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
