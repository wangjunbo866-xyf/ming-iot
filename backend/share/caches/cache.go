// 本文件提供进程内、Redis 与权威数据源之间的缓存入口。
package caches

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/eventBus"
	"gitee.com/ming-iot/share/utils"
	"github.com/maypok86/otter"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/syncx"
)

var NeedCache = true

type FastFunc = func(ctx context.Context, t time.Time, body []byte) error
type Event interface {
	Subscribe(topic string, f FastFunc) error
	Publish(ctx context.Context, topic string, arg any) error
}

// Cache 保存同一业务键类型的缓存与失效通知状态。
type Cache[dataT any, keyType comparable] struct {
	keyType    string
	cache      otter.Cache[string, *dataT]
	fastEvent  Event
	getData    func(ctx context.Context, key keyType) (*dataT, error)
	notifySlot []func(ctx context.Context, key []byte)
	notifyMu   sync.RWMutex
	// stateMu 将代际检查、失效通知与 L1 回填串行化。
	stateMu    sync.Mutex
	state      CacheStateProvider
	generation uint64
	revision   uint64
	ready      bool
	fmt        func(ctx context.Context, key keyType, data *dataT) *dataT
	expireTime time.Duration
	sf         syncx.SingleFlight
}

type CacheConfig[dataT any, keyType comparable] struct {
	KeyType    string
	FastEvent  Event
	Fmt        func(ctx context.Context, key keyType, data *dataT) *dataT
	GetData    func(ctx context.Context, key keyType) (*dataT, error)
	ExpireTime time.Duration
}

var (
	cacheMap   = map[string]any{}
	cacheMutex sync.Mutex
)

// NewCache 按业务键类型创建进程内单例，并订阅失效通知。
func NewCache[dataT any, keyType comparable](cfg CacheConfig[dataT, keyType]) (*Cache[dataT, keyType], error) {
	cacheMutex.Lock() //单例模式
	defer cacheMutex.Unlock()
	if v, ok := cacheMap[cfg.KeyType]; ok {
		if existing, ok := v.(*Cache[dataT, keyType]); ok {
			return existing, nil
		}
		return nil, fmt.Errorf("cache key type %q already registered with different Go types", cfg.KeyType)
	}
	if cfg.ExpireTime < 0 {
		return nil, fmt.Errorf("cache expiry must not be negative")
	}
	if cfg.ExpireTime == 0 {
		cfg.ExpireTime = time.Minute*10 + time.Second*time.Duration(rand.Int63n(60))
	}
	cache, err := otter.MustBuilder[string, *dataT](10_000).
		CollectStats().
		Cost(func(key string, value *dataT) uint32 {
			return 1
		}).
		WithTTL(cfg.ExpireTime/3 + time.Nanosecond).
		Build()
	if err != nil {
		return nil, err
	}
	ret := Cache[dataT, keyType]{
		sf:         syncx.NewSingleFlight(),
		keyType:    cfg.KeyType,
		cache:      cache,
		fastEvent:  cfg.FastEvent,
		getData:    cfg.GetData,
		expireTime: cfg.ExpireTime,
		fmt:        cfg.Fmt,
	}
	ret.state, _ = cfg.FastEvent.(CacheStateProvider)
	if ret.fastEvent != nil {
		err = ret.fastEvent.Subscribe(ret.genTopic(), func(ctx context.Context, t time.Time, body []byte) error {
			ret.invalidate(string(body))
			ret.notifyMu.RLock()
			slots := append([]func(context.Context, []byte){}, ret.notifySlot...)
			ret.notifyMu.RUnlock()
			for _, f := range slots {
				f(ctx, body)
			}
			return nil
		})
		if err != nil {
			ret.cache.Close()
			return nil, err
		}
	}

	cacheMap[cfg.KeyType] = &ret
	return &ret, nil
}

func GenKeyStr(in any) string {
	return utils.Fmt2(in)
}

// AddNotifySlot 安全注册失效通知回调。
func (c *Cache[dataT, keyType]) AddNotifySlot(f func(ctx context.Context, key []byte)) {
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()
	c.notifySlot = append(c.notifySlot, f)
}

func (c *Cache[dataT, keyType]) genTopic() string {
	return fmt.Sprintf(eventBus.ServerCacheSync, c.keyType)
}

func (c *Cache[dataT, keyType]) GenCacheKey(key any) string {
	return fmt.Sprintf("cache:%s:%v", c.keyType, key)
}

