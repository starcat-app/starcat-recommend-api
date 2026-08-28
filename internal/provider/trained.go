package provider

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/starcat-app/starcat-recommend-api/internal/model"
	"github.com/starcat-app/starcat-recommend-api/internal/serving"
	_ "modernc.org/sqlite"
)

// MultiQuery 是 v2 多 seed 推荐的服务内契约。
type MultiQuery struct {
	PositiveRepoIDs []int64
	NegativeRepoIDs []int64
	ExcludeRepoIDs  []int64
	Limit           int
}

// TrainedProvider 只读 Trainer 生成的 SQLite ServingBundle。
//
// Provider 不接触 Collection 原始快照，也不保留 participant_id。所有查询先固定当前
// active Bundle 路径，因此并发激活新版本不会让单个请求混用两个模型。
type TrainedProvider struct {
	registry  *serving.Registry
	databases *bundleDatabasePool
}

// NewTrainedProvider 创建自研推荐 Provider。
func NewTrainedProvider(registry *serving.Registry) *TrainedProvider {
	return &TrainedProvider{registry: registry, databases: newBundleDatabasePool()}
}

// Close 释放 Provider 复用的所有 ServingBundle 数据库连接。
func (p *TrainedProvider) Close() error {
	return p.databases.close()
}

// Recommend 查询一个 seed repo 的预计算 Top-K。
func (p *TrainedProvider) Recommend(ctx context.Context, query Query) (Result, error) {
	bundle, err := p.registry.Active()
	if err != nil {
		return Result{}, err
	}
	database, release, err := p.databases.acquire(ctx, bundle)
	if err != nil {
		return Result{}, err
	}
	defer func() { release(p.activeVersion()) }()
	rows, err := queryRows(ctx, database, []weightedSeed{{query.RepoID, 1}}, []int64{query.RepoID}, query.Limit+1, query.Offset)
	if err != nil {
		return Result{}, err
	}
	hasMore := len(rows) > query.Limit
	if hasMore {
		rows = rows[:query.Limit]
	}
	var nextOffset *int
	if hasMore {
		value := query.Offset + len(rows)
		nextOffset = &value
	}
	return Result{
		Response: model.RecommendationResponse{
			Source:       "starcat_trained",
			Fallback:     false,
			RepoID:       query.RepoID,
			ModelVersion: bundle.Version,
			Items:        rows,
			HasMore:      hasMore,
			NextOffset:   nextOffset,
		},
		CacheStatus: "bundle",
	}, nil
}

// RecommendMany 合并 positive 分数并扣减 negative 分数，返回去重结果。
func (p *TrainedProvider) RecommendMany(ctx context.Context, query MultiQuery) (model.MultiRecommendationResponse, error) {
	bundle, err := p.registry.Active()
	if err != nil {
		return model.MultiRecommendationResponse{}, err
	}
	database, release, err := p.databases.acquire(ctx, bundle)
	if err != nil {
		return model.MultiRecommendationResponse{}, err
	}
	defer func() { release(p.activeVersion()) }()
	seeds := make([]weightedSeed, 0, len(query.PositiveRepoIDs)+len(query.NegativeRepoIDs))
	for _, repoID := range query.PositiveRepoIDs {
		seeds = append(seeds, weightedSeed{repoID, 1})
	}
	for _, repoID := range query.NegativeRepoIDs {
		seeds = append(seeds, weightedSeed{repoID, -1})
	}
	excluded := append([]int64{}, query.ExcludeRepoIDs...)
	excluded = append(excluded, query.PositiveRepoIDs...)
	excluded = append(excluded, query.NegativeRepoIDs...)
	items, err := queryRows(ctx, database, seeds, excluded, query.Limit, 0)
	if err != nil {
		return model.MultiRecommendationResponse{}, err
	}
	return model.MultiRecommendationResponse{
		Source:       "starcat_trained",
		Fallback:     false,
		ModelVersion: bundle.Version,
		Items:        items,
	}, nil
}

