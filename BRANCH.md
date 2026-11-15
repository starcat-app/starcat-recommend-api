# 推荐依据专项分支

2026-11-15 创建 `codex/daily-recommendation-evidence`，基线 `dev@1ad1c21`。
Worktree：`Starcat-daily-recommendation-cards/supports/starcat-recommend-api`。
状态：已部署，待整合。`8b42f90` 已为每日推荐返回真实正向来源证据，保持原排序与分页契约；`make check`（含 race tests / vet / fmt）及 `make build` 通过。
2026-11-15 dong4j 单独授权部署，聚合仓 `5fde215` 锁定本仓 `02b748e`，通过固定提交导出后发布到既有 `starcat-api` release 45。线上清单、健康检查与 17 项接口检查通过，包含两页 10 条结果的来源证据、排序和分页验证；未更换活动模型。
完成后另行整合到本仓 dev；本次未推送源码或标签、未合并 dev，不激活生产模型。
