package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
	"github.com/starcat-app/starcat-recommend-api/internal/provider"
)

type mockRecommendationProvider struct {
	result provider.Result
	err    error
	query  provider.Query
}

func (m *mockRecommendationProvider) Recommend(ctx context.Context, query provider.Query) (provider.Result, error) {
	m.query = query
	return m.result, m.err
}

func TestRecommendHandlerSuccess(t *testing.T) {
	mock := &mockRecommendationProvider{
		result: provider.Result{
			Response: model.RecommendationResponse{
				Source: "starcat_trained",
				RepoID: 41881900,
				Items: []model.RecommendationItem{{
					RepoID:   1,
					FullName: "owner/repo",
					Score:    0.9,
					Source:   "starcat_trained",
				}},
			},
			CacheStatus: "fresh",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/41881900/recommendations?limit=10&offset=20", nil)
	req.SetPathValue("repo_id", "41881900")
	rr := httptest.NewRecorder()

	NewRecommendHandler(mock).HandleRecommendations(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if mock.query.RepoID != 41881900 || mock.query.Limit != 10 || mock.query.Offset != 20 {
		t.Fatalf("query = %+v", mock.query)
	}

	var env model.Envelope[model.RecommendationResponse]
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data.Items) != 1 || env.Data.Items[0].FullName != "owner/repo" {
		t.Fatalf("unexpected response: %+v", env.Data)
	}
	if env.Meta == nil || env.Meta.CacheStatus != "fresh" {
		t.Fatalf("unexpected meta: %+v", env.Meta)
	}
}

func TestRecommendHandlerBadRepoID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/abc/recommendations", nil)
	req.SetPathValue("repo_id", "abc")
	rr := httptest.NewRecorder()

	NewRecommendHandler(&mockRecommendationProvider{}).HandleRecommendations(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRecommendHandlerUpstreamError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/1/recommendations", nil)
	req.SetPathValue("repo_id", "1")
	rr := httptest.NewRecorder()

	NewRecommendHandler(&mockRecommendationProvider{err: errors.New("boom")}).HandleRecommendations(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadGateway)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("boom")) {
		t.Fatal("legacy response leaked internal error")
	}
}

// TestLegacyScoresAreDisplayValues protects older apps that do not decode display_score.
func TestLegacyScoresAreDisplayValues(t *testing.T) {
	calibrated := 0.97
	mock := &mockRecommendationProvider{result: provider.Result{
		Response: model.RecommendationResponse{Items: []model.RecommendationItem{
			{RepoID: 2, Score: 7.5, DisplayScore: &calibrated},
			{RepoID: 3, Score: 0.63},
		}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/1/recommendations", nil)
	req.SetPathValue("repo_id", "1")
	recorder := httptest.NewRecorder()
	NewRecommendHandler(mock).HandleRecommendations(recorder, req)
	var response model.Envelope[model.RecommendationResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Items[0].Score != 0.97 || response.Data.Items[1].Score != 0.63 {
		t.Fatal("legacy display scores must use calibration or content cosine")
	}
}
