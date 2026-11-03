package server

// 通过真实 Bundle 安装和 HTTP 契约覆盖内容兜底，尤其防止 handler 提前 304 跳过内容变化。
import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	kitgithub "github.com/starcat-app/starcat-api-kit/github"
	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

type publicMetadataFixture struct{ description string }

func (f publicMetadataFixture) GetRepoByID(context.Context, int64) (*kitgithub.Repo, error) {
	return &kitgithub.Repo{ID: 99, FullName: "external/project", Description: &f.description}, nil
}

func TestContentBundleHTTPPaginationAndRevalidation(t *testing.T) {
	directory := buildBundleDirectory(t, trainedVersion)
	database, err := sql.Open("sqlite", filepath.Join(directory, "recommendations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`CREATE TABLE content_vocabulary(term TEXT PRIMARY KEY,idf REAL);
		INSERT INTO content_vocabulary VALUES('database',1),('piano',1);
		CREATE TABLE content_postings(term TEXT,repo_id INTEGER,weight REAL,PRIMARY KEY(term,repo_id));
		INSERT INTO content_postings VALUES('database',1,1),('database',2,0.8),('database',3,0.6),('piano',4,1);`)
	database.Close()
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Options{APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: filepath.Join(t.TempDir(), "registry"), SkipListenLogEndpoints: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	defer service.Close()
	service.trainedProvider.SetMetadataReader(publicMetadataFixture{"database"})
	response := request(t, server.URL, http.MethodPost, "/internal/v1/model-bundles/"+trainedVersion+"?activate=true",
		trainedAdminKey, zipBundle(t, directory, checksumFile(t, filepath.Join(directory, "manifest.json"))), "application/zip")
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	path := "/api/v2/repos/99/recommendations?limit=2"
	response = request(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	etag := response.Header.Get("ETag")
	var first envelope[model.RecommendationResponse]
	decodeJSON(t, response, &first)
	if !first.Data.Fallback || !strings.HasPrefix(first.Data.ModelVersion, trainedVersion+"-c") ||
		len(first.Data.Items) != 2 || !first.Data.HasMore || first.Data.NextOffset == nil {
		t.Fatal("invalid cold-start HTTP response")
	}
	response = request(t, server.URL, http.MethodGet, "/api/v1/repos/99/recommendations?limit=2", trainedPublicKey, nil, "")
	assertStatus(t, response, http.StatusOK)
	var legacy envelope[model.RecommendationResponse]
	decodeJSON(t, response, &legacy)
	if !legacy.Data.Fallback || legacy.Data.ModelVersion != first.Data.ModelVersion ||
		len(legacy.Data.Items) != 2 || legacy.Data.Items[0].RepoID != first.Data.Items[0].RepoID ||
		legacy.Data.Items[0].Score != first.Data.Items[0].Score {
		t.Fatal("legacy route must share content fallback without fabricating scores")
	}
	next := fmt.Sprintf("/api/v2/repos/99/recommendations?limit=2&offset=%d", *first.Data.NextOffset)
	response = request(t, server.URL, http.MethodGet, next, trainedPublicKey, nil, "")
	var second envelope[model.RecommendationResponse]
	decodeJSON(t, response, &second)
	if len(second.Data.Items) != 1 || second.Data.Items[0].RepoID != 3 || second.Data.ModelVersion != first.Data.ModelVersion {
		t.Fatal("cold-start pages mixed identities or duplicated targets")
	}
	response = requestWithHeaders(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "", map[string]string{"If-None-Match": etag})
	assertStatus(t, response, http.StatusNotModified)
	response.Body.Close()
	service.trainedProvider.SetMetadataReader(publicMetadataFixture{"piano"})
	response = requestWithHeaders(t, server.URL, http.MethodGet, path, trainedPublicKey, nil, "", map[string]string{"If-None-Match": etag})
	assertStatus(t, response, http.StatusOK)
	var changed envelope[model.RecommendationResponse]
	decodeJSON(t, response, &changed)
	if changed.Data.ModelVersion == first.Data.ModelVersion || changed.Data.Items[0].RepoID != 4 {
		t.Fatal("source metadata change incorrectly revalidated old content")
	}
	// 既有共现 source 不进入内容路径；越界 offset 不得跳到内容列表。
	response = request(t, server.URL, http.MethodGet, "/api/v2/repos/1/recommendations?offset=50", trainedPublicKey, nil, "")
	var beyond envelope[model.RecommendationResponse]
	decodeJSON(t, response, &beyond)
	if beyond.Data.Fallback || len(beyond.Data.Items) != 0 || beyond.Data.ModelVersion != trainedVersion {
		t.Fatal("precomputed pagination fell through to unrelated content")
	}
}
