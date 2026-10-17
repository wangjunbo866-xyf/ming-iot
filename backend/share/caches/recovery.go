// 本文件为提供恢复代际的事件总线实现保守的缓存读屏障。
package caches

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	shareerrors "gitee.com/ming-iot/share/errors"
)

// CacheStateProvider 是 Event 的可选扩展；总线可能丢失通知时须改变代际，恢复前返回未就绪。
// CacheState 必须轻量、无回调，且同一实例的 generation 单调递增。
type CacheStateProvider interface {
	CacheState() (generation uint64, ready bool)
}

var (
	// ErrCacheNotReady 表示失效订阅尚未恢复，不能安全使用缓存。
	ErrCacheNotReady = errors.New("cache invalidation subscription is not ready")
	// ErrCacheInvalidated 表示读取期间发生失效或恢复，调用方可重新读取。
	ErrCacheInvalidated = errors.New("cache read invalidated; retry against current state")
)

// expirySeconds 将有效期向上取整，避免不足一秒的配置写入零 TTL。
func (c *Cache[dataT, keyType]) expirySeconds() int {
	seconds := c.expireTime / time.Second
	if c.expireTime%time.Second != 0 {
		seconds++
	}
	return int(seconds)
}

// invalidate 同步删除 L1，并让已在途的可恢复读取失效。
func (c *Cache[dataT, keyType]) invalidate(key string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.revision++
	c.cache.Delete(key)
}

// syncStateLocked 在使用 L1 前观察总线代际，恢复时全量抛弃可能遗漏失效的项。
func (c *Cache[dataT, keyType]) syncStateLocked() bool {
	generation, ready := c.state.CacheState()
	if generation != c.generation || ready != c.ready {
		c.cache.Clear()
		c.revision++
		c.generation, c.ready = generation, ready
	}
	return ready
}

// checkReadLocked 检查读取是否仍属于当前就绪代际和失效版本。
func (c *Cache[dataT, keyType]) checkReadLocked(revision uint64) error {
	if !c.syncStateLocked() {
		return ErrCacheNotReady
	}
	if revision != c.revision {
		return ErrCacheInvalidated
	}
	return nil
}

// getRecoverable 将读取与回填绑定到同一代际；共享读取的每个等待者也必须通过返回屏障。
func (c *Cache[dataT, keyType]) getRecoverable(ctx context.Context, key keyType) (*dataT, error) {
	keyStr := GenKeyStr(key)
	c.stateMu.Lock()
	if !c.syncStateLocked() {
		c.stateMu.Unlock()
		return nil, ErrCacheNotReady
	}
	revision := c.revision
	data, hit := c.cache.Get(keyStr)
	c.stateMu.Unlock()
	if hit && NeedCache {
		if data == nil {
			return nil, shareerrors.NotFind
		}
		return data, nil
	}

	// 失效后发起的请求不能加入旧 singleflight；业务键长度无需解析。
	flightKey := fmt.Sprintf("%d:%s", revision, keyStr)
	ret, readErr := c.sf.Do(flightKey, func() (any, error) {
		c.stateMu.Lock()
		if err := c.checkReadLocked(revision); err != nil {
			c.stateMu.Unlock()
			return nil, err
		}
		cached, found := c.cache.Get(keyStr)
		c.stateMu.Unlock()
		if found && NeedCache {
			return cached, nil
		}
		data, err := c.readAuthority(ctx, key)
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
		if stateErr := c.checkReadLocked(revision); stateErr != nil {
			return nil, stateErr
		}
		if err == nil || shareerrors.Cmp(err, shareerrors.NotFind) {
			c.cache.Set(keyStr, data)
		}
		return data, err
	})
	c.stateMu.Lock()
	stateErr := c.checkReadLocked(revision)
	c.stateMu.Unlock()
	if stateErr != nil {
		return nil, stateErr
	}
	if readErr != nil {
		return nil, readErr
	}
	if ret == nil {
		return nil, shareerrors.NotFind
	}
	data = ret.(*dataT)
	if data == nil {
		return nil, shareerrors.NotFind
	}
	return data, nil
}

// readAuthority 读取 Redis 或权威数据源；此路径不异步回填 Redis，避免旧读取复活已删除数据。
func (c *Cache[dataT, keyType]) readAuthority(ctx context.Context, key keyType) (*dataT, error) {
	val, redisErr := store.GetCtx(ctx, c.GenCacheKey(key))
	var data *dataT
	if redisErr == nil && val != "" && NeedCache {
		if err := json.Unmarshal([]byte(val), &data); err != nil {
			return nil, err
		}
	} else if c.getData != nil {
		var err error
		data, err = c.getData(ctx, key)
		if err != nil {
			return nil, err
		}
	} else if redisErr != nil {
		return nil, redisErr
	}
	if data == nil {
		return nil, shareerrors.NotFind
	}
	if c.fmt != nil {
		data = c.fmt(ctx, key, data)
	}
	if data == nil {
		return nil, shareerrors.NotFind
	}
	return data, nil
}
