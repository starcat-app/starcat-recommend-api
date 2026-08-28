// Package serving 管理 Trainer 发布的不可变 ServingBundle。
//
// 推荐请求只读取 active 指针指向的 Bundle。上传先进入临时目录，完成文件白名单、
// checksum、manifest 和 SQLite 校验后才原子安装，避免半包或损坏模型影响 v2 查询。
package serving

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	manifestFile  = "manifest.json"
	checksumsFile = "checksums.json"
	databaseFile  = "recommendations.sqlite"
)

var (
	// ErrNoActiveBundle 表示服务尚未激活任何自研推荐产物。
	ErrNoActiveBundle = errors.New("no active serving bundle")
	// ErrInvalidBundle 表示上传包不满足 ServingBundle 契约。
	ErrInvalidBundle = errors.New("invalid serving bundle")
	// ErrVersionConflict 表示同一不可变版本收到不同内容。
	ErrVersionConflict = errors.New("model version already exists with different content")

	versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	requiredFiles  = map[string]struct{}{
		manifestFile:  {},
		checksumsFile: {},
		databaseFile:  {},
	}
)

// Manifest 是 Recommend API 读取的 Bundle 最小发布元数据。
type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	ModelVersion  string          `json:"model_version"`
	SelectedModel string          `json:"selected_model"`
	CreatedAt     string          `json:"created_at"`
	Metrics       json.RawMessage `json:"metrics,omitempty"`
}

// ActiveBundle 是一次查询固定使用的只读版本快照。
type ActiveBundle struct {
	Version      string
	Directory    string
	DatabasePath string
	Manifest     Manifest
}

// OperationalStats exposes active model scale and manifest metadata without filesystem paths.
type OperationalStats struct {
	Active              bool            `json:"active"`
	ModelVersion        string          `json:"model_version,omitempty"`
	SelectedModel       string          `json:"selected_model,omitempty"`
	CreatedAt           string          `json:"created_at,omitempty"`
	Metrics             json.RawMessage `json:"metrics,omitempty"`
	Repositories        int64           `json:"repositories"`
	RecommendationEdges int64           `json:"recommendation_edges"`
	DatabaseBytes       int64           `json:"database_bytes"`
}

// Registry 管理版本目录和 active 指针。
//
// 查询取得不可变路径后由 TrainedProvider 按版本复用只读 SQLite 连接；Provider 通过
// 请求引用计数保证激活新版本时不会关闭仍在执行的旧请求。
type Registry struct {
	root       string
	versions   string
	activeFile string
	mu         sync.RWMutex
	active     *ActiveBundle
}

// NewRegistry 创建目录，并在存在 active.json 时恢复已激活版本。
func NewRegistry(root string) (*Registry, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("model registry directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	versions := filepath.Join(root, "versions")
	if err := os.MkdirAll(versions, 0o750); err != nil {
		return nil, err
	}
	registry := &Registry{
		root:       root,
		versions:   versions,
		activeFile: filepath.Join(root, "active.json"),
	}
	if err := registry.restoreActive(); err != nil {
		return nil, err
	}
	return registry, nil
}

// Active 返回当前版本快照；没有模型时返回 ErrNoActiveBundle。
func (r *Registry) Active() (ActiveBundle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active == nil {
		return ActiveBundle{}, ErrNoActiveBundle
	}
	return *r.active, nil
}

// Stats opens the immutable active database read-only and returns bounded aggregate metadata.
func (r *Registry) Stats() (OperationalStats, error) {
	bundle, err := r.Active()
	if errors.Is(err, ErrNoActiveBundle) {
		return OperationalStats{Active: false}, nil
	}
	if err != nil {
		return OperationalStats{}, err
	}
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(bundle.DatabasePath)+"?mode=ro")
	if err != nil {
		return OperationalStats{}, err
	}
	defer database.Close()
	result := OperationalStats{Active: true, ModelVersion: bundle.Version,
		SelectedModel: bundle.Manifest.SelectedModel, CreatedAt: bundle.Manifest.CreatedAt,
		Metrics: bundle.Manifest.Metrics}
	if err := database.QueryRow("SELECT COUNT(*) FROM repositories").Scan(&result.Repositories); err != nil {
		return OperationalStats{}, err
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM recommendations").Scan(&result.RecommendationEdges); err != nil {
		return OperationalStats{}, err
	}
	if info, err := os.Stat(bundle.DatabasePath); err == nil {
		result.DatabaseBytes = info.Size()
	} else {
		return OperationalStats{}, err
	}
	return result, nil
}

