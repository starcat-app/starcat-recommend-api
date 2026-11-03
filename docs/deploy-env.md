# Deploy Environment

Required query secret (standalone service):

```bash
fly secrets set API_KEYS="sk-starcat-..."
```

Publishing and storage settings:

```bash
fly secrets set MODEL_PUBLISH_KEYS="a-separate-publish-key"
fly secrets set MODEL_REGISTRY_DIR="/data/recommend-model-registry"
fly secrets set METRICS_STORE_FILE="/data/recommend-metrics.db"
```

Mount persistent storage at `/data` and publish/activate a validated ServingBundle before accepting recommendation traffic. Omitting `MODEL_PUBLISH_KEYS` disables publishing; it does not disable queries against an existing active model. Optional `GITHUB_TOKENS` resolves public out-of-catalog source metadata.

Production uses the aggregate `starcat-api`: prefix these variables with `RECOMMEND_` and send `X-SC-Svc: recommend`. Both recommendation routes use the same trained provider, with no third-party credentials or page TTL cache. Remove the v1 recommendation adapter after the next App release containing the dual-channel v2 switch.
