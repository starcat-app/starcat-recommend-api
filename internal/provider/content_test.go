package provider

// 内容冷启动不依赖真实 GitHub；用固定稀疏向量验证余弦、隐私、快照和请求合并。
import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kitgithub "github.com/starcat-app/starcat-api-kit/github"
)

type metadataStub struct {
	repo  *kitgithub.Repo
	err   error
	calls atomic.Int32
	delay time.Duration
}

func (s *metadataStub) GetRepoByID(ctx context.Context, id int64) (*kitgithub.Repo, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	return s.repo, s.err
}

func contentTestDatabase(t *testing.T) *bundleDatabaseHandle {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "content.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE repositories(repo_id INTEGER PRIMARY KEY,full_name TEXT,
		description TEXT,topics_json TEXT,primary_language TEXT,stars INTEGER,forks INTEGER);
		INSERT INTO repositories VALUES(1,'one/database','database parser','[]','',12,1),
		(2,'two/music','guitar piano','[]','',400,2),(3,'three/database','database tool','[]','',8,1);
		CREATE TABLE content_vocabulary(term TEXT PRIMARY KEY,idf REAL);
		INSERT INTO content_vocabulary VALUES('database',1),('parser',1),('guitar',1),('piano',1);
		CREATE TABLE content_postings(term TEXT,repo_id INTEGER,weight REAL,PRIMARY KEY(term,repo_id));
		INSERT INTO content_postings VALUES('database',1,0.8),('parser',1,0.6),
		('guitar',2,0.8),('piano',2,0.6),('database',3,1);`)
	if err != nil {
		t.Fatal(err)
	}
	return &bundleDatabaseHandle{database: db, supportsContentIndex: true}
}

func TestContentExternalSourcePaginationAndSnapshot(t *testing.T) {
	db := contentTestDatabase(t)
	description := "database parser"
	reader := &metadataStub{repo: &kitgithub.Repo{ID: 99, FullName: "external/tool", Description: &description}}
	p := &TrainedProvider{metadata: newContentMetadataCache(reader)}
	first, version, err := p.contentRecommendations(context.Background(), db, Query{RepoID: 99, Limit: 1}, "v20")
	if err != nil || len(first) != 2 || first[0].RepoID != 1 || first[1].RepoID != 3 {
		t.Fatalf("unexpected content neighbors: items=%v err=%v", first, err)
	}
	if first[0].DisplayScore != nil || version == "v20" || first[0].Score > 1 {
		t.Fatal("content must use its own snapshot without invented calibrated display score")
	}
	second, nextVersion, err := p.contentRecommendations(context.Background(), db, Query{RepoID: 99, Limit: 1, Offset: 1}, "v20")
	if err != nil || len(second) != 1 || second[0].RepoID != 3 || version != nextVersion || reader.calls.Load() != 1 {
		t.Fatal("pagination or metadata cache changed source snapshot")
	}
	// 内容变化必须有不同身份；否则客户端可能永远收到错误 304 或合并不同内容的分页。
	p.metadata.items = make(map[int64]metadataEntry)
	description = "guitar piano"
	reader.repo.Description = &description
	third, changedVersion, err := p.contentRecommendations(context.Background(), db, Query{RepoID: 99, Limit: 1}, "v20")
	if err != nil || third[0].RepoID != 2 || version == changedVersion {
		t.Fatal("metadata revision not reflected")
	}
}

func TestContentCatalogSourceExcludesSelfWithoutNetwork(t *testing.T) {
	reader := &metadataStub{err: errors.New("must not call upstream")}
	p := &TrainedProvider{metadata: newContentMetadataCache(reader)}
	items, _, err := p.contentRecommendations(context.Background(), contentTestDatabase(t), Query{RepoID: 1, Limit: 5}, "v20")
	if err != nil || len(items) != 1 || items[0].RepoID != 3 || reader.calls.Load() != 0 {
		t.Fatalf("self exclusion or catalog-only read failed: %v %v", items, err)
	}
}

func TestContentPrivateDeletedDisabledAndIdentityGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo *kitgithub.Repo
		err  error
	}{
		{"private", &kitgithub.Repo{ID: 99, FullName: "private/repo", Private: true}, nil},
		{"disabled", &kitgithub.Repo{ID: 99, FullName: "test/repo", Disabled: true}, nil},
		{"archived", &kitgithub.Repo{ID: 99, FullName: "test/repo", Archived: true}, nil},
		{"wrong-id", &kitgithub.Repo{ID: 98, FullName: "test/repo"}, nil},
		{"deleted", nil, kitgithub.ErrRepoNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &metadataStub{repo: tc.repo, err: tc.err}
			cache := newContentMetadataCache(reader)
			for range 2 {
				result, err := cache.get(context.Background(), 99)
				if err != nil || result != nil {
					t.Fatal("unavailable repository leaked content")
				}
			}
			if reader.calls.Load() != 1 || cache.items[99].repo != nil {
				t.Fatal("negative cache must only retain nil")
			}
		})
	}
}

func TestContentConcurrentLookupAndTransientFailure(t *testing.T) {
	reader := &metadataStub{repo: &kitgithub.Repo{ID: 99, FullName: "public/parser"}, delay: 20 * time.Millisecond}
	cache := newContentMetadataCache(reader)
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := cache.get(context.Background(), 99); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if reader.calls.Load() != 1 {
		t.Fatal("same-ID requests were not coalesced")
	}
	failed := newContentMetadataCache(&metadataStub{err: errors.New("upstream secret must not escape")})
	if _, err := failed.get(context.Background(), 99); err == nil || err.Error() != "public repository metadata unavailable" {
		t.Fatal("transient failures must remain errors with sanitized detail")
	}
	if len(failed.items) != 0 {
		t.Fatal("transient failures must not become success/empty snapshots")
	}
}
