package provider

// 本模块用 Trainer 导出的词表和稀疏单位向量补充无预计算边的单仓查询。
// 不回写不可变 Bundle，不上传用户 Stars，不用全站热门冒充内容相似推荐。

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	kitgithub "github.com/starcat-app/starcat-api-kit/github"
	"github.com/starcat-app/starcat-recommend-api/internal/model"
)

var contentTokens = regexp.MustCompile(`[a-z][a-z0-9_]{1,}`)

// RepositoryMetadataReader 复用 kit 的限流和 token pool；测试替换此边界，不联网。
type RepositoryMetadataReader interface {
	GetRepoByID(context.Context, int64) (*kitgithub.Repo, error)
}

type contentRepository struct {
	ID          int64
	FullName    string
	Description string
	Topics      []string
	Language    string
}

type metadataEntry struct {
	repo    *contentRepository
	expires time.Time
}

type metadataFlight struct {
	done chan struct{}
	repo *contentRepository
	err  error
}

// contentMetadataCache 对公开元数据做有界缓存及同 ID 请求合并。
// 私有/删除项目仅保存短期 nil，绝不保存其名称或描述；临时错误不伪装成永久空结果。
type contentMetadataCache struct {
	reader       RepositoryMetadataReader
	mu           sync.Mutex
	items        map[int64]metadataEntry
	active       map[int64]*metadataFlight
	slots        chan struct{}
	nextRequest  time.Time
	backoffUntil time.Time
}

func newContentMetadataCache(reader RepositoryMetadataReader) *contentMetadataCache {
	return &contentMetadataCache{reader: reader, items: make(map[int64]metadataEntry),
		active: make(map[int64]*metadataFlight), slots: make(chan struct{}, 2)}
}

func (c *contentMetadataCache) get(ctx context.Context, id int64) (*contentRepository, error) {
	c.mu.Lock()
	if entry, ok := c.items[id]; ok && time.Now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.repo, nil
	}
	if flight, ok := c.active[id]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return flight.repo, flight.err
		}
	}
	if c.reader == nil {
		c.mu.Unlock()
		return nil, nil
	}
	flight := &metadataFlight{done: make(chan struct{})}
	c.active[id] = flight
	c.mu.Unlock()
	repo, err := c.fetch(ctx, id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		// FIFO 按过期时间淘汰，容量固定；不让随机 repo ID 无限占用内存。
		if len(c.items) >= 4096 {
			var oldest int64
			for key, entry := range c.items {
				if oldest == 0 || entry.expires.Before(c.items[oldest].expires) {
					oldest = key
				}
			}
			delete(c.items, oldest)
		}
		ttl := 7 * 24 * time.Hour
		if repo == nil {
			ttl = 15 * time.Minute
		}
		c.items[id] = metadataEntry{repo, time.Now().Add(ttl)}
	}
	flight.repo, flight.err = repo, err
	delete(c.active, id)
	close(flight.done)
	return repo, err
}

func (c *contentMetadataCache) fetch(ctx context.Context, id int64) (*contentRepository, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// 使用可取消的服务级 720ms 间隔；不使用 kit 的长时间阻塞 Wait，
	// 避免上游限流让用户请求或服务关闭无限等待。
	c.mu.Lock()
	now := time.Now()
	if now.Before(c.backoffUntil) {
		c.mu.Unlock()
		return nil, fmt.Errorf("public metadata temporarily backed off")
	}
	start := c.nextRequest
	if start.Before(now) {
		start = now
	}
	c.nextRequest = start.Add(720 * time.Millisecond)
	c.mu.Unlock()
	timer := time.NewTimer(time.Until(start))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	repo, err := c.reader.GetRepoByID(ctx, id)
	if errors.Is(err, kitgithub.ErrRepoNotFound) {
		return nil, nil
	}
	if err != nil {
		c.mu.Lock()
		c.backoffUntil = time.Now().Add(time.Minute)
		c.mu.Unlock()
		// 不把上游错误正文（可能含敏感信息）传到请求日志或客户端。
		return nil, fmt.Errorf("public repository metadata unavailable")
	}
	if repo == nil || repo.ID != id || repo.Private || repo.Archived || repo.Disabled || repo.FullName == "" {
		return nil, nil
	}
	result := &contentRepository{ID: id, FullName: repo.FullName, Topics: repo.Topics}
	if repo.Description != nil {
		result.Description = *repo.Description
	}
	if repo.Language != nil {
		result.Language = *repo.Language
	}
	if len(result.FullName) > 256 || len(result.Description) > 16_384 || len(result.Topics) > 64 {
		return nil, fmt.Errorf("public metadata exceeds content budget")
	}
	return result, nil
}

