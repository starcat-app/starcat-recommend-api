// 本文件提供真实 Trainer 候选产物的可选本地契约验收。
// 只使用 httptest 与临时 Registry，不读取 .env、不请求第三方、不触碰既有 active。
package server

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

// TestCandidateBundleLocalAcceptance 通过真实 ZIP 导入及 HTTP 查询验证跨语言边界。
// 未显式传入候选路径时跳过；这里的加载只发生在 t.TempDir 内的隔离服务。
func TestCandidateBundleLocalAcceptance(t *testing.T) {
	directory := os.Getenv("STARCAT_TEST_CANDIDATE_BUNDLE")
	if directory == "" {
		t.Skip("set STARCAT_TEST_CANDIDATE_BUNDLE to verify a local candidate")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal("cannot read candidate manifest")
	}
	var manifest struct {
		ModelVersion string `json:"model_version"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || manifest.ModelVersion == "" {
		t.Fatal("invalid candidate manifest")
	}
	service, err := New(Options{
		APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: filepath.Join(t.TempDir(), "registry"),
		MaxBundleBytes:   512 << 20, SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() {
		server.Close()
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	// 只读原文件，不能复用会重写 checksums.json 的合成测试 ZIP 辅助函数。
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, name := range []string{"manifest.json", "checksums.json", "recommendations.sqlite"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal("cannot read candidate bundle member")
		}
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	response := request(t, server.URL, http.MethodPost,
		"/internal/v1/model-bundles/"+manifest.ModelVersion+"?activate=true",
		trainedAdminKey, buffer.Bytes(), "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(directory, "recommendations.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal("cannot open candidate SQLite")
	}
	defer database.Close()
	// 覆盖一组有出边的仓库和一组低 Star 长尾仓库，固定排序便于复跑比较。
	rows, err := database.Query(`SELECT repo_id FROM (
		SELECT source_repo_id AS repo_id FROM recommendations GROUP BY source_repo_id
		HAVING count(*)>=6 ORDER BY source_repo_id LIMIT 20)
		UNION SELECT repo_id FROM (
		SELECT p.repo_id FROM repositories p WHERE EXISTS (
		SELECT 1 FROM recommendations r WHERE r.source_repo_id=p.repo_id)
		ORDER BY p.stars,p.repo_id LIMIT 20) ORDER BY repo_id`)
	if err != nil {
		t.Fatal(err)
	}
	var seeds []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(seeds) < 2 {
		t.Fatal("candidate has insufficient covered repositories")
	}
	started := time.Now()
	var durations []time.Duration
	var excluded int64
	for _, seed := range seeds {
		queryStarted := time.Now()
		path := fmt.Sprintf("/api/v2/repos/%d/recommendations?limit=3&offset=0", seed)
		response := request(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "")
		assertStatus(t, response, http.StatusOK)
		etag := response.Header.Get("ETag")
		if etag == "" {
			t.Fatal("missing candidate ETag")
		}
		var first envelope[model.RecommendationResponse]
		decodeJSON(t, response, &first)
		durations = append(durations, time.Since(queryStarted))
		if first.Data.ModelVersion != manifest.ModelVersion || first.Data.Source != "starcat_trained" || len(first.Data.Items) == 0 {
			t.Fatal("candidate version/source/items mismatch")
		}
		seen := map[int64]bool{seed: true}
		for _, item := range first.Data.Items {
			if seen[item.RepoID] {
				t.Fatal("self or duplicate result")
			}
			seen[item.RepoID] = true
		}
		excluded = first.Data.Items[0].RepoID
		response = requestWithHeaders(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "", map[string]string{"If-None-Match": etag})
		assertStatus(t, response, http.StatusNotModified)
		response.Body.Close()
		if first.Data.HasMore {
			if first.Data.NextOffset == nil || *first.Data.NextOffset != 3 {
				t.Fatal("invalid candidate pagination")
			}
			response = request(t, server.URL, http.MethodGet, fmt.Sprintf("/api/v2/repos/%d/recommendations?limit=3&offset=3", seed), trainedPublicKey, nil, "")
			assertStatus(t, response, http.StatusOK)
			var second envelope[model.RecommendationResponse]
			decodeJSON(t, response, &second)
			if second.Data.ModelVersion != manifest.ModelVersion {
				t.Fatal("model changed between pages")
			}
			for _, item := range second.Data.Items {
				if seen[item.RepoID] {
					t.Fatal("duplicate across pages")
				}
				seen[item.RepoID] = true
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"positive_repo_ids": seeds[:2], "exclude_repo_ids": []int64{excluded}, "limit": 20})
	response = request(t, server.URL, http.MethodPost, "/api/v2/recommendations/query", trainedPublicKey, body, "application/json")
	assertStatus(t, response, http.StatusOK)
	var multi envelope[model.MultiRecommendationResponse]
	decodeJSON(t, response, &multi)
	if multi.Data.ModelVersion != manifest.ModelVersion || multi.Data.Source != "starcat_trained" {
		t.Fatal("multi-seed candidate source/version mismatch")
	}
	for _, item := range multi.Data.Items {
		if item.RepoID == excluded || item.RepoID == seeds[0] || item.RepoID == seeds[1] {
			t.Fatal("exclusion failed")
		}
	}
	response = request(t, server.URL, http.MethodGet, "/api/v2/repos/9223372036854775807/recommendations", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var empty envelope[model.RecommendationResponse]
	decodeJSON(t, response, &empty)
	if len(empty.Data.Items) != 0 || empty.Data.HasMore {
		t.Fatal("unknown seed must have empty results")
	}
	response = request(t, server.URL, http.MethodGet, "/api/v2/repos/1/recommendations", "", nil, "")
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()
	// 扩容 Bundle 还要经过 Go 的真实内容查询，不能只验证 Python 生成了索引表。
	var supportsContent bool
	if err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='content_vocabulary')").Scan(&supportsContent); err != nil {
		t.Fatal(err)
	}
	if supportsContent {
		rows, err := database.Query(`SELECT repo_id FROM repositories p WHERE NOT EXISTS (
			SELECT 1 FROM recommendations r WHERE r.source_repo_id=p.repo_id)
			ORDER BY repo_id DESC LIMIT 20`)
		if err != nil {
			t.Fatal(err)
		}
		var coldSeeds []int64
		for rows.Next() {
			var seed int64
			if err := rows.Scan(&seed); err != nil {
				t.Fatal(err)
			}
			coldSeeds = append(coldSeeds, seed)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		for _, seed := range coldSeeds {
			path := fmt.Sprintf("/api/v2/repos/%d/recommendations?limit=3", seed)
			response = request(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "")
			assertStatus(t, response, http.StatusOK)
			var cold envelope[model.RecommendationResponse]
			decodeJSON(t, response, &cold)
			if !cold.Data.Fallback || len(cold.Data.Items) == 0 || !strings.HasPrefix(cold.Data.ModelVersion, manifest.ModelVersion+"-c") {
				t.Fatal("real content-index fallback failed")
			}
			for _, item := range cold.Data.Items {
				if item.RepoID == seed || item.Score <= 0 || item.Score > 1.000001 || item.DisplayScore != nil {
					t.Fatal("invalid real content neighbor or calibration")
				}
			}
		}
		t.Logf("real content-index acceptance: cold_sources=%d", len(coldSeeds))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("candidate local acceptance: seeds=%d elapsed=%s single_page_p50_ms=%.3f single_page_p95_ms=%.3f",
		len(seeds), time.Since(started), float64(durations[len(durations)/2])/float64(time.Millisecond),
		float64(durations[(len(durations)*95-1)/100])/float64(time.Millisecond))
}