// DeleteByFunc 删除满足条件的 L1 项，并阻止可恢复路径的并发读取回填旧值。
func (c *Cache[dataT, keyType]) DeleteByFunc(f func(cacheKey string) bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.revision++
	c.cache.DeleteByFunc(func(key string, value *dataT) bool {
		return f(key)
	})
}

// SetData 更新 Redis 并广播失效；传 nil 删除 Redis 数据。
// Redis 与事件总线不构成原子事务：返回错误时 Redis 仍可能已更新，调用方需重试或协调修复。
func (c *Cache[dataT, keyType]) SetData(ctx context.Context, key keyType, data *dataT) error {
	cacheKey := c.GenCacheKey(key)
	keyStr := GenKeyStr(key)
	c.invalidate(keyStr)
	var writeErr error
	if data != nil {
		dataStr, err := json.Marshal(data)
		if err != nil {
			return err
		}
		writeErr = store.SetexCtx(ctx, cacheKey, string(dataStr), c.expirySeconds())
	} else {
		_, writeErr = store.DelCtx(ctx, cacheKey)
	}
	c.invalidate(keyStr)
	var publishErr error
	if c.fastEvent != nil {
		publishErr = c.fastEvent.Publish(ctx, c.genTopic(), keyStr)
	}
	if writeErr != nil {
		writeErr = fmt.Errorf("cache Redis update failed: %w", writeErr)
	}
	if publishErr != nil {
		publishErr = fmt.Errorf("cache invalidation publish failed (Redis may already be updated): %w", publishErr)
	}
	return stderrors.Join(writeErr, publishErr)
}

// GetData 读取缓存；可恢复事件总线未就绪时拒绝返回可能过期的数据。
func (c *Cache[dataT, keyType]) GetData(ctx context.Context, key keyType) (*dataT, error) {
	ctx = ctxs.WithRoot(ctx)
	if c.state != nil {
		return c.getRecoverable(ctx, key)
	}
	cacheKey := c.GenCacheKey(key)
	keyStr := GenKeyStr(key)
	temp, ok := c.cache.Get(keyStr)
	if ok && NeedCache {
		if temp == nil {
			return nil, errors.NotFind
		}
		return temp, nil
	}
	//并发获取的情况下避免击穿
	ret, err := c.sf.Do(keyStr, func() (any, error) {
		{ //内存中没有就从redis上获取
			val, err := store.GetCtx(ctx, cacheKey)
			if err != nil {
				logx.WithContext(ctx).Errorf("redis get fail(%v), key:%s, fallback to db", err, cacheKey)
				if c.getData == nil {
					return nil, err
				}
				// redis故障时降级读数据库
				data, dbErr := c.getData(ctxs.WithRoot(ctx), key)
				if dbErr == nil {
					var newData = data
					if c.fmt != nil {
						newData = c.fmt(ctx, key, data)
					}
					c.cache.Set(keyStr, newData)
					return newData, nil
				}
				if errors.Cmp(dbErr, errors.NotFind) {
					c.cache.Set(keyStr, nil)
					return nil, dbErr
				}
				logx.WithContext(ctx).Errorf("db fallback also fail: %v", dbErr)
				return nil, err
			}
			if len(val) > 0 && NeedCache {
				var ret dataT
				err = json.Unmarshal([]byte(val), &ret)
				if err != nil {
					return nil, err
				}
				data := &ret
				if c.fmt != nil {
					data = c.fmt(ctx, key, data)
				}
				c.cache.Set(keyStr, data)
				return data, nil
			}
		}
		if c.getData == nil { //如果没有设置第三级缓存则直接设置该参数为空并返回
			c.cache.Set(keyStr, nil)
			return nil, nil
		}
		//redis上没有就读数据库
		data, err := c.getData(ctxs.WithRoot(ctx), key)
		if err != nil && !errors.Cmp(err, errors.NotFind) { //如果是其他错误直接返回
			return nil, err
		}
		//读到了之后设置缓存
		var newData = data
		if c.fmt != nil {
			newData = c.fmt(ctx, key, data)
		}
		c.cache.Set(keyStr, newData)
		if data == nil {
			return data, err
		}
		ctxs.GoNewCtx(ctx, func(ctx context.Context) { //异步设置缓存
			str, err := json.Marshal(data)
			if err != nil {
				logx.WithContext(ctx).Error(err)
				return
			}
			_, err = store.SetnxExCtx(ctx, cacheKey, string(str), c.expirySeconds())
			if err != nil {
				logx.WithContext(ctx).Error(err)
				return
			}
		})
		return newData, nil
	})
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return nil, errors.NotFind
	}
	data := ret.(*dataT)
	if data == nil {
		return nil, errors.NotFind
	}
	return data, nil
}
