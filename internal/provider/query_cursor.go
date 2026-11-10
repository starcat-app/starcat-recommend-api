// 本文件定义多种子推荐的无会话分页令牌
// 令牌固定不可变模型、规范化查询与排序版本；用户的本地推荐历史不进入令牌
package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var (
	// ErrInvalidQueryCursor 表示分页位置不属于当前种子、排除条件或页大小
	ErrInvalidQueryCursor = errors.New("invalid recommendation query cursor")
	// ErrExpiredQueryCursor 要求客户端从新模型第一页重查，禁止跨版本拼接
	ErrExpiredQueryCursor = errors.New("recommendation query cursor model expired")
)

// queryCursor 只保存公共查询的身份与位置，鉴权继续由现有 Bearer 中间件负责
type queryCursor struct {
	Version      int    `json:"v"`
	ModelVersion string `json:"model"`
	Query        string `json:"query"`
	Offset       int    `json:"offset"`
}

// queryFingerprint 忽略集合输入顺序，保留正负方向、排除集合和页大小的区别
// 排序语义改变时必须更新 Policy，避免旧位置被用于另一种结果顺序
func queryFingerprint(query MultiQuery) string {
	identity := struct {
		Positive []int64
		Negative []int64
		Excluded []int64
		Limit    int
		Policy   string
	}{uniquePositiveIDs(query.PositiveRepoIDs), uniquePositiveIDs(query.NegativeRepoIDs),
		uniquePositiveIDs(query.ExcludeRepoIDs), query.Limit, "combined-score-best-rank-id-v1"}
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// decodeQueryCursor 在查询数据库前校验身份；活动版本变化只返回明确过期错误
func decodeQueryCursor(raw string, query MultiQuery, modelVersion string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, ErrInvalidQueryCursor
	}
	var cursor queryCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != 1 || cursor.Offset <= 0 ||
		cursor.ModelVersion == "" || cursor.Query != queryFingerprint(query) {
		return 0, ErrInvalidQueryCursor
	}
	if cursor.ModelVersion != modelVersion {
		return 0, ErrExpiredQueryCursor
	}
	return cursor.Offset, nil
}

// encodeQueryCursor 的字段仅含基础值，JSON 编码不存在运行时失败路径
func encodeQueryCursor(query MultiQuery, modelVersion string, offset int) string {
	data, _ := json.Marshal(queryCursor{Version: 1, ModelVersion: modelVersion,
		Query: queryFingerprint(query), Offset: offset})
	return base64.RawURLEncoding.EncodeToString(data)
}
