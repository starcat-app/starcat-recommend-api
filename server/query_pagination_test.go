// 本文件通过隔离 Registry 和真实 HTTP 路由验证多种子分页
// 固定相同分数的候选，覆盖跨越前 100 条、查询身份变化及模型切换，避免只测试令牌编码
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

// TestMultiSeedCursorPagination 使用同一模型翻过 100 条，并拒绝跨查询或跨模型复用位置
func TestMultiSeedCursorPagination(t *testing.T) {
	service, err := New(Options{
		APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: filepath.Join(t.TempDir(), "registry"),
		MaxBundleBytes:   10 << 20, SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() { server.Close(); _ = service.Close() })
	publishQueryPagingBundle(t, server.URL, "paging-v1")

	query := map[string]any{"positive_repo_ids": []int64{1, 4}, "limit": 100}
	first := queryPagingPage(t, server.URL, query)
	if len(first.Items) != 100 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("invalid first page: count=%d has_more=%v", len(first.Items), first.HasMore)
	}
	query["cursor"] = first.NextCursor
	query["positive_repo_ids"] = []int64{4, 1}
	second := queryPagingPage(t, server.URL, query)
	if len(second.Items) != 22 || second.HasMore || second.NextCursor != "" {
		t.Fatalf("invalid final page: count=%d has_more=%v", len(second.Items), second.HasMore)
	}
	seen := make(map[int64]bool)
	for _, item := range append(first.Items, second.Items...) {
		if seen[item.RepoID] || item.RepoID == 1 || item.RepoID == 4 {
			t.Fatalf("duplicate or seed leaked across pages: %d", item.RepoID)
		}
		seen[item.RepoID] = true
	}
	if first.Items[0].RepoID != 2 || second.Items[0].RepoID != 108 {
		t.Fatal("paging must preserve score/rank/id ordering")
	}

	for _, change := range []map[string]any{
		{"positive_repo_ids": []int64{1}},
		{"negative_repo_ids": []int64{3}},
		{"exclude_repo_ids": []int64{3}},
		{"limit": 20},
		{"cursor": "not-a-cursor"},
	} {
		body := map[string]any{"positive_repo_ids": []int64{1, 4}, "limit": 100, "cursor": first.NextCursor}
		for key, value := range change {
			body[key] = value
		}
		encoded, _ := json.Marshal(body)
		response := request(t, server.URL, http.MethodPost, "/api/v2/recommendations/query", trainedPublicKey, encoded, "application/json")
		assertStatus(t, response, http.StatusBadRequest)
		response.Body.Close()
	}

	publishQueryPagingBundle(t, server.URL, "paging-v2")
	encoded, _ := json.Marshal(query)
	response := request(t, server.URL, http.MethodPost, "/api/v2/recommendations/query", trainedPublicKey, encoded, "application/json")
	assertStatus(t, response, http.StatusConflict)
	var expired struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, response, &expired)
	if expired.Error.Code != "CURSOR_EXPIRED" {
		t.Fatalf("expected explicit cursor expiration, got %q", expired.Error.Code)
	}
	delete(query, "cursor")
	if page := queryPagingPage(t, server.URL, query); page.ModelVersion != "paging-v2" {
		t.Fatal("restart must use the new model")
	}
}

// queryPagingPage 保留鉴权、envelope 与真实路由，防止测试绕过 HTTP 参数校验
func queryPagingPage(t *testing.T, baseURL string, query map[string]any) model.MultiRecommendationResponse {
	t.Helper()
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, baseURL, http.MethodPost, "/api/v2/recommendations/query", trainedPublicKey, encoded, "application/json")
	assertStatus(t, response, http.StatusOK)
	var result envelope[model.MultiRecommendationResponse]
	decodeJSON(t, response, &result)
	return result.Data
}

// publishQueryPagingBundle 在临时合成 Bundle 中加入 120 个同分候选，保证需要第二页
func publishQueryPagingBundle(t *testing.T, baseURL, version string) {
	t.Helper()
	directory := buildBundleDirectory(t, version)
	database, err := sql.Open("sqlite", filepath.Join(directory, "recommendations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	for id := 10; id < 130; id++ {
		if _, err := database.Exec(`INSERT INTO repositories VALUES (?, ?, '', '[]', 'Go', '', 10, 0, NULL)`, id, fmt.Sprintf("owner/project%d", id)); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO recommendations VALUES (1, ?, 0.1, 0.2, 3, 'costar', '{}')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	bundle := zipBundle(t, directory, checksumFile(t, filepath.Join(directory, "manifest.json")))
	response := request(t, baseURL, http.MethodPost, "/internal/v1/model-bundles/"+version+"?activate=true", trainedAdminKey, bundle, "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
}
