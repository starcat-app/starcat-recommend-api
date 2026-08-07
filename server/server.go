// Package server 导出 recommend-api 的可装配 HTTP 服务。
//
// 单仓部署走 cmd/server；聚合部署（starcat-api）import 本包并挂到网关。
// 业务实现仍在 internal/，本包只负责 env 装配、路由注册与生命周期。
package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	kitenv "github.com/starcat-app/starcat-api-kit/env"
	"github.com/starcat-app/starcat-recommend-api/internal/handler"
	"github.com/starcat-app/starcat-recommend-api/internal/middleware"
	"github.com/starcat-app/starcat-recommend-api/internal/provider"
	"github.com/starcat-app/starcat-recommend-api/internal/version"
)

const (
	defaultPort            = "5005"
	defaultSimRepoEndpoint = "https://simrepo.dera.page/collections/repos/points/recommend"
)

// Options 控制 recommend 服务装配。聚合网关可显式传入，单仓部署通常用 FromEnv。
type Options struct {
	Port                   string
	APIKeys                []string
	SimRepoAPIKey          string
	SimRepoEndpoint        string
	CacheTTLSuccess        time.Duration
	CacheTTLEmpty          time.Duration
	CacheTTLError          time.Duration
	SkipListenLogEndpoints bool
}

// Service 是已装配的 recommend HTTP 服务。
type Service struct {
	opts     Options
	handler  http.Handler
	provider provider.Provider
}

// Name 返回聚合网关识别用的稳定服务名。
func Name() string { return "recommend" }

// DefaultPort 返回单仓默认监听端口。
func DefaultPort() string { return defaultPort }

// FromEnv 从环境变量装配服务（与历史 cmd/server 行为一致）。
func FromEnv() (*Service, error) {
	apiKeys, err := kitenv.RequiredCSV("API_KEYS")
	if err != nil {
		return nil, err
	}
	simRepoAPIKey, err := kitenv.LookupRequired("SIMREPO_API_KEY")
	if err != nil {
		return nil, err
	}
	opt := Options{
		Port:            kitenv.OrDefault("PORT", defaultPort),
		APIKeys:         apiKeys,
		SimRepoAPIKey:   simRepoAPIKey,
		SimRepoEndpoint: kitenv.OrDefault("SIMREPO_ENDPOINT", defaultSimRepoEndpoint),
		CacheTTLSuccess: kitenv.DurationSeconds("CACHE_TTL_SUCCESS_SECONDS", 7*24*time.Hour),
		CacheTTLEmpty:   kitenv.DurationSeconds("CACHE_TTL_EMPTY_SECONDS", time.Hour),
		CacheTTLError:   kitenv.DurationSeconds("CACHE_TTL_ERROR_SECONDS", 10*time.Minute),
	}
	return New(opt)
}

// New 按 Options 装配服务。
func New(opt Options) (*Service, error) {
	if strings.TrimSpace(opt.Port) == "" {
		opt.Port = defaultPort
	}
	if len(opt.APIKeys) == 0 {
		return nil, fmt.Errorf("APIKeys is required")
	}
	if strings.TrimSpace(opt.SimRepoAPIKey) == "" {
		return nil, fmt.Errorf("SimRepoAPIKey is required")
	}
	if strings.TrimSpace(opt.SimRepoEndpoint) == "" {
		opt.SimRepoEndpoint = defaultSimRepoEndpoint
	}
	if opt.CacheTTLSuccess <= 0 {
		opt.CacheTTLSuccess = 7 * 24 * time.Hour
	}
	if opt.CacheTTLEmpty <= 0 {
		opt.CacheTTLEmpty = time.Hour
	}
	if opt.CacheTTLError <= 0 {
		opt.CacheTTLError = 10 * time.Minute
	}

	baseProvider := provider.NewSimRepoProvider(opt.SimRepoEndpoint, opt.SimRepoAPIKey, nil)
	recommendProvider := provider.NewCachedProvider(
		baseProvider,
		opt.CacheTTLSuccess,
		opt.CacheTTLEmpty,
		opt.CacheTTLError,
	)

	authMW := middleware.NewBearerAuth(opt.APIKeys)
	recommendHandler := handler.NewRecommendHandler(recommendProvider)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.Handle("GET /api/v1/ping", authMW.Wrap(handler.HandlePingV1(version.Service, version.Version)))
	mux.Handle("GET /api/v1/repos/{repo_id}/recommendations", authMW.Wrap(http.HandlerFunc(recommendHandler.HandleRecommendations)))

	if !opt.SkipListenLogEndpoints {
		log.Printf("starcat-recommend-api %s endpoints ready", version.Version)
		log.Printf("  GET /api/v1/ping")
		log.Printf("  GET /api/v1/repos/{repo_id}/recommendations")
		log.Printf("  GET /healthz")
	}

	return &Service{
		opts:     opt,
		handler:  middleware.CORS(mux),
		provider: recommendProvider,
	}, nil
}

// Handler 返回已包 CORS 的根 handler，可供聚合网关挂载。
func (s *Service) Handler() http.Handler { return s.handler }

// Addr 返回建议监听地址（":port"）。
func (s *Service) Addr() string { return ":" + s.opts.Port }

// Close 释放资源（recommend 当前无持久连接，预留接口）。
func (s *Service) Close() error { return nil }

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func lookupRequiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s env is required", key)
	}
	return value, nil
}

func requiredListEnv(key string) ([]string, error) {
	raw, err := lookupRequiredEnv(key)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s env is required", key)
	}
	return out, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		log.Printf("[env] invalid %s=%q, using fallback %s", key, value, fallback)
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
