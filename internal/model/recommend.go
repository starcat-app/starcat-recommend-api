package model

// RecommendationItem 是 Starcat 客户端消费的稳定推荐卡片 DTO。
//
// 字段名与 Starcat 现有后端保持 snake_case。不要把 SimRepo/Qdrant 原始 payload
// 直接透给客户端, 上游变化应在 provider 层消化。
type RecommendationItem struct {
	RepoID       int64          `json:"repo_id"`
	FullName     string         `json:"full_name"`
	Description  string         `json:"description,omitempty"`
	Language     string         `json:"language,omitempty"`
	Stars        int            `json:"stars"`
	Forks        int            `json:"forks"`
	Archived     bool           `json:"archived"`
	Score        float64        `json:"score"`
	DisplayScore *float64       `json:"display_score,omitempty"`
	Source       string         `json:"source"`
	Reasons      []string       `json:"reasons,omitempty"`
	Signals      map[string]any `json:"signals,omitempty"`
}

// RecommendationResponse 是 /api/v1/repos/{repo_id}/recommendations 的 data 段。
type RecommendationResponse struct {
	Source       string               `json:"source"`
	Fallback     bool                 `json:"fallback"`
	RepoID       int64                `json:"repo_id"`
	ModelVersion string               `json:"model_version,omitempty"`
	Items        []RecommendationItem `json:"items"`
	HasMore      bool                 `json:"has_more"`
	NextOffset   *int                 `json:"next_offset,omitempty"`
}

// MultiRecommendationResponse 是 v2 多 seed 查询的 data 段。
type MultiRecommendationResponse struct {
	Source       string               `json:"source"`
	Fallback     bool                 `json:"fallback"`
	ModelVersion string               `json:"model_version"`
	Items        []RecommendationItem `json:"items"`
}
