# Deploy to Render

Use the Dockerfile at repository root.

Environment variables:

- `PORT=5005`
- `API_KEYS`
- `MODEL_REGISTRY_DIR=/data/recommend-model-registry`
- `METRICS_STORE_FILE=/data/recommend-metrics.db`
- `MODEL_PUBLISH_KEYS` optional, separate from query keys
- `GITHUB_TOKENS` optional, for public out-of-catalog metadata

Attach a persistent disk at `/data`. Publish and activate a validated ServingBundle before checking `/api/v2/repos/{repo_id}/recommendations` with a query key. `/healthz` alone does not verify model availability. Both v1 and v2 recommendation routes use the trained provider; third-party recommendation configuration is no longer supported.

Health check path:

```text
/healthz
```
