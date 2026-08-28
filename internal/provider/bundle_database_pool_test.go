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
	v1 := testBundleWithDatabase(t, "v1", false)
	v2 := testBundleWithDatabase(t, "v2", false)

	v1Handle, releaseV1First, err := pool.acquire(ctx, v1)
	if err != nil {
		t.Fatalf("acquire v1: %v", err)
	}
	v1HandleAgain, releaseV1Second, err := pool.acquire(ctx, v1)
	if err != nil {
		t.Fatalf("acquire v1 again: %v", err)
	}
	if v1Handle != v1HandleAgain {
		t.Fatal("same model version should reuse one sql.DB")
	}
	if v1Handle.supportsDisplayScore {
		t.Fatal("legacy bundle must not report display_score support")
	}
	releaseV1First(v1.Version)
	releaseV1Second(v1.Version)
	if err := v1Handle.database.PingContext(ctx); err != nil {
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
	if err := v1Held.database.PingContext(ctx); err != nil {
		t.Fatalf("referenced inactive v1 database closed too early: %v", err)
	}

	releaseV1Held(v2.Version)
	if err := v1Held.database.PingContext(ctx); err == nil {
		t.Fatal("inactive v1 database should close after its final request releases it")
	}
	if err := v2Database.database.PingContext(ctx); err != nil {
		t.Fatalf("active v2 database should remain open: %v", err)
	}

	if err := pool.close(); err != nil {
		t.Fatalf("close pool: %v", err)
	}
	if err := v2Database.database.PingContext(ctx); err == nil {
		t.Fatal("service close should close active database")
	}
}

func TestBundleDatabasePoolDetectsCalibratedDisplayScore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := newBundleDatabasePool()
	t.Cleanup(func() { _ = pool.close() })
	bundle := testBundleWithDatabase(t, "v3", true)

	handle, release, err := pool.acquire(ctx, bundle)
	if err != nil {
		t.Fatalf("acquire calibrated bundle: %v", err)
	}
	release(bundle.Version)
	if !handle.supportsDisplayScore {
		t.Fatal("new bundle should expose display_score capability")
	}
}

// testBundleWithDatabase 创建最小 SQLite 文件；连接池只关心文件可读与版本隔离，
// 推荐表结构本身由 serving.VerifyBundle 的既有测试覆盖。
func testBundleWithDatabase(t *testing.T, version string, withDisplayScore bool) serving.ActiveBundle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recommendations.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	statement := "CREATE TABLE recommendations (score REAL NOT NULL)"
	if withDisplayScore {
		statement = "CREATE TABLE recommendations (score REAL NOT NULL, display_score REAL NOT NULL)"
	}
	if _, err := database.Exec(statement); err != nil {
		_ = database.Close()
		t.Fatalf("create fixture table: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	return serving.ActiveBundle{Version: version, DatabasePath: path}
}
