# Self-hosting with pretrained models

[中文](SELF_HOSTING-ZH.md)

The service runs independently. Serving a pretrained model requires no Python, GPU,
Starcat account, GH Archive raw data or maintainer training directories.

## Docker Compose

Install Docker Compose, curl, jq, and either `sha256sum` or macOS `shasum`:

```bash
git clone https://github.com/starcat-app/starcat-recommend-api.git
cd starcat-recommend-api
cp .env.example .env
chmod 600 .env
```

Set `API_KEYS` and `MODEL_PUBLISH_KEYS` in `.env` to two different random long keys.
Generate each with `openssl rand -hex 32`, for example. Clients use the query key;
only operators and trainers receive the publishing key. `GITHUB_TOKENS` is optional:
recommendations and content retrieval for in-catalog sources work without it.

```bash
docker compose up -d --build
curl --fail http://127.0.0.1:5005/healthz
```

The port binds to localhost by default. The `recommend-data` named volume persists
models and request metrics. `docker compose down` retains it; `down -v` deletes it.
Configure an HTTPS reverse proxy and restrict network access to internal publishing
routes when exposing the service.

## Install a model

Choose a `model-<model_version>` tag from
[Trainer Releases](https://github.com/starcat-app/starcat-recsys-trainer/releases).
Review its model card, evaluation scope and checksums:

```bash
export MODEL_PUBLISH_KEY='the-publish-key-you-set'
bash scripts/install-model.sh --version YOUR_MODEL_VERSION
# Or use the model repository's latest release:
bash scripts/install-model.sh --version latest
```

The script downloads the model ZIP and checksum list, verifies SHA-256, installs and
activates it through the existing validated endpoint, then reads back the active version.
GitHub-generated Source code archives are not models. `latest` is unavailable until the
first public model Release; code Releases are not accepted as model tags.
For a fork, set `MODEL_RELEASE_REPOSITORY=YOUR_OWNER/YOUR_REPO`.

For an existing local archive, GitHub access is unnecessary:

```bash
bash scripts/install-model.sh --version YOUR_MODEL_VERSION \
  --file /path/to/YOUR_MODEL_VERSION.zip --sha256 THE_64_CHARACTER_SHA256
```

Use `--server http://127.0.0.1:PORT` for another instance. Aggregate deployments must
route `/internal/v1/model-bundles/*` correctly; the script sends `X-SC-Svc: recommend`.
Conflicting contents under an existing version are rejected. Validation precedes
installation and activation.

## Verify the active model

```bash
export API_KEY='the-query-key-you-set'
curl --fail -H "Authorization: Bearer $API_KEY" \
  http://127.0.0.1:5005/api/v2/version
curl --fail -H "Authorization: Bearer $API_KEY" \
  "http://127.0.0.1:5005/api/v2/repos/YOUR_REPO_ID/recommendations?limit=10&offset=0"
```

The version endpoint uses the query key and returns:

```json
{
  "schema_version": 1,
  "data": {
    "service": {"name": "recommend", "version": "0.0.0-dev"},
    "model": {
      "schema_version": 1,
      "model_version": "your-model",
      "selected_model": "hybrid_25",
      "created_at": "2026-11-17T03:00:00+00:00",
      "training": null,
      "evaluation": null,
      "metrics": {"recall@20": 0.03},
      "repositories": 100,
      "recommendation_edges": 1000,
      "database_bytes": 65536
    }
  }
}
```

Values above are illustrative. Actual responses also expose available source commit,
configuration hash, raw watermark, evaluation checksums and `metrics_scope`. Newly
trained bundles record actual training start/end, input range, counts and approved
parameters. Missing historical fields remain `null`. `created_at` is bundle creation,
and `pre_refit` metrics describe the evaluated candidate before final refitting.
An empty registry returns HTTP 200 with `model: null`; unauthorized requests return 401.

Use a real repository ID from the bundle to test recommendations. Process health does
not prove model installation. No third-party fallback runs without a model.
See the [model contract](https://github.com/starcat-app/starcat-recsys-trainer/blob/main/docs/MODEL_FORMAT.md).

## Run directly with Go

Use the toolchain declared by `go.mod`. No sibling `starcat-api-kit` checkout is required:

```bash
go mod download
go build -o bin/server ./cmd/server
./bin/server
```

Running from the repository root loads `.env` and stores data under `data/` by default.
For another working directory, configure absolute `MODEL_REGISTRY_DIR` and
`METRICS_STORE_FILE` paths writable by the service user. Use a process manager such as
systemd, persist and back up the registry and metrics. Model installation is identical.

## Train your own model

Follow the [training guide](https://github.com/starcat-app/starcat-recsys-trainer/blob/main/docs/TRAINING.md)
to acquire public data, train and prepare a ZIP, then use local archive installation.
Training and serving may run on separate machines. Serving does not read per-subject
training data.
