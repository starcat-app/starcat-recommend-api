// Package provider 封装推荐数据源。
//
// v1 兼容层与 v2 共用自研 Query/Result，不在客户端暴露底层存储细节。
package provider

import "github.com/starcat-app/starcat-recommend-api/internal/model"

type Query struct {
	RepoID int64
	Limit  int
	Offset int
}

type Result struct {
	Response    model.RecommendationResponse
	CacheStatus string
}
