# starcat-recommend-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-pro/main/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>Self-hostable support API for Starcat similar-repository recommendations.</strong></p>
<p>Starcat is a native macOS app that turns GitHub Stars into a searchable, organized and AI-assisted local knowledge base, with a broader ecosystem of desktop clients, plugins, CLI tools, and self-hostable services.</p>

<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/Install%20with-Homebrew-FBBF24?style=for-the-badge&logo=homebrew&logoColor=white" width="220" alt="Install with Homebrew"/></a>
<br/>
<sub><a href="./README-ZH.md">中文说明</a></sub>
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

**Preferred install method:**

```bash
brew tap starcat-app/starcat
brew trust starcat-app/starcat
brew install --cask starcat
```

**Useful links:**

- Home and downloads: https://starcat.ink
- Mac App Store: search for Starcat for GitHub
- Public support and release notes: https://github.com/starcat-app/starcat-pro
- Starcat App Homebrew tap: https://github.com/starcat-app/homebrew-starcat
- CLI / MCP: [starcat-cli](https://github.com/starcat-app/starcat-cli) / [Homebrew tap](https://github.com/starcat-app/homebrew-starcat-cli)
- AI Agent Skill: https://github.com/starcat-app/starcat-skill
- Browser plugins: [Chrome](https://github.com/starcat-app/starcat-chrome-plugin) / [Safari](https://github.com/starcat-app/starcat-safari-plugin)
- Launcher integrations: [Alfred](https://github.com/starcat-app/starcat-alfred-workflow) / [uTools](https://github.com/starcat-app/starcat-utools-plugin) / [Raycast](https://github.com/starcat-app/starcat-raycast-extension)
- Documentation: https://github.com/starcat-app/starcat-docs
- Website source: https://github.com/starcat-app/starcat-site
- Localization: https://github.com/starcat-app/starcat-localization

**Self-hostable support APIs:**

- [starcat-sharing-api](https://github.com/starcat-app/starcat-sharing-api)
- [starcat-trending-api](https://github.com/starcat-app/starcat-trending-api)
- [starcat-weekly-api](https://github.com/starcat-app/starcat-weekly-api)
- [starcat-wiki-api](https://github.com/starcat-app/starcat-wiki-api)
- [starcat-recommend-api](https://github.com/starcat-app/starcat-recommend-api)
- [starcat-discovery-api](https://github.com/starcat-app/starcat-discovery-api)

> Starcat provides hosted defaults for normal users. This API is open source so advanced users can inspect it, run it locally, or deploy their own instance.
<!-- starcat-promo:end -->

Backend service for Starcat similar-repository recommendations.

This service is Starcat's long-term recommendation entry point. `/api/v1` keeps the existing SimRepo-backed contract, while `/api/v2` reads immutable ServingBundles published by `starcat-recsys-trainer`. Clients hold neither the SimRepo key nor the model publishing key.

## Endpoints

| Method | Path | Auth | Description |
|---|---|---:|---|
| `GET` | `/healthz` | No | Process health check |
| `GET` | `/api/v1/ping` | Yes | Starcat client connectivity probe |
| `GET` | `/api/v1/repos/{repo_id}/recommendations?limit=10&offset=0` | Yes | Similar repository recommendations |
| `GET` | `/api/v2/repos/{repo_id}/recommendations?limit=10&offset=0` | Client key | Self-trained single-repository recommendations |
| `POST` | `/api/v2/recommendations/query` | Client key | Self-trained multi-seed recommendations |
| `POST` | `/internal/v1/model-bundles/{model_version}?activate=true` | Publish key | Publish and activate a ServingBundle |
| `GET` | `/internal/v1/model-bundles/active` | Publish key | Read the active model version |

The v2 single-repository endpoint returns an `ETag` derived from the active immutable model version and page parameters, together with `Cache-Control: private, no-cache`. Clients may send that value through `If-None-Match`: an unchanged model returns `304 Not Modified` without querying or transferring the recommendation page, while activating a new model returns `200` with the new bundle version and data.

The authenticated ping response includes the service identity and the build version injected from the release tag:

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
- `CACHE_TTL_EMPTY_SECONDS`: defaults to 1 hour.
- `CACHE_TTL_ERROR_SECONDS`: defaults to 10 minutes.
- `MODEL_PUBLISH_KEYS`: comma-separated Trainer publishing keys. Internal publish routes are disabled when omitted.
- `MODEL_REGISTRY_DIR`: immutable Bundle registry, defaults to `./data/model-registry`.
- `METRICS_STORE_FILE`: dedicated request metrics SQLite, defaults to `./data/recommend-metrics.db`.
- `MAX_BUNDLE_BYTES`: compressed Bundle limit, defaults to 512 MiB.

## Operations and Metrics

- `GET /internal/stats`: process-local v1 cache counters and active v2 ServingBundle scale/metadata.
- `GET /internal/metrics/{summary,timeseries,routes,status-codes}`: aggregate route traffic, errors, and latency.

Both endpoints use the Service API Key. Bundle publication remains on the isolated publish-key routes. Metrics exclude credentials and raw requests.

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
TrainedRecommendHandler -> TrainedProvider -> active ServingBundle SQLite
```

`CachedProvider` keeps at most 10,000 `repoID:limit:offset` entries. Expired entries are removed on read; when capacity is reached, the entry with the earliest expiry is evicted.

`TrainedProvider` reuses a read-only SQLite connection pool per immutable model version. An active-version switch does not interrupt requests already using the previous bundle; the old pool entry is closed after its final reference is released, and all remaining connections are closed with the service.

Single-repository v2 items preserve the raw ranking `score` and may additionally expose `display_score`, the global percentile calibrated by the immutable ServingBundle. Legacy bundles omit it safely. Multi-seed queries omit `display_score` because weighted combined scores do not follow the single-edge calibration distribution.

Future providers should keep the response DTO stable:

```text
ContentEmbeddingProvider
StarcatBehaviorProvider
HybridProvider
```
