# starcat-recommend-api

<!-- starcat-promo:start -->
<div align="center">
<a href="https://starcat.ink"><img src="https://raw.githubusercontent.com/starcat-app/starcat-site/main/assets/banner.webp" width="100%" alt="Starcat" /></a>

<p><strong>Self-hostable support API for Starcat similar-repository recommendations.</strong></p>
<p>Starcat is a native macOS app that turns GitHub Stars into a searchable, organized and AI-assisted local knowledge base, with a broader ecosystem of desktop clients, plugins, CLI tools, and self-hostable services.</p>

<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/Install%20with-Homebrew-FBBF24?style=for-the-badge&logo=homebrew&logoColor=white" width="220" alt="Install with Homebrew"/></a>
<br/>
<sub><a href="./README-ZH.md">中文说明</a></sub>
</div>

<div align="center">
<a href="https://starcat.ink"><img src="https://img.shields.io/badge/website-starcat.ink-38BDF8?style=flat&color=blue" alt="website"/></a>
<a href="https://github.com/starcat-app/Starcat/issues"><img src="https://img.shields.io/github/issues/starcat-app/Starcat?style=flat&color=blue" alt="issues"/></a>
<a href="https://github.com/starcat-app/homebrew-starcat"><img src="https://img.shields.io/badge/install-homebrew-lightgrey.svg?style=flat&color=blue" alt="homebrew"/></a>
<a href="https://github.com/starcat-app/starcat-localization"><img src="https://img.shields.io/badge/localization-open-lightgrey.svg?style=flat&color=blue" alt="localization"/></a>
</div>

<div align="center">
<img width="900" src="https://raw.githubusercontent.com/starcat-app/starcat-site/main/assets/main.webp" alt="Starcat main window"/>
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
- Public support: https://github.com/starcat-app/Starcat/issues · Release notes: https://starcat.ink/changelog.html
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

Both recommendation routes read immutable ServingBundles published by `starcat-recsys-trainer`. `/api/v1` temporarily preserves the released client contract; current App Store and Direct source code uses `/api/v2`. No third-party recommendation provider or key is used. Clients never hold the model publishing key.

Remove the v1 recommendation adapter **after the next Starcat App version containing the dual-channel v2 switch is released**, not after a backend deployment alone. The exact removal scope is recorded in [TODO.md](TODO.md).

Cutover evidence and removal checklist: [self-trained recommendation cutover](docs/自研推荐切换与v1退役.md).

## Endpoints

| Method | Path | Auth | Description |
|---|---|---:|---|
| `GET` | `/healthz` | No | Process health check |
| `GET` | `/api/v1/ping` | Yes | Starcat client connectivity probe |
| `GET` | `/api/v1/repos/{repo_id}/recommendations?limit=10&offset=0` | Yes | Temporary legacy adapter over the same trained model |
| `GET` | `/api/v2/repos/{repo_id}/recommendations?limit=10&offset=0` | Client key | Self-trained single-repository recommendations |
| `POST` | `/api/v2/recommendations/query` | Client key | Self-trained multi-seed recommendations |
| `POST` | `/internal/v1/model-bundles/{model_version}?activate=true` | Publish key | Publish and activate a ServingBundle |
| `GET` | `/internal/v1/model-bundles/active` | Publish key | Read the active model version |

The v2 single-repository endpoint returns an `ETag` derived from the effective model version and page parameters, together with `Cache-Control: private, no-cache`. For a source without precomputed edges, bundles containing a trained content index can retrieve neighbors using public name, description, topics and language. A source metadata digest is appended to `model_version`, so changed metadata cannot incorrectly return 304 or mix pagination across revisions. Conditional requests return `304 Not Modified` after fixing the effective snapshot identity.

Existing precomputed recommendations retain their ordering. Content fallback uses TF-IDF cosine similarity with a minimum score of 0.08, excludes the source, and does not fabricate globally calibrated `display_score` values. This is lexical retrieval, not a multilingual embedding model. Multi-seed v2 queries continue to use precomputed edges; this fallback applies to single-repository queries.

With `GITHUB_TOKENS` configured, an out-of-catalog source is resolved by numeric GitHub repository ID. Private, disabled, archived, missing and mismatched repositories are rejected before vector construction. Public source metadata is cached for seven days (unavailable sources: 15 minutes), bounded to 4,096 entries, with same-ID request coalescing, two upstream slots, a cancellable 720 ms request interval, an eight-second budget and one-minute failure backoff. Transient failures remain retryable errors, not cached empty recommendations. No user Star lists or training subjects enter this path.

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

Optional:

- `PORT`: defaults to `5005`.
- `MODEL_PUBLISH_KEYS`: comma-separated Trainer publishing keys. Internal publish routes are disabled when omitted.
- `MODEL_REGISTRY_DIR`: immutable Bundle registry, defaults to `./data/model-registry`.
- `METRICS_STORE_FILE`: dedicated request metrics SQLite, defaults to `./data/recommend-metrics.db`.
- `MAX_BUNDLE_BYTES`: compressed Bundle limit, defaults to 512 MiB.
- `GITHUB_TOKENS`: optional server-side PAT pool for resolving public out-of-catalog sources; use `RECOMMEND_GITHUB_TOKENS` in the aggregate deployment. Without it, content retrieval still works for sources already present in the bundle.

## Operations and Metrics

- `GET /internal/stats`: active model metadata and bundle size; the v1 alias reports `source=starcat_trained`, `compatibility_only=true` and `cache_enabled=false`.
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
  "http://127.0.0.1:5005/api/v2/repos/41881900/recommendations?limit=10&offset=0"
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
RecommendHandler (temporary v1 contract adapter) -> TrainedProvider -> active ServingBundle SQLite
TrainedRecommendHandler (v2)                    -> TrainedProvider -> active ServingBundle SQLite
```

Both handlers share one provider. There is no separate v1 page TTL cache hiding model activation. The v1 adapter keeps legacy parameter limits, pagination and error responses, and maps calibrated scores into the old 0–1 display field without changing ranking. Public metadata caching and client v2 version revalidation remain active.

`TrainedProvider` reuses a read-only SQLite connection pool per immutable model version. An active-version switch does not interrupt requests already using the previous bundle; the old pool entry is closed after its final reference is released, and all remaining connections are closed with the service.

Single-repository v2 items preserve the raw ranking `score` and may additionally expose `display_score`, the global percentile calibrated by the immutable ServingBundle. Legacy bundles omit it safely. Multi-seed queries omit `display_score` because weighted combined scores do not follow the single-edge calibration distribution.

Future providers should keep the response DTO stable:

```text
ContentEmbeddingProvider
StarcatBehaviorProvider
HybridProvider
```
