// Package handler exposes read-only Recommend operations data.
package handler

import (
	"net/http"

	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

// RecommendOperationalStats reports the temporary v1 alias and the shared active model.
type RecommendOperationalStats struct {
	V1 struct {
		ProviderConfigured bool           `json:"provider_configured"`
		Source             string         `json:"source"`
		CompatibilityOnly  bool           `json:"compatibility_only"`
		CacheEnabled       bool           `json:"cache_enabled"`
		Cache              map[string]int `json:"cache"`
	} `json:"v1"`
	V2 serving.OperationalStats `json:"v2"`
}

// HandleOperationalStats keeps existing console cache fields at zero, explicitly disabled.
// Both routes read the same immutable model; there is no separate v1 page cache or provider.
func HandleOperationalStats(registry *serving.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		servingStats, err := registry.Stats()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to inspect active model", nil)
			return
		}
		var result RecommendOperationalStats
		result.V1.ProviderConfigured = true
		result.V1.Source = "starcat_trained"
		result.V1.CompatibilityOnly = true
		result.V1.Cache = map[string]int{"entries": 0, "maximum_entries": 0, "hits": 0, "misses": 0, "upstream_errors": 0}
		result.V2 = servingStats
		writeJSON(w, result)
	}
}