// InstallZip 安装 application/zip Bundle，并可在校验成功后立即激活。
func (r *Registry) InstallZip(ctx context.Context, version string, reader io.Reader, activate bool) (ActiveBundle, error) {
	if !versionPattern.MatchString(version) {
		return ActiveBundle{}, fmt.Errorf("%w: invalid model version", ErrInvalidBundle)
	}
	staging, err := os.MkdirTemp(r.root, ".staging-")
	if err != nil {
		return ActiveBundle{}, err
	}
	defer os.RemoveAll(staging)

	archivePath := filepath.Join(staging, "bundle.zip")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ActiveBundle{}, err
	}
	if _, err := io.Copy(archive, reader); err != nil {
		archive.Close()
		return ActiveBundle{}, err
	}
	if err := archive.Close(); err != nil {
		return ActiveBundle{}, err
	}
	if err := ctx.Err(); err != nil {
		return ActiveBundle{}, err
	}

	extracted := filepath.Join(staging, "bundle")
	if err := extractBundle(archivePath, extracted); err != nil {
		return ActiveBundle{}, err
	}
	manifest, err := VerifyBundle(extracted, version)
	if err != nil {
		return ActiveBundle{}, err
	}

	destination := filepath.Join(r.versions, version)
	if _, err := os.Stat(destination); err == nil {
		same, compareErr := sameChecksums(extracted, destination)
		if compareErr != nil {
			return ActiveBundle{}, compareErr
		}
		if !same {
			return ActiveBundle{}, ErrVersionConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ActiveBundle{}, err
	} else if err := os.Rename(extracted, destination); err != nil {
		return ActiveBundle{}, err
	}

	bundle := ActiveBundle{
		Version:      version,
		Directory:    destination,
		DatabasePath: filepath.Join(destination, databaseFile),
		Manifest:     manifest,
	}
	if activate {
		if err := r.activate(bundle); err != nil {
			return ActiveBundle{}, err
		}
	}
	return bundle, nil
}

