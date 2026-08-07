# Changelog

## 2.0.0 - 2026-08-07

### Changed
- 导出可装配 `server` 包，供 `starcat-api` 聚合与独立部署共用。
- 依赖 `starcat-api-kit`：`/api/v1/ping` 改用 `httputil.HandlePingV1`；`FromEnv` 改用 `env` 解析。

## 0.1.0 - 2026-06-28

- Initial recommend API service.
- Add `/healthz`, `/api/v1/ping`, and `/api/v1/repos/{repo_id}/recommendations`.
- Add Bearer API key auth, SimRepo provider, and in-process TTL cache.
