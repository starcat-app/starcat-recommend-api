# AGENTS.md — starcat-recommend-api

> **唯一协作规范源**：本仓库根目录 `AGENTS.md` 是本项目协作规范的唯一正文维护源。
> 开工前还必须阅读并遵守上级 [`../AGENTS.md`](../AGENTS.md) 的跨仓规则。

## 项目概述

Starcat 统一推荐后端：`/api/v1` 中转 SimRepo 非官方 Qdrant Recommend API（保持现有客户端契约）；`/api/v2` 只读 `starcat-recsys-trainer` 发布的自研 ServingBundle。客户端不直连 SimRepo，也不持有 SimRepo 或模型发布密钥。生产经 `starcat-api` 聚合部署。

## 技术栈

- Go 1.25.0 · `net/http`
- `modernc.org/sqlite`（metrics 与 model registry）
- `github.com/starcat-app/starcat-api-kit` v0.3.0
- `github.com/joho/godotenv`

## 关键目录

```
cmd/server/
server/               # 可导出装配
internal/handler/     # v1 recommend + v2 query
internal/provider/    # simrepo、trained、bundle_database_pool
internal/serving/     # Bundle registry 与 stats
Makefile              # VERSION := 0.1.0
```

## 开发与测试命令

```bash
cp .env.example .env          # API_KEYS、SIMREPO_API_KEY 必填
go mod tidy && make run       # PORT=5005
make build
make check                    # fmt-check + vet + test（PR 前）
make docker-build
```

CI（`.github/workflows/go.yml`）：gofmt · vet · docker build · test -race · build。

Smoke test（README）：
```bash
curl http://127.0.0.1:5005/healthz
curl -H "Authorization: Bearer $API_KEY" http://127.0.0.1:5005/api/v1/ping
```

环境变量见 `.env.example`：`API_KEYS`、`SIMREPO_API_KEY`、`SIMREPO_ENDPOINT`、缓存 TTL、`MODEL_PUBLISH_KEYS`、`MODEL_REGISTRY_DIR`、`METRICS_STORE_FILE`、`MAX_BUNDLE_BYTES`（默认 512 MiB）。

## 代码与架构约束

- **密钥隔离**：`API_KEYS`（客户端）与 `MODEL_PUBLISH_KEYS`（Trainer 发布）必须分离；SimRepo Key 仅服务端。
- **v2 ETag**：单仓推荐根据模型版本与分页返回 `ETag`；支持 `If-None-Match` → 304。
- **内部端点**：`POST /internal/v1/model-bundles/{version}` 用 `MODEL_PUBLISH_KEYS`；`GET /internal/stats`、`/internal/metrics/*` 用 `API_KEYS`。
- **响应契约**：envelope + Bearer 鉴权；`/healthz` 公开。
- 改 provider 逻辑后跑 `make check`，注意 `server/trained_test.go` 等集成测试。

## 安全与数据边界

- 禁止入库：`.env`、`data/model-registry/` 运行时内容、`bin/`、`coverage.out`。
- 禁止向 macOS 客户端或日志泄露 `SIMREPO_API_KEY`、`MODEL_PUBLISH_KEYS`。
- Bundle 文件为 Trainer 发布产物，非 git 管理。

## 部署与发布禁令

未经 dong4j 明确授权，禁止：`make release`、`scripts/deploy.sh`、`fly deploy`、`git push`/`git tag`、向生产 registry 写入 Bundle。生产 Fly 仅经 `starcat-api`。
