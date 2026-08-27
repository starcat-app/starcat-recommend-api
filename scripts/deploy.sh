#!/usr/bin/env bash
# 仅发布源码 tag 与 GitHub Release。Starcat 官方 Fly 生产部署统一由 starcat-api 聚合仓执行。
set -euo pipefail

VERSION="${1:-}"
if [[ -z "${VERSION}" ]]; then
  echo "usage: ./scripts/deploy.sh vX.Y.Z"
  exit 1
fi

go test ./...
go test -race ./...
go vet ./...
go build ./...

git tag "${VERSION}"
git push origin "${VERSION}"