// contentRecommendations 固定 Bundle 后查内容索引；source 的内容摘要参与响应版本，
// 因而公开元数据更新不会在同一模型版本下混合分页或永久命中错误的 304。
func (p *TrainedProvider) contentRecommendations(ctx context.Context, db *bundleDatabaseHandle, query Query, version string) ([]model.RecommendationItem, string, error) {
	repo := &contentRepository{ID: query.RepoID}
	var description, language sql.NullString
	var topics string
	err := db.database.QueryRowContext(ctx, `SELECT full_name,description,topics_json,primary_language
		FROM repositories WHERE repo_id=?`, query.RepoID).Scan(&repo.FullName, &description, &topics, &language)
	if errors.Is(err, sql.ErrNoRows) {
		repo, err = p.metadata.get(ctx, query.RepoID)
	} else if err == nil {
		repo.Description, repo.Language = description.String, language.String
		err = json.Unmarshal([]byte(topics), &repo.Topics)
	}
	if err != nil || repo == nil {
		return []model.RecommendationItem{}, version, err
	}
	body, _ := json.Marshal(repo)
	digest := sha256.Sum256(body)
	version += "-c" + hex.EncodeToString(digest[:8])
	document := strings.Join([]string{repo.FullName, repo.Description,
		strings.Join(repo.Topics, " "), strings.Join(repo.Topics, " "), strings.Join(repo.Topics, " "),
		repo.Language, repo.Language}, " ")
	counts := make(map[string]int)
	for _, term := range contentTokens.FindAllString(strings.ToLower(document), -1) {
		counts[term]++
	}
	// 只读取命中的词，不在每个请求中重载完整词表；源字段本身也有输入长度预算。
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	if len(terms) > 512 {
		terms = terms[:512]
	}
	if len(terms) == 0 {
		return []model.RecommendationItem{}, version, nil
	}
	parameters := make([]any, len(terms))
	for i, term := range terms {
		parameters[i] = term
	}
	rows, err := db.database.QueryContext(ctx, "SELECT term,idf FROM content_vocabulary WHERE term IN ("+strings.TrimRight(strings.Repeat("?,", len(terms)), ",")+")", parameters...)
	if err != nil {
		return nil, version, err
	}
	type feature struct {
		term   string
		weight float64
	}
	var features []feature
	var norm float64
	for rows.Next() {
		var term string
		var idf float64
		if err := rows.Scan(&term, &idf); err != nil {
			_ = rows.Close()
			return nil, version, err
		}
		weight := (1 + math.Log(float64(counts[term]))) * idf
		norm += weight * weight
		features = append(features, feature{term, weight})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || norm == 0 {
		return []model.RecommendationItem{}, version, err
	}
	sort.Slice(features, func(i, j int) bool {
		if features[i].weight == features[j].weight {
			return features[i].term < features[j].term
		}
		return features[i].weight > features[j].weight
	})
	if len(features) > 128 {
		features = features[:128]
	}
	values := make([]string, len(features))
	parameters = nil
	for i, f := range features {
		values[i] = "(?,?)"
		parameters = append(parameters, f.term, f.weight/math.Sqrt(norm))
	}
	parameters = append(parameters, query.RepoID, query.Limit+1, query.Offset)
	statement := `WITH query(term,weight) AS (VALUES ` + strings.Join(values, ",") + `),
		scored AS (SELECT p.repo_id,SUM(p.weight*q.weight) AS score
		FROM query q JOIN content_postings p ON p.term=q.term WHERE p.repo_id<>?
		GROUP BY p.repo_id HAVING score >= 0.08)
		SELECT r.repo_id,r.full_name,coalesce(r.description,''),coalesce(r.primary_language,''),
		r.stars,r.forks,s.score FROM scored s JOIN repositories r USING(repo_id)
		ORDER BY s.score DESC,r.repo_id LIMIT ? OFFSET ?`
	resultRows, err := db.database.QueryContext(ctx, statement, parameters...)
	if err != nil {
		return nil, version, err
	}
	defer resultRows.Close()
	items := make([]model.RecommendationItem, 0, query.Limit+1)
	for resultRows.Next() {
		var item model.RecommendationItem
		if err := resultRows.Scan(&item.RepoID, &item.FullName, &item.Description, &item.Language, &item.Stars, &item.Forks, &item.Score); err != nil {
			return nil, version, err
		}
		item.Source = "starcat_trained"
		item.Signals = map[string]any{"kind": "metadata_tfidf", "cosine": item.Score}
		item.Reasons = []string{"metadata_similarity"}
		// 余弦相似度不冒充 v19 边分数百分位，客户端按原有无 display_score 路径展示。
		items = append(items, item)
	}
	return items, version, resultRows.Err()
}
