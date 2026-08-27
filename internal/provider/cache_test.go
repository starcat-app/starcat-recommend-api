package provider

import (
	"context"
	"testing"
	"time"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

type countingProvider struct {
	calls int
}

func (p *countingProvider) Recommend(ctx context.Context, query Query) (Result, error) {
	p.calls++
	return Result{
		Response: model.RecommendationResponse{
			Source: "simrepo",
			RepoID: query.RepoID,
			Items:  []model.RecommendationItem{{RepoID: 1, FullName: "owner/repo"}},
		},
		CacheStatus: "fresh",
	}, nil
}

func TestCachedProviderCachesSuccess(t *testing.T) {
	base := &countingProvider{}
	cached := NewCachedProvider(base, time.Minute, time.Minute, time.Minute)
	query := Query{RepoID: 1, Limit: 10, Offset: 0}

	first, err := cached.Recommend(t.Context(), query)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := cached.Recommend(t.Context(), query)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if base.calls != 1 {
		t.Fatalf("base calls = %d, want 1", base.calls)
	}
	if first.CacheStatus != "fresh" || second.CacheStatus != "hit" {
		t.Fatalf("cache statuses = %q/%q", first.CacheStatus, second.CacheStatus)
	}
	stats := cached.Stats()
	if stats.Misses != 1 || stats.Hits != 1 || stats.Entries != 1 || stats.UpstreamErrors != 0 {
		t.Fatalf("unexpected cache stats: %#v", stats)
	}
}

func TestCachedProviderRemovesExpiredEntryOnRead(t *testing.T) {
	cached := NewCachedProvider(&countingProvider{}, time.Minute, time.Minute, time.Minute)
	cached.set("expired", cacheEntry{expiresAt: time.Now().Add(-time.Second)})

	if _, ok := cached.get("expired", time.Now()); ok {
		t.Fatal("expired entry should miss")
	}
	if len(cached.items) != 0 {
		t.Fatalf("cached items = %d, want 0 after expired read", len(cached.items))
	}
}

func TestCachedProviderEvictsEarliestExpiryAtCapacity(t *testing.T) {
	cached := NewCachedProvider(&countingProvider{}, time.Minute, time.Minute, time.Minute)
	cached.maxEntries = 2
	now := time.Now()
	cached.set("first", cacheEntry{expiresAt: now.Add(time.Minute)})
	cached.set("second", cacheEntry{expiresAt: now.Add(2 * time.Minute)})
	cached.set("third", cacheEntry{expiresAt: now.Add(3 * time.Minute)})

	if len(cached.items) != 2 {
		t.Fatalf("cached items = %d, want bounded size 2", len(cached.items))
	}
	if _, ok := cached.items["first"]; ok {
		t.Fatal("earliest-expiring entry should be evicted")
	}
}
