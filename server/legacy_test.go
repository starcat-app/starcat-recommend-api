package server

// 本模块验证 v1 仅为旧客户端的临时契约入口，数据、分页和模型切换必须来自自研。
// 不使用第三方凭据；下一次包含双渠道 v2 的 App 上线后随 v1 入口一起删除。
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

// TestLegacyRouteSharesTrainedModel exercises real publication and model changes on both routes.
func TestLegacyRouteSharesTrainedModel(t *testing.T) {
	service, err := New(Options{APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: filepath.Join(t.TempDir(), "registry"), SkipListenLogEndpoints: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	defer service.Close()
	response := request(t, server.URL, http.MethodGet, "/api/v1/repos/1/recommendations", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusBadGateway)
	var unavailable struct {
		Error struct{ Code string }
	}
	decodeJSON(t, response, &unavailable)
	if unavailable.Error.Code != "UPSTREAM_UNAVAILABLE" {
		t.Fatal("legacy error contract changed")
	}
	for _, version := range []string{trainedVersion, "costar-next-v2"} {
		publishBundle(t, server.URL, version)
		response = request(t, server.URL, http.MethodGet, "/api/v1/repos/1/recommendations?limit=1", trainedPublicKey, nil, "")
		assertStatus(t, response, http.StatusOK)
		if response.Header.Get("Cache-Control") != "private, no-cache" {
			t.Fatal("legacy responses must not serve a separately cached model")
		}
		// 旧 DTO 不认识 model_version / display_score，但必需字段仍能直接解码和展示。
		var legacy envelope[struct {
			Source   string `json:"source"`
			Fallback bool   `json:"fallback"`
			RepoID   int64  `json:"repo_id"`
			Items    []struct {
				RepoID   int64    `json:"repo_id"`
				FullName string   `json:"full_name"`
				Stars    int      `json:"stars"`
				Forks    int      `json:"forks"`
				Archived bool     `json:"archived"`
				Score    float64  `json:"score"`
				Source   string   `json:"source"`
				Reasons  []string `json:"reasons"`
			} `json:"items"`
			ModelVersion string `json:"model_version"`
			HasMore      bool   `json:"has_more"`
			NextOffset   *int   `json:"next_offset"`
		}]
		decodeJSON(t, response, &legacy)
		if legacy.Data.Source != "starcat_trained" || legacy.Data.ModelVersion != version ||
			len(legacy.Data.Items) != 1 || legacy.Data.Items[0].RepoID != 2 ||
			legacy.Data.Items[0].Score != 0.97 || !legacy.Data.HasMore ||
			legacy.Data.NextOffset == nil || *legacy.Data.NextOffset != 1 {
			t.Fatal("legacy route did not consume active trained model")
		}
		response = request(t, server.URL, http.MethodGet, "/api/v1/repos/1/recommendations?limit=1&offset=1", trainedPublicKey, nil, "")
		var next envelope[model.RecommendationResponse]
		decodeJSON(t, response, &next)
		if len(next.Data.Items) != 1 || next.Data.Items[0].RepoID != 3 || next.Data.HasMore {
			t.Fatal("legacy pagination duplicated or lost results")
		}
		response = request(t, server.URL, http.MethodGet, "/api/v2/repos/1/recommendations?limit=1", trainedPublicKey, nil, "")
		var current envelope[model.RecommendationResponse]
		decodeJSON(t, response, &current)
		if current.Data.ModelVersion != version || current.Data.Items[0].Score != 0.9 {
			t.Fatal("legacy display adaptation must not change v2 raw scores")
		}
	}
	for _, test := range []struct {
		path string
		key  string
		code int
	}{
		{"/api/v1/repos/1/recommendations", "", 401},
		{"/api/v1/repos/abc/recommendations", trainedPublicKey, 400},
		{"/api/v1/repos/1/recommendations?limit=31", trainedPublicKey, 400},
		{"/api/v1/repos/1/recommendations?offset=-1", trainedPublicKey, 400},
	} {
		response = request(t, server.URL, http.MethodGet, test.path, test.key, nil, "")
		assertStatus(t, response, test.code)
		response.Body.Close()
	}
	response = request(t, server.URL, http.MethodGet, "/api/v1/repos/999/recommendations", trainedPublicKey, nil, "")
	var empty envelope[model.RecommendationResponse]
	decodeJSON(t, response, &empty)
	if empty.Data.Items == nil || len(empty.Data.Items) != 0 || empty.Data.HasMore {
		t.Fatal("legacy unknown source must retain empty array response")
	}
}

// TestFromEnvNeedsNoThirdPartySecret ensures standalone and aggregate startup no longer depend on SimRepo.
func TestFromEnvNeedsNoThirdPartySecret(t *testing.T) {
	t.Setenv("API_KEYS", trainedPublicKey)
	t.Setenv("SIMREPO_API_KEY", "")
	t.Setenv("SIMREPO_ENDPOINT", "http://127.0.0.1:0/must-not-be-used")
	t.Setenv("MODEL_REGISTRY_DIR", filepath.Join(t.TempDir(), "registry"))
	t.Setenv("METRICS_STORE_FILE", ":memory:")
	t.Setenv("GITHUB_TOKENS", "")
	t.Setenv("MODEL_PUBLISH_KEYS", "")
	service, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/stats", nil)
	req.Header.Set("Authorization", "Bearer "+trainedPublicKey)
	service.Handler().ServeHTTP(recorder, req)
	var stats struct {
		Data struct {
			V1 struct {
				Source            string `json:"source"`
				CompatibilityOnly bool   `json:"compatibility_only"`
				CacheEnabled      bool   `json:"cache_enabled"`
			} `json:"v1"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || stats.Data.V1.Source != "starcat_trained" ||
		!stats.Data.V1.CompatibilityOnly || stats.Data.V1.CacheEnabled {
		t.Fatal("legacy route must report one trained provider without page TTL cache")
	}
}
