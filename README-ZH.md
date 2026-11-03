# starcat-recommend-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>这是 Starcat 相似仓库推荐的可自部署支撑服务。</strong></p>
<p>Starcat 是一款原生 macOS 应用，可以把 GitHub Stars 变成可搜索、可整理、可用 AI 追问的本地知识库，并通过桌面客户端、插件、CLI 与可自部署服务组成完整生态。</p>

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

推荐接口统一读取 `starcat-recsys-trainer` 发布的自研 ServingBundle。`/api/v1` 暂时保留已发布客户端契约；当前 App Store / Direct 源码统一使用 `/api/v2`。不再使用第三方推荐 Provider 或密钥；客户端不持有模型发布密钥。

**下一次包含双渠道 v2 切换的 Starcat App 版本上线之后，删除 v1 推荐兼容层**；仅部署后端不触发删除。具体清理范围见 [TODO.md](TODO.md)。

部署与验收证据见[自研推荐切换与 v1 退役](docs/自研推荐切换与v1退役.md)。

## Endpoints

| Method | Path | Auth | Description |
|---|---|---:|---|
| `GET` | `/healthz` | No | Process health check |
| `GET` | `/api/v1/ping` | Yes | Starcat client connectivity probe |
| `GET` | `/api/v1/repos/{repo_id}/recommendations?limit=10&offset=0` | Yes | 同一自研模型的临时旧客户端适配入口 |
| `GET` | `/api/v2/repos/{repo_id}/recommendations?limit=10&offset=0` | Client key | 自研单仓推荐 |
| `POST` | `/api/v2/recommendations/query` | Client key | 自研多 seed 推荐 |
| `POST` | `/internal/v1/model-bundles/{model_version}?activate=true` | Publish key | Trainer 发布并激活 Bundle |
| `GET` | `/internal/v1/model-bundles/active` | Publish key | 查询 active 模型 |

v2 单仓接口根据实际模型版本与分页参数返回 `ETag`，并设置 `Cache-Control: private, no-cache`。源仓库没有预计算推荐边时，包含已训练内容索引的 Bundle 可用公开名称、描述、topics 和语言检索近邻。源元数据摘要追加到 `model_version`，避免内容变化后错误返回 304 或混合不同修订的分页。条件请求在确定实际快照身份后返回 `304 Not Modified`。

原有预计算推荐保持排序不变。内容补充采用 TF-IDF 余弦相似度，最低分数为 0.08，排除源仓库，不伪造全局校准的 `display_score`。这是词法检索，不是多语言 Embedding 模型。v2 多 seed 查询继续使用预计算边；本次内容补充仅用于单仓接口。

配置 `GITHUB_TOKENS` 后，目录外源仓库通过数值 GitHub 仓库 ID 获取公开元数据；私有、禁用、归档、缺失及身份不符的项目在构造向量前拒绝。公开元数据缓存七天，不可用来源缓存十五分钟，最多 4,096 条；同 ID 请求合并、上游并发为二、请求间隔 720ms 且可取消、总预算八秒，失败退避一分钟。瞬时错误保留为可重试错误，不缓存成空推荐；不发送用户 Stars 列表或训练主体。

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

Optional:

- `PORT`: defaults to `5005`.
- `MODEL_PUBLISH_KEYS`：逗号分隔的 Trainer 发布密钥；未配置时不注册内部发布路由。
- `MODEL_REGISTRY_DIR`：不可变 Bundle Registry，默认 `./data/model-registry`。
- `METRICS_STORE_FILE`：独立请求指标 SQLite，默认 `./data/recommend-metrics.db`。
- `MAX_BUNDLE_BYTES`：压缩 Bundle 上限，默认 512 MiB。
- `GITHUB_TOKENS`：可选服务端 PAT 池，用于解析目录外公开源仓库；聚合部署使用 `RECOMMEND_GITHUB_TOKENS`。未配置时，Bundle 目录内源仓库仍可使用内容索引。

## 运营与调用指标

- `GET /internal/stats`：当前自研 ServingBundle 规模和元数据；v1 入口标明 `source=starcat_trained`、`compatibility_only=true`、`cache_enabled=false`。
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
  "http://127.0.0.1:5005/api/v2/repos/41881900/recommendations?limit=10&offset=0"
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

两个入口共用同一个 Provider：

```text
RecommendHandler（临时 v1 契约适配）-> TrainedProvider -> active ServingBundle SQLite
TrainedRecommendHandler（v2）    -> TrainedProvider -> active ServingBundle SQLite
```

不再维护隐藏模型切换的独立 v1 页面 TTL 缓存。v1 保留原参数上限、分页和错误契约，并将校准分数适配到旧客户端的 0～1 展示字段，不改变排序。公开元数据缓存与客户端 v2 版本重验证继续保留。

自研响应复用现有推荐卡片字段，并额外返回 `model_version` 与 `signals`。训练、Collection 原始快照和在线查询保持隔离，Recommend API 不读取 `participant_id`。

Future providers should keep the response DTO stable:

```text
ContentEmbeddingProvider
StarcatBehaviorProvider
HybridProvider
```
