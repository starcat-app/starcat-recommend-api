# TODO

- Add persistent cache if production traffic requires cross-instance reuse.
- **下一次包含双渠道 v2 的 Starcat App 版本上线之后，删除 v1 推荐兼容逻辑。**
  本地构建或仅部署后端不算 App 上线；本轮先保留旧客户端入口。届时删除
  `GET /api/v1/repos/{repo_id}/recommendations`、`RecommendHandler` 适配器、
  `server/legacy_test.go` 及相关兼容说明/统计字段；保留 `/api/v1/ping` 等无关
  通用服务接口。核对对应版本已上线，再按独立任务测试、部署和验收。
