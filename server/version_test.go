// Package server 验证对外版本契约，包含空 Registry、旧包、新包及实际激活切换。
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestModelVersionReflectsActivation 防止把服务构建号、未激活候选和活动模型混为一谈。
func TestModelVersionReflectsActivation(t *testing.T) {
	service, err := New(Options{
		APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: t.TempDir(), SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	host := httptest.NewServer(service.Handler())
	defer host.Close()
	response := request(t, host.URL, http.MethodGet, "/api/v2/version", "", nil, "")
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()

	read := func() map[string]any {
		response := request(t, host.URL, http.MethodGet, "/api/v2/version", trainedPublicKey, nil, "")
		assertStatus(t, response, http.StatusOK)
		if response.Header.Get("Cache-Control") != "private, no-store" {
			t.Fatal("version response must not hide activation behind a cache")
		}
		var result envelope[map[string]any]
		decodeJSON(t, response, &result)
		return result.Data
	}
	if data := read(); data["model"] != nil || data["service"] == nil {
		t.Fatalf("empty registry: %#v", data)
	}
	publishBundle(t, host.URL, "legacy")
	legacy := read()["model"].(map[string]any)
	if legacy["training"] != nil || legacy["metrics_scope"] != nil {
		t.Fatalf("legacy metadata must stay unknown: %#v", legacy)
	}

	directory := buildBundleDirectory(t, "modern")
	manifest := map[string]any{
		"schema_version": 1, "model_version": "modern", "selected_model": "hybrid_25",
		"created_at": "2026-11-17T02:00:00Z", "metrics_scope": "pre_refit",
		"training": map[string]any{
			"started_at": "2026-11-17T01:00:00Z", "completed_at": "2026-11-17T01:30:00Z",
			"interactions": 42, "private_path": "/private/never-expose",
		},
		"evaluation": map[string]any{"split": "test", "k": 20},
		"metrics":    map[string]any{"recall@20": 0.04, "subject_id": "private", "coverage": nil},
	}
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	archive := zipBundle(t, directory, checksumFile(t, path))
	response = request(t, host.URL, http.MethodPost, "/internal/v1/model-bundles/modern?activate=false", trainedAdminKey, archive, "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	if read()["model"].(map[string]any)["model_version"] != "legacy" {
		t.Fatal("inactive upload changed the version endpoint")
	}
	response = request(t, host.URL, http.MethodPost, "/internal/v1/model-bundles/modern?activate=true", trainedAdminKey, archive, "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	modern := read()["model"].(map[string]any)
	training := modern["training"].(map[string]any)
	metrics := modern["metrics"].(map[string]any)
	if modern["model_version"] != "modern" || modern["metrics_scope"] != "pre_refit" ||
		training["completed_at"] != "2026-11-17T01:30:00Z" || training["private_path"] != nil ||
		modern["repositories"] != float64(4) || modern["recommendation_edges"] != float64(3) ||
		metrics["recall@20"] != 0.04 || len(metrics) != 1 {
		t.Fatalf("active metadata mismatch: %#v", modern)
	}
}