// activeVersion 在请求释放连接时重新读取 active 指针。模型切换期间，旧请求因此只会
// 在执行完查询后关闭旧版本连接，不会影响已经固定的 Bundle 快照。
func (p *TrainedProvider) activeVersion() string {
	bundle, err := p.registry.Active()
	if err != nil {
		return ""
	}
	return bundle.Version
}

type weightedSeed struct {
	repoID int64
	weight int
}

func queryRows(
	ctx context.Context,
	database *sql.DB,
	seeds []weightedSeed,
	excludeRepoIDs []int64,
	limit int,
	offset int,
) ([]model.RecommendationItem, error) {
	values := make([]string, 0, len(seeds))
	parameters := make([]any, 0, len(seeds)*2+len(excludeRepoIDs)+2)
	for _, seed := range seeds {
		values = append(values, "(?, ?)")
		parameters = append(parameters, seed.repoID, seed.weight)
	}
	excluded := uniquePositiveIDs(excludeRepoIDs)
	exclusion := ""
	if len(excluded) > 0 {
		placeholders := make([]string, len(excluded))
		for index, repoID := range excluded {
			placeholders[index] = "?"
			parameters = append(parameters, repoID)
		}
		exclusion = "AND r.target_repo_id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	parameters = append(parameters, limit, offset)
	statement := fmt.Sprintf(`
WITH seeds(repo_id, weight) AS (VALUES %s), scored AS (
    SELECT
        r.target_repo_id,
        SUM(r.score * seeds.weight) AS combined_score,
        MIN(r.rank) AS best_rank,
        MIN(r.model) AS model,
        MIN(r.signals) AS signals
    FROM recommendations r
    JOIN seeds ON seeds.repo_id = r.source_repo_id
    WHERE 1 = 1 %s
    GROUP BY r.target_repo_id
    HAVING combined_score > 0
)
SELECT
    scored.target_repo_id,
    repositories.full_name,
    repositories.description,
    repositories.primary_language,
    repositories.stars,
    repositories.forks,
    scored.combined_score,
    scored.best_rank,
    scored.model,
    scored.signals
FROM scored
JOIN repositories ON repositories.repo_id = scored.target_repo_id
ORDER BY scored.combined_score DESC, scored.best_rank, scored.target_repo_id
LIMIT ? OFFSET ?`, strings.Join(values, ","), exclusion)

	rows, err := database.QueryContext(ctx, statement, parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.RecommendationItem, 0, limit)
	for rows.Next() {
		var (
			item        model.RecommendationItem
			description sql.NullString
			language    sql.NullString
			modelName   string
			signalsJSON string
			bestRank    int
		)
		if err := rows.Scan(
			&item.RepoID,
			&item.FullName,
			&description,
			&language,
			&item.Stars,
			&item.Forks,
			&item.Score,
			&bestRank,
			&modelName,
			&signalsJSON,
		); err != nil {
			return nil, err
		}
		if description.Valid {
			item.Description = description.String
		}
		if language.Valid {
			item.Language = language.String
		}
		item.Source = "starcat_trained"
		item.Signals = parseSignals(signalsJSON)
		item.Reasons = reasonsFor(modelName, item.Signals)
		items = append(items, item)
	}
	return items, rows.Err()
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, repoID := range ids {
		if repoID <= 0 {
			continue
		}
		if _, exists := seen[repoID]; exists {
			continue
		}
		seen[repoID] = struct{}{}
		result = append(result, repoID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func parseSignals(raw string) map[string]any {
	var signals map[string]any
	if json.Unmarshal([]byte(raw), &signals) != nil {
		return map[string]any{"raw": raw}
	}
	return signals
}

func reasonsFor(modelName string, signals map[string]any) []string {
	kind, _ := signals["kind"].(string)
	switch kind {
	case "time_decayed_costar":
		return []string{"基于公开 Star 共现关系"}
	case "global_popularity":
		return []string{"基于公开 Star 热度"}
	case "truncated_svd":
		return []string{"基于仓库协同相似度"}
	default:
		if strings.Contains(modelName, "svd") {
			return []string{"基于仓库协同相似度"}
		}
		return []string{"基于自研推荐模型"}
	}
}
