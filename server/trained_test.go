package server

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
	_ "modernc.org/sqlite"
)

const (
	trainedPublicKey = "public-test-key"
	trainedAdminKey  = "publish-test-key"
	trainedVersion   = "costar-test-v1"
)

func TestTrainedBundlePublishAndV2Queries(t *testing.T) {
	service, err := New(Options{
		APIKeys:                []string{trainedPublicKey},
		SimRepoAPIKey:          "unused-in-v2-test",
		ModelRegistryDir:       filepath.Join(t.TempDir(), "registry"),
		ModelPublishKeys:       []string{trainedAdminKey},
		MaxBundleBytes:         10 << 20,
		SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() {
		server.Close()
		if err := service.Close(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})

	response := request(t, server.URL, http.MethodGet, "/api/v2/repos/1/recommendations", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusServiceUnavailable)
	response.Body.Close()

	bundle := buildBundleZip(t, trainedVersion)
	response = request(t, server.URL, http.MethodPost, "/internal/v1/model-bundles/"+trainedVersion+"?activate=true", "wrong-key", bundle, "application/zip")
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()
	response = request(t, server.URL, http.MethodPost, "/internal/v1/model-bundles/"+trainedVersion+"?activate=true", trainedAdminKey, bundle, "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()

	response = request(t, server.URL, http.MethodGet, "/internal/v1/model-bundles/active", trainedAdminKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var active envelope[struct {
		ModelVersion string `json:"model_version"`
	}]
	decodeJSON(t, response, &active)
	if active.Data.ModelVersion != trainedVersion {
		t.Fatalf("active model = %q, want %q", active.Data.ModelVersion, trainedVersion)
	}

	response = request(t, server.URL, http.MethodGet, "/internal/stats", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var stats envelope[struct {
		V2 struct {
			Active              bool  `json:"active"`
			Repositories        int64 `json:"repositories"`
			RecommendationEdges int64 `json:"recommendation_edges"`
		} `json:"v2"`
	}]
	decodeJSON(t, response, &stats)
	if !stats.Data.V2.Active || stats.Data.V2.Repositories != 4 || stats.Data.V2.RecommendationEdges != 3 {
		t.Fatalf("unexpected trained serving stats: %+v", stats.Data.V2)
	}

	response = request(t, server.URL, http.MethodGet, "/api/v2/repos/1/recommendations?limit=1&offset=0", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var page envelope[model.RecommendationResponse]
	decodeJSON(t, response, &page)
	if page.Data.ModelVersion != trainedVersion || page.Data.Source != "starcat_trained" || len(page.Data.Items) != 1 {
		t.Fatalf("unexpected page: %+v", page.Data)
	}
	if page.Data.Items[0].RepoID != 2 || page.Data.Items[0].FullName != "owner/two" {
		t.Fatalf("unexpected first recommendation: %+v", page.Data.Items[0])
	}
	if !page.Data.HasMore || page.Data.NextOffset == nil || *page.Data.NextOffset != 1 {
		t.Fatalf("unexpected paging: %+v", page.Data)
	}

	queryBody, err := json.Marshal(map[string]any{
		"positive_repo_ids": []int64{1},
		"negative_repo_ids": []int64{4},
		"exclude_repo_ids":  []int64{2},
		"limit":             10,
	})
	if err != nil {
		t.Fatal(err)
	}
	response = request(t, server.URL, http.MethodPost, "/api/v2/recommendations/query", trainedPublicKey, queryBody, "application/json")
	assertStatus(t, response, http.StatusOK)
	var multi envelope[model.MultiRecommendationResponse]
	decodeJSON(t, response, &multi)
	if len(multi.Data.Items) != 1 || multi.Data.Items[0].RepoID != 3 {
		t.Fatalf("unexpected multi recommendations: %+v", multi.Data.Items)
	}
}

func TestBundleRejectsChecksumMismatchWithoutChangingActive(t *testing.T) {
	registryDirectory := filepath.Join(t.TempDir(), "registry")
	service, err := New(Options{
		APIKeys:                []string{trainedPublicKey},
		SimRepoAPIKey:          "unused-in-v2-test",
		ModelRegistryDir:       registryDirectory,
		ModelPublishKeys:       []string{trainedAdminKey},
		MaxBundleBytes:         10 << 20,
		SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	defer service.Close()

	valid := buildBundleZip(t, trainedVersion)
	response := request(t, server.URL, http.MethodPost, "/internal/v1/model-bundles/"+trainedVersion+"?activate=true", trainedAdminKey, valid, "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()

	invalid := buildBundleZipWithManifestChecksum(t, "costar-bad-v2", "sha256:"+stringsOf('0', 64))
	response = request(t, server.URL, http.MethodPost, "/internal/v1/model-bundles/costar-bad-v2?activate=true", trainedAdminKey, invalid, "application/zip")
	assertStatus(t, response, http.StatusUnprocessableEntity)
	response.Body.Close()

	response = request(t, server.URL, http.MethodGet, "/internal/v1/model-bundles/active", trainedAdminKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var active envelope[struct {
		ModelVersion string `json:"model_version"`
	}]
	decodeJSON(t, response, &active)
	if active.Data.ModelVersion != trainedVersion {
		t.Fatalf("invalid publish changed active model to %q", active.Data.ModelVersion)
	}
}

type envelope[T any] struct {
	SchemaVersion int `json:"schema_version"`
	Data          T   `json:"data"`
}

func request(t *testing.T, baseURL string, method string, path string, key string, body []byte, contentType string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode == expected {
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	t.Fatalf("status = %d, want %d, body = %s", response.StatusCode, expected, body)
}

func decodeJSON(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func buildBundleZip(t *testing.T, version string) []byte {
	t.Helper()
	directory := buildBundleDirectory(t, version)
	manifestChecksum := checksumFile(t, filepath.Join(directory, "manifest.json"))
	return zipBundle(t, directory, manifestChecksum)
}

func buildBundleZipWithManifestChecksum(t *testing.T, version string, manifestChecksum string) []byte {
	t.Helper()
	directory := buildBundleDirectory(t, version)
	return zipBundle(t, directory, manifestChecksum)
}

func buildBundleDirectory(t *testing.T, version string) string {
	t.Helper()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "recommendations.sqlite")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE repositories (
    repo_id INTEGER PRIMARY KEY, full_name TEXT NOT NULL, description TEXT,
    topics_json TEXT NOT NULL, primary_language TEXT, license_key TEXT,
    stars INTEGER NOT NULL, forks INTEGER NOT NULL, pushed_at TEXT
);
CREATE TABLE recommendations (
    source_repo_id INTEGER NOT NULL, target_repo_id INTEGER NOT NULL,
    score REAL NOT NULL, rank INTEGER NOT NULL, model TEXT NOT NULL,
    signals TEXT NOT NULL, PRIMARY KEY(source_repo_id, target_repo_id)
);
INSERT INTO repositories VALUES
    (1, 'owner/one', '', '[]', 'Go', '', 10, 1, NULL),
    (2, 'owner/two', 'two', '[]', 'Go', '', 20, 2, NULL),
    (3, 'owner/three', 'three', '[]', 'Swift', '', 30, 3, NULL),
    (4, 'owner/four', 'four', '[]', 'Go', '', 40, 4, NULL);
INSERT INTO recommendations VALUES
    (1, 2, 0.9, 1, 'costar', '{"kind":"time_decayed_costar","support":2}'),
    (1, 3, 0.5, 2, 'costar', '{"kind":"time_decayed_costar","support":1}'),
    (4, 2, 0.8, 1, 'costar', '{"kind":"time_decayed_costar","support":2}');`)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"model_version":  version,
		"selected_model": "costar",
		"created_at":     "2026-08-24T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}

func zipBundle(t *testing.T, directory string, manifestChecksum string) []byte {
	t.Helper()
	checksums, err := json.Marshal(map[string]string{
		"manifest.json":          manifestChecksum,
		"recommendations.sqlite": checksumFile(t, filepath.Join(directory, "recommendations.sqlite")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.json"), checksums, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{"recommendations.sqlite", "manifest.json", "checksums.json"} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
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
	return output.Bytes()
}

func checksumFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stringsOf(value byte, count int) string {
	return string(bytes.Repeat([]byte{value}, count))
}
