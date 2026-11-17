# 本地部署与预训练模型

[English](SELF_HOSTING.md)

推荐服务可以独立运行。使用已训练模型时，不需要 Python、GPU、Starcat 账号、
GH Archive Raw 或维护者的训练目录。

## Docker Compose

安装 Docker Compose、curl、jq，以及 `sha256sum` 或 macOS 自带的 `shasum`：

```bash
git clone https://github.com/starcat-app/starcat-recommend-api.git
cd starcat-recommend-api
cp .env.example .env
chmod 600 .env
```

编辑 `.env`，将 `API_KEYS` 与 `MODEL_PUBLISH_KEYS` 设置为两个不同的随机长密钥。
例如分别运行 `openssl rand -hex 32` 生成。前者供客户端查询，后者只供模型安装或
训练发布。不要把发布密钥交给客户端。`GITHUB_TOKENS` 可留空，Bundle 目录内的
推荐与内容检索仍可运行。

```bash
docker compose up -d --build
curl --fail http://127.0.0.1:5005/healthz
```

默认端口仅绑定本机，模型和调用指标保存在 `recommend-data` 命名卷。正常
`docker compose down` 保留数据；`down -v` 会删除数据。对外提供服务时自行配置
HTTPS 反向代理，并限制内部发布入口的网络访问。

## 下载并激活模型

模型来自 [Trainer Releases](https://github.com/starcat-app/starcat-recsys-trainer/releases)，
tag 为 `model-<model_version>`。查看模型卡、评估范围和 `SHA256SUMS`，选择所需版本：

```bash
export MODEL_PUBLISH_KEY='the-publish-key-you-set'
bash scripts/install-model.sh --version YOUR_MODEL_VERSION
# 或获取该模型仓库当前 latest：
bash scripts/install-model.sh --version latest
```

脚本下载 ZIP 与校验清单，核对 SHA-256，通过服务端校验和激活接口安装，再回读 active。
模型 ZIP 是 Release 中的同名资产，GitHub 的 Source code ZIP 不能导入。
首次公开模型发布前，`latest` 不可用；不要把仓库代码 Release 当模型使用。
Fork 用户可设置 `MODEL_RELEASE_REPOSITORY=YOUR_OWNER/YOUR_REPO`。

已有本地 ZIP 时，无需访问 GitHub：

```bash
bash scripts/install-model.sh --version YOUR_MODEL_VERSION \
  --file /path/to/YOUR_MODEL_VERSION.zip --sha256 THE_64_CHARACTER_SHA256
```

指定其他实例时添加 `--server http://127.0.0.1:PORT`。聚合部署需要正确路由
`/internal/v1/model-bundles/*`；脚本发送 `X-SC-Svc: recommend`。
已有版本内容不同会被拒绝，不覆盖活动模型。上传和激活只有全部验证成功后才完成。

## 检查版本与推荐

```bash
export API_KEY='the-query-key-you-set'
curl --fail -H "Authorization: Bearer $API_KEY" \
  http://127.0.0.1:5005/api/v2/version
curl --fail -H "Authorization: Bearer $API_KEY" \
  "http://127.0.0.1:5005/api/v2/repos/YOUR_REPO_ID/recommendations?limit=10&offset=0"
```

`GET /api/v2/version` 使用查询密钥，返回：

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

示例数值仅说明结构。实际还返回 Bundle 中已有的源码提交、配置摘要、Raw 水位、
评估摘要与 `metrics_scope`。新增训练产物记录实际训练起止时间、数据范围、计数
与白名单参数；旧模型缺失字段返回 `null`，不推算日期。
`created_at` 是打包时间；`pre_refit` 是重训前候选指标，不代表最终重训模型再次实测。
未激活模型时返回 HTTP 200 和 `model: null`；鉴权失败返回 401。

选择 Bundle 目录内的真实 repo ID 验证推荐。空模型没有第三方回退；不应把
`healthz` 成功当成模型已安装。模型内容和版本契约见
[Trainer 模型格式](https://github.com/starcat-app/starcat-recsys-trainer/blob/main/docs/MODEL_FORMAT.md)。

## Go 直接运行

使用 `go.mod` 指定的 Go 工具链，无需额外克隆 `starcat-api-kit`：

```bash
go mod download
go build -o bin/server ./cmd/server
./bin/server
```

在仓库根目录运行时自动读取 `.env`，数据默认位于 `data/`。其他工作目录应通过环境
配置绝对 `MODEL_REGISTRY_DIR` 与 `METRICS_STORE_FILE`，并赋予运行用户写权限。
使用 systemd 等进程管理器保持服务运行，并持久化、备份 Registry 和 metrics。
后续安装步骤与 Compose 一致。

## 自己训练

按 [Trainer 训练指南](https://github.com/starcat-app/starcat-recsys-trainer/blob/main/docs/TRAINING-ZH.md)
获取公开数据、训练并生成 ZIP，然后使用本地导入命令。训练器与查询服务可以放在不同
机器；服务端只读活动模型，不读取逐主体训练数据。
