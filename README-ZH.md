# starcat-recommend-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>这是 Starcat 相似仓库推荐的可自部署支撑服务。</strong></p>
<p>Starcat 是一款原生 macOS 应用，可以把 GitHub Stars 变成可搜索、可整理、可用 AI 追问的本地知识库。当前 1.4.0 支持 README 渲染、知识库 RAG、GitHub 通知、我的项目、全局与仓库洞察、macOS 桌面小组件、标签与私有笔记、Release 追踪、仓库健康度、AI 摘要、语义搜索、浏览器插件，以及 Alfred / uTools / Raycast 外部搜索，并提供多个可自部署 API。</p>

<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/Install%20with-Homebrew-FBBF24?style=for-the-badge&logo=homebrew&logoColor=white" width="220" alt="Install with Homebrew"/></a>
<br/>
<sub><a href="./README.md">English</a></sub>
</div>

<div align="center">
<a href="https://starcat.ink"><img src="https://img.shields.io/badge/website-starcat.ink-38BDF8?style=flat&color=blue" alt="website"/></a>
<a href="https://github.com/starcat-app/starcat-pro"><img src="https://img.shields.io/badge/support-starcat--pro-lightgrey.svg?style=flat&color=blue" alt="support"/></a>
<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/install-homebrew-lightgrey.svg?style=flat&color=blue" alt="homebrew"/></a>
<a href="https://github.com/starcat-app/starcat-localization"><img src="https://img.shields.io/badge/localization-open-lightgrey.svg?style=flat&color=blue" alt="localization"/></a>
</div>

<div align="center">
<img width="900" src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/main.webp" alt="Starcat main window"/>
</div>

**首选 Homebrew 安装：**

```bash
brew tap starcat-app/starcat
brew trust starcat-app/starcat
brew install --cask starcat
```

**相关链接：**

