package handler

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
	"github.com/starcat-app/starcat-recommend-api/internal/provider"
)

const (
	defaultLimit = 10
	maxLimit     = 30
)

// RecommendationProvider 是 handler 依赖的最小 provider 接口。
type RecommendationProvider interface {
	Recommend(ctx context.Context, query provider.Query) (provider.Result, error)
}

// RecommendHandler 仅为尚未升级的客户端保留 v1 参数和响应契约。
// 数据与 v2 共用自研 Provider；下一版本 App 上线后删除，不能重新加入第三方来源。
type RecommendHandler struct {
	provider RecommendationProvider
}

func NewRecommendHandler(provider RecommendationProvider) *RecommendHandler {
	return &RecommendHandler{provider: provider}
}

func (h *RecommendHandler) HandleRecommendations(w http.ResponseWriter, r *http.Request) {
	repoID, ok := parsePositiveInt64(r.PathValue("repo_id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "repo_id must be positive", nil)
		return
	}

	limit, ok := parseBoundedInt(r.URL.Query().Get("limit"), defaultLimit, 1, maxLimit)
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be between 1 and 30", nil)
		return
	}

	offset, ok := parseBoundedInt(r.URL.Query().Get("offset"), 0, 0, 10000)
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "offset must be non-negative", nil)
		return
	}

	result, err := h.provider.Recommend(r.Context(), provider.Query{
		RepoID: repoID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		// 保留旧错误契约，但不把存储路径或公开元数据上游错误正文暴露给旧客户端。
		writeError(w, http.StatusBadGateway, "UPSTREAM_UNAVAILABLE", "recommendations are unavailable", nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	// 老版本仅用 score 展示匹配程度。优先映射全局校准分数，不能将原始共现权重
	// 当成 0～1 的展示值；排序仍由自研 Provider 决定，不因适配而重新排序。
	for index := range result.Response.Items {
		item := &result.Response.Items[index]
		score := item.Score
		if item.DisplayScore != nil {
			score = *item.DisplayScore
		}
		item.Score = math.Max(0, math.Min(1, score))
	}

	meta := &model.Meta{
		PageSize:    limit,
		Total:       len(result.Response.Items),
		Source:      result.Response.Source,
		CacheStatus: result.CacheStatus,
	}
	writeJSONWithMeta(w, result.Response, meta)
}

func parsePositiveInt64(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func parseBoundedInt(raw string, fallback, min, max int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}
