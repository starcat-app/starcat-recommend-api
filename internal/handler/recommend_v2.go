package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
	"github.com/starcat-app/starcat-recommend-api/internal/provider"
	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

// TrainedRecommendHandler 为 /api/v2 提供自研 Bundle 查询，不改变 v1 SimRepo handler。
type TrainedRecommendHandler struct {
	provider *provider.TrainedProvider
}

// NewTrainedRecommendHandler 创建 v2 推荐 handler。
func NewTrainedRecommendHandler(trainedProvider *provider.TrainedProvider) *TrainedRecommendHandler {
	return &TrainedRecommendHandler{provider: trainedProvider}
}

// HandleRecommendations 处理单仓自研推荐。
func (h *TrainedRecommendHandler) HandleRecommendations(w http.ResponseWriter, r *http.Request) {
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
	activeVersion, err := h.provider.ActiveModelVersion()
	if err != nil {
		writeServingError(w, err)
		return
	}
	entityTag := recommendationETag(activeVersion, repoID, limit, offset)
	w.Header().Set("ETag", entityTag)
	// no-cache 允许客户端保存响应，但每次复用前必须携带 ETag 向服务端确认模型版本。
	// private 防止带鉴权的个性化调用被共享代理缓存。
	w.Header().Set("Cache-Control", "private, no-cache")
	if entityTagMatches(r.Header.Get("If-None-Match"), entityTag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	result, err := h.provider.Recommend(r.Context(), provider.Query{RepoID: repoID, Limit: limit, Offset: offset})
	if err != nil {
		writeServingError(w, err)
		return
	}
	// 模型可能在版本检查和 SQLite 查询之间完成原子切换。响应 ETag 必须以本次实际
	// 查询固定的 Bundle 为准，不能继续沿用检查时看到的旧版本。
	w.Header().Set("ETag", recommendationETag(result.Response.ModelVersion, repoID, limit, offset))
	meta := &model.Meta{PageSize: limit, Total: len(result.Response.Items), Source: result.Response.Source, CacheStatus: result.CacheStatus}
	writeJSONWithMeta(w, result.Response, meta)
}

// recommendationETag 标识一个不可变 ServingBundle 中某一页推荐响应。
// model version 由 Registry 限制为安全字符，因此可直接组成 HTTP opaque-tag。
func recommendationETag(modelVersion string, repoID int64, limit int, offset int) string {
	return fmt.Sprintf(`"recommendation:%s:%d:%d:%d"`, modelVersion, repoID, limit, offset)
}

// entityTagMatches 支持 RFC 9110 定义的 If-None-Match 通配符与逗号分隔标签。
// Starcat 当前只发送一个标签，兼容列表可以避免代理合并请求头后错误漏判。
func entityTagMatches(header string, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == current {
			return true
		}
	}
	return false
}

type recommendationQueryRequest struct {
	PositiveRepoIDs []int64 `json:"positive_repo_ids"`
	NegativeRepoIDs []int64 `json:"negative_repo_ids"`
	ExcludeRepoIDs  []int64 `json:"exclude_repo_ids"`
	Limit           int     `json:"limit"`
}

// HandleQuery 处理多 positive/negative seed 的匿名推荐请求。
func (h *TrainedRecommendHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var request recommendationQueryRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	request.PositiveRepoIDs = uniqueIDs(request.PositiveRepoIDs)
	request.NegativeRepoIDs = uniqueIDs(request.NegativeRepoIDs)
	request.ExcludeRepoIDs = uniqueIDs(request.ExcludeRepoIDs)
	if len(request.PositiveRepoIDs) == 0 || len(request.PositiveRepoIDs) > 20 || len(request.NegativeRepoIDs) > 20 || len(request.ExcludeRepoIDs) > 500 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "seed or exclude limits exceeded", nil)
		return
	}
	if hasInvalidID(request.PositiveRepoIDs) || hasInvalidID(request.NegativeRepoIDs) || hasInvalidID(request.ExcludeRepoIDs) || overlaps(request.PositiveRepoIDs, request.NegativeRepoIDs) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "repo ids must be positive and positive/negative seeds must not overlap", nil)
		return
	}
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > 100 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be between 1 and 100", nil)
		return
	}
	result, err := h.provider.RecommendMany(r.Context(), provider.MultiQuery{
		PositiveRepoIDs: request.PositiveRepoIDs,
		NegativeRepoIDs: request.NegativeRepoIDs,
		ExcludeRepoIDs:  request.ExcludeRepoIDs,
		Limit:           request.Limit,
	})
	if err != nil {
		writeServingError(w, err)
		return
	}
	meta := &model.Meta{PageSize: request.Limit, Total: len(result.Items), Source: result.Source, CacheStatus: "bundle"}
	writeJSONWithMeta(w, result, meta)
}

func writeServingError(w http.ResponseWriter, err error) {
	if errors.Is(err, serving.ErrNoActiveBundle) {
		writeError(w, http.StatusServiceUnavailable, "MODEL_UNAVAILABLE", "no active recommendation model", nil)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "SERVING_UNAVAILABLE", "recommendation serving is unavailable", nil)
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, repoID := range ids {
		if _, exists := seen[repoID]; exists {
			continue
		}
		seen[repoID] = struct{}{}
		result = append(result, repoID)
	}
	return result
}

func hasInvalidID(ids []int64) bool {
	for _, repoID := range ids {
		if repoID <= 0 {
			return true
		}
	}
	return false
}

func overlaps(left []int64, right []int64) bool {
	values := make(map[int64]struct{}, len(left))
	for _, repoID := range left {
		values[repoID] = struct{}{}
	}
	for _, repoID := range right {
		if _, exists := values[repoID]; exists {
			return true
		}
	}
	return false
}
