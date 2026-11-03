# Deploy to Fly.io

Production is deployed inside `starcat-api`; its Docker context includes sibling API repositories. The following is for standalone self-hosting:

```bash
fly launch --no-deploy
fly secrets set API_KEYS="sk-starcat-..."
fly volumes create recommend_data --region nrt --size 1
fly secrets set MODEL_PUBLISH_KEYS="a-separate-publish-key"
fly secrets set MODEL_REGISTRY_DIR="/data/recommend-model-registry" METRICS_STORE_FILE="/data/recommend-metrics.db"
fly deploy
```

Configure the volume mount at `/data` in `fly.toml` before deploying. Publish and activate a validated ServingBundle using the separate publishing key. A healthy process without an active bundle cannot serve recommendations; there is no third-party fallback.

Health and authenticated business checks:

```bash
curl https://starcat-recommend-api.fly.dev/healthz
curl -H "Authorization: Bearer $API_KEY" \
  "https://starcat-recommend-api.fly.dev/api/v2/repos/41881900/recommendations?limit=5"
```

For aggregate production, use `https://starcat-api.fly.dev`, the `X-SC-Svc: recommend` header and `RECOMMEND_` configuration prefixes. Keep the existing persistent registry during service upgrades.
