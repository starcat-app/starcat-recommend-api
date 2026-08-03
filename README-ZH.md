# starcat-recommend-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>这是 Starcat 相似仓库推荐的可自部署支撑服务。</strong></p>
<p>Starcat 是一款原生 macOS 应用，可以把 GitHub Stars 变成可搜索、可整理、可用 AI 追问的本地知识库。当前 1.3.0 支持 README 渲染、知识库 RAG、我的项目、全局与仓库洞察、macOS 桌面小组件、标签与私有笔记、Release 追踪、仓库健康度、AI 摘要、语义搜索、浏览器插件，以及 Alfred / uTools / Raycast 外部搜索，并提供多个可自部署 API。</p>

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
- 公开支持与发布说明: https://github.com/starcat-app/starcat-pro
- Starcat App Homebrew tap: https://github.com/starcat-app/homebrew-starcat
- CLI / MCP: [starcat-cli](https://github.com/starcat-app/starcat-cli) / [Homebrew tap](https://github.com/starcat-app/homebrew-starcat-cli)
- AI Agent Skill: https://github.com/starcat-app/starcat-skill
- 浏览器插件: [Chrome](https://github.com/starcat-app/starcat-chrome-plugin) / [Safari](https://github.com/starcat-app/starcat-safari-plugin)
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

第一版通过服务端中转 SimRepo 的非官方 Qdrant Recommend API, 为 Starcat macOS 客户端提供稳定的 `/api/v1/repos/{repo_id}/recommendations` 契约。客户端不直连 SimRepo, 不持有 SimRepo key, 后续可以在本服务内替换为 Starcat 自研推荐 Provider。

## Endpoints

| Method | Path | Auth | Description |
|---|---|---:|---|
| `GET` | `/healthz` | No | Process health check |
| `GET` | `/api/v1/ping` | Yes | Starcat client connectivity probe |
| `GET` | `/api/v1/repos/{repo_id}/recommendations?limit=10&offset=0` | Yes | Similar repository recommendations |

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
```

## Quality Gates

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## Provider Boundary

The current provider chain is:

```text
RecommendHandler -> CachedProvider -> SimRepoProvider -> SimRepo Qdrant API
```

`CachedProvider` 最多保留 10000 个 `repoID:limit:offset` 组合键。读取时删除过期项；容量已满时淘汰最早到期的条目。

Future providers should keep the response DTO stable:

```text
ContentEmbeddingProvider
StarcatBehaviorProvider
HybridProvider
```