- 官网与下载: https://starcat.ink
- Mac App Store: 搜索 Starcat for GitHub
- 当前 Direct 版本: https://starcat.ink/downloads/Starcat-1.4.0-arm64.dmg
- 公开支持与发布说明: https://github.com/starcat-app/starcat-pro
- Starcat App Homebrew tap: https://github.com/starcat-app/homebrew-starcat
- CLI / MCP: [starcat-cli](https://github.com/starcat-app/starcat-cli) / [Homebrew tap](https://github.com/starcat-app/homebrew-starcat-cli)
- AI Agent Skill: https://github.com/starcat-app/starcat-skill
- 浏览器插件: [Chrome](https://github.com/starcat-app/starcat-chrome-plugin) / [Safari](https://github.com/starcat-app/starcat-safari-plugin)
- 启动器集成: [Alfred](https://github.com/starcat-app/starcat-alfred-workflow) / [uTools](https://github.com/starcat-app/starcat-utools-plugin) / [Raycast](https://github.com/starcat-app/starcat-raycast-extension)
- 官方文档: https://github.com/starcat-app/starcat-docs
- 官网源码: https://github.com/starcat-app/starcat-site
- 本地化: https://github.com/starcat-app/starcat-localization

**可自部署支撑 API：**

- [starcat-sharing-api](https://github.com/starcat-app/starcat-sharing-api)
- [starcat-trending-api](https://github.com/starcat-app/starcat-trending-api)
- [starcat-weekly-api](https://github.com/starcat-app/starcat-weekly-api)
- [starcat-wiki-api](https://github.com/starcat-app/starcat-wiki-api)
- [starcat-recommend-api](https://github.com/starcat-app/starcat-recommend-api)
- [starcat-discovery-api](https://github.com/starcat-app/starcat-discovery-api)

> Starcat 为普通用户提供默认托管服务。这个 API 开源出来，是为了让进阶用户可以审查实现、本地运行，或部署自己的实例。
<!-- starcat-promo:end -->

Starcat 相似仓库推荐后端。

本服务是 Starcat 长期统一推荐入口。`/api/v1` 继续中转 SimRepo 的非官方 Qdrant Recommend API，保持现有客户端和第三方契约不变；`/api/v2` 只读 `starcat-recsys-trainer` 发布的自研 ServingBundle。客户端不直连 SimRepo，也不持有 SimRepo 或模型发布密钥。

## Endpoints

| Method | Path | Auth | Description |
|---|---|---:|---|
| `GET` | `/healthz` | No | Process health check |
| `GET` | `/api/v1/ping` | Yes | Starcat client connectivity probe |
| `GET` | `/api/v1/repos/{repo_id}/recommendations?limit=10&offset=0` | Yes | Similar repository recommendations |
| `GET` | `/api/v2/repos/{repo_id}/recommendations?limit=10&offset=0` | Client key | 自研单仓推荐 |
| `POST` | `/api/v2/recommendations/query` | Client key | 自研多 seed 推荐 |
| `POST` | `/internal/v1/model-bundles/{model_version}?activate=true` | Publish key | Trainer 发布并激活 Bundle |
| `GET` | `/internal/v1/model-bundles/active` | Publish key | 查询 active 模型 |

v2 单仓接口会根据当前不可变模型版本与分页参数返回 `ETag`，并设置 `Cache-Control: private, no-cache`。客户端可通过 `If-None-Match` 重验证：模型未变化时直接返回 `304 Not Modified`，不查询或传输推荐列表；激活新模型后返回 `200` 及新版本数据。

鉴权后的 ping 响应包含服务标识，以及由发布 tag 注入的构建版本：

```json
{"schema_version":1,"data":{"service":"recommend","version":"1.2.3","ok":true}}
```

## Environment

```bash
cp .env.example .env
```

Required:

- `API_KEYS`: comma-separated Bearer tokens accepted by Starcat clients.
- `SIMREPO_API_KEY`: SimRepo Qdrant read-only key. Keep it server-side only.

Optional:

- `PORT`: defaults to `5005`.
- `SIMREPO_ENDPOINT`: defaults to SimRepo's Qdrant recommend endpoint.
- `CACHE_TTL_SUCCESS_SECONDS`: defaults to 7 days.
- `CACHE_TTL_EMPTY_SECONDS`：默认 1 小时。
- `CACHE_TTL_ERROR_SECONDS`: defaults to 10 minutes.
- `MODEL_PUBLISH_KEYS`：逗号分隔的 Trainer 发布密钥；未配置时不注册内部发布路由。
- `MODEL_REGISTRY_DIR`：不可变 Bundle Registry，默认 `./data/model-registry`。
- `METRICS_STORE_FILE`：独立请求指标 SQLite，默认 `./data/recommend-metrics.db`。
- `MAX_BUNDLE_BYTES`：压缩 Bundle 上限，默认 512 MiB。

## 运营与调用指标

- `GET /internal/stats`：进程内 v1 缓存计数，以及当前 v2 ServingBundle 规模和元数据。
- `GET /internal/metrics/{summary,timeseries,routes,status-codes}`：路由调用量、错误与延迟聚合。

两类接口均使用 Service API Key；模型发布继续使用隔离的 Publish Key。指标不保存凭据或原始请求。

## Local Development

```bash
go mod tidy
go run ./cmd/server
```

Smoke test:

```bash
curl http://127.0.0.1:5005/healthz
curl -H "Authorization: Bearer $API_KEY" http://127.0.0.1:5005/api/v1/ping
curl -H "Authorization: Bearer $API_KEY" \
  "http://127.0.0.1:5005/api/v1/repos/41881900/recommendations?limit=10&offset=0"
curl -H "Authorization: Bearer $API_KEY" \
  "http://127.0.0.1:5005/api/v2/repos/41881900/recommendations?limit=10&offset=0"
```

Trainer 发布包只允许包含 `recommendations.sqlite`、`manifest.json` 和 `checksums.json`：

```bash
curl -X POST \
  -H "Authorization: Bearer $MODEL_PUBLISH_KEY" \
  -H "Content-Type: application/zip" \
  --data-binary @serving-bundle.zip \
  "http://127.0.0.1:5005/internal/v1/model-bundles/costar-20260824?activate=true"
```

服务端只有在文件白名单、manifest 版本、checksum、SQLite `quick_check` 和必需表全部通过后才安装并原子更新 `active.json`。同一版本内容不同会返回 `409`，失败发布不会改变当前 active 模型。

## Quality Gates

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## Provider Boundary

当前两条 Provider 链互不覆盖：

```text
RecommendHandler -> CachedProvider -> SimRepoProvider -> SimRepo Qdrant API
TrainedRecommendHandler -> TrainedProvider -> active ServingBundle SQLite
```

`CachedProvider` 最多保留 10000 个 `repoID:limit:offset` 组合键。读取时删除过期项；容量已满时淘汰最早到期的条目。

自研响应复用现有推荐卡片字段，并额外返回 `model_version` 与 `signals`。训练、Collection 原始快照和在线查询保持隔离，Recommend API 不读取 `participant_id`。

Future providers should keep the response DTO stable:

```text
ContentEmbeddingProvider
StarcatBehaviorProvider
HybridProvider
```
