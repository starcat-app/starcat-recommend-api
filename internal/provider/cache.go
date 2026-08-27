package provider

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CachedProvider 为上游 recommend 结果加进程内 TTL cache。
//
// 首版服务没有持久化层, 进程内缓存足够降低 SimRepo 重复调用压力。未来如果需要多实例
// 共享缓存, 在这里替换为 Redis/SQLite, handler 和客户端契约不需要变化。
type CachedProvider struct {
	base       Provider
	successTTL time.Duration
	emptyTTL   time.Duration
	errorTTL   time.Duration

	mu         sync.Mutex
	items      map[string]cacheEntry
	maxEntries int
	hits       atomic.Int64
	misses     atomic.Int64
	errors     atomic.Int64
}

// CacheStats is a process-lifetime snapshot; counters reset on service restart.
type CacheStats struct {
	Entries        int   `json:"entries"`
	MaximumEntries int   `json:"maximum_entries"`
	Hits           int64 `json:"hits"`
	Misses         int64 `json:"misses"`
	UpstreamErrors int64 `json:"upstream_errors"`
}

// defaultCacheMaxEntries 防止 repoID × limit × offset 的组合键让常驻进程内存无界增长。
// 10k 条足以覆盖客户端常用查询，同时无需引入额外 LRU 依赖。
const defaultCacheMaxEntries = 10_000

type cacheEntry struct {
	result    Result
	err       error
	expiresAt time.Time
}

func NewCachedProvider(base Provider, successTTL, emptyTTL, errorTTL time.Duration) *CachedProvider {
	return &CachedProvider{
		base:       base,
		successTTL: successTTL,
		emptyTTL:   emptyTTL,
		errorTTL:   errorTTL,
		items:      map[string]cacheEntry{},
		maxEntries: defaultCacheMaxEntries,
	}
}

func (p *CachedProvider) Recommend(ctx context.Context, query Query) (Result, error) {
	key := cacheKey(query)
	now := time.Now()
	if entry, ok := p.get(key, now); ok {
		p.hits.Add(1)
		if entry.err != nil {
			return Result{}, entry.err
		}
		entry.result.CacheStatus = "hit"
		return entry.result, nil
	}
	p.misses.Add(1)

	result, err := p.base.Recommend(ctx, query)
	ttl := p.successTTL
	if err != nil {
		p.errors.Add(1)
		ttl = p.errorTTL
	} else if len(result.Response.Items) == 0 {
		ttl = p.emptyTTL
	}
	p.set(key, cacheEntry{
		result:    result,
		err:       err,
		expiresAt: now.Add(ttl),
	})
	return result, err
}

// Stats returns a race-safe snapshot without exposing cache keys or repository ids.
func (p *CachedProvider) Stats() CacheStats {
	p.mu.Lock()
	entries := len(p.items)
	maximum := p.maxEntries
	p.mu.Unlock()
	return CacheStats{Entries: entries, MaximumEntries: maximum, Hits: p.hits.Load(),
		Misses: p.misses.Load(), UpstreamErrors: p.errors.Load()}
}

func (p *CachedProvider) get(key string, now time.Time) (cacheEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.items[key]
	if !ok {
		return cacheEntry{}, false
	}
	if !now.Before(entry.expiresAt) {
		// 读取时顺手删除过期项，避免低频 key 永久占用容量。
		delete(p.items, key)
		return cacheEntry{}, false
	}
	return entry, true
}

func (p *CachedProvider) set(key string, entry cacheEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.items[key]; !exists && len(p.items) >= p.maxEntries {
		p.makeRoomLocked(time.Now())
	}
	p.items[key] = entry
}

// makeRoomLocked 先清理全部过期项；容量仍满时淘汰最早到期项。
// 这里按 expiresAt 淘汰而非实现完整 LRU，是因为不同结果已有明确 TTL，最早到期项的
// 剩余复用价值最低，且该策略不需要在每次命中时维护额外链表。
func (p *CachedProvider) makeRoomLocked(now time.Time) {
	for key, entry := range p.items {
		if !now.Before(entry.expiresAt) {
			delete(p.items, key)
		}
	}
	for len(p.items) >= p.maxEntries {
		var earliestKey string
		var earliestExpiry time.Time
		for key, entry := range p.items {
			if earliestKey == "" || entry.expiresAt.Before(earliestExpiry) {
				earliestKey = key
				earliestExpiry = entry.expiresAt
			}
		}
		if earliestKey == "" {
			return
		}
		delete(p.items, earliestKey)
	}
}

func cacheKey(query Query) string {
	return fmt.Sprintf("%d:%d:%d", query.RepoID, query.Limit, query.Offset)
}