// VerifyBundle 校验目录中的三个标准文件及只读 Serving schema。
func VerifyBundle(directory string, expectedVersion string) (Manifest, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Manifest{}, err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return Manifest{}, fmt.Errorf("%w: directories are not allowed", ErrInvalidBundle)
		}
		if _, ok := requiredFiles[entry.Name()]; !ok {
			return Manifest{}, fmt.Errorf("%w: unexpected file %s", ErrInvalidBundle, entry.Name())
		}
		seen[entry.Name()] = struct{}{}
	}
	if len(seen) != len(requiredFiles) {
		return Manifest{}, fmt.Errorf("%w: bundle files are incomplete", ErrInvalidBundle)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(directory, manifestFile))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: invalid manifest: %v", ErrInvalidBundle, err)
	}
	if manifest.SchemaVersion != 1 || manifest.ModelVersion != expectedVersion || strings.TrimSpace(manifest.SelectedModel) == "" {
		return Manifest{}, fmt.Errorf("%w: manifest version or model is invalid", ErrInvalidBundle)
	}

	checksumBytes, err := os.ReadFile(filepath.Join(directory, checksumsFile))
	if err != nil {
		return Manifest{}, err
	}
	var checksums map[string]string
	if err := json.Unmarshal(checksumBytes, &checksums); err != nil {
		return Manifest{}, fmt.Errorf("%w: invalid checksums: %v", ErrInvalidBundle, err)
	}
	for _, name := range []string{manifestFile, databaseFile} {
		want, ok := checksums[name]
		if !ok || len(want) != len("sha256:")+64 || !strings.HasPrefix(want, "sha256:") {
			return Manifest{}, fmt.Errorf("%w: missing checksum for %s", ErrInvalidBundle, name)
		}
		got, err := sha256File(filepath.Join(directory, name))
		if err != nil {
			return Manifest{}, err
		}
		if !strings.EqualFold(got, want) {
			return Manifest{}, fmt.Errorf("%w: checksum mismatch for %s", ErrInvalidBundle, name)
		}
	}
	if err := verifyDatabase(filepath.Join(directory, databaseFile)); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (r *Registry) restoreActive() error {
	data, err := os.ReadFile(r.activeFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pointer struct {
		ModelVersion string `json:"model_version"`
	}
	if err := json.Unmarshal(data, &pointer); err != nil {
		return fmt.Errorf("invalid active model pointer: %w", err)
	}
	directory := filepath.Join(r.versions, pointer.ModelVersion)
	manifest, err := VerifyBundle(directory, pointer.ModelVersion)
	if err != nil {
		return fmt.Errorf("restore active model: %w", err)
	}
	r.active = &ActiveBundle{
		Version:      pointer.ModelVersion,
		Directory:    directory,
		DatabasePath: filepath.Join(directory, databaseFile),
		Manifest:     manifest,
	}
	return nil
}

func (r *Registry) activate(bundle ActiveBundle) error {
	pointer := struct {
		ModelVersion string `json:"model_version"`
		ActivatedAt  string `json:"activated_at"`
	}{bundle.Version, time.Now().UTC().Format(time.RFC3339Nano)}
	data, err := json.Marshal(pointer)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(r.root, ".active-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, r.activeFile); err != nil {
		return err
	}
	r.mu.Lock()
	r.active = &bundle
	r.mu.Unlock()
	return nil
}

func extractBundle(archivePath string, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: invalid zip: %v", ErrInvalidBundle, err)
	}
	defer reader.Close()
	if len(reader.File) != len(requiredFiles) {
		return fmt.Errorf("%w: zip must contain exactly three files", ErrInvalidBundle)
	}
	if err := os.Mkdir(destination, 0o750); err != nil {
		return err
	}
	for _, file := range reader.File {
		name := file.Name
		if filepath.Base(name) != name || strings.Contains(name, "\\") {
			return fmt.Errorf("%w: unsafe zip path", ErrInvalidBundle)
		}
		if _, ok := requiredFiles[name]; !ok || file.FileInfo().IsDir() {
			return fmt.Errorf("%w: unexpected zip entry %s", ErrInvalidBundle, name)
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(filepath.Join(destination, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func verifyDatabase(path string) error {
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer database.Close()
	var quickCheck string
	if err := database.QueryRow("PRAGMA quick_check").Scan(&quickCheck); err != nil {
		return fmt.Errorf("%w: sqlite quick_check failed: %v", ErrInvalidBundle, err)
	}
	if quickCheck != "ok" {
		return fmt.Errorf("%w: sqlite quick_check returned %s", ErrInvalidBundle, quickCheck)
	}
	queries := []string{
		"SELECT repo_id, full_name, description, primary_language, stars, forks FROM repositories LIMIT 0",
		"SELECT source_repo_id, target_repo_id, score, rank, model, signals FROM recommendations LIMIT 0",
	}
	for _, query := range queries {
		rows, err := database.Query(query)
		if err != nil {
			return fmt.Errorf("%w: serving schema mismatch: %v", ErrInvalidBundle, err)
		}
		rows.Close()
	}
	return nil
}

func sameChecksums(left string, right string) (bool, error) {
	leftData, err := os.ReadFile(filepath.Join(left, checksumsFile))
	if err != nil {
		return false, err
	}
	rightData, err := os.ReadFile(filepath.Join(right, checksumsFile))
	if err != nil {
		return false, err
	}
	return string(leftData) == string(rightData), nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
