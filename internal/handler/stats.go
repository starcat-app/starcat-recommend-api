// Package handler exposes read-only Recommend operations data.
package handler

import (
	"net/http"

	"github.com/starcat-app/starcat-recommend-api/internal/provider"
	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

// RecommendOperationalStats separates process-local v1 cache counters from persistent v2 model metadata.
type RecommendOperationalStats struct {
	V1 struct {
		ProviderConfigured bool                `json:"provider_configured"`
		Cache              provider.CacheStats `json:"cache"`
		CountersReset      string              `json:"counters_reset"`
	} `json:"v1"`
	V2 serving.OperationalStats `json:"v2"`
}

// HandleOperationalStats returns v1 cache and v2 active bundle state.
func HandleOperationalStats(cache *provider.CachedProvider, registry *serving.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		servingStats, err := registry.Stats()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to inspect active model", nil)
			return
		}
		var result RecommendOperationalStats
		result.V1.ProviderConfigured = true
		result.V1.Cache = cache.Stats()
		result.V1.CountersReset = "service_restart"
		result.V2 = servingStats
		writeJSON(w, result)
	}
}
