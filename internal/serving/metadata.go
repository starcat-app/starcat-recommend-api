// Package serving 定义可公开的模型说明；未知历史字段保留 null，不从打包时间猜测。
package serving

import (
	"encoding/json"
	"strconv"
	"strings"
)

// TrainingMetadata 描述实际训练执行区间与输入范围，不包含训练主体或文件路径。
type TrainingMetadata struct {
	StartedAt    *string            `json:"started_at"`
	CompletedAt  *string            `json:"completed_at"`
	DataStart    *string            `json:"data_start"`
	DataEnd      *string            `json:"data_end"`
	Interactions *int64             `json:"interactions"`
	Subjects     *int64             `json:"subjects"`
	Parameters   map[string]float64 `json:"parameters"`
}

// EvaluationMetadata 将可比较的指标绑定到切分与数据摘要，避免把 refit 当成再次实测。
type EvaluationMetadata struct {
	Split           *string `json:"split"`
	K               *int    `json:"k"`
	TrainEnd        *string `json:"train_end"`
	ValidationEnd   *string `json:"validation_end"`
	DataEnd         *string `json:"data_end"`
	MaximumSeeds    *int    `json:"maximum_seeds"`
	DatasetChecksum *string `json:"dataset_checksum"`
}

// ModelInfo 固定同一个活动 Bundle 的元数据与规模，切换时不会拼接新旧模型字段。
type ModelInfo struct {
	Manifest
	Metrics             map[string]float64 `json:"metrics"`
	Repositories        int64              `json:"repositories"`
	RecommendationEdges int64              `json:"recommendation_edges"`
	DatabaseBytes       int64              `json:"database_bytes"`
}

// ModelInfo 返回当前实际激活模型；空 Registry 是可观察状态而不是服务故障。
func (r *Registry) ModelInfo() (*ModelInfo, error) {
	stats, err := r.Stats()
	if err != nil || !stats.Active {
		return nil, err
	}
	return &ModelInfo{
		Manifest: stats.manifest, Repositories: stats.Repositories,
		RecommendationEdges: stats.RecommendationEdges, DatabaseBytes: stats.DatabaseBytes,
		Metrics: publicModelMetrics(stats.Metrics),
	}, nil
}

// publicModelMetrics 仅返回聚合数值；历史 manifest 的任意附加内容不能透传到公开接口。
func publicModelMetrics(raw json.RawMessage) map[string]float64 {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	result := make(map[string]float64)
	for name, field := range fields {
		allowed := false
		switch name {
		case "coverage", "empty_result_rate", "evaluated_subjects", "skipped_subjects",
			"catalog_recall", "catalog_truth_count", "truth_count", "truth_catalog_coverage":
			allowed = true
		default:
			metric, cutoff, found := strings.Cut(name, "@")
			if found && (metric == "recall" || metric == "ndcg" || metric == "mrr") {
				value, err := strconv.Atoi(cutoff)
				allowed = err == nil && value > 0
			}
		}
		if !allowed || string(field) == "null" {
			continue
		}
		var value float64
		if err := json.Unmarshal(field, &value); err == nil {
			result[name] = value
		}
	}
	return result
}
