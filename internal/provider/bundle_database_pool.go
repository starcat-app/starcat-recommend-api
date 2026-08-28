package provider

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

// bundleDatabasePool 按不可变模型版本复用只读 SQLite 连接池。
//
// 激活新模型时，已经进入旧模型查询的请求仍应使用同一个 Bundle 完成。因此这里不能
// 简单关闭“上一条连接”，而是给每个版本计数；只有版本已不再 active 且引用归零时才关闭。
type bundleDatabasePool struct {
	mu      sync.Mutex
	entries map[string]*bundleDatabaseEntry
	closed  bool
}

type bundleDatabaseEntry struct {
	path       string
	handle     *bundleDatabaseHandle
	references int
}

// bundleDatabaseHandle 把共享连接与该不可变 Bundle 的可选能力固定在一起。
// 旧 v12 没有 display_score，新 Bundle 才有；能力只需在首次打开版本时探测一次。
type bundleDatabaseHandle struct {
	database             *sql.DB
	supportsDisplayScore bool
}

func newBundleDatabasePool() *bundleDatabasePool {
	return &bundleDatabasePool{entries: make(map[string]*bundleDatabaseEntry)}
}

// acquire 返回指定不可变 Bundle 的共享只读连接，以及必须调用一次的释放函数。
func (p *bundleDatabasePool) acquire(
	ctx context.Context,
	bundle serving.ActiveBundle,
) (*bundleDatabaseHandle, func(string), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, fmt.Errorf("trained bundle database pool is closed")
	}

	path := filepath.Clean(bundle.DatabasePath)
	if entry, ok := p.entries[bundle.Version]; ok {
		if entry.path != path {
			return nil, nil, fmt.Errorf("model version %s database path changed", bundle.Version)
		}
		entry.references++
		return entry.handle, p.releaseFunc(bundle.Version), nil
	}

	// ServingBundle 安装后内容不可变，immutable=1 可省掉 SQLite 的文件变更检查；
	// mode=ro 则从连接层阻止推荐服务意外写入训练产物。
	database, err := sql.Open(
		"sqlite",
		"file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1",
	)
	if err != nil {
		return nil, nil, err
	}
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, nil, err
	}
	supportsDisplayScore, err := tableHasColumn(ctx, database, "recommendations", "display_score")
	if err != nil {
		_ = database.Close()
		return nil, nil, err
	}
	handle := &bundleDatabaseHandle{
		database:             database,
		supportsDisplayScore: supportsDisplayScore,
	}
	p.entries[bundle.Version] = &bundleDatabaseEntry{
		path:       path,
		handle:     handle,
		references: 1,
	}
	return handle, p.releaseFunc(bundle.Version), nil
}

// tableHasColumn 支持 API 在不中断旧 Bundle 的前提下增量扩展 Serving schema。
func tableHasColumn(ctx context.Context, database *sql.DB, table string, column string) (bool, error) {
	var count int
	err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
		table,
		column,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// releaseFunc 让调用方在请求结束时传入当时的 active 版本，以安全回收所有非 active 连接。
func (p *bundleDatabasePool) releaseFunc(version string) func(string) {
	return func(activeVersion string) {
		p.mu.Lock()
		defer p.mu.Unlock()
		entry, ok := p.entries[version]
		if ok && entry.references > 0 {
			entry.references--
		}
		for candidateVersion, candidate := range p.entries {
			if candidateVersion == activeVersion || candidate.references > 0 {
				continue
			}
			_ = candidate.handle.database.Close()
			delete(p.entries, candidateVersion)
		}
	}
}

// close 关闭服务持有的全部 SQLite 连接。Service.Close 会且只会调用一次。
func (p *bundleDatabasePool) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var firstErr error
	for version, entry := range p.entries {
		if err := entry.handle.database.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.entries, version)
	}
	return firstErr
}
