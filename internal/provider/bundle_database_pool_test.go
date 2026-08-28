package provider

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

func TestBundleDatabasePoolReusesActiveVersionAndRetiresInactiveVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := newBundleDatabasePool()
	t.Cleanup(func() { _ = pool.close() })
	v1 := testBundleWithDatabase(t, "v1")
	v2 := testBundleWithDatabase(t, "v2")

	v1Database, releaseV1First, err := pool.acquire(ctx, v1)
	if err != nil {
		t.Fatalf("acquire v1: %v", err)
	}
	v1DatabaseAgain, releaseV1Second, err := pool.acquire(ctx, v1)
	if err != nil {
		t.Fatalf("acquire v1 again: %v", err)
	}
	if v1Database != v1DatabaseAgain {
		t.Fatal("same model version should reuse one sql.DB")
	}
	releaseV1First(v1.Version)
	releaseV1Second(v1.Version)
	if err := v1Database.PingContext(ctx); err != nil {
		t.Fatalf("active v1 database should remain open: %v", err)
	}

	v1Held, releaseV1Held, err := pool.acquire(ctx, v1)
	if err != nil {
		t.Fatalf("hold v1: %v", err)
	}
	v2Database, releaseV2, err := pool.acquire(ctx, v2)
	if err != nil {
		t.Fatalf("acquire v2: %v", err)
	}
	releaseV2(v2.Version)
	if err := v1Held.PingContext(ctx); err != nil {
		t.Fatalf("referenced inactive v1 database closed too early: %v", err)
	}

	releaseV1Held(v2.Version)
	if err := v1Held.PingContext(ctx); err == nil {
		t.Fatal("inactive v1 database should close after its final request releases it")
	}
	if err := v2Database.PingContext(ctx); err != nil {
		t.Fatalf("active v2 database should remain open: %v", err)
	}

	if err := pool.close(); err != nil {
		t.Fatalf("close pool: %v", err)
	}
	if err := v2Database.PingContext(ctx); err == nil {
		t.Fatal("service close should close active database")
	}
}

// testBundleWithDatabase 创建最小 SQLite 文件；连接池只关心文件可读与版本隔离，
// 推荐表结构本身由 serving.VerifyBundle 的既有测试覆盖。
func testBundleWithDatabase(t *testing.T, version string) serving.ActiveBundle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recommendations.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	if _, err := database.Exec("CREATE TABLE probe (id INTEGER PRIMARY KEY)"); err != nil {
		_ = database.Close()
		t.Fatalf("create fixture table: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	return serving.ActiveBundle{Version: version, DatabasePath: path}
}
